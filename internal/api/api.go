package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/security"
)

type DistributedRateLimiter interface {
	Allow(context.Context, string, string, int) (bool, int, error)
	Ping(context.Context) error
}

type Server struct {
	DB                                  *pgxpool.Pool
	DistributedLimiter                  DistributedRateLimiter
	EncryptionKey                       []byte
	EncryptionKeyID                     string
	DemoReceiverOrigin                  string
	AllowHTTPTest                       bool
	EventRateLimit, ManagementRateLimit int
	limitMu                             sync.Mutex
	limitEntries                        map[string]rateEntry
	lastLimitCleanup                    time.Time
	rateNow                             func() time.Time
}
type principal struct {
	KeyID  string
	OrgID  string
	Role   string
	Scopes map[string]bool
}
type contextKey int

const principalKey contextKey = 1

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/health/ready", s.ready)
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.authenticate)
		r.Use(s.rateLimit)
		r.With(require("events:write")).Post("/events", s.createEvent)
		r.With(require("events:read")).Get("/events", s.listEvents)
		r.With(require("events:read")).Get("/events/{id}", s.getEvent)
		r.With(require("endpoints:write")).Post("/endpoints", s.createEndpoint)
		r.With(require("endpoints:read")).Get("/endpoints", s.listEndpoints)
		r.With(require("endpoints:read")).Get("/endpoints/{id}", s.getEndpoint)
		r.With(require("endpoints:write")).Patch("/endpoints/{id}", s.patchEndpoint)
		r.With(require("endpoints:write")).Delete("/endpoints/{id}", s.deleteEndpoint)
		r.With(require("endpoints:write")).Post("/endpoints/{id}/rotate-secret", s.rotateEndpointSecret)
		r.With(require("endpoints:write")).Post("/endpoints/{id}/verify", s.verifyEndpoint)
		r.With(require("deliveries:read")).Get("/deliveries", s.listDeliveries)
		r.With(require("deliveries:read")).Get("/deliveries/{id}", s.getDelivery)
		r.With(require("deliveries:write")).Post("/deliveries/{id}/replay", s.replayDelivery)
		r.With(require("deliveries:write")).Patch("/deliveries/{id}/review", s.reviewDelivery)
		r.With(require("metrics:read")).Get("/metrics/summary", s.metricsSummary)
		r.With(require("metrics:read")).Get("/analytics/deliveries", s.deliveryAnalytics)
		r.With(require("team:read")).Get("/team/members", s.listTeamMembers)
		r.With(require("team:write")).Post("/team/members", s.createTeamMember)
		r.With(require("team:write")).Patch("/team/members/{id}", s.patchTeamMember)
	})
	return otelhttp.NewHandler(r, "http.server")
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if !health.Ready(ctx, s.DB) {
		problem(w, 503, "storage_unavailable", "Database is not ready")
		return
	}
	if s.DistributedLimiter != nil && s.DistributedLimiter.Ping(ctx) != nil {
		problem(w, 503, "rate_limit_unavailable", "Rate limiter is not ready")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			problem(w, 401, "unauthorized", "Bearer API key required")
			return
		}
		prefix, err := security.KeyPrefix(parts[1])
		if err != nil {
			problem(w, 401, "unauthorized", "Invalid API key")
			return
		}
		var hash []byte
		var orgID, keyID string
		var scopes []string
		var role string
		err = s.DB.QueryRow(r.Context(), `SELECT k.id,k.key_hash,k.organization_id,k.scopes,COALESCE(m.role,'owner') FROM api_keys k JOIN organizations o ON o.id=k.organization_id LEFT JOIN team_members m ON m.id=k.member_id AND m.organization_id=k.organization_id WHERE k.key_prefix=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND o.status='active' AND (m.id IS NULL OR m.status='active')`, prefix).Scan(&keyID, &hash, &orgID, &scopes, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 401, "unauthorized", "Invalid API key")
			return
		}
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		if subtle.ConstantTimeCompare(security.Hash(parts[1]), hash) != 1 {
			problem(w, 401, "unauthorized", "Invalid API key")
			return
		}
		set := make(map[string]bool, len(scopes))
		for _, scope := range scopes {
			set[scope] = roleAllows(role, scope)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal{KeyID: keyID, OrgID: orgID, Role: role, Scopes: set})))
	})
}

type rateEntry struct {
	window int64
	count  int
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.Context().Value(principalKey).(principal)
		category := "management"
		limit := s.ManagementRateLimit
		if r.Method == http.MethodPost && r.URL.Path == "/v1/events" {
			category = "events"
			limit = s.EventRateLimit
		}
		if limit <= 0 {
			if category == "events" {
				limit = 600
			} else {
				limit = 120
			}
		}
		if s.DistributedLimiter != nil {
			allowed, retryAfter, err := s.DistributedLimiter.Allow(r.Context(), p.KeyID, category, limit)
			if err == nil {
				if !allowed {
					w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
					problem(w, 429, "rate_limited", "Request rate limit exceeded")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// Writes fail closed; reads retain a bounded local fallback.
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				problem(w, 503, "rate_limit_unavailable", "Rate limiter unavailable")
				return
			}
		}
		now := time.Now()
		if s.rateNow != nil {
			now = s.rateNow()
		}
		window := now.Unix() / 60
		key := p.KeyID + ":" + category
		s.limitMu.Lock()
		if s.limitEntries == nil {
			s.limitEntries = make(map[string]rateEntry)
		}
		if now.Sub(s.lastLimitCleanup) >= time.Minute {
			for k, v := range s.limitEntries {
				if v.window < window {
					delete(s.limitEntries, k)
				}
			}
			s.lastLimitCleanup = now
		}
		entry := s.limitEntries[key]
		if entry.window != window {
			entry = rateEntry{window: window}
		}
		allowed := entry.count < limit
		if allowed {
			entry.count++
			s.limitEntries[key] = entry
		}
		s.limitMu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", strconv.FormatInt(60-now.Unix()%60, 10))
			problem(w, 429, "rate_limited", "Request rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func require(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := r.Context().Value(principalKey).(principal)
			if !ok || !p.Scopes[scope] {
				problem(w, 403, "forbidden", "API key lacks required scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func tenant(r *http.Request) string { return r.Context().Value(principalKey).(principal).OrgID }

func decode(w http.ResponseWriter, r *http.Request, dst any, max int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		decodeError(w, err)
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		problem(w, 400, "invalid_json", "Expected one JSON object")
		return false
	}
	return true
}

func decodeError(w http.ResponseWriter, err error) {
	var sizeErr *http.MaxBytesError
	if errors.As(err, &sizeErr) {
		problem(w, 413, "payload_too_large", "Request body too large")
		return
	}
	problem(w, 400, "invalid_json", "Invalid JSON request")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": http.StatusText(status), "status": status, "code": code, "detail": detail})
}

func fingerprint(eventType string, canonical []byte) []byte {
	h := sha256.Sum256(append(append([]byte(eventType), 0), canonical...))
	return h[:]
}

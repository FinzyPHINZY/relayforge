package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/security"
)

type Server struct {
	DB              *pgxpool.Pool
	EncryptionKey   []byte
	EncryptionKeyID string
}
type principal struct {
	OrgID  string
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
		r.With(require("events:write")).Post("/events", s.createEvent)
		r.With(require("events:read")).Get("/events", s.listEvents)
		r.With(require("events:read")).Get("/events/{id}", s.getEvent)
		r.With(require("endpoints:write")).Post("/endpoints", s.createEndpoint)
		r.With(require("endpoints:read")).Get("/endpoints", s.listEndpoints)
		r.With(require("endpoints:read")).Get("/endpoints/{id}", s.getEndpoint)
		r.With(require("endpoints:write")).Patch("/endpoints/{id}", s.patchEndpoint)
		r.With(require("endpoints:write")).Delete("/endpoints/{id}", s.deleteEndpoint)
	})
	return r
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	var version int
	if err := s.DB.QueryRow(ctx, `SELECT version FROM schema_migrations WHERE version = 1`).Scan(&version); err != nil {
		problem(w, 503, "storage_unavailable", "Database is not ready")
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
		var orgID string
		var scopes []string
		err = s.DB.QueryRow(r.Context(), `SELECT k.key_hash, k.organization_id, k.scopes FROM api_keys k JOIN organizations o ON o.id=k.organization_id WHERE k.key_prefix=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND o.status='active'`, prefix).Scan(&hash, &orgID, &scopes)
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
			set[scope] = true
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, principal{OrgID: orgID, Scopes: set})))
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

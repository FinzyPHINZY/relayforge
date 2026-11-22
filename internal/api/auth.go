package api

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"webhook_delivery/internal/accounts"
)

const defaultOAuthStartPerIPPerHour = 30

func (s *Server) authRoutes(r chi.Router) {
	r.Use(s.requireDashboardToken)
	r.Use(s.regionWrites)
	r.Get("/providers", s.authProviders)
	r.Group(func(r chi.Router) {
		r.Use(s.requireProvider(s.emailEnabled))
		r.Post("/email/start", s.emailStart)
		r.Post("/email/consume", s.emailConsume)
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireProvider(s.githubEnabled))
		r.Post("/github/start", s.githubStart)
		r.Post("/github/callback", s.githubCallback)
	})
}

func (s *Server) requireDashboardToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validDashboardToken(r, s.DashboardToken) {
			problem(w, 401, "dashboard_token_required", "Valid dashboard token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireProvider(enabled func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled() {
				problem(w, 404, "provider_disabled", "Sign-in provider is not enabled")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// emailEnabled requires a dashboard URL, IP hash key and dashboard token.
func (s *Server) emailEnabled() bool {
	return s.Accounts != nil && len(s.Accounts.IPHashKey) > 0 && s.MagicLinks.DashboardURL != "" && s.MagicLinks.Mailer != nil && s.DashboardToken != ""
}

func (s *Server) githubEnabled() bool { return s.emailEnabled() && s.GitHub != nil }

func (s *Server) authProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]bool{"github": s.githubEnabled(), "email": s.emailEnabled()})
}

func rateLimited(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	problem(w, 429, "rate_limited", "Too many sign-in attempts")
}

func (s *Server) emailStart(w http.ResponseWriter, r *http.Request) {
	if s.MagicLinks.Limiter == nil {
		problem(w, 503, "rate_limit_unavailable", "Rate limiter unavailable")
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	err := s.Accounts.RequestMagicLink(r.Context(), s.MagicLinks, in.Email, clientIP(r, s.DashboardToken))
	var rl accounts.ErrRateLimited
	switch {
	case err == nil:
	case errors.Is(err, accounts.ErrInvalidEmail):
		problem(w, 422, "invalid_email", "A valid email address is required")
		return
	case errors.As(err, &rl):
		rateLimited(w, rl.RetryAfter)
		return
	case errors.Is(err, accounts.ErrLimiterUnavailable):
		problem(w, 503, "rate_limit_unavailable", "Rate limiter unavailable")
		return
	case errors.Is(err, accounts.ErrMailSend):
		// The link is stored; report success so the response reveals nothing.
		// Never log err text: provider errors can echo the recipient address.
		slog.ErrorContext(r.Context(), "magic link mail send failed", "error_code", errorCode(err))
	default:
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 202, map[string]any{})
}

// errorCode returns an AWS/smithy-style API error code when present.
func errorCode(err error) string {
	var api interface{ ErrorCode() string }
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	return "unknown"
}

func (s *Server) emailConsume(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	token, sess, u, err := s.Accounts.ConsumeMagicLink(r.Context(), in.Token, clientIP(r, s.DashboardToken), r.UserAgent())
	switch {
	case err == nil:
		writeSession(w, token, sess, u)
	case errors.Is(err, accounts.ErrLinkInvalid), errors.Is(err, accounts.ErrUserInactive), errors.Is(err, accounts.ErrIdentityConflict):
		problem(w, 400, "link_invalid", "Sign-in link is invalid or expired")
	default:
		problem(w, 503, "storage_unavailable", "Storage unavailable")
	}
}

func (s *Server) githubStart(w http.ResponseWriter, r *http.Request) {
	if s.MagicLinks.Limiter == nil {
		problem(w, 503, "rate_limit_unavailable", "Rate limiter unavailable")
		return
	}
	ip := clientIP(r, s.DashboardToken)
	if ip == "" {
		ip = "unknown"
	}
	limit := s.OAuthStartPerIPPerHour
	if limit <= 0 {
		limit = defaultOAuthStartPerIPPerHour
	}
	ok, retry, err := s.MagicLinks.Limiter.AllowWindow(r.Context(), hex.EncodeToString(s.Accounts.HashIP(ip)), "oauth_ip", limit, time.Hour)
	if err != nil {
		problem(w, 503, "rate_limit_unavailable", "Rate limiter unavailable")
		return
	}
	if !ok {
		rateLimited(w, retry)
		return
	}
	st, err := s.GitHub.Start()
	if err != nil {
		problem(w, 503, "storage_unavailable", "Could not start sign-in")
		return
	}
	writeJSON(w, 200, map[string]string{"authorize_url": st.AuthorizeURL, "state": st.State, "verifier": st.Verifier})
}

func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}
	if !decode(w, r, &in, 4<<10) {
		return
	}
	if in.Code == "" || in.Verifier == "" {
		problem(w, 400, "github_failed", "GitHub sign-in failed")
		return
	}
	profile, err := s.GitHub.Exchange(r.Context(), in.Code, in.Verifier)
	if err != nil {
		problem(w, 400, "github_failed", "GitHub sign-in failed")
		return
	}
	token, sess, u, err := s.Accounts.SignInWithGitHub(r.Context(), profile, clientIP(r, s.DashboardToken), r.UserAgent())
	switch {
	case err == nil:
		writeSession(w, token, sess, u)
	case errors.Is(err, accounts.ErrGitHubEmailUnverified):
		problem(w, 403, "github_email_unverified", "GitHub account has no verified primary email")
	case errors.Is(err, accounts.ErrUserInactive), errors.Is(err, accounts.ErrIdentityConflict):
		problem(w, 400, "github_failed", "GitHub sign-in failed")
	default:
		problem(w, 503, "storage_unavailable", "Storage unavailable")
	}
}

func writeSession(w http.ResponseWriter, token string, sess accounts.Session, u accounts.User) {
	writeJSON(w, 200, map[string]any{
		"session_token": token,
		"expires_at":    sess.ExpiresAt.UTC().Format(time.RFC3339),
		"user":          map[string]string{"id": u.ID, "email": u.Email, "display_name": u.DisplayName},
	})
}

package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/accounts"
	"webhook_delivery/internal/audit"
	"webhook_delivery/internal/id"
)

const (
	sessionTokenPrefix          = "rfs_"
	orgHeader                   = "X-RelayForge-Org"
	defaultMaxWorkspacesPerUser = 3
	maxWorkspaceName            = 80
	maxMemberName               = 120
)

func (s *Server) authenticateSession(w http.ResponseWriter, r *http.Request, token string, next http.Handler) {
	if s.Accounts == nil {
		problem(w, 401, "unauthorized", "Invalid session")
		return
	}
	sess, u, err := s.Accounts.LookupSession(r.Context(), token)
	if err != nil {
		problem(w, 401, "unauthorized", "Invalid session")
		return
	}
	p := principal{UserID: u.ID, SessionID: sess.ID, Scopes: map[string]bool{}, user: u}
	// account routes are user-scoped and ignore the header; /v1/me reports the
	// membership when there is one. neither may fail on a stale header.
	accountRoute := r.URL.Path == "/v1/account" || strings.HasPrefix(r.URL.Path, "/v1/account/")
	lenient := r.URL.Path == "/v1/me"
	if org := r.Header.Get(orgHeader); org != "" && !accountRoute {
		if !id.Valid(org) {
			if lenient {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
				return
			}
			problem(w, 400, "invalid_org", "X-RelayForge-Org must be a workspace id")
			return
		}
		var role string
		err := s.DB.QueryRow(r.Context(), `SELECT m.role FROM team_members m JOIN organizations o ON o.id=m.organization_id
			WHERE m.user_id=$1 AND m.organization_id=$2 AND m.status='active' AND o.status='active'`, u.ID, org).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			if lenient {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
				return
			}
			problem(w, 403, "not_a_member", "Not an active member of this workspace")
			return
		}
		if err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		p.OrgID, p.Role = org, role
		for _, scope := range allScopes {
			if roleAllows(role, scope) {
				p.Scopes[scope] = true
			}
		}
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
}

func requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(principalKey).(principal).SessionID == "" {
			problem(w, 403, "session_required", "This route requires a console session")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accountRoutes(r chi.Router) {
	r.Use(requireSession)
	r.Get("/", s.getAccount)
	r.Post("/workspaces", s.createWorkspace)
	r.Get("/sessions", s.listAccountSessions)
	r.Delete("/sessions/{id}", s.deleteAccountSession)
	r.Post("/sign-out", s.signOut)
}

type workspaceView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
	Plan string `json:"plan"`
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	rows, err := s.DB.Query(r.Context(), `SELECT o.id,o.name,m.role,o.plan FROM team_members m JOIN organizations o ON o.id=m.organization_id
		WHERE m.user_id=$1 AND m.status='active' AND o.status='active' ORDER BY o.created_at,o.id`, p.UserID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	workspaces := []workspaceView{}
	for rows.Next() {
		var ws workspaceView
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.Role, &ws.Plan); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		workspaces = append(workspaces, ws)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{
		"user":       map[string]string{"id": p.UserID, "email": p.user.Email, "display_name": p.user.DisplayName, "avatar_url": p.user.AvatarURL},
		"workspaces": workspaces,
	})
}

func memberName(displayName, email string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	if utf8.RuneCountInString(name) > maxMemberName {
		name = string([]rune(name)[:maxMemberName])
	}
	return name
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in, 2048) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > maxWorkspaceName {
		problem(w, 422, "invalid_workspace", "Workspace name must be 1 to 80 characters")
		return
	}
	limit := s.MaxWorkspacesPerUser
	if limit <= 0 {
		limit = defaultMaxWorkspacesPerUser
	}
	orgID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	memberID, err := id.New()
	if err != nil {
		problem(w, 503, "internal_error", "ID generation failed")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer tx.Rollback(ctx)
	// locking the user row serialises concurrent creations for the limit check.
	var email, displayName string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(email,''),COALESCE(display_name,'') FROM users WHERE id=$1 AND status='active' FOR UPDATE`, p.UserID).Scan(&email, &displayName); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if email == "" {
		problem(w, 422, "email_required", "An email address is required to create a workspace")
		return
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE created_by_user_id=$1 AND status<>'deleted'`, p.UserID).Scan(&count); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if count >= limit {
		problem(w, 429, "workspace_limit", "Workspace limit reached")
		return
	}
	var plan string
	if err = tx.QueryRow(ctx, `INSERT INTO organizations (id,name,created_by_user_id) VALUES ($1,$2,$3) RETURNING plan`, orgID, in.Name, p.UserID).Scan(&plan); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO team_members (id,organization_id,name,email,role,user_id) VALUES ($1,$2,$3,$4,'owner',$5)`, memberID, orgID, memberName(displayName, email), email, p.UserID); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = audit.Write(ctx, tx, audit.Entry{OrganizationID: orgID, ActorUserID: p.UserID, ActorRole: "owner",
		Action: "organization.created_by_user", TargetType: "organization", TargetID: orgID}); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 201, workspaceView{ID: orgID, Name: in.Name, Role: "owner", Plan: plan})
}

type sessionView struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	UserAgent  string    `json:"user_agent"`
	Current    bool      `json:"current"`
}

func (s *Server) listAccountSessions(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	list, err := s.Accounts.ListSessions(r.Context(), p.UserID)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	out := make([]sessionView, 0, len(list))
	for _, si := range list {
		out = append(out, sessionView{ID: si.ID, CreatedAt: si.CreatedAt, LastSeenAt: si.LastSeenAt, ExpiresAt: si.ExpiresAt, UserAgent: si.UserAgent, Current: si.ID == p.SessionID})
	}
	writeJSON(w, 200, map[string]any{"data": out})
}

func (s *Server) deleteAccountSession(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	err := s.Accounts.RevokeSession(r.Context(), p.UserID, chi.URLParam(r, "id"))
	switch {
	case err == nil:
		w.WriteHeader(204)
	case errors.Is(err, accounts.ErrNotFound):
		problem(w, 404, "not_found", "Session not found")
	default:
		problem(w, 503, "storage_unavailable", "Storage unavailable")
	}
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	p := r.Context().Value(principalKey).(principal)
	err := s.Accounts.SignOut(r.Context(), p.UserID, p.SessionID)
	if err != nil && !errors.Is(err, accounts.ErrNotFound) {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	w.WriteHeader(204)
}

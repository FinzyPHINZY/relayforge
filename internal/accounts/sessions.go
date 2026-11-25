package accounts

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/audit"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

const (
	SessionAbsolute = 30 * 24 * time.Hour
	SessionIdle     = 7 * 24 * time.Hour
	touchInterval   = 5 * time.Minute
	sessionPrefix   = "rfs_"
	sessionTokenLen = 47
	maxUserAgent    = 256
)

type Session struct {
	ID, UserID string
	ExpiresAt  time.Time
}

type SessionInfo struct {
	ID                               string
	CreatedAt, LastSeenAt, ExpiresAt time.Time
	UserAgent                        string
}

func truncateUA(ua string) string {
	if !utf8.ValidString(ua) {
		ua = strings.ToValidUTF8(ua, "")
	}
	if utf8.RuneCountInString(ua) <= maxUserAgent {
		return ua
	}
	return string([]rune(ua)[:maxUserAgent])
}

func (s *Store) CreateSession(ctx context.Context, tx pgx.Tx, userID, ip, userAgent string) (string, Session, error) {
	token, err := security.NewToken(sessionPrefix)
	if err != nil {
		return "", Session{}, err
	}
	sid, err := id.New()
	if err != nil {
		return "", Session{}, err
	}
	now := s.now()
	sess := Session{ID: sid, UserID: userID, ExpiresAt: now.Add(SessionAbsolute)}
	var ua *string
	if userAgent != "" {
		v := truncateUA(userAgent)
		ua = &v
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (id, user_id, token_hash, created_at, last_seen_at, expires_at, ip_hash, user_agent)
		VALUES ($1,$2,$3,$4,$4,$5,$6,$7)`, sid, userID, security.Hash(token), now, sess.ExpiresAt, s.HashIP(ip), ua); err != nil {
		return "", Session{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET last_sign_in_at=$2 WHERE id=$1`, userID, now); err != nil {
		return "", Session{}, err
	}
	return token, sess, nil
}

func (s *Store) LookupSession(ctx context.Context, token string) (Session, User, error) {
	if !strings.HasPrefix(token, sessionPrefix) || len(token) != sessionTokenLen {
		return Session{}, User{}, ErrSessionInvalid
	}
	now := s.now()
	var (
		sess     Session
		u        User
		lastSeen time.Time
		verified *time.Time
	)
	err := s.DB.QueryRow(ctx, `SELECT s.id, s.user_id, s.expires_at, s.last_seen_at,
			COALESCE(u.email,''), u.email_verified_at, COALESCE(u.display_name,''), COALESCE(u.avatar_url,''), u.kind, u.status
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at > $2 AND s.last_seen_at >= $3 AND u.status='active'`,
		security.Hash(token), now, now.Add(-SessionIdle)).
		Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &lastSeen, &u.Email, &verified, &u.DisplayName, &u.AvatarURL, &u.Kind, &u.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, User{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, User{}, err
	}
	u.ID = sess.UserID
	u.EmailVerified = verified != nil
	// the touch is best-effort: the session is already proven valid, and a failed write
	// only delays the idle-timeout extension to the next request.
	if now.Sub(lastSeen) > touchInterval {
		if _, err := s.DB.Exec(ctx, `UPDATE sessions SET last_seen_at=$2 WHERE id=$1 AND last_seen_at < $2`, sess.ID, now); err != nil {
			slog.Debug("session last_seen_at touch failed", "session_id", sess.ID, "error", err)
		}
	}
	return sess, u, nil
}

func (s *Store) RevokeSession(ctx context.Context, userID, sessionID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sessionID, userID, s.now())
	if err != nil {
		if isInvalidUUID(err) {
			return ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := auditUserEvent(ctx, tx, userID, "session.revoked", "session", sessionID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	now := s.now()
	rows, err := s.DB.Query(ctx, `SELECT id, created_at, last_seen_at, expires_at, COALESCE(user_agent,'')
		FROM sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > $2 AND last_seen_at >= $3
		ORDER BY created_at DESC, id DESC`, userID, now, now.Add(-SessionIdle))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var si SessionInfo
		if err := rows.Scan(&si.ID, &si.CreatedAt, &si.LastSeenAt, &si.ExpiresAt, &si.UserAgent); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *Store) SignOut(ctx context.Context, userID, sessionID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$3 WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sessionID, userID, s.now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := auditUserEvent(ctx, tx, userID, "user.signed_out", "user", userID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func auditSignIn(ctx context.Context, tx pgx.Tx, userID, method string) error {
	return auditUserEvent(ctx, tx, userID, "user.signed_in", "user", userID, map[string]any{"method": method})
}

func auditUserEvent(ctx context.Context, tx pgx.Tx, userID, action, targetType, targetID string, details map[string]any) error {
	rows, err := tx.Query(ctx, `SELECT m.organization_id, m.role FROM team_members m JOIN organizations o ON o.id = m.organization_id
		WHERE m.user_id=$1 AND m.status='active' AND o.status='active' ORDER BY m.organization_id`, userID)
	if err != nil {
		return err
	}
	type membership struct{ org, role string }
	var ms []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.org, &m.role); err != nil {
			rows.Close()
			return err
		}
		ms = append(ms, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range ms {
		if err := audit.Write(ctx, tx, audit.Entry{OrganizationID: m.org, ActorUserID: userID, ActorRole: m.role, Action: action,
			TargetType: targetType, TargetID: targetID, Details: details}); err != nil {
			return err
		}
	}
	return nil
}

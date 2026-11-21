package accounts

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/mail"
	"webhook_delivery/internal/security"
)

const (
	MagicLinkTTL    = 15 * time.Minute
	magicLinkPrefix = "rfm_"
	magicLinkLen    = 47
)

var (
	ErrInvalidEmail = errors.New("invalid email")
	ErrLinkInvalid  = errors.New("magic link invalid")
)

// ErrRateLimited reports a limited request; use errors.As.
type ErrRateLimited struct{ RetryAfter int }

func (e ErrRateLimited) Error() string {
	return "rate limited; retry after " + strconv.Itoa(e.RetryAfter) + "s"
}

type WindowLimiter interface {
	AllowWindow(ctx context.Context, subject, category string, limit int, window time.Duration) (bool, int, error)
}

type MagicLinkConfig struct {
	DashboardURL    string
	Mailer          mail.Mailer
	Limiter         WindowLimiter
	PerEmailPerHour int
	PerIPPerHour    int
}

// RequestMagicLink rate limits, stores a single-use link and mails it. It
// returns nil whether or not a user exists for the address.
func (s *Store) RequestMagicLink(ctx context.Context, cfg MagicLinkConfig, rawEmail, ip string) error {
	email, err := NormaliseEmail(rawEmail)
	if err != nil {
		return ErrInvalidEmail
	}
	if len(s.IPHashKey) == 0 {
		return errors.New("accounts: IPHashKey is required for magic-link requests")
	}
	// Fail closed: requests with no IP share one bucket rather than skipping the limit.
	ipSubject := ip
	if ipSubject == "" {
		ipSubject = "unknown"
	}
	ok, retry, err := cfg.Limiter.AllowWindow(ctx, hex.EncodeToString(s.HashIP(ipSubject)), "magic_link_ip", cfg.PerIPPerHour, time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRateLimited{RetryAfter: retry}
	}
	ok, retry, err = cfg.Limiter.AllowWindow(ctx, hex.EncodeToString(security.Hash(email)), "magic_link_email", cfg.PerEmailPerHour, time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRateLimited{RetryAfter: retry}
	}
	token, err := security.NewToken(magicLinkPrefix)
	if err != nil {
		return err
	}
	lid, err := id.New()
	if err != nil {
		return err
	}
	now := s.now()
	if _, err := s.DB.Exec(ctx, `INSERT INTO magic_links (id, email, token_hash, purpose, expires_at, requested_ip_hash, created_at)
		VALUES ($1,$2,$3,'sign_in',$4,$5,$6)`, lid, email, security.Hash(token), now.Add(MagicLinkTTL), s.HashIP(ip), now); err != nil {
		return err
	}
	link := strings.TrimRight(cfg.DashboardURL, "/") + "/auth/email?token=" + token
	return cfg.Mailer.Send(ctx, mail.SignInMessage(email, link))
}

// ConsumeMagicLink atomically spends the link, finds or creates the user and
// opens a session.
func (s *Store) ConsumeMagicLink(ctx context.Context, token, ip, userAgent string) (string, User, error) {
	if !strings.HasPrefix(token, magicLinkPrefix) || len(token) != magicLinkLen {
		return "", User{}, ErrLinkInvalid
	}
	now := s.now()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var email string
	err = tx.QueryRow(ctx, `UPDATE magic_links SET consumed_at=$2 WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at > $2 RETURNING email`,
		security.Hash(token), now).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", User{}, ErrLinkInvalid
	}
	if err != nil {
		return "", User{}, err
	}
	uid, err := id.New()
	if err != nil {
		return "", User{}, err
	}
	local, _, _ := strings.Cut(email, "@")
	// Insert-if-absent, then lock the row so concurrent first sign-ins converge on one user.
	if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, display_name, kind, created_at) VALUES ($1,$2,$3,'regular',$4)
		ON CONFLICT (email) DO NOTHING`, uid, email, local, now); err != nil {
		return "", User{}, err
	}
	u := User{Email: email}
	var avatar, display *string
	err = tx.QueryRow(ctx, `UPDATE users SET email_verified_at = COALESCE(email_verified_at, $2) WHERE email=$1 AND status='active'
		RETURNING id, display_name, avatar_url, kind, status`, email, now).Scan(&u.ID, &display, &avatar, &u.Kind, &u.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", User{}, ErrLinkInvalid // user is deleting or deleted
	}
	if err != nil {
		return "", User{}, err
	}
	if display != nil {
		u.DisplayName = *display
	}
	if avatar != nil {
		u.AvatarURL = *avatar
	}
	u.EmailVerified = true
	iid, err := id.New()
	if err != nil {
		return "", User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_identities (id, user_id, provider, provider_subject, created_at) VALUES ($1,$2,'email',$3,$4)
		ON CONFLICT (provider, provider_subject) DO NOTHING`, iid, u.ID, email, now); err != nil {
		return "", User{}, err
	}
	sessionToken, _, err := s.CreateSession(ctx, tx, u.ID, ip, userAgent)
	if err != nil {
		return "", User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", User{}, err
	}
	return sessionToken, u, nil
}

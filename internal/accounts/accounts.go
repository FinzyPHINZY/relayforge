// Package accounts holds user accounts and server-side sessions.
package accounts

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSessionInvalid = errors.New("session invalid")
	ErrNotFound       = errors.New("not found")
)

type Store struct {
	DB        *pgxpool.Pool
	IPHashKey []byte
	Now       func() time.Time
}

type User struct {
	ID, Email, DisplayName, AvatarURL, Kind, Status string
	EmailVerified                                   bool
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// NormaliseEmail trims, lowercases and requires a bare address (no display name).
func NormaliseEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" || len(e) > 254 {
		return "", errors.New("invalid email")
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e || strings.Count(e, "@") != 1 {
		return "", errors.New("invalid email")
	}
	return e, nil
}

// HashIP returns HMAC-SHA256(IPHashKey, ip), or nil for an empty IP.
func (s *Store) HashIP(ip string) []byte {
	if ip == "" {
		return nil
	}
	m := hmac.New(sha256.New, s.IPHashKey)
	m.Write([]byte(ip))
	return m.Sum(nil)
}

func isInvalidUUID(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "22P02"
}

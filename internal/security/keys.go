package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

func NewToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func KeyPrefix(raw string) (string, error) {
	if !strings.HasPrefix(raw, "whk_") || len(raw) != 47 {
		return "", errors.New("invalid API key")
	}
	return raw[:16], nil
}

func Hash(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

func EqualHash(raw string, stored []byte) bool {
	return subtle.ConstantTimeCompare(Hash(raw), stored) == 1
}

package config

import (
	"encoding/base64"
	"errors"
	"os"
)

type Config struct {
	DatabaseURL, ListenAddr, EncryptionKeyID string
	EncryptionKey                            []byte
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), ListenAddr: os.Getenv("LISTEN_ADDR"), EncryptionKeyID: os.Getenv("ENCRYPTION_KEY_ID")}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.EncryptionKeyID == "" {
		c.EncryptionKeyID = "local-v1"
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	var err error
	c.EncryptionKey, err = base64.StdEncoding.DecodeString(os.Getenv("ENCRYPTION_KEY_BASE64"))
	if err != nil || len(c.EncryptionKey) != 32 {
		return c, errors.New("ENCRYPTION_KEY_BASE64 must encode 32 bytes")
	}
	return c, nil
}

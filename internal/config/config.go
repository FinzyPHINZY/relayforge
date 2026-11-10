package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL, ListenAddr, EncryptionKeyID, RedisURL string
	EncryptionKey                                      []byte
	EventRateLimit, ManagementRateLimit                int
	DemoReceiverOrigin                                 string
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), ListenAddr: os.Getenv("LISTEN_ADDR"), EncryptionKeyID: os.Getenv("ENCRYPTION_KEY_ID"), RedisURL: os.Getenv("REDIS_URL")}
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
	c.EventRateLimit, err = positiveEnv("EVENT_RATE_LIMIT_PER_MINUTE", 600)
	if err != nil {
		return c, err
	}
	c.ManagementRateLimit, err = positiveEnv("MANAGEMENT_RATE_LIMIT_PER_MINUTE", 120)
	if err != nil {
		return c, err
	}
	c.DemoReceiverOrigin = os.Getenv("DEMO_RECEIVER_ORIGIN")
	if c.DemoReceiverOrigin != "" && c.DemoReceiverOrigin != "http://receiver:8090" {
		return c, errors.New("DEMO_RECEIVER_ORIGIN must be http://receiver:8090 or empty")
	}
	return c, nil
}

func positiveEnv(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100000 {
		return 0, errors.New(name + " must be between 1 and 100000")
	}
	return n, nil
}

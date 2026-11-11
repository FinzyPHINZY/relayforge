// Package relayforge provides a small publisher client for the RelayForge API.
package relayforge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	base    *url.URL
	key     string
	http    *http.Client
	retries int
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.http = client
		}
	}
}

func WithRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 && n <= 10 {
			c.retries = n
		}
	}
}

func New(baseURL, apiKey string, options ...Option) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid RelayForge URL")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return nil, errors.New("RelayForge URL must use HTTPS outside localhost")
	}
	if !strings.HasPrefix(apiKey, "whk_") || len(apiKey) > 512 {
		return nil, errors.New("invalid API key")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	c := &Client{base: u, key: apiKey, http: &http.Client{Timeout: 10 * time.Second}, retries: 2}
	for _, option := range options {
		option(c)
	}
	return c, nil
}

type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("RelayForge: HTTP %d: %s", e.StatusCode, e.Message)
}

// Publish uses a stable idempotency key for every attempt. Pass an existing
// key when retrying a call after the process exits or its result is unknown.
func (c *Client) Publish(ctx context.Context, eventType string, data any, idempotencyKey string) (Event, error) {
	if eventType == "" {
		return Event{}, errors.New("event type required")
	}
	if idempotencyKey == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return Event{}, err
		}
		idempotencyKey = hex.EncodeToString(random[:])
	}
	if len(idempotencyKey) > 255 {
		return Event{}, errors.New("idempotency key too long")
	}
	body, err := json.Marshal(struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}{eventType, data})
	if err != nil {
		return Event{}, err
	}
	if len(body) > 270*1024 {
		return Event{}, errors.New("event body too large")
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/events"
	var last error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Event{}, ctx.Err()
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return Event{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		resp, err := c.http.Do(req)
		if err != nil {
			last = err
			if ctx.Err() != nil {
				return Event{}, ctx.Err()
			}
			continue
		}
		content, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			return Event{}, readErr
		}
		if resp.StatusCode == http.StatusAccepted {
			var event Event
			if err = json.Unmarshal(content, &event); err != nil || event.ID == "" {
				return Event{}, errors.New("invalid acceptance response")
			}
			return event, nil
		}
		var problem struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(content, &problem)
		if problem.Detail == "" {
			problem.Detail = http.StatusText(resp.StatusCode)
		}
		last = &APIError{StatusCode: resp.StatusCode, Code: problem.Code, Message: problem.Detail}
		if resp.StatusCode != 408 && resp.StatusCode != 425 && resp.StatusCode != 429 && resp.StatusCode < 500 {
			return Event{}, last
		}
		if retry := resp.Header.Get("Retry-After"); retry != "" && attempt < c.retries {
			if seconds, e := strconv.Atoi(retry); e == nil && seconds > 0 && seconds <= 30 {
				timer := time.NewTimer(time.Duration(seconds) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return Event{}, ctx.Err()
				case <-timer.C:
				}
			}
		}
	}
	return Event{}, last
}

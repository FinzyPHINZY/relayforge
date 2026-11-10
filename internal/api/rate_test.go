package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type failedLimiter struct{}

func (failedLimiter) Allow(context.Context, string, string, int) (bool, int, error) {
	return false, 0, errors.New("redis unavailable")
}
func (failedLimiter) Ping(context.Context) error { return errors.New("redis unavailable") }

func TestRateLimitOutagePolicy(t *testing.T) {
	s := &Server{DistributedLimiter: failedLimiter{}, EventRateLimit: 2, ManagementRateLimit: 1}
	handler := s.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	call := func(method, path string) int {
		r := httptest.NewRequest(method, path, nil).WithContext(context.WithValue(context.Background(), principalKey, principal{KeyID: "key-1"}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if got := call(http.MethodPost, "/v1/events"); got != 503 {
		t.Fatalf("event write during outage=%d", got)
	}
	if got := call(http.MethodPatch, "/v1/endpoints/one"); got != 503 {
		t.Fatalf("management write during outage=%d", got)
	}
	for _, want := range []int{204, 429} {
		if got := call(http.MethodGet, "/v1/events"); got != want {
			t.Fatalf("read during outage=%d want=%d", got, want)
		}
	}
}

func TestRateLimitsSeparateEventsAndManagement(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	s := &Server{EventRateLimit: 2, ManagementRateLimit: 1, rateNow: func() time.Time { return now }}
	hits := 0
	handler := s.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; w.WriteHeader(204) }))
	call := func(method, path string) int {
		r := httptest.NewRequest(method, path, nil).WithContext(context.WithValue(context.Background(), principalKey, principal{KeyID: "key-1"}))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	for _, want := range []int{204, 204, 429} {
		if got := call(http.MethodPost, "/v1/events"); got != want {
			t.Fatalf("event status=%d want=%d", got, want)
		}
	}
	for _, want := range []int{204, 429} {
		if got := call(http.MethodGet, "/v1/deliveries"); got != want {
			t.Fatalf("management status=%d want=%d", got, want)
		}
	}
	if hits != 3 {
		t.Fatalf("allowed=%d", hits)
	}
	now = now.Add(time.Minute)
	if got := call(http.MethodPost, "/v1/events"); got != 204 {
		t.Fatalf("new window=%d", got)
	}
}

package monitor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNotifyOnlyOnChangeAndRetryFailedNotification(t *testing.T) {
	var reasons []string
	var messages []string
	fail := true
	m := Monitor{
		Check: func(context.Context) []string { return append([]string(nil), reasons...) },
		Notify: func(_ context.Context, message string) error {
			messages = append(messages, message)
			if fail {
				fail = false
				return errors.New("unavailable")
			}
			return nil
		},
	}
	ctx := context.Background()
	if err := m.Tick(ctx); err != nil || len(messages) != 0 {
		t.Fatalf("healthy initial tick: err=%v messages=%v", err, messages)
	}
	reasons = []string{"worker unavailable", "backlog too old"}
	if err := m.Tick(ctx); err == nil || m.Last != "" {
		t.Fatalf("failed send must not update state: err=%v last=%q", err, m.Last)
	}
	if err := m.Tick(ctx); err != nil || len(messages) != 2 || messages[0] != messages[1] {
		t.Fatalf("retry: err=%v messages=%v", err, messages)
	}
	if err := m.Tick(ctx); err != nil || len(messages) != 2 {
		t.Fatalf("duplicate notification: err=%v messages=%v", err, messages)
	}
	reasons = nil
	if err := m.Tick(ctx); err != nil || len(messages) != 3 || messages[2] != "RelayForge recovered: all checks healthy" || m.Last != "" {
		t.Fatalf("recovery: err=%v messages=%v last=%q", err, messages, m.Last)
	}
}

func TestCheckerReportsUnavailableServiceAndDatabase(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if r.URL.Path == "/failed" {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	checker := Checker{Services: map[string]string{"api": "http://api/ready", "worker": "http://worker/failed"}, HTTPClient: client}
	reasons := checker.Check(context.Background())
	if len(reasons) != 2 || reasons[0] != "worker unavailable" || reasons[1] != "database unavailable" {
		t.Fatalf("reasons=%v", reasons)
	}
}

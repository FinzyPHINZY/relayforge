package delivery

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryDelayBounds(t *testing.T) {
	if got := RetryDelay(1, 0); got != 30*time.Second {
		t.Fatalf("first delay = %v", got)
	}
	if got := RetryDelay(2, 1); got != 72*time.Second {
		t.Fatalf("second delay with jitter = %v", got)
	}
	if got := RetryDelay(8, 1); got != 72*time.Minute {
		t.Fatalf("capped delay with jitter = %v", got)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"120", 2 * time.Minute},
		{"9223372036854775807", time.Hour},
		{"-1", 0},
		{now.Add(10 * time.Minute).Format(http.TimeFormat), 10 * time.Minute},
		{"invalid", 0},
	} {
		if got := RetryAfter(tc.value, now); got != tc.want {
			t.Errorf("RetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

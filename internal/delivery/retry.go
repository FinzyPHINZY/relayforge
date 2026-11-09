package delivery

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

const maxRetryDelay = time.Hour

// RetryDelay applies exponential backoff and at most 20% positive jitter.
// jitter must be in [0,1]; callers inject a random value for each failure.
func RetryDelay(attempt int, jitter float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	if math.IsNaN(jitter) || jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	base := 30 * time.Second * time.Duration(1<<(attempt-1))
	if base > maxRetryDelay {
		base = maxRetryDelay
	}
	return base + time.Duration(float64(base)*0.2*jitter)
}

func RetryAfter(value string, now time.Time) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		if seconds >= int64(maxRetryDelay/time.Second) {
			return maxRetryDelay
		}
		d := time.Duration(seconds) * time.Second
		return d
	}
	if when, err := http.ParseTime(value); err == nil {
		d := when.Sub(now)
		if d <= 0 {
			return 0
		}
		if d > maxRetryDelay {
			return maxRetryDelay
		}
		return d
	}
	return 0
}

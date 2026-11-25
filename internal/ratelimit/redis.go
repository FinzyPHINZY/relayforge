package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis coordinates fixed one-minute windows across API processes. PostgreSQL
// remains the durable source of truth for events and monthly quotas.
type Redis struct {
	client *redis.Client
	script *redis.Script
	window *redis.Script
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client, script: redis.NewScript(`
local now = tonumber(redis.call('TIME')[1])
local window = math.floor(now / 60)
local retry_after = 60 - (now % 60)
local key = 'relayforge:rate:' .. ARGV[1] .. ':' .. ARGV[2] .. ':' .. window
local count = tonumber(redis.call('GET', key) or '0')
if count >= tonumber(ARGV[3]) then return {0, retry_after} end
count = redis.call('INCR', key)
if count == 1 then redis.call('EXPIRE', key, 120) end
return {1, retry_after}
`), window: redis.NewScript(`
local secs = tonumber(ARGV[4])
local now = tonumber(redis.call('TIME')[1])
local idx = math.floor(now / secs)
local retry_after = secs - (now % secs)
local key = 'relayforge:rate:' .. ARGV[1] .. ':' .. ARGV[2] .. ':' .. idx
local count = tonumber(redis.call('GET', key) or '0')
if count >= tonumber(ARGV[3]) then return {0, retry_after} end
count = redis.call('INCR', key)
if count == 1 then redis.call('EXPIRE', key, secs * 2) end
return {1, retry_after}
`)}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Allow(ctx context.Context, keyID, category string, limit int) (bool, int, error) {
	if limit < 1 || (category != "events" && category != "management") {
		return false, 0, fmt.Errorf("invalid rate limit policy")
	}
	// Redis time and the increment run atomically, including at minute boundaries.
	value, err := r.script.Run(ctx, r.client, nil, keyID, category, limit).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(value) != 2 {
		return false, 0, fmt.Errorf("unexpected rate limit response")
	}
	allowed, ok := value[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected rate limit decision")
	}
	retryAfter, ok := value[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected rate limit reset")
	}
	return allowed == 1, int(retryAfter), nil
}

// AllowWindow keys are relayforge:rate:<category>:<subject>:<window-index>; subject
// must not be a raw email or IP.
func (r *Redis) AllowWindow(ctx context.Context, subject, category string, limit int, window time.Duration) (bool, int, error) {
	switch category {
	case "magic_link_ip", "magic_link_email", "oauth_ip":
	default:
		return false, 0, fmt.Errorf("invalid rate limit category")
	}
	if limit < 1 || subject == "" || window < time.Minute || window > 24*time.Hour || window%time.Minute != 0 {
		return false, 0, fmt.Errorf("invalid rate limit policy")
	}
	value, err := r.window.Run(ctx, r.client, nil, category, subject, limit, int64(window/time.Second)).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(value) != 2 {
		return false, 0, fmt.Errorf("unexpected rate limit response")
	}
	allowed, ok1 := value[0].(int64)
	retryAfter, ok2 := value[1].(int64)
	if !ok1 || !ok2 {
		return false, 0, fmt.Errorf("unexpected rate limit response")
	}
	return allowed == 1, int(retryAfter), nil
}

func NewClient(rawURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	options.MaxRetries = 0
	options.DialTimeout = time.Second
	options.ReadTimeout = time.Second
	options.WriteTimeout = time.Second
	options.PoolTimeout = time.Second
	return redis.NewClient(options), nil
}

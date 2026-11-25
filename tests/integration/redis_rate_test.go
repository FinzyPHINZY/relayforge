package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/ratelimit"
)

func TestDistributedRateLimitAndRedisOutage(t *testing.T) {
	pool, _ := setup(t)
	org, key := tenantKey(t, pool, "rate-limit")
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	client, err := ratelimit.NewClient(fmt.Sprintf("redis://%s:%s/0", host, port.Port()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	limiter := ratelimit.NewRedis(client)
	if err := limiter.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	newServer := func() *httptest.Server {
		server := httptest.NewServer((&api.Server{
			DB: pool, DistributedLimiter: limiter, EncryptionKey: bytes.Repeat([]byte{1}, 32),
			EncryptionKeyID: "test", EventRateLimit: 2, ManagementRateLimit: 1,
		}).Router())
		t.Cleanup(server.Close)
		return server
	}
	first, second := newServer(), newServer()
	for i, server := range []*httptest.Server{first, second, first} {
		resp, body := request(t, http.MethodPost, server.URL+"/v1/events", key, `{"type":"order.created","data":{"id":1}}`)
		want := http.StatusAccepted
		if i == 2 {
			want = http.StatusTooManyRequests
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("429 response missing Retry-After")
			}
		}
		if resp.StatusCode != want {
			t.Fatalf("request %d: status=%d want=%d body=%s", i, resp.StatusCode, want, body)
		}
	}
	var events, outbox int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE organization_id=$1`, org).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE organization_id=$1`, org).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if events != 2 || outbox != 2 {
		t.Fatalf("events=%d outbox=%d; expected only accepted requests to commit", events, outbox)
	}
	if err := container.Stop(ctx, nil); err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, http.MethodPost, first.URL+"/v1/events", key, `{"type":"order.created","data":{"id":2}}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("write during outage: status=%d body=%s", resp.StatusCode, body)
	}
	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		resp, body := request(t, http.MethodGet, second.URL+"/v1/events", key, "")
		if resp.StatusCode != want {
			t.Fatalf("read %d during outage: status=%d want=%d body=%s", i, resp.StatusCode, want, body)
		}
	}
	ready := httptest.NewRecorder()
	health.Handler(pool, limiter.Ping).ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness during outage: status=%d", ready.Code)
	}
}

func startRedisLimiter(t testing.TB) *ratelimit.Redis {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatal(err)
	}
	client, err := ratelimit.NewClient(fmt.Sprintf("redis://%s:%s/0", host, port.Port()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	l := ratelimit.NewRedis(client)
	if err := l.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestAllowWindowHourly(t *testing.T) {
	ctx := context.Background()
	l := startRedisLimiter(t)
	for i := 0; i < 5; i++ {
		ok, _, err := l.AllowWindow(ctx, "subj", "magic_link_email", 5, time.Hour)
		if err != nil || !ok {
			t.Fatalf("call %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, retry, err := l.AllowWindow(ctx, "subj", "magic_link_email", 5, time.Hour)
	if err != nil || ok || retry <= 0 || retry > 3600 {
		t.Fatalf("6th: ok=%v retry=%d err=%v", ok, retry, err)
	}
	if ok, _, err := l.AllowWindow(ctx, "other", "magic_link_email", 5, time.Hour); err != nil || !ok {
		t.Fatalf("other subject: ok=%v err=%v", ok, err)
	}
	for _, bad := range []struct {
		cat   string
		limit int
		w     time.Duration
	}{{"nope", 5, time.Hour}, {"oauth_ip", 0, time.Hour}, {"oauth_ip", 5, 90 * time.Second}, {"oauth_ip", 5, 25 * time.Hour}, {"oauth_ip", 5, 0}} {
		if _, _, err := l.AllowWindow(ctx, "s", bad.cat, bad.limit, bad.w); err == nil {
			t.Fatalf("expected error for %+v", bad)
		}
	}
}

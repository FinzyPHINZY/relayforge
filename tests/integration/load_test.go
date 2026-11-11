package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
)

// TestSustainedDeliveryLoad is opt-in so ordinary integration tests stay fast.
// It reports observed rates and latency without imposing a hardware-specific SLO.
func TestSustainedDeliveryLoad(t *testing.T) {
	raw := os.Getenv("RELAYFORGE_LOAD_SECONDS")
	if raw == "" {
		t.Skip("set RELAYFORGE_LOAD_SECONDS (5..120) to run the sustained load scenario")
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 5 || seconds > 120 {
		t.Fatal("RELAYFORGE_LOAD_SECONDS must be between 5 and 120")
	}
	pool, _ := setup(t)
	orgA, keyA := tenantKey(t, pool, "load tenant A")
	orgB, keyB := tenantKey(t, pool, "load tenant B")
	ctx := context.Background()
	for _, org := range []string{orgA, orgB} {
		if _, err := pool.Exec(ctx, `UPDATE organizations SET monthly_event_quota=1000000 WHERE id=$1`, org); err != nil {
			t.Fatal(err)
		}
	}
	var received atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(receiver.Close)
	for _, org := range []string{orgA, orgB} {
		addDeliveryEndpoint(t, pool, org, receiver.URL)
	}
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", EventRateLimit: 100000, ManagementRateLimit: 100000}).Router())
	t.Cleanup(server.Close)
	runCtx, cancel := context.WithCancel(context.Background())
	engineDone := make(chan struct{})
	engine := delivery.Engine{DB: pool, Sender: testSender(pool), WorkerID: "load-worker", Concurrency: 4, EndpointConcurrency: 2, TenantConcurrency: 2, Lease: 45 * time.Second, DrainTimeout: 5 * time.Second}
	go func() { engine.Run(runCtx); close(engineDone) }()
	dispatchDone := make(chan struct{})
	dispatchError := make(chan error, 1)
	go func() {
		defer close(dispatchDone)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for runCtx.Err() == nil {
			if _, err := delivery.DispatchBatch(runCtx, pool, 50); err != nil && runCtx.Err() == nil {
				select {
				case dispatchError <- err:
				default:
				}
				return
			}
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-engineDone:
		case <-time.After(10 * time.Second):
			t.Error("worker did not stop")
		}
		<-dispatchDone
	})
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	latencies := make([]time.Duration, 0, seconds*20)
	acceptedByTenant := [2]int{}
	published := 0
	started := time.Now()
	deadline := started.Add(time.Duration(seconds) * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		<-ticker.C
		key := keyA
		if published%2 == 1 {
			key = keyB
		}
		requestStarted := time.Now()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/events", bytes.NewBufferString(benchmarkPayload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("publish status=%d body=%s", resp.StatusCode, body)
		}
		latencies = append(latencies, time.Since(requestStarted))
		acceptedByTenant[published%2]++
		published++
		select {
		case err := <-dispatchError:
			t.Fatalf("dispatcher: %v", err)
		default:
		}
	}
	publishElapsed := time.Since(started)
	drainDeadline := time.Now().Add(30 * time.Second)
	var delivered int
	for time.Now().Before(drainDeadline) {
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE status='succeeded' AND organization_id IN ($1,$2)`, orgA, orgB).Scan(&delivered); err != nil {
			t.Fatal(err)
		}
		if delivered == published {
			break
		}
		select {
		case err := <-dispatchError:
			t.Fatalf("dispatcher: %v", err)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if delivered != published || received.Load() != int64(published) {
		t.Fatalf("accepted=%d delivered=%d receiver_calls=%d", published, delivered, received.Load())
	}
	if acceptedByTenant[0] == 0 || acceptedByTenant[1] == 0 {
		t.Fatal("load did not exercise both tenants")
	}
	runtime.ReadMemStats(&after)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p int) time.Duration { return latencies[(len(latencies)*p+99)/100-1] }
	var oldestPending float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::double precision FROM deliveries WHERE status IN ('pending','retry_wait') AND organization_id IN ($1,$2)`, orgA, orgB).Scan(&oldestPending); err != nil {
		t.Fatal(err)
	}
	stats := pool.Stat()
	t.Log(fmt.Sprintf("LOAD_RESULT duration=%s accepted=%d delivered=%d accepted_per_sec=%.2f delivered_per_sec=%.2f p50=%s p95=%s p99=%s receiver_calls=%d oldest_pending_seconds=%.2f heap_alloc_delta_bytes=%d db_max_conns=%d db_acquired=%d go=%s cpus=%d workers=4 endpoint_cap=2 tenant_cap=2 tenants=2 payload_bytes=%d", publishElapsed.Round(time.Millisecond), published, delivered, float64(published)/publishElapsed.Seconds(), float64(delivered)/time.Since(started).Seconds(), percentile(50), percentile(95), percentile(99), received.Load(), oldestPending, int64(after.HeapAlloc)-int64(before.HeapAlloc), stats.MaxConns(), stats.AcquiredConns(), runtime.Version(), runtime.NumCPU(), len(benchmarkPayload)))
}

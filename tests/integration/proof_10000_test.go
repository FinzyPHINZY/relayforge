package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
	"webhook_delivery/internal/health"
	"webhook_delivery/internal/security"
)

// Run with RELAYFORGE_PROOF_EVENTS=10000 for the definition-of-done proof.
// Smaller counts exercise the harness without claiming the 10,000-event result.
func TestTenThousandEventReliabilityProof(t *testing.T) {
	raw := os.Getenv("RELAYFORGE_PROOF_EVENTS")
	if raw == "" {
		t.Skip("set RELAYFORGE_PROOF_EVENTS=10000 for the reliability proof")
	}
	total, err := strconv.Atoi(raw)
	if err != nil || total < 100 || total > 10000 {
		t.Fatal("RELAYFORGE_PROOF_EVENTS must be 100..10000")
	}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })
	pool, _ := setup(t)
	ctx := context.Background()
	orgs := [2]string{}
	keys := [2]string{}
	for i := range orgs {
		orgs[i], keys[i] = tenantKey(t, pool, fmt.Sprintf("proof tenant %d", i+1))
		if _, err := pool.Exec(ctx, `UPDATE organizations SET monthly_event_quota=20000 WHERE id=$1`, orgs[i]); err != nil {
			t.Fatal(err)
		}
	}
	type receipt struct {
		id                     string
		index, attempt, status int
		valid                  bool
	}
	var mu sync.Mutex
	receipts := make([]receipt, 0, total+50)
	secret := "whsec_test_delivery_secret"
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
		var event struct {
			Data struct {
				Index int `json:"index"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &event)
		attempt, _ := strconv.Atoi(r.Header.Get("webhook-attempt"))
		ts, tsErr := strconv.ParseInt(r.Header.Get("webhook-timestamp"), 10, 64)
		valid := tsErr == nil && security.WebhookSignature(secret, ts, body) == r.Header.Get("webhook-signature")
		status := http.StatusNoContent
		if attempt == 1 && event.Data.Index > 0 {
			switch event.Data.Index % 2000 {
			case 2:
				status = http.StatusBadRequest
			case 3:
				status = http.StatusServiceUnavailable
			case 4:
				time.Sleep(1200 * time.Millisecond)
			}
		}
		mu.Lock()
		receipts = append(receipts, receipt{id: r.Header.Get("webhook-id"), index: event.Data.Index, attempt: attempt, status: status, valid: valid})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	defer receiver.Close()
	for _, org := range orgs {
		endpoint, _ := addDeliveryEndpoint(t, pool, org, receiver.URL)
		if _, err := pool.Exec(ctx, `UPDATE endpoints SET timeout_ms=1000,concurrency_limit=20 WHERE id=$1`, endpoint); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE subscriptions SET event_type='proof.event' WHERE endpoint_id=$1`, endpoint); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", EventRateLimit: 100000, ManagementRateLimit: 100000}).Router())
	defer server.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	accept := func(index int) (string, time.Duration, error) {
		body := fmt.Sprintf(`{"type":"proof.event","data":{"index":%d}}`, index)
		started := time.Now()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/events", strings.NewReader(body))
		if err != nil {
			return "", 0, err
		}
		req.Header.Set("Authorization", "Bearer "+keys[index%2])
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("proof-%d", index))
		resp, err := client.Do(req)
		if err != nil {
			return "", 0, err
		}
		defer resp.Body.Close()
		var result struct {
			ID string `json:"id"`
		}
		if resp.StatusCode != http.StatusAccepted {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			return "", 0, fmt.Errorf("index %d: HTTP %d: %s", index, resp.StatusCode, data)
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", 0, err
		}
		return result.ID, time.Since(started), nil
	}
	seedID, seedLatency, err := accept(0)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := delivery.DispatchBatch(ctx, pool, 1); err != nil || n != 1 {
		t.Fatalf("seed dispatch=%d err=%v", n, err)
	}
	type accepted struct {
		id      string
		latency time.Duration
		err     error
	}
	jobs := make(chan int, 64)
	results := make(chan accepted, total)
	var publishers sync.WaitGroup
	for range 16 {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for index := range jobs {
				id, latency, err := accept(index)
				results <- accepted{id, latency, err}
			}
		}()
	}
	publishStart := time.Now()
	go func() {
		for index := 1; index < total; index++ {
			jobs <- index
		}
		close(jobs)
		publishers.Wait()
		close(results)
	}()
	first := startV01Process(t, pool, "crash")
	line := waitV01Line(t, first.lines, 15*time.Second)
	if !strings.HasPrefix(line, "SENT ") {
		t.Fatalf("crash milestone=%q stderr=%s", line, first.stderr.String())
	}
	crashedDelivery := strings.TrimPrefix(line, "SENT ")
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := first.cmd.Wait(); err == nil {
		t.Fatal("worker did not die from SIGKILL")
	}
	var crashedEvent string
	if err := pool.QueryRow(ctx, `SELECT event_id FROM deliveries WHERE id=$1 AND status='processing'`, crashedDelivery).Scan(&crashedEvent); err != nil || crashedEvent != seedID {
		t.Fatalf("crashed event=%s err=%v", crashedEvent, err)
	}
	runCtx, stop := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	engine := delivery.Engine{DB: pool, Sender: testSender(pool), WorkerID: "proof-worker", Concurrency: 32, EndpointConcurrency: 20, TenantConcurrency: 20, Lease: 45 * time.Second, DrainTimeout: 10 * time.Second}
	go func() { engine.Run(runCtx); close(workerDone) }()
	dispatchDone := make(chan struct{})
	dispatchErrors := make(chan error, 1)
	go func() {
		defer close(dispatchDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for runCtx.Err() == nil {
			if _, err := delivery.DispatchBatch(runCtx, pool, 100); err != nil && runCtx.Err() == nil {
				dispatchErrors <- err
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
		stop()
		select {
		case <-workerDone:
		case <-time.After(15 * time.Second):
			t.Error("worker shutdown timed out")
		}
		<-dispatchDone
	})
	acceptedIDs := map[string]bool{seedID: true}
	latencies := []time.Duration{seedLatency}
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.id == "" || acceptedIDs[result.id] {
			t.Fatalf("missing or duplicate accepted ID %q", result.id)
		}
		acceptedIDs[result.id] = true
		latencies = append(latencies, result.latency)
	}
	publishElapsed := time.Since(publishStart)
	if len(acceptedIDs) != total {
		t.Fatalf("accepted=%d want=%d", len(acceptedIDs), total)
	}
	deadline := time.Now().Add(10 * time.Minute)
	var pending int
	for time.Now().Before(deadline) {
		select {
		case err := <-dispatchErrors:
			t.Fatal(err)
		default:
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM events e LEFT JOIN deliveries d ON d.event_id=e.id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND (d.id IS NULL OR d.status IN ('pending','processing','retry_wait'))`, orgs[0], orgs[1]).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending == 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if pending != 0 {
		t.Fatalf("drain deadline: %d events lack terminal deliveries", pending)
	}
	var eventCount, outboxCount, deliveryCount, succeeded, exhausted, attemptCount, retryCount, reclaimCount int
	err = pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM events WHERE event_type='proof.event' AND organization_id IN ($1,$2)),
 (SELECT count(*) FROM outbox o JOIN events e ON e.id=o.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND o.status='dispatched'),
 (SELECT count(*) FROM deliveries d JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2)),
 (SELECT count(*) FROM deliveries d JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND d.status='succeeded'),
 (SELECT count(*) FROM deliveries d JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND d.status='exhausted'),
 (SELECT count(*) FROM delivery_attempts a JOIN deliveries d ON d.id=a.delivery_id JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2)),
 (SELECT count(*) FROM delivery_attempts a JOIN deliveries d ON d.id=a.delivery_id JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND a.attempt_number>1),
 (SELECT count(*) FROM delivery_attempts a JOIN deliveries d ON d.id=a.delivery_id JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2) AND a.error_code='lease_expired')`, orgs[0], orgs[1]).Scan(&eventCount, &outboxCount, &deliveryCount, &succeeded, &exhausted, &attemptCount, &retryCount, &reclaimCount)
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != total || outboxCount != total || deliveryCount != total || succeeded+exhausted != total || exhausted == 0 || retryCount == 0 || reclaimCount != 1 {
		t.Fatalf("reconciliation events=%d outbox=%d deliveries=%d succeeded=%d exhausted=%d attempts=%d retries=%d reclaims=%d", eventCount, outboxCount, deliveryCount, succeeded, exhausted, attemptCount, retryCount, reclaimCount)
	}
	var timeouts, unavailable, badRequests int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE a.outcome='timeout'),count(*) FILTER (WHERE a.http_status=503),count(*) FILTER (WHERE a.http_status=400) FROM delivery_attempts a JOIN deliveries d ON d.id=a.delivery_id JOIN events e ON e.id=d.event_id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2)`, orgs[0], orgs[1]).Scan(&timeouts, &unavailable, &badRequests); err != nil {
		t.Fatal(err)
	}
	if timeouts == 0 || unavailable == 0 || badRequests == 0 {
		t.Fatalf("injected outcomes timeout=%d http_503=%d http_400=%d", timeouts, unavailable, badRequests)
	}
	for id := range acceptedIDs {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM deliveries d JOIN outbox o ON o.event_id=d.event_id WHERE d.event_id=$1 AND o.status='dispatched' AND d.status IN ('succeeded','exhausted')`, id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("unaccounted event=%s deliveries=%d err=%v", id, n, err)
		}
	}
	mu.Lock()
	got := append([]receipt(nil), receipts...)
	mu.Unlock()
	duplicates := 0
	seen := make(map[string]bool, total)
	for _, r := range got {
		if !r.valid || r.id == "" {
			t.Fatalf("invalid signed receipt=%+v", r)
		}
		if seen[r.id] {
			duplicates++
		}
		seen[r.id] = true
	}
	if duplicates == 0 || len(got) < attemptCount {
		t.Fatalf("receipts=%d attempts=%d duplicate_receipts=%d", len(got), attemptCount, duplicates)
	}
	var crashReceipts int
	for _, r := range got {
		if r.id == crashedDelivery {
			crashReceipts++
		}
	}
	if crashReceipts != 2 {
		t.Fatalf("crashed delivery receipts=%d", crashReceipts)
	}
	metricResponse := httptest.NewRecorder()
	health.Handler(pool).ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricResponse.Code != http.StatusOK || !strings.Contains(metricResponse.Body.String(), "webhook_lease_reclaims_recorded") {
		t.Fatalf("metrics status=%d body=%s", metricResponse.Code, metricResponse.Body.String())
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p int) time.Duration { return latencies[(len(latencies)*p+99)/100-1] }
	deliveryRows, err := pool.Query(ctx, `SELECT e.created_at,d.completed_at FROM events e JOIN deliveries d ON d.event_id=e.id WHERE e.event_type='proof.event' AND e.organization_id IN ($1,$2)`, orgs[0], orgs[1])
	if err != nil {
		t.Fatal(err)
	}
	deliveryLatencies := make([]time.Duration, 0, total)
	for deliveryRows.Next() {
		var acceptedAt, completedAt time.Time
		if err := deliveryRows.Scan(&acceptedAt, &completedAt); err != nil {
			deliveryRows.Close()
			t.Fatal(err)
		}
		deliveryLatencies = append(deliveryLatencies, completedAt.Sub(acceptedAt))
	}
	if err := deliveryRows.Err(); err != nil {
		deliveryRows.Close()
		t.Fatal(err)
	}
	deliveryRows.Close()
	if len(deliveryLatencies) != total {
		t.Fatalf("delivery latency samples=%d want=%d", len(deliveryLatencies), total)
	}
	sort.Slice(deliveryLatencies, func(i, j int) bool { return deliveryLatencies[i] < deliveryLatencies[j] })
	deliveryPct := func(p int) time.Duration { return deliveryLatencies[(len(deliveryLatencies)*p+99)/100-1] }
	var oldest float64
	_ = pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::double precision FROM deliveries WHERE status IN ('pending','retry_wait') AND organization_id IN ($1,$2)`, orgs[0], orgs[1]).Scan(&oldest)
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	t.Logf("PROOF_RESULT accepted=%d outbox=%d deliveries=%d succeeded=%d exhausted=%d attempts=%d retries=%d lease_reclaims=%d timeouts=%d http_503=%d http_400=%d receiver_calls=%d duplicates=%d publish_duration=%s total_duration=%s accepted_per_sec=%.2f p50_accept=%s p95_accept=%s p99_accept=%s p50_delivery=%s p95_delivery=%s p99_delivery=%s oldest_pending_seconds=%.2f heap_alloc_bytes=%d db_max_conns=%d go=%s cpus=%d workers=32 tenant_cap=20 endpoint_cap=20 tenants=2 payload=proof.event/index", total, outboxCount, deliveryCount, succeeded, exhausted, attemptCount, retryCount, reclaimCount, timeouts, unavailable, badRequests, len(got), duplicates, publishElapsed, time.Since(publishStart), float64(total)/publishElapsed.Seconds(), pct(50), pct(95), pct(99), deliveryPct(50), deliveryPct(95), deliveryPct(99), oldest, memory.HeapAlloc, pool.Stat().MaxConns(), runtime.Version(), runtime.NumCPU())
}

package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"webhook_delivery/internal/api"
	"webhook_delivery/internal/delivery"
)

const benchmarkPayload = `{"type":"order.created","data":{"order_id":"benchmark","amount_minor":4999}}`

func reportLatencies(b *testing.B, samples []int64) {
	if len(samples) == 0 {
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	for _, p := range []int{50, 95, 99} {
		index := (len(samples)*p+99)/100 - 1
		b.ReportMetric(float64(samples[index])/1e6, "p"+strconv.Itoa(p)+"_ms")
	}
}
func benchmarkServer(b *testing.B) (string, string, func()) {
	pool, _ := setup(b)
	org, key := tenantKey(b, pool, "benchmark")
	if _, err := pool.Exec(context.Background(), `UPDATE organizations SET monthly_event_quota=1000000000 WHERE id=$1`, org); err != nil {
		b.Fatal(err)
	}
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", EventRateLimit: 1000000000, ManagementRateLimit: 1000000000}).Router())
	b.Logf("Go=%s CPUs=%d GOMAXPROCS=%d DB max connections=%d payload bytes=%d", runtime.Version(), runtime.NumCPU(), runtime.GOMAXPROCS(0), pool.Config().MaxConns, len(benchmarkPayload))
	return server.URL, key, server.Close
}
func BenchmarkEventIngestion(b *testing.B) {
	serverURL, key, closeServer := benchmarkServer(b)
	defer closeServer()
	samples := make([]int64, b.N)
	var accepted atomic.Int64
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))
	b.ResetTimer()
	started := time.Now()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			requestStarted := time.Now()
			req, err := http.NewRequest(http.MethodPost, serverURL+"/v1/events", bytes.NewBufferString(benchmarkPayload))
			if err != nil {
				b.Error(err)
				return
			}
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				b.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 202 {
				b.Errorf("ingestion status=%d", resp.StatusCode)
				return
			}
			samples[accepted.Add(1)-1] = time.Since(requestStarted).Nanoseconds()
		}
	})
	elapsed := time.Since(started)
	b.StopTimer()
	if elapsed > 0 {
		b.ReportMetric(float64(accepted.Load())/elapsed.Seconds(), "accepted/s")
	}
	reportLatencies(b, samples[:accepted.Load()])
}

func BenchmarkPublishToAcknowledgedDelivery(b *testing.B) {
	pool, _ := setup(b)
	org, key := tenantKey(b, pool, "delivery benchmark")
	if _, err := pool.Exec(context.Background(), `UPDATE organizations SET monthly_event_quota=1000000000 WHERE id=$1`, org); err != nil {
		b.Fatal(err)
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(b, pool, org, receiver.URL)
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: testEncryptionKey, EncryptionKeyID: "test", EventRateLimit: 1000000000}).Router())
	defer server.Close()
	b.Logf("Go=%s CPUs=%d GOMAXPROCS=%d DB max connections=%d payload bytes=%d", runtime.Version(), runtime.NumCPU(), runtime.GOMAXPROCS(0), pool.Config().MaxConns, len(benchmarkPayload))
	samples := make([]int64, b.N)
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkPayload)))
	b.ResetTimer()
	started := time.Now()
	for i := 0; i < b.N; i++ {
		iteration := time.Now()
		resp, body := request(b, http.MethodPost, server.URL+"/v1/events", key, benchmarkPayload)
		if resp.StatusCode != 202 {
			b.Fatalf("ingestion=%d body=%s", resp.StatusCode, body)
		}
		if n, err := delivery.DispatchBatch(context.Background(), pool, 1); err != nil || n != 1 {
			b.Fatalf("dispatch=%d err=%v", n, err)
		}
		claim, err := delivery.ClaimOne(context.Background(), pool, "benchmark-worker", 45*time.Second)
		if err != nil || claim == nil {
			b.Fatalf("claim=%+v err=%v", claim, err)
		}
		result := testSender(pool).Send(context.Background(), *claim)
		if applied, err := delivery.Complete(context.Background(), pool, *claim, result); err != nil || !applied || result.Outcome != "success" {
			b.Fatalf("complete=%v result=%+v err=%v", applied, result, err)
		}
		samples[i] = time.Since(iteration).Nanoseconds()
	}
	elapsed := time.Since(started)
	b.StopTimer()
	if elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "deliveries/s")
	}
	reportLatencies(b, samples)
}

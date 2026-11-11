package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"webhook_delivery/internal/delivery"
)

func TestTraceContextSurvivesAsyncDeliveryHandoffs(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	pool, apiServer := setup(t)
	org, key := tenantKey(t, pool, "trace handoff")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer receiver.Close()
	addDeliveryEndpoint(t, pool, org, receiver.URL)
	const traceID = "0123456789abcdef0123456789abcdef"
	request, err := http.NewRequest(http.MethodPost, apiServer.URL+"/v1/events", bytes.NewBufferString(`{"type":"order.created","data":{"id":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("traceparent", "00-"+traceID+"-0123456789abcdef-01")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 202 {
		t.Fatalf("publish status=%d body=%s", response.StatusCode, body)
	}
	var accepted struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &accepted); err != nil {
		t.Fatal(err)
	}
	var eventParent string
	if err := pool.QueryRow(context.Background(), `SELECT traceparent FROM events WHERE id=$1`, accepted.ID).Scan(&eventParent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(eventParent, traceID) {
		t.Fatalf("event traceparent=%q", eventParent)
	}
	if n, err := delivery.DispatchBatch(context.Background(), pool, 10); err != nil || n != 1 {
		t.Fatalf("dispatch=%d err=%v", n, err)
	}
	var deliveryParent string
	if err := pool.QueryRow(context.Background(), `SELECT traceparent FROM deliveries WHERE event_id=$1`, accepted.ID).Scan(&deliveryParent); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deliveryParent, traceID) || deliveryParent == eventParent {
		t.Fatalf("delivery traceparent=%q event=%q", deliveryParent, eventParent)
	}
	attempts := make(chan string, 1)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	engine := delivery.Engine{DB: pool, WorkerID: "trace-worker", Concurrency: 1, EndpointConcurrency: 2, TenantConcurrency: 2,
		SendFunc: func(ctx context.Context, claim delivery.Claimed) delivery.Result {
			attempts <- trace.SpanContextFromContext(ctx).TraceID().String()
			return delivery.Result{Outcome: "success", HTTPStatus: 204}
		}}
	go func() { engine.Run(runCtx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.Now().Add(8 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(context.Background(), `SELECT status FROM deliveries WHERE event_id=$1`, accepted.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "succeeded" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "succeeded" {
		t.Fatalf("delivery status=%s", status)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop")
	}
	select {
	case attemptTrace := <-attempts:
		if attemptTrace != traceID {
			t.Fatalf("attempt trace=%s", attemptTrace)
		}
	default:
		t.Fatal("attempt was not recorded")
	}
	seen := map[string]bool{}
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() != traceID {
			continue
		}
		seen[span.Name()] = true
	}
	for _, name := range []string{"event.accept", "outbox.dispatch", "delivery.claim", "webhook.attempt", "delivery.complete"} {
		if !seen[name] {
			t.Fatalf("missing correlated span %q: seen=%v", name, seen)
		}
	}
}

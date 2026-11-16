package observability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestOTLPTraceExportUsesProtocolAndExcludesSensitiveAttributes(t *testing.T) {
	var path, contentType string
	var body map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path, contentType = r.URL.Path, r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	exporter, err := newOTLPTraceExporter("http://collector:4318", "relayforge-test")
	if err != nil {
		t.Fatal(err)
	}
	exporter.client = &http.Client{Transport: transport}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	ctx, span := provider.Tracer("test").Start(context.Background(), "event.accept")
	span.SetAttributes(attribute.String("url.full", "https://example.test/private?token=secret"))
	_, child := provider.Tracer("test").Start(ctx, "outbox.dispatch")
	child.End()
	span.End()
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/v1/traces" || contentType != "application/json" {
		t.Fatalf("path=%s content-type=%s", path, contentType)
	}
	data, _ := json.Marshal(body)
	if strings.Contains(string(data), "token=secret") || strings.Contains(string(data), "url.full") {
		t.Fatalf("sensitive attribute exported: %s", data)
	}
	resourceSpans := body["resourceSpans"].([]any)
	spans := resourceSpans[0].(map[string]any)["scopeSpans"].([]any)[0].(map[string]any)["spans"].([]any)
	if len(spans) != 1 {
		t.Fatalf("spans=%d, want last synchronous export", len(spans))
	}
	last := spans[0].(map[string]any)
	if len(last["traceId"].(string)) != 32 || len(last["spanId"].(string)) != 16 {
		t.Fatalf("invalid IDs: %v", last)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOTLPTraceExporterRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://collector:4318", "http://user:pass@collector:4318", "http://collector:4318?key=secret"} {
		if _, err := newOTLPTraceExporter(endpoint, "test"); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
}

func TestOTLPTraceExporterRetriesTransientFailure(t *testing.T) {
	exporter, err := newOTLPTraceExporter("http://collector:4318", "relayforge-test")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	exporter.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		status := http.StatusServiceUnavailable
		if requests == 2 {
			status = http.StatusOK
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	_, span := provider.Tracer("test").Start(context.Background(), "webhook.attempt")
	span.End()
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

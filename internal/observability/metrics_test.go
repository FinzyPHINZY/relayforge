package observability

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDeliveryMetricsReachOpenTelemetryReader(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(noop.NewMeterProvider())
		_ = provider.Shutdown(context.Background())
	})
	ctx := context.Background()
	RecordAccepted(ctx)
	RecordDispatch(ctx, 240*time.Millisecond)
	RecordAttempt(ctx, "http_error", 125*time.Millisecond, true)
	RecordReclaims(ctx, 2)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &collected); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"relayforge.events.accepted":            false,
		"relayforge.outbox.dispatched":          false,
		"relayforge.outbox.dispatch_lag":        false,
		"relayforge.delivery.attempts":          false,
		"relayforge.delivery.attempt_duration":  false,
		"relayforge.delivery.retries_scheduled": false,
		"relayforge.delivery.leases_reclaimed":  false,
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if _, ok := want[m.Name]; ok {
				want[m.Name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing OpenTelemetry metric %s", name)
		}
	}
}

func TestDeliveryMetricsExportOverOTLPHTTP(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Path != "/v1/metrics" || r.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Errorf("request path=%s content-type=%s", r.URL.Path, r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			t.Errorf("empty OTLP metric body: %v", err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	exporter, err := otlpmetrichttp.New(context.Background(), otlpmetrichttp.WithEndpointURL("http://collector.invalid:4318/v1/metrics"), otlpmetrichttp.WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Hour))))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(noop.NewMeterProvider())
		_ = provider.Shutdown(context.Background())
	})
	RecordAccepted(context.Background())
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("OTLP requests=%d", requests)
	}
}

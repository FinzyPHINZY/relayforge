package observability

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Start installs a bounded OpenTelemetry trace pipeline. The local exporter
// writes structured span summaries without tenant IDs, URLs, or payloads.
func Start(service string) func(context.Context) error {
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(logExporter{}, sdktrace.WithBatchTimeout(time.Second), sdktrace.WithMaxExportBatchSize(256)),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", service))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown
}

// TraceParent serializes only the W3C parent identifier for durable handoffs.
func TraceParent(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

func ContextWithTraceParent(ctx context.Context, parent string) context.Context {
	if parent == "" {
		return ctx
	}
	return propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": parent})
}

type logExporter struct{}

func (logExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		slog.Info("trace span", "operation", span.Name(), "trace_id", span.SpanContext().TraceID().String(), "span_id", span.SpanContext().SpanID().String(), "duration_ms", span.EndTime().Sub(span.StartTime()).Milliseconds(), "status", span.Status().Code.String())
	}
	return nil
}
func (logExporter) Shutdown(context.Context) error { return nil }

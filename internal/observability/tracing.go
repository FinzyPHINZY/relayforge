package observability

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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
	return provider.Shutdown
}

type logExporter struct{}

func (logExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		slog.Info("trace span", "operation", span.Name(), "trace_id", span.SpanContext().TraceID().String(), "span_id", span.SpanContext().SpanID().String(), "duration_ms", span.EndTime().Sub(span.StartTime()).Milliseconds(), "status", span.Status().Code.String())
	}
	return nil
}
func (logExporter) Shutdown(context.Context) error { return nil }

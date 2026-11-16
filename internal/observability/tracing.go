package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Start installs bounded trace and metric pipelines. Without an OTLP endpoint,
// trace summaries go to structured logs and metrics stay disabled.
func Start(service string) func(context.Context) error {
	slog.SetDefault(slog.New(&traceLogHandler{Handler: slog.NewJSONHandler(os.Stdout, nil)}).With("service", service))
	exporter := sdktrace.SpanExporter(logExporter{})
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		remote, err := newOTLPTraceExporter(endpoint, service)
		if err != nil {
			slog.Error("trace exporter configuration rejected", "error", err)
		} else {
			exporter = remote
		}
	}
	res := resource.NewWithAttributes("", attribute.String("service.name", service))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second), sdktrace.WithMaxExportBatchSize(256)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	var metricProvider *sdkmetric.MeterProvider
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		metricExporter, err := otlpmetrichttp.New(context.Background())
		if err != nil {
			slog.Error("metric exporter configuration rejected", "error", err)
		} else {
			metricProvider = sdkmetric.NewMeterProvider(
				sdkmetric.WithResource(res),
				sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(30*time.Second), sdkmetric.WithTimeout(5*time.Second))),
			)
			otel.SetMeterProvider(metricProvider)
		}
	}
	return func(ctx context.Context) error {
		if metricProvider != nil {
			if err := metricProvider.Shutdown(ctx); err != nil {
				slog.Error("metric export shutdown failed", "error", err)
			}
		}
		return provider.Shutdown(ctx)
	}
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

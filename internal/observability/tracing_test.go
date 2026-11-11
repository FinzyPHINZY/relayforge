package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceParentRoundTrip(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	parent := TraceParent(ctx)
	if parent != "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" {
		t.Fatalf("traceparent=%q", parent)
	}
	got := trace.SpanContextFromContext(ContextWithTraceParent(context.Background(), parent))
	if got.TraceID() != traceID || got.SpanID() != spanID {
		t.Fatalf("restored context=%s/%s", got.TraceID(), got.SpanID())
	}
}

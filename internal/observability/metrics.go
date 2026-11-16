package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "relayforge/delivery"

func RecordAccepted(ctx context.Context) {
	c, err := otel.Meter(meterName).Int64Counter("relayforge.events.accepted")
	if err == nil {
		c.Add(ctx, 1)
	}
}

func RecordDispatch(ctx context.Context, lag time.Duration) {
	m := otel.Meter(meterName)
	if c, err := m.Int64Counter("relayforge.outbox.dispatched"); err == nil {
		c.Add(ctx, 1)
	}
	if h, err := m.Float64Histogram("relayforge.outbox.dispatch_lag", metric.WithUnit("s")); err == nil {
		h.Record(ctx, max(0, lag.Seconds()))
	}
}

func RecordAttempt(ctx context.Context, outcome string, duration time.Duration, retry bool) {
	switch outcome {
	case "success", "http_error", "network_error", "timeout", "security_blocked", "unknown":
	default:
		outcome = "other"
	}
	m := otel.Meter(meterName)
	label := metric.WithAttributes(attribute.String("outcome", outcome))
	if c, err := m.Int64Counter("relayforge.delivery.attempts"); err == nil {
		c.Add(ctx, 1, label)
	}
	if h, err := m.Float64Histogram("relayforge.delivery.attempt_duration", metric.WithUnit("s")); err == nil {
		h.Record(ctx, max(0, duration.Seconds()), label)
	}
	if retry {
		if c, err := m.Int64Counter("relayforge.delivery.retries_scheduled"); err == nil {
			c.Add(ctx, 1)
		}
	}
}

func RecordReclaims(ctx context.Context, count int) {
	if count < 1 {
		return
	}
	if c, err := otel.Meter(meterName).Int64Counter("relayforge.delivery.leases_reclaimed"); err == nil {
		c.Add(ctx, int64(count))
	}
}

package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// otlpTraceExporter sends the small, allowlisted subset of span data needed
// to correlate durable handoffs. OTLP/HTTP JSON is part of the OTLP protocol.
// In particular, neither HTTP attributes nor endpoint URLs are exported.
type otlpTraceExporter struct {
	url, service string
	client       *http.Client
}

func newOTLPTraceExporter(endpoint, service string) (*otlpTraceExporter, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT must be an HTTP(S) origin or base path")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/traces"
	return &otlpTraceExporter{url: u.String(), service: service, client: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (e *otlpTraceExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}
	type value struct {
		StringValue string `json:"stringValue"`
	}
	type attr struct {
		Key   string `json:"key"`
		Value value  `json:"value"`
	}
	type status struct {
		Code int `json:"code"`
	}
	type spanJSON struct {
		TraceID      string `json:"traceId"`
		SpanID       string `json:"spanId"`
		ParentSpanID string `json:"parentSpanId,omitempty"`
		Name         string `json:"name"`
		Kind         int    `json:"kind"`
		Start        string `json:"startTimeUnixNano"`
		End          string `json:"endTimeUnixNano"`
		Status       status `json:"status"`
	}
	items := make([]spanJSON, 0, len(spans))
	for _, s := range spans {
		if !s.SpanContext().IsValid() {
			continue
		}
		code := 0
		if s.Status().Code.String() == "Error" {
			code = 2
		}
		if s.Status().Code.String() == "Ok" {
			code = 1
		}
		parentID := ""
		if s.Parent().IsValid() {
			parentID = s.Parent().SpanID().String()
		}
		items = append(items, spanJSON{TraceID: s.SpanContext().TraceID().String(), SpanID: s.SpanContext().SpanID().String(), ParentSpanID: parentID, Name: s.Name(), Kind: int(s.SpanKind()), Start: fmt.Sprint(s.StartTime().UnixNano()), End: fmt.Sprint(s.EndTime().UnixNano()), Status: status{Code: code}})
	}
	if len(items) == 0 {
		return nil
	}
	requestBody := map[string]any{"resourceSpans": []any{map[string]any{"resource": map[string]any{"attributes": []attr{{Key: "service.name", Value: value{StringValue: e.service}}}}, "scopeSpans": []any{map[string]any{"scope": map[string]string{"name": "relayforge"}, "spans": items}}}}}
	data, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(1<<uint(attempt-1)) * 200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := e.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		response.Body.Close()
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil
		}
		last = fmt.Errorf("OTLP trace export HTTP %d", response.StatusCode)
		if response.StatusCode != 429 && response.StatusCode < 500 {
			return last
		}
	}
	return last
}

func (e *otlpTraceExporter) Shutdown(context.Context) error {
	e.client.CloseIdleConnections()
	return nil
}

type traceLogHandler struct{ slog.Handler }

func (h *traceLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := oteltrace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, record)
}
func (h *traceLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}
func (h *traceLogHandler) WithGroup(name string) slog.Handler {
	return &traceLogHandler{Handler: h.Handler.WithGroup(name)}
}

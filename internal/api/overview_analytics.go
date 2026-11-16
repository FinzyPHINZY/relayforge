package api

import (
	"context"
	"net/http"
	"time"
)

type overviewBucket struct {
	Start          time.Time `json:"start"`
	Events         int64     `json:"events"`
	Succeeded      int64     `json:"succeeded"`
	Exhausted      int64     `json:"exhausted"`
	Cancelled      int64     `json:"cancelled"`
	Attempts       int64     `json:"attempts"`
	FailedAttempts int64     `json:"failed_attempts"`
	Retries        int64     `json:"retries"`
	P50LatencyMS   *float64  `json:"p50_latency_ms"`
	P95LatencyMS   *float64  `json:"p95_latency_ms"`
}

type overviewTotals struct {
	Events         int64 `json:"events"`
	Succeeded      int64 `json:"succeeded"`
	Exhausted      int64 `json:"exhausted"`
	Cancelled      int64 `json:"cancelled"`
	Attempts       int64 `json:"attempts"`
	FailedAttempts int64 `json:"failed_attempts"`
	Retries        int64 `json:"retries"`
}

// overviewAnalytics uses one UTC bucket grid and one fixed upper bound for all
// series. Delivery outcomes are counted when terminal; acceptance and attempts
// are counted when they occur. A retry is an attempt numbered greater than one.
func (s *Server) overviewAnalytics(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	window := r.URL.Query().Get("window")
	var start time.Time
	var step string
	switch window {
	case "", "24h":
		window, step = "24h", "1 hour"
		start = now.Truncate(time.Hour).Add(-23 * time.Hour)
	case "7d":
		step = "1 day"
		start = now.Truncate(24 * time.Hour).Add(-6 * 24 * time.Hour)
	case "30d":
		step = "1 day"
		start = now.Truncate(24 * time.Hour).Add(-29 * 24 * time.Hour)
	default:
		problem(w, 400, "invalid_window", "Choose 24h, 7d, or 30d")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := s.DB.Query(ctx, `WITH buckets AS (
  SELECT generate_series($2::timestamptz, date_bin($4::interval,$3::timestamptz,'1970-01-01 UTC'::timestamptz), $4::interval) AS start
), event_counts AS (
  SELECT date_bin($4::interval,created_at,'1970-01-01 UTC'::timestamptz) AS start,count(*)::bigint AS events
  FROM events WHERE organization_id=$1 AND created_at >= $2 AND created_at <= $3 GROUP BY 1
), delivery_counts AS (
  SELECT date_bin($4::interval,d.completed_at,'1970-01-01 UTC'::timestamptz) AS start,
    count(*) FILTER (WHERE d.status='succeeded')::bigint AS succeeded,
    count(*) FILTER (WHERE d.status='exhausted')::bigint AS exhausted,
    count(*) FILTER (WHERE d.status='cancelled')::bigint AS cancelled,
    percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (d.completed_at-e.created_at))*1000) FILTER (WHERE d.status IN ('succeeded','exhausted')) AS p50_latency_ms,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (d.completed_at-e.created_at))*1000) FILTER (WHERE d.status IN ('succeeded','exhausted')) AS p95_latency_ms
  FROM deliveries d JOIN events e ON e.id=d.event_id AND e.organization_id=d.organization_id
  WHERE d.organization_id=$1 AND d.completed_at >= $2 AND d.completed_at <= $3
    AND d.status IN ('succeeded','exhausted','cancelled') GROUP BY 1
), attempt_counts AS (
  SELECT date_bin($4::interval,started_at,'1970-01-01 UTC'::timestamptz) AS start,
    count(*)::bigint AS attempts,
    count(*) FILTER (WHERE finished_at IS NOT NULL AND outcome<>'success')::bigint AS failed_attempts,
    count(*) FILTER (WHERE attempt_number>1)::bigint AS retries
  FROM delivery_attempts WHERE organization_id=$1 AND started_at >= $2 AND started_at <= $3 GROUP BY 1
)
SELECT b.start,coalesce(ec.events,0),coalesce(dc.succeeded,0),coalesce(dc.exhausted,0),coalesce(dc.cancelled,0),
  coalesce(ac.attempts,0),coalesce(ac.failed_attempts,0),coalesce(ac.retries,0),dc.p50_latency_ms,dc.p95_latency_ms
FROM buckets b LEFT JOIN event_counts ec ON ec.start=b.start
LEFT JOIN delivery_counts dc ON dc.start=b.start
LEFT JOIN attempt_counts ac ON ac.start=b.start ORDER BY b.start`, tenant(r), start, now, step)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	buckets := make([]overviewBucket, 0, 30)
	totals := overviewTotals{}
	for rows.Next() {
		var b overviewBucket
		if err := rows.Scan(&b.Start, &b.Events, &b.Succeeded, &b.Exhausted, &b.Cancelled, &b.Attempts, &b.FailedAttempts, &b.Retries, &b.P50LatencyMS, &b.P95LatencyMS); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		buckets = append(buckets, b)
		totals.Events += b.Events
		totals.Succeeded += b.Succeeded
		totals.Exhausted += b.Exhausted
		totals.Cancelled += b.Cancelled
		totals.Attempts += b.Attempts
		totals.FailedAttempts += b.FailedAttempts
		totals.Retries += b.Retries
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"window": window, "from": start, "to": now, "bucket_unit": step, "totals": totals, "buckets": buckets})
}

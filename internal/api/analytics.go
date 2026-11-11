package api

import (
	"net/http"
	"time"
)

type analyticsBucket struct {
	Start         time.Time `json:"start"`
	Attempts      int64     `json:"attempts"`
	Succeeded     int64     `json:"succeeded"`
	Failed        int64     `json:"failed"`
	P50DurationMS *float64  `json:"p50_duration_ms"`
	P95DurationMS *float64  `json:"p95_duration_ms"`
}

func (s *Server) deliveryAnalytics(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	var hours int
	switch window {
	case "", "24h":
		window, hours = "24h", 24
	case "7d":
		hours = 168
	case "30d":
		hours = 720
	default:
		problem(w, 400, "invalid_window", "Choose 24h, 7d, or 30d")
		return
	}
	// The tenant ID is always part of the indexable predicate. Empty buckets
	// are omitted; clients can fill them with zeroes for charts.
	rows, err := s.DB.Query(r.Context(), `SELECT date_trunc('hour',started_at) AS bucket,count(*)::bigint,count(*) FILTER (WHERE outcome='success')::bigint,count(*) FILTER (WHERE outcome<>'success')::bigint,percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL),percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL) FROM delivery_attempts WHERE organization_id=$1 AND started_at>=now()-($2::integer*interval '1 hour') GROUP BY bucket ORDER BY bucket`, tenant(r), hours)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	defer rows.Close()
	buckets := make([]analyticsBucket, 0)
	for rows.Next() {
		var b analyticsBucket
		if err = rows.Scan(&b.Start, &b.Attempts, &b.Succeeded, &b.Failed, &b.P50DurationMS, &b.P95DurationMS); err != nil {
			problem(w, 503, "storage_unavailable", "Storage unavailable")
			return
		}
		buckets = append(buckets, b)
	}
	if rows.Err() != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"window": window, "buckets": buckets})
}

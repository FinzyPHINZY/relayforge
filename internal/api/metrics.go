package api

import (
	"context"
	"net/http"
	"time"
)

// metricsSummary reads durable state so the numbers remain meaningful after
// dispatcher or worker restarts. Every query is scoped to the API key tenant.
func (s *Server) metricsSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	orgID := tenant(r)
	var accepted, outbox, pending, retryWait, exhausted, succeeded, processing int64
	var oldest *time.Time
	err := s.DB.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM events WHERE organization_id=$1),
  (SELECT count(*) FROM outbox WHERE organization_id=$1 AND status='pending'),
  (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='pending'),
  (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='retry_wait'),
  (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='processing'),
  (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='succeeded'),
  (SELECT count(*) FROM deliveries WHERE organization_id=$1 AND status='exhausted'),
  (SELECT min(created_at) FROM deliveries WHERE organization_id=$1 AND status IN ('pending','retry_wait'))`, orgID).Scan(&accepted, &outbox, &pending, &retryWait, &processing, &succeeded, &exhausted, &oldest)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var attempts, reclaims int64
	err = s.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE error_code='lease_expired') FROM delivery_attempts WHERE organization_id=$1`, orgID).Scan(&attempts, &reclaims)
	if err != nil {
		problem(w, 503, "storage_unavailable", "Storage unavailable")
		return
	}
	var ageSeconds float64
	if oldest != nil {
		ageSeconds = time.Since(*oldest).Seconds()
		if ageSeconds < 0 {
			ageSeconds = 0
		}
	}
	writeJSON(w, 200, map[string]any{"events_accepted": accepted, "outbox_pending": outbox, "deliveries_pending": pending, "deliveries_retry_wait": retryWait, "deliveries_processing": processing, "deliveries_succeeded": succeeded, "deliveries_exhausted": exhausted, "delivery_attempts": attempts, "lease_reclaims": reclaims, "oldest_pending_age_seconds": ageSeconds})
}

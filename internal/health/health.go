package health

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const SchemaVersion = 21

func Ready(ctx context.Context, pool *pgxpool.Pool) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var version int
	return pool.QueryRow(checkCtx, `SELECT version FROM schema_migrations WHERE version=$1`, SchemaVersion).Scan(&version) == nil
}

// Serve exposes process liveness and database/schema readiness on an internal
// port. The caller controls whether that port is published by the deployment.
func Serve(ctx context.Context, pool *pgxpool.Pool, addr string, dependencies ...func(context.Context) error) {
	server := &http.Server{Addr: addr, Handler: Handler(pool, dependencies...), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("health server failed", "error", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func Handler(pool *pgxpool.Pool, dependencies ...func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !Ready(r.Context(), pool) {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, check := range dependencies {
			if check(ctx) != nil {
				http.Error(w, "dependency not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) { metrics(w, r, pool) })
	return mux
}

func metrics(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var accepted, outbox, pending, retryWait, processing, exhausted, unreviewed, reclaims, recentFailures, throttled int64
	var age, dueAge, dispatchAge, dispatchP95, attemptP95 float64
	err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM events),
  (SELECT count(*) FROM outbox WHERE status='pending'),
  (SELECT count(*) FROM deliveries WHERE status='pending'),
  (SELECT count(*) FROM deliveries WHERE status='retry_wait'),
  (SELECT count(*) FROM deliveries WHERE status='processing'),
  (SELECT count(*) FROM deliveries WHERE status='exhausted'),
  (SELECT count(*) FROM deliveries d WHERE d.status='exhausted' AND NOT EXISTS (SELECT 1 FROM delivery_reviews r WHERE r.delivery_id=d.id AND r.organization_id=d.organization_id)),
  (SELECT count(*) FROM delivery_attempts WHERE error_code='lease_expired'),
  (SELECT count(*) FROM delivery_attempts WHERE finished_at>=now()-interval '5 minutes' AND outcome IS DISTINCT FROM 'success'),
  (SELECT count(*) FROM deliveries d JOIN endpoints e ON e.id=d.endpoint_id AND e.organization_id=d.organization_id JOIN endpoint_circuits ec ON ec.endpoint_id=e.id AND ec.organization_id=e.organization_id WHERE d.status IN ('pending','retry_wait') AND d.next_attempt_at<=now() AND e.rate_limit_per_minute>0 AND ec.next_rate_at>now()),
  COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(created_at)) FROM deliveries WHERE status IN ('pending','retry_wait')),0)::double precision,
  GREATEST(COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(next_attempt_at)) FROM deliveries WHERE status IN ('pending','retry_wait') AND next_attempt_at<=now()),0),COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(available_at)) FROM outbox WHERE status='pending' AND available_at<=now()),0))::double precision,
  COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(available_at)) FROM outbox WHERE status='pending' AND available_at<=now()),0)::double precision,
  COALESCE((SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM dispatched_at-available_at)) FROM outbox WHERE dispatched_at>=now()-interval '5 minutes'),0)::double precision,
  COALESCE((SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms/1000.0) FROM delivery_attempts WHERE finished_at>=now()-interval '5 minutes' AND duration_ms IS NOT NULL),0)::double precision`).Scan(&accepted, &outbox, &pending, &retryWait, &processing, &exhausted, &unreviewed, &reclaims, &recentFailures, &throttled, &age, &dueAge, &dispatchAge, &dispatchP95, &attemptP95)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	rows, err := pool.Query(ctx, `SELECT outcome,count(*) FROM delivery_attempts GROUP BY outcome ORDER BY outcome`)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	type outcomeCount struct {
		outcome string
		count   int64
	}
	outcomes := []outcomeCount{}
	for rows.Next() {
		var x outcomeCount
		if err = rows.Scan(&x.outcome, &x.count); err != nil {
			break
		}
		outcomes = append(outcomes, x)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	for _, m := range []struct {
		name  string
		value any
	}{
		{"webhook_events_stored", accepted}, {"webhook_outbox_pending", outbox}, {"webhook_deliveries_pending", pending}, {"webhook_deliveries_retry_wait", retryWait}, {"webhook_deliveries_processing", processing}, {"webhook_deliveries_exhausted", exhausted}, {"webhook_deliveries_unreviewed_exhausted", unreviewed}, {"webhook_lease_reclaims_recorded", reclaims}, {"webhook_delivery_recent_failures_5m", recentFailures}, {"webhook_delivery_rate_throttled", throttled}, {"webhook_delivery_oldest_pending_age_seconds", age}, {"webhook_delivery_oldest_due_age_seconds", dueAge}, {"webhook_outbox_oldest_due_age_seconds", dispatchAge}, {"webhook_outbox_dispatch_lag_p95_seconds_5m", dispatchP95}, {"webhook_delivery_attempt_duration_p95_seconds_5m", attemptP95},
	} {
		fmt.Fprintf(w, "# TYPE %s gauge\n%s %v\n", m.name, m.name, m.value)
	}
	fmt.Fprint(w, "# TYPE webhook_delivery_attempts_recorded gauge\n")
	for _, x := range outcomes {
		fmt.Fprintf(w, "webhook_delivery_attempts_recorded{outcome=%q} %d\n", x.outcome, x.count)
	}
}

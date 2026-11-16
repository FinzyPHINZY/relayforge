package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"webhook_delivery/internal/security"
)

type Checker struct {
	DB                 *pgxpool.Pool
	Services           map[string]string
	BacklogAgeLimit    time.Duration
	RecentFailureLimit int64
	UnreviewedLimit    int64
	HTTPClient         *http.Client
}

func (c Checker) Check(ctx context.Context) []string {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	reasons := make([]string, 0, len(c.Services)+1)
	for name, url := range c.Services {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			reasons = append(reasons, name+" unavailable")
			continue
		}
		response, err := client.Do(request)
		if err != nil {
			reasons = append(reasons, name+" unavailable")
			continue
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			reasons = append(reasons, name+" unavailable")
		}
	}
	if c.DB == nil {
		return append(reasons, "database unavailable")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var age float64
	var recentFailures, unreviewed int64
	err := c.DB.QueryRow(queryCtx, `SELECT GREATEST(
  COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(next_attempt_at)) FROM deliveries WHERE status IN ('pending','retry_wait') AND next_attempt_at<=now()),0),
  COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(available_at)) FROM outbox WHERE status='pending' AND available_at<=now()),0)
)::double precision,
(SELECT count(*) FROM delivery_attempts WHERE finished_at>=now()-interval '5 minutes' AND outcome IS DISTINCT FROM 'success'),
(SELECT count(*) FROM deliveries d WHERE d.status='exhausted' AND NOT EXISTS (SELECT 1 FROM delivery_reviews r WHERE r.delivery_id=d.id AND r.organization_id=d.organization_id))`).Scan(&age, &recentFailures, &unreviewed)
	if err != nil {
		return append(reasons, "database unavailable")
	}
	if age > c.BacklogAgeLimit.Seconds() {
		reasons = append(reasons, fmt.Sprintf("due delivery backlog age exceeds %.0fs", c.BacklogAgeLimit.Seconds()))
	}
	if c.RecentFailureLimit > 0 && recentFailures >= c.RecentFailureLimit {
		reasons = append(reasons, fmt.Sprintf("at least %d failed attempts in 5 minutes", c.RecentFailureLimit))
	}
	if c.UnreviewedLimit > 0 && unreviewed >= c.UnreviewedLimit {
		reasons = append(reasons, fmt.Sprintf("at least %d exhausted deliveries need review", c.UnreviewedLimit))
	}
	return reasons
}

// WebhookNotifier uses the same pinned HTTPS egress policy as delivery to
// protect an operator-configured notification destination from redirects and
// private-network targets.
type WebhookNotifier struct{ URL string }

func (n WebhookNotifier) Notify(ctx context.Context, message string) error {
	client, err := security.PinnedClient(ctx, n.URL, 5*time.Second, false)
	if err != nil {
		return errors.New("notification destination unavailable")
	}
	body, _ := json.Marshal(map[string]string{"text": message, "content": message})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.URL, bytes.NewReader(body))
	if err != nil {
		return errors.New("notification request invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("notification request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("notification destination rejected request")
	}
	return nil
}

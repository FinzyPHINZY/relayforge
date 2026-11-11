package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"webhook_delivery/internal/api"
	"webhook_delivery/internal/id"
	"webhook_delivery/internal/security"
)

func setup(t testing.TB) (*pgxpool.Pool, *httptest.Server) {
	pool, server, _ := setupWithContainer(t)
	return pool, server
}

func setupWithContainer(t testing.TB) (*pgxpool.Pool, *httptest.Server, *postgres.PostgresContainer) {
	t.Helper()
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:17-alpine", postgres.WithDatabase("webhooks"), postgres.WithUsername("webhooks"), postgres.WithPassword("webhooks"), postgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("PostgreSQL did not accept a connection after readiness: %v", err)
	}
	for _, path := range []string{"../../db/migrations/000001_foundation.up.sql", "../../db/migrations/000002_delivery_engine.up.sql", "../../db/migrations/000003_recovery.up.sql", "../../db/migrations/000004_retention.up.sql", "../../db/migrations/000005_endpoint_circuits.up.sql", "../../db/migrations/000006_delivery_reviews.up.sql", "../../db/migrations/000007_fair_claims.up.sql", "../../db/migrations/000008_trace_context.up.sql"} {
		schema, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(schema), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE schema_migrations (version integer PRIMARY KEY); INSERT INTO schema_migrations (version) VALUES (1),(2),(3),(4),(5),(6),(7),(8)`, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&api.Server{DB: pool, EncryptionKey: bytes.Repeat([]byte{1}, 32), EncryptionKeyID: "test"}).Router())
	t.Cleanup(server.Close)
	return pool, server, container
}

func tenantKey(t testing.TB, pool *pgxpool.Pool, name string) (string, string) {
	t.Helper()
	ctx := context.Background()
	org, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := security.NewToken("whk_")
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := security.KeyPrefix(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO organizations (id,name) VALUES ($1,$2)`, org, name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO api_keys (id,organization_id,name,key_prefix,key_hash,scopes) VALUES ($1,$2,'test',$3,$4,$5)`, keyID, org, prefix, security.Hash(raw), []string{"events:write", "events:read", "endpoints:write", "endpoints:read", "deliveries:read", "deliveries:write", "metrics:read"})
	if err != nil {
		t.Fatal(err)
	}
	return org, raw
}

func request(t testing.TB, method, url, key, body string) (*http.Response, []byte) {
	t.Helper()
	r, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+key)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return response, data
}

func TestConcurrentIdempotencyAndOutbox(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "one")
	const n = 20
	ids := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := http.NewRequest("POST", server.URL+"/v1/events", bytes.NewBufferString(`{"type":"order.created","data":{"order_id":"123"}}`))
			if err != nil {
				errs <- err
				return
			}
			r.Header.Set("Authorization", "Bearer "+key)
			r.Header.Set("Idempotency-Key", "order-123")
			resp, err := http.DefaultClient.Do(r)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 202 {
				b, _ := io.ReadAll(resp.Body)
				errs <- fmt.Errorf("status %d: %s", resp.StatusCode, b)
				return
			}
			var event struct {
				ID string `json:"id"`
			}
			if err = json.NewDecoder(resp.Body).Decode(&event); err != nil {
				errs <- err
				return
			}
			ids <- event.ID
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		t.Error(err)
	}
	var first string
	for got := range ids {
		if first == "" {
			first = got
		}
		if got != first {
			t.Errorf("different event IDs: %s / %s", first, got)
		}
	}
	if first == "" {
		t.Fatal("no accepted event")
	}
	var events, outbox, usage int
	err := pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE organization_id=$1`, org).Scan(&events)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox WHERE organization_id=$1`, org).Scan(&outbox)
	if err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(context.Background(), `SELECT sum(event_count) FROM monthly_event_usage WHERE organization_id=$1`, org).Scan(&usage)
	if err != nil {
		t.Fatal(err)
	}
	if events != 1 || outbox != 1 || usage != 1 {
		t.Fatalf("events=%d outbox=%d usage=%d", events, outbox, usage)
	}
	resp, _ := request(t, "GET", server.URL+"/v1/events/"+first, key, "")
	if resp.StatusCode != 200 {
		t.Fatalf("event read status=%d", resp.StatusCode)
	}
	_, otherKey := tenantKey(t, pool, "two")
	resp, _ = request(t, "GET", server.URL+"/v1/events/"+first, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant event read status=%d", resp.StatusCode)
	}
}

func TestEndpointTenantIsolationAndSecretOnce(t *testing.T) {
	pool, server := setup(t)
	_, key := tenantKey(t, pool, "one")
	_, otherKey := tenantKey(t, pool, "two")
	resp, body := request(t, "POST", server.URL+"/v1/endpoints", key, `{"url":"https://8.8.8.8/webhooks","event_types":["order.created"]}`)
	if resp.StatusCode != 201 {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		Endpoint struct {
			ID string `json:"id"`
		} `json:"endpoint"`
		SigningSecret string `json:"signing_secret"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.Endpoint.ID == "" || created.SigningSecret == "" {
		t.Fatal("missing endpoint or one-time secret")
	}
	resp, body = request(t, "GET", server.URL+"/v1/endpoints/"+created.Endpoint.ID, key, "")
	if resp.StatusCode != 200 || bytes.Contains(body, []byte(created.SigningSecret)) {
		t.Fatal("GET leaked secret or failed")
	}
	resp, _ = request(t, "GET", server.URL+"/v1/endpoints/"+created.Endpoint.ID, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant endpoint read status=%d", resp.StatusCode)
	}
	resp, _ = request(t, "PATCH", server.URL+"/v1/endpoints/"+created.Endpoint.ID, otherKey, `{"description":"changed"}`)
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant endpoint patch status=%d", resp.StatusCode)
	}
	resp, _ = request(t, "DELETE", server.URL+"/v1/endpoints/"+created.Endpoint.ID, otherKey, "")
	if resp.StatusCode != 404 {
		t.Fatalf("cross-tenant endpoint delete status=%d", resp.StatusCode)
	}
}

func TestOutboxFailureRollsBackAcceptance(t *testing.T) {
	pool, server := setup(t)
	org, key := tenantKey(t, pool, "one")
	_, err := pool.Exec(context.Background(), `CREATE FUNCTION reject_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'outbox unavailable'; END $$; CREATE TRIGGER fail_outbox BEFORE INSERT ON outbox FOR EACH ROW EXECUTE FUNCTION reject_outbox()`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := request(t, "POST", server.URL+"/v1/events", key, `{"type":"order.created","data":{"id":1}}`)
	if resp.StatusCode != 503 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE organization_id=$1`, org).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("event committed without outbox: %d", count)
	}
}

func TestIdempotencyConflict(t *testing.T) {
	pool, server := setup(t)
	_, key := tenantKey(t, pool, "one")
	for i, body := range []string{`{"type":"order.created","data":{"id":1}}`, `{"type":"order.created","data":{"id":2}}`} {
		r, err := http.NewRequest("POST", server.URL+"/v1/events", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Idempotency-Key", "same")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		want := 202
		if i == 1 {
			want = 409
		}
		if resp.StatusCode != want {
			t.Fatalf("request %d status=%d", i, resp.StatusCode)
		}
	}
}

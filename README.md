# RelayForge

RelayForge is a multi-tenant service for accepting events and delivering signed HTTP webhooks to subscribed endpoints. It stores accepted events in PostgreSQL, creates endpoint-specific deliveries through an outbox dispatcher, and records each delivery attempt so operators can investigate failures and replay exhausted deliveries.

The quickstart below accepts an event and sends a signed webhook to an optional local receiver.

## Delivery contract

| Property | Contract |
| --- | --- |
| Durable acceptance | `POST /v1/events` returns `202` only after the event and outbox row commit in one PostgreSQL transaction. |
| Idempotent ingestion | Repeating a request with the same organization, idempotency key, type, and payload returns the original event ID. Reusing the key for different content returns `409`. |
| Delivery | HTTP attempts are at least once. A receiver can see duplicates, including when its acknowledgment is lost. |
| Acknowledgment | Any HTTP 2xx acknowledges an attempt; it does not prove the receiver completed downstream work. |
| Ordering | v1 does not guarantee event order across endpoints or concurrent attempts. |
| Receiver deduplication | A stable `webhook-id` identifies a delivery run across retries. Receivers should deduplicate with it. |
| Tenant isolation | API keys belong to one organization; resource queries are constrained by organization ID. |

Webhook requests include `webhook-id`, `webhook-timestamp`, `webhook-attempt`, and `webhook-signature` headers. The signature is `v1,<base64(HMAC-SHA256(secret, timestamp + "." + raw_body_bytes))>`. Receivers should compare signatures in constant time, check timestamp freshness, and deduplicate by `webhook-id`.

## Architecture

```text
Publisher → Go/Chi API → PostgreSQL event + outbox → 202 Accepted
                                  ↓
                         Outbox dispatcher
                                  ↓
                         Durable deliveries
                                  ↓
                    Go workers → signed HTTP POST
                                  ↓
                      Attempts, retry, replay
```

PostgreSQL is the source of truth for accepted events, outbox records, delivery state, leases, and attempts. The custom Go worker engine uses bounded requests and database leases with fencing generations. Redis, accessed through go-redis, is reserved for distributed throttling; it is never the durable delivery queue. The optional operator dashboard uses Next.js, TypeScript, Tailwind CSS, and shadcn/ui components.

The backend uses Go, Chi, pgx, and sqlc, with OpenTelemetry trace spans and database-backed metrics. Integration tests use Go testing and Testcontainers. Docker Compose runs the local services, and GitHub Actions runs CI.

Trace context follows an accepted event through PostgreSQL into its outbox dispatch, delivery claim, and HTTP attempt. The services log spans with a shared trace ID; the trace identifier is not included in public API responses or webhook requests.

## Advanced features

- **Publisher SDKs:** Go and TypeScript clients publish events with stable idempotency keys across bounded retries. Use the TypeScript client only in trusted server code; API keys must not be bundled into a browser.
- **Scheduled events:** `POST /v1/events` accepts an optional `deliver_at` time within the next 30 days. The outbox waits until that time before creating deliveries.
- **Endpoint verification:** Set `verification_required` when creating an endpoint, then call `POST /v1/endpoints/{id}/verify`. The receiver must echo the `Webhook-Verification-Challenge` header as `Webhook-Verification-Response` on an HTTP 200 response. Changing the endpoint URL clears verification.
- **Delivery analytics:** `GET /v1/analytics/deliveries?window=24h|7d|30d` returns tenant-scoped hourly attempt counts and p50/p95 duration.
- **Team roles:** Owners can issue a personal API key for an admin, operator, or viewer through `/v1/team/members`. Each request checks the member's current role and status; suspending a member stops key access.
- **Payload transformations:** An endpoint `field_map` selects and renames top-level event data fields. Each delivery snapshots the map, so retries send the same signed body. No arbitrary code or external lookups run in workers.
- **Alert webhooks:** Subscribe an endpoint to `relayforge.delivery.exhausted` to receive a signed alert event when another delivery exhausts its attempts. The alert contains IDs and failure metadata, not the original payload. Alert delivery failure does not create another alert.
- **Regional standby:** With `REGION_ROLE=standby`, the API serves reads but rejects writes, and the dispatcher and worker remain idle. PostgreSQL replication and promotion are external operations; fence the old primary before promoting a standby. This is a single-writer design, not active-active replication.

`deploy/standby-compose.yaml` runs only the read API, dashboard, and HTTPS proxy against an externally replicated PostgreSQL standby. It does not start a database, migrator, dispatcher, or worker. To fail over, first fence the old primary and confirm replica catch-up, promote PostgreSQL, then start the full primary Compose stack in the promoted region with `REGION_ROLE=primary` and switch DNS. [PostgreSQL's failover guidance](https://www.postgresql.org/docs/17/warm-standby-failover.html) explains why fencing is required. [Asynchronous replication](https://www.postgresql.org/docs/17/warm-standby.html) can lose recently acknowledged writes if the primary fails before the replica receives them; measure and accept that recovery point before using this design for customer data.

## Security and recovery design

The v1 design limits payloads to 256 KiB, defaults to eight total delivery attempts, uses exponential backoff with jitter, and reclaims expired worker leases. Terminal delivery history is preserved; manual replay creates a new delivery run. Endpoint URLs must use HTTPS, and each outbound connection must recheck DNS and the actual destination IP to block private and metadata addresses. Redirects are disabled. API keys are hashed, signing secrets are encrypted at rest, and ordinary logs exclude raw credentials and payloads.

Event and delivery history is retained for at least 30 days by default. The retention sweep removes an event only after its outbox is dispatched and every delivery run is terminal and has been complete for 30 days. Failed deliveries remain replayable during that period; pending work is never removed by the sweep. Expired idempotency records remain for at least 24 hours. Set `RETENTION_DAYS` between 30 and 3650 to extend the history window.

Each endpoint has a PostgreSQL-backed circuit breaker shared by workers. Three consecutive retryable failures pause new claims for one minute. When the pause expires, one delivery probes the receiver; success closes the circuit and a retryable failure opens it again. Non-retryable responses do not open the circuit. `GET /v1/endpoints` shows each circuit's state and next probe time.

Workers share claim capacity in PostgreSQL: two in-flight deliveries per endpoint and ten per tenant by default. Set `ENDPOINT_CONCURRENCY_LIMIT` and `TENANT_CONCURRENCY_LIMIT` to change those limits. Due claims prefer the tenant served least recently, so an older backlog from one tenant does not always take the next worker slot. Completed and reclaimed leases release capacity. These limits govern concurrent attempts, not monthly event quotas.

Exhausted deliveries enter an unreviewed dead-letter queue in the dashboard. Operators can mark a delivery investigating or resolved with `PATCH /v1/deliveries/{id}/review`; resolutions require a note. Review changes are recorded in an audit history returned by `GET /v1/deliveries/{id}`. `GET /v1/deliveries?review_status=unreviewed|investigating|resolved` filters the queue. Review state does not change delivery state or automatically replay an event.

Event ingestion and management requests have separate per-key, per-minute rate limits. Compose uses Redis to share these limits across API instances (600 event requests and 120 management requests per minute by default). Exceeded limits return HTTP 429 with `Retry-After`. If Redis is unavailable, writes return HTTP 503, reads use a bounded local limit, and API readiness fails. A standalone API without `REDIS_URL` uses local limits. Monthly event and endpoint quotas remain enforced in PostgreSQL. The dispatcher deletes expired idempotency records after at least 24 hours and removes events only when their outbox is dispatched and every delivery is terminal and past the configured retention window (30 days by default). The worker stops claiming on shutdown and allows in-flight attempts up to `WORKER_DRAIN_SECONDS` to complete.

## Run locally

You need Docker Compose, Go 1.25 or newer, and `jq` for the demo commands. Copy `.env.example` to `.env`, generate a random 32-byte key with `openssl rand -base64 32`, and set `ENCRYPTION_KEY_BASE64` in `.env`. Keep the key for as long as its encrypted endpoint secrets must remain readable.

```sh
cp .env.example .env
# Set ENCRYPTION_KEY_BASE64 and DEMO_RECEIVER_ORIGIN=http://receiver:8090 in .env, then:
docker compose --profile demo up --build -d
curl http://localhost:8080/health/ready
```

Compose binds PostgreSQL to host port 5432 and the API to 8080 by default. Redis is available only to Compose services. If either published port is occupied, set `POSTGRES_HOST_PORT` and `API_HOST_PORT` before running Compose. The examples below assume the defaults.

Provision an organization and scoped API key from the host:

```sh
export DATABASE_URL='postgres://webhooks:webhooks@127.0.0.1:5432/webhooks?sslmode=disable'
go run ./cmd/bootstrap -org 'Example organization'
```

The CLI prints the API key once. Store it securely, then use it to publish an event:

```sh
export WEBHOOK_API_KEY='paste-the-key-from-bootstrap'
curl -i http://localhost:8080/v1/events \
  -H "Authorization: Bearer ${WEBHOOK_API_KEY}" \
  -H 'Idempotency-Key: order-123-created' \
  -H 'Content-Type: application/json' \
  -d '{"type":"order.created","data":{"order_id":"ord_123"}}'
```

The response contains a stable event ID and means the event was durably accepted. Repeat the request with the same key and body to get the same ID.

### Deliver to the local receiver

The demo profile starts a receiver at `127.0.0.1:8090`. It records signature checks and duplicate IDs without exposing payloads. The HTTP exception accepts only the configured `receiver:8090` Docker service and is disabled when `DEMO_RECEIVER_ORIGIN` is empty. Normal endpoints require public HTTPS.

```sh
ENDPOINT_JSON=$(curl -fsS -X POST http://localhost:8080/v1/endpoints -H "Authorization: Bearer ${WEBHOOK_API_KEY}" -H 'Content-Type: application/json' -d '{"url":"http://receiver:8090/webhooks","event_types":["order.created"],"max_attempts":3}')
ENDPOINT_ID=$(printf '%s' "$ENDPOINT_JSON" | jq -r '.endpoint.id')
SIGNING_SECRET=$(printf '%s' "$ENDPOINT_JSON" | jq -r '.signing_secret')
curl -fsS -X POST http://localhost:8090/admin/config -H 'Content-Type: application/json' -d "$(jq -nc --arg secret "$SIGNING_SECRET" '{secret:$secret,failures:2}')"
EVENT_JSON=$(curl -fsS -X POST http://localhost:8080/v1/events -H "Authorization: Bearer ${WEBHOOK_API_KEY}" -H 'Content-Type: application/json' -H "Idempotency-Key: demo-$(date +%s)" -d '{"type":"order.created","data":{"order_id":"demo-1"}}')
EVENT_ID=$(printf '%s' "$EVENT_JSON" | jq -r '.id')
curl -fsS http://localhost:8090/admin/receipts | jq
curl -fsS "http://localhost:8080/v1/deliveries?event_id=${EVENT_ID}" -H "Authorization: Bearer ${WEBHOOK_API_KEY}" | jq
```

The receiver returns 503 twice, then 204. Retry backoff begins at 30 seconds; check receipts again after about two minutes. Each receipt shows `valid_signature: true` and the same `webhook_id` across attempts. Use `GET /v1/deliveries/{id}` to inspect attempt history. To demonstrate crash recovery, configure the receiver with `delay_ms: 10000`, publish another event, kill the worker after the first receipt appears, restart it, and observe a duplicate ID after lease expiry. Receivers must deduplicate that ID.

For an exhausted delivery, set the endpoint's `max_attempts` to 1 for a new event, configure one receiver failure, then call `POST /v1/deliveries/{id}/replay` with an `Idempotency-Key`. Replay creates a linked run and preserves the original attempt history.

### Use the operator dashboard

Generate a separate 32-byte dashboard session key with `openssl rand -base64 32` and set `DASHBOARD_SESSION_KEY` in `.env`. Start the dashboard profile with `docker compose --profile demo --profile dashboard up --build -d`, then open `http://localhost:3000/login`. Sign in with the scoped API key printed by `cmd/bootstrap`.

The dashboard shows tenant-scoped metrics, endpoints, circuit state, events, deliveries, attempt history, dead-letter reviews, and replay controls. You can register and pause endpoints; a newly created endpoint's signing secret appears once. The browser submits the API key to the dashboard server at sign-in; the server verifies it and stores it in an encrypted HTTP-only cookie for eight hours. Browser code does not call the Go API with the key. Key revocation is enforced on subsequent API requests. The Compose dashboard binds to localhost and permits an HTTP cookie for local use; use TLS and secure cookies when deploying it beyond a local machine.

## Develop and test

```sh
go test ./...
go vet ./...
```

Integration tests start real PostgreSQL with Testcontainers and require Docker access. When changing SQL queries or the schema, install sqlc and run `sqlc generate`; commit the generated code with the query changes. Migrations live in `db/migrations/`.

For a focused failure demonstration, run `go test ./tests/integration -run 'Test(CommittedEventSurvivesLostHTTPResponse|DispatchFailureRollsBackMaterializedDeliveries|DatabaseRestartPreservesAcceptedBacklog|SlowReceiverTimesOutWithoutBlockingRecovery)$' -count=1 -v`. It checks a lost acceptance response, atomic dispatch rollback, recovery after PostgreSQL restarts, and a bounded timeout for a slow receiver.

To reproduce the benchmark on a Docker-capable machine, run `go test ./tests/integration -run '^$' -bench 'Benchmark(EventIngestion|PublishToAcknowledgedDelivery)$' -benchtime=500x -count=1 -v`. It reports throughput, allocations, and p50/p95/p99 latency. The delivery benchmark includes event acceptance, outbox dispatch, HTTP send, and attempt persistence. Each CI run publishes its measured output as a `benchmark-results` artifact and job summary, including the runner CPU and Go version. These short fixed-iteration measurements are a baseline, not a sustained-load result or a latency SLO.

For a sustained two-tenant run, use `RELAYFORGE_LOAD_SECONDS=30 go test ./tests/integration -run '^TestSustainedDeliveryLoad$' -count=1 -timeout=2m -v`. It offers roughly 20 events per second to an in-process API backed by Testcontainers PostgreSQL, four workers, and a local receiver, then waits for delivery. The `LOAD_RESULT` line reports measured acceptance and delivery rates, p50/p95/p99 acceptance latency, queue age, memory change, and environment details. CI runs a shorter 15-second version and publishes `load-results`; results are evidence for that runner, not a throughput promise.

The development Compose credentials in `docker-compose.yml` are for local use. Supply production credentials and encryption keys through deployment secrets management, and keep raw API keys, signing secrets, and event payloads out of ordinary logs.

### Single-host test deployment

`deploy/compose.yaml` runs the application behind Caddy HTTPS on a server with two DNS names: one for the dashboard and one for the API. PostgreSQL and Redis have no published host ports. Keep an env file outside the repository with `DB_SECRET_PATH` (path to a private file containing the database password), a matching URL-encoded `DATABASE_URL` pointing at `postgres`, `ENCRYPTION_KEY_BASE64`, `ENCRYPTION_KEY_ID`, `DASHBOARD_SESSION_KEY`, `RELAYFORGE_DASHBOARD_DOMAIN`, and `RELAYFORGE_API_DOMAIN`. Generate separate 32-byte encryption and session keys; protect both the env file and the password file with mode 0600. Point both DNS names at the server and open TCP 80/443 and UDP 443 before starting:

```sh
docker compose --env-file /private/relayforge.env -f deploy/compose.yaml up --build -d
docker compose --env-file /private/relayforge.env -f deploy/compose.yaml ps
```

Migrations complete before the API and workers start. Confirm both HTTPS health/dashboard routes and a signed test delivery after startup. Back up PostgreSQL regularly to private off-host storage and test a restore before relying on the deployment. For example, with a private backup directory and a restrictive shell umask, run `docker compose --env-file /private/relayforge.env -f deploy/compose.yaml exec -T postgres pg_dump -U webhooks -Fc webhooks > /private/backups/relayforge.dump`. Keep the encryption key alongside the backup in approved secret storage.

For tenant removal, `cmd/tenant-admin` supports `-action=suspend`, `-action=export -file=<new-private-file>`, and `-action=purge -file=<export-file> -confirm-org-id=<same-id>`. Suspension stops new API access and queued delivery claims; existing HTTP attempts can finish. The export is a JSON Lines customer-data snapshot with credential metadata but no API key hashes or signing-secret ciphertext. Purge requires a completed export and no active attempts. Take a separate database backup and follow a maintenance procedure before purging; the export is not a database restore backup.

## Repository guide

| Path | Purpose |
| --- | --- |
| `cmd/api`, `cmd/bootstrap`, `cmd/migrate` | HTTP API, organization/key provisioning, and schema migration entry points. |
| `cmd/tenant-admin` | Operator-only tenant suspension, export, and confirmed purge. |
| `cmd/dispatcher`, `cmd/worker`, `cmd/demo-receiver` | Outbox materialization, HTTP delivery, and optional local receiver. |
| `web/dashboard` | Optional Next.js operator console and server-side API client. |
| `internal/api`, `internal/security`, `internal/config` | Handlers, authorization and cryptography, and environment validation. |
| `internal/ratelimit` | Redis-backed API limits shared across instances. |
| `db/migrations`, `db/queries`, `internal/database/sqlc` | PostgreSQL schema, source queries, and generated Go query code. |
| `tests/integration` | Database-backed behavior and tenant-isolation tests. |

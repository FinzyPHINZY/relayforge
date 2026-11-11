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

## Security and recovery design

The v1 design limits payloads to 256 KiB, defaults to eight total delivery attempts, uses exponential backoff with jitter, and reclaims expired worker leases. Terminal delivery history is preserved; manual replay creates a new delivery run. Endpoint URLs must use HTTPS, and each outbound connection must recheck DNS and the actual destination IP to block private and metadata addresses. Redirects are disabled. API keys are hashed, signing secrets are encrypted at rest, and ordinary logs exclude raw credentials and payloads.

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

## Repository guide

| Path | Purpose |
| --- | --- |
| `cmd/api`, `cmd/bootstrap`, `cmd/migrate` | HTTP API, organization/key provisioning, and schema migration entry points. |
| `cmd/dispatcher`, `cmd/worker`, `cmd/demo-receiver` | Outbox materialization, HTTP delivery, and optional local receiver. |
| `web/dashboard` | Optional Next.js operator console and server-side API client. |
| `internal/api`, `internal/security`, `internal/config` | Handlers, authorization and cryptography, and environment validation. |
| `internal/ratelimit` | Redis-backed API limits shared across instances. |
| `db/migrations`, `db/queries`, `internal/database/sqlc` | PostgreSQL schema, source queries, and generated Go query code. |
| `tests/integration` | Database-backed behavior and tenant-isolation tests. |

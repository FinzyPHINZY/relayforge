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

PostgreSQL is the source of truth for accepted events, outbox records, delivery state, leases, and attempts. The custom Go worker engine uses bounded requests and database leases with fencing generations. Redis, accessed through go-redis, is reserved for P1 distributed throttling; it is never the durable delivery queue. The P1 dashboard uses Next.js, TypeScript, Tailwind CSS, and shadcn/ui.

The backend uses Go, Chi, pgx, and sqlc, with OpenTelemetry trace spans and database-backed metrics. Integration tests use Go testing and Testcontainers. Docker Compose runs the local services, and GitHub Actions runs CI.

## Security and recovery design

The v1 design limits payloads to 256 KiB, defaults to eight total delivery attempts, uses exponential backoff with jitter, and reclaims expired worker leases. Terminal delivery history is preserved; manual replay creates a new delivery run. Endpoint URLs must use HTTPS, and each outbound connection must recheck DNS and the actual destination IP to block private and metadata addresses. Redirects are disabled. API keys are hashed, signing secrets are encrypted at rest, and ordinary logs exclude raw credentials and payloads.

Event ingestion and management requests have separate per-key rate limits. These limits are local to each API instance; monthly event and endpoint quotas are enforced in PostgreSQL. The dispatcher deletes expired idempotency records after at least 24 hours and removes events only when their outbox is dispatched and every delivery is terminal and past the configured retention window (30 days by default). The worker stops claiming on shutdown and allows in-flight attempts up to `WORKER_DRAIN_SECONDS` to complete.

## Run locally

You need Docker Compose, Go 1.25 or newer, and `jq` for the demo commands. Copy `.env.example` to `.env`, generate a random 32-byte key with `openssl rand -base64 32`, and set `ENCRYPTION_KEY_BASE64` in `.env`. Keep the key for as long as its encrypted endpoint secrets must remain readable.

```sh
cp .env.example .env
# Set ENCRYPTION_KEY_BASE64 and DEMO_RECEIVER_ORIGIN=http://receiver:8090 in .env, then:
docker compose --profile demo up --build -d
curl http://localhost:8080/health/ready
```

Compose binds PostgreSQL to host port 5432 and the API to 8080 by default. If either port is occupied, set `POSTGRES_HOST_PORT` and `API_HOST_PORT` before running Compose. The examples below assume the defaults.

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

## Develop and test

```sh
go test ./...
go vet ./...
```

Integration tests start real PostgreSQL with Testcontainers and require Docker access. When changing SQL queries or the schema, install sqlc and run `sqlc generate`; commit the generated code with the query changes. Migrations live in `db/migrations/`.

For a focused failure demonstration, run `go test ./tests/integration -run 'Test(CommittedEventSurvivesLostHTTPResponse|DispatchFailureRollsBackMaterializedDeliveries|DatabaseRestartPreservesAcceptedBacklog)$' -count=1 -v`. It checks a lost acceptance response, atomic dispatch rollback, and recovery after PostgreSQL restarts.

To reproduce the benchmark on a Docker-capable machine, run `go test ./tests/integration -run '^$' -bench 'Benchmark(EventIngestion|PublishToAcknowledgedDelivery)$' -benchtime=500x -count=1 -v`. It reports throughput, allocations, and p50/p95/p99 latency. The delivery benchmark includes event acceptance, outbox dispatch, HTTP send, and attempt persistence. Each CI run publishes its measured output as a `benchmark-results` artifact and job summary, including the runner CPU and Go version. These short fixed-iteration measurements are a baseline, not a sustained-load result or a latency SLO.

The development Compose credentials in `docker-compose.yml` are for local use. Supply production credentials and encryption keys through deployment secrets management, and keep raw API keys, signing secrets, and event payloads out of ordinary logs.

## Repository guide

| Path | Purpose |
| --- | --- |
| `cmd/api`, `cmd/bootstrap`, `cmd/migrate` | HTTP API, organization/key provisioning, and schema migration entry points. |
| `cmd/dispatcher`, `cmd/worker`, `cmd/demo-receiver` | Outbox materialization, HTTP delivery, and optional local receiver. |
| `internal/api`, `internal/security`, `internal/config` | Handlers, authorization and cryptography, and environment validation. |
| `db/migrations`, `db/queries`, `internal/database/sqlc` | PostgreSQL schema, source queries, and generated Go query code. |
| `tests/integration` | Database-backed behavior and tenant-isolation tests. |

# RelayForge

RelayForge is a multi-tenant service for accepting events and delivering signed HTTP webhooks to subscribed endpoints. It stores accepted events in PostgreSQL, creates endpoint-specific deliveries through an outbox dispatcher, and records each delivery attempt so operators can investigate failures and replay exhausted deliveries.

The quickstart below exercises the ingestion API. It does not send a webhook; the dispatcher and worker are separate components in the platform architecture.

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

The planned webhook request includes `webhook-id`, `webhook-timestamp`, `webhook-attempt`, and `webhook-signature` headers. The signature is `v1,<base64(HMAC-SHA256(secret, timestamp + "." + raw_body_bytes))>`. Receivers should compare signatures in constant time, check timestamp freshness, and deduplicate by `webhook-id`.

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

The backend uses Go, Chi, pgx, and sqlc, with OpenTelemetry for tracing and metrics. Integration tests use Go testing and Testcontainers. Docker Compose runs the local services, and GitHub Actions runs CI.

## Security and recovery design

The v1 design limits payloads to 256 KiB, defaults to eight total delivery attempts, uses exponential backoff with jitter, and reclaims expired worker leases. Terminal delivery history is preserved; manual replay creates a new delivery run. Endpoint URLs must use HTTPS, and each outbound connection must recheck DNS and the actual destination IP to block private and metadata addresses. Redirects are disabled. API keys are hashed, signing secrets are encrypted at rest, and ordinary logs exclude raw credentials and payloads.

## Run locally

You need Docker Compose and Go 1.25 or newer. Copy `.env.example` to `.env`, generate a random 32-byte key with `openssl rand -base64 32`, and set `ENCRYPTION_KEY_BASE64` in `.env`. Keep the key for as long as its encrypted endpoint secrets must remain readable.

```sh
cp .env.example .env
# Set ENCRYPTION_KEY_BASE64 in .env, then:
docker compose up --build -d
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

The response contains a stable event ID and means the event was durably accepted. Repeat the request with the same key and body to get the same ID. The endpoint API accepts only public HTTPS receiver URLs.

## Develop and test

```sh
go test ./...
go vet ./...
```

Integration tests start real PostgreSQL with Testcontainers and require Docker access. When changing SQL queries or the schema, install sqlc and run `sqlc generate`; commit the generated code with the query changes. Migrations live in `db/migrations/`.

The development Compose credentials in `docker-compose.yml` are for local use. Supply production credentials and encryption keys through deployment secrets management, and keep raw API keys, signing secrets, and event payloads out of ordinary logs.

## Repository guide

| Path | Purpose |
| --- | --- |
| `cmd/api`, `cmd/bootstrap`, `cmd/migrate` | HTTP API, organization/key provisioning, and schema migration entry points. |
| `internal/api`, `internal/security`, `internal/config` | Handlers, authorization and cryptography, and environment validation. |
| `db/migrations`, `db/queries`, `internal/database/sqlc` | PostgreSQL schema, source queries, and generated Go query code. |
| `tests/integration` | Database-backed behavior and tenant-isolation tests. |

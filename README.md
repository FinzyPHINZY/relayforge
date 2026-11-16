# RelayForge

RelayForge is a multi-tenant service for accepting events and delivering signed HTTP webhooks to subscribed endpoints. It stores accepted events in PostgreSQL, creates endpoint-specific deliveries through an outbox dispatcher, and records each delivery attempt so operators can investigate failures and replay exhausted deliveries.

The quickstart below accepts an event and sends a signed webhook to an optional local receiver.

The public [How RelayForge Works](web/dashboard/app/how-it-works/page.tsx) page explains the delivery path, post-receipt crash window, controls, and measured reliability proof. The `/demo` route illustrates acceptance, a retryable receiver failure, recovery, and delivery history without an API key. It uses example data and makes no API calls.

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

Trace context follows an accepted event through PostgreSQL into its outbox dispatch, delivery claim, and HTTP attempt. Services emit JSON logs with a trace ID for in-context events and export allowlisted spans and OpenTelemetry metrics to an OTLP collector when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. The trace identifier is not included in public API responses or webhook requests. The trace exporter omits span attributes such as destination URLs and payloads. Metric labels use a fixed outcome set, avoiding tenant or endpoint IDs.

## Capabilities at a glance

| Area | What RelayForge provides |
| --- | --- |
| Publish | Organization-scoped API keys, idempotent event acceptance, scheduled publication, monthly quotas, Go and TypeScript publisher SDKs. |
| Deliver | Transactional outbox, subscription matching, signed HTTP requests, bounded retries with jitter and `Retry-After`, durable leases and attempt history. |
| Control | Endpoint verification, secret rotation, pause/resume, timeout and retry policies, outbound rate and concurrency limits, circuit breakers, and payload field mapping. |
| Recover | Exhausted-delivery review, manual replay as a linked new run, immutable operator audit history, and signed exhaustion alert events. |
| Protect | Tenant isolation, key scopes and team roles, private-address and metadata blocking, DNS revalidation, pinned egress, and blocked redirects. |
| Observe | Overview trends, event explorer, endpoint management, delivery inspector, OpenTelemetry traces and metrics, structured logs, and a monitor command. |
| Operate | Docker Compose local stack, PostgreSQL-backed retention and tenant export/purge, and a read-only regional standby mode for externally managed failover. |

The public entry points are `/` for the landing page, `/how-it-works` for the architecture and failure proof, and `/demo` for the guided illustration. The authenticated console starts at `/dashboard` and includes events, endpoints, deliveries, team administration, and audit history according to the key's scopes.

## Measured reliability proof

On 2026-11-15, the opt-in Testcontainers proof published exactly **10,000 accepted events** across two tenants. It reconciled **10,000 dispatched outbox records and 10,000 terminal deliveries**: 9,995 succeeded and five exhausted after intentionally injected HTTP 400 responses. The run also injected five timeouts, five HTTP 503 responses, and a worker crash after a receiver acknowledgment. It recorded 10,011 attempts, 11 retries, one reclaimed lease, and 11 duplicate receiver receipts. No accepted publisher event was missing from the event, outbox, and delivery reconciliation in this run.

| Measurement | Observed result |
| --- | ---: |
| Publish duration | 1m 55s |
| Total test duration | 2m 48s |
| Accepted events per second | 86.85 |
| Acceptance latency p50 / p95 / p99 | 155 / 403 / 536 ms |
| Event-to-terminal-delivery latency p50 / p95 / p99 | 22.37 / 50.43 / 52.40 s |
| Oldest pending delivery at reconciliation | 0 s |

The run used PostgreSQL 17 in Testcontainers, Go 1.25.0, an 8-CPU host, 16 concurrent publishers, 32 workers, and two endpoints with per-tenant and per-endpoint claim caps of 20. These are measurements from one local test, not a production throughput or latency guarantee. The 11 duplicates demonstrate why receivers must deduplicate by `webhook-id`. Reproduce the test with the command below.

## Advanced features

- **Publisher SDKs:** Go and TypeScript clients publish events with stable idempotency keys across bounded retries. Use the TypeScript client only in trusted server code; API keys must not be bundled into a browser.
- **Scheduled events:** `POST /v1/events` accepts an optional `deliver_at` time within the next 30 days. The outbox waits until that time before creating deliveries.
- **Endpoint verification:** Set `verification_required` when creating an endpoint, then call `POST /v1/endpoints/{id}/verify`. The receiver must echo the `Webhook-Verification-Challenge` header as `Webhook-Verification-Response` on an HTTP 200 response. Changing the endpoint URL clears verification.
- **Delivery analytics:** `GET /v1/analytics/deliveries?window=24h|7d|30d` returns tenant-scoped hourly attempt counts and p50/p95 duration.
- **Overview trends:** The console shows tenant-scoped delivery success rate, accepted events, exhausted deliveries, retries, throughput, and event-to-terminal delivery latency for selectable UTC windows. `GET /v1/analytics/overview?window=24h|7d|30d` supplies the exact bucket values. Failed attempts and exhausted deliveries are reported separately.
- **Event explorer:** Search tenant events by ID, type prefix, UTC acceptance time, or delivery outcome, then follow each event to its delivery runs and attempts. An event matches an outcome when any delivery run has that status. Event metadata reads omit payloads; owners and admins can explicitly reveal a redacted view, and each reveal is audited.
- **Endpoint policy controls:** Create or update receiver timeout, maximum attempts, per-endpoint rate, and concurrent attempts from the console. New delivery runs snapshot the timeout and retry limit; the detail view shows circuit state, verification, and signing-secret overlap.
- **Delivery inspector:** Follow a delivery from event acceptance through dispatch, attempts, exact retry schedules, terminal outcome, review, and replay in a chronological timeline. The console shows the next due time and only safe, allowlisted response excerpts.
- **Team roles:** Owners can issue a personal API key for an admin, operator, or viewer through `/v1/team/members`. Each request checks the member's current role and status; suspending a member stops key access.
- **Operator audit history:** Endpoint, team, key issuance, review, and replay changes are recorded transactionally with actor and target IDs. Owners can inspect the history in the console or through `GET /v1/audit`.
- **Payload transformations:** An endpoint `field_map` selects and renames top-level event data fields. Each delivery snapshots the map, so retries send the same signed body. No arbitrary code or external lookups run in workers.
- **Alert webhooks:** Subscribe an endpoint to `relayforge.delivery.exhausted` to receive a signed alert event when another delivery exhausts its attempts. The alert contains IDs and failure metadata, not the original payload. Alert delivery failure does not create another alert.
- **Regional standby:** With `REGION_ROLE=standby`, the API serves reads but rejects writes, and the dispatcher and worker remain idle. PostgreSQL replication and promotion are external operations; fence the old primary before promoting a standby. This is a single-writer design, not active-active replication.

`deploy/standby-compose.yaml` runs only the read API, dashboard, and HTTPS proxy against an externally replicated PostgreSQL standby. It does not start a database, migrator, dispatcher, or worker. To fail over, first fence the old primary and confirm replica catch-up, promote PostgreSQL, then start the full primary Compose stack in the promoted region with `REGION_ROLE=primary` and switch DNS. [PostgreSQL's failover guidance](https://www.postgresql.org/docs/17/warm-standby-failover.html) explains why fencing is required. [Asynchronous replication](https://www.postgresql.org/docs/17/warm-standby.html) can lose recently acknowledged writes if the primary fails before the replica receives them; measure and accept that recovery point before using this design for customer data.

## Security and recovery design

The v1 design limits payloads to 256 KiB, defaults to eight total delivery attempts, uses exponential backoff with jitter, and reclaims expired worker leases. Terminal delivery history is preserved; manual replay creates a new delivery run. Endpoint URLs must use HTTPS, and each outbound connection must recheck DNS and the actual destination IP to block private and metadata addresses. Redirects are disabled. API keys are hashed, signing secrets are encrypted at rest, and ordinary logs exclude raw credentials and payloads. After a signing-secret overlap expires, the dispatcher retires it and erases its encrypted key material while retaining non-secret metadata for history.

Event and delivery history is retained for at least 30 days by default. The retention sweep removes an event only after its outbox is dispatched and every delivery run is terminal and has been complete for 30 days. Failed deliveries remain replayable during that period; pending work is never removed by the sweep. Expired idempotency records remain for at least 24 hours. Set `RETENTION_DAYS` between 30 and 3650 to extend the history window.

Each endpoint has a PostgreSQL-backed circuit breaker shared by workers. Three consecutive retryable failures pause new claims for one minute. When the pause expires, one delivery probes the receiver; success closes the circuit and a retryable failure opens it again. Non-retryable responses do not open the circuit. `GET /v1/endpoints` shows each circuit's state and next probe time.

Workers share claim capacity in PostgreSQL. Each endpoint defaults to two concurrent attempts and an unlimited delivery rate; `concurrency_limit` (1–100) and `rate_limit_per_minute` (0–6,000, where 0 means unlimited) can be set when creating or updating an endpoint. Rate-limited attempts are evenly paced, with each claim reserving a durable slot before sending. A failed send still consumes that slot. An edited rate retains the previous reservation and applies the new spacing to subsequent claims. `ENDPOINT_CONCURRENCY_LIMIT` is an operator safety ceiling across all endpoints (100 by default), while `TENANT_CONCURRENCY_LIMIT` caps each tenant at ten concurrent attempts by default. Due claims prefer the tenant served least recently, so an older backlog from one tenant does not always take the next worker slot. Completed and reclaimed leases release concurrency capacity. These outbound controls differ from API request limits and monthly event quotas.

Exhausted deliveries enter an unreviewed dead-letter queue in the dashboard. Operators can mark a delivery investigating or resolved with `PATCH /v1/deliveries/{id}/review`; resolutions require a note. Review changes are recorded in the delivery's history returned by `GET /v1/deliveries/{id}`. `GET /v1/deliveries?review_status=unreviewed|investigating|resolved` filters the queue. Review state does not change delivery state or automatically replay an event.

The general audit trail records endpoint creation, configuration, disablement, verification and secret rotation; team and key issuance changes; delivery review and replay; payload inspections; and organization bootstrap or suspension. Each record includes the actor, action, target, timestamp, and limited non-secret metadata. Mutations and their audit records commit together, and normal updates/deletes of audit records are blocked in PostgreSQL. Owners can read their tenant's history with `GET /v1/audit` or the dashboard's **Team → Audit history** link. Audit records remain for the organization lifetime, are included in a tenant export, and are removed only by a confirmed tenant purge after export. Raw keys, signing secrets, payloads, destination URLs, review notes, and member email addresses are excluded from audit metadata.

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

The receiver returns 503 twice, then 204. Retry backoff begins at 30 seconds; check receipts again after about two minutes. Each receipt shows `valid_signature: true` and the same `webhook_id` across attempts. Use `GET /v1/deliveries/{id}` to inspect attempt history. Receivers must deduplicate that ID.

For an exhausted delivery, set the endpoint's `max_attempts` to 1 for a new event, configure one receiver failure, then call `POST /v1/deliveries/{id}/replay` with an `Idempotency-Key`. Replay creates a linked run and preserves the original attempt history.

### Use the operator dashboard

Generate a separate 32-byte dashboard session key with `openssl rand -base64 32` and set `DASHBOARD_SESSION_KEY` in `.env`. Start the dashboard profile with `docker compose --profile demo --profile dashboard up --build -d`, then open `http://localhost:3000` for the public landing page. Select **Open console** and sign in with the scoped API key printed by `cmd/bootstrap`. The authenticated overview is at `/dashboard`.

The dashboard shows tenant-scoped metrics, endpoints, circuit state, events, deliveries, attempt history, dead-letter reviews, replay controls, and owner-only audit history. You can register and pause endpoints; a newly created endpoint's signing secret appears once. The browser submits the API key to the dashboard server at sign-in; the server verifies it and stores it in an encrypted HTTP-only cookie for eight hours. Browser code does not call the Go API with the key. Key revocation is enforced on subsequent API requests. The Compose dashboard binds to localhost and permits an HTTP cookie for local use; use TLS and secure cookies when deploying it beyond a local machine.

The public `/demo` route walks visitors through a simulated failed receiver and successful retry. Authenticated operators can publish or schedule events, inspect analytics, manage team access, configure endpoint verification and field mappings, subscribe to exhaustion alerts, and rotate signing secrets. Write controls appear only for keys with the matching scope.

## Develop and test

```sh
go test ./...
go vet ./...
```

Integration tests start real PostgreSQL with Testcontainers and require Docker access. When changing SQL queries or the schema, install sqlc and run `sqlc generate`; commit the generated code with the query changes. Migrations live in `db/migrations/`.

For a focused failure demonstration, run `go test ./tests/integration -run 'Test(CommittedEventSurvivesLostHTTPResponse|DispatchFailureRollsBackMaterializedDeliveries|DatabaseRestartPreservesAcceptedBacklog|SlowReceiverTimesOutWithoutBlockingRecovery)$' -count=1 -v`. It checks a lost acceptance response, atomic dispatch rollback, recovery after PostgreSQL restarts, and a bounded timeout for a slow receiver.

To reproduce the post-receipt crash window with a real OS process, run `go test ./tests/integration -run '^TestRealProcessCrashAfterReceiverAcknowledges$' -count=1 -v` with Docker available. The test starts a worker process, waits until its signed webhook receives HTTP 204, then kills that process before it records completion. After the five-second lease expires, a second process reclaims the attempt and sends the same delivery ID again. Assertions check two receiver receipts, an `unknown` attempt with `lease_expired`, a successful second attempt, one durable event, one delivery, and a dispatched outbox record. This is why delivery is at least once: the receiver may finish its work while the sender loses the acknowledgment or crashes before recording it. The receiver should use `webhook-id` as its deduplication key and persist that key with its own work. Manual replay is different: it creates a new delivery run.

To reproduce the benchmark on a Docker-capable machine, run `go test ./tests/integration -run '^$' -bench 'Benchmark(EventIngestion|PublishToAcknowledgedDelivery)$' -benchtime=500x -count=1 -v`. It reports throughput, allocations, and p50/p95/p99 latency. The delivery benchmark includes event acceptance, outbox dispatch, HTTP send, and attempt persistence. Each CI run publishes its measured output as a `benchmark-results` artifact and job summary, including the runner CPU and Go version. These short fixed-iteration measurements are a baseline, not a sustained-load result or a latency SLO.

For a sustained two-tenant run, use `RELAYFORGE_LOAD_SECONDS=30 go test ./tests/integration -run '^TestSustainedDeliveryLoad$' -count=1 -timeout=2m -v`. It offers roughly 20 events per second to an in-process API backed by Testcontainers PostgreSQL, four workers, and a local receiver, then waits for delivery. The `LOAD_RESULT` line reports measured acceptance and delivery rates, p50/p95/p99 acceptance latency, queue age, memory change, and environment details. CI runs a shorter 15-second version and publishes `load-results`; results are evidence for that runner, not a throughput promise.

For the 10,000-event reliability proof, run `RELAYFORGE_PROOF_EVENTS=10000 go test ./tests/integration -run '^TestTenThousandEventReliabilityProof$' -count=1 -timeout=20m -v` on a Docker-capable host. The opt-in test uses two tenants with one subscribed endpoint each, 32 workers, deterministic HTTP 503, timeout, and HTTP 400 injections, plus a killed sender process in the post-receipt crash window. It reconciles all 10,000 publisher events with dispatched outbox rows and one terminal delivery each, then checks attempt history, retries, duplicate receipts, signatures, and the internal metrics endpoint. The `PROOF_RESULT` line reports measured rates, acceptance and delivery latency percentiles, queue age, and resource context. Use `RELAYFORGE_PROOF_EVENTS=100` for a short harness check; only the 10,000-event run satisfies this proof.

The development Compose credentials in `docker-compose.yml` are for local use. Supply production credentials and encryption keys through deployment secrets management, and keep raw API keys, signing secrets, and event payloads out of ordinary logs.

### Single-host test deployment

`deploy/compose.yaml` runs the application behind Caddy HTTPS on a server with two DNS names: one for the dashboard and one for the API. PostgreSQL and Redis have no published host ports. Keep an env file outside the repository with `DB_SECRET_PATH` (path to a private file containing the database password), a matching URL-encoded `DATABASE_URL` pointing at `postgres`, `ENCRYPTION_KEY_BASE64`, `ENCRYPTION_KEY_ID`, `DASHBOARD_SESSION_KEY`, `RELAYFORGE_DASHBOARD_DOMAIN`, and `RELAYFORGE_API_DOMAIN`. Generate separate 32-byte encryption and session keys; protect both the env file and the password file with mode 0600. Point both DNS names at the server and open TCP 80/443 and UDP 443 before starting:

```sh
docker compose --env-file /private/relayforge.env -f deploy/compose.yaml up --build -d
docker compose --env-file /private/relayforge.env -f deploy/compose.yaml ps
```

Migrations complete before the API and workers start. Confirm both HTTPS health/dashboard routes and a signed test delivery after startup. Back up PostgreSQL regularly to private off-host storage and test a restore before relying on the deployment. For example, with a private backup directory and a restrictive shell umask, run `docker compose --env-file /private/relayforge.env -f deploy/compose.yaml exec -T postgres pg_dump -U webhooks -Fc webhooks > /private/backups/relayforge.dump`. Keep the encryption key alongside the backup in approved secret storage.

For basic internal monitoring, set `MONITOR_WEBHOOK_URL` in the private env file to an operator-owned HTTPS notification webhook, then start the `monitoring` profile. The monitor probes API, dispatcher, and worker readiness, due queue age, failed attempts in the last five minutes, and exhausted deliveries awaiting review every 60 seconds. It sends a notification when the failure set changes or recovers. `MONITOR_BACKLOG_AGE_SECONDS` defaults to 300; `MONITOR_FAILURE_THRESHOLD` defaults to 5 and `MONITOR_UNREVIEWED_THRESHOLD` to 1. Notification destinations are HTTPS-only, re-resolved and pinned before each send, and their URL is never logged. Configure an **external** uptime check for `https://<api-domain>/health/ready` and `https://<dashboard-domain>/` to detect a full VM or network outage; an on-host monitor cannot detect its own host disappearing. Test both notification delivery and recovery. The monitor requires a reachable webhook and starts with `docker compose --env-file /private/relayforge.env -f deploy/compose.yaml --profile monitoring up --build -d`.

For a local operator view of traces and service metrics, use the optional telemetry overlay after setting `ENCRYPTION_KEY_BASE64` in `.env`:

```sh
docker compose -f docker-compose.yml -f deploy/telemetry/compose.yaml --profile demo --profile dashboard up --build -d
```

Jaeger is at `http://localhost:16686` and Prometheus is at `http://localhost:9090`. Search Jaeger for the `relayforge-api`, `relayforge-dispatcher`, and `relayforge-worker` services. In Prometheus, query `webhook_delivery_oldest_due_age_seconds{job="relayforge-api"}`, `webhook_delivery_recent_failures_5m{job="relayforge-api"}`, `webhook_deliveries_unreviewed_exhausted{job="relayforge-api"}`, `webhook_delivery_rate_throttled{job="relayforge-api"}`, `webhook_delivery_attempt_duration_p95_seconds_5m{job="relayforge-api"}`, or `webhook_delivery_attempts_recorded{job="relayforge-api"}`. These are database-wide snapshots; use one job label to avoid counting the same values three times. The local telemetry UIs bind to loopback. The `/metrics` endpoint stays on each service's internal health port. The collector forwards traces to Jaeger and OpenTelemetry counters and histograms to Prometheus. Prometheus retains seven days of data in the overlay container. For production, set `OTEL_EXPORTER_OTLP_ENDPOINT` to your private OTLP receiver and scrape the internal `/metrics` endpoints with your monitoring system.

For tenant removal, `cmd/tenant-admin` supports `-action=suspend`, `-action=export -file=<new-private-file>`, and `-action=purge -file=<export-file> -confirm-org-id=<same-id>`. Suspension stops new API access and queued delivery claims; existing HTTP attempts can finish. The export is a JSON Lines customer-data snapshot with credential metadata and audit history, but no API key hashes or signing-secret ciphertext. Purge requires a completed export and no active attempts. Take a separate database backup and follow a maintenance procedure before purging; the export is not a database restore backup.

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

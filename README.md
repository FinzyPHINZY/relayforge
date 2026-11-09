# Reliable Webhook Delivery Platform

Week 1 is complete. The API accepts tenant-scoped events into PostgreSQL and records an outbox row in the same transaction. PostgreSQL integration tests and a Compose smoke check passed. Dispatch and HTTP delivery are planned for Week 2; see [PRODUCT.md](PRODUCT.md) and [Week 1 plan](docs/week-1-plan.md).

## Local setup

Requires Go 1.25+, Docker Compose, and sqlc. A verified Go 1.27.3 binary is available locally through `scripts/with-local-go`; it uses writable workspace caches and takes precedence over the older system Go. Create `.env` from `.env.example` and set `ENCRYPTION_KEY_BASE64` to a random 32-byte key encoded as base64 (for example, `openssl rand -base64 32`). Keep that key private.

```sh
scripts/with-local-go go mod tidy
sqlc generate
docker compose up --build
```

Compose defaults to host ports 5432 and 8080. If either is occupied, set `POSTGRES_HOST_PORT` and `API_HOST_PORT` before starting Compose, then use those ports in the bootstrap and API examples below.

Provision an organization and API key from the host:

```sh
export DATABASE_URL='postgres://webhooks:webhooks@localhost:5432/webhooks?sslmode=disable'
scripts/with-local-go go run ./cmd/bootstrap -org 'Example organization'
```

The bootstrap command prints the API key once. Use it to create an endpoint or publish an event:

```sh
curl -i http://localhost:8080/v1/events \
  -H 'Authorization: Bearer YOUR_KEY' \
  -H 'Idempotency-Key: order-123-created' \
  -H 'Content-Type: application/json' \
  -d '{"type":"order.created","data":{"order_id":"ord_123"}}'
```

The response is `202` after the event, outbox row, usage count, and idempotency record commit. The same key and body return the same event ID. A different body with the same key returns `409`.

## Tests

```sh
scripts/with-local-go go test ./...
```

The integration suite uses Testcontainers and needs Docker. It checks concurrent idempotency, outbox atomicity, one-time endpoint secret exposure, and tenant isolation. API keys and signing secrets are sensitive; do not paste them into issue reports or logs.

If the Codex workspace cannot access external networking or Docker sockets, run `./scripts/verify-week1` in a regular terminal. It checks Docker access, downloads Go modules, installs sqlc, generates query code, and runs the tests. Its output is saved to `.tools/week1-verify.log` for review.

Run `./scripts/smoke-week1` in a regular terminal to build and start the Compose stack, provision an organization, create an endpoint, publish the same event twice, and verify the single committed outbox row. It cleans up its isolated Compose project and saves output to `.tools/week1-smoke.log`.

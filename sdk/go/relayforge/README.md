# relayforge (Go)

A small Go client for publishing events to RelayForge. It sends every attempt with the same idempotency key and retries transient failures. It uses only the standard library.

## Install

Requires Go 1.22 or newer.

```sh
go get github.com/FinzyPHINZY/relayforge/sdk/go/relayforge
```

```go
import "github.com/FinzyPHINZY/relayforge/sdk/go/relayforge"
```

## Publish an event

```go
client, err := relayforge.New(
	os.Getenv("RELAYFORGE_URL"),     // e.g. http://localhost:8080
	os.Getenv("RELAYFORGE_API_KEY"), // whk_..., needs the events:write scope
)
if err != nil {
	log.Fatal(err)
}

event, err := client.Publish(ctx, "order.created",
	map[string]any{"order_id": "ord_81f2", "total": 4200},
	"order.created:ord_81f2", // idempotency key
)
if err != nil {
	return err
}
// event.ID, event.Type, event.Status ("accepted"), event.CreatedAt
```

The base URL must use HTTPS, except for `localhost`, `127.0.0.1`, and `::1`.

Event types are lowercase and dotted (`order.created`) and cannot start with `relayforge.`. Event data is marshaled with `encoding/json` and can be up to 256 KiB.

### Choose your own idempotency key

If you pass an empty key, the client generates one per call. That key covers the client's own retries, but not your process restarting and calling `Publish` again. Derive the key from the business action, such as `order.created:ord_81f2`, so any retry of that action reuses it. Keys can be up to 255 characters.

Sending the same key with the same type, data, and delivery time returns the original event. Reusing a key for a different event fails with `409 idempotency_conflict`.

## Schedule an event

```go
event, err := client.PublishAt(ctx, "trial.ending",
	map[string]any{"account_id": "acc_19"},
	time.Now().Add(24*time.Hour),
	"trial.ending:acc_19",
)
```

The delivery time must be in the future and within 30 days. If you retry after a restart, keep both the key and the time the same.

## Options

```go
client, err := relayforge.New(url, key,
	relayforge.WithRetries(4),                                   // 0–10, default 2
	relayforge.WithHTTPClient(&http.Client{Timeout: 5 * time.Second}), // default: 10s timeout
)
```

Cancel a publish, including its retry waits, through `ctx`.

Retries cover network errors and HTTP 408, 425, 429, and 5xx. The backoff starts at 200 ms, doubles each time, and caps at 2 s. A `Retry-After` header of up to 30 seconds is honored. Other 4xx responses fail immediately.

## Errors

```go
var apiErr *relayforge.APIError
if errors.As(err, &apiErr) {
	log.Printf("HTTP %d %s: %s", apiErr.StatusCode, apiErr.Code, apiErr.Message)
}
```

| Status | `Code` | Meaning |
| --- | --- | --- |
| 401 | `unauthorized` | Missing, revoked, or expired API key |
| 403 | `forbidden` | Key lacks `events:write` |
| 409 | `idempotency_conflict` | Key already used for a different event |
| 413 | `payload_too_large` | Data exceeds 256 KiB |
| 422 | `invalid_event` / `invalid_schedule` | Bad type or data, or delivery time out of range |
| 429 | `rate_limited` / `quota_exceeded` | Request rate or monthly event quota exceeded |
| 503 | `storage_unavailable` | The API could not commit the event |

Network failures that outlast every retry are returned as the underlying error.

## Receiving webhooks

See the receiver guide at `/docs#receive` on the RelayForge site. In short: verify `webhook-signature` over the raw body, reject stale `webhook-timestamp` values, and deduplicate by `webhook-id`.

## Releasing

This directory is its own Go module, separate from the server's `webhook_delivery` module, so its versions are tagged with a directory prefix:

```sh
git tag sdk/go/relayforge/v0.1.0
git push origin sdk/go/relayforge/v0.1.0
```

The `Release SDK` workflow runs the tests and asks `proxy.golang.org` to index the version, so `go get` and pkg.go.dev pick it up straight away.

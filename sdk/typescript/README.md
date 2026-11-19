# @finzyphinzy/relayforge

A small TypeScript client for publishing events to RelayForge. It sends every attempt with the same idempotency key and retries transient failures.

Use it only in trusted server code. The API key must never be bundled into browser JavaScript.

Requires Node.js 20 or newer (global `fetch` and `crypto.randomUUID`). It has no runtime dependencies.

## Install

```sh
npm install @finzyphinzy/relayforge
```

## Publish an event

```ts
import { Publisher } from "@finzyphinzy/relayforge";

const relayforge = new Publisher(
  process.env.RELAYFORGE_URL!,     // e.g. http://localhost:8080
  process.env.RELAYFORGE_API_KEY!, // whk_..., needs the events:write scope
);

const event = await relayforge.publish(
  "order.created",
  { order_id: "ord_81f2", total: 4200 },
  { idempotencyKey: "order.created:ord_81f2" },
);
// { id: "evt_...", type: "order.created", status: "accepted", created_at: "..." }
```

The base URL must use HTTPS, except for `localhost`, `127.0.0.1`, and `[::1]`.

Event types are lowercase and dotted (`order.created`) and cannot start with `relayforge.`. Event data can be up to 256 KiB.

### Choose your own idempotency key

If `idempotencyKey` is left out, the client generates one per `publish` call. That key covers the client's own retries, but not your process restarting and calling `publish` again. Derive the key from the business action, such as `order.created:ord_81f2`, so any retry of that action reuses it. Keys can be up to 255 characters.

Sending the same key with the same type, data, and `deliverAt` returns the original event. Reusing a key for a different event fails with `409 idempotency_conflict`.

## Schedule an event

```ts
await relayforge.publish(
  "trial.ending",
  { account_id: "acc_19" },
  { idempotencyKey: "trial.ending:acc_19", deliverAt: new Date(Date.now() + 86_400_000) },
);
```

`deliverAt` must be in the future and within 30 days. If you retry after a restart, keep both the key and the time the same.

## Options

```ts
new Publisher(url, key, {
  retries: 4,         // 0–10, default 2
  fetch: customFetch, // default: global fetch
});

await relayforge.publish(type, data, {
  idempotencyKey,
  signal: AbortSignal.timeout(15_000), // also cancels retry waits
});
```

Retries cover network errors and HTTP 408, 425, 429, and 5xx. The backoff starts at 200 ms, doubles each time, and caps at 2 s. A `Retry-After` header of up to 30 seconds is honored. Other 4xx responses fail immediately.

## Errors

```ts
import { RelayForgeError } from "@finzyphinzy/relayforge";

try {
  await relayforge.publish(type, data, { idempotencyKey });
} catch (error) {
  if (error instanceof RelayForgeError) {
    console.error(error.status, error.code, error.message);
  } else {
    throw error; // network failure after every retry
  }
}
```

| Status | `code` | Meaning |
| --- | --- | --- |
| 401 | `unauthorized` | Missing, revoked, or expired API key |
| 403 | `forbidden` | Key lacks `events:write` |
| 409 | `idempotency_conflict` | Key already used for a different event |
| 413 | `payload_too_large` | Data exceeds 256 KiB |
| 422 | `invalid_event` / `invalid_schedule` | Bad type or data, or `deliverAt` out of range |
| 429 | `rate_limited` / `quota_exceeded` | Request rate or monthly event quota exceeded |
| 503 | `storage_unavailable` | The API could not commit the event |

## Receiving webhooks

See the receiver guide at `/docs#receive` on the RelayForge site. In short: verify `webhook-signature` over the raw body, reject stale `webhook-timestamp` values, and deduplicate by `webhook-id`.

## Development

```sh
npm ci
npm test   # builds, then runs test/client.test.mjs
```

## Releasing

1. Bump `version` in `package.json` (and `package-lock.json`) and merge to `main`.
2. Tag the commit `sdk/typescript/v<version>` and push the tag, for example `git tag sdk/typescript/v0.1.0 && git push origin sdk/typescript/v0.1.0`.
3. The `Release SDK` workflow checks the tag matches `package.json`, runs the tests, and publishes to npm with provenance. It needs an `NPM_TOKEN` repository secret holding an npm automation (or granular publish) token for the `@finzyphinzy` scope.

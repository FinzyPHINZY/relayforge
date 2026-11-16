import { test } from "node:test";
import assert from "node:assert/strict";
import { buildDeliveryTimeline } from "./delivery-timeline.ts";

const stamp = (seconds) => new Date(Date.UTC(2026, 10, 16, 12, 0, seconds)).toISOString();
const base = (status) => ({ delivery: { id: "delivery-1", event_id: "event-1", endpoint_id: "endpoint-1", run_number: 1, status, attempt_count: 1, max_attempts: 3, created_at: stamp(2), updated_at: stamp(6), completed_at: stamp(6), next_attempt_at: stamp(5) }, attempts: [], review_history: [], event_accepted_at: stamp(0), outbox_dispatched_at: stamp(1) });

test("timeline includes acceptance, dispatch, attempts, retry schedule, and terminal outcome", () => {
  const detail = base("succeeded");
  detail.attempts = [{ id: "a1", attempt_number: 1, started_at: stamp(3), finished_at: stamp(4), outcome: "http_error", http_status: 429, retry_scheduled_at: stamp(5) }, { id: "a2", attempt_number: 2, started_at: stamp(5), finished_at: stamp(6), outcome: "success", http_status: 204 }];
  const timeline = buildDeliveryTimeline(detail);
  assert.deepEqual(timeline.map((item) => item.title), ["Event accepted", "Outbox dispatched", "Delivery queued", "Attempt 1 claimed", "Attempt 1: http error", "Retry scheduled", "Attempt 2 claimed", "Attempt 2: success", "Delivery succeeded"]);
  assert.match(timeline.find((item) => item.title === "Attempt 1: http error").detail, /HTTP 429/);
});

test("timeout, lease expiry, exhausted outcome, review, and replay remain distinguishable", () => {
  const detail = base("exhausted");
  detail.delivery.replayed_from_delivery_id = "older-delivery";
  detail.delivery.run_number = 2;
  detail.attempts = [{ id: "a1", attempt_number: 1, started_at: stamp(3), finished_at: stamp(4), outcome: "timeout", retry_scheduled_at: stamp(5) }, { id: "a2", attempt_number: 2, started_at: stamp(5), finished_at: stamp(6), outcome: "unknown", error_code: "lease_expired" }];
  detail.review_history = [{ status: "investigating", created_at: stamp(7) }];
  const timeline = buildDeliveryTimeline(detail);
  assert.ok(timeline.some((item) => item.title === "Replay run 2 created"));
  assert.match(timeline.find((item) => item.title === "Attempt 1: timeout").detail, /timeout/);
  assert.match(timeline.find((item) => item.title === "Attempt 2: unknown").detail, /lease expired/i);
  assert.ok(timeline.some((item) => item.title === "Delivery exhausted"));
  assert.equal(timeline.at(-1).title, "Review investigating");
});

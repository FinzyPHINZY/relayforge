import type { DeliveryDetail } from "./types";

export type TimelineItem = { at: string; title: string; detail: string; kind: "event" | "delivery" | "attempt" | "retry" | "review"; dueAt?: string };

export function buildDeliveryTimeline({ delivery, attempts, review_history, event_accepted_at, outbox_dispatched_at }: DeliveryDetail): TimelineItem[] {
  const items: TimelineItem[] = [{ at: event_accepted_at, title: "Event accepted", detail: "Event and outbox record committed together.", kind: "event" }];
  if (outbox_dispatched_at) items.push({ at: outbox_dispatched_at, title: "Outbox dispatched", detail: "Subscribed endpoint delivery runs were materialized.", kind: "event" });
  items.push({ at: delivery.created_at, title: delivery.replayed_from_delivery_id ? `Replay run ${delivery.run_number} created` : "Delivery queued", detail: delivery.replayed_from_delivery_id ? `New run from ${delivery.replayed_from_delivery_id}. The source run keeps its history.` : "Waiting for an eligible worker claim.", kind: "delivery" });
  for (const attempt of attempts) {
    items.push({ at: attempt.started_at, title: `Attempt ${attempt.attempt_number} claimed`, detail: "Worker reserved the delivery and began the HTTP send.", kind: "attempt" });
    if (attempt.finished_at) {
      let reason = attempt.outcome === "success" ? "Receiver acknowledged the webhook." : attempt.outcome === "timeout" ? "Receiver did not respond before the timeout." : attempt.error_code === "lease_expired" ? "Worker lease expired before success was recorded; the receiver may already have processed the webhook." : attempt.http_status === 429 ? "Receiver returned HTTP 429 (rate limited)." : attempt.http_status ? `Receiver returned HTTP ${attempt.http_status}.` : attempt.error_code ? `Error: ${attempt.error_code}.` : `Outcome: ${attempt.outcome}.`;
      if (attempt.response_excerpt) reason += ` Response: ${attempt.response_excerpt}`;
      items.push({ at: attempt.finished_at, title: `Attempt ${attempt.attempt_number}: ${attempt.outcome.replaceAll("_", " ")}`, detail: reason, kind: "attempt" });
      if (attempt.retry_scheduled_at) items.push({ at: attempt.finished_at, title: "Retry scheduled", detail: "Rate, circuit, or tenant limits can delay the next claim.", kind: "retry", dueAt: attempt.retry_scheduled_at });
    } else {
      items.push({ at: attempt.started_at, title: `Attempt ${attempt.attempt_number} in progress`, detail: "The result has not been recorded yet.", kind: "attempt" });
    }
  }
  if (delivery.status === "pending" || delivery.status === "retry_wait") {
    items.push({ at: delivery.next_attempt_at, title: delivery.status === "retry_wait" ? "Next retry due" : "First attempt due", detail: "A worker may claim this delivery after this time, subject to endpoint and tenant limits.", kind: "retry" });
  }
  if (delivery.status === "succeeded" || delivery.status === "exhausted" || delivery.status === "cancelled") {
    items.push({ at: delivery.completed_at ?? delivery.updated_at, title: `Delivery ${delivery.status}`, detail: delivery.status === "succeeded" ? "Receiver acknowledged this run." : delivery.status === "exhausted" ? "No further automatic attempts; review or manually replay this run." : "Delivery was cancelled.", kind: "delivery" });
  }
  for (const review of review_history) items.push({ at: review.created_at, title: `Review ${review.status}`, detail: "Operator review recorded; delivery outcome is unchanged.", kind: "review" });
  return items.sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime());
}

"use client";

import { useState } from "react";
import { ArrowRight, Check, Clock3, Database, RotateCcw, ShieldCheck, Webhook, X } from "lucide-react";

const steps = [
  { title: "Publish", phase: "Event submitted", detail: "The publisher sends order.created with an idempotency key. Repeating the same request returns the original event ID.", request: "POST /v1/events", response: "Idempotency-Key: order-1042", icon: Database },
  { title: "Accept", phase: "Durably accepted", detail: "RelayForge commits the event and outbox record in one PostgreSQL transaction before replying 202 Accepted.", request: "202 Accepted", response: "event_id: evt_example_1042", icon: Check },
  { title: "Attempt 1", phase: "Receiver unavailable", detail: "The first signed POST reaches the endpoint, but its HTTP 503 response is retryable. RelayForge records the attempt.", request: "POST /webhooks → HTTP 503", response: "webhook-id: del_example_1042", icon: X },
  { title: "Retry", phase: "Waiting to retry", detail: "A bounded backoff schedules the next attempt. The durable delivery remains visible while the receiver recovers.", request: "status: retry_wait", response: "attempts recorded: 1", icon: Clock3 },
  { title: "Attempt 2", phase: "Delivery acknowledged", detail: "The retry uses the same webhook-id and a fresh HMAC signature. The receiver returns HTTP 204, acknowledging the attempt.", request: "POST /webhooks → HTTP 204", response: "webhook-id: del_example_1042", icon: ShieldCheck },
  { title: "Inspect", phase: "Full history available", detail: "The console shows both attempts, their status codes, and the final delivery state. If every attempt had failed, an operator could review and replay it.", request: "status: succeeded", response: "attempts: 503 → 204", icon: RotateCcw },
] as const;

export function GuidedDemo() {
  const [active, setActive] = useState(0);
  const step = steps[active];
  const Icon = step.icon;
  return <div className="demo-shell">
    <div className="demo-shell-head"><div><span className="demo-live-dot" /> Illustrative run</div><span>ORDER #1042 <Webhook size={15} aria-hidden="true" /></span></div>
    <div className="demo-body">
      <nav className="demo-timeline" aria-label="Demo steps">{steps.map((item, index) => <button key={item.title} type="button" onClick={() => setActive(index)} aria-current={index === active ? "step" : undefined} className={`demo-timeline-step ${index === active ? "is-active" : ""} ${index < active ? "is-complete" : ""}`}><span className="demo-timeline-number">{index < active ? <Check size={13} aria-hidden="true" /> : String(index + 1).padStart(2, "0")}</span><span><strong>{item.title}</strong><small>{item.phase}</small></span></button>)}</nav>
      <div className="demo-stage" aria-live="polite">
        <div className="demo-stage-meta"><span>STEP {String(active + 1).padStart(2, "0")} / {String(steps.length).padStart(2, "0")}</span><span>{active === 2 ? "RETRYABLE FAILURE" : active === 4 || active === 5 ? "DELIVERED" : "IN PROGRESS"}</span></div>
        <div className={`demo-stage-icon ${active === 2 ? "is-failed" : active >= 4 ? "is-success" : ""}`}><Icon size={30} strokeWidth={1.6} aria-hidden="true" /></div>
        <h2>{step.phase}</h2><p>{step.detail}</p>
        <div className="demo-terminal"><div><span /><span /><span /><small>relayforge / delivery trace</small></div><code><span>$</span> {step.request}</code><code><span>↳</span> {step.response}</code></div>
        <div className="demo-stage-footer"><p>{active === 2 || active === 4 ? "The same delivery ID lets receivers deduplicate retries." : "This walkthrough uses example IDs and performs no writes."}</p><div><button type="button" className="demo-reset" onClick={() => setActive(0)}>Restart</button><button type="button" className="demo-next" onClick={() => setActive((current) => Math.min(current + 1, steps.length - 1))} disabled={active === steps.length - 1}>{active === steps.length - 1 ? "Complete" : "Next step"} <ArrowRight size={15} aria-hidden="true" /></button></div></div>
      </div>
    </div>
    <div className="demo-footnote"><ShieldCheck size={16} aria-hidden="true" /><span>Webhook requests are HMAC signed. HTTP delivery is at least once; receivers should deduplicate by <code>webhook-id</code>.</span></div>
  </div>;
}

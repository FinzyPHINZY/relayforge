"use client";

import { useActionState } from "react";
import { rotateEndpointControl, updateEndpointControls, updateEndpointDeliveryPolicy, verifyEndpointControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { dateTime } from "@/lib/format";
import type { Endpoint } from "@/lib/types";

const initial: ControlState = {};
const inputClass = "w-full rounded-lg border border-white/15 bg-slate-950/60 px-3 py-2 text-sm text-white focus:border-emerald-400 focus:outline-none";

function Feedback({ state }: { state: ControlState }) {
  return <>{state.error ? <p role="alert" className="mt-3 text-sm text-rose-300">{state.error}</p> : null}{state.success ? <p role="status" className="mt-3 text-sm text-emerald-300">{state.success}</p> : null}</>;
}

export function EndpointControls({ endpoint, activeOverlapUntil }: { endpoint: Endpoint; activeOverlapUntil?: string }) {
  const [editState, editAction, editing] = useActionState(updateEndpointControls, initial);
  const [policyState, policyAction, savingPolicy] = useActionState(updateEndpointDeliveryPolicy, initial);
  const [verifyState, verifyAction, verifying] = useActionState(verifyEndpointControl, initial);
  const [rotateState, rotateAction, rotating] = useActionState(rotateEndpointControl, initial);
  const hasAlerts = endpoint.event_types.includes("relayforge.delivery.exhausted");
  return <div className="mt-6 grid gap-6 xl:grid-cols-2">
    <Card><CardHeader><h2 className="font-semibold">Delivery policy</h2><p className="mt-1 text-xs leading-5 text-slate-500">Rate and concurrency are shared across workers. New delivery runs snapshot the timeout and maximum attempts.</p></CardHeader><CardContent>
      <form action={policyAction} className="space-y-4"><input type="hidden" name="id" value={endpoint.id} />
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="text-sm text-slate-300">Receiver timeout (ms)<input name="timeoutMs" type="number" min={1000} max={30000} defaultValue={endpoint.timeout_ms} required className={`mt-2 ${inputClass}`} /><span className="mt-1 block text-xs text-slate-500">1,000–30,000 ms per HTTP attempt.</span></label>
          <label className="text-sm text-slate-300">Maximum attempts<input name="maxAttempts" type="number" min={1} max={8} defaultValue={endpoint.max_attempts} required className={`mt-2 ${inputClass}`} /><span className="mt-1 block text-xs text-slate-500">Includes the first send.</span></label>
          <label className="text-sm text-slate-300">Concurrent attempts<input name="concurrencyLimit" type="number" min={1} max={100} defaultValue={endpoint.concurrency_limit} required className={`mt-2 ${inputClass}`} /><span className="mt-1 block text-xs text-slate-500">An operator safety ceiling can further limit this.</span></label>
          <label className="text-sm text-slate-300">Delivery rate per minute<input name="rateLimitPerMinute" type="number" min={0} max={6000} defaultValue={endpoint.rate_limit_per_minute} required className={`mt-2 ${inputClass}`} /><span className="mt-1 block text-xs text-slate-500">0 means unlimited; attempts are evenly paced.</span></label>
        </div>
        <p className="text-xs leading-5 text-slate-500">Retryable failures wait at least 30 seconds, then use exponential backoff with up to 20% jitter. Receiver <code>Retry-After</code> can extend a wait. Non-retryable responses end the run; existing runs keep their original timeout and attempt limit.</p>
        {endpoint.rate_limit_per_minute > 0 && endpoint.next_rate_at ? <p className="text-xs text-slate-400">Next rate slot: {dateTime(endpoint.next_rate_at)}</p> : null}
        <Button disabled={savingPolicy}>{savingPolicy ? "Saving…" : "Save delivery policy"}</Button><Feedback state={policyState} />
      </form>
    </CardContent></Card>
    <Card className="xl:row-span-2"><CardHeader><h2 className="font-semibold">Subscriptions and transformation</h2><p className="mt-1 text-xs leading-5 text-slate-500">Field mappings select or rename top-level event data fields. Existing deliveries keep their original snapshot.</p></CardHeader><CardContent>
      <form action={editAction} className="space-y-5"><input type="hidden" name="id" value={endpoint.id} />
        <div><label htmlFor="editEventTypes" className="mb-2 block text-sm text-slate-300">Event types</label><textarea id="editEventTypes" name="eventTypes" rows={3} defaultValue={endpoint.event_types.filter((type) => type !== "relayforge.delivery.exhausted").join(", ")} className={inputClass} /></div>
        <label className="flex items-start gap-3 text-sm text-slate-300"><input type="checkbox" name="exhaustionAlerts" defaultChecked={hasAlerts} className="mt-1 accent-emerald-400" /><span>Receive signed exhaustion alerts<small className="mt-1 block text-xs text-slate-500">Adds relayforge.delivery.exhausted to this endpoint’s subscriptions.</small></span></label>
        <div><label htmlFor="editFieldMap" className="mb-2 block text-sm text-slate-300">Field mapping JSON</label><textarea id="editFieldMap" name="fieldMap" rows={5} maxLength={4096} defaultValue={Object.keys(endpoint.field_map ?? {}).length ? JSON.stringify(endpoint.field_map, null, 2) : ""} placeholder={'{"order_id":"id"}'} className={`${inputClass} font-mono`} /><p className="mt-1 text-xs text-slate-500">Destination field → source field. Leave empty to send original data.</p></div>
        <Button disabled={editing}>{editing ? "Saving…" : "Save controls"}</Button><Feedback state={editState} />
      </form>
    </CardContent></Card>
    <Card><CardHeader><h2 className="font-semibold">Receiver verification</h2></CardHeader><CardContent><p className="text-sm leading-6 text-slate-400">{endpoint.verification_required ? endpoint.verified_at ? "This receiver has passed the challenge." : "Delivery waits until the receiver echoes a challenge." : "Verification was not required when this endpoint was created."}</p>{endpoint.verification_required && !endpoint.verified_at ? <form action={verifyAction} className="mt-4"><input type="hidden" name="id" value={endpoint.id} /><Button variant="outline" disabled={verifying}>{verifying ? "Verifying…" : "Verify receiver"}</Button><Feedback state={verifyState} /></form> : null}</CardContent></Card>
    <Card><CardHeader><h2 className="font-semibold">Signing secret</h2><p className="mt-1 text-xs text-slate-500">A new secret appears once. Configure the receiver before the one-hour overlap ends.</p></CardHeader><CardContent>
      {activeOverlapUntil ? <p className="mb-4 text-sm text-amber-200">Previous secret valid until {dateTime(activeOverlapUntil)}. Accept either secret during this period, then remove the retired one from your receiver.</p> : <p className="mb-4 text-xs text-slate-500">No previous secret is currently in its overlap window.</p>}
      <form action={rotateAction}><input type="hidden" name="id" value={endpoint.id} /><Button variant="outline" disabled={rotating}>{rotating ? "Rotating…" : "Rotate secret"}</Button><Feedback state={rotateState} />{rotateState.secret ? <div className="mt-4 rounded-lg border border-amber-400/30 bg-amber-400/10 p-3"><p className="text-xs font-semibold text-amber-200">Copy this new signing secret now</p><code className="mt-2 block break-all text-sm text-white">{rotateState.secret}</code></div> : null}</form>
      <ul className="mt-5 divide-y divide-white/10 text-xs text-slate-400">{endpoint.signing_secrets?.map((secret) => <li key={secret.id} className="flex items-center justify-between gap-2 py-2"><span className="capitalize">{secret.state} · {dateTime(secret.created_at)}</span><span className="font-mono text-slate-500">…{secret.id.slice(-8)}</span></li>)}</ul>
    </CardContent></Card>
  </div>;
}

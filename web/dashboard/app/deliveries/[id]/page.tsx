import Link from "next/link";
import { notFound } from "next/navigation";
import { replayDelivery, reviewDelivery } from "@/app/actions";
import { PageTitle, Shell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { ApiError, readApi, requireOrgSession } from "@/lib/api";
import { uuidPattern } from "@/lib/session-value";
import { buildDeliveryTimeline } from "@/lib/delivery-timeline";
import { dateTime } from "@/lib/format";
import type { DeliveryDetail, Principal } from "@/lib/types";

export default async function DeliveryPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ error?: string }> }) {
  const session = await requireOrgSession();
  const { id } = await params;
  if (!uuidPattern.test(id)) notFound();
  const { error: actionError } = await searchParams;
  let result: DeliveryDetail;
  try { result = await readApi<DeliveryDetail>(session, `/v1/deliveries/${encodeURIComponent(id)}`); }
  catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  const me = await readApi<Principal>(session, "/v1/me");
  const { delivery, attempts, review_history: reviewHistory } = result;
  const timeline = buildDeliveryTimeline(result);
  const canWrite = me.scopes.includes("deliveries:write");
  const waiting = delivery.status === "pending" || delivery.status === "retry_wait";
  return <Shell>
    <PageTitle eyebrow="Delivery inspector" title="Full delivery timeline" description={`Run ${delivery.run_number} · Created ${dateTime(delivery.created_at)}`} action={<StatusBadge status={delivery.status} />} />
    {actionError ? <p role="alert" className="mb-5 rounded-lg border border-rose-400/20 bg-rose-400/10 p-3 text-sm text-rose-200">{actionError === "review" ? "Could not save the review. Check the note and try again." : "Replay failed. Check that the endpoint is active and try again."}</p> : null}
    <div className="grid gap-5 xl:grid-cols-3">
      <Card className="xl:col-span-2"><CardHeader><h2 className="font-semibold">Timeline</h2><p className="mt-1 text-xs text-slate-500">Acceptance, outbox dispatch, claims, HTTP results, retry plans, terminal outcome, and reviews. Future due times are plans, not guaranteed send times.</p></CardHeader><CardContent>
        <ol className="relative ml-2 border-l border-white/10">{timeline.map((item, index) => <li key={`${item.at}-${item.title}-${index}`} className="relative pb-6 pl-6 last:pb-0"><span className={`absolute -left-[5px] top-1.5 h-2.5 w-2.5 rounded-full ${item.kind === "retry" ? "bg-amber-300" : item.kind === "attempt" ? "bg-emerald-300" : "bg-slate-400"}`} /><time className="text-xs text-slate-500" dateTime={item.at}>{dateTime(item.at)}</time><h3 className="mt-1 text-sm font-semibold text-slate-100">{item.title}</h3><p className="mt-1 text-xs leading-5 text-slate-400">{item.detail}</p>{item.dueAt ? <p className="mt-1 text-xs text-amber-200">Due {dateTime(item.dueAt)}</p> : null}</li>)}</ol>
      </CardContent></Card>
      <Card><CardHeader><h2 className="font-semibold">Delivery run</h2></CardHeader><CardContent className="space-y-4 text-sm">
        <Detail label="Delivery ID" value={delivery.id} />
        <Detail label="Destination" value={delivery.destination_url} />
        <Detail label="Attempts" value={`${delivery.attempt_count} of ${delivery.max_attempts}`} />
        {waiting ? <Detail label="Next attempt due" value={dateTime(delivery.next_attempt_at)} /> : null}
        <Detail label="Last HTTP status" value={delivery.last_http_status?.toString() ?? "—"} />
        <Detail label="Last error" value={delivery.last_error_code ?? "—"} />
        <div><p className="text-xs text-slate-500">Event</p><Link className="mt-1 block break-all font-mono text-xs text-emerald-300 hover:underline" href={`/events/${delivery.event_id}`}>{delivery.event_id}</Link></div>
        <div><p className="text-xs text-slate-500">Endpoint</p><Link className="mt-1 block break-all font-mono text-xs text-emerald-300 hover:underline" href={`/endpoints/${delivery.endpoint_id}`}>{delivery.endpoint_id}</Link></div>
        {delivery.replayed_from_delivery_id ? <div><p className="text-xs text-slate-500">Replayed from</p><Link className="mt-1 block break-all font-mono text-xs text-emerald-300 hover:underline" href={`/deliveries/${delivery.replayed_from_delivery_id}`}>{delivery.replayed_from_delivery_id}</Link></div> : null}
        {canWrite && (delivery.status === "exhausted" || delivery.status === "cancelled") ? <form action={replayDelivery} className="border-t border-white/10 pt-4"><input type="hidden" name="id" value={delivery.id} /><Button className="w-full">Replay delivery</Button><p className="mt-2 text-xs text-slate-500">Creates a new run and preserves this history.</p></form> : null}
      </CardContent></Card>
    </div>
    <Card className="mt-5"><CardHeader><h2 className="font-semibold">HTTP attempts</h2><p className="mt-1 text-xs text-slate-500">Receiver bodies are shown only when they match a small set of safe diagnostic phrases. Other response bodies are withheld.</p></CardHeader><CardContent>
      {attempts.length ? <ol className="space-y-4">{attempts.map((attempt) => <li key={attempt.id} className="rounded-xl border border-white/10 bg-slate-950/60 p-4"><div className="flex flex-wrap items-center justify-between gap-2"><p className="font-medium">Attempt {attempt.attempt_number}</p><StatusBadge status={attempt.outcome} /></div><div className="mt-3 grid gap-2 text-xs text-slate-400 sm:grid-cols-3"><span>Started {dateTime(attempt.started_at)}</span><span>HTTP {attempt.http_status ?? "—"}</span><span>Duration {attempt.duration_ms == null ? "—" : `${attempt.duration_ms} ms`}</span></div>{attempt.error_code ? <p className="mt-3 font-mono text-xs text-rose-300">{attempt.error_code}</p> : null}{attempt.response_excerpt ? <p className="mt-2 break-words text-xs text-slate-400">Response: {attempt.response_excerpt}</p> : null}{attempt.retry_scheduled_at ? <p className="mt-2 text-xs text-amber-200">Retry scheduled for {dateTime(attempt.retry_scheduled_at)}</p> : null}</li>)}</ol> : <EmptyState title="No attempts yet" description="A worker will claim this delivery when it becomes due." />}
    </CardContent></Card>
    {delivery.status === "exhausted" ? <Card className="mt-5"><CardHeader><h2 className="font-semibold">Dead-letter review</h2></CardHeader><CardContent className="space-y-5"><p className="text-sm text-slate-400">Review state: <span className="font-medium text-white">{delivery.review_status ?? "unreviewed"}</span></p>{canWrite ? <form action={reviewDelivery} className="space-y-3"><input type="hidden" name="id" value={delivery.id} /><label htmlFor="review-note" className="block text-sm text-slate-300">Review note</label><textarea id="review-note" name="note" maxLength={500} rows={3} className="w-full rounded-lg border border-white/15 bg-slate-950/60 p-3 text-sm text-white focus:border-emerald-400 focus:outline-none" placeholder="What happened and what should happen next?" /><div className="flex flex-wrap gap-2"><Button name="status" value="investigating">Mark investigating</Button><Button name="status" value="resolved" variant="outline">Resolve with note</Button></div><p className="text-xs text-slate-500">Resolutions require a note. Every change is kept in the review history.</p></form> : null}{reviewHistory.length ? <div className="border-t border-white/10 pt-4"><h3 className="mb-3 text-sm font-medium">Review history</h3><ol className="space-y-3">{reviewHistory.map((entry, index) => <li key={`${entry.created_at}-${index}`} className="rounded-lg border border-white/10 p-3 text-sm"><div className="flex items-center justify-between gap-3"><StatusBadge status={entry.status} /><time className="text-xs text-slate-500" dateTime={entry.created_at}>{dateTime(entry.created_at)}</time></div>{entry.note ? <p className="mt-2 whitespace-pre-wrap text-slate-300">{entry.note}</p> : null}</li>)}</ol></div> : null}</CardContent></Card> : null}
  </Shell>;
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs text-slate-500">{label}</p><p className="mt-1 break-all text-slate-200">{value}</p></div>;
}

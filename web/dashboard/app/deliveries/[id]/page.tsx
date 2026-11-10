import Link from "next/link";
import { notFound } from "next/navigation";
import { replayDelivery } from "@/app/actions";
import { PageTitle, Shell, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { ApiError, readApi, requireKey } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { Attempt, Delivery } from "@/lib/types";

export default async function DeliveryPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ error?: string }> }) {
  const key = await requireKey();
  const { id } = await params;
  const { error: actionError } = await searchParams;
  let result: { delivery: Delivery; attempts: Attempt[] };
  try { result = await readApi(key, `/v1/deliveries/${encodeURIComponent(id)}`); }
  catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  const { delivery, attempts } = result;
  return <Shell><PageTitle eyebrow="Delivery details" title="Attempt history" description={`Run ${delivery.run_number} · Created ${dateTime(delivery.created_at)}`} action={<StatusBadge status={delivery.status} />} />
    {actionError ? <p role="alert" className="mb-5 rounded-lg border border-rose-400/20 bg-rose-400/10 p-3 text-sm text-rose-200">Replay failed. Check that the endpoint is active and try again.</p> : null}
    <div className="grid gap-5 lg:grid-cols-3"><Card className="lg:col-span-2"><CardHeader><h2 className="font-semibold">Attempts</h2></CardHeader><CardContent>{attempts.length ? <ol className="space-y-4">{attempts.map((attempt) => <li key={attempt.id} className="rounded-xl border border-white/10 bg-slate-950/60 p-4"><div className="flex flex-wrap items-center justify-between gap-2"><p className="font-medium">Attempt {attempt.attempt_number}</p><StatusBadge status={attempt.outcome} /></div><div className="mt-3 grid gap-2 text-xs text-slate-400 sm:grid-cols-3"><span>Started {dateTime(attempt.started_at)}</span><span>HTTP {attempt.http_status ?? "—"}</span><span>Duration {attempt.duration_ms == null ? "—" : `${attempt.duration_ms} ms`}</span></div>{attempt.error_code ? <p className="mt-3 font-mono text-xs text-rose-300">{attempt.error_code}</p> : null}</li>)}</ol> : <EmptyState title="No attempts yet" description="A worker will claim this delivery when it becomes due." />}</CardContent></Card>
      <Card><CardHeader><h2 className="font-semibold">Delivery run</h2></CardHeader><CardContent className="space-y-4 text-sm"><Detail label="Delivery ID" value={delivery.id} /><Detail label="Destination" value={delivery.destination_url} /><Detail label="Attempts" value={`${delivery.attempt_count} of ${delivery.max_attempts}`} /><Detail label="Last HTTP status" value={delivery.last_http_status?.toString() ?? "—"} /><Detail label="Last error" value={delivery.last_error_code ?? "—"} /><div><p className="text-xs text-slate-500">Event</p><Link className="mt-1 block break-all font-mono text-xs text-emerald-300 hover:underline" href={`/events/${delivery.event_id}`}>{delivery.event_id}</Link></div>{delivery.replayed_from_delivery_id ? <div><p className="text-xs text-slate-500">Replayed from</p><Link className="mt-1 block break-all font-mono text-xs text-emerald-300 hover:underline" href={`/deliveries/${delivery.replayed_from_delivery_id}`}>{delivery.replayed_from_delivery_id}</Link></div> : null}{delivery.status === "exhausted" || delivery.status === "cancelled" ? <form action={replayDelivery} className="border-t border-white/10 pt-4"><input type="hidden" name="id" value={delivery.id} /><Button className="w-full">Replay delivery</Button><p className="mt-2 text-xs text-slate-500">Creates a new run and preserves this history.</p></form> : null}</CardContent></Card></div>
  </Shell>;
}

function Detail({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs text-slate-500">{label}</p><p className="mt-1 break-all text-slate-200">{value}</p></div>;
}

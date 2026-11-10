import Link from "next/link";
import { ArrowRight, Clock3, DatabaseZap, RadioTower, Workflow } from "lucide-react";
import { Shell, PageTitle, EmptyState } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Delivery, Endpoint, Event, Metrics, Page } from "@/lib/types";

export default async function OverviewPage() {
  const key = await requireKey();
  const [metrics, deliveries, events, endpoints] = await Promise.all([
    readApi<Metrics>(key, "/v1/metrics/summary"),
    readApi<Page<Delivery>>(key, "/v1/deliveries?limit=5"),
    readApi<Page<Event>>(key, "/v1/events?limit=5"),
    readApi<Page<Endpoint>>(key, "/v1/endpoints?limit=5"),
  ]);
  const cards = [
    { label: "Accepted events", value: metrics.events_accepted, icon: DatabaseZap, detail: "Durably stored" },
    { label: "Successful deliveries", value: metrics.deliveries_succeeded, icon: Workflow, detail: "Acknowledged by receivers" },
    { label: "Waiting to deliver", value: metrics.deliveries_pending + metrics.deliveries_retry_wait, icon: Clock3, detail: `${metrics.deliveries_retry_wait} waiting to retry` },
    { label: "Exhausted", value: metrics.deliveries_exhausted, icon: RadioTower, detail: "Eligible for review or replay" },
  ];
  return <Shell><PageTitle eyebrow="Overview" title="Delivery at a glance" description="Durable state from PostgreSQL, scoped to your API key." />
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{cards.map(({ label, value, icon: Icon, detail }) => <Card key={label}><CardContent><div className="flex items-center justify-between text-sm text-slate-400"><span>{label}</span><Icon size={18} className="text-emerald-400" /></div><p className="mt-5 text-3xl font-semibold tabular-nums text-white">{value.toLocaleString()}</p><p className="mt-2 text-xs text-slate-500">{detail}</p></CardContent></Card>)}</div>
    <div className="mt-5 grid gap-4 sm:grid-cols-3"><MiniStat label="Outbox pending" value={metrics.outbox_pending} /><MiniStat label="In progress" value={metrics.deliveries_processing} /><MiniStat label="Oldest pending" value={metrics.oldest_pending_age_seconds > 0 ? `${Math.round(metrics.oldest_pending_age_seconds)}s` : "—"} /></div>
    <div className="mt-8 grid gap-6 xl:grid-cols-5"><Card className="xl:col-span-3"><CardHeader><div className="flex items-center justify-between"><h2 className="font-semibold">Recent deliveries</h2><Link className="flex items-center gap-1 text-xs text-emerald-300 hover:text-emerald-200" href="/deliveries">View all <ArrowRight size={14} /></Link></div></CardHeader><CardContent className="space-y-1">{deliveries.data.length ? deliveries.data.map((delivery) => <Link key={delivery.id} href={`/deliveries/${delivery.id}`} className="flex items-center justify-between gap-3 rounded-lg px-2 py-3 hover:bg-white/5"><div className="min-w-0"><p className="font-mono text-sm text-slate-200">{shortId(delivery.id)}</p><p className="mt-1 truncate text-xs text-slate-500">{delivery.destination_url}</p></div><StatusBadge status={delivery.status} /></Link>) : <EmptyState title="No deliveries yet" description="Publish an event to a subscribed endpoint." />}</CardContent></Card>
      <Card className="xl:col-span-2"><CardHeader><div className="flex items-center justify-between"><h2 className="font-semibold">Recent events</h2><Link className="flex items-center gap-1 text-xs text-emerald-300 hover:text-emerald-200" href="/events">View all <ArrowRight size={14} /></Link></div></CardHeader><CardContent className="space-y-1">{events.data.length ? events.data.map((event) => <Link key={event.id} href={`/events/${event.id}`} className="block rounded-lg px-2 py-3 hover:bg-white/5"><p className="text-sm font-medium text-slate-200">{event.type}</p><p className="mt-1 text-xs text-slate-500">{dateTime(event.created_at)}</p></Link>) : <EmptyState title="No events yet" description="Accepted events will appear here." />}</CardContent></Card></div>
    <p className="mt-7 text-xs text-slate-500">{endpoints.data.length} endpoint{endpoints.data.length === 1 ? "" : "s"} in this view · {metrics.delivery_attempts} recorded attempts · {metrics.lease_reclaims} lease reclaims</p>
  </Shell>;
}

function MiniStat({ label, value }: { label: string; value: number | string }) {
  return <div className="rounded-xl border border-white/10 bg-slate-900/40 px-4 py-3"><p className="text-xs text-slate-500">{label}</p><p className="mt-1 text-lg font-medium tabular-nums text-slate-200">{value}</p></div>;
}

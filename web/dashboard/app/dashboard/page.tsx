import Link from "next/link";
import type { Metadata } from "next";
import { ArrowRight, DatabaseZap, RadioTower, RotateCcw, Workflow } from "lucide-react";
import { Shell, PageTitle, EmptyState } from "@/components/shell";
import { StarterChecklist } from "@/components/starter-checklist";
import { OverviewCharts } from "@/components/overview-charts";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Delivery, Endpoint, Event, Metrics, OverviewAnalytics, OverviewWindow, Page, Principal } from "@/lib/types";

export const metadata: Metadata = { title: "Overview | RelayForge Console" };

export default async function OverviewPage({ searchParams }: { searchParams: Promise<{ window?: string }> }) {
  const key = await requireKey();
  const requested = (await searchParams).window;
  const window: OverviewWindow = requested === "7d" || requested === "30d" ? requested : "24h";
  const [metrics, overview, deliveries, events, endpoints, me] = await Promise.all([
    readApi<Metrics>(key, "/v1/metrics/summary"),
    readApi<OverviewAnalytics>(key, `/v1/analytics/overview?window=${window}`),
    readApi<Page<Delivery>>(key, "/v1/deliveries?limit=5"),
    readApi<Page<Event>>(key, "/v1/events?limit=5"),
    readApi<Page<Endpoint>>(key, "/v1/endpoints?limit=5"),
    readApi<Principal>(key, "/v1/me"),
  ]);
  const finalized = overview.totals.succeeded + overview.totals.exhausted;
  const successRate = finalized ? `${(overview.totals.succeeded / finalized * 100).toFixed(1)}%` : "—";
  const cards = [
    { label: "Delivery success rate", value: successRate, icon: Workflow, detail: finalized ? `${finalized.toLocaleString()} finalized: succeeded ÷ (succeeded + exhausted)` : "No finalized deliveries in this window" },
    { label: "Accepted events", value: overview.totals.events, icon: DatabaseZap, detail: "Durably stored in this window" },
    { label: "Failed deliveries", value: overview.totals.exhausted, icon: RadioTower, detail: "Exhausted in this window" },
    { label: "Retries", value: overview.totals.retries, icon: RotateCcw, detail: "Attempts numbered two or higher" },
  ];
  return <Shell><PageTitle eyebrow="Overview" title="Delivery at a glance" description="Tenant-scoped activity from PostgreSQL. Window totals use UTC buckets; current backlog is shown separately." />
    <StarterChecklist hasEndpoint={endpoints.data.length > 0} hasEvent={metrics.events_accepted > 0} hasAttempt={metrics.delivery_attempts > 0} canCreateEndpoint={me.scopes.includes("endpoints:write")} canPublishEvent={me.scopes.includes("events:write")} />
    <div className="mb-5 flex flex-wrap items-center gap-2" aria-label="Overview time window">{(["24h", "7d", "30d"] as const).map((option) => <Link key={option} href={`/dashboard?window=${option}`} aria-current={option === window ? "page" : undefined} className={`rounded-lg px-4 py-2 text-sm font-medium ${option === window ? "bg-emerald-400 text-slate-950" : "border border-white/10 text-slate-400 hover:text-white"}`}>{option}</Link>)}<span className="ml-auto text-xs text-slate-500">{window === "24h" ? "Current hour + previous 23 UTC hours" : `Today + previous ${window === "7d" ? 6 : 29} UTC days`}</span></div>
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{cards.map(({ label, value, icon: Icon, detail }) => <Card key={label}><CardContent><div className="flex items-center justify-between text-sm text-slate-400"><span>{label}</span><Icon size={18} className="text-emerald-400" /></div><p className="mt-5 text-3xl font-semibold tabular-nums text-white">{typeof value === "number" ? value.toLocaleString() : value}</p><p className="mt-2 text-xs text-slate-500">{detail}</p></CardContent></Card>)}</div>
    <div className="mt-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-4"><MiniStat label="Succeeded in window" value={overview.totals.succeeded} /><MiniStat label="Failed attempts in window" value={overview.totals.failed_attempts} /><MiniStat label="Waiting to deliver now" value={metrics.deliveries_pending + metrics.deliveries_retry_wait} /><MiniStat label="Oldest pending now" value={metrics.oldest_pending_age_seconds > 0 ? `${Math.round(metrics.oldest_pending_age_seconds)}s` : "—"} /></div>
    <p className="mt-3 text-xs text-slate-500">Failed attempts include completed non-success sends; a later retry may still succeed. Cancelled deliveries are excluded from the success rate ({overview.totals.cancelled} in this window).</p>
    <OverviewCharts data={overview} />
    <div className="mt-8 grid gap-6 xl:grid-cols-5"><Card className="xl:col-span-3"><CardHeader><div className="flex items-center justify-between"><h2 className="font-semibold">Recent deliveries</h2><Link className="flex items-center gap-1 text-xs text-emerald-300 hover:text-emerald-200" href="/deliveries">View all <ArrowRight size={14} /></Link></div></CardHeader><CardContent className="space-y-1">{deliveries.data.length ? deliveries.data.map((delivery) => <Link key={delivery.id} href={`/deliveries/${delivery.id}`} className="flex items-center justify-between gap-3 rounded-lg px-2 py-3 hover:bg-white/5"><div className="min-w-0"><p className="font-mono text-sm text-slate-200">{shortId(delivery.id)}</p><p className="mt-1 truncate text-xs text-slate-500">{delivery.destination_url}</p></div><StatusBadge status={delivery.status} /></Link>) : <EmptyState title="No deliveries yet" description="Publish an event to a subscribed endpoint." />}</CardContent></Card>
      <Card className="xl:col-span-2"><CardHeader><div className="flex items-center justify-between"><h2 className="font-semibold">Recent events</h2><Link className="flex items-center gap-1 text-xs text-emerald-300 hover:text-emerald-200" href="/events">View all <ArrowRight size={14} /></Link></div></CardHeader><CardContent className="space-y-1">{events.data.length ? events.data.map((event) => <Link key={event.id} href={`/events/${event.id}`} className="block rounded-lg px-2 py-3 hover:bg-white/5"><p className="text-sm font-medium text-slate-200">{event.type}</p><p className="mt-1 text-xs text-slate-500">{dateTime(event.created_at)}</p></Link>) : <EmptyState title="No events yet" description="Accepted events will appear here." />}</CardContent></Card></div>
    <p className="mt-7 text-xs text-slate-500">{endpoints.data.length} endpoint{endpoints.data.length === 1 ? "" : "s"} in this view · {metrics.outbox_pending} outbox pending · {metrics.deliveries_processing} in progress · {metrics.lease_reclaims} lifetime lease reclaims</p>
  </Shell>;
}

function MiniStat({ label, value }: { label: string; value: number | string }) {
  return <div className="rounded-xl border border-white/10 bg-slate-900/40 px-4 py-3"><p className="text-xs text-slate-500">{label}</p><p className="mt-1 text-lg font-medium tabular-nums text-slate-200">{value}</p></div>;
}

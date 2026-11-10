import Link from "next/link";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Delivery, Page } from "@/lib/types";

const statuses = ["all", "pending", "processing", "retry_wait", "succeeded", "exhausted", "cancelled"];

export default async function DeliveriesPage({ searchParams }: { searchParams: Promise<{ status?: string; event_id?: string; endpoint_id?: string; cursor?: string }> }) {
  const key = await requireKey();
  const query = await searchParams;
  const status = statuses.includes(query.status ?? "all") ? query.status ?? "all" : "all";
  const params = new URLSearchParams({ limit: "50" });
  if (status !== "all") params.set("status", status);
  if (query.event_id) params.set("event_id", query.event_id);
  if (query.endpoint_id) params.set("endpoint_id", query.endpoint_id);
  if (query.cursor) params.set("cursor", query.cursor);
  const page = await readApi<Page<Delivery>>(key, `/v1/deliveries?${params}`);
  const filterQuery = new URLSearchParams();
  if (query.event_id) filterQuery.set("event_id", query.event_id);
  if (query.endpoint_id) filterQuery.set("endpoint_id", query.endpoint_id);
  return <Shell><PageTitle eyebrow="Delivery operations" title="Deliveries" description="Inspect each run, its attempts, and its terminal outcome." />
    <div className="mb-5 flex flex-wrap gap-2" aria-label="Filter by status">{statuses.map((item) => { const p = new URLSearchParams(filterQuery); if (item !== "all") p.set("status", item); return <Link key={item} href={`/deliveries${p.size ? `?${p}` : ""}`} className={`rounded-full px-3 py-1.5 text-xs capitalize ${status === item ? "bg-emerald-400 text-slate-950" : "border border-white/10 text-slate-400 hover:text-white"}`}>{item.replaceAll("_", " ")}</Link>; })}</div>
    <Card><CardHeader><h2 className="font-semibold">Delivery runs</h2></CardHeader><CardContent>{page.data.length ? <div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="border-b border-white/10 text-xs uppercase tracking-wide text-slate-500"><tr><th className="pb-3 font-medium">Run</th><th className="pb-3 font-medium">Destination</th><th className="pb-3 font-medium">Status</th><th className="pb-3 font-medium">Attempts</th><th className="pb-3 font-medium">Created</th><th className="pb-3 font-medium" /></tr></thead><tbody className="divide-y divide-white/10">{page.data.map((delivery) => <tr key={delivery.id}><td className="py-4 font-mono text-xs text-slate-300">{shortId(delivery.id)} <span className="text-slate-600">#{delivery.run_number}</span></td><td className="max-w-56 truncate py-4 text-slate-300">{delivery.destination_url}</td><td className="py-4"><StatusBadge status={delivery.status} /></td><td className="py-4 tabular-nums text-slate-400">{delivery.attempt_count}/{delivery.max_attempts}</td><td className="py-4 text-slate-400">{dateTime(delivery.created_at)}</td><td className="py-4 text-right"><Link href={`/deliveries/${delivery.id}`} className="text-emerald-300 hover:underline">Inspect →</Link></td></tr>)}</tbody></table></div> : <EmptyState title="No deliveries match" description="Try another status or publish a matching event." />}{page.next_cursor ? <Link className="mt-5 inline-block text-sm text-emerald-300" href={`/deliveries?${new URLSearchParams({ ...Object.fromEntries(filterQuery), ...(status !== "all" ? { status } : {}), cursor: page.next_cursor })}`}>Next page →</Link> : null}</CardContent></Card>
  </Shell>;
}

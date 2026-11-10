import Link from "next/link";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Delivery, Page } from "@/lib/types";

const states = ["unreviewed", "investigating", "resolved"] as const;

export default async function DeadLettersPage({ searchParams }: { searchParams: Promise<{ review_status?: string; cursor?: string }> }) {
  const key = await requireKey();
  const query = await searchParams;
  const state = states.find((item) => item === query.review_status) ?? "unreviewed";
  const params = new URLSearchParams({ status: "exhausted", review_status: state, limit: "50" });
  if (query.cursor) params.set("cursor", query.cursor);
  const page = await readApi<Page<Delivery>>(key, `/v1/deliveries?${params}`);
  return <Shell><PageTitle eyebrow="Recovery" title="Dead letters" description="Review exhausted deliveries, record a resolution, or replay a repaired endpoint." />
    <nav className="mb-5 flex flex-wrap gap-2" aria-label="Review status">{states.map((item) => <Link key={item} href={`/dead-letters?review_status=${item}`} className={`rounded-full px-3 py-1.5 text-xs capitalize ${state === item ? "bg-emerald-400 text-slate-950" : "border border-white/10 text-slate-400 hover:text-white"}`}>{item}</Link>)}</nav>
    <Card><CardHeader><h2 className="font-semibold">Exhausted deliveries</h2></CardHeader><CardContent>{page.data.length ? <div className="overflow-x-auto"><table className="w-full min-w-[680px] text-left text-sm"><thead className="border-b border-white/10 text-xs uppercase text-slate-500"><tr><th className="pb-3 font-medium">Run</th><th className="pb-3 font-medium">Destination</th><th className="pb-3 font-medium">Last error</th><th className="pb-3 font-medium">Completed</th><th className="pb-3 font-medium">Review</th></tr></thead><tbody className="divide-y divide-white/10">{page.data.map((delivery) => <tr key={delivery.id}><td className="py-4 font-mono text-xs text-slate-300">{shortId(delivery.id)}</td><td className="max-w-56 truncate py-4 text-slate-300">{delivery.destination_url}</td><td className="py-4 font-mono text-xs text-rose-300">{delivery.last_error_code ?? delivery.last_http_status ?? "—"}</td><td className="py-4 text-slate-400">{dateTime(delivery.completed_at ?? delivery.created_at)}</td><td className="py-4"><Link href={`/deliveries/${delivery.id}`} className="inline-flex items-center gap-2 text-emerald-300 hover:underline"><StatusBadge status={delivery.review_status ?? "unreviewed"} /> Inspect →</Link></td></tr>)}</tbody></table></div> : <EmptyState title="No deliveries in this review state" description="Choose another state or wait for an exhausted delivery." />}{page.next_cursor ? <Link className="mt-5 inline-block text-sm text-emerald-300" href={`/dead-letters?review_status=${state}&cursor=${encodeURIComponent(page.next_cursor)}`}>Next page →</Link> : null}</CardContent></Card>
  </Shell>;
}

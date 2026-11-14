import Link from "next/link";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { PublishEventForm } from "@/components/publish-event-form";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Event, Page, Principal } from "@/lib/types";

export default async function EventsPage({ searchParams }: { searchParams: Promise<{ cursor?: string }> }) {
  const key = await requireKey();
  const { cursor } = await searchParams;
  const params = new URLSearchParams({ limit: "50" });
  if (cursor) params.set("cursor", cursor);
  const [page, me] = await Promise.all([readApi<Page<Event>>(key, `/v1/events?${params}`), readApi<Principal>(key, "/v1/me")]);
  return <Shell><PageTitle eyebrow="Publisher activity" title="Events" description="Accepted events are immutable and durably stored before delivery starts." />
    {me.scopes.includes("events:write") ? <Card className="mb-6"><CardHeader><h2 className="font-semibold">Publish an event</h2><p className="mt-1 text-xs text-slate-500">Schedule it for later or deliver as soon as the outbox dispatches it. Retries keep the same delivery ID.</p></CardHeader><CardContent><PublishEventForm /></CardContent></Card> : null}
    <Card><CardHeader><h2 className="font-semibold">Event history</h2></CardHeader><CardContent>{page.data.length ? <div className="overflow-x-auto"><table className="w-full min-w-[600px] text-left text-sm"><thead className="border-b border-white/10 text-xs uppercase tracking-wide text-slate-500"><tr><th className="pb-3 font-medium">Event</th><th className="pb-3 font-medium">ID</th><th className="pb-3 font-medium">Accepted</th><th className="pb-3 font-medium" /></tr></thead><tbody className="divide-y divide-white/10">{page.data.map((event) => <tr key={event.id}><td className="py-4 font-medium text-slate-200">{event.type}</td><td className="py-4 font-mono text-xs text-slate-400">{shortId(event.id)}</td><td className="py-4 text-slate-400">{dateTime(event.created_at)}</td><td className="py-4 text-right"><Link className="text-emerald-300 hover:underline" href={`/events/${event.id}`}>Inspect →</Link></td></tr>)}</tbody></table></div> : <EmptyState title="No events yet" description="Accepted events will appear here." />}{page.next_cursor ? <Link className="mt-5 inline-block text-sm text-emerald-300" href={`/events?cursor=${encodeURIComponent(page.next_cursor)}`}>Next page →</Link> : null}</CardContent></Card>
  </Shell>;
}

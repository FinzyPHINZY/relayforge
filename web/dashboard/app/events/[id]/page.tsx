import Link from "next/link";
import { notFound } from "next/navigation";
import { PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { PayloadReveal } from "@/components/payload-reveal";
import { StatusBadge } from "@/components/status-badge";
import { ApiError, readApi, requireOrgSession } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { Delivery, Event, Page, Principal } from "@/lib/types";

export default async function EventPage({ params }: { params: Promise<{ id: string }> }) {
  const session = await requireOrgSession();
  const { id } = await params;
  let event: Event;
  try { event = await readApi<Event>(session, `/v1/events/${encodeURIComponent(id)}`); }
  catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  const me = await readApi<Principal>(session, "/v1/me");
  const canReadDeliveries = me.scopes.includes("deliveries:read");
  const deliveries = canReadDeliveries ? await readApi<Page<Delivery>>(session, `/v1/deliveries?event_id=${encodeURIComponent(id)}&limit=50`) : { data: [], next_cursor: "" };
  const canReveal = me.scopes.includes("events:read") && (me.role === "owner" || me.role === "admin");
  return <Shell><PageTitle eyebrow="Event details" title={event.type} description={`Accepted ${dateTime(event.created_at)}`} action={<Link className="text-sm text-emerald-300 hover:underline" href="/events">← All events</Link>} />
    <Card><CardHeader><h2 className="font-semibold">Event record</h2></CardHeader><CardContent className="space-y-4 text-sm"><div><p className="text-xs text-slate-500">Event ID</p><p className="mt-1 break-all font-mono text-slate-200">{event.id}</p></div><div><p className="text-xs text-slate-500">Type</p><p className="mt-1 text-slate-200">{event.type}</p></div>{canReveal ? <PayloadReveal eventId={event.id} /> : <p className="rounded-lg border border-white/10 bg-slate-950 p-3 text-xs text-slate-400">Payload inspection is available to owners and admins. Event metadata and delivery history remain visible with read access.</p>}</CardContent></Card>
    <Card className="mt-6"><CardHeader><h2 className="font-semibold">Delivery runs</h2><p className="mt-1 text-xs text-slate-500">Each run leads to its full attempt history and HTTP responses. An event may have several endpoint outcomes or manual replay runs.</p></CardHeader><CardContent>{!canReadDeliveries ? <p className="text-sm text-slate-500">This key does not have delivery read access.</p> : deliveries.data.length ? <div className="overflow-x-auto"><table className="w-full min-w-[680px] text-left text-sm"><thead className="border-b border-white/10 text-xs text-slate-500"><tr><th className="pb-3">Delivery</th><th className="pb-3">Status</th><th className="pb-3">Attempts</th><th className="pb-3">Last HTTP</th><th className="pb-3" /></tr></thead><tbody className="divide-y divide-white/10">{deliveries.data.map((delivery) => <tr key={delivery.id}><td className="py-3 font-mono text-xs text-slate-300">{shortId(delivery.id)} <span className="text-slate-500">run #{delivery.run_number}</span></td><td className="py-3"><StatusBadge status={delivery.status} /></td><td className="py-3 text-slate-400">{delivery.attempt_count}/{delivery.max_attempts}</td><td className="py-3 text-slate-400">{delivery.last_http_status ?? "—"}</td><td className="py-3 text-right"><Link href={`/deliveries/${delivery.id}`} className="text-emerald-300 hover:underline">View attempts →</Link></td></tr>)}</tbody></table></div> : <p className="text-sm text-slate-500">No delivery runs yet. The event may be scheduled or have no matching subscription.</p>}{deliveries.next_cursor ? <Link href={`/deliveries?event_id=${encodeURIComponent(id)}`} className="mt-4 inline-block text-sm text-emerald-300">View all delivery runs →</Link> : null}</CardContent></Card>
  </Shell>;
}

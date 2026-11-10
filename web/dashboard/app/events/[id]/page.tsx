import Link from "next/link";
import { notFound } from "next/navigation";
import { PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { ApiError, readApi, requireKey } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { Event } from "@/lib/types";

export default async function EventPage({ params }: { params: Promise<{ id: string }> }) {
  const key = await requireKey();
  const { id } = await params;
  let event: Event;
  try { event = await readApi<Event>(key, `/v1/events/${encodeURIComponent(id)}`); }
  catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  return <Shell><PageTitle eyebrow="Event details" title={event.type} description={`Accepted ${dateTime(event.created_at)}`} action={<Link className="text-sm text-emerald-300 hover:underline" href={`/deliveries?event_id=${event.id}`}>View deliveries →</Link>} />
    <Card><CardHeader><h2 className="font-semibold">Event record</h2></CardHeader><CardContent className="space-y-4 text-sm"><div><p className="text-xs text-slate-500">Event ID</p><p className="mt-1 break-all font-mono text-slate-200">{event.id}</p></div><div><p className="text-xs text-slate-500">Type</p><p className="mt-1 text-slate-200">{event.type}</p></div><p className="rounded-lg border border-white/10 bg-slate-950 p-3 text-xs text-slate-400">Payload content is hidden in the operator console because events may contain sensitive data.</p></CardContent></Card>
  </Shell>;
}

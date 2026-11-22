import Link from "next/link";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { PublishEventForm } from "@/components/publish-event-form";
import { ApiError, readApi, requireOrgSession } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { EventListItem, Page, Principal } from "@/lib/types";

type Filters = { cursor?: string; event_id?: string; type?: string; created_after?: string; created_before?: string; outcome?: string };
const outcomes = ["", "no_delivery", "pending", "processing", "retry_wait", "succeeded", "exhausted", "cancelled"];

function outcomeSummary(event: EventListItem): string {
  const s = event.delivery_summary;
  if (!s.total) return "No delivery";
  const waiting = s.pending + s.processing + s.retry_wait;
  return [s.succeeded && `${s.succeeded} succeeded`, s.exhausted && `${s.exhausted} exhausted`, waiting && `${waiting} in progress`, s.cancelled && `${s.cancelled} cancelled`].filter(Boolean).join(" · ");
}

export default async function EventsPage({ searchParams }: { searchParams: Promise<Filters> }) {
  const session = await requireOrgSession();
  const query = await searchParams;
  const params = new URLSearchParams({ limit: "50" });
  for (const name of ["event_id", "type", "created_after", "created_before", "outcome", "cursor"] as const) {
    if (query[name]) params.set(name, query[name]);
  }
  let page: Page<EventListItem> = { data: [], next_cursor: "" };
  let filterError = "";
  try { page = await readApi<Page<EventListItem>>(session, `/v1/events?${params}`); }
  catch (error) { if (error instanceof ApiError && error.status === 400) filterError = error.message; else throw error; }
  const me = await readApi<Principal>(session, "/v1/me");
  const nextParams = new URLSearchParams(params);
  nextParams.set("cursor", page.next_cursor);
  return <Shell><PageTitle eyebrow="Publisher activity" title="Event explorer" description="Search accepted events and inspect each event's delivery runs. Filters are scoped to your organization." />
    {me.scopes.includes("events:write") ? <Card id="publish-event" className="mb-6 scroll-mt-6"><CardHeader><h2 className="font-semibold">Publish an event</h2><p className="mt-1 text-xs text-slate-500">Schedule it for later or deliver as soon as the outbox dispatches it. Retries keep the same delivery ID.</p></CardHeader><CardContent><PublishEventForm /></CardContent></Card> : null}
    <Card className="mb-6"><CardHeader><h2 className="font-semibold">Find events</h2><p className="mt-1 text-xs text-slate-500">Type matches the beginning of an event name. Times are UTC; before is exclusive. An event matches an outcome when any of its delivery runs has that status, so it can appear under more than one outcome.</p></CardHeader><CardContent><form method="get" action="/events" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      <label className="text-xs text-slate-400">Event ID<input name="event_id" defaultValue={query.event_id ?? ""} placeholder="Exact UUID" className="mt-1 w-full rounded-lg border border-white/10 bg-slate-950 px-3 py-2 text-sm text-white" /></label>
      <label className="text-xs text-slate-400">Type prefix<input name="type" defaultValue={query.type ?? ""} placeholder="order.created" className="mt-1 w-full rounded-lg border border-white/10 bg-slate-950 px-3 py-2 text-sm text-white" /></label>
      <label className="text-xs text-slate-400">Delivery outcome<select name="outcome" defaultValue={outcomes.includes(query.outcome ?? "") ? query.outcome : ""} className="mt-1 w-full rounded-lg border border-white/10 bg-slate-950 px-3 py-2 text-sm text-white">{outcomes.map((item) => <option key={item} value={item}>{item ? item.replaceAll("_", " ") : "Any outcome"}</option>)}</select></label>
      <label className="text-xs text-slate-400">Accepted after (UTC)<input type="datetime-local" name="created_after" defaultValue={query.created_after ?? ""} className="mt-1 w-full rounded-lg border border-white/10 bg-slate-950 px-3 py-2 text-sm text-white" /></label>
      <label className="text-xs text-slate-400">Accepted before (UTC)<input type="datetime-local" name="created_before" defaultValue={query.created_before ?? ""} className="mt-1 w-full rounded-lg border border-white/10 bg-slate-950 px-3 py-2 text-sm text-white" /></label>
      <div className="flex items-end gap-3"><button type="submit" className="rounded-lg bg-emerald-400 px-4 py-2 text-sm font-semibold text-slate-950">Search</button><Link href="/events" className="py-2 text-sm text-slate-400 hover:text-white">Clear</Link></div>
    </form>{filterError ? <p role="alert" className="mt-3 text-sm text-red-300">{filterError}</p> : null}</CardContent></Card>
    <Card><CardHeader><h2 className="font-semibold">Event history</h2></CardHeader><CardContent>{page.data.length ? <div className="overflow-x-auto"><table className="w-full min-w-[760px] text-left text-sm"><thead className="border-b border-white/10 text-xs uppercase tracking-wide text-slate-500"><tr><th className="pb-3 font-medium">Event</th><th className="pb-3 font-medium">ID</th><th className="pb-3 font-medium">Delivery outcomes</th><th className="pb-3 font-medium">Accepted</th><th className="pb-3 font-medium" /></tr></thead><tbody className="divide-y divide-white/10">{page.data.map((event) => <tr key={event.id}><td className="py-4 font-medium text-slate-200">{event.type}</td><td className="py-4 font-mono text-xs text-slate-400">{shortId(event.id)}</td><td className="py-4 text-xs text-slate-400">{outcomeSummary(event)}</td><td className="py-4 text-slate-400">{dateTime(event.created_at)}</td><td className="py-4 text-right"><Link className="text-emerald-300 hover:underline" href={`/events/${event.id}`}>Inspect →</Link></td></tr>)}</tbody></table></div> : <EmptyState title={filterError ? "Check the filters" : "No events match"} description={filterError ? "Correct the filter above and search again." : "Try a wider time range or publish an event."} />}{page.next_cursor ? <Link className="mt-5 inline-block text-sm text-emerald-300" href={`/events?${nextParams}`}>Next page →</Link> : null}</CardContent></Card>
  </Shell>;
}

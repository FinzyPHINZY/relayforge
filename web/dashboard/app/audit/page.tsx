import Link from "next/link";
import { notFound } from "next/navigation";
import { PageTitle, Shell, EmptyState } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime, shortId } from "@/lib/format";
import type { AuditEvent, Page, Principal } from "@/lib/types";

const targetPath: Partial<Record<AuditEvent["target_type"], string>> = { endpoint: "/endpoints", delivery: "/deliveries", event: "/events" };

export default async function AuditPage({ searchParams }: { searchParams: Promise<{ cursor?: string }> }) {
  const key = await requireKey();
  const me = await readApi<Principal>(key, "/v1/me");
  if (me.role !== "owner" || !me.scopes.includes("team:read")) notFound();
  const { cursor } = await searchParams;
  const params = new URLSearchParams({ limit: "50" });
  if (cursor) params.set("cursor", cursor);
  const page = await readApi<Page<AuditEvent>>(key, `/v1/audit?${params}`);
  return <Shell><PageTitle eyebrow="Access and operations" title="Audit history" description="A tenant-scoped record of operator changes and payload inspections. Secrets, payloads, and destination URLs are never recorded here." />
    <Card><CardHeader><h2 className="font-semibold">Recent actions</h2><p className="mt-1 text-xs text-slate-500">Newest first · retained for the organization lifetime and included in its export.</p></CardHeader><CardContent>
      {page.data.length ? <ol className="divide-y divide-white/10">{page.data.map((entry) => {
        const path = targetPath[entry.target_type];
        return <li key={entry.id} className="py-4 first:pt-0 last:pb-0"><div className="flex flex-wrap items-start justify-between gap-2"><div><p className="text-sm font-semibold text-slate-100">{entry.action.replaceAll(".", " · ")}</p><p className="mt-1 text-xs text-slate-400">{entry.actor_key_name} ({entry.actor_role}) · {entry.target_type} {path ? <Link href={`${path}/${entry.target_id}`} className="font-mono text-emerald-300 hover:underline">{shortId(entry.target_id)}</Link> : <span className="font-mono">{shortId(entry.target_id)}</span>}</p></div><time className="text-xs text-slate-500" dateTime={entry.created_at}>{dateTime(entry.created_at)}</time></div>{Object.keys(entry.details).length ? <p className="mt-2 break-words font-mono text-xs text-slate-500">{JSON.stringify(entry.details)}</p> : null}</li>;
      })}</ol> : <EmptyState title="No audited actions yet" description="Endpoint, team, review, and replay changes will appear here." />}
      {page.next_cursor ? <Link href={`/audit?cursor=${encodeURIComponent(page.next_cursor)}`} className="mt-5 inline-block text-sm text-emerald-300 hover:underline">Older actions →</Link> : null}
    </CardContent></Card>
  </Shell>;
}

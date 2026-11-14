import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { setEndpointStatus } from "@/app/actions";
import { CreateEndpointForm } from "@/components/create-endpoint-form";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { Endpoint, Page, Principal } from "@/lib/types";

export default async function EndpointsPage({ searchParams }: { searchParams: Promise<{ cursor?: string; error?: string }> }) {
  const key = await requireKey();
  const { cursor, error } = await searchParams;
  const params = new URLSearchParams({ limit: "50" });
  if (cursor) params.set("cursor", cursor);
  const [page, me] = await Promise.all([readApi<Page<Endpoint>>(key, `/v1/endpoints?${params}`), readApi<Principal>(key, "/v1/me")]);
  const canWrite = me.scopes.includes("endpoints:write");
  return <Shell><PageTitle eyebrow="Destinations" title="Endpoints" description="Manage subscriptions, verification, transformations, and alerts." />
    {error ? <p role="alert" className="mb-5 rounded-lg border border-rose-400/20 bg-rose-400/10 p-3 text-sm text-rose-200">Could not update the endpoint.</p> : null}
    <Card><CardHeader><h2 className="font-semibold">Registered endpoints</h2></CardHeader><CardContent>{page.data.length ? <div className="divide-y divide-white/10">{page.data.map((endpoint) => <div key={endpoint.id} className="flex flex-wrap items-center justify-between gap-4 py-4 first:pt-0 last:pb-0">
      <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><p className="break-all text-sm font-medium text-slate-100">{endpoint.url}</p><StatusBadge status={endpoint.status} />{endpoint.circuit_status !== "closed" ? <StatusBadge status={endpoint.circuit_status} /> : null}{endpoint.verification_required && !endpoint.verified_at ? <span className="rounded bg-amber-400/10 px-2 py-0.5 text-xs text-amber-300">Awaiting verification</span> : null}</div><p className="mt-1 text-xs text-slate-500">{endpoint.description || endpoint.id} · Created {dateTime(endpoint.created_at)}{endpoint.circuit_status === "open" && endpoint.circuit_open_until ? ` · Retry probe after ${dateTime(endpoint.circuit_open_until)}` : ""}</p><div className="mt-2 flex flex-wrap gap-1">{endpoint.event_types.map((type) => <span key={type} className="rounded bg-white/5 px-2 py-0.5 font-mono text-xs text-slate-400">{type}</span>)}</div></div>
      <div className="flex items-center gap-2"><Link href={`/endpoints/${endpoint.id}`} className="inline-flex h-8 items-center gap-1 rounded-lg px-2 text-xs text-emerald-300 hover:bg-white/5">Manage <ArrowRight size={13} /></Link>{canWrite && endpoint.status !== "disabled" ? <form action={setEndpointStatus}><input type="hidden" name="id" value={endpoint.id} /><input type="hidden" name="status" value={endpoint.status === "active" ? "paused" : "active"} /><Button variant="outline" size="sm">{endpoint.status === "active" ? "Pause" : "Resume"}</Button></form> : null}</div>
    </div>)}</div> : <EmptyState title="No endpoints" description="Create one below to receive signed webhooks." />}{page.next_cursor ? <Link className="mt-5 inline-block text-sm text-emerald-300" href={`/endpoints?cursor=${encodeURIComponent(page.next_cursor)}`}>Next page →</Link> : null}</CardContent></Card>
    {canWrite ? <Card className="mt-6"><CardHeader><h2 className="font-semibold">Add an endpoint</h2><p className="mt-1 text-sm text-slate-500">The signing secret is displayed once after creation.</p></CardHeader><CardContent><CreateEndpointForm /></CardContent></Card> : null}
  </Shell>;
}

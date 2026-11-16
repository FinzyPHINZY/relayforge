import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft, ArrowUpRight, ShieldCheck } from "lucide-react";
import { EndpointControls } from "@/components/endpoint-controls";
import { PageTitle, Shell } from "@/components/shell";
import { StatusBadge } from "@/components/status-badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { ApiError, readApi, requireKey } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { Endpoint, Principal } from "@/lib/types";

export default async function EndpointPage({ params }: { params: Promise<{ id: string }> }) {
  const key = await requireKey();
  const { id } = await params;
  let endpoint: Endpoint;
  try { endpoint = await readApi<Endpoint>(key, `/v1/endpoints/${encodeURIComponent(id)}`); } catch (error) { if (error instanceof ApiError && error.status === 404) notFound(); throw error; }
  const me = await readApi<Principal>(key, "/v1/me");
  const canWrite = me.scopes.includes("endpoints:write") && endpoint.status !== "disabled";
  const overlap = endpoint.signing_secrets?.find((secret) => secret.state === "overlap" && secret.overlap_until && new Date(secret.overlap_until).getTime() > Date.now());
  return <Shell>
    <Link href="/endpoints" className="mb-7 inline-flex items-center gap-2 text-xs text-slate-400 hover:text-emerald-300"><ArrowLeft size={15} /> All endpoints</Link>
    <PageTitle eyebrow="Destination" title="Endpoint settings" description="Subscriptions, verification, and signed payload controls." action={<StatusBadge status={endpoint.status} />} />
    <Card><CardContent><p className="break-all font-mono text-sm text-slate-100">{endpoint.url}</p><p className="mt-2 text-xs text-slate-500">{endpoint.description || "No description"} · Created {dateTime(endpoint.created_at)}</p><div className="mt-4 flex flex-wrap gap-2 text-xs"><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Circuit: {endpoint.circuit_status}{endpoint.circuit_open_until && endpoint.circuit_status === "open" ? ` · probe after ${dateTime(endpoint.circuit_open_until)}` : ""}</span><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Verification: {endpoint.verification_required ? endpoint.verified_at ? `verified ${dateTime(endpoint.verified_at)}` : "pending" : "not required"}</span><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Timeout: {endpoint.timeout_ms.toLocaleString()} ms</span><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Max attempts: {endpoint.max_attempts}</span><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Concurrency: {endpoint.concurrency_limit}</span><span className="rounded-lg border border-white/10 px-3 py-1.5 text-slate-300">Rate: {endpoint.rate_limit_per_minute || "unlimited"}{endpoint.rate_limit_per_minute ? "/min" : ""}</span>{overlap ? <span className="rounded-lg border border-amber-400/20 px-3 py-1.5 text-amber-200">Previous secret valid until {dateTime(overlap.overlap_until!)}</span> : null}</div><Link href={`/deliveries?endpoint_id=${encodeURIComponent(endpoint.id)}`} className="mt-5 inline-flex items-center gap-2 text-xs text-emerald-300 hover:underline">View deliveries <ArrowUpRight size={14} /></Link></CardContent></Card>
    <p className="mt-3 text-xs text-slate-500">Circuit closed: normal delivery. Open: claims pause until the probe time. Half-open: one recovery probe is eligible or in progress.</p>
    {canWrite ? <EndpointControls endpoint={endpoint} activeOverlapUntil={overlap?.overlap_until} /> : <Card className="mt-6"><CardContent><p className="text-sm text-slate-400">This key can inspect this endpoint. Changes require endpoint write access.</p></CardContent></Card>}
    {endpoint.verification_required ? <p className="mt-5 flex items-start gap-2 text-xs leading-5 text-slate-500"><ShieldCheck size={15} className="mt-0.5 shrink-0" /> Verification sends a bounded GET to the destination with a challenge header. The receiver must return the challenge in Webhook-Verification-Response.</p> : null}
  </Shell>;
}

import Link from "next/link";
import { Activity, CheckCircle2, CircleAlert } from "lucide-react";
import { PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import type { AnalyticsBucket, DeliveryAnalytics } from "@/lib/types";

type Window = "24h" | "7d" | "30d";
type Bar = { label: string; attempts: number; succeeded: number; failed: number; p95: number | null };

function chartBars(buckets: AnalyticsBucket[], window: Window): Bar[] {
  const count = window === "24h" ? 24 : window === "7d" ? 7 : 30;
  const unit = window === "24h" ? 3_600_000 : 86_400_000;
  const end = new Date();
  if (window === "24h") end.setUTCMinutes(0, 0, 0); else end.setUTCHours(0, 0, 0, 0);
  const map = new Map<string, Bar>();
  for (let i = count - 1; i >= 0; i--) {
    const date = new Date(end.getTime() - i * unit);
    const key = date.toISOString().slice(0, window === "24h" ? 13 : 10);
    map.set(key, { label: window === "24h" ? `${date.getUTCHours().toString().padStart(2, "0")}:00 UTC` : date.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" }), attempts: 0, succeeded: 0, failed: 0, p95: null });
  }
  for (const bucket of buckets) {
    const key = bucket.start.slice(0, window === "24h" ? 13 : 10);
    const bar = map.get(key);
    if (!bar) continue;
    bar.attempts += bucket.attempts;
    bar.succeeded += bucket.succeeded;
    bar.failed += bucket.failed;
    if (bucket.p95_duration_ms !== null) bar.p95 = bucket.p95_duration_ms;
  }
  return [...map.values()];
}

export default async function AnalyticsPage({ searchParams }: { searchParams: Promise<{ window?: string }> }) {
  const key = await requireKey();
  const requested = (await searchParams).window;
  const window: Window = requested === "7d" || requested === "30d" ? requested : "24h";
  const analytics = await readApi<DeliveryAnalytics>(key, `/v1/analytics/deliveries?window=${window}`);
  const bars = chartBars(analytics.buckets, window);
  const total = bars.reduce((sum, bar) => sum + bar.attempts, 0);
  const succeeded = bars.reduce((sum, bar) => sum + bar.succeeded, 0);
  const failed = bars.reduce((sum, bar) => sum + bar.failed, 0);
  const max = Math.max(...bars.map((bar) => bar.attempts), 1);
  return <Shell><PageTitle eyebrow="Delivery trends" title="Analytics" description="Tenant-scoped attempt counts and latency from recorded delivery history." />
    <div className="mb-6 flex gap-2">{(["24h", "7d", "30d"] as const).map((option) => <Link key={option} href={`/analytics?window=${option}`} aria-current={option === window ? "page" : undefined} className={`rounded-lg px-4 py-2 text-sm font-medium ${option === window ? "bg-emerald-400 text-slate-950" : "border border-white/10 text-slate-400 hover:text-white"}`}>{option}</Link>)}</div>
    <div className="grid gap-4 sm:grid-cols-3"><Metric icon={Activity} label="Attempts" value={total} /><Metric icon={CheckCircle2} label="Acknowledged" value={succeeded} /><Metric icon={CircleAlert} label="Failed attempts" value={failed} /></div>
    <Card className="mt-6"><CardHeader><h2 className="font-semibold">Attempts over time</h2><p className="mt-1 text-xs text-slate-500">{window === "24h" ? "Hourly" : "Daily"} UTC buckets. Green is acknowledged; amber is failed.</p></CardHeader><CardContent>{total ? <div role="img" aria-label={`${total} delivery attempts in the selected window`} className="flex h-56 items-end gap-1.5 border-b border-white/10 pb-1">{bars.map((bar, index) => <div key={index} title={`${bar.label}: ${bar.attempts} attempts, ${bar.succeeded} acknowledged, ${bar.failed} failed`} className="flex h-full min-w-0 flex-1 flex-col justify-end gap-px"><div className="rounded-t-sm bg-amber-300/80" style={{ height: `${Math.max(bar.failed ? 3 : 0, bar.failed / max * 100)}%` }} /><div className="rounded-t-sm bg-emerald-400" style={{ height: `${Math.max(bar.succeeded ? 3 : 0, bar.succeeded / max * 100)}%` }} /></div>)}</div> : <p className="py-20 text-center text-sm text-slate-500">No attempts recorded in this window.</p>}<div className="mt-3 flex justify-between text-xs text-slate-500"><span>{bars[0]?.label}</span><span>{bars.at(-1)?.label}</span></div></CardContent></Card>
    <Card className="mt-6"><CardHeader><h2 className="font-semibold">Recent active buckets</h2></CardHeader><CardContent>{analytics.buckets.length ? <div className="overflow-x-auto"><table className="w-full min-w-[570px] text-left text-sm"><thead className="border-b border-white/10 text-xs text-slate-500"><tr><th className="pb-3 font-medium">UTC hour</th><th className="pb-3 font-medium">Attempts</th><th className="pb-3 font-medium">Success</th><th className="pb-3 font-medium">Failed</th><th className="pb-3 font-medium">P50</th><th className="pb-3 font-medium">P95</th></tr></thead><tbody className="divide-y divide-white/10">{analytics.buckets.slice(-12).reverse().map((bucket) => <tr key={bucket.start}><td className="py-3 text-slate-300">{new Date(bucket.start).toISOString().slice(0, 16).replace("T", " ")}</td><td className="py-3 text-slate-400">{bucket.attempts}</td><td className="py-3 text-emerald-300">{bucket.succeeded}</td><td className="py-3 text-amber-300">{bucket.failed}</td><td className="py-3 text-slate-400">{bucket.p50_duration_ms === null ? "—" : `${Math.round(bucket.p50_duration_ms)} ms`}</td><td className="py-3 text-slate-400">{bucket.p95_duration_ms === null ? "—" : `${Math.round(bucket.p95_duration_ms)} ms`}</td></tr>)}</tbody></table></div> : <p className="text-sm text-slate-500">No recorded attempts yet.</p>}</CardContent></Card>
  </Shell>;
}

function Metric({ icon: Icon, label, value }: { icon: typeof Activity; label: string; value: number }) {
  return <Card><CardContent><div className="flex items-center gap-2 text-xs text-slate-400"><Icon size={16} className="text-emerald-400" />{label}</div><p className="mt-4 text-3xl font-semibold text-white">{value.toLocaleString()}</p></CardContent></Card>;
}

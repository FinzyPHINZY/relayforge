import { Card, CardContent, CardHeader } from "@/components/ui/card";
import type { OverviewAnalytics, OverviewBucket } from "@/lib/types";

function bucketLabel(start: string, hourly: boolean): string {
  const date = new Date(start);
  return hourly ? `${date.getUTCHours().toString().padStart(2, "0")}:00` : date.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
}

function duration(ms: number | null): string {
  if (ms === null) return "—";
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.round(ms)} ms`;
}

function points(buckets: OverviewBucket[], field: "p50_latency_ms" | "p95_latency_ms", max: number): { x: number; y: number }[][] {
  const segments: { x: number; y: number }[][] = [];
  let segment: { x: number; y: number }[] = [];
  buckets.forEach((bucket, index) => {
    const value = bucket[field];
    if (value === null) {
      if (segment.length) segments.push(segment);
      segment = [];
      return;
    }
    segment.push({ x: 24 + index * 672 / Math.max(buckets.length - 1, 1), y: 176 - value / max * 142 });
  });
  if (segment.length) segments.push(segment);
  return segments;
}

export function OverviewCharts({ data }: { data: OverviewAnalytics }) {
  const hourly = data.bucket_unit === "1 hour";
  const maxEvents = Math.max(1, ...data.buckets.map((bucket) => bucket.events));
  const maxLatency = Math.max(1, ...data.buckets.map((bucket) => bucket.p95_latency_ms ?? 0));
  const hasLatency = data.buckets.some((bucket) => bucket.p95_latency_ms !== null);
  const tickEvery = data.buckets.length > 24 ? 5 : data.buckets.length > 7 ? 4 : 1;
  return <>
    <div className="mt-6 grid gap-4 xl:grid-cols-2">
      <Card><CardHeader><h2 className="font-semibold text-white">Event throughput</h2><p className="mt-1 text-xs text-slate-400">Accepted events per {hourly ? "hour" : "UTC day"} · {data.totals.events.toLocaleString()} in this window</p></CardHeader><CardContent>
        {data.totals.events ? <div role="img" aria-label={`Accepted events by ${hourly ? "hour" : "day"}; ${data.totals.events} total`} className="flex h-48 items-end gap-1 border-b border-white/10">{data.buckets.map((bucket) => <div key={bucket.start} title={`${bucketLabel(bucket.start, hourly)} UTC: ${bucket.events} accepted`} className="flex h-full min-w-0 flex-1 items-end"><div className="w-full rounded-t bg-emerald-400/90" style={{ height: `${bucket.events ? Math.max(3, bucket.events / maxEvents * 100) : 0}%` }} /></div>)}</div> : <p className="flex h-48 items-center justify-center text-sm text-slate-500">No events accepted in this window.</p>}
        <div className="mt-2 flex justify-between text-xs text-slate-500"><span>{bucketLabel(data.buckets[0].start, hourly)} UTC</span><span>{bucketLabel(data.buckets.at(-1)!.start, hourly)} UTC</span></div>
      </CardContent></Card>
      <Card><CardHeader><h2 className="font-semibold text-white">Delivery latency</h2><p className="mt-1 text-xs text-slate-400">Event acceptance to terminal delivery · p50 <span className="text-emerald-300">●</span> · p95 <span className="text-amber-300">●</span></p></CardHeader><CardContent>
        {hasLatency ? <svg viewBox="0 0 720 190" role="img" aria-label="Median and 95th percentile event-to-terminal-delivery latency by bucket" className="h-48 w-full overflow-visible">
          <path d="M24 34H696 M24 105H696 M24 176H696" stroke="currentColor" className="text-white/10" fill="none" />
          {points(data.buckets, "p95_latency_ms", maxLatency).map((segment, index) => <g key={`p95-${index}`}><polyline points={segment.map((p) => `${p.x},${p.y}`).join(" ")} fill="none" stroke="#fbbf24" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />{segment.length === 1 && <circle cx={segment[0].x} cy={segment[0].y} r="3.5" fill="#fbbf24" />}</g>)}
          {points(data.buckets, "p50_latency_ms", maxLatency).map((segment, index) => <g key={`p50-${index}`}><polyline points={segment.map((p) => `${p.x},${p.y}`).join(" ")} fill="none" stroke="#34d399" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />{segment.length === 1 && <circle cx={segment[0].x} cy={segment[0].y} r="3.5" fill="#34d399" />}</g>)}
        </svg> : <p className="flex h-48 items-center justify-center text-sm text-slate-500">No deliveries completed in this window.</p>}
        <div className="mt-2 flex justify-between text-xs text-slate-500"><span>{bucketLabel(data.buckets[0].start, hourly)} UTC</span><span>{hasLatency ? `Top of scale: ${duration(maxLatency)}` : bucketLabel(data.buckets.at(-1)!.start, hourly)}</span></div>
      </CardContent></Card>
    </div>
    <details className="mt-4 rounded-xl border border-white/10 bg-slate-900/40 px-4 py-3"><summary className="cursor-pointer text-sm font-medium text-slate-300">View exact bucket values</summary><div className="mt-4 overflow-x-auto"><table className="w-full min-w-[780px] text-left text-xs"><thead className="border-b border-white/10 text-slate-400"><tr><th className="pb-2">UTC bucket</th><th className="pb-2">Events</th><th className="pb-2">Succeeded</th><th className="pb-2">Exhausted</th><th className="pb-2">Failed attempts</th><th className="pb-2">Retries</th><th className="pb-2">P50 latency</th><th className="pb-2">P95 latency</th></tr></thead><tbody className="divide-y divide-white/10">{data.buckets.map((bucket) => <tr key={bucket.start}><td className="py-2 text-slate-300">{bucketLabel(bucket.start, hourly)}</td><td>{bucket.events}</td><td>{bucket.succeeded}</td><td>{bucket.exhausted}</td><td>{bucket.failed_attempts}</td><td>{bucket.retries}</td><td>{duration(bucket.p50_latency_ms)}</td><td>{duration(bucket.p95_latency_ms)}</td></tr>)}</tbody></table></div></details>
  </>;
}

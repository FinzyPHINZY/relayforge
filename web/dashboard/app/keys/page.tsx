import type { Metadata } from "next";
import { KeyRound } from "lucide-react";
import { KeyDetailsSheet } from "@/components/key-details-sheet";
import { CreateKeySheet } from "@/components/key-controls";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, readPrincipal, requireOrgSession } from "@/lib/api";
import { keyStatus } from "@/lib/key-summary";
import type { ApiKey } from "@/lib/types";

export const metadata: Metadata = { title: "API keys · RelayForge" };

const th = "text-xs font-semibold uppercase tracking-[0.12em] text-slate-500";

export default async function KeysPage() {
  const session = await requireOrgSession();
  const me = await readPrincipal(session);
  const canRead = me.scopes.includes("keys:read");
  const canWrite = me.scopes.includes("keys:write");
  const keys = canRead ? (await readApi<{ data: ApiKey[] }>(session, "/v1/keys")).data : [];
  const now = new Date();
  const active = keys.filter((key) => keyStatus(key, now).label === "Active").length;
  return <Shell><PageTitle eyebrow="Access" title="API keys" description="Keys let scripts and services call the API. Each key is capped by its role and scopes." action={canWrite ? <CreateKeySheet myRole={me.role} myScopes={me.scopes} /> : undefined} />
    <Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-3"><KeyRound size={19} className="text-emerald-400" aria-hidden="true" /><h2 className="font-semibold">Workspace keys</h2></div>{canRead && keys.length ? <span className="text-xs text-slate-500">{active} active · {keys.length} total</span> : null}</div></CardHeader><CardContent className="p-2 sm:p-3">
      {!canRead ? <div className="p-3"><EmptyState title="Keys are hidden" description="This session doesn't have the keys:read scope." /></div>
        : keys.length ? <>
          <div aria-hidden="true" className="hidden items-center gap-3 border-b border-white/10 px-3 pb-2.5 pt-1 sm:flex"><div className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_10rem_5.5rem_6rem] gap-4"><span className={th}>Name</span><span className={th}>Prefix</span><span className={th}>Role</span><span className={th}>Last used</span></div><span className="w-4 shrink-0" /></div>
          <ul className="divide-y divide-white/10">{keys.map((key) => <li key={key.id}><KeyDetailsSheet apiKey={key} canWrite={canWrite} now={now.toISOString()} /></li>)}</ul>
        </> : <div className="p-3"><EmptyState title="No API keys yet" description={canWrite ? "Use Create key to add one for a script or service." : "Ask an owner or admin to create one."} /></div>}
    </CardContent></Card>
  </Shell>;
}

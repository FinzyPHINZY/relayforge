import type { Metadata } from "next";
import { KeyRound } from "lucide-react";
import { CreateKeyForm, RevokeKeyButton } from "@/components/key-controls";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireOrgSession } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { allowedScopes } from "@/lib/key-form";
import type { ApiKey, Principal } from "@/lib/types";

export const metadata: Metadata = { title: "API keys · RelayForge" };

function keyState(key: ApiKey, now: number): { label: string; tone: "good" | "warn" | "bad" | "neutral" } {
  if (key.revoked_at) return { label: "Revoked", tone: "bad" };
  if (key.expires_at && new Date(key.expires_at).getTime() <= now) return { label: "Expired", tone: "neutral" };
  return { label: "Active", tone: "good" };
}

function Scopes({ apiKey }: { apiKey: ApiKey }) {
  const everything = allowedScopes(apiKey.role);
  const complete = everything.length > 0 && everything.every((scope) => apiKey.scopes.includes(scope));
  if (complete) return <span title={apiKey.scopes.join(", ")} className="inline-block whitespace-nowrap rounded bg-white/5 px-2 py-0.5 text-xs text-slate-300">All {apiKey.role} scopes</span>;
  return <div className="flex flex-wrap gap-1">{apiKey.scopes.map((scope) => <span key={scope} className="rounded bg-white/5 px-2 py-0.5 font-mono text-[11px] text-slate-400">{scope}</span>)}</div>;
}

const th = "whitespace-nowrap px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 first:pl-0 last:pr-0";
// Below lg each row becomes a labelled card; the label comes from data-label.
const td = "px-3 py-3 align-top text-sm first:pl-0 last:pr-0 max-lg:grid max-lg:grid-cols-[6.5rem_minmax(0,1fr)] max-lg:gap-3 max-lg:px-0 max-lg:py-1.5 max-lg:before:text-xs max-lg:before:font-medium max-lg:before:text-slate-500 max-lg:before:content-[attr(data-label)]";

export default async function KeysPage() {
  const session = await requireOrgSession();
  const me = await readApi<Principal>(session, "/v1/me");
  const canRead = me.scopes.includes("keys:read");
  const canWrite = me.scopes.includes("keys:write");
  const keys = canRead ? (await readApi<{ data: ApiKey[] }>(session, "/v1/keys")).data : [];
  const now = Date.now();
  const active = keys.filter((key) => keyState(key, now).label === "Active").length;
  return <Shell><PageTitle eyebrow="Access" title="API keys" description="Keys let scripts and services call the API. Each key is capped by its role and scopes." />
    <Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-3"><KeyRound size={19} className="text-emerald-400" aria-hidden="true" /><h2 className="font-semibold">Workspace keys</h2></div>{canRead && keys.length ? <span className="text-xs text-slate-500">{active} active · {keys.length} total</span> : null}</div></CardHeader><CardContent>
      {!canRead ? <EmptyState title="Keys are hidden" description="This session doesn't have the keys:read scope." />
        : keys.length ? <table className="w-full border-collapse max-lg:block">
          <thead className="max-lg:sr-only"><tr className="border-b border-white/10"><th className={th}>Name</th><th className={th}>Prefix</th><th className={th}>Role</th><th className={th}>Scopes</th><th className={th}>Created by</th><th className={th}>Last used</th><th className={th}>Expires</th><th className={th}><span className="sr-only">Actions</span></th></tr></thead>
          <tbody className="max-lg:block max-lg:space-y-3">{keys.map((key) => {
            const state = keyState(key, now);
            const live = state.label === "Active";
            return <tr key={key.id} className={`border-b border-white/10 last:border-b-0 max-lg:block max-lg:rounded-xl max-lg:border max-lg:border-white/10 max-lg:p-4 max-lg:last:border-b ${live ? "" : "opacity-70"}`}>
              <td className={td} data-label="Name"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="break-words font-medium text-slate-100">{key.name}</span>{live ? null : <Badge tone={state.tone}>{state.label}</Badge>}</div><div className="mt-0.5 text-xs text-slate-500">Created {dateTime(key.created_at)}</div></div></td>
              <td className={td} data-label="Prefix"><code className="font-mono text-xs text-slate-300">{key.prefix}…</code></td>
              <td className={td} data-label="Role"><span className="capitalize text-slate-200">{key.role}</span></td>
              <td className={`${td} lg:max-w-[16rem]`} data-label="Scopes"><Scopes apiKey={key} /></td>
              <td className={td} data-label="Created by"><span className="text-slate-300">{key.created_by ? key.created_by.display_name || "Team member" : <span className="text-slate-500">System</span>}</span></td>
              <td className={`${td} lg:whitespace-nowrap`} data-label="Last used"><span className="text-slate-300">{key.last_used_at ? dateTime(key.last_used_at) : <span className="text-slate-500">Never</span>}</span></td>
              <td className={`${td} lg:whitespace-nowrap`} data-label="Expires"><span className="text-slate-300">{key.revoked_at ? <>Revoked {dateTime(key.revoked_at)}</> : key.expires_at ? dateTime(key.expires_at) : <span className="text-slate-500">Never</span>}</span></td>
              <td className={`${td} lg:text-right max-lg:mt-2 max-lg:block max-lg:before:hidden max-lg:empty:hidden`} data-label="">{live && canWrite ? <RevokeKeyButton id={key.id} name={key.name} /> : null}</td>
            </tr>;
          })}</tbody>
        </table> : <EmptyState title="No API keys yet" description={canWrite ? "Create one below for a script or service." : "Ask an owner or admin to create one."} />}
    </CardContent></Card>
    {canWrite ? <Card className="mt-6"><CardHeader><h2 className="font-semibold">Create a key</h2><p className="mt-1 text-xs text-slate-500">The key is shown once. Store it in your secret manager.</p></CardHeader><CardContent><CreateKeyForm myRole={me.role} myScopes={me.scopes} /></CardContent></Card> : null}
  </Shell>;
}

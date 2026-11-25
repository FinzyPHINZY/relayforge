"use client";

import { useActionState, useState } from "react";
import { Check, Copy, TriangleAlert } from "lucide-react";
import { createKeyControl, revokeKeyControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { allowedRoles, expiryChoices, offeredScopes, type Role } from "@/lib/key-form";

const initial: ControlState = {};
const selectClass = "h-10 w-full rounded-lg border border-white/15 bg-slate-950/60 px-3 text-sm text-white focus:border-emerald-400 focus:outline-none";

const resources = [
  { id: "events", label: "Events" },
  { id: "endpoints", label: "Endpoints" },
  { id: "deliveries", label: "Deliveries" },
  { id: "metrics", label: "Metrics" },
  { id: "team", label: "Team" },
  { id: "keys", label: "API keys" },
] as const;

function readScopes(scopes: string[]): string[] {
  return scopes.filter((scope) => scope.endsWith(":read"));
}

export function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }
  return <Button type="button" variant="outline" size="sm" onClick={copy} aria-live="polite">{copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}{copied ? "Copied" : label}</Button>;
}

// the fields live in a child keyed by the created key's id, so a successful create
// remounts them with defaults.
export function CreateKeyForm({ myRole, myScopes }: { myRole: string; myScopes: string[] }) {
  const [state, action, pending] = useActionState(createKeyControl, initial);
  if (!allowedRoles(myRole).length) return <p className="text-sm text-slate-500">Your role can't issue keys in this workspace.</p>;
  return <div>
    {state.secret ? <div role="status" className="mb-6 rounded-xl border border-amber-400/30 bg-amber-400/10 p-4">
      <div className="flex items-start gap-3"><TriangleAlert size={18} aria-hidden="true" className="mt-0.5 shrink-0 text-amber-300" /><div className="min-w-0 flex-1"><p className="text-sm font-semibold text-amber-200">{state.success} Copy it now.</p><p className="mt-1 text-xs text-amber-100">You won't see this key again.</p></div></div>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center"><code className="block min-w-0 flex-1 break-all rounded-lg border border-white/10 bg-slate-900 px-3 py-2 font-mono text-sm text-white">{state.secret}</code><CopyButton value={state.secret} /></div>
    </div> : null}
    <KeyFields key={state.id ?? "new"} myRole={myRole} myScopes={myScopes} action={action} pending={pending} error={state.error} />
  </div>;
}

function KeyFields({ myRole, myScopes, action, pending, error }: { myRole: string; myScopes: string[]; action: (form: FormData) => void; pending: boolean; error?: string }) {
  const roles = allowedRoles(myRole);
  const [role, setRole] = useState<Role>(roles[roles.length - 1] ?? "viewer");
  const offered = offeredScopes(role, myScopes);
  const [selected, setSelected] = useState<Set<string>>(() => new Set(readScopes(offered)));

  function changeRole(next: Role) {
    setRole(next);
    const allowed = new Set(offeredScopes(next, myScopes));
    setSelected((previous) => new Set([...previous].filter((scope) => allowed.has(scope))));
  }

  function toggle(scope: string, on: boolean) {
    setSelected((previous) => {
      const next = new Set(previous);
      if (on) next.add(scope); else next.delete(scope);
      return next;
    });
  }

  return <form action={action} className="grid gap-5">
      <div className="grid gap-4 md:grid-cols-3">
        <div><label htmlFor="keyName" className="mb-2 block text-xs text-slate-300">Name</label><Input id="keyName" name="name" required maxLength={80} placeholder="CI deploys" autoComplete="off" /></div>
        <div><label htmlFor="keyRole" className="mb-2 block text-xs text-slate-300">Role</label><select id="keyRole" name="role" value={role} onChange={(event) => changeRole(event.target.value as Role)} className={`${selectClass} capitalize`}>{roles.map((item) => <option key={item} value={item}>{item[0].toUpperCase() + item.slice(1)}</option>)}</select></div>
        <div><label htmlFor="keyExpiry" className="mb-2 block text-xs text-slate-300">Expires</label><select id="keyExpiry" name="expiry" defaultValue="90" className={selectClass}>{expiryChoices.map((choice) => <option key={choice.value} value={choice.value}>{choice.label}</option>)}</select></div>
      </div>
      <fieldset>
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2"><legend className="text-xs text-slate-300">Scopes</legend><div className="flex gap-3 text-xs"><button type="button" className="text-emerald-300 hover:underline" onClick={() => setSelected(new Set(offered))}>Select all</button><button type="button" className="text-emerald-300 hover:underline" onClick={() => setSelected(new Set(readScopes(offered)))}>Read only</button><button type="button" className="text-slate-500 hover:underline" onClick={() => setSelected(new Set())}>Clear</button></div></div>
        {offered.length ? <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">{resources.map(({ id, label }) => {
          const scopes = offered.filter((scope) => scope.startsWith(`${id}:`));
          if (!scopes.length) return null;
          return <div key={id} className="flex items-center justify-between gap-3 rounded-lg border border-white/10 bg-slate-950/40 px-3 py-2.5"><span className="text-sm font-medium text-slate-200">{label}</span><span className="flex gap-3">{scopes.map((scope) => {
            const action = scope.split(":")[1];
            return <label key={scope} className="flex cursor-pointer items-center gap-1.5 text-xs text-slate-400"><input type="checkbox" name="scopes" value={scope} checked={selected.has(scope)} onChange={(event) => toggle(scope, event.target.checked)} className="size-4 accent-[#3c7a40]" /><span className="capitalize">{action}</span><span className="sr-only"> {label}</span></label>;
          })}</span></div>;
        })}</div> : <p className="text-sm text-slate-500">This role has no scopes you can grant.</p>}
        <p className="mt-2 text-xs text-slate-500">Only scopes the {role} role allows, and that you hold yourself, are offered.</p>
      </fieldset>
      <div><Button disabled={pending || selected.size === 0}>{pending ? "Creating…" : "Create key"}</Button>{error ? <p role="alert" className="mt-3 text-sm text-rose-300">{error}</p> : null}</div>
  </form>;
}

export function RevokeKeyButton({ id, name }: { id: string; name: string }) {
  const [confirming, setConfirming] = useState(false);
  const [state, action, pending] = useActionState(revokeKeyControl, initial);
  return <div className="flex flex-col items-start gap-2 lg:items-end">
    {confirming ? <form action={action} className="flex flex-wrap items-center gap-2"><input type="hidden" name="id" value={id} /><span className="text-xs text-slate-400">Revoke <span className="font-medium text-slate-200">{name}</span>?</span><Button variant="destructive" size="sm" disabled={pending}>{pending ? "Revoking…" : "Revoke"}</Button><Button type="button" variant="ghost" size="sm" disabled={pending} onClick={() => setConfirming(false)}>Cancel</Button></form>
      : <Button type="button" variant="outline" size="sm" onClick={() => setConfirming(true)} aria-label={`Revoke ${name}`}>Revoke</Button>}
    {state.error ? <p role="alert" className="max-w-xs text-xs text-rose-300">{state.error}</p> : null}
  </div>;
}

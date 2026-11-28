"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { RevokeKeyButton } from "@/components/key-controls";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { dateTime } from "@/lib/format";
import { keyStatus, lastUsedLabel, type KeyStatus } from "@/lib/key-summary";
import type { ApiKey } from "@/lib/types";

const badgeTone = { neutral: "good", warning: "warn", danger: "bad" } as const satisfies Record<KeyStatus["tone"], "good" | "warn" | "bad">;

export function StatusBadge({ status }: { status: KeyStatus }) {
  return <Badge tone={badgeTone[status.tone]} className="shrink-0">{status.label}</Badge>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="grid gap-1 border-b border-white/10 py-3 last:border-b-0 sm:grid-cols-[7.5rem_minmax(0,1fr)] sm:gap-4">
    <dt className="text-xs font-medium text-slate-500">{label}</dt>
    <dd className="min-w-0 text-sm text-slate-200">{children}</dd>
  </div>;
}

// now comes from the server render so relative times match on hydration.
export function KeyDetailsSheet({ apiKey, canWrite, now }: { apiKey: ApiKey; canWrite: boolean; now?: string }) {
  const at = now ? new Date(now) : new Date();
  const status = keyStatus(apiKey, at);
  const live = status.label === "Active";
  const lastUsed = lastUsedLabel(apiKey, at);
  const [open, setOpen] = useState(false);
  const [revokedHere, setRevokedHere] = useState(false);
  const notice = useRef<HTMLParagraphElement>(null);
  const wasLive = useRef(live);

  // the revoke form unmounts once the list revalidates, so focus moves to the notice instead of the body.
  useEffect(() => {
    if (open && wasLive.current && status.label === "Revoked") setRevokedHere(true);
    wasLive.current = live;
  }, [open, live, status.label]);

  useEffect(() => {
    if (revokedHere) notice.current?.focus();
  }, [revokedHere]);

  return <Sheet open={open} onOpenChange={(next) => { setOpen(next); if (!next) setRevokedHere(false); }}>
    <SheetTrigger asChild>
      <button type="button" className={`group flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-white/5 ${live ? "" : "opacity-70"}`}>
        <span className="block min-w-0 flex-1 sm:grid sm:grid-cols-[minmax(0,1fr)_10rem_5.5rem_6rem] sm:items-center sm:gap-4">
          <span className="flex min-w-0 items-center gap-2"><span className="truncate font-medium text-slate-100"><span className="sr-only">View details for </span>{apiKey.name}</span>{live ? null : <StatusBadge status={status} />}</span>
          <span className="mt-1 flex min-w-0 flex-wrap gap-x-3 gap-y-1 text-xs text-slate-500 sm:contents">
            <code className="truncate font-mono text-xs text-slate-300">{apiKey.prefix}…</code>
            <span className="capitalize text-slate-400 sm:text-sm sm:text-slate-300">{apiKey.role}</span>
            <span className="text-slate-400 sm:text-sm sm:text-slate-300"><span className="sm:sr-only">Last used </span>{lastUsed === "Never" ? <span className="text-slate-500">{lastUsed}</span> : lastUsed}</span>
          </span>
        </span>
        <ChevronRight size={16} aria-hidden="true" className="shrink-0 text-slate-500 transition-transform group-hover:translate-x-0.5" />
      </button>
    </SheetTrigger>
    <SheetContent side="right">
      <SheetHeader>
        <div className="flex flex-wrap items-center gap-2"><StatusBadge status={status} /><span className="text-xs text-slate-500"><span className="capitalize">{apiKey.role}</span> key</span></div>
        <SheetTitle>{apiKey.name}</SheetTitle>
        <SheetDescription>Key details. The secret itself was only shown when the key was created.</SheetDescription>
      </SheetHeader>
      {revokedHere ? <p ref={notice} tabIndex={-1} role="status" className="mb-4 rounded-lg border border-rose-400/30 bg-rose-400/10 px-3 py-2 text-sm text-rose-300 outline-none">Key revoked.</p> : null}
      <dl>
        <Field label="Prefix"><code className="break-all font-mono text-xs text-slate-300">{apiKey.prefix}…</code></Field>
        <Field label="Role"><span className="capitalize">{apiKey.role}</span></Field>
        <Field label="Status"><StatusBadge status={status} /></Field>
        <Field label={`Scopes (${apiKey.scopes.length})`}>
          {apiKey.scopes.length ? <ul className="flex flex-wrap gap-1.5">{apiKey.scopes.map((scope) => <li key={scope} className="rounded bg-white/5 px-2 py-0.5 font-mono text-[11px] text-slate-400">{scope}</li>)}</ul> : <span className="text-slate-500">None</span>}
        </Field>
        <Field label="Created by">{apiKey.created_by ? apiKey.created_by.display_name || "Team member" : <span className="text-slate-500">System</span>}</Field>
        <Field label="Created">{dateTime(apiKey.created_at)}</Field>
        <Field label="Last used">{apiKey.last_used_at ? <>{lastUsed} <span className="text-slate-500">· {dateTime(apiKey.last_used_at)}</span></> : <span className="text-slate-500">Never</span>}</Field>
        {apiKey.revoked_at ? <Field label="Revoked">{dateTime(apiKey.revoked_at)}</Field>
          : <Field label={status.label === "Expired" ? "Expired" : "Expires"}>{apiKey.expires_at ? dateTime(apiKey.expires_at) : <span className="text-slate-500">Never</span>}</Field>}
      </dl>
      {live && canWrite ? <div className="mt-6 rounded-xl border border-white/10 p-4">
        <p className="text-sm font-medium text-slate-200">Revoke this key</p>
        <p className="mb-3 mt-1 text-xs text-slate-500">Scripts and services using it lose access.</p>
        <RevokeKeyButton id={apiKey.id} name={apiKey.name} />
      </div> : null}
    </SheetContent>
  </Sheet>;
}

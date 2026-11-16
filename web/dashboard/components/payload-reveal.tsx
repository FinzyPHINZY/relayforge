"use client";

import { useActionState } from "react";
import { revealEventPayload, type RevealState } from "@/app/events/[id]/actions";

export function PayloadReveal({ eventId }: { eventId: string }) {
  const [state, action, pending] = useActionState(revealEventPayload, {} as RevealState);
  return <div className="rounded-lg border border-amber-400/20 bg-amber-400/5 p-4">
    <p className="text-xs leading-6 text-slate-400">Payloads can contain sensitive data. The reveal is recorded in the audit trail and redacts known sensitive field names and credential patterns. Review any remaining values carefully.</p>
    <form action={action} className="mt-3"><input type="hidden" name="eventId" value={eventId} /><button type="submit" disabled={pending} className="rounded-lg border border-amber-300/50 px-3 py-2 text-xs font-semibold text-amber-200 hover:bg-amber-300/10 disabled:opacity-50">{pending ? "Revealing…" : "Reveal redacted JSON"}</button></form>
    {state.error ? <p role="alert" className="mt-3 text-sm text-red-300">{state.error}</p> : null}
    {state.data !== undefined ? <pre className="mt-4 max-h-96 overflow-auto rounded-lg border border-white/10 bg-slate-950 p-4 text-xs leading-6 text-slate-200">{JSON.stringify(state.data, null, 2)}</pre> : null}
  </div>;
}

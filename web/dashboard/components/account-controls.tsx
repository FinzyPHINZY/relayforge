"use client";

import { useActionState, useState } from "react";
import { LogOut } from "lucide-react";
import { revokeSessionControl, signOutEverywhereControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";

const initial: ControlState = {};

export function RevokeSessionButton({ id, label }: { id: string; label: string }) {
  const [state, action, pending] = useActionState(revokeSessionControl, initial);
  return <form action={action} className="flex flex-col items-start gap-1 sm:items-end"><input type="hidden" name="id" value={id} /><Button variant="outline" size="sm" disabled={pending} aria-label={`Sign out ${label}`}>{pending ? "Signing out…" : "Sign out"}</Button>{state.error ? <p role="alert" className="text-xs text-rose-300">{state.error}</p> : null}</form>;
}

export function SignOutEverywhere() {
  const [confirming, setConfirming] = useState(false);
  const [state, action, pending] = useActionState(signOutEverywhereControl, initial);
  return <div>
    {confirming ? <form action={action} className="flex flex-wrap items-center gap-2"><span className="text-sm text-slate-300">Sign out of every browser, including this one?</span><Button variant="destructive" size="sm" disabled={pending}>{pending ? "Signing out…" : "Sign out everywhere"}</Button><Button type="button" variant="ghost" size="sm" disabled={pending} onClick={() => setConfirming(false)}>Cancel</Button></form>
      : <Button type="button" variant="destructive" onClick={() => setConfirming(true)}><LogOut size={15} aria-hidden="true" />Sign out everywhere</Button>}
    {state.error ? <p role="alert" className="mt-3 text-sm text-rose-300">{state.error}</p> : null}
  </div>;
}

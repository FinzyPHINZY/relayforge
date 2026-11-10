"use client";

import { useActionState } from "react";
import { createEndpoint, type CreateEndpointState } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const initial: CreateEndpointState = {};

export function CreateEndpointForm() {
  const [state, action, pending] = useActionState(createEndpoint, initial);
  return <form action={action} className="grid gap-4 md:grid-cols-2">
    <div className="md:col-span-2"><label htmlFor="url" className="mb-2 block text-sm text-slate-300">HTTPS destination</label><Input id="url" name="url" type="url" placeholder="https://example.com/webhooks" required maxLength={2048} /></div>
    <div><label htmlFor="eventTypes" className="mb-2 block text-sm text-slate-300">Event types</label><Input id="eventTypes" name="eventTypes" placeholder="order.created, order.cancelled" required /></div>
    <div><label htmlFor="description" className="mb-2 block text-sm text-slate-300">Description</label><Input id="description" name="description" placeholder="Orders integration" maxLength={1000} /></div>
    <div className="md:col-span-2"><Button disabled={pending}>{pending ? "Creating…" : "Create endpoint"}</Button></div>
    {state.error ? <p role="alert" className="md:col-span-2 text-sm text-rose-300">{state.error}</p> : null}
    {state.secret ? <div role="status" className="md:col-span-2 rounded-xl border border-amber-400/30 bg-amber-400/10 p-4"><p className="font-medium text-amber-200">Save this signing secret now</p><p className="mt-1 text-xs text-amber-100/70">It will not be shown again. Configure your receiver before publishing events.</p><code className="mt-3 block break-all rounded-lg bg-slate-950 p-3 text-sm text-white">{state.secret}</code></div> : null}
  </form>;
}

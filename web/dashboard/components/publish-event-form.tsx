"use client";

import Link from "next/link";
import { useActionState, useState } from "react";
import { publishEventControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { sampleEvents } from "@/lib/sample-events";

const initial: ControlState = {};

export function PublishEventForm() {
  const [state, action, pending] = useActionState(publishEventControl, initial);
  const [localSchedule, setLocalSchedule] = useState("");
  const [type, setType] = useState<string>(sampleEvents[0].type);
  const [data, setData] = useState<string>(JSON.stringify(sampleEvents[0].data, null, 2));
  const schedule = localSchedule ? new Date(localSchedule) : null;
  return <form action={action} className="grid gap-4 md:grid-cols-2">
    <fieldset className="md:col-span-2"><legend className="mb-2 text-sm font-medium text-slate-300">Start with an example</legend><p className="mb-3 text-xs text-slate-500">Choose a sample, then edit its type or JSON. These examples are never sent automatically.</p><div className="grid gap-2 sm:grid-cols-3">{sampleEvents.map((sample) => {
      const selected = type === sample.type && data === JSON.stringify(sample.data, null, 2);
      return <button key={sample.id} type="button" aria-pressed={selected} onClick={() => { setType(sample.type); setData(JSON.stringify(sample.data, null, 2)); }} className={`rounded-lg border px-3 py-3 text-left transition-colors ${selected ? "border-emerald-400 bg-emerald-400/10" : "border-white/15 bg-slate-950/60 hover:border-emerald-400/40"}`}><span className="block text-sm font-semibold text-slate-100">{sample.label}</span><span className="mt-1 block font-mono text-xs text-emerald-300">{sample.type}</span><span className="mt-2 block text-xs text-slate-500">{sample.description}</span></button>;
    })}</div></fieldset>
    <div><label htmlFor="publishType" className="mb-2 block text-sm text-slate-300">Event type</label><Input id="publishType" name="type" value={type} onChange={(event) => setType(event.target.value)} placeholder="order.created" required maxLength={128} /></div>
    <div><label htmlFor="publishTime" className="mb-2 block text-sm text-slate-300">Deliver later <span className="text-slate-500">(optional)</span></label><Input id="publishTime" type="datetime-local" value={localSchedule} onChange={(event) => setLocalSchedule(event.target.value)} /><input type="hidden" name="deliverAt" value={localSchedule ? schedule && Number.isFinite(schedule.getTime()) ? schedule.toISOString() : "invalid" : ""} /><p className="mt-1 text-xs text-slate-500">Your local time, within 30 days.</p></div>
    <div className="md:col-span-2"><label htmlFor="publishData" className="mb-2 block text-sm text-slate-300">JSON data</label><textarea id="publishData" name="data" rows={7} value={data} onChange={(event) => setData(event.target.value)} required className="w-full rounded-lg border border-white/15 bg-slate-950/60 px-3 py-2 font-mono text-sm text-white focus:border-emerald-400 focus:outline-none" /></div>
    <div className="md:col-span-2"><Button disabled={pending}>{pending ? "Publishing…" : "Publish event"}</Button><p className="mt-2 text-xs text-slate-500">Publishing creates a real event and sends it to matching endpoints. Subscribe your endpoint to the selected event type first.</p>{state.error ? <p role="alert" className="mt-3 text-sm text-rose-300">{state.error}</p> : null}{state.success ? <p role="status" className="mt-3 text-sm text-emerald-300">{state.success} {state.id ? <Link className="underline" href={`/events/${state.id}`}>Inspect event →</Link> : null}</p> : null}</div>
  </form>;
}

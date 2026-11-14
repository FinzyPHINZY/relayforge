"use client";

import Link from "next/link";
import { useActionState, useState } from "react";
import { publishEventControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const initial: ControlState = {};

export function PublishEventForm() {
  const [state, action, pending] = useActionState(publishEventControl, initial);
  const [localSchedule, setLocalSchedule] = useState("");
  const schedule = localSchedule ? new Date(localSchedule) : null;
  return <form action={action} className="grid gap-4 md:grid-cols-2"><div><label htmlFor="publishType" className="mb-2 block text-sm text-slate-300">Event type</label><Input id="publishType" name="type" placeholder="order.created" required maxLength={128} /></div><div><label htmlFor="publishTime" className="mb-2 block text-sm text-slate-300">Deliver later <span className="text-slate-500">(optional)</span></label><Input id="publishTime" type="datetime-local" value={localSchedule} onChange={(event) => setLocalSchedule(event.target.value)} /><input type="hidden" name="deliverAt" value={localSchedule ? schedule && Number.isFinite(schedule.getTime()) ? schedule.toISOString() : "invalid" : ""} /><p className="mt-1 text-xs text-slate-500">Your local time, within 30 days.</p></div><div className="md:col-span-2"><label htmlFor="publishData" className="mb-2 block text-sm text-slate-300">JSON data</label><textarea id="publishData" name="data" rows={5} defaultValue={'{"order_id":"ord_1042"}'} required className="w-full rounded-lg border border-white/15 bg-slate-950/60 px-3 py-2 font-mono text-sm text-white focus:border-emerald-400 focus:outline-none" /></div><div className="md:col-span-2"><Button disabled={pending}>{pending ? "Publishing…" : "Publish event"}</Button>{state.error ? <p role="alert" className="mt-3 text-sm text-rose-300">{state.error}</p> : null}{state.success ? <p role="status" className="mt-3 text-sm text-emerald-300">{state.success} {state.id ? <Link className="underline" href={`/events/${state.id}`}>Inspect event →</Link> : null}</p> : null}</div></form>;
}

"use client";

import { useActionState } from "react";
import { createTeamMemberControl, updateTeamMemberControl, type ControlState } from "@/app/console-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { TeamMember } from "@/lib/types";

const initial: ControlState = {};
const selectClass = "h-10 rounded-lg border border-white/15 bg-slate-950/60 px-3 text-sm text-white focus:border-emerald-400 focus:outline-none";

export function MemberAccessForm({ member }: { member: TeamMember }) {
  const [state, action, pending] = useActionState(updateTeamMemberControl, initial);
  return <form action={action} className="flex flex-wrap items-center gap-2"><input type="hidden" name="id" value={member.id} /><label><span className="sr-only">Role for {member.name}</span><select name="role" defaultValue={member.role} className={selectClass}><option value="admin">Admin</option><option value="operator">Operator</option><option value="viewer">Viewer</option></select></label><label><span className="sr-only">Status for {member.name}</span><select name="status" defaultValue={member.status} className={selectClass}><option value="active">Active</option><option value="suspended">Suspended</option></select></label><Button variant="outline" size="sm" disabled={pending}>{pending ? "Saving…" : "Save"}</Button>{state.error ? <p role="alert" className="w-full text-xs text-rose-300">{state.error}</p> : null}{state.success ? <p role="status" className="w-full text-xs text-emerald-300">{state.success}</p> : null}</form>;
}

export function CreateMemberForm() {
  const [state, action, pending] = useActionState(createTeamMemberControl, initial);
  return <form action={action} className="grid gap-4 md:grid-cols-3"><div><label htmlFor="memberName" className="mb-2 block text-xs text-slate-300">Name</label><Input id="memberName" name="name" required maxLength={120} /></div><div><label htmlFor="memberEmail" className="mb-2 block text-xs text-slate-300">Email</label><Input id="memberEmail" name="email" type="email" required maxLength={254} /></div><div><label htmlFor="memberRole" className="mb-2 block text-xs text-slate-300">Role</label><select id="memberRole" name="role" className={`${selectClass} w-full`}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select></div><div className="md:col-span-3"><Button disabled={pending}>{pending ? "Adding…" : "Create member"}</Button>{state.error ? <p role="alert" className="mt-3 text-sm text-rose-300">{state.error}</p> : null}{state.success ? <p role="status" className="mt-3 text-sm text-emerald-300">{state.success}</p> : null}{state.secret ? <div className="mt-4 rounded-lg border border-amber-400/30 bg-amber-400/10 p-4"><p className="text-sm font-semibold text-amber-200">Copy this member key now</p><code className="mt-2 block break-all text-sm text-white">{state.secret}</code></div> : null}</div></form>;
}

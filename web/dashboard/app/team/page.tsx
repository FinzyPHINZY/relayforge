import { UsersRound } from "lucide-react";
import { CreateMemberForm, MemberAccessForm } from "@/components/team-controls";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { readApi, requireKey } from "@/lib/api";
import { dateTime } from "@/lib/format";
import type { Principal, TeamMember } from "@/lib/types";

export default async function TeamPage() {
  const key = await requireKey();
  const me = await readApi<Principal>(key, "/v1/me");
  const members = me.scopes.includes("team:read") ? await readApi<{ data: TeamMember[] }>(key, "/v1/team/members") : { data: [] };
  const canManage = me.role === "owner" && me.scopes.includes("team:write");
  return <Shell><PageTitle eyebrow="Access" title="Team members" description="Each member has a personal API key and a role checked on every request." />
    <Card><CardHeader><div className="flex items-center gap-3"><UsersRound size={19} className="text-emerald-400" /><h2 className="font-semibold">Current access</h2></div></CardHeader><CardContent><p className="text-sm text-slate-400">Your role: <span className="font-semibold capitalize text-slate-100">{me.role}</span>. {canManage ? "You can add, suspend, and change members." : "Only an owner key with team write scope can change access."}</p></CardContent></Card>
    <Card className="mt-6"><CardHeader><h2 className="font-semibold">Members</h2></CardHeader><CardContent>{members.data.length ? <div className="divide-y divide-white/10">{members.data.map((member) => <div key={member.id} className="flex flex-wrap items-center justify-between gap-4 py-4 first:pt-0 last:pb-0"><div><p className="text-sm font-medium text-slate-100">{member.name} <span className="ml-2 text-xs font-normal text-slate-500">{member.email}</span></p><p className="mt-1 text-xs capitalize text-slate-500">{member.role} · {member.status} · Added {dateTime(member.created_at)}</p></div>{canManage ? <MemberAccessForm member={member} /> : null}</div>)}</div> : <EmptyState title="No team members" description={me.scopes.includes("team:read") ? "Add a member to issue a personal key." : "This key does not have team read access."} />}</CardContent></Card>
    {canManage ? <Card className="mt-6"><CardHeader><h2 className="font-semibold">Add member</h2><p className="mt-1 text-xs text-slate-500">The personal API key is revealed once. Share it through a secure channel.</p></CardHeader><CardContent><CreateMemberForm /></CardContent></Card> : null}
  </Shell>;
}

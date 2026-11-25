import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { Laptop, ShieldCheck, UserRound } from "lucide-react";
import { RevokeSessionButton, SignOutEverywhere } from "@/components/account-controls";
import { EmptyState, PageTitle, Shell } from "@/components/shell";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { fetchAccount, type Account } from "@/lib/account";
import { ApiError, readApi, requireSession } from "@/lib/api";
import { deviceLabel } from "@/lib/device-label";
import { dateTime } from "@/lib/format";
import type { AccountSession } from "@/lib/types";

export const metadata: Metadata = { title: "Account · RelayForge" };

// only https avatars are shown; anything else falls back to an initial.
function avatarUrl(value: string | null | undefined): string | null {
  if (!value) return null;
  try { return new URL(value).protocol === "https:" ? value : null; } catch { return null; }
}

export default async function AccountPage() {
  const session = await requireSession();
  if (session.kind !== "user") redirect("/dashboard");
  const personal = { kind: "user", token: session.token, org: null } as const;
  let account: Account;
  try {
    account = await fetchAccount(session.token);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    throw error;
  }
  const { data } = await readApi<{ data: AccountSession[] }>(personal, "/v1/account/sessions");
  const sessions = [...data].sort((a, b) => Number(b.current) - Number(a.current) || b.last_seen_at.localeCompare(a.last_seen_at));
  const { user } = account;
  const name = user.display_name?.trim() || user.email?.split("@")[0] || "RelayForge user";
  const avatar = avatarUrl(user.avatar_url);
  const others = sessions.filter((item) => !item.current).length;

  return <Shell><PageTitle eyebrow="Account" title="Your account" description="Your profile and the browsers signed in as you." />
    <Card><CardHeader><div className="flex items-center gap-3"><UserRound size={19} className="text-emerald-400" aria-hidden="true" /><h2 className="font-semibold">Profile</h2></div></CardHeader><CardContent>
      <div className="flex items-center gap-4">
        {avatar ? <img src={avatar} alt="" width={56} height={56} referrerPolicy="no-referrer" className="size-14 shrink-0 rounded-full border border-white/10 object-cover" /> : <span aria-hidden="true" className="flex size-14 shrink-0 items-center justify-center rounded-full bg-[#b7f36d] text-xl font-bold text-[#173322]">{[...name][0]?.toUpperCase()}</span>}
        <div className="min-w-0"><p className="truncate text-lg font-semibold text-slate-100">{name}</p><p className="truncate text-sm text-slate-400">{user.email ?? "No email address"}</p></div>
      </div>
      <dl className="mt-5 grid gap-4 border-t border-white/10 pt-5 text-sm sm:grid-cols-3">
        <div><dt className="text-xs text-slate-500">Name</dt><dd className="mt-1 break-words text-slate-200">{user.display_name?.trim() || <span className="text-slate-500">Not set</span>}</dd></div>
        <div><dt className="text-xs text-slate-500">Email</dt><dd className="mt-1 break-all text-slate-200">{user.email ?? <span className="text-slate-500">Not set</span>}</dd></div>
        <div><dt className="text-xs text-slate-500">Workspaces</dt><dd className="mt-1 text-slate-200">{account.workspaces.length}</dd></div>
      </dl>
    </CardContent></Card>

    <Card className="mt-6"><CardHeader><div className="flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-3"><Laptop size={19} className="text-emerald-400" aria-hidden="true" /><h2 className="font-semibold">Sessions</h2></div><span className="text-xs text-slate-500">{sessions.length} signed in</span></div></CardHeader><CardContent>
      {sessions.length ? <ul className="divide-y divide-white/10">{sessions.map((item) => {
        const label = deviceLabel(item.user_agent);
        return <li key={item.id} className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="text-sm font-medium text-slate-100" title={item.user_agent || "No user agent recorded"}>{label}</p>{item.current ? <Badge tone="good" className="normal-case">This browser</Badge> : null}</div><p className="mt-1 text-xs text-slate-500">Last seen {dateTime(item.last_seen_at)} · Signed in {dateTime(item.created_at)}</p></div>
          {item.current ? null : <RevokeSessionButton id={item.id} label={label} />}
        </li>;
      })}</ul> : <EmptyState title="No active sessions" description="Sessions appear here after you sign in." />}
    </CardContent></Card>

    <Card className="mt-6"><CardHeader><div className="flex items-center gap-3"><ShieldCheck size={19} className="text-emerald-400" aria-hidden="true" /><h2 className="font-semibold">Sign out everywhere</h2></div><p className="mt-1 text-xs text-slate-500">{others === 0 ? "Only this browser is signed in. Signing out everywhere ends this session too." : `Ends ${others === 1 ? "the other session" : `all ${others} other sessions`}, then this one. Use it if a device was lost or shared.`}</p></CardHeader><CardContent><SignOutEverywhere /></CardContent></Card>
  </Shell>;
}

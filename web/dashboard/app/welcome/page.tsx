import { ArrowRight, Building2, LogOut } from "lucide-react";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { signOut } from "@/app/actions";
import { createFirstWorkspace } from "@/app/auth-actions";
import { AuthShell } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { fetchAccount, type Account } from "@/lib/account";
import { ApiError, requireSession } from "@/lib/api";

export const metadata: Metadata = { title: "Name your workspace · RelayForge" };

const messages: Record<string, string> = {
  limit: "You've reached the 3-workspace limit on the free plan.",
  name: "Use a workspace name between 1 and 80 characters.",
  email: "Your account needs an email address before it can create a workspace.",
  unavailable: "RelayForge is unavailable. Try again in a moment.",
};

export default async function WelcomePage({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const session = await requireSession();
  if (session.kind !== "user") redirect("/dashboard");
  const { error } = await searchParams;
  let account: Account | null = null;
  try {
    account = await fetchAccount(session.token);
  } catch (caught) {
    if (caught instanceof ApiError && caught.status === 401) redirect("/login?error=expired");
  }
  const message = error && Object.hasOwn(messages, error) ? messages[error] : !account ? messages.unavailable : null;
  const existing = (account?.workspaces.length ?? 0) > 0;
  const email = account?.user.email;
  const footer = <><span>{email ? <>Signed in as <strong className="login-identity">{email}</strong></> : "Signed in"}</span><form action={signOut}><button type="submit" className="login-link-button"><LogOut size={14} aria-hidden="true" /> Sign out</button></form></>;
  return <AuthShell labelledBy="welcome-heading" back={existing ? { href: "/dashboard", label: "Back to dashboard" } : { href: "/", label: "Back to RelayForge" }} footer={footer}>
    <div className="login-form-icon"><Building2 size={24} aria-hidden="true" /></div><span className="login-kicker">{existing ? "NEW WORKSPACE" : "FIRST WORKSPACE"}</span><h2 id="welcome-heading">Name your workspace</h2><p>A workspace holds your endpoints, events and team. Use your company or project name; you can invite teammates later.</p>
    <form action={createFirstWorkspace} className="login-form"><label htmlFor="name">Workspace name</label><Input id="name" name="name" type="text" autoComplete="organization" maxLength={80} placeholder="Acme Inc." required autoFocus /><Button className="login-submit">Create workspace <ArrowRight size={17} aria-hidden="true" /></Button></form>
    {message ? <p role="alert" className="login-error">{message}</p> : null}
  </AuthShell>;
}

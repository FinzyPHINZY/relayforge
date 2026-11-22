import { ArrowRight, LockKeyhole, MailOpen, TriangleAlert } from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";
import { confirmMagicLink } from "@/app/auth-actions";
import { AuthShell } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { magicLinkToken } from "@/lib/magic-link";

// The token sits in the URL, so no outgoing request may carry it as a Referer.
export const metadata: Metadata = { title: "Sign in · RelayForge", referrer: "no-referrer", robots: { index: false, follow: false } };

// GET only renders a button: mail-client link scanners that fetch the URL must
// not consume the single-use link. Only the POST below does.
export default async function EmailLinkPage({ searchParams }: { searchParams: Promise<{ token?: string | string[] }> }) {
  const { token } = await searchParams;
  const valid = typeof token === "string" && magicLinkToken.test(token);
  if (!valid) {
    return <AuthShell labelledBy="email-link-heading" back={{ href: "/login", label: "Back to sign in" }}>
      <div className="login-form-icon login-form-icon-warn"><TriangleAlert size={24} aria-hidden="true" /></div><span className="login-kicker">EMAIL LINK</span><h2 id="email-link-heading">This link can&apos;t be used</h2>
      <p role="alert" className="login-error">That sign-in link has expired or was already used. Request a new one.</p>
      <p className="login-alt"><Link href="/login">Request a new link <ArrowRight size={14} aria-hidden="true" /></Link></p>
    </AuthShell>;
  }
  return <AuthShell labelledBy="email-link-heading" back={{ href: "/login", label: "Back to sign in" }}>
    <div className="login-form-icon"><MailOpen size={24} aria-hidden="true" /></div><span className="login-kicker">EMAIL LINK</span><h2 id="email-link-heading">Sign in to RelayForge</h2><p>Confirm to finish signing in. The link works once.</p>
    <form action={confirmMagicLink} className="login-form"><input type="hidden" name="token" value={token} /><Button className="login-submit">Continue to RelayForge <ArrowRight size={17} aria-hidden="true" /></Button></form>
    <p className="login-security"><LockKeyhole size={15} aria-hidden="true" /> Didn&apos;t request this? Close the page. Nothing happens until you continue.</p>
  </AuthShell>;
}

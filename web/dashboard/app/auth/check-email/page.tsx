import { ArrowLeft, MailCheck } from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";
import { AuthShell } from "@/components/auth-shell";

export const metadata: Metadata = { title: "Check your email · RelayForge" };

export default function CheckEmailPage() {
  return <AuthShell labelledBy="check-email-heading" back={{ href: "/login", label: "Back to sign in" }}>
    <div className="login-form-icon"><MailCheck size={24} aria-hidden="true" /></div><span className="login-kicker">EMAIL LINK</span><h2 id="check-email-heading">Check your email</h2><p>If that address can sign in, we&apos;ve sent a link. It expires in 15 minutes.</p>
    <p className="login-note">Open the link on this device to finish signing in. Nothing arrived? Check spam, or request a new link.</p>
    <p className="login-alt"><Link href="/login"><ArrowLeft size={14} aria-hidden="true" /> Back to sign in</Link></p>
  </AuthShell>;
}

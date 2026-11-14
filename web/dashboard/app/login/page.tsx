import { ArrowLeft, ArrowRight, Check, Clock3, KeyRound, LockKeyhole, Webhook } from "lucide-react";
import Link from "next/link";
import { signIn } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import "./login.css";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const { error } = await searchParams;
  const message = error === "invalid" ? "That API key is invalid or lacks the metrics:read scope." : error === "expired" ? "Your API key is no longer accepted. Sign in again." : error === "unavailable" ? "RelayForge is unavailable. Try again in a moment." : null;
  return <main className="console-theme login-page">
    <div className="login-frame">
      <section className="login-story" aria-label="RelayForge delivery flow">
        <Link href="/" className="login-brand"><span className="login-brand-mark"><Webhook size={21} aria-hidden="true" /></span><span>relayforge<span className="login-brand-period">.</span></span></Link>
        <div className="login-story-content"><span className="login-kicker">THE OPERATOR WORKSPACE</span><h1>Every delivery.<br /><em>Accounted for.</em></h1><p>See the event, follow every attempt, and act when a receiver needs attention.</p>
          <div className="login-flow" aria-hidden="true"><div><span className="login-flow-icon"><Webhook size={18} /></span><span><small>EVENT ACCEPTED</small><strong>order.created</strong></span><Check size={18} /></div><div><span className="login-flow-icon"><Clock3 size={18} /></span><span><small>DELIVERY IN PROGRESS</small><strong>Retry scheduled</strong></span><span className="login-flow-pulse" /></div><div><span className="login-flow-icon"><Check size={18} /></span><span><small>RECEIVER ACKNOWLEDGED</small><strong>HTTP 204 · Delivered</strong></span><Check size={18} /></div></div>
        </div>
        <p className="login-story-footer">Reliable webhook delivery, with the full history in view.</p>
      </section>
      <section className="login-form-panel" aria-labelledby="login-heading"><div className="login-form-top"><Link href="/" className="login-back"><ArrowLeft size={15} aria-hidden="true" /> Back to RelayForge</Link><span>SECURE ACCESS</span></div>
        <div className="login-form-content"><div className="login-form-icon"><KeyRound size={24} aria-hidden="true" /></div><span className="login-kicker">WELCOME BACK</span><h2 id="login-heading">Sign in to your workspace</h2><p>Use your scoped RelayForge API key to inspect deliveries and manage endpoints.</p>
          <form action={signIn} className="login-form"><label htmlFor="apiKey">API key</label><Input id="apiKey" name="apiKey" type="password" autoComplete="off" placeholder="whk_…" required /><Button className="login-submit">Open console <ArrowRight size={17} aria-hidden="true" /></Button></form>
          {message ? <p role="alert" className="login-error">{message}</p> : null}
          <p className="login-security"><LockKeyhole size={15} aria-hidden="true" /> Your key is encrypted in an HTTP-only session cookie. Browser code never calls the API with it.</p>
        </div>
        <div className="login-form-bottom"><span>Need a quick tour first?</span><Link href="/demo">Explore the guided demo <ArrowRight size={14} aria-hidden="true" /></Link></div>
      </section>
    </div>
  </main>;
}

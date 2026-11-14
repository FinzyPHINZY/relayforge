import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft, ArrowUpRight, Webhook } from "lucide-react";
import { GuidedDemo } from "@/components/guided-demo";
import "../landing.css";
import "./demo.css";

export const metadata: Metadata = {
  title: "Guided demo | RelayForge",
  description: "Explore how RelayForge accepts an event, retries a failed webhook, and records the outcome. No account or API key needed.",
};

export default function DemoPage() {
  return <div className="landing demo-page">
    <header className="landing-header"><div className="landing-container landing-header-inner">
      <Link href="/" className="landing-brand" aria-label="RelayForge home"><span className="landing-brand-mark"><Webhook size={21} aria-hidden="true" /></span><span>relayforge<span className="landing-brand-period">.</span></span></Link>
      <Link href="/dashboard" className="landing-header-cta">Operator console <ArrowUpRight size={16} aria-hidden="true" /></Link>
    </div></header>
    <main className="landing-container demo-main">
      <Link href="/" className="demo-back"><ArrowLeft size={15} aria-hidden="true" /> Back to RelayForge</Link>
      <div className="demo-intro"><span className="landing-kicker">PUBLIC GUIDED DEMO</span><h1>Watch a webhook<br /><span>make it through.</span></h1><p>Follow an event from durable acceptance to a receiver that fails once, then succeeds. Every screen below is an illustration of RelayForge’s behavior. It makes no live API calls.</p></div>
      <GuidedDemo />
      <div className="demo-after"><div><span className="landing-kicker">WANT TO GO DEEPER?</span><h2>See the system behind the story.</h2><p>The README shows the real Compose demo, failure tests, wire contract, and benchmark commands you can reproduce locally.</p></div><a href="https://github.com/FinzyPHINZY/relayforge#run-locally" target="_blank" rel="noopener noreferrer" className="landing-button landing-button-secondary">Explore the source <ArrowUpRight size={17} aria-hidden="true" /></a></div>
    </main>
  </div>;
}

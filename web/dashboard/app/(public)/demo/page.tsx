import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft, ArrowUpRight } from "lucide-react";
import { GuidedDemo } from "@/components/guided-demo";
import "./demo.css";

export const metadata: Metadata = {
  title: "Guided demo | RelayForge",
  description: "Explore how RelayForge accepts an event, retries a failed webhook, and records the outcome. No account or API key needed.",
};

export default function DemoPage() {
  return <div className="landing demo-page">
    <main className="landing-container demo-main">
      <Link href="/" className="demo-back"><ArrowLeft size={15} aria-hidden="true" /> Back to RelayForge</Link>
      <div className="demo-intro"><span className="landing-kicker">PUBLIC GUIDED DEMO</span><h1>Watch a webhook<br /><span>make it through.</span></h1><p>Follow an event from durable acceptance to a receiver that fails once, then succeeds. Every screen below is an illustration of RelayForge’s behavior. It makes no live API calls.</p></div>
      <GuidedDemo />
      <div className="demo-after"><div><span className="landing-kicker">WANT TO GO DEEPER?</span><h2>See the system behind the story.</h2><p>Follow the durable delivery path, the crash window that can produce duplicates, and the measured 10,000-event run.</p></div><Link href="/how-it-works" className="landing-button landing-button-secondary">How RelayForge works <ArrowUpRight size={17} aria-hidden="true" /></Link></div>
    </main>
  </div>;
}

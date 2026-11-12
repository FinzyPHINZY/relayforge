import type { Metadata } from "next";
import Link from "next/link";
import {
  Activity,
  ArrowDownRight,
  ArrowRight,
  ArrowUpRight,
  Check,
  CheckCheck,
  Clock3,
  Fingerprint,
  GitBranch,
  LockKeyhole,
  RadioTower,
  RotateCcw,
  ShieldCheck,
  Terminal,
  Webhook,
} from "lucide-react";
import "./landing.css";

export const metadata: Metadata = {
  title: "RelayForge — Webhooks built to make it through",
  description:
    "Durable event acceptance, signed webhook delivery, retries, and a clear path to recovery. Explore RelayForge's reliable webhook infrastructure.",
};

const features = [
  {
    number: "01",
    icon: GitBranch,
    title: "Accepted means stored.",
    body: "Events and their outbox records commit together in PostgreSQL before the API acknowledges them.",
    tag: "Transactional outbox",
  },
  {
    number: "02",
    icon: RotateCcw,
    title: "Failure has a next step.",
    body: "Bounded retries, lease recovery, circuit breakers, and manual replay keep failed deliveries visible and recoverable.",
    tag: "Built for recovery",
  },
  {
    number: "03",
    icon: Fingerprint,
    title: "Every request is verifiable.",
    body: "Webhook bodies are signed with HMAC. Receivers can verify the signature and deduplicate by a stable delivery ID.",
    tag: "Signed delivery",
  },
];

function Brand({ light = false }: { light?: boolean }) {
  return (
    <Link href="/" className={`landing-brand${light ? " landing-brand-light" : ""}`} aria-label="RelayForge home">
      <span className="landing-brand-mark"><Webhook size={21} strokeWidth={2.2} aria-hidden="true" /></span>
      <span>relayforge<span className="landing-brand-period">.</span></span>
    </Link>
  );
}

export default function LandingPage() {
  return (
    <div className="landing">
      <header className="landing-header">
        <div className="landing-container landing-header-inner">
          <Brand />
          <nav className="landing-nav" aria-label="Main navigation">
            <a href="#how-it-works">How it works</a>
            <a href="#capabilities">Capabilities</a>
            <a href="https://github.com/FinzyPHINZY/relayforge" target="_blank" rel="noopener noreferrer">GitHub <ArrowUpRight size={14} aria-hidden="true" /></a>
          </nav>
          <Link href="/dashboard" className="landing-header-cta">Open console <ArrowUpRight size={16} aria-hidden="true" /></Link>
        </div>
      </header>

      <main>
        <section className="landing-hero landing-container" aria-labelledby="landing-heading">
          <div className="landing-hero-copy">
            <div className="landing-eyebrow"><span className="landing-eyebrow-dot" /> Reliable webhook infrastructure</div>
            <h1 id="landing-heading">Your event happened.<br /><span>Make sure the message gets through.</span></h1>
            <p>RelayForge turns accepted events into signed, observable webhook deliveries. When a receiver goes down, you have retries, recovery, and the full story of what happened.</p>
            <div className="landing-hero-actions">
              <Link href="/dashboard" className="landing-button landing-button-primary">Open the console <ArrowUpRight size={18} aria-hidden="true" /></Link>
              <a href="#how-it-works" className="landing-button landing-button-secondary">See how it works <ArrowDownRight size={17} aria-hidden="true" /></a>
            </div>
            <div className="landing-hero-note"><span className="landing-note-line" /> Built around durable state, at-least-once delivery, and a clear recovery path.</div>
          </div>

          <div className="landing-visual" aria-label="Illustration of an event moving through RelayForge to a webhook endpoint" role="img">
            <div className="landing-visual-grid" />
            <div className="landing-visual-top"><span><span className="landing-live-dot" /> DELIVERY PIPELINE</span><span>LIVE FLOW <Activity size={13} aria-hidden="true" /></span></div>
            <div className="landing-flow">
              <div className="landing-flow-row landing-flow-row-first">
                <span className="landing-flow-icon landing-flow-icon-a"><Terminal size={19} aria-hidden="true" /></span>
                <div><span className="landing-flow-label">EVENT RECEIVED</span><strong>order.created</strong><small>Idempotency key · order_81f2</small></div>
                <span className="landing-flow-check"><Check size={15} aria-hidden="true" /></span>
              </div>
              <div className="landing-flow-connector"><span /><span>committed to outbox</span></div>
              <div className="landing-flow-row landing-flow-row-middle">
                <span className="landing-flow-icon landing-flow-icon-b"><GitBranch size={19} aria-hidden="true" /></span>
                <div><span className="landing-flow-label">RELAYFORGE</span><strong>Safely in motion</strong><small>Durable queue · bounded retries</small></div>
                <span className="landing-flow-pulse"><span /></span>
              </div>
              <div className="landing-flow-connector"><span /><span>signed &amp; sent</span></div>
              <div className="landing-flow-row landing-flow-row-last">
                <span className="landing-flow-icon landing-flow-icon-c"><Webhook size={19} aria-hidden="true" /></span>
                <div><span className="landing-flow-label">YOUR ENDPOINT</span><strong>Delivery acknowledged</strong><small>HTTP 204 · signature verified</small></div>
                <span className="landing-flow-success"><CheckCheck size={17} aria-hidden="true" /></span>
              </div>
            </div>
            <div className="landing-visual-bottom"><span><span className="landing-tiny-square" /> Every attempt has a record</span><span>01 / 01</span></div>
            <div className="landing-float-card"><ShieldCheck size={19} aria-hidden="true" /><span><strong>Signed request</strong><small>Integrity built in</small></span></div>
          </div>
        </section>

        <div className="landing-proof-wrap"><div className="landing-container landing-proof">
          <span className="landing-proof-intro">THE FOUNDATION</span>
          <span><LockKeyhole size={17} aria-hidden="true" /> Durable by design</span>
          <span><Clock3 size={17} aria-hidden="true" /> Retry with intent</span>
          <span><Activity size={17} aria-hidden="true" /> Visible end to end</span>
        </div></div>

        <section id="how-it-works" className="landing-section landing-container" aria-labelledby="how-heading">
          <div className="landing-section-head">
            <div><span className="landing-kicker">THE DELIVERY PATH</span><h2 id="how-heading">From event to endpoint.<br /><em>Without the guesswork.</em></h2></div>
            <p>Simple to publish. Deliberate about everything that happens after.</p>
          </div>
          <div className="landing-steps">
            <div className="landing-step"><span className="landing-step-index">01 / PUBLISH</span><span className="landing-step-icon"><Terminal size={23} aria-hidden="true" /></span><h3>Send your event</h3><p>Publish once with an idempotency key. RelayForge returns after the event and its outbox entry commit together.</p></div>
            <div className="landing-step"><span className="landing-step-index">02 / DELIVER</span><span className="landing-step-icon"><RadioTower size={23} aria-hidden="true" /></span><h3>We handle the handoff</h3><p>Matching endpoints receive signed HTTP requests. Workers track attempts and retry transient failures.</p></div>
            <div className="landing-step"><span className="landing-step-index">03 / RECOVER</span><span className="landing-step-icon"><Activity size={23} aria-hidden="true" /></span><h3>See what happened</h3><p>Inspect every attempt, review exhausted deliveries, and replay when the receiver is ready.</p></div>
          </div>
        </section>

        <section id="capabilities" className="landing-capabilities" aria-labelledby="capabilities-heading">
          <div className="landing-container">
            <div className="landing-section-head landing-section-head-dark">
              <div><span className="landing-kicker">BUILT FOR THE MESSY PART</span><h2 id="capabilities-heading">Reliability is a series<br />of <em>good decisions.</em></h2></div>
              <p>The details that matter when the network, a worker, or a receiver does not cooperate.</p>
            </div>
            <div className="landing-feature-grid">
              {features.map(({ number, icon: Icon, title, body, tag }) => <article className="landing-feature" key={number}>
                <div className="landing-feature-top"><span>{number} / 03</span><Icon size={22} strokeWidth={1.7} aria-hidden="true" /></div>
                <h3>{title}</h3><p>{body}</p><span className="landing-feature-tag"><Check size={13} aria-hidden="true" /> {tag}</span>
              </article>)}
            </div>
            <div className="landing-honest-note"><span className="landing-honest-icon">i</span><p><strong>A clear guarantee.</strong> HTTP webhook delivery is at least once. A receiver may see a duplicate, so it should deduplicate using the stable <code>webhook-id</code>.</p></div>
          </div>
        </section>

        <section className="landing-final landing-container" aria-labelledby="final-heading">
          <div className="landing-final-panel"><div className="landing-final-glow" /><span className="landing-kicker">READY TO EXPLORE?</span><h2 id="final-heading">Build with confidence.<br /><em>Operate with clarity.</em></h2><p>See the delivery pipeline in action, inspect its behavior, and make every webhook accountable.</p><div className="landing-final-actions"><Link href="/dashboard" className="landing-button landing-button-primary">Open the console <ArrowUpRight size={18} aria-hidden="true" /></Link><a href="https://github.com/FinzyPHINZY/relayforge" target="_blank" rel="noopener noreferrer" className="landing-final-link">View on GitHub <ArrowRight size={17} aria-hidden="true" /></a></div></div>
        </section>
      </main>

      <footer className="landing-footer"><div className="landing-container landing-footer-inner"><Brand light /><span>Reliable delivery, made visible.</span><a href="https://github.com/FinzyPHINZY/relayforge" target="_blank" rel="noopener noreferrer">Source code <ArrowUpRight size={14} aria-hidden="true" /></a></div></footer>
    </div>
  );
}

import type { Metadata } from "next";
import Link from "next/link";
import { MeasuredCount } from "@/components/measured-count";
import { LandingPipeline } from "@/components/landing-pipeline";
import {
  Activity,
  ArrowDownRight,
  ArrowRight,
  ArrowUpRight,
  Check,
  Clock3,
  CalendarClock,
  Building2,
  CreditCard,
  Fingerprint,
  GitBranch,
  LockKeyhole,
  RadioTower,
  RotateCcw,
  ShieldCheck,
  SlidersHorizontal,
  Database,
  ListFilter,
  ScanSearch,
  ShoppingBag,
  Siren,
  Terminal,
  Webhook,
} from "lucide-react";

export const metadata: Metadata = {
  title: "RelayForge | Webhooks built to make it through",
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
    icon: Building2,
    title: "Every tenant has its own lane.",
    body: "Organization-scoped keys, endpoints, events, deliveries, and metrics keep customer operations separate.",
    tag: "Tenant isolation",
  },
  {
    number: "03",
    icon: RotateCcw,
    title: "Failure has a next step.",
    body: "Bounded retries, lease recovery, circuit breakers, and manual replay keep failed deliveries visible and recoverable.",
    tag: "Built for recovery",
  },
  {
    number: "04",
    icon: Fingerprint,
    title: "Every request is verifiable.",
    body: "Webhook bodies are signed with HMAC. Receivers can verify the signature and deduplicate by a stable delivery ID.",
    tag: "Signed delivery",
  },
  {
    number: "05",
    icon: SlidersHorizontal,
    title: "Control the destination.",
    body: "Manage subscriptions, verification, signing-secret rotation, timeout, retry, rate, and concurrency policies per endpoint.",
    tag: "Endpoint controls",
  },
  {
    number: "06",
    icon: ScanSearch,
    title: "Investigate the whole run.",
    body: "See attempts, response codes, exact retry schedules, exhausted reviews, and replay history in one delivery timeline.",
    tag: "Delivery inspector",
  },
  {
    number: "07",
    icon: ShieldCheck,
    title: "Block unsafe destinations.",
    body: "Public-IP checks, DNS revalidation, pinned connections, and blocked redirects protect outbound delivery.",
    tag: "SSRF protection",
  },
  {
    number: "08",
    icon: Activity,
    title: "See the system work.",
    body: "OpenTelemetry traces and metrics, structured logs, and an operator monitor surface failures and queue health.",
    tag: "Observability",
  },
  {
    number: "09",
    icon: Database,
    title: "Account for every event.",
    body: "Durable outbox and delivery records support reconciliation even after timeouts, worker crashes, or lost acknowledgments.",
    tag: "Auditable outcomes",
  },
];

const useCases = [
  { icon: ShoppingBag, title: "Order fulfillment", role: "ECOMMERCE DEVELOPER", story: "As an ecommerce developer, I want order.created events to reach fulfillment after an outage, so orders keep moving." },
  { icon: CreditCard, title: "Payment processing", role: "PAYMENT INTEGRATOR", story: "As a payment integrator, I want signed deliveries with a stable webhook ID, so my receiver can avoid processing a payment twice." },
  { icon: Building2, title: "Customer isolation", role: "PLATFORM OPERATOR", story: "As a platform operator, I want each customer's events and endpoints kept separate, so one account cannot see or affect another." },
  { icon: Siren, title: "Incident recovery", role: "ON-CALL ENGINEER", story: "As an on-call engineer, I want attempt history, review notes, and replay, so I can recover a failed delivery with a clear record." },
  { icon: CalendarClock, title: "Scheduled delivery", role: "PRODUCT DEVELOPER", story: "As a product developer, I want to publish an event for later, so a customer's webhook arrives when the planned action is due." },
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
      <main>
        <section className="landing-hero landing-container" aria-labelledby="landing-heading">
          <div className="landing-hero-copy">
            <div className="landing-eyebrow"><span className="landing-eyebrow-dot" /> Reliable webhook infrastructure</div>
            <h1 id="landing-heading">Your event happened.<br /><span>Make sure the message gets through.</span></h1>
            <p>RelayForge turns accepted events into signed, observable webhook deliveries. When a receiver goes down, you have retries, recovery, and the full story of what happened.</p>
            <div className="landing-hero-actions">
              <Link href="/demo" className="landing-button landing-button-primary">Explore the guided demo <ArrowUpRight size={18} aria-hidden="true" /></Link>
              <Link href="/dashboard" className="landing-button landing-button-secondary">Operator console <ArrowDownRight size={17} aria-hidden="true" /></Link>
            </div>
            <div className="landing-hero-note"><span className="landing-note-line" /> Built around durable state, at-least-once delivery, and a clear recovery path.</div>
          </div>

          <LandingPipeline />
        </section>

        <div className="landing-proof-wrap"><div className="landing-container landing-proof">
          <span className="landing-proof-intro">THE FOUNDATION</span>
          <span><LockKeyhole size={17} aria-hidden="true" /> Durable by design</span>
          <span><Clock3 size={17} aria-hidden="true" /> Retry with intent</span>
          <span><Activity size={17} aria-hidden="true" /> Visible end to end</span>
        </div></div>

        <section className="landing-scale" aria-labelledby="scale-heading">
          <div className="landing-container landing-scale-inner">
            <div className="landing-scale-copy">
              <span className="landing-kicker">TESTED UNDER FAILURE</span>
              <h2 id="scale-heading">10,000 events.<br /><em>Every one accounted for.</em></h2>
              <p>In a measured two-tenant run, RelayForge accepted 10,000 events, dispatched 10,000 outbox records, and reached a terminal outcome for every delivery while a worker crashed and the receiver injected timeouts and HTTP failures.</p>
              <Link href="/how-it-works#proof" scroll={false} className="landing-scale-link">Explore the measured proof <ArrowRight size={16} aria-hidden="true" /></Link>
            </div>
            <div className="landing-scale-results" aria-label="Measured reliability test results">
              <div className="landing-scale-primary"><span>ACCEPTED &amp; ACCOUNTED FOR</span><strong><MeasuredCount /> <small>/ 10,000</small></strong><p>Events, outbox records, and terminal deliveries reconciled.</p></div>
              <div className="landing-scale-secondary"><div><strong>9,995</strong><span>succeeded</span></div><div><strong>5</strong><span>exhausted after injected HTTP 400</span></div><div><strong>11</strong><span>duplicate receipts observed</span></div></div>
            </div>
            <p className="landing-scale-footnote">One measured Testcontainers run on an 8-CPU host, Go 1.25.0, PostgreSQL 17 · 32 workers · 2m 48s total. At-least-once delivery can produce duplicates; receivers should deduplicate by <code>webhook-id</code>.</p>
          </div>
        </section>

        <section id="how-it-works" className="landing-section landing-container" aria-labelledby="how-heading">
          <div className="landing-section-head">
            <div><span className="landing-kicker">THE DELIVERY PATH</span><h2 id="how-heading">From event to endpoint.<br /><em>Without the guesswork.</em></h2></div>
            <p>Simple to publish. Deliberate about everything that happens after.</p>
          </div>
          <div className="landing-steps">
            <div className="landing-step"><span className="landing-step-index">01 / PUBLISH</span><span className="landing-step-icon"><Terminal size={23} aria-hidden="true" /></span><h3>Send your event</h3><p>Publish once with an idempotency key. RelayForge returns after the event and its outbox entry commit together.</p><Link href="/docs#publish" scroll={false} className="landing-step-link">Publish with the Go or TypeScript SDK <ArrowRight size={14} aria-hidden="true" /></Link></div>
            <div className="landing-step"><span className="landing-step-index">02 / DELIVER</span><span className="landing-step-icon"><RadioTower size={23} aria-hidden="true" /></span><h3>We handle the handoff</h3><p>Matching endpoints receive signed HTTP requests. Workers track attempts and retry transient failures.</p></div>
            <div className="landing-step"><span className="landing-step-index">03 / RECOVER</span><span className="landing-step-icon"><Activity size={23} aria-hidden="true" /></span><h3>See what happened</h3><p>Inspect every attempt, review exhausted deliveries, and replay when the receiver is ready.</p></div>
          </div>
          <Link href="/how-it-works" className="landing-scale-link">See the complete delivery path <ArrowRight size={16} aria-hidden="true" /></Link>
        </section>

        <section id="use-cases" className="landing-section landing-use-cases" aria-labelledby="use-cases-heading">
          <div className="landing-container">
            <div className="landing-section-head"><div><span className="landing-kicker">WHERE RELAYFORGE FITS</span><h2 id="use-cases-heading">Real workflows.<br /><em>Reliable handoffs.</em></h2></div><p>Five ways teams use the same delivery foundation.</p></div>
            <div className="landing-use-case-grid">{useCases.map(({ icon: Icon, title, role, story }, index) => <article className="landing-use-case" key={title}><div className="landing-use-case-top"><span className="landing-use-case-icon"><Icon size={21} strokeWidth={1.8} aria-hidden="true" /></span><span>{String(index + 1).padStart(2, "0")} / 05</span></div><div><span className="landing-use-case-role">{role}</span><h3>{title}</h3><p>“{story}”</p></div></article>)}</div>
            <div className="landing-use-case-next"><span>See these delivery states in action.</span><Link href="/demo">Walk through the guided demo <ArrowRight size={16} aria-hidden="true" /></Link></div>
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
                <div className="landing-feature-top"><span>{number} / {String(features.length).padStart(2, "0")}</span><Icon size={22} strokeWidth={1.7} aria-hidden="true" /></div>
                <h3>{title}</h3><p>{body}</p><span className="landing-feature-tag"><Check size={13} aria-hidden="true" /> {tag}</span>
              </article>)}
            </div>
            <div className="landing-console-guide">
              <div><span className="landing-kicker">THE OPERATOR CONSOLE</span><h3>Four views. One delivery story.</h3><p>Follow aggregate health, find an event, configure the endpoint, and inspect the exact delivery run.</p></div>
              <ol>
                <li><Activity size={18} aria-hidden="true" /><span><strong>Overview</strong><small>Success, failures, retries, throughput, and latency.</small></span></li>
                <li><ListFilter size={18} aria-hidden="true" /><span><strong>Event explorer</strong><small>Search, filter, and inspect event history.</small></span></li>
                <li><SlidersHorizontal size={18} aria-hidden="true" /><span><strong>Endpoint management</strong><small>Subscriptions, secrets, and delivery policies.</small></span></li>
                <li><ScanSearch size={18} aria-hidden="true" /><span><strong>Delivery inspector</strong><small>Attempts, retry timeline, review, and replay.</small></span></li>
              </ol>
            </div>
            <div className="landing-honest-note"><span className="landing-honest-icon">i</span><p><strong>A clear guarantee.</strong> HTTP webhook delivery is at least once. A receiver may see a duplicate, so it should deduplicate using the stable <code>webhook-id</code>.</p></div>
          </div>
        </section>

        <section className="landing-final landing-container" aria-labelledby="final-heading">
          <div className="landing-final-panel"><div className="landing-final-glow" /><span className="landing-kicker">READY TO EXPLORE?</span><h2 id="final-heading">Build with confidence.<br /><em>Operate with clarity.</em></h2><p>See the delivery pipeline in action, inspect its behavior, and make every webhook accountable.</p><div className="landing-final-actions"><Link href="/demo" className="landing-button landing-button-primary">Explore the demo <ArrowUpRight size={18} aria-hidden="true" /></Link><Link href="/how-it-works" className="landing-final-link">How RelayForge works <ArrowRight size={17} aria-hidden="true" /></Link></div></div>
        </section>
      </main>

      <footer className="landing-footer"><div className="landing-container landing-footer-inner"><Brand light /><span>Reliable delivery, made visible.</span><span className="landing-footer-links"><Link href="/docs">SDK docs</Link><a href="https://github.com/FinzyPHINZY/relayforge" target="_blank" rel="noopener noreferrer">Source code <ArrowUpRight size={14} aria-hidden="true" /></a></span></div></footer>
    </div>
  );
}

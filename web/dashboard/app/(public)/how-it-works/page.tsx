import type { Metadata } from "next";
import Link from "next/link";
import {
  Activity,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  Check,
  CircleAlert,
  Database,
  Fingerprint,
  GitBranch,
  LockKeyhole,
  RotateCcw,
  ShieldCheck,
  Webhook,
} from "lucide-react";
import "./works.css";

export const metadata: Metadata = {
  title: "How RelayForge works | RelayForge",
  description: "Explore RelayForge's durable outbox, signed webhook delivery, at-least-once recovery, security controls, and measured 10,000-event reliability proof.",
};

const stages = [
  { icon: GitBranch, number: "01", label: "ACCEPT", title: "Commit before acknowledging", body: "The Go API writes the event and its outbox row in one PostgreSQL transaction. Only then does it return HTTP 202." },
  { icon: Database, number: "02", label: "DISPATCH", title: "Create durable delivery runs", body: "A dispatcher reads pending outbox rows, matches subscriptions, and creates endpoint-specific delivery records in the same transaction that marks each row dispatched." },
  { icon: Fingerprint, number: "03", label: "SEND", title: "Sign the exact bytes sent", body: "Workers lease due deliveries, pin an approved destination IP, and send an HMAC-signed HTTP request with a stable webhook ID." },
  { icon: RotateCcw, number: "04", label: "RECOVER", title: "Keep the outcome visible", body: "Every attempt records its result. Retryable failures receive a durable next attempt time; exhausted deliveries can be reviewed and replayed." },
];

const crashSteps = [
  { number: "01", title: "Receiver accepts", body: "The receiver processes the webhook and returns HTTP 204. Its side effect has happened." },
  { number: "02", title: "Worker stops", body: "The worker crashes before it records that 204 in PostgreSQL. The delivery lease eventually expires." },
  { number: "03", title: "Another worker retries", body: "A new worker reclaims the delivery and sends it again with the same webhook-id. The receiver may see a duplicate." },
  { number: "04", title: "History explains it", body: "The abandoned attempt is recorded as unknown with a lease-expired error, followed by the later outcome. The accepted event remains durable." },
];

const controls = [
  { icon: LockKeyhole, title: "Tenant boundaries", body: "API keys carry scopes and belong to one organization. Events, endpoints, deliveries, replay, and metrics are queried within that organization." },
  { icon: ShieldCheck, title: "Safe destinations", body: "Registration and each send check destination addresses. Workers pin the approved IP, reject private and metadata networks, and do not follow redirects." },
  { icon: Fingerprint, title: "Key rotation", body: "Signing secrets are encrypted at rest and shown once. A bounded overlap supports receiver migration; expired key material is erased." },
  { icon: Activity, title: "Operator signals", body: "OpenTelemetry traces and metrics, structured logs, attempt history, and an operator monitor make failures and backlog visible." },
];

export default function HowItWorksPage() {
  return <div className="landing workings">
    <nav className="workings-outline" aria-label="On this page"><div className="landing-container workings-outline-inner">
      <span>ON THIS PAGE</span><a href="#pipeline">Pipeline</a><a href="#crash-window">Crash window</a><a href="#proof">Measured proof</a><a href="#controls">Controls</a>
    </div></nav>

    <main>
      <section className="landing-container workings-hero" aria-labelledby="workings-title">
        <div className="workings-hero-copy"><Link href="/" className="workings-back"><ArrowLeft size={15} aria-hidden="true" /> Back to RelayForge</Link><span className="landing-kicker">INSIDE THE DELIVERY ENGINE</span><h1 id="workings-title">A webhook is a journey.<br /><span>Every step should have a record.</span></h1><p>RelayForge accepts an event, commits it durably, delivers a signed request, and keeps the outcome inspectable. Here is what happens when the path is smooth—and when it is not.</p><div className="workings-hero-actions"><a href="#pipeline" className="landing-button landing-button-primary">Trace the delivery path <ArrowRight size={17} aria-hidden="true" /></a><Link href="/demo" className="landing-button landing-button-secondary">Try the guided demo <ArrowUpRight size={17} aria-hidden="true" /></Link></div></div>
        <div className="workings-terminal" aria-label="Illustrative delivery state diagram"><div className="workings-terminal-head"><span><span className="landing-live-dot" /> DELIVERY RUN</span><span>ILLUSTRATIVE</span></div><div className="workings-terminal-line"><span>event.accepted</span><strong>202</strong></div><div className="workings-terminal-line"><span>outbox.dispatched</span><strong>committed</strong></div><div className="workings-terminal-line"><span>attempt.1</span><strong className="workings-warning">503 → retry</strong></div><div className="workings-terminal-line"><span>attempt.2</span><strong className="workings-success">204 → succeeded</strong></div><div className="workings-terminal-foot">One delivery ID across both attempts.</div></div>
      </section>

      <section id="pipeline" className="workings-section workings-pipeline" aria-labelledby="pipeline-title"><div className="landing-container"><div className="workings-section-heading"><span className="landing-kicker">THE DURABLE PATH</span><h2 id="pipeline-title">From API request to receiver response.</h2><p>PostgreSQL is the source of truth throughout the handoff. Redis helps with distributed API throttling; it is not the durable delivery queue.</p></div><ol className="workings-stage-grid">{stages.map(({ icon: Icon, number, label, title, body }) => <li key={number} className="workings-stage"><div className="workings-stage-top"><span>{number} / {label}</span><Icon size={23} aria-hidden="true" /></div><h3>{title}</h3><p>{body}</p></li>)}</ol><div className="workings-contract"><Check size={19} aria-hidden="true" /><p><strong>The delivery contract:</strong> HTTP delivery is at least once. Any 2xx acknowledges an attempt; it does not prove the receiver finished downstream work. Cross-endpoint ordering is not guaranteed.</p></div></div></section>

      <section id="crash-window" className="workings-section workings-crash" aria-labelledby="crash-title"><div className="landing-container workings-crash-layout"><div className="workings-crash-intro"><span className="landing-kicker">THE HARD FAILURE CASE</span><h2 id="crash-title">The receiver said yes.<br /><em>The worker never wrote it down.</em></h2><p>A real-process test kills a worker immediately after the receiver acknowledges the request. This narrow window is why a reliable system can still deliver the same webhook more than once.</p><div className="workings-crash-note"><CircleAlert size={20} aria-hidden="true" /><span>Receivers should verify the signature, check timestamp freshness, and deduplicate by the stable <code>webhook-id</code> across retries.</span></div></div><ol className="workings-crash-steps">{crashSteps.map(({ number, title, body }) => <li key={number}><span className="workings-crash-index">{number}</span><div><h3>{title}</h3><p>{body}</p></div></li>)}</ol></div></section>

      <section id="proof" className="workings-section workings-proof" aria-labelledby="proof-title"><div className="landing-container"><div className="workings-section-heading"><span className="landing-kicker">MEASURED, NOT ESTIMATED</span><h2 id="proof-title">10,000 accepted. 10,000 accounted for.</h2><p>One local Testcontainers run on 2026-11-15 published across two tenants while injecting timeouts, HTTP failures, and a post-receipt worker crash.</p></div><div className="workings-proof-grid"><div className="workings-proof-lead"><span>RECONCILED OUTCOMES</span><strong>10,000 <small>/ 10,000</small></strong><p>Accepted events, dispatched outbox rows, and terminal deliveries all matched.</p></div><div><strong>9,995</strong><span>succeeded</span></div><div><strong>5</strong><span>exhausted</span></div><div><strong>11</strong><span>duplicate receiver receipts</span></div><div><strong>1</strong><span>lease reclaimed</span></div></div><p className="workings-proof-footnote">The run recorded 10,011 attempts and 11 retries in 2m 48s on an 8-CPU host with Go 1.25.0, PostgreSQL 17, and 32 workers. These are measurements from one test, not a production throughput or latency guarantee. <a href="https://github.com/FinzyPHINZY/relayforge#measured-reliability-proof" target="_blank" rel="noopener noreferrer">Read the method and reproducible command <ArrowUpRight size={14} aria-hidden="true" /></a></p></div></section>

      <section id="controls" className="workings-section workings-controls" aria-labelledby="controls-title"><div className="landing-container"><div className="workings-section-heading"><span className="landing-kicker">BUILT TO OPERATE</span><h2 id="controls-title">Control risk. Explain outcomes.</h2><p>Reliability includes the controls around delivery and the tools to investigate it afterward.</p></div><div className="workings-control-grid">{controls.map(({ icon: Icon, title, body }) => <article key={title}><Icon size={22} aria-hidden="true" /><h3>{title}</h3><p>{body}</p></article>)}</div><div className="workings-console"><div><span className="landing-kicker">THE CONSOLE</span><h3>Four views of the same system.</h3><p>Overview shows aggregate health. Event explorer finds accepted events. Endpoint management sets destination policies. Delivery inspector shows each attempt, retry time, review, and replay.</p></div><Link href="/dashboard" className="landing-button landing-button-primary">Open the console <ArrowUpRight size={17} aria-hidden="true" /></Link></div></div></section>

      <section className="landing-container workings-end"><div><span className="landing-kicker">SEE IT FOR YOURSELF</span><h2>Follow a delivery from start to finish.</h2><p>The guided demo illustrates the state changes without an API key or live requests.</p></div><Link href="/demo" className="landing-button landing-button-primary">Explore the guided demo <ArrowRight size={17} aria-hidden="true" /></Link></section>
    </main>

    <footer className="landing-footer"><div className="landing-container landing-footer-inner"><Link href="/" className="landing-brand" aria-label="RelayForge home"><span className="landing-brand-mark"><Webhook size={21} aria-hidden="true" /></span><span>relayforge<span className="landing-brand-period">.</span></span></Link><span>Reliable delivery, made visible.</span><Link href="/" className="workings-footer-link">Back home <ArrowRight size={14} aria-hidden="true" /></Link></div></footer>
  </div>;
}

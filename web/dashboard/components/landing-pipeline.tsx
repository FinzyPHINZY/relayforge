"use client";

import { useEffect, useRef, useState } from "react";
import { useInView, useReducedMotion, motion } from "motion/react";
import { Activity, Check, CheckCheck, GitBranch, ShieldCheck, Terminal, Webhook } from "lucide-react";

export function LandingPipeline() {
  const visualRef = useRef<HTMLDivElement>(null);
  const inView = useInView(visualRef, { once: true, amount: 0.45 });
  const reduceMotion = useReducedMotion();
  const [stage, setStage] = useState(-1);

  useEffect(() => {
    if (!inView || reduceMotion) return;
    const timers = [0, 850, 1700].map((delay, index) =>
      window.setTimeout(() => setStage(index), delay),
    );
    return () => timers.forEach(window.clearTimeout);
  }, [inView, reduceMotion]);

  const rowMotion = (index: number) => ({
    scale: stage === index && !reduceMotion ? 1.025 : 1,
  });

  return (
    <div ref={visualRef} className="landing-visual" aria-label="Illustration of an event moving through RelayForge to a webhook endpoint" role="img">
      <div className="landing-visual-grid" />
      <div className="landing-visual-top"><span><span className="landing-live-dot" /> DELIVERY PIPELINE</span><span>LIVE FLOW <Activity size={13} aria-hidden="true" /></span></div>
      <div className="landing-flow">
        <motion.div className={`landing-flow-row landing-flow-row-first${stage === 0 ? " is-active" : ""}`} animate={rowMotion(0)} transition={{ duration: 0.45 }}>
          <span className="landing-flow-icon landing-flow-icon-a"><Terminal size={19} aria-hidden="true" /></span>
          <div><span className="landing-flow-label">EVENT RECEIVED</span><strong>order.created</strong><small>Idempotency key · order_81f2</small></div>
          <span className="landing-flow-check"><Check size={15} aria-hidden="true" /></span>
        </motion.div>
        <div className="landing-flow-connector"><span /><span>committed to outbox</span></div>
        <motion.div className={`landing-flow-row landing-flow-row-middle${stage === 1 ? " is-active" : ""}`} animate={rowMotion(1)} transition={{ duration: 0.45 }}>
          <span className="landing-flow-icon landing-flow-icon-b"><GitBranch size={19} aria-hidden="true" /></span>
          <div><span className="landing-flow-label">RELAYFORGE</span><strong>Safely in motion</strong><small>Durable queue · bounded retries</small></div>
          <span className="landing-flow-pulse"><span /></span>
        </motion.div>
        <div className="landing-flow-connector"><span /><span>signed &amp; sent</span></div>
        <motion.div className={`landing-flow-row landing-flow-row-last${stage === 2 ? " is-active" : ""}`} animate={rowMotion(2)} transition={{ duration: 0.45 }}>
          <span className="landing-flow-icon landing-flow-icon-c"><Webhook size={19} aria-hidden="true" /></span>
          <div><span className="landing-flow-label">YOUR ENDPOINT</span><strong>Delivery acknowledged</strong><small>HTTP 204 · signature verified</small></div>
          <span className="landing-flow-success"><CheckCheck size={17} aria-hidden="true" /></span>
        </motion.div>
      </div>
      <div className="landing-visual-bottom"><span><span className="landing-tiny-square" /> Every attempt has a record</span><span>01 / 01</span></div>
      <div className="landing-float-card"><ShieldCheck size={19} aria-hidden="true" /><span><strong>Signed request</strong><small>Integrity built in</small></span></div>
    </div>
  );
}

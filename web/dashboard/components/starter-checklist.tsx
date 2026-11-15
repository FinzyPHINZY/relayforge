import Link from "next/link";
import { ArrowRight, Check, FlaskConical, Webhook } from "lucide-react";

type StarterChecklistProps = {
  hasEndpoint: boolean;
  hasEvent: boolean;
  hasAttempt: boolean;
  canCreateEndpoint: boolean;
  canPublishEvent: boolean;
};

export function StarterChecklist({ hasEndpoint, hasEvent, hasAttempt, canCreateEndpoint, canPublishEvent }: StarterChecklistProps) {
  const steps = [
    { title: "Add a destination", detail: "Connect an HTTPS receiver, subscribe it to order.created, and verify it if required.", done: hasEndpoint, href: "/endpoints#add-endpoint", action: "Add endpoint", allowed: canCreateEndpoint, waiting: "Ask an owner or operator to add an endpoint." },
    { title: "Publish an example", detail: "Choose an editable sample event and publish it when you are ready.", done: hasEvent, href: "/events#publish-event", action: "Choose sample", allowed: canPublishEvent && hasEndpoint, waiting: hasEndpoint ? "Ask an owner or operator to publish an event." : "Add a destination first." },
    { title: "Inspect the attempt", detail: "Follow the receiver response, retry history, and final status.", done: hasAttempt, href: "/deliveries", action: "View deliveries", allowed: hasEvent, waiting: hasEvent ? "Check the endpoint subscription if no delivery appears." : "Publish an event first." },
  ] as const;
  const completed = steps.filter((step) => step.done).length;
  if (completed === steps.length) return null;
  return <section aria-labelledby="starter-heading" className="mb-8 overflow-hidden rounded-xl border border-[#cddfc3] bg-[#f0f6e9] shadow-[0_14px_34px_rgba(26,52,30,0.04)]">
    <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[#d8e6d0] px-5 py-5 sm:px-6"><div><p className="mb-2 text-xs font-bold uppercase tracking-[0.16em] text-[#3c7a40]">START HERE</p><h2 id="starter-heading" className="text-xl font-semibold tracking-tight text-[#1c3426]">Your first real delivery</h2><p className="mt-2 max-w-2xl text-sm leading-6 text-[#5c7061]">Follow these steps to fill this workspace with real events and delivery attempts. Examples stay out of your metrics until you publish one.</p></div><span className="rounded-full border border-[#b8d5a9] bg-white px-3 py-1 text-xs font-semibold text-[#3c7a40]">{completed} of 3 complete</span></div>
    <ol className="grid divide-y divide-[#d8e6d0] lg:grid-cols-3 lg:divide-x lg:divide-y-0">{steps.map((step, index) => <li key={step.title} className="flex flex-col gap-3 p-5 sm:p-6"><div className="flex items-center gap-3"><span className={`grid size-8 shrink-0 place-items-center rounded-lg text-xs font-bold ${step.done ? "bg-[#b7f36d] text-[#1c3426]" : "border border-[#bad4ae] bg-white text-[#3c7a40]"}`}>{step.done ? <Check size={17} aria-hidden="true" /> : String(index + 1).padStart(2, "0")}</span><h3 className="font-semibold text-[#20392b]">{step.title}</h3></div><p className="min-h-12 text-xs leading-5 text-[#5c7061]">{step.detail}</p><div className="mt-auto">{step.done ? <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-[#3c7a40]"><Check size={14} aria-hidden="true" /> Complete</span> : step.allowed ? <Link href={step.href} className="inline-flex items-center gap-1.5 text-xs font-semibold text-[#2e6a35] underline-offset-4 hover:underline">{step.action} <ArrowRight size={14} aria-hidden="true" /></Link> : <span className="text-xs text-[#637767]">{step.waiting}</span>}</div></li>)}</ol>
    <div className="flex flex-wrap items-center gap-2 border-t border-[#d8e6d0] px-5 py-3 text-xs text-[#5c7061] sm:px-6"><FlaskConical size={15} className="text-[#3c7a40]" aria-hidden="true" /><span>No receiver yet?</span><Link href="/demo" className="inline-flex items-center gap-1 font-semibold text-[#2e6a35] underline-offset-4 hover:underline">Explore the illustrative demo <Webhook size={13} aria-hidden="true" /></Link></div>
  </section>;
}

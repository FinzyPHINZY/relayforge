import Link from "next/link";
import { Activity, ArrowUpRight, Boxes, CircleAlert, RadioTower, ScrollText, Workflow } from "lucide-react";
import { signOut } from "@/app/actions";
import { Button } from "@/components/ui/button";

const navigation = [
  { href: "/dashboard", label: "Overview", icon: Activity },
  { href: "/endpoints", label: "Endpoints", icon: RadioTower },
  { href: "/events", label: "Events", icon: Boxes },
  { href: "/deliveries", label: "Deliveries", icon: Workflow },
  { href: "/dead-letters", label: "Dead letters", icon: CircleAlert },
];

export function Shell({ children }: { children: React.ReactNode }) {
  return <div className="min-h-screen bg-[#090f13] lg:flex">
    <aside className="border-b border-white/10 bg-[#0d161a] lg:sticky lg:top-0 lg:flex lg:h-screen lg:w-64 lg:flex-col lg:border-b-0 lg:border-r">
      <div className="flex items-center gap-3 border-b border-white/10 px-6 py-6">
        <div className="flex size-10 items-center justify-center rounded-xl bg-emerald-400 text-slate-950"><ScrollText size={22} strokeWidth={2.5} /></div>
        <div><div className="text-base font-bold tracking-tight text-white">RelayForge</div><div className="text-xs text-slate-500">Operator console</div></div>
      </div>
      <nav aria-label="Main navigation" className="flex gap-1 overflow-x-auto px-3 py-3 lg:flex-col lg:gap-1 lg:py-6">
        {navigation.map(({ href, label, icon: Icon }) => <Link key={href} href={href} className="flex shrink-0 items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-slate-300 transition-colors hover:bg-white/8 hover:text-white"><Icon size={17} />{label}</Link>)}
      </nav>
      <div className="hidden flex-1 lg:block" />
      <div className="hidden border-t border-white/10 p-4 lg:block">
        <div className="mb-4 rounded-xl border border-emerald-400/15 bg-emerald-400/5 p-3 text-xs leading-5 text-slate-400">Delivery attempts are at least once. Receivers should deduplicate by <code className="text-emerald-300">webhook-id</code>.</div>
        <form action={signOut}><Button variant="ghost" className="w-full justify-between">Sign out <ArrowUpRight size={15} /></Button></form>
      </div>
    </aside>
    <div className="min-w-0 flex-1">
      <header className="flex items-center justify-between border-b border-white/10 px-5 py-4 lg:px-10">
        <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Delivery operations</div>
        <div className="flex items-center gap-3"><span className="flex items-center gap-2 text-xs text-slate-400"><span className="size-2 rounded-full bg-emerald-400" /> Live data</span><form action={signOut} className="lg:hidden"><Button variant="ghost" size="sm">Sign out</Button></form></div>
      </header>
      <main className="mx-auto max-w-7xl px-5 py-8 lg:px-10 lg:py-10">{children}</main>
    </div>
  </div>;
}

export function PageTitle({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) {
  return <div className="mb-8 flex flex-wrap items-end justify-between gap-4"><div><p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-emerald-400">{eyebrow}</p><h1 className="text-3xl font-semibold tracking-tight text-white sm:text-4xl">{title}</h1><p className="mt-2 text-sm text-slate-400">{description}</p></div>{action}</div>;
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="rounded-xl border border-dashed border-white/15 px-6 py-12 text-center"><p className="font-medium text-slate-200">{title}</p><p className="mt-2 text-sm text-slate-500">{description}</p></div>;
}

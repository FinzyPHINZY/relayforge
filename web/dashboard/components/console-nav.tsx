"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Boxes, ChartNoAxesCombined, CircleAlert, RadioTower, UsersRound, Workflow } from "lucide-react";

const navigation = [
  { href: "/dashboard", label: "Overview", icon: Activity },
  { href: "/endpoints", label: "Endpoints", icon: RadioTower },
  { href: "/events", label: "Events", icon: Boxes },
  { href: "/deliveries", label: "Deliveries", icon: Workflow },
  { href: "/dead-letters", label: "Dead letters", icon: CircleAlert },
  { href: "/analytics", label: "Analytics", icon: ChartNoAxesCombined },
  { href: "/team", label: "Team", icon: UsersRound },
];

export function ConsoleNav() {
  const pathname = usePathname();
  return <nav aria-label="Main navigation" className="flex gap-1 overflow-x-auto px-3 py-3 lg:flex-col lg:gap-1 lg:py-6">
    {navigation.map(({ href, label, icon: Icon }) => {
      const active = pathname === href || pathname.startsWith(`${href}/`);
      return <Link key={href} href={href} aria-current={active ? "page" : undefined} className={`flex shrink-0 items-center gap-3 rounded-lg border px-3 py-2.5 text-sm transition-colors ${active ? "border-[#527b4b] bg-[#29442e] font-semibold text-[#d5f8ae]" : "border-transparent text-slate-300 hover:bg-white/8 hover:text-white"}`}><Icon size={17} aria-hidden="true" />{label}</Link>;
    })}
  </nav>;
}

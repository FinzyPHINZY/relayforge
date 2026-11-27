"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowUpRight, Webhook } from "lucide-react";
import { isActiveNav, publicNav } from "@/lib/public-nav";

export function PublicHeader() {
  const pathname = usePathname();
  return (
    <div className="landing">
      <header className="landing-header">
        <div className="landing-container landing-header-inner">
          <Link href="/" className="landing-brand" aria-label="RelayForge home">
            <span className="landing-brand-mark"><Webhook size={21} strokeWidth={2.2} aria-hidden="true" /></span>
            <span>relayforge<span className="landing-brand-period">.</span></span>
          </Link>
          <nav className="landing-nav" aria-label="Main navigation">
            {publicNav.map((item) => (
              <Link key={item.href} href={item.href} aria-current={isActiveNav(pathname, item.href) ? "page" : undefined}>{item.label}</Link>
            ))}
          </nav>
          <Link href="/dashboard" className="landing-header-cta">Open console <ArrowUpRight size={16} aria-hidden="true" /></Link>
        </div>
      </header>
    </div>
  );
}

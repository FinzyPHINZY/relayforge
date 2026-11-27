"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowUpRight, Menu, Webhook, X } from "lucide-react";
import { isActiveNav, publicNav } from "@/lib/public-nav";

const githubUrl = "https://github.com/FinzyPHINZY/relayforge";

export function PublicHeader() {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const headerRef = useRef<HTMLElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => { setOpen(false); }, [pathname]);

  useEffect(() => {
    if (!open) return;
    panelRef.current?.querySelector("a")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
      buttonRef.current?.focus();
    };
    const onPointer = (e: PointerEvent) => {
      if (!headerRef.current?.contains(e.target as Node)) setOpen(false);
    };
    // the panel is a disclosure, so tabbing out of the header closes it
    const onFocus = (e: FocusEvent) => {
      if (!headerRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("focusin", onFocus);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("focusin", onFocus);
    };
  }, [open]);

  return (
    // header sits outside each page's .landing wrapper, so it needs its own for fonts and link styles
    <div className="landing landing-header-shell">
      <header ref={headerRef} className="landing-header" style={{ viewTransitionName: "public-header" }}>
        <div className="landing-container landing-header-inner">
          <Link href="/" className="landing-brand" aria-label="RelayForge home">
            <span className="landing-brand-mark"><Webhook size={21} strokeWidth={2.2} aria-hidden="true" /></span>
            <span>relayforge<span className="landing-brand-period">.</span></span>
          </Link>
          <nav className="landing-nav" aria-label="Main navigation">
            {publicNav.map((item) => (
              <Link key={item.href} href={item.href} aria-current={isActiveNav(pathname, item.href) ? "page" : undefined}>{item.label}</Link>
            ))}
            <a href={githubUrl} target="_blank" rel="noopener noreferrer">GitHub <ArrowUpRight size={14} aria-hidden="true" /></a>
          </nav>
          <Link href="/dashboard" className="landing-header-cta">Open console <ArrowUpRight size={16} aria-hidden="true" /></Link>
          <button
            ref={buttonRef}
            type="button"
            className="landing-menu-button"
            aria-expanded={open}
            aria-controls="public-mobile-nav"
            aria-label={open ? "Close menu" : "Open menu"}
            onClick={() => setOpen((v) => !v)}
          >
            {open ? <X size={18} aria-hidden="true" /> : <Menu size={18} aria-hidden="true" />}
          </button>
        </div>
        <div ref={panelRef} id="public-mobile-nav" className="landing-mobile-nav" hidden={!open}>
          <nav className="landing-container" aria-label="Main navigation, compact">
            {publicNav.map((item) => (
              <Link key={item.href} href={item.href} aria-current={isActiveNav(pathname, item.href) ? "page" : undefined} onClick={() => setOpen(false)}>{item.label}</Link>
            ))}
            <a href={githubUrl} target="_blank" rel="noopener noreferrer" onClick={() => setOpen(false)}>GitHub <ArrowUpRight size={14} aria-hidden="true" /></a>
          </nav>
        </div>
      </header>
    </div>
  );
}

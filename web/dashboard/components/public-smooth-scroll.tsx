"use client";

import { useEffect, useRef } from "react";
import { usePathname } from "next/navigation";
import type Lenis from "lenis";
import { scrollTargetFor } from "@/lib/public-nav";
import "lenis/dist/lenis.css";

function settle(lenis: Lenis, fromHistory: boolean) {
  const target = scrollTargetFor(window.location.href);
  if (target !== "top") {
    if (document.getElementById(target.slice(1))) lenis.scrollTo(target);
    return;
  }
  // back/forward keeps the position the browser restored; lenis just needs to pick it up.
  if (fromHistory) lenis.resize();
  else lenis.scrollTo(0, { immediate: true });
}

/** One Lenis instance for the public pages; it follows route changes instead of being recreated. */
export function PublicSmoothScroll() {
  const pathname = usePathname();
  const lenisRef = useRef<Lenis | null>(null);
  const fromHistory = useRef(false);
  const firstPath = useRef(true);

  useEffect(() => {
    let cancelled = false;
    let frame = 0;
    const onPopState = () => { fromHistory.current = true; };
    window.addEventListener("popstate", onPopState);

    import("lenis").then(({ default: LenisClass }) => {
      if (cancelled) return;
      const lenis = new LenisClass({
        autoRaf: true,
        anchors: true,
        lerp: 0.075,
        smoothWheel: true,
        syncTouch: true,
        stopInertiaOnNavigate: true,
      });
      lenisRef.current = lenis;
      // a fresh deep link: next has already jumped natively, lenis lines up with the target.
      if (window.location.hash) frame = window.requestAnimationFrame(() => settle(lenis, true));
    });

    return () => {
      cancelled = true;
      window.cancelAnimationFrame(frame);
      window.removeEventListener("popstate", onPopState);
      lenisRef.current?.destroy();
      lenisRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (firstPath.current) { firstPath.current = false; return; }
    const lenis = lenisRef.current;
    const historyMove = fromHistory.current;
    fromHistory.current = false;
    if (!lenis) return;
    const frame = window.requestAnimationFrame(() => settle(lenis, historyMove));
    return () => window.cancelAnimationFrame(frame);
  }, [pathname]);

  return null;
}

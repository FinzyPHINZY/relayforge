"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import "lenis/dist/lenis.css";

const publicPages = new Set([
  "/",
  "/docs",
  "/how-it-works",
  "/demo",
  "/login",
  "/welcome",
  "/auth/email",
  "/auth/check-email",
]);

/** Keep the public pages on one scroll behavior without changing the console. */
export function PublicSmoothScroll() {
  const pathname = usePathname();

  useEffect(() => {
    if (!publicPages.has(pathname)) return;

    let instance: { destroy: () => void; scrollTo: (target: string) => void } | undefined;
    let cancelled = false;
    let frame = 0;

    import("lenis").then(({ default: Lenis }) => {
      if (cancelled) return;
      instance = new Lenis({
        autoRaf: true,
        anchors: true,
        lerp: 0.075,
        smoothWheel: true,
        syncTouch: true,
        stopInertiaOnNavigate: true,
      });

      // Next.js handles route changes, while Lenis handles the destination hash.
      if (window.location.hash) {
        frame = window.requestAnimationFrame(() => {
          let hash = window.location.hash;
          try { hash = decodeURIComponent(hash); } catch { return; }
          if (document.getElementById(hash.slice(1))) instance?.scrollTo(hash);
        });
      }
    });

    return () => {
      cancelled = true;
      window.cancelAnimationFrame(frame);
      instance?.destroy();
    };
  }, [pathname]);

  return null;
}

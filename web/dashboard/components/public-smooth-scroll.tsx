"use client";

import { useEffect } from "react";
import "lenis/dist/lenis.css";

/** Smooth scrolling is scoped to the public storytelling pages. */
export function PublicSmoothScroll() {
  useEffect(() => {
    let instance: { destroy: () => void } | undefined;
    let cancelled = false;

    import("lenis").then(({ default: Lenis }) => {
      if (cancelled) return;
      instance = new Lenis({
        autoRaf: true,
        anchors: true,
        lerp: 0.075,
        smoothWheel: true,
        stopInertiaOnNavigate: true,
      });
    });

    return () => {
      cancelled = true;
      instance?.destroy();
    };
  }, []);

  return null;
}

"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui/button";

export default function ErrorPage({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { console.error("Dashboard request failed", error.message); }, [error]);
  return <main className="console-theme flex min-h-screen items-center justify-center bg-[#f6f7f1] px-5"><div className="max-w-md rounded-xl border border-white/10 bg-slate-900 p-8 shadow-[0_10px_32px_rgba(26,52,30,0.05)]"><h1 className="text-2xl font-semibold">Could not load this view</h1><p className="mt-3 text-sm text-slate-400">Check the RelayForge API and try again.</p><Button className="mt-6" onClick={reset}>Retry</Button></div></main>;
}

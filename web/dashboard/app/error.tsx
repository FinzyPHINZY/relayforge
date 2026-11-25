"use client";

import Link from "next/link";
import { useEffect } from "react";
import { signOut } from "@/app/actions";
import { Button } from "@/components/ui/button";

export default function ErrorPage({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { console.error("Dashboard request failed", error.message); }, [error]);
  return <main className="console-theme flex min-h-screen items-center justify-center bg-[#f6f7f1] px-5"><div className="max-w-md rounded-xl border border-white/10 bg-slate-900 p-8 shadow-[0_10px_32px_rgba(26,52,30,0.05)]"><h1 className="text-2xl font-semibold">Could not load this view</h1><p className="mt-3 text-sm text-slate-400">Check the RelayForge API and try again.</p><div className="mt-6 flex flex-wrap items-center gap-3"><Button onClick={reset}>Retry</Button><Link href="/account" className="text-sm text-emerald-300 hover:underline">Your account</Link><form action={signOut}><Button variant="ghost" size="sm">Sign out</Button></form></div></div></main>;
}

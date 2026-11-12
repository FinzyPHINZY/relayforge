import { KeyRound, LockKeyhole, RadioTower } from "lucide-react";
import Link from "next/link";
import { signIn } from "@/app/actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const { error } = await searchParams;
  const message = error === "invalid" ? "That API key is invalid or lacks the metrics:read scope." : error === "expired" ? "Your API key is no longer accepted. Sign in again." : error === "unavailable" ? "RelayForge is unavailable. Try again in a moment." : null;
  return <main className="flex min-h-screen items-center justify-center bg-[radial-gradient(circle_at_50%_0%,#12352d_0%,#090f13_42%)] px-5 py-12">
    <div className="w-full max-w-md">
      <Link href="/" className="mb-8 flex w-fit items-center gap-3"><div className="flex size-11 items-center justify-center rounded-xl bg-emerald-400 text-slate-950"><RadioTower size={23} /></div><div><p className="text-xl font-bold tracking-tight">RelayForge</p><p className="text-xs text-slate-400">Operator console</p></div></Link>
      <Card className="bg-slate-900/90"><CardContent className="p-7">
        <div className="mb-6 flex size-11 items-center justify-center rounded-xl border border-emerald-400/20 bg-emerald-400/10 text-emerald-300"><KeyRound size={21} /></div>
        <h1 className="text-2xl font-semibold tracking-tight">Sign in to your workspace</h1>
        <p className="mt-2 text-sm leading-6 text-slate-400">Use a scoped RelayForge API key to inspect deliveries and manage endpoints.</p>
        <form action={signIn} className="mt-7 space-y-4"><div><label htmlFor="apiKey" className="mb-2 block text-sm text-slate-300">API key</label><Input id="apiKey" name="apiKey" type="password" autoComplete="off" placeholder="whk_…" required /></div><Button className="w-full">Open console</Button></form>
        {message ? <p role="alert" className="mt-4 rounded-lg border border-rose-400/20 bg-rose-400/10 p-3 text-sm text-rose-200">{message}</p> : null}
        <p className="mt-6 flex items-start gap-2 text-xs leading-5 text-slate-500"><LockKeyhole size={15} className="mt-0.5 shrink-0" /> Your key is encrypted in an HTTP-only session cookie. Browser code never calls the API with it.</p>
      </CardContent></Card>
    </div>
  </main>;
}

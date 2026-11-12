import Link from "next/link";

export default function NotFound() {
  return <main className="flex min-h-screen items-center justify-center px-5"><div className="max-w-md text-center"><p className="text-sm font-semibold text-emerald-300">404</p><h1 className="mt-2 text-3xl font-semibold">Record not found</h1><p className="mt-3 text-slate-400">It may have been removed by retention or is outside your organization.</p><Link href="/dashboard" className="mt-6 inline-block text-sm text-emerald-300 hover:underline">Back to overview →</Link></div></main>;
}

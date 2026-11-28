import { Webhook } from "lucide-react";
import { AccountMenu } from "@/components/account-menu";
import { ConsoleNav } from "@/components/console-nav";
import { fetchAccount, type Workspace } from "@/lib/account";
import { fetchPrincipal } from "@/lib/api";
import { getSession } from "@/lib/session";

type ShellContext = { kind: "user" | "key" | null; scopes: string[]; workspaces: Workspace[]; currentId: string | null };

async function loadShellContext(): Promise<ShellContext> {
  const session = await getSession();
  if (!session) return { kind: null, scopes: [], workspaces: [], currentId: null };
  const needsOrg = session.kind === "user" && !session.org;
  const [me, account] = await Promise.all([
    needsOrg ? null : fetchPrincipal(session).catch(() => null),
    session.kind === "user" ? fetchAccount(session.token).catch(() => null) : null,
  ]);
  return {
    kind: session.kind,
    scopes: me?.scopes ?? [],
    workspaces: account?.workspaces ?? [],
    currentId: session.kind === "user" ? session.org : null,
  };
}

export async function Shell({ children }: { children: React.ReactNode }) {
  const context = await loadShellContext();
  // below lg the sidebar is a top bar: brand and account card on one row, the nav under them.
  return <div className="console-theme min-h-screen bg-[#f6f7f1] lg:flex">
    <aside className="console-sidebar flex flex-wrap items-center border-b border-white/10 bg-[#14281e] lg:sticky lg:top-0 lg:h-screen lg:w-64 lg:flex-col lg:flex-nowrap lg:items-stretch lg:border-b-0 lg:border-r">
      <div className="order-1 flex min-w-0 flex-1 items-center gap-3 px-5 py-4 lg:order-none lg:flex-none lg:border-b lg:border-white/10 lg:px-6 lg:py-6">
        <div className="flex size-10 shrink-0 -rotate-6 items-center justify-center rounded-xl bg-[#b7f36d] text-[#173322]"><Webhook size={22} strokeWidth={2.3} /></div>
        <div className="text-base font-bold tracking-tight text-white">relayforge<span className="text-[#b7f36d]">.</span></div>
      </div>
      <div className="order-3 w-full min-w-0 border-t border-white/10 lg:order-none lg:min-h-0 lg:flex-1 lg:overflow-y-auto lg:border-t-0">
        <ConsoleNav showKeys={context.scopes.includes("keys:read")} />
      </div>
      {context.kind ? <div className="order-2 min-w-0 shrink-0 pr-4 lg:order-none lg:border-t lg:border-white/10 lg:p-3">
        <AccountMenu kind={context.kind} workspaces={context.workspaces} currentId={context.currentId} />
      </div> : null}
    </aside>
    <div className="min-w-0 flex-1">
      <header className="flex items-center justify-between border-b border-[#dfe5db] bg-[#f9faf6] px-5 py-4 lg:px-10">
        <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Delivery operations</div>
        <span className="flex items-center gap-2 text-xs text-slate-400"><span className="size-2 rounded-full bg-emerald-400" /> Live data</span>
      </header>
      <main className="mx-auto max-w-7xl px-5 py-8 lg:px-10 lg:py-10">{children}</main>
    </div>
  </div>;
}

export function PageTitle({ eyebrow, title, description, note, action }: { eyebrow: string; title: string; description: string; note?: React.ReactNode; action?: React.ReactNode }) {
  return <div className="mb-8 flex flex-wrap items-end justify-between gap-4"><div><p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-emerald-400">{eyebrow}</p><h1 className="text-3xl font-semibold tracking-tight text-white sm:text-4xl">{title}</h1><p className="mt-2 text-sm text-slate-400">{description}</p>{note ? <p className="mt-3 inline-block rounded-lg border border-[#cfe0c6] bg-[#eef5e6] px-3 py-2 text-xs leading-5 text-slate-500">{note}</p> : null}</div>{action}</div>;
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="rounded-xl border border-dashed border-white/15 px-6 py-12 text-center"><p className="font-medium text-slate-200">{title}</p><p className="mt-2 text-sm text-slate-500">{description}</p></div>;
}

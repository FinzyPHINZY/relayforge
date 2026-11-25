"use client";

import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";
import { useFormStatus } from "react-dom";
import { Check, ChevronsUpDown, LoaderCircle, Plus } from "lucide-react";
import { switchWorkspace } from "@/app/auth-actions";
import type { Workspace } from "@/lib/account";

function initial(name: string): string {
  return [...name.trim()][0]?.toUpperCase() ?? "?";
}

function Tile({ name, active = false }: { name: string; active?: boolean }) {
  return <span aria-hidden="true" className={`flex size-8 shrink-0 items-center justify-center rounded-lg text-sm font-bold ${active ? "bg-[#b7f36d] text-[#173322]" : "bg-white/10 text-slate-200"}`}>{initial(name)}</span>;
}

function SwitchButton({ workspace }: { workspace: Workspace }) {
  const { pending } = useFormStatus();
  return <button type="submit" disabled={pending} className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm text-slate-200 transition-colors hover:bg-white/8 hover:text-white disabled:opacity-60">
    <Tile name={workspace.name} /><span className="min-w-0 flex-1"><span className="block truncate font-medium">{workspace.name}</span><span className="block text-xs capitalize text-slate-500">{workspace.role}</span></span>
    {pending ? <LoaderCircle size={15} className="animate-spin text-slate-400" aria-label="Switching" /> : null}
  </button>;
}

export function WorkspaceSwitcher({ workspaces, currentId }: { workspaces: Workspace[]; currentId: string | null }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menuId = useId();
  const current = workspaces.find((w) => w.id.toLowerCase() === currentId?.toLowerCase()) ?? null;
  const others = workspaces.filter((w) => w !== current);

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") { setOpen(false); trigger.current?.focus(); } };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("pointerdown", onPointer); document.removeEventListener("keydown", onKey); };
  }, [open]);

  return <div ref={root} className="relative px-3 pt-3 lg:pt-4">
    <button ref={trigger} type="button" aria-expanded={open} aria-controls={menuId} onClick={() => setOpen((value) => !value)} className="flex w-full items-center gap-3 rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 text-left transition-colors hover:bg-white/8">
      <Tile name={current?.name ?? "?"} active={Boolean(current)} />
      <span className="min-w-0 flex-1"><span className="block truncate text-sm font-semibold text-white">{current?.name ?? "Choose a workspace"}</span><span className="block text-xs capitalize text-slate-500">{current ? current.role : `${workspaces.length} available`}</span></span>
      <ChevronsUpDown size={16} aria-hidden="true" className="shrink-0 text-slate-400" />
      <span className="sr-only">Switch workspace</span>
    </button>
    {open ? <div id={menuId} role="group" aria-label="Workspaces" className="absolute inset-x-3 top-full z-30 mt-2 rounded-xl border border-white/10 bg-[#1b3426] p-1.5 shadow-[0_18px_40px_rgba(6,20,12,0.45)]">
      {current ? <div className="flex items-center gap-3 rounded-lg px-2 py-2 text-sm" aria-current="true"><Tile name={current.name} active /><span className="min-w-0 flex-1"><span className="block truncate font-semibold text-white">{current.name}</span><span className="block text-xs text-slate-500"><span className="capitalize">{current.role}</span> · current</span></span><Check size={15} aria-hidden="true" className="text-[#b7f36d]" /></div> : null}
      {others.length ? <div className={current ? "mt-1 border-t border-white/10 pt-1" : undefined}>{others.map((workspace) => <form key={workspace.id} action={switchWorkspace}><input type="hidden" name="org" value={workspace.id} /><SwitchButton workspace={workspace} /></form>)}</div> : null}
      <div className="mt-1 border-t border-white/10 pt-1"><Link href="/welcome" onClick={() => setOpen(false)} className="flex items-center gap-3 rounded-lg px-2 py-2 text-sm text-slate-300 transition-colors hover:bg-white/8 hover:text-white"><span aria-hidden="true" className="flex size-8 items-center justify-center rounded-lg border border-dashed border-white/20"><Plus size={15} /></span>New workspace</Link></div>
    </div> : null}
  </div>;
}

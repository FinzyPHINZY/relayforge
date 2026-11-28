"use client";

import Link from "next/link";
import { useRef, useSyncExternalStore } from "react";
import { useFormStatus } from "react-dom";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronUp, CircleUserRound, KeyRound, LoaderCircle, LogOut, Plus } from "lucide-react";
import { signOut } from "@/app/actions";
import { switchWorkspace } from "@/app/auth-actions";
import type { Workspace } from "@/lib/account";
import { accountMenuModel } from "@/lib/account-menu";

const desktopQuery = "(min-width: 1024px)";

function subscribe(onChange: () => void) {
  const media = window.matchMedia(desktopQuery);
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}

// the sidebar is a column from lg up (card at the bottom) and a top bar below it.
function useDesktop(): boolean {
  return useSyncExternalStore(subscribe, () => window.matchMedia(desktopQuery).matches, () => false);
}

function initial(name: string): string {
  return [...name.trim()][0]?.toUpperCase() ?? "?";
}

function Tile({ name, active = false, icon }: { name: string; active?: boolean; icon?: React.ReactNode }) {
  return <span aria-hidden="true" className={`flex size-8 shrink-0 items-center justify-center rounded-lg text-sm font-bold ${active ? "bg-[#b7f36d] text-[#173322]" : "bg-white/10 text-slate-200"}`}>{icon ?? initial(name)}</span>;
}

const itemClass = "flex w-full cursor-pointer select-none items-center gap-3 rounded-lg px-2 py-2 text-left text-sm text-slate-300 outline-none transition-colors data-[highlighted]:bg-white/8 data-[highlighted]:text-white";

// rendered inside the switch form so it can read that form's pending state.
function SwitchTrigger({ label, title, subtitle, tile }: { label: string; title: string; subtitle: string; tile: React.ReactNode }) {
  const { pending } = useFormStatus();
  return <Menu.Trigger disabled={pending} aria-busy={pending || undefined} aria-label={pending ? "Switching workspace…" : label} className="group flex w-full min-w-0 items-center gap-3 rounded-xl border border-white/10 bg-white/5 p-1.5 text-left transition-colors hover:bg-white/8 disabled:cursor-progress disabled:opacity-70 data-[state=open]:bg-white/8 min-[400px]:pr-3 lg:px-3 lg:py-2.5">
    {tile}
    <span className="hidden min-w-0 flex-1 min-[400px]:block">
      <span className="block max-w-32 truncate text-sm font-semibold text-white lg:max-w-none">{title}</span>
      {subtitle ? <span className="hidden truncate text-xs text-slate-500 lg:block">{subtitle}</span> : null}
    </span>
    {pending
      ? <LoaderCircle size={16} aria-hidden="true" className="shrink-0 animate-spin text-slate-400" />
      : <ChevronUp size={16} aria-hidden="true" className="shrink-0 text-slate-400 max-lg:rotate-180" />}
    <span className="sr-only" aria-live="polite">{pending ? "Switching workspace…" : ""}</span>
  </Menu.Trigger>;
}

export function AccountMenu({ kind, workspaces, currentId }: { kind: "user" | "key"; workspaces: Workspace[]; currentId: string | null }) {
  const model = accountMenuModel({ kind, workspaces, currentId });
  const desktop = useDesktop();
  const switchForm = useRef<HTMLFormElement>(null);
  const orgInput = useRef<HTMLInputElement>(null);
  const signOutForm = useRef<HTMLFormElement>(null);
  const hasTopSection = model.others.length > 0 || model.showNewWorkspace;

  // the forms live outside the menu so closing it never unmounts them mid-submit.
  const switchTo = (id: string) => {
    if (!orgInput.current || !switchForm.current) return;
    orgInput.current.value = id;
    switchForm.current.requestSubmit();
  };

  return <Menu.Root>
    <form ref={switchForm} action={switchWorkspace} className="min-w-0">
      <input ref={orgInput} type="hidden" name="org" defaultValue="" />
      <SwitchTrigger
        label={kind === "key" ? "API key session menu" : `Workspace and account menu: ${model.title}`}
        title={model.title}
        subtitle={model.subtitle}
        tile={<Tile name={model.current ? model.title : "?"} active={Boolean(model.current)} icon={kind === "key" ? <KeyRound size={15} /> : undefined} />}
      />
    </form>
    <form ref={signOutForm} action={signOut} hidden />
    <Menu.Portal>
      {/* the portal renders outside the shell, so the content carries the sidebar palette itself */}
      <Menu.Content
        side={desktop ? "top" : "bottom"}
        align={desktop ? "start" : "end"}
        sideOffset={8}
        collisionPadding={12}
        className="rf-menu console-theme console-sidebar z-50 w-[var(--radix-dropdown-menu-trigger-width)] min-w-60 max-w-[calc(100vw-24px)] rounded-xl border border-white/10 bg-[#1b3426] p-1.5 shadow-[0_18px_40px_rgba(6,20,12,0.45)] outline-none"
      >
        {model.others.length ? <Menu.Group>
          <Menu.Label className="px-2 pb-1 pt-1.5 text-xs font-medium uppercase tracking-[0.14em] text-slate-500">Switch workspace</Menu.Label>
          {model.others.map((workspace) => <Menu.Item key={workspace.id} className={itemClass} onSelect={() => switchTo(workspace.id)}>
            <Tile name={workspace.name} />
            <span className="min-w-0 flex-1"><span className="block truncate font-medium text-slate-200">{workspace.name}</span><span className="block text-xs capitalize text-slate-500">{workspace.role}</span></span>
          </Menu.Item>)}
        </Menu.Group> : null}
        {model.showNewWorkspace ? <Menu.Item asChild className={itemClass}>
          <Link href="/welcome"><span aria-hidden="true" className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-dashed border-white/20"><Plus size={15} /></span>New workspace</Link>
        </Menu.Item> : null}
        {hasTopSection ? <Menu.Separator className="my-1 h-px bg-white/10" /> : null}
        {model.showAccount ? <Menu.Item asChild className={itemClass}>
          <Link href="/account"><CircleUserRound size={16} aria-hidden="true" className="mx-2 shrink-0" />Account</Link>
        </Menu.Item> : null}
        <Menu.Item className={itemClass} onSelect={() => signOutForm.current?.requestSubmit()}>
          <LogOut size={16} aria-hidden="true" className="mx-2 shrink-0" />Sign out
        </Menu.Item>
      </Menu.Content>
    </Menu.Portal>
  </Menu.Root>;
}

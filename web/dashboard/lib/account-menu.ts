import type { Workspace } from "./account";

export type AccountMenuModel = { current: Workspace | null; title: string; subtitle: string; others: Workspace[]; showNewWorkspace: boolean; showAccount: boolean };

function capitalise(value: string): string {
  return value ? value[0].toUpperCase() + value.slice(1) : value;
}

export function accountMenuModel(input: { kind: "user" | "key"; workspaces: Workspace[]; currentId: string | null }): AccountMenuModel {
  if (input.kind === "key") return { current: null, title: "API key session", subtitle: "", others: [], showNewWorkspace: false, showAccount: false };
  const currentId = input.currentId?.toLowerCase();
  const current = currentId ? input.workspaces.find((w) => w.id.toLowerCase() === currentId) ?? null : null;
  return {
    current,
    title: current?.name ?? "Choose a workspace",
    subtitle: current ? [current.role, current.plan].filter(Boolean).map(capitalise).join(" · ") : "",
    others: input.workspaces.filter((w) => w !== current),
    showNewWorkspace: true,
    showAccount: true,
  };
}

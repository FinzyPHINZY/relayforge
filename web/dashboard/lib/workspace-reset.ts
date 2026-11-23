// A user session whose active workspace is gone (removed, suspended, or a
// malformed id) gets these from org-scoped routes. The dashboard then resets
// the cookie's workspace instead of showing an error with no way out.
const staleWorkspace: Record<number, readonly string[]> = { 400: ["invalid_org", "org_required"], 403: ["not_a_member"] };

export const workspaceResetPath = "/auth/workspace-reset";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function needsWorkspaceReset(kind: "key" | "user", status: number, code: string | undefined): boolean {
  return kind === "user" && code !== undefined && (staleWorkspace[status]?.includes(code) ?? false);
}

/** Keeps the current workspace when it is still listed, else the first one, else null. */
export function workspaceAfterReset(current: string | null, workspaceIds: readonly string[]): string | null {
  const ids = workspaceIds.filter((w) => uuid.test(w)).map((w) => w.toLowerCase());
  const now = current?.toLowerCase() ?? null;
  if (now && ids.includes(now)) return now;
  return ids[0] ?? null;
}

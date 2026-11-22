import { apiJson } from "@/lib/api";
import { saveSession, type SessionValue } from "@/lib/session";
import { uuidPattern } from "@/lib/session-value";

export type Workspace = { id: string; name: string; role: string; plan: string };
export type Account = { user: { id: string; email?: string | null; display_name?: string | null }; workspaces: Workspace[] };

const sessionToken = /^rfs_[A-Za-z0-9_-]{1,508}$/;

// The account call never sends an org header: a stale active org (the user was
// removed from it) must not stop the user from listing or switching workspaces.
export async function fetchAccount(token: string): Promise<Account> {
  const account = await apiJson<Account>({ kind: "user", token, org: null }, "/v1/account");
  return { ...account, workspaces: (account.workspaces ?? []).filter((w) => uuidPattern.test(w.id)) };
}

// Saves a fresh user session and returns where the browser should land next.
// Throws ApiError when the account can't be read; nothing is saved then.
export async function completeSignIn(token: string): Promise<"/dashboard" | "/welcome"> {
  if (!sessionToken.test(token)) throw new Error("Invalid session token");
  const { workspaces } = await fetchAccount(token);
  const org = workspaces[0]?.id.toLowerCase() ?? null;
  const session: SessionValue = { kind: "user", token, org };
  await saveSession(session);
  return org ? "/dashboard" : "/welcome";
}

import { cache } from "react";
import { apiJson } from "@/lib/api";
import { saveSession, type SessionValue } from "@/lib/session";
import { uuidPattern } from "@/lib/session-value";

export type Workspace = { id: string; name: string; role: string; plan: string };
export type Account = { user: { id: string; email?: string | null; display_name?: string | null; avatar_url?: string | null }; workspaces: Workspace[] };

const sessionToken = /^rfs_[A-Za-z0-9_-]{1,508}$/;

// the account call never sends an org header: a stale active org (the user was removed
// from it) must not stop the user from listing or switching workspaces.
export const fetchAccount = cache(async (token: string): Promise<Account> => {
  const account = await apiJson<Account>({ kind: "user", token, org: null }, "/v1/account");
  return { ...account, workspaces: (account.workspaces ?? []).filter((w) => uuidPattern.test(w.id)) };
});

export async function completeSignIn(token: string): Promise<"/dashboard" | "/welcome"> {
  if (!sessionToken.test(token)) throw new Error("Invalid session token");
  const { workspaces } = await fetchAccount(token);
  const org = workspaces[0]?.id.toLowerCase() ?? null;
  const session: SessionValue = { kind: "user", token, org };
  await saveSession(session);
  return org ? "/dashboard" : "/welcome";
}

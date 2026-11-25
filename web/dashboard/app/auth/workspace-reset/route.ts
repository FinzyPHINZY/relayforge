import { ApiError } from "@/lib/api";
import { fetchAccount } from "@/lib/account";
import { redirectTo } from "@/lib/redirect-response";
import { getSession, setActiveOrg } from "@/lib/session";
import { workspaceAfterReset } from "@/lib/workspace-reset";

export const dynamic = "force-dynamic";

// the only side effect is re-sealing the caller's own cookie onto a workspace they
// still belong to, or none, so a cross-site GET can't do more than that.
export async function GET() {
  const session = await getSession();
  if (!session) return redirectTo("/login");
  if (session.kind !== "user") return redirectTo("/dashboard");
  let ids: string[];
  try {
    ids = (await fetchAccount(session.token)).workspaces.map((w) => w.id);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return redirectTo("/login?error=expired");
    return redirectTo("/account");
  }
  const org = workspaceAfterReset(session.org, ids);
  try {
    await setActiveOrg(org);
  } catch {
    return redirectTo("/login?error=expired");
  }
  return redirectTo(org ? "/dashboard" : "/welcome");
}

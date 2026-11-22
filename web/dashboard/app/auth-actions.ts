"use server";

import { revalidatePath } from "next/cache";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { completeSignIn, fetchAccount, type Workspace } from "@/lib/account";
import { apiJson, ApiError, authApi, clientIpFrom, requireSession } from "@/lib/api";
import { magicLinkToken } from "@/lib/magic-link";
import { setActiveOrg } from "@/lib/session";
import { uuidPattern } from "@/lib/session-value";

type SessionBody = { session_token: string; expires_at: string };

async function clientIp(): Promise<string> {
  return clientIpFrom(await headers());
}

// The response is the same page for every address, so it never reveals
// whether an address can sign in. Only input and rate errors are surfaced.
export async function requestMagicLink(formData: FormData) {
  const email = String(formData.get("email") ?? "").trim();
  if (!email || email.length > 254) redirect("/login?error=invalid_email");
  let error: ApiError | null = null;
  try {
    await authApi("/v1/auth/email/start", { email }, await clientIp());
  } catch (caught) {
    if (caught instanceof ApiError) error = caught;
    // Never log the address; the class and message are enough to debug.
    else console.error("magic link request failed:", caught instanceof Error ? `${caught.name}: ${caught.message}` : typeof caught);
  }
  if (error?.status === 422) redirect("/login?error=invalid_email");
  if (error?.status === 429) redirect("/login?error=rate");
  redirect("/auth/check-email");
}

export async function confirmMagicLink(formData: FormData) {
  const token = String(formData.get("token") ?? "");
  if (!magicLinkToken.test(token)) redirect("/login?error=link");
  let destination: string;
  try {
    const body = await authApi<SessionBody>("/v1/auth/email/consume", { token }, await clientIp());
    destination = await completeSignIn(body.session_token);
  } catch (error) {
    if (error instanceof ApiError && error.code === "link_invalid") redirect("/login?error=link");
    if (error instanceof ApiError && error.status === 429) redirect("/login?error=rate");
    redirect("/login?error=unavailable");
  }
  redirect(destination);
}

export async function createFirstWorkspace(formData: FormData) {
  const session = await requireSession();
  if (session.kind !== "user") redirect("/dashboard");
  const name = String(formData.get("name") ?? "").trim();
  if (name.length < 1 || [...name].length > 80) redirect("/welcome?error=name");
  let workspace: Workspace;
  try {
    workspace = await apiJson<Workspace>({ kind: "user", token: session.token, org: null }, "/v1/account/workspaces", { method: "POST", body: JSON.stringify({ name }) });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (error instanceof ApiError && error.code === "workspace_limit") redirect("/welcome?error=limit");
    if (error instanceof ApiError && error.code === "email_required") redirect("/welcome?error=email");
    if (error instanceof ApiError && error.status === 422) redirect("/welcome?error=name");
    redirect("/welcome?error=unavailable");
  }
  if (!uuidPattern.test(workspace.id)) redirect("/welcome?error=unavailable");
  await setActiveOrg(workspace.id);
  revalidatePath("/", "layout");
  redirect("/dashboard");
}

export async function switchWorkspace(formData: FormData) {
  const session = await requireSession();
  if (session.kind !== "user") redirect("/dashboard");
  const org = String(formData.get("org") ?? "").toLowerCase();
  if (!uuidPattern.test(org)) redirect("/dashboard");
  let member = false;
  try {
    const { workspaces } = await fetchAccount(session.token);
    member = workspaces.some((w) => w.id.toLowerCase() === org);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    redirect("/dashboard");
  }
  if (!member) redirect("/dashboard");
  await setActiveOrg(org);
  revalidatePath("/", "layout");
  redirect("/dashboard");
}

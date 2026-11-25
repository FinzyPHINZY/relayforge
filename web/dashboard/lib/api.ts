import { headers as requestHeaders } from "next/headers";
import { redirect } from "next/navigation";
import { cache } from "react";
import { buildApiUrl } from "@/lib/api-url";
import { buildAuthHeaders, forwardedUserAgent } from "@/lib/auth-headers";
import { getSession, type SessionValue } from "@/lib/session";
import type { Principal } from "@/lib/types";
import { needsWorkspaceReset, workspaceResetPath } from "@/lib/workspace-reset";

export { clientIpFrom } from "@/lib/client-ip";

export class ApiError extends Error {
  constructor(public readonly status: number, message: string, public readonly code?: string) {
    super(message);
  }
}

export async function requireSession(): Promise<SessionValue> {
  const session = await getSession();
  if (!session) redirect("/login");
  return session;
}

export async function requireOrgSession(): Promise<SessionValue> {
  const session = await requireSession();
  if (session.kind === "user" && !session.org) redirect("/welcome");
  return session;
}

function apiOrigin(): string {
  return process.env.RELAYFORGE_API_URL ?? "http://127.0.0.1:8080";
}

async function send<T>(url: URL, init: RequestInit, headers: Headers): Promise<T> {
  if (init.body) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      headers,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(8000),
    });
  } catch {
    throw new ApiError(503, "RelayForge API is unavailable", "unavailable");
  }
  if (!response.ok) {
    const problem = (await response.json().catch(() => null)) as { code?: unknown; detail?: unknown; title?: unknown } | null;
    const detail = typeof problem?.detail === "string" ? problem.detail : typeof problem?.title === "string" ? problem.title : `API returned ${response.status}`;
    throw new ApiError(response.status, detail, typeof problem?.code === "string" ? problem.code : undefined);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export async function apiJson<T>(s: SessionValue, path: string, init: RequestInit = {}): Promise<T> {
  const url = buildApiUrl(path, apiOrigin(), "/v1/");
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${s.token}`);
  headers.delete("X-RelayForge-Org");
  if (s.kind === "user" && s.org) headers.set("X-RelayForge-Org", s.org);
  return send<T>(url, init, headers);
}

async function browserUserAgent(): Promise<string | null> {
  try {
    return forwardedUserAgent((await requestHeaders()).get("user-agent"));
  } catch {
    return null;
  }
}

async function authRequest(path: string, clientIp: string): Promise<{ url: URL; headers: Headers }> {
  const url = buildApiUrl(path, apiOrigin(), "/v1/auth/");
  const token = process.env.DASHBOARD_API_TOKEN;
  if (!token) throw new Error("DASHBOARD_API_TOKEN is not set; the dashboard cannot call /v1/auth routes");
  return { url, headers: buildAuthHeaders(token, clientIp, await browserUserAgent()) };
}

export async function authApi<T>(path: `/v1/auth/${string}`, body: unknown, clientIp: string): Promise<T> {
  const { url, headers } = await authRequest(path, clientIp);
  return send<T>(url, { method: "POST", body: JSON.stringify(body ?? {}) }, headers);
}

export async function authGet<T>(path: `/v1/auth/${string}`, clientIp: string): Promise<T> {
  const { url, headers } = await authRequest(path, clientIp);
  return send<T>(url, { method: "GET" }, headers);
}

export async function readApi<T>(s: SessionValue, path: string): Promise<T> {
  try {
    return await apiJson<T>(s, path);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (error instanceof ApiError && needsWorkspaceReset(s.kind, error.status, error.code)) redirect(workspaceResetPath);
    throw error;
  }
}

// cache() keys on arguments by identity, so it takes primitives rather than the
// session object.
const principalFor = cache((kind: "key" | "user", token: string, org: string | null): Promise<Principal> =>
  apiJson<Principal>(kind === "key" ? { kind, token } : { kind, token, org }, "/v1/me"));

export function fetchPrincipal(s: SessionValue): Promise<Principal> {
  return principalFor(s.kind, s.token, s.kind === "user" ? s.org : null);
}

export async function readPrincipal(s: SessionValue): Promise<Principal> {
  try {
    return await fetchPrincipal(s);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    if (error instanceof ApiError && needsWorkspaceReset(s.kind, error.status, error.code)) redirect(workspaceResetPath);
    throw error;
  }
}

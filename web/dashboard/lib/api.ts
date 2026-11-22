import { redirect } from "next/navigation";
import { buildApiUrl } from "@/lib/api-url";
import { getSession, type SessionValue } from "@/lib/session";

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

// Every /v1/auth call carries the dashboard token and the end user's IP (R19).
function authRequest(path: string, clientIp: string): { url: URL; headers: Headers } {
  const url = buildApiUrl(path, apiOrigin(), "/v1/auth/");
  const token = process.env.DASHBOARD_API_TOKEN;
  if (!token) throw new Error("DASHBOARD_API_TOKEN is not set; the dashboard cannot call /v1/auth routes");
  return { url, headers: new Headers({ "X-RelayForge-Dashboard-Token": token, "X-RelayForge-Client-IP": clientIp }) };
}

export async function authApi<T>(path: `/v1/auth/${string}`, body: unknown, clientIp: string): Promise<T> {
  const { url, headers } = authRequest(path, clientIp);
  return send<T>(url, { method: "POST", body: JSON.stringify(body ?? {}) }, headers);
}

export async function authGet<T>(path: `/v1/auth/${string}`, clientIp: string): Promise<T> {
  const { url, headers } = authRequest(path, clientIp);
  return send<T>(url, { method: "GET" }, headers);
}

export async function readApi<T>(s: SessionValue, path: string): Promise<T> {
  try {
    return await apiJson<T>(s, path);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    throw error;
  }
}

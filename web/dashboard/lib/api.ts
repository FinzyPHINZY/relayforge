import { redirect } from "next/navigation";
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

async function send<T>(path: string, init: RequestInit, headers: Headers): Promise<T> {
  if (init.body) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetch(new URL(path, apiOrigin()), {
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
  if (!path.startsWith("/v1/") || path.startsWith("//")) throw new Error("Invalid API path");
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${s.token}`);
  headers.delete("X-RelayForge-Org");
  if (s.kind === "user" && s.org) headers.set("X-RelayForge-Org", s.org);
  return send<T>(path, init, headers);
}

export async function authApi<T>(path: `/v1/auth/${string}`, body: unknown, clientIp: string): Promise<T> {
  if (!path.startsWith("/v1/auth/") || path.includes("..")) throw new Error("Invalid auth API path");
  const token = process.env.DASHBOARD_API_TOKEN;
  if (!token) throw new Error("DASHBOARD_API_TOKEN is not set; the dashboard cannot call /v1/auth routes");
  const headers = new Headers({ "X-RelayForge-Dashboard-Token": token, "X-RelayForge-Client-IP": clientIp });
  return send<T>(path, { method: "POST", body: JSON.stringify(body ?? {}) }, headers);
}

export async function readApi<T>(s: SessionValue, path: string): Promise<T> {
  try {
    return await apiJson<T>(s, path);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    throw error;
  }
}

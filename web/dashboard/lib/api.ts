import { redirect } from "next/navigation";
import { getSessionKey } from "@/lib/session";

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
  }
}

export async function requireKey(): Promise<string> {
  const key = await getSessionKey();
  if (!key) redirect("/login");
  return key;
}

export async function apiJson<T>(key: string, path: string, init: RequestInit = {}): Promise<T> {
  if (!path.startsWith("/v1/") || path.startsWith("//")) throw new Error("Invalid API path");
  const origin = process.env.RELAYFORGE_API_URL ?? "http://127.0.0.1:8080";
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${key}`);
  if (init.body) headers.set("Content-Type", "application/json");
  let response: Response;
  try {
    response = await fetch(new URL(path, origin), {
      ...init,
      headers,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(8000),
    });
  } catch {
    throw new ApiError(503, "RelayForge API is unavailable");
  }
  if (!response.ok) {
    const problem = (await response.json().catch(() => null)) as { detail?: string; title?: string } | null;
    throw new ApiError(response.status, problem?.detail ?? problem?.title ?? `API returned ${response.status}`);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export async function readApi<T>(key: string, path: string): Promise<T> {
  try {
    return await apiJson<T>(key, path);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) redirect("/login?error=expired");
    throw error;
  }
}

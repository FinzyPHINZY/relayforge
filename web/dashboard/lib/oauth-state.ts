import { timingSafeEqual } from "node:crypto";

export type OAuthCookie = { state: string; verifier: string };

const maxLength = 512;

// Constant-time state comparison; unequal lengths and empty values never match.
export function verifyOAuthState(expected: unknown, actual: unknown): boolean {
  if (typeof expected !== "string" || typeof actual !== "string" || !expected || !actual) return false;
  const a = Buffer.from(expected, "utf8");
  const b = Buffer.from(actual, "utf8");
  return a.length === b.length && timingSafeEqual(a, b);
}

export function encodeOAuthCookie(value: OAuthCookie): string {
  return JSON.stringify({ state: value.state, verifier: value.verifier });
}

export function decodeOAuthCookie(raw: string): OAuthCookie | null {
  let parsed: unknown;
  try { parsed = JSON.parse(raw); } catch { return null; }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
  const { state, verifier } = parsed as { state?: unknown; verifier?: unknown };
  const valid = (v: unknown): v is string => typeof v === "string" && v.length > 0 && v.length <= maxLength;
  return valid(state) && valid(verifier) ? { state, verifier } : null;
}

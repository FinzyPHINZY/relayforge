export type SessionValue = { kind: "key"; token: string } | { kind: "user"; token: string; org: string | null };

const keyToken = /^whk_[A-Za-z0-9_-]{1,508}$/;
const userToken = /^rfs_[A-Za-z0-9_-]{1,508}$/;
export const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function encodeSessionValue(v: SessionValue): string {
  return JSON.stringify(v.kind === "key" ? { kind: "key", token: v.token } : { kind: "user", token: v.token, org: v.org });
}

export function decodeSessionValue(raw: string): SessionValue | null {
  if (raw.startsWith("whk_")) return keyToken.test(raw) ? { kind: "key", token: raw } : null;
  let parsed: unknown;
  try { parsed = JSON.parse(raw); } catch { return null; }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
  const value = parsed as { kind?: unknown; token?: unknown; org?: unknown };
  if (typeof value.token !== "string") return null;
  if (value.kind === "key") return keyToken.test(value.token) ? { kind: "key", token: value.token } : null;
  if (value.kind !== "user" || !userToken.test(value.token)) return null;
  if (value.org !== null && (typeof value.org !== "string" || !uuidPattern.test(value.org))) return null;
  return { kind: "user", token: value.token, org: value.org };
}

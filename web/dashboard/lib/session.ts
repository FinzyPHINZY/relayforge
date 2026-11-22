import { cookies } from "next/headers";
import { openSessionPayload, sealSession } from "@/lib/session-crypto";
import { decodeSessionValue, encodeSessionValue, uuidPattern, type SessionValue } from "@/lib/session-value";

export type { SessionValue } from "@/lib/session-value";

const cookieName = "relayforge_session";
const keyLifetimeSeconds = 8 * 60 * 60;
const userLifetimeSeconds = 30 * 24 * 60 * 60;

export function sessionKey(): Buffer {
  const raw = process.env.DASHBOARD_SESSION_KEY;
  const key = raw ? Buffer.from(raw, "base64") : Buffer.alloc(0);
  if (key.length !== 32) {
    throw new Error("DASHBOARD_SESSION_KEY must be a base64-encoded 32-byte key");
  }
  return key;
}

export function secureCookies(): boolean {
  return process.env.DASHBOARD_SECURE_COOKIES === "true" || (process.env.NODE_ENV === "production" && process.env.DASHBOARD_SECURE_COOKIES !== "false");
}

async function readSession(): Promise<{ value: SessionValue; expires: number } | null> {
  const cookie = (await cookies()).get(cookieName)?.value;
  const opened = cookie ? openSessionPayload(cookie, sessionKey()) : null;
  const value = opened ? decodeSessionValue(opened.value) : null;
  return opened && value ? { value, expires: opened.expires } : null;
}

async function writeSession(value: SessionValue, lifetimeSeconds: number): Promise<void> {
  (await cookies()).set(cookieName, sealSession(encodeSessionValue(value), sessionKey(), Date.now(), lifetimeSeconds), {
    httpOnly: true,
    sameSite: "lax",
    secure: secureCookies(),
    path: "/",
    maxAge: lifetimeSeconds,
  });
}

export async function getSession(): Promise<SessionValue | null> {
  return (await readSession())?.value ?? null;
}

export async function saveSession(v: SessionValue): Promise<void> {
  await writeSession(v, v.kind === "user" ? userLifetimeSeconds : keyLifetimeSeconds);
}

// Switching organisation re-seals the cookie with the remaining lifetime, so the
// session never outlives its original expiry.
export async function setActiveOrg(org: string): Promise<void> {
  if (!uuidPattern.test(org)) throw new Error("Invalid organisation id");
  const session = await readSession();
  if (!session || session.value.kind !== "user") throw new Error("No user session");
  const remaining = Math.floor((session.expires - Date.now()) / 1000);
  if (remaining <= 0) throw new Error("No user session");
  await writeSession({ kind: "user", token: session.value.token, org: org.toLowerCase() }, remaining);
}

export async function clearSession(): Promise<void> {
  (await cookies()).delete(cookieName);
}

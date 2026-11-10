import { cookies } from "next/headers";
import { openSession, sealSession } from "@/lib/session-crypto";

const cookieName = "relayforge_session";
const lifetimeSeconds = 8 * 60 * 60;

function sessionKey(): Buffer {
  const raw = process.env.DASHBOARD_SESSION_KEY;
  const key = raw ? Buffer.from(raw, "base64") : Buffer.alloc(0);
  if (key.length !== 32) {
    throw new Error("DASHBOARD_SESSION_KEY must be a base64-encoded 32-byte key");
  }
  return key;
}

export async function getSessionKey(): Promise<string | null> {
  const value = (await cookies()).get(cookieName)?.value;
  return value ? openSession(value, sessionKey()) : null;
}

export async function saveSession(apiKey: string): Promise<void> {
  (await cookies()).set(cookieName, sealSession(apiKey, sessionKey()), {
    httpOnly: true,
    sameSite: "lax",
    secure: process.env.DASHBOARD_SECURE_COOKIES === "true" || (process.env.NODE_ENV === "production" && process.env.DASHBOARD_SECURE_COOKIES !== "false"),
    path: "/",
    maxAge: lifetimeSeconds,
  });
}

export async function clearSession(): Promise<void> {
  (await cookies()).delete(cookieName);
}

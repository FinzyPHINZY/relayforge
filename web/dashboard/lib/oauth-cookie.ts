import { cookies } from "next/headers";
import { decodeOAuthCookie, encodeOAuthCookie, type OAuthCookie } from "@/lib/oauth-state";
import { openSession, sealSession } from "@/lib/session-crypto";
import { secureCookies, sessionKey } from "@/lib/session";

const cookieName = "relayforge_oauth";
const cookiePath = "/auth/github";
const lifetimeSeconds = 600;

export async function saveOAuthCookie(value: OAuthCookie): Promise<void> {
  (await cookies()).set(cookieName, sealSession(encodeOAuthCookie(value), sessionKey(), Date.now(), lifetimeSeconds), {
    httpOnly: true,
    sameSite: "lax",
    secure: secureCookies(),
    path: cookiePath,
    maxAge: lifetimeSeconds,
  });
}

// Reads and always deletes the cookie, so each OAuth state is usable once.
export async function takeOAuthCookie(): Promise<OAuthCookie | null> {
  const jar = await cookies();
  const raw = jar.get(cookieName)?.value;
  jar.set(cookieName, "", { httpOnly: true, sameSite: "lax", secure: secureCookies(), path: cookiePath, maxAge: 0 });
  const opened = raw ? openSession(raw, sessionKey()) : null;
  return opened ? decodeOAuthCookie(opened) : null;
}

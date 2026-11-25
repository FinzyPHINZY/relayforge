// route handlers get no server-action Origin check, so state-changing ones call this:
// fetch metadata must say same-origin (or be absent, for older browsers) and any
// Origin header must be the dashboard's own.
export function isSameOriginRequest(headers: Headers, expected: string): boolean {
  const site = headers.get("sec-fetch-site");
  if (site !== null && site !== "same-origin") return false;
  const origin = headers.get("origin");
  return origin === null || origin === expected;
}

export function expectedOrigin(publicUrl: string | undefined, requestUrl: string): string {
  if (publicUrl) {
    try { return new URL(publicUrl).origin; } catch { /* fall back to the request */ }
  }
  return new URL(requestUrl).origin;
}

// Headers for every /v1/auth call (R19), plus the browser's User-Agent so the
// API records the real device on the session it creates (R36).

const maxUserAgent = 256;

/** The browser's agent, trimmed, stripped of control characters and capped at 256 characters; null when absent. */
export function forwardedUserAgent(value: string | null | undefined): string | null {
  const cleaned = (value ?? "").replace(/[\u0000-\u001f\u007f]/g, "").trim();
  if (!cleaned) return null;
  return [...cleaned].slice(0, maxUserAgent).join("");
}

export function buildAuthHeaders(dashboardToken: string, clientIp: string, userAgent: string | null): Headers {
  const headers = new Headers({ "X-RelayForge-Dashboard-Token": dashboardToken, "X-RelayForge-Client-IP": clientIp });
  if (userAgent) headers.set("User-Agent", userAgent);
  return headers;
}

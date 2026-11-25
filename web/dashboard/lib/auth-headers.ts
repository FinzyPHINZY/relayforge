
const maxUserAgent = 256;

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

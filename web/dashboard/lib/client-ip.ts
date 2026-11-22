import { isIP } from "node:net";

// Caddy appends the connecting address to X-Forwarded-For, so only the last
// entry is trustworthy; earlier entries are client-supplied.
export function clientIpFrom(headers: Headers): string {
  const forwarded = headers.get("x-forwarded-for");
  if (!forwarded) return "";
  const last = forwarded.split(",").at(-1)?.trim() ?? "";
  return isIP(last) ? last : "";
}

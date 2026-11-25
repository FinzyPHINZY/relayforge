// refuses anything that could resolve outside `prefix` on the configured origin: dot
// segments, percent encoding or backslashes in the path part, control characters and
// fragments.
export function buildApiUrl(path: string, origin: string, prefix: "/v1/" | "/v1/auth/"): URL {
  const queryAt = path.indexOf("?");
  const rawPath = queryAt === -1 ? path : path.slice(0, queryAt);
  const invalid =
    !rawPath.startsWith(prefix) ||
    rawPath.startsWith("//") ||
    /[%\\#\s\u0000-\u001f\u007f]/.test(rawPath) ||
    /[#\s\u0000-\u001f\u007f]/.test(path) ||
    rawPath.split("/").some((segment) => segment === "." || segment === "..");
  if (invalid) throw new Error("Invalid API path");
  const base = new URL(origin);
  const url = new URL(path, base);
  if (url.origin !== base.origin || !url.pathname.startsWith(prefix)) throw new Error("Invalid API path");
  return url;
}

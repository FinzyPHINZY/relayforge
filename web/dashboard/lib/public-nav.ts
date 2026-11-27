export const publicNav: { href: string; label: string }[] = [
  { href: "/how-it-works", label: "How it works" },
  { href: "/demo", label: "Guided demo" },
  { href: "/docs", label: "SDK docs" },
];

function normalize(path: string): string {
  const withoutHash = path.split("#")[0];
  return withoutHash.length > 1 ? withoutHash.replace(/\/+$/, "") : withoutHash || "/";
}

export function isActiveNav(pathname: string, href: string): boolean {
  return normalize(pathname) === normalize(href);
}

/** Where a public page should land after navigating to url: the top, or a decoded "#id". */
export function scrollTargetFor(url: string): "top" | string {
  const at = url.indexOf("#");
  if (at === -1 || at === url.length - 1) return "top";
  try {
    return decodeURIComponent(url.slice(at));
  } catch {
    return "top";
  }
}

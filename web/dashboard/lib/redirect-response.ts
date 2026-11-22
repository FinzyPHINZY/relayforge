import { NextResponse } from "next/server";

// A relative Location keeps redirects on whatever host the browser used,
// rather than the internal address the server is bound to.
export function redirectTo(path: `/${string}` | URL, status: 302 | 303 = 302): NextResponse {
  return new NextResponse(null, { status, headers: { Location: String(path), "Cache-Control": "no-store" } });
}

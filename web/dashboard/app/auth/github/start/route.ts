import type { NextRequest } from "next/server";
import { ApiError, authApi, clientIpFrom } from "@/lib/api";
import { saveOAuthCookie } from "@/lib/oauth-cookie";
import { redirectTo } from "@/lib/redirect-response";
import { expectedOrigin, isSameOriginRequest } from "@/lib/same-origin";

export const dynamic = "force-dynamic";

type Start = { authorize_url: string; state: string; verifier: string };

function authorizeUrl(raw: unknown): URL | null {
  if (typeof raw !== "string") return null;
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && url.host === "github.com" && url.pathname === "/login/oauth/authorize" ? url : null;
  } catch {
    return null;
  }
}

// POST only; there is deliberately no GET, so a link or prefetch can't start a flow,
// and cross-site posts are refused.
export async function POST(request: NextRequest) {
  if (!isSameOriginRequest(request.headers, expectedOrigin(process.env.DASHBOARD_PUBLIC_URL, request.url))) return redirectTo("/login?error=oauth", 303);
  let body: Start;
  try {
    body = await authApi<Start>("/v1/auth/github/start", {}, clientIpFrom(request.headers));
  } catch (error) {
    return redirectTo(error instanceof ApiError && error.status === 429 ? "/login?error=rate" : "/login?error=oauth", 303);
  }
  const target = authorizeUrl(body.authorize_url);
  if (!target || !body.state || !body.verifier) return redirectTo("/login?error=oauth", 303);
  await saveOAuthCookie({ state: body.state, verifier: body.verifier });
  return redirectTo(target, 303);
}

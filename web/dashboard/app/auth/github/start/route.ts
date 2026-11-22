import type { NextRequest } from "next/server";
import { ApiError, authApi, clientIpFrom } from "@/lib/api";
import { saveOAuthCookie } from "@/lib/oauth-cookie";
import { redirectTo } from "@/lib/redirect-response";

export const dynamic = "force-dynamic";

type Start = { authorize_url: string; state: string; verifier: string };

function authorizeUrl(raw: unknown): URL | null {
  if (typeof raw !== "string") return null;
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && url.pathname === "/login/oauth/authorize" ? url : null;
  } catch {
    return null;
  }
}

async function start(request: NextRequest, status: 302 | 303) {
  let body: Start;
  try {
    body = await authApi<Start>("/v1/auth/github/start", {}, clientIpFrom(request.headers));
  } catch (error) {
    return redirectTo(error instanceof ApiError && error.status === 429 ? "/login?error=rate" : "/login?error=oauth", 303);
  }
  const target = authorizeUrl(body.authorize_url);
  if (!target || !body.state || !body.verifier) return redirectTo("/login?error=oauth", 303);
  await saveOAuthCookie({ state: body.state, verifier: body.verifier });
  return redirectTo(target, status);
}

// The login page submits a POST form, so link prefetching never starts a flow.
export async function POST(request: NextRequest) {
  return start(request, 303);
}

export async function GET(request: NextRequest) {
  return start(request, 302);
}

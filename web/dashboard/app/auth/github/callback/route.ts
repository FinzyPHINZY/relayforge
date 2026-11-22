import type { NextRequest } from "next/server";
import { completeSignIn } from "@/lib/account";
import { ApiError, authApi, clientIpFrom } from "@/lib/api";
import { takeOAuthCookie } from "@/lib/oauth-cookie";
import { verifyOAuthState } from "@/lib/oauth-state";
import { redirectTo } from "@/lib/redirect-response";

export const dynamic = "force-dynamic";

type SessionBody = { session_token: string; expires_at: string };

export async function GET(request: NextRequest) {
  const params = request.nextUrl.searchParams;
  const saved = await takeOAuthCookie();
  // `error` is set when the user declines on GitHub.
  if (params.has("error") || !saved || !verifyOAuthState(saved.state, params.get("state"))) return redirectTo("/login?error=oauth");
  const code = params.get("code");
  if (!code || code.length > 512) return redirectTo("/login?error=oauth");
  let destination: string;
  try {
    const body = await authApi<SessionBody>("/v1/auth/github/callback", { code, verifier: saved.verifier }, clientIpFrom(request.headers));
    destination = await completeSignIn(body.session_token);
  } catch (error) {
    if (error instanceof ApiError && error.code === "github_email_unverified") return redirectTo("/login?error=github_email");
    if (error instanceof ApiError && error.status === 429) return redirectTo("/login?error=rate");
    if (error instanceof ApiError && error.status >= 500) return redirectTo("/login?error=unavailable");
    return redirectTo("/login?error=oauth");
  }
  return redirectTo(destination as `/${string}`);
}

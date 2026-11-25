import { ArrowRight, KeyRound, LockKeyhole, Mail } from "lucide-react";
import { headers } from "next/headers";
import Link from "next/link";
import { signInWithKey } from "@/app/actions";
import { requestMagicLink } from "@/app/auth-actions";
import { AuthShell, GitHubMark } from "@/components/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { authGet, clientIpFrom } from "@/lib/api";

type Providers = { github: boolean; email: boolean };

const messages: Record<string, string> = {
  invalid: "That API key is invalid or lacks the metrics:read scope.",
  expired: "Your session has expired. Sign in again.",
  unavailable: "RelayForge is unavailable. Try again in a moment.",
  oauth: "GitHub sign-in didn't complete. Try again.",
  github_email: "Your GitHub account has no verified email. Add one on GitHub or use an email link.",
  link: "That sign-in link has expired or was already used. Request a new one.",
  rate: "Too many attempts. Wait a few minutes and try again.",
  invalid_email: "Enter a valid email address.",
};

async function loadProviders(): Promise<Providers> {
  try {
    const providers = await authGet<Partial<Providers>>("/v1/auth/providers", clientIpFrom(await headers()));
    return { github: providers.github === true, email: providers.email === true };
  } catch {
    return { github: false, email: false };
  }
}

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ error?: string; method?: string }> }) {
  const { error, method } = await searchParams;
  const message = error && Object.hasOwn(messages, error) ? messages[error] : null;
  const providers = await loadProviders();
  const hasProvider = providers.github || providers.email;
  const showKey = !hasProvider || method === "key" || error === "invalid";
  const footer = <><span>Need a quick tour first?</span><Link href="/demo">Explore the guided demo <ArrowRight size={14} aria-hidden="true" /></Link></>;
  const alert = message ? <p role="alert" className="login-error">{message}</p> : null;

  if (showKey) {
    return <AuthShell labelledBy="login-heading" footer={footer}>
      <div className="login-form-icon"><KeyRound size={24} aria-hidden="true" /></div><span className="login-kicker">WELCOME BACK</span><h2 id="login-heading">Sign in to your workspace</h2><p>Use your scoped RelayForge API key to inspect deliveries and manage endpoints.</p>
      <form action={signInWithKey} className="login-form"><label htmlFor="apiKey">API key</label><Input id="apiKey" name="apiKey" type="password" autoComplete="off" placeholder="whk_…" required /><Button className="login-submit">Open console <ArrowRight size={17} aria-hidden="true" /></Button></form>
      {alert}
      <p className="login-security"><LockKeyhole size={15} aria-hidden="true" /> Your key is encrypted in an HTTP-only session cookie. Browser code never calls the API with it.</p>
      {hasProvider ? <p className="login-alt"><Link href="/login">{providers.github ? "Sign in with GitHub or email instead" : "Sign in with an email link instead"}</Link></p> : null}
    </AuthShell>;
  }

  const subtext = providers.github && providers.email ? "Use GitHub or a one-time email link. No password needed." : providers.github ? "Use your GitHub account. No password needed." : "Use a one-time email link. No password needed.";
  return <AuthShell labelledBy="login-heading" footer={footer}>
    <span className="login-kicker">WELCOME</span><h2 id="login-heading">Sign in to RelayForge</h2><p>{subtext}</p>
    <div className="login-methods">
      {providers.github ? <form action="/auth/github/start" method="post"><button type="submit" className="login-github"><GitHubMark /> Continue with GitHub</button></form> : null}
      {providers.github && providers.email ? <div className="login-divider" role="separator" aria-label="or"><span>or</span></div> : null}
      {providers.email ? <form action={requestMagicLink} className="login-form login-email-form"><label htmlFor="email">Email address</label><Input id="email" name="email" type="email" inputMode="email" autoComplete="email" maxLength={254} placeholder="you@company.com" required /><Button className="login-submit">Email me a sign-in link <Mail size={17} aria-hidden="true" /></Button></form> : null}
    </div>
    {alert}
    <p className="login-security"><LockKeyhole size={15} aria-hidden="true" /> Your session is encrypted in an HTTP-only cookie. Browser code never sees your sign-in token.</p>
    <hr className="login-rule" />
    <p className="login-alt login-alt-key"><Link href="/login?method=key"><KeyRound size={14} aria-hidden="true" /> Sign in with an API key</Link></p>
  </AuthShell>;
}

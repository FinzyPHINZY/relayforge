import assert from "node:assert/strict";
import { test } from "node:test";
import { buildApiUrl } from "./api-url.ts";

const origin = "http://api.internal:8080";

test("valid paths resolve against the configured origin", () => {
  assert.equal(buildApiUrl("/v1/auth/email/start", origin, "/v1/auth/").href, "http://api.internal:8080/v1/auth/email/start");
  assert.equal(buildApiUrl("/v1/events?cursor=a%2Fb&limit=5", origin, "/v1/").href, "http://api.internal:8080/v1/events?cursor=a%2Fb&limit=5");
});

test("encoded or dot segments are rejected", () => {
  for (const path of ["/v1/auth/%2e%2e/account/sign-out", "/v1/auth/../account/sign-out", "/v1/auth/./providers", "/v1/auth/a/..", "/v1/%2e%2e/health", "/v1/auth/\\..\\account"]) {
    assert.throws(() => buildApiUrl(path, origin, path.startsWith("/v1/auth/") ? "/v1/auth/" : "/v1/"), /Invalid API path/, path);
  }
});

test("paths outside the prefix or origin are rejected", () => {
  for (const [path, prefix] of [["/v1/account", "/v1/auth/"], ["//evil.example/v1/auth/x", "/v1/auth/"], ["https://evil.example/v1/x", "/v1/"], ["/health/live", "/v1/"], ["/v1/x\n", "/v1/"], ["/v1/x#frag", "/v1/"]]) {
    assert.throws(() => buildApiUrl(path, origin, prefix), /Invalid API path/, path);
  }
});

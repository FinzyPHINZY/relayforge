import assert from "node:assert/strict";
import { test } from "node:test";
import { buildAuthHeaders, forwardedUserAgent } from "./auth-headers.ts";

const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";

test("forwardedUserAgent keeps a normal browser agent", () => {
  assert.equal(forwardedUserAgent(chrome), chrome);
  assert.equal(forwardedUserAgent(`  ${chrome}  `), chrome);
});

test("forwardedUserAgent omits missing or blank agents", () => {
  assert.equal(forwardedUserAgent(null), null);
  assert.equal(forwardedUserAgent(undefined), null);
  assert.equal(forwardedUserAgent("   "), null);
});

test("forwardedUserAgent truncates to 256 characters", () => {
  assert.equal(forwardedUserAgent("a".repeat(1000)), "a".repeat(256));
  assert.equal([...forwardedUserAgent("é".repeat(300))].length, 256);
});

test("forwardedUserAgent drops control characters", () => {
  assert.equal(forwardedUserAgent("Agent\u0000/1\u007f.0"), "Agent/1.0");
});

test("buildAuthHeaders always sends the dashboard token and client IP", () => {
  const h = buildAuthHeaders("tok", "203.0.113.9", null);
  assert.equal(h.get("X-RelayForge-Dashboard-Token"), "tok");
  assert.equal(h.get("X-RelayForge-Client-IP"), "203.0.113.9");
  assert.equal(h.has("User-Agent"), false);
});

test("buildAuthHeaders forwards the browser agent when present", () => {
  const h = buildAuthHeaders("tok", "", chrome);
  assert.equal(h.get("User-Agent"), chrome);
  assert.equal(h.get("X-RelayForge-Client-IP"), "");
});

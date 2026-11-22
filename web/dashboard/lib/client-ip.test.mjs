import assert from "node:assert/strict";
import { test } from "node:test";
import { clientIpFrom } from "./client-ip.ts";

test("uses the last X-Forwarded-For entry appended by the nearest proxy", () => {
  assert.equal(clientIpFrom(new Headers({ "x-forwarded-for": "203.0.113.9, 198.51.100.7" })), "198.51.100.7");
  assert.equal(clientIpFrom(new Headers({ "x-forwarded-for": " 2001:db8::1 " })), "2001:db8::1");
});

test("returns an empty string when the last entry is missing or not an IP", () => {
  assert.equal(clientIpFrom(new Headers()), "");
  assert.equal(clientIpFrom(new Headers({ "x-forwarded-for": "198.51.100.7, " })), "");
  assert.equal(clientIpFrom(new Headers({ "x-forwarded-for": "198.51.100.7, evil.example" })), "");
});

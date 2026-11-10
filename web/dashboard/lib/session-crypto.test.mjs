import assert from "node:assert/strict";
import { test } from "node:test";
import { sealSession, openSession } from "./session-crypto.ts";

const secret = Buffer.alloc(32, 7);

test("session encrypts the key and expires after eight hours", () => {
  const value = sealSession("whk_example", secret, 1000);
  assert.equal(value.includes("whk_example"), false);
  assert.equal(openSession(value, secret, 1001), "whk_example");
  assert.equal(openSession(value, secret, 1000 + 8 * 60 * 60 * 1000), null);
});

test("tampering or changing the server key invalidates a session", () => {
  const value = sealSession("whk_example", secret, 1000);
  assert.equal(openSession(value, Buffer.alloc(32, 8), 1001), null);
  const bytes = Buffer.from(value, "base64url");
  bytes[bytes.length - 1] ^= 1;
  assert.equal(openSession(bytes.toString("base64url"), secret, 1001), null);
});

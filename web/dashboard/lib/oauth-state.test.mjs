import assert from "node:assert/strict";
import { test } from "node:test";
import { decodeOAuthCookie, encodeOAuthCookie, verifyOAuthState } from "./oauth-state.ts";

const state = "s".repeat(43);

test("matching states verify", () => {
  assert.equal(verifyOAuthState(state, state), true);
});

test("different states are rejected", () => {
  assert.equal(verifyOAuthState(state, "t".repeat(43)), false);
  assert.equal(verifyOAuthState(state, state.slice(1)), false);
  assert.equal(verifyOAuthState(state, state + "s"), false);
});

test("empty states are rejected", () => {
  assert.equal(verifyOAuthState("", ""), false);
  assert.equal(verifyOAuthState(state, ""), false);
  assert.equal(verifyOAuthState("", state), false);
  assert.equal(verifyOAuthState(state, undefined), false);
  assert.equal(verifyOAuthState(state, null), false);
});

test("the cookie payload round-trips", () => {
  const value = { state, verifier: "v".repeat(43) };
  assert.deepEqual(decodeOAuthCookie(encodeOAuthCookie(value)), value);
});

test("malformed cookie payloads are rejected", () => {
  for (const raw of ["", "null", "[]", "not json", '{"state":"a"}', '{"state":"","verifier":"b"}', '{"state":1,"verifier":"b"}', JSON.stringify({ state: "a".repeat(600), verifier: "b" })]) {
    assert.equal(decodeOAuthCookie(raw), null, raw);
  }
});

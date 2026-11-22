import assert from "node:assert/strict";
import { test } from "node:test";
import { decodeSessionValue, encodeSessionValue } from "./session-value.ts";

const rfs = "rfs_" + "a".repeat(43);
const whk = "whk_" + "b".repeat(43);
const org = "6f1c2d3e-4a5b-4c6d-8e7f-901234567890";

test("a user session round-trips", () => {
  for (const value of [{ kind: "user", token: rfs, org }, { kind: "user", token: rfs, org: null }]) {
    assert.deepEqual(decodeSessionValue(encodeSessionValue(value)), value);
  }
});

test("a key session round-trips", () => {
  assert.deepEqual(decodeSessionValue(encodeSessionValue({ kind: "key", token: whk })), { kind: "key", token: whk });
});

test("a legacy plain API key decodes to a key session", () => {
  assert.deepEqual(decodeSessionValue("whk_abc"), { kind: "key", token: "whk_abc" });
});

test("a user session without a token is rejected", () => {
  assert.equal(decodeSessionValue('{"kind":"user"}'), null);
  assert.equal(decodeSessionValue(JSON.stringify({ kind: "user", org })), null);
});

test("a token whose prefix does not match its kind is rejected", () => {
  assert.equal(decodeSessionValue(JSON.stringify({ kind: "key", token: rfs })), null);
  assert.equal(decodeSessionValue(JSON.stringify({ kind: "user", token: whk, org: null })), null);
  assert.equal(decodeSessionValue(rfs), null);
});

test("malformed values are rejected", () => {
  for (const raw of ["", "not json", "null", "[]", '"whk_abc"', JSON.stringify({ kind: "admin", token: whk }), JSON.stringify({ kind: "user", token: rfs, org: "not-a-uuid" }), JSON.stringify({ kind: "user", token: rfs + "\r\nX: y", org: null }), "whk_abc def"]) {
    assert.equal(decodeSessionValue(raw), null, raw);
  }
});

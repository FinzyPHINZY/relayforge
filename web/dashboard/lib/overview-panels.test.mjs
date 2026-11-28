import assert from "node:assert/strict";
import { test } from "node:test";
import { panelResult } from "./overview-panels.ts";

test("a fulfilled read is available with its value", () => {
  assert.deepEqual(panelResult({ status: "fulfilled", value: { n: 1 } }), { available: true, value: { n: 1 } });
});

test("a 403 read degrades to unavailable", () => {
  const reason = Object.assign(new Error("API key lacks required scope"), { status: 403, code: "forbidden" });
  assert.deepEqual(panelResult({ status: "rejected", reason }), { available: false });
});

test("any other failure is rethrown unchanged", () => {
  const reason = Object.assign(new Error("RelayForge API is unavailable"), { status: 503 });
  assert.throws(() => panelResult({ status: "rejected", reason }), (error) => error === reason);
});

test("a redirect signal without a status is rethrown", () => {
  const reason = Object.assign(new Error("NEXT_REDIRECT"), { digest: "NEXT_REDIRECT;replace;/login;307;" });
  assert.throws(() => panelResult({ status: "rejected", reason }), (error) => error === reason);
});

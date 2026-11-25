import assert from "node:assert/strict";
import { test } from "node:test";
import { diffMemberUpdate } from "./team-form.ts";

test("only changed fields are sent", () => {
  assert.deepEqual(diffMemberUpdate({ role: "admin", status: "active" }, { role: "admin", status: "suspended" }), { status: "suspended" });
  assert.deepEqual(diffMemberUpdate({ role: "viewer", status: "active" }, { role: "operator", status: "active" }), { role: "operator" });
  assert.deepEqual(diffMemberUpdate({ role: "viewer", status: "active" }, { role: "admin", status: "suspended" }), { role: "admin", status: "suspended" });
});

test("an unchanged form sends nothing", () => {
  assert.deepEqual(diffMemberUpdate({ role: "operator", status: "active" }, { role: "operator", status: "active" }), {});
});

test("an owner's role is never sent from the form", () => {
  assert.deepEqual(diffMemberUpdate({ role: "owner", status: "active" }, { role: "admin", status: "active" }), {});
  assert.deepEqual(diffMemberUpdate({ role: "owner", status: "active" }, { role: null, status: "suspended" }), { status: "suspended" });
  assert.equal(diffMemberUpdate({ role: "admin", status: "active" }, { role: "owner", status: "active" }), null);
});

test("missing fields are left alone and invalid values are refused", () => {
  assert.deepEqual(diffMemberUpdate({ role: "viewer", status: "active" }, { role: null, status: null }), {});
  assert.equal(diffMemberUpdate({ role: "viewer", status: "active" }, { role: "superuser", status: "active" }), null);
  assert.equal(diffMemberUpdate({ role: "viewer", status: "active" }, { role: "viewer", status: "deleted" }), null);
  assert.equal(diffMemberUpdate({ role: "nope", status: "active" }, { role: "viewer", status: "active" }), null);
});

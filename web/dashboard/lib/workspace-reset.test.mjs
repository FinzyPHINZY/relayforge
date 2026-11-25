import assert from "node:assert/strict";
import { test } from "node:test";
import { needsWorkspaceReset, workspaceAfterReset } from "./workspace-reset.ts";

test("stale-workspace codes on a user session trigger a reset", () => {
  assert.equal(needsWorkspaceReset("user", 403, "not_a_member"), true);
  assert.equal(needsWorkspaceReset("user", 400, "invalid_org"), true);
  assert.equal(needsWorkspaceReset("user", 400, "org_required"), true);
});

test("other failures and key sessions do not", () => {
  assert.equal(needsWorkspaceReset("user", 403, "forbidden"), false);
  assert.equal(needsWorkspaceReset("user", 401, "unauthorized"), false);
  assert.equal(needsWorkspaceReset("user", 503, "unavailable"), false);
  assert.equal(needsWorkspaceReset("user", 403, undefined), false);
  assert.equal(needsWorkspaceReset("user", 500, "not_a_member"), false);
  assert.equal(needsWorkspaceReset("key", 403, "not_a_member"), false);
});

test("reset keeps a still-valid workspace, else picks the first, else none", () => {
  const a = "11111111-1111-4111-8111-111111111111";
  const b = "22222222-2222-4222-8222-222222222222";
  assert.equal(workspaceAfterReset(b, [a, b]), b);
  assert.equal(workspaceAfterReset(b.toUpperCase(), [a, b]), b);
  assert.equal(workspaceAfterReset("33333333-3333-4333-8333-333333333333", [a, b]), a);
  assert.equal(workspaceAfterReset(null, [a.toUpperCase()]), a);
  assert.equal(workspaceAfterReset(a, []), null);
  assert.equal(workspaceAfterReset(a, ["not-a-uuid"]), null);
});

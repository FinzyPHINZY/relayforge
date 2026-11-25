import assert from "node:assert/strict";
import { test } from "node:test";
import { allRoles, allScopes, allowedRoles, allowedScopes, offeredScopes } from "./key-form.ts";

const reads = ["events:read", "endpoints:read", "deliveries:read", "metrics:read", "team:read"];

test("allowedRoles returns roles at or below the caller's role", () => {
  assert.deepEqual(allowedRoles("owner"), ["owner", "admin", "operator", "viewer"]);
  assert.deepEqual(allowedRoles("admin"), ["admin", "operator", "viewer"]);
  assert.deepEqual(allowedRoles("operator"), ["operator", "viewer"]);
  assert.deepEqual(allowedRoles("viewer"), ["viewer"]);
  assert.deepEqual(allowedRoles(""), []);
  assert.deepEqual(allowedRoles("superuser"), []);
});

test("owner and admin keys may hold every scope", () => {
  assert.deepEqual(allowedScopes("owner"), allScopes);
  assert.deepEqual(allowedScopes("admin"), allScopes);
  assert.equal(allScopes.length, 11);
});

test("operator keys exclude team:write and keys:write", () => {
  const scopes = allowedScopes("operator");
  assert.ok(!scopes.includes("team:write"));
  assert.ok(!scopes.includes("keys:write"));
  assert.deepEqual(scopes, allScopes.filter((s) => s !== "team:write" && s !== "keys:write"));
});

test("viewer keys hold only the five read scopes, without keys:read", () => {
  assert.deepEqual([...allowedScopes("viewer")].sort(), [...reads].sort());
  assert.ok(!allowedScopes("viewer").includes("keys:read"));
});

test("unknown roles allow nothing", () => {
  assert.deepEqual(allowedScopes(""), []);
  assert.deepEqual(allowedScopes("root"), []);
});

test("offeredScopes intersects the role policy with the caller's own scopes", () => {
  assert.deepEqual(offeredScopes("admin", ["events:read", "keys:write", "bogus"]), ["events:read", "keys:write"]);
  assert.deepEqual(offeredScopes("viewer", ["events:read", "events:write", "keys:read"]), ["events:read"]);
  assert.deepEqual(allRoles, ["owner", "admin", "operator", "viewer"]);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { keyStatus, lastUsedLabel } from "./key-summary.ts";

const now = new Date("2026-11-27T12:00:00Z");
const base = {
  id: "7b0c7e8e-1f3a-4c55-9a43-0c3d7b1b2a10",
  name: "CI deploys",
  prefix: "rf_live_abc",
  role: "operator",
  scopes: ["events:read"],
  created_by: null,
  created_at: "2026-11-01T09:00:00Z",
  last_used_at: null,
  expires_at: null,
  revoked_at: null,
};

test("a revoked key is Revoked with the danger tone", () => {
  assert.deepEqual(keyStatus({ ...base, revoked_at: "2026-11-20T09:00:00Z" }, now), { label: "Revoked", tone: "danger" });
});

test("revoked wins over expired", () => {
  assert.deepEqual(keyStatus({ ...base, revoked_at: "2026-11-20T09:00:00Z", expires_at: "2026-11-21T09:00:00Z" }, now), { label: "Revoked", tone: "danger" });
});

test("a key past its expiry is Expired with the warning tone", () => {
  assert.deepEqual(keyStatus({ ...base, expires_at: "2026-11-27T11:59:59Z" }, now), { label: "Expired", tone: "warning" });
});

test("a key with a future or no expiry is Active", () => {
  assert.deepEqual(keyStatus({ ...base, expires_at: "2026-12-27T12:00:00Z" }, now), { label: "Active", tone: "neutral" });
  assert.deepEqual(keyStatus(base, now), { label: "Active", tone: "neutral" });
});

test("lastUsedLabel says Never for an unused key", () => {
  assert.equal(lastUsedLabel(base, now), "Never");
});

test("lastUsedLabel is relative", () => {
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-11-27T10:00:00Z" }, now), "2h ago");
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-11-24T12:00:00Z" }, now), "3d ago");
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-11-27T11:45:00Z" }, now), "15m ago");
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-11-27T11:59:40Z" }, now), "Just now");
});

test("lastUsedLabel treats a slightly future timestamp as just now", () => {
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-11-27T12:00:05Z" }, now), "Just now");
});

test("lastUsedLabel falls back to months and years for old use", () => {
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2026-08-27T12:00:00Z" }, now), "3mo ago");
  assert.equal(lastUsedLabel({ ...base, last_used_at: "2024-11-27T12:00:00Z" }, now), "2y ago");
});

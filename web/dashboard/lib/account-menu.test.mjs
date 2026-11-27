import assert from "node:assert/strict";
import { test } from "node:test";
import { accountMenuModel } from "./account-menu.ts";

const a = { id: "7b0c7e8e-1f3a-4c55-9a43-0c3d7b1b2a10", name: "Acme", role: "owner", plan: "free" };
const b = { id: "8c1d8f9f-2a4b-4d66-8b54-1d4e8c2c3b21", name: "Beta", role: "operator", plan: "pro" };
const c = { id: "9d2e9a0a-3b5c-4e77-9c65-2e5f9d3d4c32", name: "Gamma", role: "viewer", plan: "free" };

test("a user sees the current workspace with its role and plan, and the rest to switch to", () => {
  const model = accountMenuModel({ kind: "user", workspaces: [a, b, c], currentId: a.id });
  assert.equal(model.title, "Acme");
  assert.equal(model.subtitle, "Owner · Free");
  assert.deepEqual(model.others, [b, c]);
  assert.equal(model.showNewWorkspace, true);
  assert.equal(model.showAccount, true);
});

test("the current workspace matches regardless of id case", () => {
  const model = accountMenuModel({ kind: "user", workspaces: [a, b], currentId: b.id.toUpperCase() });
  assert.equal(model.title, "Beta");
  assert.equal(model.subtitle, "Operator · Pro");
  assert.deepEqual(model.others, [a]);
});

test("a user with one workspace has nothing to switch to", () => {
  const model = accountMenuModel({ kind: "user", workspaces: [a], currentId: a.id });
  assert.equal(model.title, "Acme");
  assert.deepEqual(model.others, []);
  assert.equal(model.showNewWorkspace, true);
});

test("a user without a current workspace is asked to choose one from all", () => {
  const model = accountMenuModel({ kind: "user", workspaces: [a, b, c], currentId: null });
  assert.equal(model.title, "Choose a workspace");
  assert.equal(model.subtitle, "");
  assert.deepEqual(model.others, [a, b, c]);
});

test("an api key session only offers sign out", () => {
  const model = accountMenuModel({ kind: "key", workspaces: [a, b], currentId: a.id });
  assert.equal(model.title, "API key session");
  assert.equal(model.subtitle, "");
  assert.deepEqual(model.others, []);
  assert.equal(model.showNewWorkspace, false);
  assert.equal(model.showAccount, false);
});

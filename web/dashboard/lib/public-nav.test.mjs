import assert from "node:assert/strict";
import test from "node:test";
import { isActiveNav, publicNav } from "./public-nav.ts";

test("isActiveNav matches exact path, trailing slash and hash", () => {
  assert.equal(isActiveNav("/docs", "/docs"), true);
  assert.equal(isActiveNav("/docs/", "/docs"), true);
  assert.equal(isActiveNav("/docs#receive", "/docs"), true);
  assert.equal(isActiveNav("/", "/docs"), false);
  assert.equal(isActiveNav("/demo", "/docs"), false);
});

test("publicNav lists the three public pages in order", () => {
  assert.deepEqual(publicNav.map((n) => n.href), ["/how-it-works", "/demo", "/docs"]);
});

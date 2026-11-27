import assert from "node:assert/strict";
import test from "node:test";
import { isActiveNav, publicNav, scrollTargetFor } from "./public-nav.ts";

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

test("scrollTargetFor returns top without a hash and the decoded hash otherwise", () => {
  assert.equal(scrollTargetFor("/docs"), "top");
  assert.equal(scrollTargetFor("/docs#receive"), "#receive");
  assert.equal(scrollTargetFor("/docs#%E2%9C%93"), "#✓");
  assert.equal(scrollTargetFor("/docs#%E0%A4%A"), "top");
});

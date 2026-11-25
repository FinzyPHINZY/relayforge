import assert from "node:assert/strict";
import { test } from "node:test";
import { isSameOriginRequest, expectedOrigin } from "./same-origin.ts";

const origin = "https://app.relayforge.site";
const h = (init) => new Headers(init);

test("same-origin requests are allowed", () => {
  assert.equal(isSameOriginRequest(h({ "sec-fetch-site": "same-origin", origin }), origin), true);
  assert.equal(isSameOriginRequest(h({ "sec-fetch-site": "same-origin" }), origin), true);
});

test("requests without fetch metadata or Origin are allowed", () => {
  assert.equal(isSameOriginRequest(h({}), origin), true);
});

test("cross-site and same-site requests are rejected", () => {
  for (const site of ["cross-site", "same-site", "none"]) {
    assert.equal(isSameOriginRequest(h({ "sec-fetch-site": site, origin }), origin), false, site);
  }
});

test("a foreign or opaque Origin is rejected", () => {
  assert.equal(isSameOriginRequest(h({ origin: "https://evil.example" }), origin), false);
  assert.equal(isSameOriginRequest(h({ "sec-fetch-site": "same-origin", origin: "https://evil.example" }), origin), false);
  assert.equal(isSameOriginRequest(h({ origin: "null" }), origin), false);
});

test("the expected origin prefers the public URL", () => {
  assert.equal(expectedOrigin("https://app.relayforge.site/some/path", "http://0.0.0.0:3000/auth/github/start"), origin);
  assert.equal(expectedOrigin(undefined, "http://localhost:3000/auth/github/start"), "http://localhost:3000");
  assert.equal(expectedOrigin("not a url", "http://localhost:3000/x"), "http://localhost:3000");
});

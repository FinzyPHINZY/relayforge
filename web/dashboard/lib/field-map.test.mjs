import assert from "node:assert/strict";
import test from "node:test";
import { parseFieldMap } from "./field-map.ts";

test("accepts a bounded top-level mapping", () => {
  assert.deepEqual(parseFieldMap('{"order_id":"id"}'), { order_id: "id" });
  assert.deepEqual(parseFieldMap(""), {});
});

test("rejects executable or nested mapping shapes", () => {
  for (const input of ['[]', '{"x.y":"id"}', '{"id":{"source":"id"}}', '{"id":"user.name"}', 'null']) {
    assert.throws(() => parseFieldMap(input));
  }
});

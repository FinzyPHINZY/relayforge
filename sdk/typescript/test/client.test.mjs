import assert from "node:assert/strict";
import test from "node:test";
import { Publisher, RelayForgeError } from "../src/index.ts";

test("publisher retries with one idempotency key", async () => {
  const keys = [];
  const client = new Publisher("https://relayforge.example", "whk_test", {
    fetch: async (_url, init) => {
      keys.push(init.headers["Idempotency-Key"]);
      if (keys.length === 1) return new Response("", { status: 503 });
      return Response.json({ id: "event-1", type: "order.created", status: "accepted", created_at: new Date().toISOString() }, { status: 202 });
    },
  });
  const result = await client.publish("order.created", { order_id: "1" });
  assert.equal(result.id, "event-1");
  assert.equal(keys.length, 2);
  assert.equal(keys[0], keys[1]);
});

test("publisher exposes API problem details without retrying conflicts", async () => {
  let calls = 0;
  const client = new Publisher("https://relayforge.example", "whk_test", {
    fetch: async () => {
      calls++;
      return Response.json({ code: "idempotency_conflict", detail: "Key already used" }, { status: 409 });
    },
  });
  await assert.rejects(client.publish("order.created", { id: 1 }, { idempotencyKey: "same" }), (error) => error instanceof RelayForgeError && error.code === "idempotency_conflict");
  assert.equal(calls, 1);
});

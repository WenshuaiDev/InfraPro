import { test } from "node:test";
import assert from "node:assert/strict";
import { connect, DisplayError } from "../../packages/ts/display-sdk/dist/index.js";
import { portTransport } from "../../packages/ts/display-sdk/dist/transport.js";
import { MessageChannel } from "node:worker_threads";
import { createHash } from "node:crypto";
import { sha256 } from "../../apps/display-runtime/src/crypto.ts";
const tick = () => new Promise((r) => setImmediate(r));
function fake() {
  const calls = [],
    listeners = new Set();
  return {
    calls,
    emit: (e) => {
      for (const f of listeners) f(e);
    },
    request: async (method, params) => {
      calls.push({ method, params });
      if (method === "subscription.create") return { id: "sub1" };
      return {};
    },
    onEvent: (f) => {
      listeners.add(f);
      return () => listeners.delete(f);
    },
    dispose() {
      calls.push({ method: "dispose" });
    },
  };
}
const event = (seq = "1", gen = "1") => ({
  subscription_id: "sub1",
  kind: "snapshot",
  generation: gen,
  sequence: seq,
});
test("HTTP SHA256 matches standard vectors and binary data", () => {
  for (const input of [
    "",
    "abc",
    "测点".repeat(100),
    new Uint8Array([0, 128, 255]),
  ])
    assert.equal(
      sha256(input),
      createHash("sha256").update(input).digest("hex"),
    );
});
test("subscriptions coalesce, await every consumer, replay failures and independently release", async () => {
  const t = fake(),
    client = await connect({ transport: t });
  let release,
    attempts = 0;
  const wait = new Promise((r) => (release = r));
  const a = await client.monitoring.subscribe(
    "latest",
    { target: { binding: "a" } },
    async () => {
      attempts++;
      await wait;
    },
  );
  const b = await client.monitoring.subscribe(
    "latest",
    { target: { binding: "a" } },
    () => {},
  );
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.create").length,
    1,
  );
  t.emit(event());
  await tick();
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.ack").length,
    0,
  );
  release();
  await tick();
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.ack").length,
    1,
  );
  t.emit(event());
  await tick();
  assert.equal(attempts, 1);
  a.unsubscribe();
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.cancel").length,
    0,
  );
  b.unsubscribe();
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.cancel").length,
    1,
  );
  client.dispose();
});
test("failed consumer cannot advance cursor; old generations cannot overwrite reset", async () => {
  const t = fake(),
    c = await connect({ transport: t });
  let failed = true,
    count = 0;
  await c.monitoring.subscribe("overview", {}, () => {
    count++;
    if (failed) throw Error("render failure");
  });
  t.emit(event());
  await tick();
  assert.equal(
    t.calls.filter((c) => c.method === "subscription.ack").length,
    0,
  );
  failed = false;
  t.emit(event());
  await tick();
  t.emit(event("1", "2"));
  await tick();
  t.emit(event("2", "1"));
  await tick();
  assert.equal(count, 3);
  c.dispose();
});
test("same callback still has separate handles and revocation frees owned blob URLs", async () => {
  const t = fake(),
    c = await connect({ transport: t });
  let calls = 0;
  const consume = () => calls++;
  const a = await c.monitoring.subscribe("latest", {}, consume),
    b = await c.monitoring.subscribe("latest", {}, consume);
  a.unsubscribe();
  t.emit(event());
  await tick();
  assert.equal(calls, 1);
  const url = c.spatial.createObjectURL(new Blob(["test"]));
  assert.equal(await (await fetch(url)).text(), "test");
  t.emit({ type: "revoked" });
  await assert.rejects(fetch(url));
  await assert.rejects(c.context.get(), { code: "disposed" });
  b.unsubscribe();
});
test("MessageChannel cancellation and response routing are bounded", async () => {
  const { port1, port2 } = new MessageChannel(),
    t = portTransport(port1),
    abort = new AbortController();
  const received = [];
  port2.on("message", (m) => received.push(m));
  const call = t.request("monitoring.latest", {}, { signal: abort.signal });
  abort.abort();
  await assert.rejects(call, { code: "aborted" });
  await tick();
  assert(received.some((v) => v.kind === "cancel"));
  t.dispose();
  port2.close();
});


test("typed failures preserve safe field issues",()=>{
 const e=DisplayError.from({code:"binding_invalid",message:"Mismatch",issues:[{path:"/bindings/site",code:"type",message:"Wrong resource"},{path:3,code:"bad",message:"bad"}]});
 assert.equal(e.code,"binding_invalid");assert.equal(e.issues.length,1);assert.equal(e.issues[0].path,"/bindings/site");
});


test("bridge rejects non-JSON and oversized structured clone payloads",async()=>{
 const {validParams}=await import('../../apps/display-runtime/src/params.ts');
 assert.equal(validParams(undefined),true);assert.equal(validParams({name:'site'}),true);
 const cycle={};cycle.self=cycle;
 for(const v of [cycle,{value:1n},'value',[],{text:'桥'.repeat(90000)}])assert.equal(validParams(v),false);
});

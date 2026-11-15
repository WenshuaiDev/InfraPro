import assert from "node:assert/strict";
import { test } from "node:test";
import { readSpatialMetadata } from "../../apps/console/src/features/spatial/metadata-reader.ts";
const tick = () => new Promise((resolve) => setImmediate(resolve));
test("optional metadata runs at most two reads and canceled queued scopes never start", async () => {
  let active = 0,
    peak = 0;
  const releases = [],
    started = [];
  const task = (id, signal) =>
    readSpatialMetadata(
      signal,
      () =>
        new Promise((resolve) => {
          started.push(id);
          peak = Math.max(peak, ++active);
          releases.push(() => {
            active--;
            resolve(id);
          });
        }),
    );
  const controllers = Array.from({ length: 4 }, () => new AbortController());
  const jobs = controllers.map((c, i) => task(i, c.signal));
  await tick();
  assert.deepEqual(started, [0, 1]);
  const canceled = assert.rejects(jobs[2], { name: "AbortError" });
  controllers[2].abort();
  await canceled;
  releases.shift()();
  await jobs[0];
  await tick();
  assert.deepEqual(started, [0, 1, 3]);
  while (releases.length) releases.shift()();
  assert.deepEqual(await Promise.all([jobs[1], jobs[3]]), [1, 3]);
  assert.equal(peak, 2);
});
test("failure releases capacity and an already aborted request does no work", async () => {
  const controller = new AbortController();
  controller.abort();
  let started = false;
  await assert.rejects(
    readSpatialMetadata(controller.signal, async () => {
      started = true;
    }),
    { name: "AbortError" },
  );
  assert.equal(started, false);
  await assert.rejects(
    readSpatialMetadata(new AbortController().signal, async () => {
      throw Error("unavailable");
    }),
    /unavailable/,
  );
  assert.equal(
    await readSpatialMetadata(new AbortController().signal, async () => 42),
    42,
  );
});

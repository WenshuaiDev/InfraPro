import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { setTimeout as wait } from 'node:timers/promises';

export function visualRecordingDrivers() {
  const hash = (url) =>
    createHash('sha256').update(readFileSync(url)).digest('hex');
  return {
    driver: hash(new URL('./architecture-visual.mjs', import.meta.url)),
    recordingDriver: hash(new URL(import.meta.url)),
  };
}

function input(request) {
  const { pathname, search } = new URL(request.url());
  const address = request.method() + ' ' + pathname + search;
  const body = request.postDataBuffer();
  const headers = request.headers();
  const bodySHA256 =
    body === null ? null : createHash('sha256').update(body).digest('hex');
  const key = JSON.stringify([
    address,
    bodySHA256,
    headers['content-type'] ?? null,
    headers.range ?? null,
    headers.accept ?? null,
  ]);
  return { key, address: request.method() + ' ' + pathname };
}

// The reference and candidate renderers read the same recording independently.
// Reserve a position at request start so concurrent responses retain the
// browser's original order, even when the second response finishes first.
export function createVisualRecording({
  now = () => performance.now(),
  waitForResponse = wait,
} = {}) {
  const responses = new Map();
  const addresses = new Map();
  let count = 0;
  return {
    reserve(request) {
      const { key, address } = input(request);
      if (!responses.has(key))
        responses.set(key, {
          address,
          inputNumber: responses.size + 1,
          entries: [],
        });
      if (!addresses.has(address)) addresses.set(address, new Set());
      addresses.get(address).add(key);
      const started = now();
      const entry = {};
      responses.get(key).entries.push(entry);
      count++;
      return (response) => {
        entry.response = response;
        entry.delayMs = Math.round(now() - started);
      };
    },
    reader() {
      const positions = new Map();
      return {
        async take(request) {
          const { key } = input(request);
          const index = positions.get(key) ?? 0;
          const entry = responses.get(key)?.entries[index];
          if (!entry?.response) return undefined;
          positions.set(key, index + 1);
          // A redirect can unmount the original screen while its queries are in
          // flight. Immediate answers change which requests that screen issues.
          if (entry.delayMs > 0) await waitForResponse(entry.delayMs);
          return entry.response;
        },
        unusedBusinessInputs() {
          const unused = [];
          for (const [key, { address, inputNumber, entries }] of responses) {
            if (
              !address.startsWith('POST ') ||
              address.startsWith('POST /api/v1/auth/activity')
            )
              continue;
            const remaining = entries.length - (positions.get(key) ?? 0);
            if (remaining) unused.push({ address, inputNumber, remaining });
          }
          return unused;
        },
      };
    },
    summary() {
      return {
        requests: count,
        distinctInputs: responses.size,
        addressesWithMultipleInputs: [...addresses]
          .filter(([, keys]) => keys.size > 1)
          .map(([address]) => address),
        responseDelays: [...responses.values()].map(
          ({ inputNumber, entries }) => ({
            inputNumber,
            milliseconds: entries.map((entry) => entry.delayMs ?? null),
          }),
        ),
      };
    },
  };
}

export function scheduleVisualRequests(order) {
  if (order === 'natural') return (run) => run();
  if (order !== 'serial') throw Error('Unknown visual request order ' + order);
  let tail = Promise.resolve();
  return (run) => {
    const current = tail.then(run);
    tail = current.catch(() => {});
    return current;
  };
}

import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  createVisualRecording,
  scheduleVisualRequests,
} from '../../scripts/architecture-visual-recording.mjs';

function request(
  selection,
  { method = 'POST', url = '/monitoring/queries/latest', headers = {} } = {},
) {
  return {
    url: () => 'http://localhost' + url,
    method: () => method,
    postDataBuffer: () =>
      selection === undefined
        ? null
        : Buffer.from(JSON.stringify({ selection_id: selection })),
    headers: () => ({ 'content-type': 'application/json', ...headers }),
  };
}

test('two different selections submitted to the same URL retain their own value and quality', async () => {
  const tape = createVisualRecording({ now: () => 0 }),
    first = request('raw'),
    second = request('calculated');
  const storeFirst = tape.reserve(first),
    storeSecond = tape.reserve(second);
  storeSecond({ status: 200, value: '18', quality: 'good' });
  storeFirst({ status: 200, value: '9', quality: 'good' });
  for (const reader of [tape.reader(), tape.reader()]) {
    assert.deepEqual(await reader.take(second), {
      status: 200,
      value: '18',
      quality: 'good',
    });
    assert.deepEqual(reader.unusedBusinessInputs(), [
      {
        address: 'POST /monitoring/queries/latest',
        inputNumber: 1,
        remaining: 1,
      },
    ]);
    assert.deepEqual(await reader.take(first), {
      status: 200,
      value: '9',
      quality: 'good',
    });
    assert.deepEqual(reader.unusedBusinessInputs(), []);
  }
  assert.deepEqual(tape.summary(), {
    requests: 2,
    distinctInputs: 2,
    addressesWithMultipleInputs: ['POST /monitoring/queries/latest'],
    responseDelays: [
      { inputNumber: 1, milliseconds: [0] },
      { inputNumber: 2, milliseconds: [0] },
    ],
  });
});

test('equal inputs preserve their recorded sequence and readers cannot borrow unseen responses', async () => {
  const tape = createVisualRecording(),
    same = request('one');
  const first = tape.reserve(same),
    second = tape.reserve(same);
  second({ status: 503 });
  first({ status: 200 });
  const original = tape.reader(),
    candidate = tape.reader();
  assert.equal(await original.take(request('different')), undefined);
  assert.equal(
    await original.take(request('one', { url: '/other' })),
    undefined,
  );
  assert.equal(
    await original.take(request('one', { method: 'PUT' })),
    undefined,
  );
  assert.equal(
    await original.take(request('one', { headers: { range: 'bytes=0-3' } })),
    undefined,
  );
  for (const reader of [original, candidate]) {
    assert.equal((await reader.take(same)).status, 200);
    assert.equal((await reader.take(same)).status, 503);
    assert.equal(await reader.take(same), undefined);
  }
});

test('fixed renders that omit different selections cannot qualify as the same consumption', async () => {
  const tape = createVisualRecording();
  const west = request('west'),
    east = request('east');
  tape.reserve(west)({ value: 1 });
  tape.reserve(east)({ value: 2 });
  const original = tape.reader(),
    candidate = tape.reader();
  await original.take(west);
  await candidate.take(east);
  assert.notDeepEqual(
    original.unusedBusinessInputs(),
    candidate.unusedBusinessInputs(),
  );
});

test('distinct query credentials stay in memory while evidence contains the public address', async () => {
  const tape = createVisualRecording();
  const withFirst = request('one', {
    url: '/exchange?permit=first-private-value',
  });
  const withSecond = request('one', {
    url: '/exchange?permit=second-private-value',
  });
  tape.reserve(withFirst)({ status: 200 });
  tape.reserve(withSecond)({ status: 403 });
  const reader = tape.reader();
  assert.equal((await reader.take(withSecond)).status, 403);
  assert.equal(
    await reader.take(request('one', { url: '/exchange?permit=not-recorded' })),
    undefined,
  );
  assert.deepEqual(reader.unusedBusinessInputs(), [
    { address: 'POST /exchange', inputNumber: 1, remaining: 1 },
  ]);
  assert.deepEqual(tape.summary().addressesWithMultipleInputs, [
    'POST /exchange',
  ]);
});

test('the supplemental FIFO lets a real request finish before the next, including failures', async () => {
  const serial = scheduleVisualRequests('serial'),
    order = [];
  let finish;
  const gate = new Promise((resolve) => {
    finish = resolve;
  });
  const first = serial(async () => {
    order.push('first-start');
    await gate;
    order.push('first-finish');
    throw Error('observed failure');
  });
  const second = serial(async () => {
    order.push('second');
  });
  await Promise.resolve();
  assert.deepEqual(order, ['first-start']);
  finish();
  await assert.rejects(first, /observed failure/);
  await second;
  assert.deepEqual(order, ['first-start', 'first-finish', 'second']);
  assert.throws(() => scheduleVisualRequests('unknown'));
});

test('both renderers wait for each observed response and reserve simultaneous identical GETs in order', async () => {
  let clock = 0;
  const waiting = [];
  const tape = createVisualRecording({
    now: () => clock,
    waitForResponse: (ms) =>
      new Promise((resolve) => waiting.push({ ms, resolve })),
  });
  const same = request(undefined, { method: 'GET', url: '/monitoring/jobs' });
  const first = tape.reserve(same);
  clock = 5;
  const second = tape.reserve(same);
  clock = 405;
  second({ generation: 2 });
  clock = 523;
  first({ generation: 1 });
  assert.deepEqual(tape.summary().responseDelays, [
    { inputNumber: 1, milliseconds: [523, 400] },
  ]);
  for (const reader of [tape.reader(), tape.reader()]) {
    const a = reader.take(same),
      b = reader.take(same);
    assert.deepEqual(
      waiting.map(({ ms }) => ms),
      [523, 400],
    );
    assert.equal(await reader.take(same), undefined);
    waiting.pop().resolve();
    assert.deepEqual(await b, { generation: 2 });
    waiting.pop().resolve();
    assert.deepEqual(await a, { generation: 1 });
  }
});

import assert from 'node:assert/strict';
import test from 'node:test';
import { indexCursorPages, readCursorWindow } from '../../packages/ts/ui/src/lib/cursor-pagination.ts';

const pages = [
  { items: [1, 2], next_cursor: 'x' },
  { items: [], next_cursor: 'y' },
  { items: [3, 4, 5, 6], next_cursor: 'z' },
  { items: [7] },
];
const read = async cursor => pages[{ x: 1, y: 2, z: 3 }[cursor]];
const signal = new AbortController().signal;

test('index counts filtered chunks, including an empty intermediate response', async () => {
  const { total, bookmarks } = await indexCursorPages(pages[0], read, signal);
  assert.equal(total, 7);
  assert.deepEqual(bookmarks.map(entry => entry.offset), [2, 2, 6]);
  const windows = await Promise.all([0, 3, 6].map(offset => readCursorWindow(pages[0], read, signal, offset, 3, bookmarks)));
  assert.deepEqual(windows.map(w => w.data.items), [[1, 2, 3], [4, 5, 6], [7]]);
  assert.deepEqual(windows.map(w => w.hasNext), [true, true, false]);
});

test('numbered jumps reuse cursors discovered on counted endpoints', async () => {
  const seen = new Map();
  const remember = entry => seen.set(entry.offset, entry);
  await readCursorWindow(pages[0], read, signal, 3, 3, seen.values(), remember);
  const calls = [];
  const last = await readCursorWindow(pages[0], async cursor => { calls.push(cursor); return read(cursor); }, signal, 6, 3, seen.values(), remember);
  assert.deepEqual(calls, ['z']);
  assert.deepEqual(last.data.items, [7]);
});

test('cancelled requests and repeated cursors stop traversal', async () => {
  const aborted = AbortSignal.abort();
  const first = { items: [1], next_cursor: 'x' };
  await assert.rejects(indexCursorPages(first, () => assert.fail(), aborted), { name: 'AbortError' });
  await assert.rejects(readCursorWindow(first, () => assert.fail(), aborted, 1, 1, [{ offset: 1, cursor: 'x' }]), { name: 'AbortError' });
  await assert.rejects(indexCursorPages(first, async () => first, signal), /游标重复/);
  await assert.rejects(readCursorWindow(first, async () => first, signal, 8, 1, []), /游标重复/);
});

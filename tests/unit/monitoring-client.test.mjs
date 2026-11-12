import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { SHA256 } from '../../apps/console/src/features/monitoring/sha256.ts';
import { decimalCoefficient, formatNS, parseNS, recentRange, shiftRange, zoomRange, MIN_NS, MAX_NS } from '../../apps/console/src/features/monitoring/format.ts';

test('incremental file SHA-256 matches the independent native implementation across part and block boundaries', () => {
  for (const length of [0, 1, 55, 56, 63, 64, 65, 127, 128, 1025, 1000000, 32 * 1024 * 1024 + 67]) {
    const data = Buffer.alloc(length); for (let i = 0; i < length; i++) data[i] = (i * 7 + 127) % 256;
    const expected = createHash('sha256').update(data).digest('hex');
    for (const chunkSize of length > 1000000 ? [8191, 32 * 1024 * 1024] : [1, 63, 64, 65, 501]) {
      const digest = new SHA256(); for (let start = 0; start < length; start += chunkSize) digest.update(data.subarray(start, start + chunkSize));
      assert.equal(digest.digest(), expected, `length ${length} / chunk ${chunkSize}`);
      assert.throws(() => digest.update(new Uint8Array([1])), /finalized/);
    }
  }
});
test('nanosecond navigation preserves negative epochs, offsets, and the upper domain endpoint', () => {
  const cases = [MIN_NS, -1000000001n, -1n, 0n, 1n, 1609459200000000001n, MAX_NS];
  for (const ns of cases) assert.equal(parseNS(formatNS(ns)), ns);
  assert.equal(parseNS('1970-01-01T08:00:00.000000001+08:00'), 1n);
  assert.deepEqual(recentRange(formatNS(MAX_NS)), { from: formatNS(MAX_NS - 86400000000000n), to: null });
  const lower = recentRange(formatNS(MIN_NS)); assert.equal(lower.from, formatNS(MIN_NS));
  const upper = recentRange(formatNS(MAX_NS)); assert.deepEqual(shiftRange(upper, 999999n), upper);
  assert.equal(zoomRange(upper, 2n, 1n).to, null);
  assert.throws(() => parseNS('2020-01-01T00:00:00'), /时区/);
});
test('plot coordinates subtract the exact decimal baseline before approximate conversion', () => {
  const baseline = decimalCoefficient('99999999999999999999999999.999999999998');
  const next = decimalCoefficient('99999999999999999999999999.999999999999');
  assert.equal(next - baseline, 1n);
  assert.equal(Number(next - baseline) / 1e12, 1e-12);
  assert.equal(decimalCoefficient('-9223372036854775808'), -9223372036854775808000000000000n);
  assert.equal(decimalCoefficient('-0.000000000001'), -1n);
});

test('viewport tiles share stable edges, bound visible/prefetched points and include the nanosecond endpoints', async () => {
  const {viewportTiles} = await import('../../apps/console/src/features/monitoring/format.ts');
  for (const [start, end] of [[MIN_NS, MIN_NS+1n],[-1n,1n],[0n,1n],[1n,1000000000000000n],[MIN_NS,MAX_NS],[MAX_NS,MAX_NS]]) {
    for (const curves of [1,10,50]) {
      const range={from:formatNS(start),to:end===MAX_NS?null:formatNS(end)};
      const plan=viewportTiles(range,375,curves), visible=plan.tiles.filter(t=>t.visible);
      assert.ok(visible.length>=1&&visible.length<=3);assert.ok(plan.tiles.length<=5);
      assert.ok(plan.budget*visible.length<=2000);assert.ok(plan.budget*visible.length*curves<=20000);
      for (let i=1;i<plan.tiles.length;i++) assert.equal(plan.tiles[i-1].range.to,plan.tiles[i].range.from);
      assert.ok(parseNS(visible[0].range.from)<=start);
      if(end===MAX_NS)assert.equal(visible.at(-1).range.to,null);
      else assert.ok(visible.at(-1).range.to===null||parseNS(visible.at(-1).range.to)>=end);
    }
  }
  const a=viewportTiles({from:formatNS(0n),to:formatNS(1000000000000n)},960,1);
  const b=viewportTiles({from:formatNS(10000000000n),to:formatNS(1010000000000n)},960,1);
  assert.ok(a.tiles.some(t=>b.tiles.some(v=>v.id===t.id&&v.visible)),'small pans reuse complete cached tiles');
});

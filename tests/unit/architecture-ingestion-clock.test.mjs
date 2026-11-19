import assert from 'node:assert/strict';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { partitionEvidence } from '../../scripts/architecture-ingestion-report.mjs';

test('the fixed partition uses elapsed time independently of UTC correction', () => {
  const sample = { duration_clock: 'monotonic', duration_seconds: 45.001, utc_seconds: 44.998 };
  assert.deepEqual(partitionEvidence('', sample), {
    clock: 'monotonic',
    elapsed_seconds: 45.001,
    utc_seconds: 44.998,
  });
  assert.throws(() => partitionEvidence('', { ...sample, duration_seconds: 44.999, utc_seconds: 45.1 }));
});

test('an archived short UTC difference without its executable evidence stays unverified', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-ingestion-clock-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  assert.throws(() => partitionEvidence(dir, { duration_seconds: 44.998 }));
  assert.deepEqual(partitionEvidence(dir, { duration_seconds: 45.001 }), { clock: 'utc', utc_seconds: 45.001 });
});

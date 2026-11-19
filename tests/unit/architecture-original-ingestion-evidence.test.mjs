import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { archiveOriginalIngestionLoad } from '../../scripts/architecture-original-ingestion-evidence.mjs';

function fixture(t) {
  const cwd = mkdtempSync(join(tmpdir(), 'infrapro-original-ingestion-'));
  t.after(() => rmSync(cwd, { recursive: true, force: true }));
  const base = join(cwd, '.artifacts/ingestion');
  const session = join(base, 'test-abc123');
  const directory = join(cwd, 'command');
  mkdirSync(session, { recursive: true });
  mkdirSync(directory);
  const log = '--- PASS: TestIngestionSustainedLoad (302.58s)\n';
  writeFileSync(join(session, 'tests.log'), log);
  writeFileSync(join(directory, 'command.log'), log);
  const original = { session, exit: 0, synthetic: true };
  writeFileSync(join(session, 'result.json'), JSON.stringify(original));
  writeFileSync(join(base, 'latest-tests.json'), JSON.stringify(original));
  const now = Date.now();
  const result = { status: 'passed', exitCode: 0, log: 'command.log',
    startedAt: new Date(now - 310000).toISOString(), finishedAt: new Date(now + 1000).toISOString() };
  const events = [
    { event: 'started', started_at: new Date(now - 304000).toISOString(), duration_seconds: 180,
      target_measurement_frames_per_second: 100, real_parser: true, synthetic: true, schema: 'identity_test_sample',
      tenants: 2, projects: 2, transport: 'mTLS DDP + HTTPS batch' },
    { event: 'inner_channel_stopped', seconds: 120.5 }, { event: 'inner_channel_restored', seconds: 165.1 },
    { event: 'sending_complete', seconds: 180, frames_per_second: 100, measurement_frames: 18000, receipts: 13501 },
    { event: 'latencies', values: { takeover_p95_seconds: 2, end_to_end_p95_seconds: 65, total_seconds: 301 } },
    { event: 'reconciled', receipts: 13501, expected_receipts: 13501, results: 18000, expected_results: 18000,
      pending_ack: 0, failed: 0, duplicate_default_plans: 0, cross_tenant_units: 0 },
  ];
  const writeEvents = () => writeFileSync(join(session, 'load.jsonl'), events.map((r) => JSON.stringify(r)).join('\n') + '\n');
  writeEvents();
  return { base, session, directory, cwd, result, events, writeEvents,
    check: () => archiveOriginalIngestionLoad(directory, cwd, 'test:ingestion:load', result) };
}

test('archives original ingestion reconciliation and actual worker pause', (t) => {
  const f = fixture(t);
  f.check();
  assert.equal(f.result.status, 'passed');
  const proof = f.result.ingestionLoadEvidence;
  assert.ok(proof.complete);
  assert.equal(proof.completedResults, 18000);
  assert.equal(proof.confirmedReceipts, 13501);
  assert.ok(Math.abs(proof.observedStoppedSeconds - 44.6) < 1e-9);
  assert.equal(proof.files.length, 3);
  assert.ok(proof.files.every((entry) => /^[a-f0-9]{64}$/.test(entry.sha256)));
  writeFileSync(join(f.base, 'latest-tests.json'), '{}');
  f.check();
  assert.ok(f.result.ingestionLoadEvidence.complete);
  f.events[5].cross_tenant_units = 1;
  f.writeEvents();
  f.check();
  assert.equal(f.result.status, 'failed');
  assert.match(f.result.failure, /transcript changed/);
});

test('rejects a successful shell exit with a missing stop or incomplete reconciled result', (t) => {
  const f = fixture(t);
  const [stop] = f.events.splice(1, 1);
  f.writeEvents();
  f.check();
  assert.equal(f.result.status, 'failed');
  assert.equal(f.result.exitCode, 0);
  assert.match(f.result.failure, /ordered start/);
  f.events.splice(1, 0, stop);
  f.events.at(-1).results--;
  f.writeEvents();
  f.result.status = 'passed';
  f.check();
  assert.equal(f.result.status, 'failed');
  assert.match(f.result.failure, /did not reconcile/);
});

test('preserves reconciled totals and measured latency when the original Go input timing fails', (t) => {
  const f = fixture(t);
  const failed = '--- FAIL: TestIngestionSustainedLoad (302.58s)\n';
  writeFileSync(join(f.session, 'tests.log'), failed);
  writeFileSync(join(f.directory, 'command.log'), failed);
  writeFileSync(join(f.session, 'result.json'), JSON.stringify({ session: f.session, exit: 1, synthetic: true }));
  f.result.exitCode = 1;
  f.result.status = 'failed';
  f.events[3].seconds = 210;
  f.events[3].frames_per_second = 18000 / 210;
  f.writeEvents();
  f.check();
  const proof = f.result.ingestionLoadEvidence;
  assert.equal(f.result.exitCode, 1);
  assert.equal(f.result.status, 'failed');
  assert.equal(proof.complete, false);
  assert.equal(proof.verifiedTranscript, true);
  assert.equal(proof.originalGoOutcome, 'FAIL');
  assert.equal(proof.inputWithinOriginalTiming, false);
  assert.equal(proof.reconciled, true);
  assert.equal(proof.completedResults, 18000);
  assert.equal(proof.endToEndP95Seconds, 65);
  assert.match(proof.reason, /180.9-second/);
  writeFileSync(join(f.base, 'latest-tests.json'), '{}');
  f.check();
  assert.equal(f.result.ingestionLoadEvidence.verifiedTranscript, true);
});

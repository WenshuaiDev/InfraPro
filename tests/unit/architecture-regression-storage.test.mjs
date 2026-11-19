import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { read, write } from '../../scripts/architecture-lab.mjs';
import { readChecks, regression } from '../../scripts/architecture-regression.mjs';
import { regressionQueue } from '../../scripts/architecture-regression-queue.mjs';
import { performanceCommands, satisfied, skipReason } from '../../scripts/architecture-acceptance-scope.mjs';
import { captureRecordingOverlay, mapGoTestEndpoints, stopRegressionStorage } from '../../scripts/architecture-regression-infra.mjs';
import { httpImageBuilds } from '../../scripts/architecture-http-network.mjs';
import { monitoringRecoveryEvidence, monitoringVolumes } from '../../scripts/architecture-monitoring-restart.mjs';

function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-architecture-regression-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  return dir;
}

test('selecting a new release preserves a prior failure and leaves the current result pending', (t) => {
  const dir = fixture(t);
  const receipt = join(dir, 'regression/candidate/test-identity.json');
  const failure = {
    command: ['pnpm', 'test:identity'],
    status: 'failed',
    exitCode: 1,
    provenance: { releaseSHA256: 'a'.repeat(64) },
  };
  write(receipt, failure);
  const selected = join(dir, 'candidate.evaluated.json');
  write(selected, { inputs: { sourceSHA256: 'b'.repeat(64) } });
  const identity = () => readChecks(dir, 'candidate').find((row) => row.command[1] === 'test:identity');
  assert.equal(identity().status, 'pending');
  assert.equal(identity().priorStatus, 'failed');
  assert.deepEqual(read(receipt), failure);
  write(selected, { inputs: { sourceSHA256: failure.provenance.releaseSHA256 } });
  assert.equal(identity().status, 'failed');
});

test('an inconsistent Compose project is rejected before stopping any container', (t) => {
  const dir = fixture(t);
  write(join(dir, 'private/lab.json'), { id: 'abc123' });
  const storage = join(dir, 'private/regression-storage/candidate');
  write(join(storage, 'compose.json'), { name: 'arc-reg-abc123-candidate' });
  write(join(storage, 'account-compose.json'), { name: 'infrapro' });
  assert.throws(() => stopRegressionStorage(dir, 'candidate'), /Unexpected Compose project/);
});

test('concurrent browser and HTTP recorder invocations capture their own test endpoints', async (t) => {
  const dir = fixture(t), source = '/repo/internal/transport/http/monitoring_e2e_test.go';
  const overlay = join(dir, 'overlay.json'), patched = join(dir, 'recording.go'), lock = join(dir, 'overlay.flock');
  const cases = await Promise.all(['62001', '63001'].map(async (port) => {
    const env = {};
    const [[file, text]] = await captureRecordingOverlay(lock, env, async () => {
      writeFileSync(patched, '[]string{"127.0.0.1:' + port + '", "59000", "59001"}');
      write(overlay, { Replace: { [source]: patched } });
      await new Promise(resolve => setTimeout(resolve, 30));
      return { GOFLAGS: '-overlay=' + overlay };
    });
    assert.equal(file, source);
    return mapGoTestEndpoints(file, text, { '59000': port, '59001': String(Number(port) + 1) }, 'isolated', 'accounts');
  }));
  assert.deepEqual(cases, [
    '[]string{"127.0.0.1:62001", "62001", "62002"}',
    '[]string{"127.0.0.1:63001", "63001", "63002"}',
  ]);
});

test('original HTTP initializers share a single build before the API and web images', () => {
  const ops = { image: 'ops', build: { dockerfile: 'ops.Dockerfile', context: '.' } };
  const api = { image: 'api', build: { dockerfile: 'api.Dockerfile', context: '.' } };
  const web = { image: 'web', build: { dockerfile: 'web.Dockerfile', context: '.' } };
  assert.deepEqual(httpImageBuilds({ migrate: ops, sync: { ...ops }, api, db: { image: 'postgres' }, web }), ['migrate', 'api', 'web']);
  assert.throws(() => httpImageBuilds({ migrate: ops, sync: { ...api, image: ops.image } }));
});

test('the two volume inventories are compared by actual volume IDs', () => {
  const internal = { Url: 's3:8080', VolumeIds: ' 1-3 13', Volumes: 4, Max: 64 };
  const external = { Url: 'audit-volume:8080', VolumeIds: ' 4-6 18', Volumes: 4, Max: 64 };
  const snapshot = { Topology: { DataCenters: [{ Racks: [{ DataNodes: [internal, external] }] }] } };
  assert.deepEqual(monitoringVolumes(snapshot), { 's3:8080': [1, 2, 3, 13], 'audit-volume:8080': [4, 5, 6, 18] });
  external.VolumeIds = '4-5 18';
  assert.throws(() => monitoringVolumes(snapshot));
});

test('the original ClickHouse part read shares the deadline with cluster startup', (t) => {
  const dir = fixture(t), folder = join(dir, '.artifacts/monitoring-contract/run1');
  mkdirSync(folder, { recursive: true });
  const state = join(dir, 'outage.json');
  write(state, { startedAt: '2026-11-18T23:00:01Z', readyAt: '2026-11-18T23:00:09Z', after: { 's3:8080': [1] } });
  writeFileSync(join(dir, 'run.log'), 'S-01 real component evidence: ' + folder + '\n');
  const run = { cwd: dir, log: 'run.log' }, Test = 'TestStorageContract/online_s3_parts_and_failure';
  const events = [{ Test, Time: '2026-11-18T23:00:00Z', Output: 'object outage fails explicitly' },
    { Test, Time: '2026-11-18T23:00:20Z', Action: 'pass' }];
  const save = () => writeFileSync(join(folder, 'tests.jsonl'), events.map(v => JSON.stringify(v)).join('\n'));
  save();
  assert.equal(monitoringRecoveryEvidence(dir, run, state).elapsedMs, 19_000);
  events[1].Time = '2026-11-18T23:00:22Z';
  save();
  assert.throws(() => monitoringRecoveryEvidence(dir, run, state), /exceeded 20 seconds/);
  events[1].Action = 'fail';
  save();
  assert.throws(() => monitoringRecoveryEvidence(dir, run, state), /must both pass/);
});

test('the functional queue preserves old measurements and never starts excluded commands', async (t) => {
  const dir = fixture(t), raw = join(dir, 'regression/baseline/test-ingestion-load.json');
  const previous = { status: 'passed', exitCode: 0, command: ['pnpm', 'test:ingestion:load'], provenance: { releaseSHA256: 'a'.repeat(64) } };
  write(raw, previous);
  const rows = await regression(dir, 'baseline', [...performanceCommands]);
  assert(rows.every(r => r.status === 'skipped' && r.skipReason === skipReason && r.exitCode === undefined));
  assert.equal(rows.find(r => r.command[1] === previous.command[1]).priorStatus, 'passed');
  assert.deepEqual(read(raw), previous);
  assert(rows.every(satisfied));
  assert(!satisfied({ id: 'ARC-016', status: 'skipped', skipReason }));
  assert(!satisfied({ command: ['pnpm', 'test:ingestion'], status: 'skipped', skipReason }));
  const queue = regressionQueue(dir);
  for (const side of ['baseline', 'candidate']) {
    assert.deepEqual(queue.sides[side].filter(r => r.status === 'skipped').map(r => r.command).sort(), [...performanceCommands].sort());
    const display = queue.sides[side].find(r => r.command === 'test:display:load');
    assert.equal(display.status, 'pending');
    assert.deepEqual(display.arguments, ['--boundary']);
    assert.deepEqual(display.skippedStages, [{ name: 'TestDisplayReadLoadTenInstances', status: 'skipped', skipReason }]);
    assert.equal(queue.sides[side].find(r => r.command === 'test:ingestion').status, 'pending');
  }
});

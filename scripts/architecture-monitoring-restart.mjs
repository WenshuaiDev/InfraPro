import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { read, sleep, write } from './architecture-lab.mjs';

const deadlineMs = 20_000;
const nodes = ['audit-volume:8080', 's3:8080'];

export function monitoringVolumes(status) {
  const volumes = {};
  for (const dc of status.Topology.DataCenters) for (const rack of dc.Racks) for (const node of rack.DataNodes) {
    assert(!Object.hasOwn(volumes, node.Url));
    const ids = new Set();
    for (const range of (node.VolumeIds || '').trim().split(/\s+/).filter(Boolean)) {
      const match = /^(\d+)(?:-(\d+))?$/.exec(range);
      assert(match, 'Unrecognized SeaweedFS volume range');
      const first = Number(match[1]), last = Number(match[2] ?? first);
      assert(first <= last && last - first < node.Max);
      for (let id = first; id <= last; id++) ids.add(id);
    }
    assert.equal(ids.size, node.Volumes);
    volumes[node.Url] = [...ids].sort((a, b) => a - b);
  }
  return volumes;
}

function topology(config) {
  const bytes = execFileSync(config.docker, [
    'exec', config.project + '-s3-1', 'wget', '-qO-', '-T', '1', 'http://s3:9333/dir/status',
  ], { timeout: 1700, stdio: ['ignore', 'pipe', 'pipe'] });
  return monitoringVolumes(JSON.parse(bytes));
}

// One successful GET can reach the internal volume while the external volume
// still has to rejoin. Compose start waits for the actual pre-outage inventory.
export async function monitoringCompose(config, args) {
  const action = args.at(-2), state = config.monitoringRestart;
  if (action === 'stop') {
    const before = topology(config);
    assert.deepEqual(Object.keys(before).sort(), nodes);
    assert(Object.values(before).some(ids => ids.length));
    write(state, { before, stoppingAt: new Date().toISOString() });
  }
  const prior = existsSync(state) && read(state);
  const started = Date.now();
  const run = spawnSync(config.docker, args, { stdio: 'inherit' });
  if (run.status !== 0 || action !== 'start' || !prior || prior.startedAt) return run.status ?? 1;
  prior.startedAt = new Date(started).toISOString();
  write(state, prior);
  let after;
  while (Date.now() - started < deadlineMs) {
    try { after = topology(config); } catch { /* The master is still starting. */ }
    if (after && Object.entries(prior.before).every(([url, ids]) =>
      Object.hasOwn(after, url) && ids.every(id => after[url].includes(id)))) {
      write(state, { ...prior, after, readyAt: new Date().toISOString() });
      return 0;
    }
    await sleep(150);
  }
  throw Error('SeaweedFS did not restore the original volume inventory within 20 seconds');
}

export function monitoringRecoveryEvidence(directory, run, stateFile) {
  const record = read(stateFile);
  assert(record.startedAt && record.readyAt && record.after);
  const original = readFileSync(join(directory, run.log), 'utf8');
  const marker = /^S-01 real component evidence: (.+)$/m.exec(original);
  assert(marker, 'S-01 did not report its Go evidence');
  const folder = resolve(marker[1].trim());
  assert.equal(dirname(folder), resolve(run.cwd, '.artifacts/monitoring-contract'));
  const events = readFileSync(join(folder, 'tests.jsonl'), 'utf8').trim().split('\n').map(line => JSON.parse(line));
  const subtest = 'TestStorageContract/online_s3_parts_and_failure';
  const rejected = events.find(e => e.Test === subtest && e.Output?.includes('object outage fails explicitly'));
  const passed = events.find(e => e.Test === subtest && e.Action === 'pass');
  assert(rejected && passed, 'Original S-01 outage and recovered ClickHouse query must both pass');
  const elapsedMs = Date.parse(passed.Time) - Date.parse(record.startedAt);
  assert(Date.parse(rejected.Time) <= Date.parse(record.startedAt));
  assert(Date.parse(record.readyAt) <= Date.parse(passed.Time));
  assert(elapsedMs > 0 && elapsedMs <= deadlineMs, 'Full S-01 recovery exceeded 20 seconds from the start request');
  return { ...record, originalGoCase: subtest, finishedAt: passed.Time, elapsedMs, deadlineMs };
}

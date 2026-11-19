import assert from 'node:assert/strict';
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { openDisplayCampaign } from '../../scripts/architecture-display-campaign.mjs';
import { measuredDisplayBudget, publishedServiceLimits } from '../../scripts/architecture-display-processes.mjs';

test('resuming cannot replace a measured miss, a timeout, or an interrupted trial', (t) => {
  const temporary = mkdtempSync(join(tmpdir(), 'infrapro-display-campaign-'));
  t.after(() => rmSync(temporary, { recursive: true, force: true }));
  const scope = { rounds: 3, first: 'b' }, input = { candidate: 'release-1', snapshot: 'snapshot-1' };
  const open = (resume) => openDisplayCampaign(temporary, 'fixture-driver', scope, input, resume);
  const first = open(false), idle = { side: 'b', round: 1, scene: 'idle' };
  const preparation = first.store(idle, { passed: false, failures: [['prepare', 'dependency unavailable']] });
  assert(existsSync(join(first.folder, preparation.receipt)));
  assert.equal(open(true).previous(idle), undefined);
  const now = new Date().toISOString();
  first.prepare(idle).begin(now);
  const miss = first.store(idle, { passed: false, metrics: { hot: { p95Ms: 2001 } } });
  assert.equal(open(true).previous(idle).metrics.hot.p95Ms, 2001);
  assert.throws(() => first.prepare(idle));
  assert.throws(() => first.store(idle, { passed: true }));
  const history = { ...idle, scene: 'history' };
  first.prepare(history).begin(now);
  assert.equal(open(true).previous(history).failures[0][0], 'timed-interruption');
  assert.throws(() => first.prepare(history));
  const mixed = { ...idle, scene: 'mixed' };
  const captured = first.prepare(mixed);
  captured.begin(now, { database: 'isolated-mixed-database' });
  mkdirSync(dirname(captured.file), { recursive: true });
  writeFileSync(captured.file, JSON.stringify({ passed: false, failures: [['stream', 'digest mismatch']] }));
  const recovered = open(true).previous(mixed);
  assert.equal(recovered.database, 'isolated-mixed-database');
  assert.deepEqual(recovered.failures, [['stream', 'digest mismatch']]);
  assert.throws(() => first.prepare(mixed));
  const replacement = open(false);
  assert.notEqual(replacement.campaign, first.campaign);
  assert.equal(replacement.previous(idle), undefined);
  assert.equal(first.previous(idle).receipt, miss.receipt);
});

test('a measured scene must carry the selected binary, real sockets and equal service limits', () => {
  const slot = { cpu: 2, memory_mib: 40 };
  const serviceLimits = publishedServiceLimits({ schema58: { api: slot }, roles: { query: slot } });
  assert.throws(() => publishedServiceLimits({ schema58: { api: slot }, roles: { query: { ...slot, cpu: 3 } } }));
  const roles = { query: [1, 1, 2, 32] };
  const bytes = 'd'.repeat(64);
  const profile = { budgetProfile: 'example', roles: { b: roles }, total: roles.query, serviceLimits, objectRequests: 1 };
  const row = {
    side: 'b',
    budget: {
      profile: 'example', roles, total: roles.query,
      deployed: { query: { cpu: 2, memoryMiB: 40, goThreads: 2, goMemoryMiB: 32 } },
      processLimits: { query: { pid: 170, binarySHA256: bytes, goThreads: 2, heapMiB: 32, clickhouseLimit: 1 } },
      roleLogins: { accounts: { query: { limit: 1 } } },
      objectRequests: { limit: 1, peak: 1, completed: 7 },
    },
    resources: [{ connections: { roles: { query: { postgres: 1, clickhouse: 1 } } } }],
  };
  const valid = sample => measuredDisplayBudget(sample, profile, { query: bytes });
  assert.equal(valid(row), true);
  for (const change of [
    sample => { sample.budget.deployed.query.cpu = 3; },
    sample => { sample.budget.deployed.query.memoryMiB = 48; },
    sample => { sample.budget.processLimits.query.binarySHA256 = 'e'.repeat(64); },
    sample => { sample.budget.processLimits.query.goThreads = 1; },
    sample => { sample.resources[0].connections.roles.query.postgres = 2; },
    sample => { sample.resources[0].connections.roles.query.clickhouse = 0; },
    sample => { sample.budget.objectRequests.completed = 0; },
  ]) {
    const bad = structuredClone(row);
    change(bad);
    assert.equal(valid(bad), false);
  }
});

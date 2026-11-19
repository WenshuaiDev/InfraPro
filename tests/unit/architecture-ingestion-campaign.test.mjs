import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import {
  verifyCompletedIngestionRound,
  verifyIngestionPause,
  verifyWorkerPrincipals,
} from '../../scripts/architecture-ingestion-report.mjs';
import { grantFiles, workerPG } from '../../scripts/fixtures/architecture-ingestion-database.mjs';
import { inventory } from '../../scripts/architecture-runtime.mjs';

test('resume accepts only completed registered rounds before an unused directory', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-ingestion-pause-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const rounds = [{ name: 'a-new1', mode: 'a' }, { name: 'pg-new1', mode: 'pg' }];
  const completed = join(dir, rounds[0].name);
  mkdirSync(completed);
  const binaries = { driver: 'frozen-S0-driver', workers: { 'ingest-worker': 'S0' } };
  writeFileSync(join(completed, 'binaries.json'), JSON.stringify(binaries));
  const result = (name) => writeFileSync(join(completed, 'result.json'), JSON.stringify({ passed: true, mode: 'a', name }));
  const resume = () => verifyIngestionPause(dir, rounds, { before: rounds[1].name }, (path, round) => {
    verifyCompletedIngestionRound(path, round, binaries);
  });
  result('a-old1');
  assert.throws(resume, /another round/);
  result(rounds[0].name);
  resume();
  assert.throws(() => verifyCompletedIngestionRound(completed, rounds[0], { ...binaries, driver: 'another' }), /different executables/);
  mkdirSync(join(dir, rounds[1].name));
  writeFileSync(join(dir, rounds[1].name, 'start.json'), '{}');
  assert.throws(resume, /already contains raw data/);
});

test('the formal role gate checks both observations, applied grants and measured connections', () => {
  const issued = Object.entries(workerPG).map(([role, budget]) => ({
    role, budget, login: 'fixture_' + role, group: inventory.roles[role].sqlRole,
  }));
  const grants = Object.fromEntries(grantFiles.map((f, i) => [f, String(i)]));
  const database = 'fixture-db';
  const schema = 'fixture-schema';
  const rows = issued.map(({ role, login, group, budget }) => ({
    role, login, expected_group: group, member_of: [group], budget, connection_limit: budget,
    sessions: budget, slot_sessions: budget, restricted: true, single_group: true, non_owner: true,
    superuser: false, create_database: false, create_role: false, replication: false, bypass_rls: false,
    other_round_databases: 0,
  }));
  const proof = { database, namespace: schema, roles: rows };
  const peaks = Object.fromEntries(issued.map((x) => [x.login, x.budget]));
  const round = { database, database_principals: { database, schema, issued, grant_sql_sha256: grants },
    replica_slots: proof, final_replica_slots: structuredClone(proof), resources: { peak_connections_by_login: peaks } };
  assert.ok(verifyWorkerPrincipals(round, grants));
  assert.throws(() => verifyWorkerPrincipals(round, { ...grants, [grantFiles[0]]: 'different' }));
  round.final_replica_slots.roles[0].member_of.push('another_group');
  assert.throws(() => verifyWorkerPrincipals(round, grants));
  round.final_replica_slots.roles[0].member_of.pop();
  peaks[issued[0].login]++;
  assert.throws(() => verifyWorkerPrincipals(round, grants));
});

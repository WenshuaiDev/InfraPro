import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { digest, root, write } from '../../scripts/architecture-lab.mjs';
import {
  bindPublicEvidence,
  currentPublicEvidence,
  observedPublicFixture,
  publicReplayInputs,
} from '../../scripts/architecture-parity-evidence.mjs';
import { postgresRoleSQLFiles } from '../../scripts/architecture-postgres.mjs';
import { compareParserAttestation } from '../../scripts/architecture-ingestion-bodies.mjs';

test('a parser result identifies the serving executable while preserving method and output evidence', () => {
  const builds = ['1'.repeat(64), '2'.repeat(64)];
  const results = builds.map(service_build_digest => ({
    contract_version: 1, service_build_digest, method: { artifact_digest: '3'.repeat(64) },
    records: [{ value: '14.00', quality: 'good' }],
  }));
  for (const [operation, shape] of [
    ['Parsing:platform:runs:response', value => value],
    ['Parsing:tenant:runs:response', value => value],
    ['Ingestion:tenant:messages:response', value => value],
    ['Parsing:platform:runs:result', value => ({ response: value })],
    ['Parsing:tenant:runs:result', value => ({ response: value })],
    ['Ingestion:tenant:messages:results', value => [{ response: value }]],
  ]) {
    const [a,b] = results.map((value,i) => compareParserAttestation(operation,shape(value),builds[i]));
    assert.deepEqual(a,b);
    assert.throws(() => compareParserAttestation(operation,shape(results[0]),builds[1]));
    for (const other of [{ ...results[1], records: [] }, { ...results[1], method: {} }]) {
      assert.notDeepEqual(a,compareParserAttestation(operation,shape(other),builds[1]));
    }
  }
  assert.deepEqual(results.map(item => item.service_build_digest), builds);
  assert.equal(compareParserAttestation('getOtherOperation', results[0], builds[1]), results[0]);
});

test('an HTTP replay belongs to its completed command, release and installed role grants', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'infrapro-http-receipts-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const grants = Object.fromEntries(
    postgresRoleSQLFiles.map((file) => [file, digest(readFileSync(join(root, file)))]),
  );
  const release = 'a'.repeat(64);
  const label = 'architecture-http-parity',
    file = 'http/operations.json';
  const fixture = { candidateSHA256: release, appliedRoleSQL: grants };
  const stale = {
    ...fixture,
    appliedRoleSQL: { ...grants, [postgresRoleSQLFiles[0]]: 'b'.repeat(64) },
  };
  write(join(dir, 'snapshot/manifest.json'), { pg: { seed: 'synthetic-seed' } });
  const inputs = publicReplayInputs(dir,label);
  assert.equal(publicReplayInputs(dir,'architecture-edge'),publicReplayInputs(dir,'catalog-devices'));
  assert.notEqual(inputs,publicReplayInputs(dir,'architecture-edge'));
  const command = {
    status: 'passed',
    startedAt: new Date(Date.now() - 1000).toISOString(),
    provenance: { releaseSHA256: release, pairedFixture: randomUUID(), publicInputs: inputs },
    publicInputsAtCompletion: inputs,
  };
  const sandbox = join(dir, 'http', 'sandboxes', 'operations-sample');
  write(join(dir, 'commands', 'public-fixtures', command.provenance.pairedFixture + '.json'), {
    directory: sandbox,
  });
  write(join(sandbox, 'snapshot.json'), fixture);
  write(join(dir, file), { fixture });
  write(join(dir, 'role-grants.json'), stale.appliedRoleSQL);
  command.finishedAt = new Date().toISOString();
  bindPublicEvidence(dir, label, command);
  write(join(dir, 'commands', label + '.json'), command);
  const current = currentPublicEvidence(dir, release);
  assert(current.has(file));
  write(join(dir, 'commands', label + '.json'), { ...command, publicInputsAtCompletion: 'old inputs' });
  assert.equal(currentPublicEvidence(dir, release).size, 0);
  write(join(dir, 'commands', label + '.json'), command);
  assert(observedPublicFixture(dir, file, { fixture }, release, current));
  assert.equal(observedPublicFixture(dir, file, { fixture }, release, new Set()), false);
  assert.equal(observedPublicFixture(dir, file, { fixture: stale }, release, current), false);
  assert.equal(currentPublicEvidence(dir, 'c'.repeat(64)).size, 0);
  write(join(dir, file), { fixture: stale });
  assert.equal(currentPublicEvidence(dir, release).size, 0);
  write(join(sandbox, 'snapshot.json'), stale);
  write(join(dir, 'role-grants.json'), grants);
  bindPublicEvidence(dir, label, command);
  assert.equal(command.processStatus, 'passed');
  assert.equal(command.status, 'failed');
});

import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { archiveIdentityRegressionEvidence, readIdentityRegressionEvidence, validIdentityRegressionEvidence } from '../../scripts/architecture-identity-regression-evidence.mjs';
import { checkGoResults, planGoTests } from '../../scripts/go-tests.mjs';

const json = file => JSON.parse(readFileSync(file));
const write = (file, value) => writeFileSync(file, JSON.stringify(value, null, 2) + '\n');
const sha = file => createHash('sha256').update(readFileSync(file)).digest('hex');

function fixture(t, interrupted = false) {
  const root = mkdtempSync(join(tmpdir(), 'original-identity-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const directory = join(root, 'regression/candidate'), cwd = join(root, 'checkout');
  const session = '.artifacts/go-tests/2026-11-19T00-00-01.000Z', native = join(cwd, session);
  mkdirSync(directory, { recursive: true }); mkdirSync(native, { recursive: true });
  const source = 'c'.repeat(64), startedAt = '2026-11-19T00:00:00.000Z', finishedAt = '2026-11-19T00:00:09.000Z';
  const row = { cwd, command: ['pnpm', 'test:identity'], status: interrupted ? 'failed' : 'passed',
    exitCode: interrupted ? 1 : 0, startedAt, finishedAt, log: 'test-identity.log',
    provenance: { role: 'candidate', releaseSHA256: source, parallelGoBatches: 1 },
    sourceAtCompletion: { sourceSHA256: source, matchesSelectedRelease: true } };
  const packages = [
    { cwd, name: 'example/identity', tests: ['TestActivity', 'TestOptional'] },
    { cwd, name: 'example/audit', tests: ['TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke'] },
    { cwd, name: 'example/http', tests: ['TestAuthHTTP'] },
  ];
  const jobs = planGoTests(packages), results = [];
  write(join(native, 'plan.json'), { packages, concurrency: 1, jobs });
  const lines = [`[go:test] 3 batches, 4 tests; evidence ${session}`];
  for (const [i, job] of jobs.entries()) {
    if (interrupted && i === 2) break;
    const index = i + 1, actions = new Map(), tests = [];
    const events = [], seconds = 1.3, code = interrupted ? 1 : 0;
    const event = (Action, Test, Output) => events.push(JSON.stringify({ Time: '2026-11-19T00:00:03.000Z', Package: job.name, Action, Test, Output }));
    event('start');
    if (!interrupted || i === 0) {
      for (const name of job.tests) {
        const action = name === 'TestOptional' ? 'skip' : code ? 'fail' : 'pass';
        const reason = action === 'skip' ? 'requires optional original fixture\n' : undefined;
        event('run', name); if (reason) event('output', name, reason);
        event(action, name); actions.set(name, action); tests.push({ name, action, ...(reason && { reason }) });
      }
      event(code ? 'fail' : 'pass');
    }
    const check = checkGoResults(job, actions, code);
    writeFileSync(join(native, `${index}.jsonl`), events.join('\n') + '\n');
    results.push({ index, ...job, tests, code, seconds, ...check });
    lines.push(`[go:test] ${index}/3 ${job.name} (${job.tests.length} tests, ${job.race ? 'race' : 'timed boundary'})`);
    lines.push(`[go:test] ${index}/3 ${check.passed ? 'passed' : 'FAILED'} in 1.3s`);
  }
  write(join(native, 'result.json'), { passed: !interrupted, interrupted, expectedBatches: 3, results, seconds: 5 });
  writeFileSync(join(directory, row.log), lines.join('\n') + '\n');
  const archive = () => { row.identityBatchEvidence = archiveIdentityRegressionEvidence(directory, cwd, row); };
  const read = () => readIdentityRegressionEvidence(directory, row, source);
  return { cwd, directory, row, source, native, archive, read, valid: () => validIdentityRegressionEvidence(directory, row, source) };
}

test('archives immutable native discovery, race and timed batches, raw outcomes and their original skip', t => {
  const f = fixture(t); f.archive(); const ref = f.row.identityBatchEvidence;
  f.archive(); assert.deepEqual(f.row.identityBatchEvidence, ref);
  assert(f.valid()); assert(!validIdentityRegressionEvidence(f.directory, f.row, 'e'.repeat(64)));
  const observed = f.read().summary;
  assert.equal(observed.complete, true); assert.equal(observed.state, 'passed');
  assert.deepEqual(observed.outcomes, { pass: 3, fail: 0, skip: 1 });
  assert.equal(observed.batches[1].race, false);
  const original = join(f.native, '1.jsonl'), raw = readFileSync(original);
  writeFileSync(original, 'changed after the command\n'); assert.throws(f.archive);
  assert.equal(sha(join(f.directory, dirname(ref.file), '1.jsonl')), createHash('sha256').update(raw).digest('hex'));
  rmSync(f.native, { recursive: true }); assert(f.valid());
});

test('retains original nonzero assertion, incomplete group and unstarted groups separately', t => {
  const f = fixture(t, true); f.archive(); const x = f.read();
  assert.equal(x.exitCode, 1); assert.equal(x.summary.state, 'interrupted'); assert(!f.valid());
  assert.deepEqual(x.summary.outcomes, { pass: 0, fail: 1, skip: 1 });
  assert.deepEqual(x.summary.batches[0].failed, ['TestActivity']);
  assert.deepEqual(x.summary.batches[1].missing, ['TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke']);
  assert.deepEqual(x.summary.notStarted, [3]);
  f.row.status = 'passed'; f.row.exitCode = 0; assert(!f.valid());
});

test('does not certify a missing raw group, shortened discovery list, or a changed terminal event', t => {
  for (const edit of [
    f => rmSync(join(f.native, '3.jsonl')),
    f => { const p = join(f.native, 'plan.json'); const j = json(p); j.packages[0].tests.pop(); write(p,j); },
    f => { const p = join(f.native, '1.jsonl'); writeFileSync(p, readFileSync(p, 'utf8').replace('"Action":"pass"', '"Action":"skip"')); },
  ]) {
    const f = fixture(t); edit(f); f.archive(); assert(!f.valid()); assert.equal(f.read().summary.state, 'invalid_native_evidence');
  }
});

test('rechecks raw semantic outcomes even if a damaged archive is rehashed', t => {
  const f = fixture(t); f.archive(); const ref = f.row.identityBatchEvidence;
  const manifest = join(f.directory, ref.file), native = join(dirname(manifest),'1.jsonl');
  writeFileSync(native, readFileSync(native,'utf8').replace('"Action":"skip"','"Action":"pass"'));
  assert(!f.valid());
  const j = json(manifest); j.files['1.jsonl'] = { size: readFileSync(native).length, sha256: sha(native) };
  write(manifest,j); ref.sha256 = sha(manifest); assert(!f.valid());
});

test('preserves setup failures without inventing a discovered or executed batch', t => {
  const f = fixture(t, true); writeFileSync(join(f.directory,f.row.log), 'fixture startup failed\n');
  f.archive(); assert(!f.valid()); assert.equal(f.read().summary.state, 'before_discovery');
  assert.deepEqual(Object.keys(f.read().files), ['command.log']);
});

test('keeps the native result readable when the outer release qualification fails', t => {
  const f = fixture(t);
  f.archive();
  f.row.processStatus = 'passed';
  f.row.status = 'failed';
  assert.equal(f.read().status, 'passed');
  assert.equal(f.read().summary.complete, true);
  assert.equal(f.valid(), false);
});

import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import {
  existsSync, lstatSync, mkdirSync, mkdtempSync, readdirSync, readFileSync,
  realpathSync, renameSync, rmSync, writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { checkGoResults, planGoTests } from './go-tests.mjs';

const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const terminals = new Set(['pass', 'fail', 'skip']);
const rawBatch = /^([1-9]\d*)\.jsonl$/;

function commandIdentity(row, cwd) {
  assert.deepEqual(row.command, ['pnpm', 'test:identity']);
  assert.equal(row.log, 'test-identity.log');
  assert.equal(resolve(cwd), row.cwd);
  assert(['baseline', 'candidate'].includes(row.provenance.role));
  assert.match(row.provenance.releaseSHA256, /^[a-f0-9]{64}$/);
  assert(['passed', 'failed'].includes(row.status));
  assert(Number.isInteger(row.exitCode) || (row.status === 'failed' && row.exitCode === null));
  for (const time of [row.startedAt, row.finishedAt]) assert.equal(new Date(time).toISOString(), time);
  assert(Date.parse(row.startedAt) <= Date.parse(row.finishedAt));
  return join('identity-batches', row.startedAt.replaceAll(':', '-'));
}

function regularFile(path) {
  assert(lstatSync(path).isFile(), 'Expected a regular original identity evidence file');
  return readFileSync(path);
}

function discovery(command) {
  const lines = [...command.toString().matchAll(
    /^\[go:test\] (\d+) batches, (\d+) tests; evidence (.+)$/gm,
  )];
  assert(lines.length <= 1, 'Ambiguous native Go test discovery run');
  if (!lines.length) return null;
  const [, batches, tests, session] = lines[0];
  assert.match(session, /^\.artifacts\/go-tests\/\d{4}-\d\d-\d\dT\d\d-\d\d-\d\d\.\d{3}Z$/);
  return { batches: Number(batches), tests: Number(tests), session };
}

function rawFiles(folder, command) {
  const files = new Map([['command.log', command]]);
  if (folder) for (const name of readdirSync(folder).sort()) {
    if (name === 'plan.json' || name === 'result.json' || rawBatch.test(name))
      files.set(name, regularFile(join(folder, name)));
  }
  return files;
}

function observations(bytes, job, started, finished) {
  const top = new Map(), runs = new Set(), output = new Map();
  let packageOutcome = null;
  for (const line of bytes.toString('utf8').split('\n')) {
    if (!line) continue;
    const event = JSON.parse(line);
    if (event.Action === 'diagnostic' || (event.Package !== job.name && !event.Test)) continue;
    assert.equal(event.Package, job.name);
    if (event.Test?.includes('/')) continue;
    if (event.Action === 'output' && event.Test) output.set(event.Test, (output.get(event.Test) ?? '') + event.Output);
    if (!terminals.has(event.Action) && event.Action !== 'run') continue;
    const time = Date.parse(event.Time);
    assert(time >= started - 1 && time <= finished + 1, 'Go event belongs to another original run');
    if (!event.Test) {
      assert.equal(packageOutcome, null, 'Duplicate native package completion');
      packageOutcome = event.Action;
    } else if (event.Action === 'run') {
      assert(!runs.has(event.Test), 'Duplicate top-level Go test start');
      runs.add(event.Test);
    } else {
      assert(runs.has(event.Test) && !top.has(event.Test), 'Missing or duplicate original Go test execution');
      top.set(event.Test, event.Action);
    }
  }
  return { top, runs, output, packageOutcome };
}

function inspectOriginal(files, row, found) {
  const commandLines = new Set(files.get('command.log').toString('utf8').split('\n'));
  if (!found || !files.has('plan.json')) {
    assert.equal(row.status, 'failed', 'The original successful command lacks its discovered plan');
    return { state: found ? 'plan_missing' : 'before_discovery', complete: false };
  }
  const plan = JSON.parse(files.get('plan.json'));
  assert.equal(plan.jobs.length, found.batches);
  assert.equal(plan.packages.reduce((n, p) => n + p.tests.length, 0), found.tests);
  assert(found.batches > 0 && found.tests > 0);
  assert.equal(new Set(plan.packages.map(p => p.name)).size, plan.packages.length);
  assert.deepEqual(plan.jobs, planGoTests(plan.packages));
  assert(Number.isInteger(plan.concurrency) && plan.concurrency >= 1 && plan.concurrency <= 4);
  if (row.provenance.parallelGoBatches != null) assert.equal(plan.concurrency, row.provenance.parallelGoBatches);
  const native = files.has('result.json') ? JSON.parse(files.get('result.json')) : null;
  if (native) {
    assert.equal(native.expectedBatches, plan.jobs.length);
    assert.equal(typeof native.interrupted, 'boolean');
  }
  const results = new Map(native?.results.map(item => [item.index, item]) ?? []);
  assert.equal(results.size, native?.results.length ?? 0);
  const batches = [], totals = { pass: 0, fail: 0, skip: 0 };
  const seen = new Set();
  for (const [name, bytes] of files) {
    const match = rawBatch.exec(name);
    if (!match) continue;
    const index = Number(match[1]), job = plan.jobs[index - 1], actual = results.get(index);
    assert(job, 'Raw Go batch outside the native plan');
    seen.add(index);
    const start = `[go:test] ${index}/${plan.jobs.length} ${job.name} (${job.tests.length} tests, ${job.race ? 'race' : 'timed boundary'})`;
    assert(commandLines.has(start), 'Original stdout did not start the archived Go batch');
    const { top, runs, output, packageOutcome } = observations(bytes, job, Date.parse(row.startedAt), Date.parse(row.finishedAt));
    for (const action of top.values()) totals[action]++;
    const code = actual?.code ?? null;
    const check = checkGoResults(job, top, code);
    if (actual) {
      assert.equal(actual.name, job.name);
      assert.equal(actual.cwd, job.cwd);
      assert.equal(actual.race, job.race);
      assert(Number.isInteger(code));
      assert.deepEqual(actual.tests.map(t => [t.name, t.action]), [...top]);
      for (const test of actual.tests.filter(t => t.action === 'skip')) assert.equal(test.reason, output.get(test.name) ?? '');
      for (const key of ['missing', 'unexpected', 'failed', 'passed']) assert.deepEqual(actual[key], check[key]);
      const end = `[go:test] ${index}/${plan.jobs.length} ${check.passed ? 'passed' : 'FAILED'} in ${actual.seconds.toFixed(1)}s`;
      assert(commandLines.has(end), 'Original stdout lacks this batch exit');
      if (check.passed) {
        assert.equal(packageOutcome, 'pass');
        assert.equal(runs.size, top.size, 'A started original Go test lacks its terminal event');
      }
    }
    batches.push({ index, package: job.name, race: job.race, code, packageOutcome,
      observed: top.size, skipped: [...top.values()].filter(action => action === 'skip').length,
      ...check });
  }
  assert([...results.keys()].every(index => seen.has(index)), 'Native result references a missing original batch log');
  batches.sort((a, b) => a.index - b.index);
  const notStarted = plan.jobs.map((_, index) => index + 1).filter(index => !seen.has(index));
  const complete = !!native && !native.interrupted && !notStarted.length && results.size === plan.jobs.length &&
    batches.every(batch => batch.passed) && native.passed;
  if (native) assert.equal(native.passed, !native.interrupted && results.size === plan.jobs.length && batches.every(batch => batch.passed));
  if (row.status === 'passed') assert(complete && row.exitCode === 0, 'The original identity runner did not pass its complete plan');
  return { state: !native ? 'result_missing' : native.interrupted ? 'interrupted' : complete ? 'passed' : 'failed',
    complete, nativePassed: native?.passed ?? null, interrupted: native?.interrupted ?? null,
    packages: plan.packages.length, plannedTests: found.tests, plannedBatches: found.batches,
    startedBatches: seen.size, reportedBatches: results.size, notStarted, outcomes: totals, batches };
}

function manifest(files, row) {
  // The outer qualification can fail after the original pnpm process exited.
  // Preserve what that process reported when reading its immutable archive.
  if (row.processStatus) row = { ...row, status: row.processStatus };
  const found = discovery(files.get('command.log'));
  let summary;
  try { summary = inspectOriginal(files, row, found); }
  catch (error) {
    // Retain the original exit and all surviving bytes when a process or its
    // transcripts ended mid-write. Only a verified full run can be resumed.
    summary = { state: 'invalid_native_evidence', complete: false,
      validationError: error.message.split('\n')[0].slice(0, 200) };
  }
  return { role: row.provenance.role, source: row.provenance.releaseSHA256,
    sourceAtCompletion: row.sourceAtCompletion, cwd: row.cwd, originalCommand: row.command,
    startedAt: row.startedAt, finishedAt: row.finishedAt, exitCode: row.exitCode, status: row.status,
    session: found?.session ?? null, summary,
    files: Object.fromEntries([...files].map(([name, bytes]) => [name, { size: bytes.length, sha256: digest(bytes) }])) };
}

// Copy a single original attempt. Previously archived bytes are never replaced.
// The caller records the returned reference in its own completed command receipt.
export function archiveIdentityRegressionEvidence(directory, cwd, row) {
  const relative = commandIdentity(row, cwd), output = join(directory, relative);
  const command = regularFile(join(directory, row.log)), found = discovery(command);
  const source = found && resolve(cwd, found.session);
  if (source && existsSync(source)) assert.equal(realpathSync(source), join(realpathSync(cwd), found.session));
  const files = rawFiles(source && existsSync(source) ? source : null, command);
  const result = manifest(files, row);
  const bytes = Buffer.from(JSON.stringify(result, null, 2) + '\n');
  const reference = { file: join(relative, 'evidence.json'), sha256: digest(bytes) };
  mkdirSync(dirname(output), { recursive: true, mode: 0o700 });
  const temporary = mkdtempSync(join(dirname(output), '.writing-'));
  try {
    for (const [name, content] of files) writeFileSync(join(temporary, name), content, { mode: 0o600 });
    writeFileSync(join(temporary, 'evidence.json'), bytes, { mode: 0o600 });
    try { renameSync(temporary, output); }
    catch (e) {
      if (!['ENOTEMPTY', 'EEXIST'].includes(e.code)) throw e;
      readIdentityRegressionEvidence(directory, { ...row, identityBatchEvidence: reference });
    }
  } finally { rmSync(temporary, { recursive: true, force: true }); }
  return reference;
}

export function readIdentityRegressionEvidence(directory, row, expectedSource = row.provenance.releaseSHA256) {
  const relative = commandIdentity(row, row.cwd), ref = row.identityBatchEvidence;
  assert.equal(row.provenance.releaseSHA256, expectedSource);
  assert.equal(ref?.file, join(relative, 'evidence.json'));
  const output = join(directory, relative);
  assert.equal(realpathSync(output), join(realpathSync(directory), relative));
  const bytes = regularFile(join(output, 'evidence.json'));
  assert.equal(digest(bytes), ref.sha256);
  const command = regularFile(join(output, 'command.log'));
  assert(command.equals(regularFile(join(directory, row.log))), 'The current command log differs from the archived invocation');
  const saved = JSON.parse(bytes);
  assert.deepEqual(saved, manifest(rawFiles(output, command), row));
  return saved;
}

export function validIdentityRegressionEvidence(directory, row, expectedSource = row?.provenance?.releaseSHA256) {
  try {
    if (row.status !== 'passed' || row.exitCode !== 0 || !row.sourceAtCompletion?.matchesSelectedRelease ||
      row.sourceAtCompletion.sourceSHA256 !== expectedSource) return false;
    return readIdentityRegressionEvidence(directory, row, expectedSource).summary.complete === true;
  } catch { return false; }
}

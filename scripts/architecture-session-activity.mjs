import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { closeSync, copyFileSync, mkdirSync, mkdtempSync, openSync, readFileSync, realpathSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { read, root, runDirectory, write } from './architecture-lab.mjs';
import { regressionRelease, verifyRegressionSource } from './architecture-regression.mjs';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const identity = 'packages/go/platform/identity';
const http = 'services/api/internal/transport/http';
const clock = 'TestSessionActivityFlushUsesDatabaseTime';
const limits = 'TestSessionActivityKeepsIdleAndAbsoluteLimits';
const external = 'TestActivityHTTPRespectsSessionLifetime';
const frozen = 'TestArchitectureFrozenSessionActivityObservation';
const externalCases = ['active_before_idle', 'already_idle', 'absolute_limit', 'revoked'];
const clockCases = ['trusted', 'large_skew', 'legacy_cache', 'missing_marker', 'scheduled', 'cancelled', 'revoked', 'expired', 'old_observation', 'newer_activity', 'same_time_new_version', 'stale_version'];

const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
const directoryFor = (directory, role) => join(directory, 'identity', 'session-activity', role);

function selectorInputs(role, source) {
  const names = role === 'baseline'
    ? ['scripts/fixtures/architecture-session-frozen.go.inc', http + '/session_activity_test.go']
    : [identity + '/session_activity_test.go', http + '/session_activity_test.go'];
  return Object.fromEntries(names.map((name) => [name, digest(readFileSync(join(source, name)))]));
}

function expectedNames(role) {
  const names = [external, ...externalCases.map((name) => external + '/' + name)];
  return names.concat(role === 'baseline' ? [frozen] : [limits, clock, ...clockCases.map((name) => clock + '/' + name)]).sort();
}

function observations(bytes) {
  const states = new Map(), checkpoint = {};
  for (const line of bytes.toString('utf8').split('\n')) {
    if (!line.startsWith('{')) continue;
    const event = JSON.parse(line);
    if (!event.Test) continue;
    const pkg = event.Test.split('/')[0] === external ? http : identity;
    assert.equal(event.Package, 'github.com/WenshuaiDev/InfraPro/' + pkg);
    if (['pass', 'skip', 'fail'].includes(event.Action)) {
      assert.ok(!states.has(event.Test), 'duplicate final test outcome');
      states.set(event.Test, event.Action);
    }
    if (event.Action !== 'output') continue;
    const match = event.Output?.match(/\bsession_checkpoint (\{.*\})\s*$/);
    if (!match) continue;
    const sample = JSON.parse(match[1]);
    assert.equal(typeof sample.case, 'string');
    assert.ok(!Object.hasOwn(checkpoint, sample.case), 'duplicate session checkpoint');
    const [, subcase] = event.Test.split('/');
    const expected = event.Test === frozen ? 'frozen_future_loss' : event.Test === limits ? 'limits' :
      (event.Test.startsWith(external + '/') ? 'http_' : 'clock_') + subcase;
    assert.equal(sample.case, expected);
    checkpoint[sample.case] = sample;
  }
  const cases = [...states.keys()].sort().map((name) => ({ name, state: states.get(name) }));
  return { cases, checkpoint };
}

function checkMeasurements(role, cases, checkpoints) {
  assert.deepEqual(cases.map((c) => c.name), expectedNames(role));
  assert.ok(cases.every((c) => c.state === 'pass'));
  for (const name of externalCases) {
    const sample = checkpoints['http_' + name];
    assert.equal(sample.original_absolute_hours, 12);
    assert.equal(sample.idle_hours, 2);
    assert.equal(sample.absolute_unchanged, true);
    if (name === 'active_before_idle') {
      assert.equal(sample.activity_status, 204);
      assert.equal(sample.hot_read_status, 200);
      assert.equal(sample.after_cache_clear_status, 200);
      assert.equal(sample.reached_old_idle_cutoff, true);
      assert.equal(sample.durable_matches_verified, true);
    } else {
      assert.equal(sample.activity_status, 401);
      assert.equal(sample.read_after_flush_status, 401);
      assert.equal(sample.activity_cache_keys, 0);
      assert.equal(sample.durable_unchanged, true);
    }
  }
  if (role === 'baseline') {
    const sample = checkpoints.frozen_future_loss;
    assert.equal(sample.lead_ms, 850);
    for (const key of ['dirty_before','first_observed_before_due','postgres_unchanged_before_due','postgres_unchanged_after_due','warm_matches_observation','cold_matches_previous','old_idle_cutoff_denied']) {
      assert.equal(sample[key], true);
    }
    assert.equal(sample.dirty_after_first, false);
    assert.equal(sample.dirty_after_due, false);
    assert.ok(sample.warm_minus_cold_ms > 119 * 60 * 1000);
    return;
  }
  assert.deepEqual(checkpoints.limits, { case: 'limits', idle_hours: 2, absolute_hours: 12, recent_durable_after_cache_clear: true, idle_denied: true, absolute_denied: true, renewal_kept_initial_absolute_expiry: true });
  for (const name of clockCases) {
    const sample = checkpoints['clock_' + name];
    const deferred = ['large_skew','legacy_cache','missing_marker','scheduled','cancelled'].includes(name);
    const replaced = ['newer_activity','same_time_new_version'].includes(name);
    const old = name === 'old_observation';
    assert.equal(sample.observation_minus_pg_us, name === 'large_skew' ? 850000 : old ? -2 * 3600 * 1000000 : 7000);
    assert.equal(sample.initial_conditional_writes, deferred || old ? 1 : 2);
    assert.equal(sample.dirty_on_return, deferred || replaced);
    assert.equal(sample.persisted_on_return, name === 'trusted' || replaced);
    assert.equal(sample.initial_value_unchanged, name !== 'trusted' && !replaced);
    assert.equal(sample.pending_version_matches, name !== 'same_time_new_version');
    assert.equal(sample.durable_matches_observation_after_due, deferred || name === 'trusted' || name === 'same_time_new_version');
  }
}

export function verifySessionActivityResult(directory, role, expectedSHA256) {
  try {
    assert.ok(['baseline', 'candidate'].includes(role));
    assert.match(expectedSHA256, /^[a-f0-9]{64}$/);
    const folder = directoryFor(directory, role), result = read(join(folder, 'result.json'));
    assert.equal(result.role, role);
    assert.equal(result.passed, true);
    assert.equal(result.exitCode, 0);
    assert.equal(result.sourceBefore, expectedSHA256);
    assert.equal(result.sourceAfter, expectedSHA256);
    assert.equal(result.expectedLegacyObservation, role === 'baseline');
    assert.deepEqual(result.selectorInputs, selectorInputs(role, root));
    const path = resolve(directory, result.rawPath);
    assert.equal(relative(directory, path), result.rawPath);
    assert.match(relative(realpathSync(folder), realpathSync(path)), /^attempt-[\w-]+\/go-test\.jsonl$/);
    const bytes = readFileSync(path);
    assert.equal(digest(bytes), result.rawSHA256);
    if (role === 'baseline') {
      const attempt = resolve(path, '..');
      assert.equal(digest(readFileSync(join(attempt, 'frozen_observation_test.go'))), result.selectorInputs['scripts/fixtures/architecture-session-frozen.go.inc']);
      assert.equal(digest(readFileSync(join(attempt, 'session_activity_http_test.go'))), result.selectorInputs[http + '/session_activity_test.go']);
    }
    const { cases, checkpoint } = observations(bytes);
    assert.deepEqual(result.cases, cases);
    assert.deepEqual(result.checkpoint, checkpoint);
    checkMeasurements(role, cases, checkpoint);
    return true;
  } catch {
    return false;
  }
}

export function sessionActivity(directory, role, { resume = false } = {}) {
  assert.ok(['baseline', 'candidate'].includes(role), 'select baseline or candidate');
  const release = regressionRelease(directory, role);
  const source = role === 'baseline' ? release.source : root;
  const sourceBefore = verifyRegressionSource(release).sourceSHA256, inputs = selectorInputs(role, root);
  const folder = directoryFor(directory, role);
  if (resume && verifySessionActivityResult(directory, role, sourceBefore)) return read(join(folder, 'result.json'));
  mkdirSync(folder, { recursive: true, mode: 0o700 });
  const attempt = mkdtempSync(join(folder, 'attempt-'));
  ensureTestDependencies();
  const selectors = role === 'baseline' ? [frozen, external] : [clock, limits, external];
  const args = ['test', '-p=1', '-parallel=1', '-race', '-count=1', '-json', '-timeout=8m', '-run', '^(' + selectors.join('|') + ')$'];
  if (role === 'baseline') {
    const overlay = join(attempt, 'overlay.json');
    const observed = join(attempt, 'frozen_observation_test.go');
    const transported = join(attempt, 'session_activity_http_test.go');
    copyFileSync(join(root, 'scripts/fixtures/architecture-session-frozen.go.inc'), observed);
    copyFileSync(join(root, http, 'session_activity_test.go'), transported);
    write(overlay, { Replace: {
      [join(source, identity, 'architecture_frozen_session_activity_test.go')]: observed,
      [join(source, http, 'session_activity_test.go')]: transported,
    } });
    args.push('-overlay', overlay);
  }
  args.push('./' + identity, './' + http);
  const raw = join(attempt, 'go-test.jsonl'), fd = openSync(raw, 'wx', 0o600);
  const startedAt = new Date().toISOString();
  let executed;
  try {
    executed = spawnSync('go', args, { cwd: source, env: { ...process.env, ...testEnv, GOWORK: join(source,'go.work'), GOMAXPROCS: '2' }, stdio: ['ignore', fd, fd] });
  } finally {
    closeSync(fd);
  }
  const completed = { exitCode: executed.status };
  verifyRegressionSource(release, completed);
  const bytes = readFileSync(raw);
  const result = { passed: false, role, sourceBefore, sourceAfter: completed.sourceAtCompletion.sourceSHA256,
    exitCode: executed.status, expectedLegacyObservation: role === 'baseline', selectorInputs: inputs, command: ['go', ...args], selectors, cwd: source,
    startedAt, finishedAt: new Date().toISOString(), rawPath: relative(directory, raw), rawSHA256: digest(bytes), cases: [], checkpoint: {} };
  try {
    Object.assign(result, observations(bytes));
    assert.equal(result.exitCode, 0);
    assert.equal(result.sourceAfter, sourceBefore);
    assert.deepEqual(inputs, selectorInputs(role, root));
    checkMeasurements(role, result.cases, result.checkpoint);
    result.passed = true;
  } catch (error) {
    result.failure = error.message;
  }
  write(join(attempt,'result.json'), result);
  write(join(folder,'result.json'), result);
  return result;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = sessionActivity(runDirectory(process.argv[2]), process.argv[3], { resume: process.argv.includes('--resume') });
  console.log(result.role, result.passed ? 'passed' : 'failed', result.rawPath);
  process.exitCode = result.passed ? 0 : 1;
}

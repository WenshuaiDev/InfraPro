import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { closeSync, mkdirSync, openSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileLease, digest, read, root, runDirectory, write } from './architecture-lab.mjs';
import { regressionInfra } from './architecture-regression-infra.mjs';
import { regressionRelease, verifyRegressionSource } from './architecture-regression.mjs';
import { installClockTrialSetup } from './architecture-trial-setup.mjs';

const [, , location, side] = process.argv;
assert(location && ['baseline', 'candidate'].includes(side), 'Pass a run directory and release side');
const run = runDirectory(location);
const release = regressionRelease(run, side);
const cwd = side === 'baseline' ? join(run, 'baseline-source') : root;
const directory = join(run, 'regression', side);
const unlock = await fileLease(join(run, 'regression', 'locks', 'contract-' + side + '.flock'));
try {
  verifyRegressionSource(release);
  const startedAt = new Date().toISOString();
  const output = join(directory, 'trial-setup-checks', startedAt.replaceAll(':', '-'));
  mkdirSync(output, { recursive: true, mode: 0o700 });
  const env = await regressionInfra(run, side, cwd, {}, undefined, 'general', true);
  const originalOverlay = /(?:^| )-overlay=(\S+)/.exec(env.GOFLAGS);
  assert(originalOverlay);
  const overlay = read(originalOverlay[1]);
  const setup = installClockTrialSetup(cwd, output, overlay.Replace, true);
  const focusedOverlay = join(output, 'overlay.json');
  write(focusedOverlay, overlay);

  const cases = [
    setup.case,
    'TestDerivedInteractiveBudgetHandsOffWithoutFailure',
    'TestDerivedTrialUsesRealSnapshotAndSharedTask',
  ];
  const args = ['test', '-count=1', '-p', '1', '-tags', 'monitoringintegration', '-timeout=9m',
    '-run', '^(' + cases.join('|') + ')$', '-v', './packages/go/platform/resources'];
  const launch = `
    import { spawnSync } from 'node:child_process';
    import { ensureTestDependencies, testEnv } from './scripts/test-environment.mjs';
    ensureTestDependencies();
    const child = spawnSync('go', JSON.parse(process.argv[1]), {
      stdio: 'inherit', env: { ...process.env, ...testEnv },
    });
    process.exitCode = child.status ?? 1;
  `;
  const log = join(output, 'go.log');
  const fd = openSync(log, 'w', 0o600);
  let proc;
  try {
    proc = spawnSync(process.execPath, ['--input-type=module', '-e', launch, JSON.stringify(args)], {
      cwd, env: {
        ...Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('INFRAPRO_'))),
        ...env, GOFLAGS: env.GOFLAGS.replace(originalOverlay[1], focusedOverlay),
        GOMAXPROCS: '2', INFRAPRO_MONITORING_CONTRACT: '1',
      }, stdio: ['ignore', fd, fd],
    });
  } finally {
    closeSync(fd);
  }
  const raw = readFileSync(log, 'utf8');
  const samples = [...raw.matchAll(/clock trial setup initial_state=(\w+) submission_ms=(\d+) run=([\w-]+) job=([\w-]+) target_revision=(\d+) receipt=([\w-]+)/g)]
    .map(([, initialState, elapsed, runId, jobId, targetRevision, receipt]) =>
      ({ initialState, submissionMs: Number(elapsed), runId, jobId, targetRevision, receipt }));
  const checks = Object.fromEntries(cases.map(name => [name, raw.includes('--- PASS: ' + name + ' (')]));
  const clock = raw.match(/clock foreground deadline remaining_ns=(\d+) waited_ns=(\d+)/);
  const observedDeadline = clock && { remainingNs: Number(clock[1]), waitedNs: Number(clock[2]) };
  checks.originalDefaultDeadlineHandedOff = Boolean(observedDeadline && observedDeadline.remainingNs > 0 &&
    observedDeadline.remainingNs <= 3e9 && observedDeadline.waitedNs >= observedDeadline.remainingNs &&
    samples.some(x => x.initialState === 'queued' && x.submissionMs >= 2900));
  checks.bothChannelsReadOriginalReceipts = samples.length === 2 && new Set(samples.map(s => s.jobId)).size === 2;
  const report = {
    side, startedAt, finishedAt: new Date().toISOString(), command: ['go', ...args],
    exitCode: proc.status, status: proc.status === 0 && Object.values(checks).every(Boolean) ? 'passed' : 'failed',
    releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
    setup, checks, observedDeadline, samples, log: relative(directory, log), logSHA256: digest(raw),
  };
  verifyRegressionSource(release, report);
  write(join(output, 'evidence.json'), report);
  write(join(directory, 'trial-setup-check.json'), report);
  console.log(side, report.status, relative(root, join(output, 'evidence.json')));
  if (report.status !== 'passed') process.exitCode = 1;
} finally {
  await unlock();
}

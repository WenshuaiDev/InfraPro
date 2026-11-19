import assert from 'node:assert/strict';
import { execFileSync, spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { closeSync, mkdirSync, openSync, readFileSync, writeFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { availablePort } from './fixtures/parsing-lab.mjs';
import { unusedDockerSubnet } from './architecture-http-network.mjs';
import { digest, root, runDirectory, safeDiagnosticLine, write } from './architecture-lab.mjs';
import { regressionRelease, verifyRegressionSource } from './architecture-regression.mjs';
import { testEnv } from './test-environment.mjs';

const run = runDirectory(process.argv[2]);
const startedAt = new Date().toISOString();
const work = join(run, 'regression', 'derived-cancelled-query', startedAt.replaceAll(':', '-'));
mkdirSync(work, { recursive: true, mode: 0o700 });
const project = 'arc-derived-cancel-' + randomUUID().slice(0, 12);
const compose = join(work, 'compose.json');
const template = JSON.parse(execFileSync('docker', [
  'compose', '-f', join(root, 'deploy/compose/test.yaml'), 'config', '--format', 'json',
], { encoding: 'utf8' }));
assert.deepEqual(Object.keys(template.services).sort(), ['postgres', 'redis']);
const endpoints = {};
for (const [role, service] of Object.entries(template.services)) {
  let port;
  do { port = await availablePort(); } while (Object.values(endpoints).includes(port));
  assert.equal(service.ports.length, 1);
  assert.equal(service.ports[0].host_ip, '127.0.0.1');
  endpoints[role] = port;
  service.ports[0].published = String(port);
}
const subnet = unusedDockerSubnet('docker', '10.247', parseInt(digest(project).slice(0, 2), 16));
write(compose, {
  name: project,
  services: template.services,
  networks: { default: { ipam: { config: [{ subnet }] } } },
});
function docker(...args) {
  return execFileSync('docker', ['compose', '-p', project, '-f', compose, ...args],
    { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
}
const fixture = readFileSync(new URL('./fixtures/architecture-derived-cancel.go.inc', import.meta.url), 'utf8');
const summary = { startedAt, project, subnet, fixtureSHA256: digest(fixture), sides: {} };
const db = new URL(testEnv.INFRAPRO_TEST_DATABASE_URL);
db.port = String(endpoints.postgres);
const cache = new URL(testEnv.INFRAPRO_TEST_REDIS_URL);
cache.port = String(endpoints.redis);
let stage = 'starting isolated dependencies';
try {
  docker('up', '-d', '--wait', '--wait-timeout', '90');
  for (const side of ['baseline', 'candidate']) {
    stage = side + ' verification';
    const release = regressionRelease(run, side);
    verifyRegressionSource(release);
    const cwd = side === 'baseline' ? join(run, 'baseline-source') : root;
    const source = join(cwd, 'packages/go/platform/derivedmonitoring/control_test.go');
    const original = readFileSync(source, 'utf8');
    const extra = join(work, side + '-test.go');
    writeFileSync(extra, original + '\n' + fixture, { mode: 0o600 });
    const overlay = join(work, side + '-overlay.json');
    write(overlay, { Replace: { [source]: extra } });
    const args = ['test', '-p', '1', '-count=1', '-v', '-overlay', overlay, '-timeout=2m',
      '-run', '^TestArchitectureCancelActiveDerivedSQL$', './packages/go/platform/derivedmonitoring'];
    const log = join(work, side + '-go.log');
    const fd = openSync(log, 'w', 0o600);
    const since = new Date().toISOString();
    let child;
    try {
      child = spawnSync('go', args, {
        cwd, env: {
          ...Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('INFRAPRO_'))),
          GOMAXPROCS: '2', GOFLAGS: '-p=2',
          INFRAPRO_TEST_DATABASE_URL: db.href, INFRAPRO_TEST_REDIS_URL: cache.href,
        }, stdio: ['ignore', fd, fd],
      });
    } finally { closeSync(fd); }
    const raw = readFileSync(log, 'utf8');
    const observed = raw.match(/ARCH_PG_CANCEL run=([\w-]+) job=([\w-]+) request=([\w-]+) active_pid=(\d+) active_after=false state=cancelled replayed=true receipts=1/);
    const result = {
      side, startedAt: since, finishedAt: new Date().toISOString(), command: ['go', ...args],
      exitCode: child.status, releaseSHA256: release.inputs?.sourceSHA256 ?? release.source_archive_sha256,
      originalSHA256: digest(original), overlaySHA256: digest(original + '\n' + fixture),
      log: relative(run, log), logSHA256: digest(raw), status: child.status === 0 && observed ? 'passed' : 'failed',
      ...(observed && { runID: observed[1], jobID: observed[2], requestID: observed[3],
        activeBackendPID: Number(observed[4]), activeAfterCancel: false, terminalState: 'cancelled',
        replayedSameCommand: true, matchingReceipts: 1 }),
    };
    verifyRegressionSource(release, result);
    summary.sides[side] = result;
    write(join(work, side + '.json'), result);
    console.log(side, result.status, relative(run, log));
    assert.equal(result.status, 'passed', 'Inspect the isolated raw Go observation');
  }
} catch (error) {
  const detail = String(error.stderr || error.message).trim().split(/\r?\n/).at(-1);
  summary.failure = { stage, reason: safeDiagnosticLine(detail) };
  throw error;
} finally {
  docker('down', '--volumes');
  summary.stoppedAt = new Date().toISOString();
  summary.remainingContainers = execFileSync('docker', ['ps', '-aq', '--filter',
    'label=com.docker.compose.project=' + project], { encoding: 'utf8' }).trim().split('\n').filter(Boolean);
  summary.passed = ['baseline','candidate'].every(s => summary.sides[s]?.status === 'passed') &&
    summary.remainingContainers.length === 0;
  write(join(work, 'summary.json'), summary);
  write(join(run, 'regression', 'derived-cancelled-query.json'), summary);
}
assert(summary.passed, 'Both original task commands must cancel their database work and release the test containers');

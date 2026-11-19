import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { digest, read, root, write } from './architecture-lab.mjs';

export function verifySummaryPermissions(replay, out) {
  const lab = read(join(replay.sourceRun, 'private/lab.json'));
  assert.match(lab.id, /^[0-9a-f]{12}$/);
  assert.equal(lab.b.database, 'arc_http_' + lab.id + '_b');
  const fixture = 'scripts/fixtures/architecture-summary-permissions.sql';
  const bytes = readFileSync(join(root, fixture));
  const report = {
    status: 'failed',
    sourceRun: replay.sourceRun,
    candidateSHA256: replay.candidateSHA256,
    roleSQLSHA256: replay.role_grants['deploy/architecture/postgres-roles.sql'],
    fixture,
    fixtureSHA256: digest(bytes),
    startedAt: new Date().toISOString(),
    checks: [],
  };
  const file = 'deploy/summary-repair-check.json';
  try {
    const output = execFileSync('docker', [
      'exec', '-i', lab.container, 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1',
      '-U', 'archtest', '-d', lab.b.database,
    ], { input: bytes, encoding: 'utf8', stdio: 'pipe' });
    report.checks = output.trim().split('\n').map(line => JSON.parse(line));
    assert.deepEqual(report.checks.filter(row => row.allowed).map(row => row.check).sort(), [
      'infrapro_monitoring_history/covered-dirty-range',
      'infrapro_monitoring_incremental/covered-dirty-range',
      'infrapro_monitoring_read/covered-dirty-range',
      'live-matching-contribution',
      'release-after-current-repair',
    ]);
    assert.deepEqual(report.checks.filter(row => row.sqlstate === '42501').map(row => row.check).sort(), [
      'different-contribution-object', 'expired-rebuild-lease', 'head-ahead-of-tenant-position',
      'old-contribution-fence', 'release-without-repair-receipt',
    ]);
    assert.equal(report.checks.length, 10);
    report.status = 'passed';
  } catch (error) {
    const line = String(error.stderr || '').split('\n').find(row => row.includes('ERROR:'));
    report.error = (line || error.message.split('\n')[0]).slice(0, 240);
    throw error;
  } finally {
    report.endedAt = new Date().toISOString();
    write(join(out, file), report);
  }
  return { file, checks: report.checks.length };
}

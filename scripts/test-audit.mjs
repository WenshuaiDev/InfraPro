import { execFileSync } from 'node:child_process';
import { mkdirSync, openSync, closeSync, readFileSync } from 'node:fs';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

ensureTestDependencies();
execFileSync("docker", ["compose", "-f", "deploy/monitoring-contract/compose.yaml", "up", "-d", "--wait", "s3", "audit-volume"], {stdio:"inherit"});
mkdirSync('.artifacts/audit', { recursive: true });
const log = process.argv.includes('--restore')?'.artifacts/audit/restore.log':'.artifacts/audit/domain.log';
const fd = openSync(log, 'w');
try {
  execFileSync('go', ['test', '-p', '1', '-race', '-count=1', '-timeout=10m', '-v', ...(process.argv.includes('--restore')?['-run','^TestAuditFrozenBackupIndependentRestore$']:['-skip','^TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke$']), './packages/go/platform/auditrecord', './packages/go/platform/audit'], {
    env: { ...process.env, ...testEnv }, stdio: ['ignore', fd, fd],
  });
  if (!process.argv.includes('--restore')) {
    // Check the fixed 15-second production snapshot budget without race instrumentation.
    // All smaller concurrent state-machine and authorization tests above retain -race.
    execFileSync('go', ['test', '-count=1', '-timeout=4m', '-v', '-run', '^TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke$', './packages/go/platform/audit'], {env:{...process.env,...testEnv},stdio:['ignore',fd,fd]});
    execFileSync('go', ['test', '-count=1', '-v', './packages/go/platform/config'], {env:{...process.env,...testEnv},stdio:['ignore',fd,fd]});
    execFileSync('go', ['test', '-p', '1', '-race', '-count=1', '-timeout=5m', '-v', '-run', '^TestAuditHTTP', './services/api/internal/transport/http'], {env:{...process.env,...testEnv},stdio:['ignore',fd,fd]});
    execFileSync('node', ['--test', 'tests/unit/audit-client.test.mjs'], {stdio:['ignore',fd,fd]});
  }
} finally {
  closeSync(fd);
  process.stdout.write(readFileSync(log, 'utf8'));
}
if (/--- SKIP:/.test(readFileSync(log, 'utf8'))) throw new Error('Audit verification skipped a required scenario');

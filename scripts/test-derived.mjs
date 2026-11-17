import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createWriteStream, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const args = process.argv.slice(2);
if (args.some((arg) => !['--contract', '--database', '--keep', '--calculator-load'].includes(arg))) throw new Error('Usage: test-derived.mjs [--contract] [--database] [--keep] [--calculator-load]');
const contract = args.includes('--contract');
const database = args.includes('--database');
const calculatorLoad = args.includes('--calculator-load');
const runId = new Date().toISOString().replaceAll(':', '-');
const output = join(root, '.artifacts', 'derived-monitoring', runId);
mkdirSync(output, { recursive: true });
const command = (name, argv) => execFileSync(name, argv, { cwd: root, encoding: 'utf8' }).trim();
const paths = command('git', ['ls-files', '-co', '--exclude-standard', '-z']).split('\0').filter(path => path && existsSync(join(root, path))).sort();
const source = createHash('sha256'), files = [];
for (const path of new Set(paths)) {
  const bytes = readFileSync(join(root, path));
  const sha256 = createHash('sha256').update(bytes).digest('hex');
  source.update(path).update('\0').update(sha256).update('\n');
  files.push({ path, sha256, bytes: bytes.length });
}
writeFileSync(join(output, 'source.json'), JSON.stringify({ head: command('git', ['rev-parse', 'HEAD']), sha256: source.digest('hex'), files }, null, 2));
writeFileSync(join(output, 'worktree.txt'), command('git', ['status', '--short']));
const env = { ...process.env, ...(database ? testEnv : {}), INFRAPRO_MONITORING_CONTRACT: '1' };
function run(command, argv, filename, overrides = {}) {
  return new Promise((resolve, reject) => {
    const log = createWriteStream(join(output, filename));
    const child = spawn(command, argv, { cwd: root, env: { ...env, ...overrides }, stdio: ['ignore', 'pipe', 'pipe'] });
    for (const stream of [child.stdout, child.stderr]) stream.on('data', (chunk) => { log.write(chunk); process.stdout.write(chunk); });
    child.on('error', (error) => { log.end(); reject(error); });
    child.on('close', (code) => { log.end(); if (code === 0) resolve(); else reject(new Error(`${filename} failed (${code})`)); });
  });
}
let passed = false;
console.log(`Derived monitoring evidence: ${output}`);
try {
  if (database) ensureTestDependencies();
  await run('go', ['test', '-race', '-count=1', './packages/go/platform/derivedmonitoring/...', './packages/go/platform/monitoringvalue', './packages/go/platform/monitoring/internal/observation', './packages/go/platform/monitoring'], 'rules.log');
  if (database) await run('go', ['test', '-race', '-count=1', '-timeout=10m', './services/api/internal/transport/http', './packages/go/platform/resources', './packages/go/platform/access', '-run', 'TestDerived|TestAnalysis|TestRoutePolicy|TestCatalogPolicy|TestResourceLifecycleAndStableHistory|TestProjectResourceExplicitGrantsContextAndCreation|TestSharedResourcePreviewConflictLifecycleArchiveAndReceipts|TestProjectCreationRollsBackEveryCoupledWrite|TestDualIdentityDoesNotFallbackAndCreationDoesNotInheritDownward'], 'database-http-resources.log');
  if (contract) {
    await run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', '--wait-timeout', '180'], 'compose.log');
    const info = JSON.parse(execFileSync('docker', ['info', '--format', '{{json .}}'], { encoding: 'utf8' }));
    writeFileSync(join(output, 'environment.json'), JSON.stringify({ cpus: info.NCPU, memoryBytes: info.MemTotal, architecture: info.Architecture, docker: info.ServerVersion, go: execFileSync('go', ['version'], { encoding: 'utf8' }).trim() }, null, 2));
    await run('go', ['test', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=5m', './packages/go/platform/monitoring/contracttest', '-run', '^TestDerived'], 'storage-contract.log');
    await run('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=5m', './packages/go/platform/derivedmonitoring/internal/resultstore'], 'result-storage.log');
    if (database) await run('go', ['test', '-p', '1', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=45m', './packages/go/platform/resources', '-run', '^(TestDerived|TestAnalysis|TestCalculationRawReader|TestMonitoringCSVPublicationAndFixedDuplicateSet|TestMonitoringPublicationCommitFailureAndRecovery|TestMonitoringActualAdoptionsRevokeAndReadopt)'], 'raw-reader-publication.log');
  }
  if (calculatorLoad) await run('go', ['test', '-tags', 'derivedload', '-count=1', '-v', '-timeout=20m', './packages/go/platform/derivedmonitoring/internal/compute', '-run', '^TestMillionInputCalculator$'], 'calculator-load.log', { GOMAXPROCS: '2', GOMEMLIMIT: '1GiB' });
  passed = true;
} finally {
  writeFileSync(join(output, 'result.json'), JSON.stringify({ runId, calculatorLoad, database, contract, status: passed ? 'passed' : 'failed', scope: [ 'derived rules and raw numeric compatibility', ...(database ? ['isolated definition/channel transactions, HTTP boundaries and affected resource compatibility'] : []), ...(contract ? ['real numeric storage contract'] : []) ], endToEndQualification: false, completedAt: new Date().toISOString() }, null, 2));
  if (contract && !args.includes('--keep')) await run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'stop'], 'stop.log');
}

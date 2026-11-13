import { spawn, execFileSync } from 'node:child_process';
import { createWriteStream, mkdirSync, writeFileSync, readFileSync, mkdtempSync, copyFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const legacy = process.argv.includes('--legacy');
const legacyCommit = '22e236ab4e8f86901572654511720e9b9d655872';
const runId = new Date().toISOString().replaceAll(':', '-');
const output = join(root, '.artifacts', 'monitoring-restore', runId);
mkdirSync(output, { recursive: true, mode: 0o700 });
const project = `infrapro-monitoring-restore-${Date.now().toString(36)}`;
const compose = ['compose', '-p', project, '-f', 'deploy/monitoring-restore/compose.yaml'];
const env = { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1', INFRAPRO_MONITORING_RESTORE: '1', INFRAPRO_RESTORE_ARTIFACTS: output, INFRAPRO_RESTORE_PG_CONTAINER: `${project}-postgres-1` };
async function run(command, args, filename, quiet = false, cwd = root) {
  await new Promise((resolve, reject) => {
    const log = createWriteStream(join(output, filename));
    const child = spawn(command, args, { cwd, env, stdio: ['ignore', 'pipe', 'pipe'] });
    for (const stream of [child.stdout, child.stderr]) stream.on('data', (data) => { log.write(data); if (!quiet) process.stdout.write(data); });
    child.on('error', reject);
    child.on('close', (code) => { log.end(); if (code === 0) resolve(); else reject(new Error(`${filename} failed (${code})`)); });
  });
}
console.log(`Independent restore evidence: ${output}`);
let passed = false;
let legacyDirectory;
try {
  ensureTestDependencies();
  await run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', '--wait-timeout', '180'], 'source.log');
  await run('docker', [...compose, 'up', '-d', '--wait', '--wait-timeout', '180'], 'target.log');
  if (legacy) {
    // A source archive is a test input, not a Git checkout/worktree. The active
    // checkout, branch and all user databases remain untouched.
    legacyDirectory = mkdtempSync(join(tmpdir(), 'infrapro-legacy-v12-'));
    const archive = join(legacyDirectory, 'source.tar');
    execFileSync('git', ['archive', '--format=tar', `--output=${archive}`, legacyCommit], { cwd: root });
    execFileSync('tar', ['-xf', archive, '-C', legacyDirectory]);
    rmSync(archive);
    copyFileSync(join(root, 'scripts/fixtures/derived-legacy-v12.go.txt'), join(legacyDirectory, 'packages/go/platform/resources/derived_legacy_fixture_test.go'));
    const testPath = join(legacyDirectory, 'packages/go/platform/resources/monitoring_restore_test.go');
    let source = readFileSync(testPath, 'utf8');
    for (const [before, after] of [
      ['f := monitoringSetup(t)', 'f := monitoringSetup(t)\n\tlegacy := prepareLegacyUpgrade(t, f)'],
      ['t.Log(string(raw))', 't.Log(string(raw))\n\twriteLegacyUpgradeProof(t, f, pool, legacy, tenantActor, projectActor.ID, project.id)'],
    ]) {
      if (source.split(before).length !== 2) throw new Error('Legacy fixture insertion point changed');
      source = source.replace(before, after);
    }
    writeFileSync(testPath, source);
    writeFileSync(join(output, 'legacy-source.json'), JSON.stringify({ commit: legacyCommit, migrationVersion: 12, fixture: 'scripts/fixtures/derived-legacy-v12.go.txt' }, null, 2));
  }
  await run('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=15m', './packages/go/platform/resources', '-run', '^TestMonitoringIndependentRestore$'], 'restore.log', false, legacyDirectory ?? root);
  if (legacy) {
    await run('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=10m', './packages/go/platform/resources', '-run', '^TestDerivedUpgradeFromLegacyBackup$'], 'upgrade.log');
    await run('go', ['test', '-tags', 'monitoringintegration', '-count=1', '-v', './packages/go/platform/resources', '-run', '^TestLegacyRejectsNewMigration$'], 'old-instance-readiness.log', false, legacyDirectory);
  }
  passed = true;
} finally {
  writeFileSync(join(output, 'result.json'), JSON.stringify({ runId, project, status: passed ? 'passed' : 'failed', completedAt: new Date().toISOString() }, null, 2));
  await run('docker', [...compose, 'logs', '--no-color', '--tail=100'], 'target-services.log', true);
  // Only the uniquely named target created by this invocation is removed.
  if (!process.argv.includes('--keep')) await run('docker', [...compose, 'down', '--volumes'], 'cleanup.log');
  if (legacyDirectory) rmSync(legacyDirectory, { recursive: true });
}

import { spawn } from 'node:child_process';
import { createWriteStream, mkdirSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const runId = new Date().toISOString().replaceAll(':', '-');
const output = join(root, '.artifacts', 'monitoring-restore', runId);
mkdirSync(output, { recursive: true, mode: 0o700 });
const project = `infrapro-monitoring-restore-${Date.now().toString(36)}`;
const compose = ['compose', '-p', project, '-f', 'deploy/monitoring-restore/compose.yaml'];
const env = { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1', INFRAPRO_MONITORING_RESTORE: '1', INFRAPRO_RESTORE_ARTIFACTS: output, INFRAPRO_RESTORE_PG_CONTAINER: `${project}-postgres-1` };
async function run(command, args, filename, quiet = false) {
  await new Promise((resolve, reject) => {
    const log = createWriteStream(join(output, filename));
    const child = spawn(command, args, { cwd: root, env, stdio: ['ignore', 'pipe', 'pipe'] });
    for (const stream of [child.stdout, child.stderr]) stream.on('data', (data) => { log.write(data); if (!quiet) process.stdout.write(data); });
    child.on('error', reject);
    child.on('close', (code) => { log.end(); if (code === 0) resolve(); else reject(new Error(`${filename} failed (${code})`)); });
  });
}
console.log(`Independent restore evidence: ${output}`);
let passed = false;
try {
  ensureTestDependencies();
  await run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', '--wait-timeout', '180'], 'source.log');
  await run('docker', [...compose, 'up', '-d', '--wait', '--wait-timeout', '180'], 'target.log');
  await run('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=15m', './packages/go/platform/resources', '-run', '^TestMonitoringIndependentRestore$'], 'restore.log');
  passed = true;
} finally {
  writeFileSync(join(output, 'result.json'), JSON.stringify({ runId, project, status: passed ? 'passed' : 'failed', completedAt: new Date().toISOString() }, null, 2));
  await run('docker', [...compose, 'logs', '--no-color', '--tail=100'], 'target-services.log', true);
  // Only the uniquely named target created by this invocation is removed.
  if (!process.argv.includes('--keep')) await run('docker', [...compose, 'down', '--volumes'], 'cleanup.log');
}

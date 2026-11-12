import { spawn } from 'node:child_process';
import { createWriteStream, mkdirSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const runId = new Date().toISOString().replaceAll(':', '-');
const output = join(root, '.artifacts', 'monitoring-integration', runId);
mkdirSync(output, { recursive: true });
const env = { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1' };
function run(command, args, filename) {
  return new Promise((resolve, reject) => {
    const log = createWriteStream(join(output, filename));
    const child = spawn(command, args, { cwd: root, env, stdio: ['ignore', 'pipe', 'pipe'] });
    for (const stream of [child.stdout, child.stderr]) stream.on('data', (chunk) => { log.write(chunk); process.stdout.write(chunk); });
    child.on('error', reject);
    child.on('close', (code) => { log.end(); if (code === 0) resolve(); else reject(new Error(`${filename}: command failed (${code})`)); });
  });
}
console.log(`Monitoring functional evidence: ${output}`);
let passed = false;
try {
  ensureTestDependencies();
  // Reuse is explicit: the real contract suite must already have initialized
  // and qualified this disposable lab. Default runs include its full evidence.
  if (!process.argv.includes('--reuse-contract')) await run('node', ['scripts/test-monitoring-contract.mjs', '--keep'], 'contract.log');
  await run('node', ['--test', 'tests/unit/monitoring-client.test.mjs'], 'client-rules.log');
  await run('go', ['test', '-race', '-count=1', './packages/go/platform/monitoring/...'], 'rules.log');
  await run('go', ['test', '-race', '-tags', 'monitoringintegration', '-count=1', '-v', '-timeout=15m', './packages/go/platform/resources', '-run', '^TestMonitoring'], 'integration.log');
  await run('go', ['test', '-count=1', './services/api/internal/transport/http/...', './tools/infrapro/internal/command'], 'http-cli.log');
  passed = true;
} finally {
  writeFileSync(join(output, 'result.json'), JSON.stringify({ runId, status: passed ? 'passed' : 'failed', contractReused: process.argv.includes('--reuse-contract'), scope: 'synthetic samples, local functional and failure tests', completedAt: new Date().toISOString() }, null, 2));
  if (!process.argv.includes('--keep')) await run('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'stop'], 'stop.log');
}

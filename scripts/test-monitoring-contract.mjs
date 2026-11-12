import { spawn, execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync, createWriteStream } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

const root = fileURLToPath(new URL('../', import.meta.url));
const runId = new Date().toISOString().replaceAll(':', '-');
const output = join(root, '.artifacts', 'monitoring-contract', runId);
mkdirSync(output, { recursive: true });
const compose = ['compose', '-f', join(root, 'deploy/monitoring-contract/compose.yaml')];
const environment = { ...process.env, INFRAPRO_MONITORING_CONTRACT: '1', INFRAPRO_CONTRACT_ROOT: root, INFRAPRO_CONTRACT_ARTIFACTS: output };
function run(command, args, filename, json = false) {
  return new Promise((resolve, reject) => {
    const log = createWriteStream(join(output, filename));
    const child = spawn(command, args, { cwd: root, env: environment, stdio: ['ignore', 'pipe', 'pipe'] });
    let pending = '';
    child.stdout.on('data', (data) => {
      log.write(data);
      if (!json) return;
      pending += data.toString();
      const lines = pending.split('\n'); pending = lines.pop();
      for (const line of lines) {
        const event = JSON.parse(line);
        if (event.Action === 'pass' || event.Action === 'fail') console.log(`${event.Action}: ${event.Test ?? event.Package} (${event.Elapsed}s)`);
        if (event.Output?.includes('CAPACITY')) process.stdout.write(event.Output);
      }
    });
    child.stderr.on('data', (data) => log.write(data));
    child.on('error', (error) => { log.end(); reject(error); });
    child.on('close', (code) => { log.end(); if (code === 0) resolve(); else reject(new Error(`${command} failed (${code}); see ${join(output, filename)}`)); });
  });
}

console.log(`S-01 real component evidence: ${output}`);
let passed = false;
try {
  await run('docker', [...compose, 'up', '-d', '--wait', '--wait-timeout', '180'], 'compose.log');
  const hardware = execFileSync('docker', ['info', '--format', '{{json .}}'], { encoding: 'utf8' });
  const info = JSON.parse(hardware);
  writeFileSync(join(output, 'environment.json'), JSON.stringify({ runId, cpus: info.NCPU, memoryBytes: info.MemTotal, architecture: info.Architecture, docker: info.ServerVersion, go: execFileSync('go', ['version'], { encoding: 'utf8' }).trim() }, null, 2));
  await run('go', ['test', '-race', '-count=1', './packages/go/platform/monitoring/internal/observation'], 'rules.log');
  await run('go', ['test', '-race', '-json', '-tags', 'monitoringintegration', '-count=1', '-timeout=10m', './packages/go/platform/monitoring/contracttest'], 'tests.jsonl', true);
  passed = true;
} finally {
  writeFileSync(join(output, 'result.json'), JSON.stringify({ status: passed ? 'passed' : 'failed', scope: 'S-01 synthetic P-0, local services only', completedAt: new Date().toISOString() }, null, 2));
  // Preserve logs on failure; never touch the application's local stack.
  if (!process.argv.includes('--keep')) await run('docker', [...compose, 'stop'], 'stop.log');
}

import { spawn, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createWriteStream, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { cpus, totalmem, platform, arch } from 'node:os';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
import { sampleProcessTree } from './verification-processes.mjs';

const root = fileURLToPath(new URL('../', import.meta.url));
const args = process.argv.slice(2);
if (args.length && (args.length !== 2 || args[0] !== '--operator' || !['rate', 'mean', 'variance', 'quantile', 'interactions'].includes(args[1]))) throw new Error('Usage: test-derived-load.mjs [--operator rate|mean|variance|quantile|interactions]');
const operator = args[1];
const started = new Date();
const output = join(root, '.artifacts/derived-load', started.toISOString().replaceAll(':', '-'));
mkdirSync(output, { recursive: true });
const command = (name, argv) => execFileSync(name, argv, { cwd: root, encoding: 'utf8' }).trim();
const files = command('git', ['ls-files', '-co', '--exclude-standard', '-z']).split('\0').filter(Boolean).sort();
const source = createHash('sha256');
const inventory = [];
for (const path of new Set(files)) {
  const bytes = readFileSync(join(root, path));
  const sha256 = createHash('sha256').update(bytes).digest('hex');
  source.update(path).update('\0').update(sha256).update('\n');
  inventory.push({ path, sha256, bytes: bytes.length });
}
writeFileSync(join(output, 'source.json'), JSON.stringify({ head: command('git', ['rev-parse', 'HEAD']), sha256: source.digest('hex'), files: inventory }, null, 2));
writeFileSync(join(output, 'worktree.txt'), command('git', ['status', '--short']));
console.log(`DP-0 real pipeline evidence: ${output}`);
ensureTestDependencies();
const docker = JSON.parse(command('docker', ['info', '--format', '{{json .}}']));
const containers = command('docker', ['ps', '--format', '{{json .}}']).split('\n').filter(Boolean).map(JSON.parse);
const ids = command('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'ps', '-q']).split('\n').filter(Boolean);
const services = ids.length ? JSON.parse(command('docker', ['inspect', ...ids])).map(v => ({ name: v.Name, image: v.Image, memoryLimit: v.HostConfig.Memory, nanoCPUs: v.HostConfig.NanoCpus, command: v.Config.Cmd, runtime: v.Config.Env.filter(e => /^(GOMEMLIMIT|GOGC|GOMAXPROCS)=/.test(e)) })) : [];
writeFileSync(join(output, 'environment.json'), JSON.stringify({ startedAt: started.toISOString(), operator: operator ?? 'all', node: process.version, go: command('go', ['version']), host: { platform: platform(), arch: arch(), cpus: cpus().length, memoryBytes: totalmem() }, docker: { version: docker.ServerVersion, cpus: docker.NCPU, memoryBytes: docker.MemTotal }, services, containers, versions: JSON.parse(readFileSync(join(root, 'deploy/monitoring-contract/versions.json'), 'utf8')), processBudget: { GOMAXPROCS: '2', GOMEMLIMIT: '1GiB' } }, null, 2));
const log = createWriteStream(join(output, 'pipeline.log'));
const metrics = [];
const interactions = [];
let pending = '';
const resources = createWriteStream(join(output, 'docker-samples.jsonl'));
const hostResources = createWriteStream(join(output, 'host-process-samples.jsonl'));
let sampler;
let processPID;
function sample() {
  if (sampler) return;
  hostResources.write(JSON.stringify({ at: new Date().toISOString(), ...sampleProcessTree(processPID) }) + '\n');
  let body = '';
  sampler = spawn('docker', ['stats', '--no-stream', '--format', '{{json .}}'], { stdio: ['ignore', 'pipe', 'pipe'] });
  sampler.stdout.on('data', b => { body += b; });
  sampler.stderr.on('data', () => {});
  sampler.on('error', e => resources.write(JSON.stringify({ at: new Date().toISOString(), error: e.message }) + '\n'));
  sampler.on('close', code => {
    resources.write(JSON.stringify({ at: new Date().toISOString(), code, samples: body.trim().split('\n').filter(Boolean).map(line => { try { return JSON.parse(line); } catch { return { invalid: line }; } }) }) + '\n');
    sampler = undefined;
  });
}
sample();
const timer = setInterval(sample, 15000);
let status = 'failed';
try {
  await new Promise((resolve, reject) => {
    const pattern = operator === 'interactions' ? '^TestDerivedInteractionTargets$' : operator ? `^TestMillionInputPipeline$/${operator}$` : '^(TestMillionInputPipeline|TestDerivedInteractionTargets)$';
    const child = spawn('go', ['test', '-p', '1', '-tags=monitoringintegration,derivedload', './packages/go/platform/resources', '-run', pattern, '-count=1', '-v', '-timeout=4h'], { cwd: root, env: { ...process.env, ...testEnv, INFRAPRO_MONITORING_CONTRACT: '1', GOMAXPROCS: '2', GOMEMLIMIT: '1GiB' }, stdio: ['ignore', 'pipe', 'pipe'] });
    processPID = child.pid;
    for (const stream of [child.stdout, child.stderr]) stream.on('data', chunk => {
      log.write(chunk); process.stdout.write(chunk); pending += chunk;
      const lines = pending.split('\n'); pending = lines.pop();
      for (const line of lines) {
        const index = line.indexOf('DP0_PIPELINE '); if (index >= 0) metrics.push(JSON.parse(line.slice(index + 13)));
        const interaction = line.indexOf('DP0_INTERACTIONS '); if (interaction >= 0) interactions.push(JSON.parse(line.slice(interaction + 17)));
      }
    });
    child.on('error', reject);
    child.on('close', code => code === 0 ? resolve() : reject(new Error(`DP-0 pipeline failed (${code}); evidence ${output}`)));
  });
  status = 'passed';
} finally {
  clearInterval(timer);
  if (sampler) { sampler.kill(); await new Promise(resolve => sampler.once('close', resolve)); }
  resources.end(); hostResources.end(); log.end();
  writeFileSync(join(output, 'result.json'), JSON.stringify({ status, operator: operator ?? 'all', startedAt: started.toISOString(), completedAt: new Date().toISOString(), elapsedSeconds: (Date.now() - started) / 1000, metrics, interactions, fullQualification: !operator && status === 'passed' && metrics.length === 4 && interactions.length === 1 }, null, 2));
}

import { spawn, execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync, rmSync, readFileSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { createServer } from 'node:net';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';

const output = resolve('.artifacts/mfa', new Date().toISOString().replaceAll(':', '-'));
const privateDirectory = mkdtempSync(join(tmpdir(), 'infrapro-mfa-tests-'));
mkdirSync(output, { recursive: true });
async function port() { const s = createServer(); await new Promise(r => s.listen(0, '127.0.0.1', r)); const p = s.address().port; await new Promise(r => s.close(r)); return p; }
const api = await port(), consolePort = await port();
const binary = join(privateDirectory, 'infrapro');
const env = { ...process.env, ...testEnv, INFRAPRO_MFA_RESTORE: '1', INFRAPRO_MFA_CLI_BINARY: binary, INFRAPRO_E2E_API_URL: `http://127.0.0.1:${api}`, INFRAPRO_E2E_CONSOLE_URL: `http://127.0.0.1:${consolePort}`, INFRAPRO_E2E_FIXTURE_FILE: join(privateDirectory, 'fixture.json') };
const checks = [];
const redact = text => text.replace(/^\s*-\s*(?:cookie|authorization|x-csrf-token):.*$/gim, '    - authentication header: [redacted]');
function scrubFailureArtifacts(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) scrubFailureArtifacts(path);
    else if (entry.name === 'error-context.md') writeFileSync(path, redact(readFileSync(path, 'utf8')));
  }
}
async function run(command, args, name) {
  const started = Date.now();
  const code = await new Promise((resolve, reject) => {
    const child = spawn(command, args, { env, stdio: ['ignore', 'pipe', 'pipe'] });
    for (const stream of [child.stdout, child.stderr]) {
      let pending = '';
      stream.on('data', chunk => {
        pending += chunk.toString();
        const boundary = pending.lastIndexOf('\n');
        if (boundary >= 0) { process.stdout.write(redact(pending.slice(0, boundary + 1))); pending = pending.slice(boundary + 1); }
      });
      stream.on('end', () => { if (pending) process.stdout.write(redact(pending)); });
    }
    const stop = () => child.kill('SIGTERM'); process.once('SIGTERM', stop); process.once('SIGINT', stop);
    child.once('error', reject); child.once('exit', code => { process.off('SIGTERM', stop); process.off('SIGINT', stop); resolve(code ?? 1); });
  });
  checks.push({ name, command: [command, ...args], passed: code === 0, seconds: (Date.now() - started) / 1000 });
  if (code) throw new Error(name + ' failed; see the finite test error above');
}
let passed = false;
try {
  ensureTestDependencies();
  await run('node', ['--test', 'tests/unit/mfa-credential.test.mjs'], 'memory-only recovery fragment purpose and account isolation');
  execFileSync('go', ['build', '-o', binary, './tools/infrapro/cmd/infrapro'], { stdio: 'inherit' });
  await run('go', ['test', '-race', '-count=1', './packages/go/platform/identity', '-run', '^TestMFA'], 'identity, atomicity, migration and real snapshot restore');
  await run('go', ['test', '-race', '-count=1', './services/api/internal/transport/http', '-run', '^TestMFA(HTTP|CLI)'], 'HTTP, TLS and real interactive CLI');
  await run('go', ['test', '-race', '-count=1', './packages/go/platform/runtime', '-run', '^TestMFA'], 'machine processes without personnel decryption keys');
  await run('node', ['scripts/e2e.mjs', 'tests/e2e/mfa.spec.ts', 'tests/e2e/mfa-policy.spec.ts', '--reporter=line', '--output', join(output, 'browser')], 'desktop and narrow-screen MFA journeys');
  passed = true;
} finally {
  rmSync(privateDirectory, { recursive: true, force: true });
  scrubFailureArtifacts(output);
  writeFileSync(join(output, 'result.json'), JSON.stringify({ passed, synthetic: true, checks, completed_at: new Date().toISOString() }, null, 2) + '\n');
}
console.log('MFA evidence: ' + join(output, 'result.json'));

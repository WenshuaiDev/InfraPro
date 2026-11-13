import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
import { seedDerivedAcceptance } from './fixtures/derived-acceptance.mjs';

// A disposable, explicitly synthetic acceptance session. Each start owns a
// new PG schema, CH database and S3 prefix; TestE2EServer removes only these
// resources on graceful shutdown. Existing local application stacks are unused.
const root = fileURLToPath(new URL('../', import.meta.url));
process.chdir(root);
if (process.argv.length > 2) throw new Error('Usage: pnpm derived:demo');
const origin = 'http://127.0.0.1:15373', apiURL = 'http://127.0.0.1:18380';
for (const port of [15373, 18380]) {
  await new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', () => reject(new Error(`Port ${port} is already in use; the existing process was left untouched.`)));
    server.listen(port, '127.0.0.1', () => server.close(resolve));
  });
}
const directory = join(root, '.artifacts/derived-demo', new Date().toISOString().replaceAll(':', '-'));
mkdirSync(directory, { recursive: true, mode: 0o700 });
ensureTestDependencies();
execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', '--wait-timeout', '180'], { stdio: 'inherit' });
const binary = join(directory, 'api-test');
execFileSync('go', ['test', '-c', '-o', binary, './services/api/internal/transport/http'], { stdio: 'inherit' });
const env = { ...process.env, ...testEnv, INFRAPRO_CONFIG: '', INFRAPRO_E2E_SERVER: '1', INFRAPRO_MONITORING_E2E: '1', INFRAPRO_HTTP_ADDR: '127.0.0.1:18380', INFRAPRO_E2E_CONSOLE_URL: origin, INFRAPRO_E2E_ORIGINS: origin, INFRAPRO_E2E_FIXTURE_FILE: join(directory, 'fixture.json'), INFRAPRO_E2E_OPS_CONFIG: join(directory, 'operator.json'), API_PROXY_TARGET: apiURL, CONSOLE_PORT: '15373', VITE_API_BASE_URL: '/api/v1' };
const children = [];
let stopped = false;
const stop = code => {
  if (stopped) return;
  stopped = true;
  process.exitCode = code;
  writeFileSync(join(directory, 'session.json'), JSON.stringify({ state: 'stopped', stoppedAt: new Date().toISOString(), synthetic: true }, null, 2), { mode: 0o600 });
  for (const child of children) { try { process.kill(-child.pid, 'SIGTERM'); } catch { /* already exited */ } }
  const timer = setTimeout(() => {
    for (const child of children) { try { process.kill(-child.pid, 'SIGKILL'); } catch { /* already exited */ } }
    process.exit(code);
  }, 20000);
  timer.unref();
};
for (const [command, args] of [[binary, ['-test.run=^TestE2EServer$', '-test.timeout=0']], ['pnpm', ['--filter', '@infrapro/console', 'dev']]]) {
  const child = spawn(command, args, { cwd: root, env, detached: true, stdio: 'inherit' });
  children.push(child);
  child.on('error', error => { console.error(error.message); stop(1); });
  child.on('exit', code => stop(code || 0));
}
process.on('SIGINT', () => stop(0)); process.on('SIGTERM', () => stop(0));
async function ready(url) {
  const deadline = Date.now() + 120000;
  while (Date.now() < deadline && !stopped) {
    try { if ((await fetch(url, { signal: AbortSignal.timeout(1000) })).ok) return; } catch { /* startup */ }
    await new Promise(resolve => setTimeout(resolve, 500));
  }
  throw new Error(`Acceptance service did not become ready: ${url}`);
}
try {
  await ready(`${apiURL}/readyz`);
  await ready(`${origin}/api/v1/public/account-help`);
  const result = await seedDerivedAcceptance(directory, origin);
  writeFileSync(join(directory, 'session.json'), JSON.stringify({ state: 'ready', readyAt: new Date().toISOString(), synthetic: true, ...result }, null, 2), { mode: 0o600 });
  console.log(`\n派生监测合成人工验收入口：${result.url}\n登录资料：${join(directory, 'login.json')}\n样本和核对记录：${join(directory, 'acceptance.json')}\n此会话停止后清理本次隔离数据；再次启动会创建新会话。既有业务数据不受影响。`);
} catch (error) { console.error(error.message); stop(1); }

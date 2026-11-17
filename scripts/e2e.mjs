import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
ensureTestDependencies();
const env = { ...process.env, ...testEnv };
if (env.INFRAPRO_E2E_PREVIEW) {
  const root = resolve('.artifacts/e2e-preview');
  mkdirSync(root, { recursive: true });
  env.INFRAPRO_E2E_PREVIEW_DIR = mkdtempSync(join(root, 'run-'));
  env.VITE_APP_BASE = new URL(env.INFRAPRO_E2E_CONSOLE_URL || 'http://127.0.0.1:15173').pathname;
  execFileSync('pnpm', ['--filter', '@infrapro/console', 'build', '--outDir', env.INFRAPRO_E2E_PREVIEW_DIR, '--emptyOutDir'], { env, stdio: 'inherit' });
}
const child = spawn('pnpm', ['exec', 'playwright', 'test', ...process.argv.slice(2)], { stdio: 'inherit', env, detached: process.platform !== 'win32' });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => {
  try { process.kill(process.platform === 'win32' ? child.pid : -child.pid, signal); }
  catch (error) { if (error.code !== 'ESRCH') throw error; }
});
child.on('exit', (code) => process.exit(code ?? 1));

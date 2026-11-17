// THROWAWAY frontend only. Uses the running local API; prototype actions remain in memory.
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('../', import.meta.url));
const port = '25174';
const url = `http://localhost:${port}/t/64a4ebf9-c549-4706-94a9-117333b62d4b/spatial?variant=A`;
try {
  const existing = await fetch(`http://localhost:${port}/@vite/client`, { signal: AbortSignal.timeout(1000) });
  if (existing.ok && (await existing.text()).includes('WebSocket')) {
    console.log(`Spatial prototype is already running: ${url}`);
    process.exit(0);
  }
} catch { /* Start the isolated frontend if the prototype port is unused. */ }
console.log(`Spatial prototype: ${url}\nUse the existing localhost login. A / B / C share in-memory demo state.`);
const child = spawn('pnpm', ['--filter', '@infrapro/console', 'dev'], {
  cwd: root, stdio: 'inherit',
  env: { ...process.env, CONSOLE_PORT: port, API_PROXY_TARGET: 'http://127.0.0.1:28080', VITE_API_BASE_URL: '/api/v1', VITE_APP_BASE: '/' },
});
process.on('SIGINT', () => child.kill('SIGINT'));
process.on('SIGTERM', () => child.kill('SIGTERM'));
child.on('exit', code => process.exit(code ?? 0));

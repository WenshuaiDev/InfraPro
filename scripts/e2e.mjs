import { spawn } from 'node:child_process';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
ensureTestDependencies();
const child = spawn('pnpm', ['exec', 'playwright', 'test', ...process.argv.slice(2)], { stdio: 'inherit', env: { ...process.env, ...testEnv } });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('exit', (code) => process.exit(code ?? 1));

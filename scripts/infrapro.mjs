import { existsSync } from 'node:fs';
import { spawn } from 'node:child_process';
if (existsSync('.env')) process.loadEnvFile('.env');
const child = spawn('go', ['run', './tools/infrapro/cmd/infrapro', ...process.argv.slice(2)], { stdio: 'inherit' });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('exit', (code) => process.exit(code ?? 1));

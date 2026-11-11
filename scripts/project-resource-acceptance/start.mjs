import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { environment, origin, root } from './environment.mjs';

const env = environment();
if (process.argv.includes('--seed-mode')) {
  // Only initial bulk preparation needs these limits; ordinary startup uses defaults.
  Object.assign(env, { INFRAPRO_INVITATION_RATE: '200', INFRAPRO_INVITATION_LIMIT: '100', INFRAPRO_IP_REQUEST_LIMIT: '10000', INFRAPRO_CREDENTIAL_REQUEST_LIMIT: '1000' });
}
const binary = resolve(root, 'build/project-resource-acceptance/api');
if (!existsSync(binary)) throw new Error('Run node scripts/project-resource-acceptance/setup.mjs first.');
const children = [];
let stopping = false;
function stop(code) {
  if (stopping) return;
  stopping = true;
  const pending = children.filter((child) => child.exitCode === null && child.signalCode === null);
  const done = pending.map((child) => new Promise((resolve) => child.once('close', resolve)));
  const signal = (child, name) => { try { process.kill(-child.pid, name); } catch (error) { if (error.code !== 'ESRCH') console.error(error.message); } };
  pending.forEach((child) => signal(child, 'SIGTERM'));
  const timer = setTimeout(() => pending.forEach((child) => signal(child, 'SIGKILL')), 8000);
  Promise.all(done).then(() => { clearTimeout(timer); process.exit(code); });
}
for (const [command, args] of [[binary, []], ['pnpm', ['--filter', '@infrapro/console', 'dev']]]) {
  const child = spawn(command, args, { cwd: root, env, stdio: 'inherit', detached: true });
  children.push(child);
  child.on('error', (error) => { console.error(error.message); stop(1); });
  child.on('exit', (code) => { if (!stopping) stop(code || 1); });
}
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => stop(0));
console.log(`Acceptance console: ${origin}/ — Ctrl+C stops services and retains all data.`);

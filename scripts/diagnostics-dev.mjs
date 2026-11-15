import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
const explicit = process.argv.indexOf('--config');
const server = explicit >= 0 ? resolve(process.argv[explicit + 1]) : resolve('.artifacts/diagnostics/dev/server.json');
mkdirSync(resolve('build/bin'), { recursive: true });
execFileSync('go', ['build', '-o', 'build/bin/infrapro-diagnostics-dev', './services/diagnostics/cmd/infrapro-diagnostics'], { stdio: 'inherit' });
if (!existsSync(server)) {
  if (explicit >= 0) throw Error('The requested diagnostic configuration does not exist.');
  mkdirSync(dirname(dirname(server)), { recursive: true, mode: 0o700 });
  execFileSync('go', ['run', './tools/infrapro/cmd/infrapro', 'diagnostics', 'config', 'init', '--directory', dirname(server), '--instance-id', 'development', '--profile', 'native'], { stdio: 'inherit' });
  const cfg = JSON.parse(readFileSync(server, 'utf8')); cfg.development_origins = ['http://127.0.0.1:5196'];
  writeFileSync(server, JSON.stringify(cfg, null, 2), { mode: 0o600 });
}
const cfg = JSON.parse(readFileSync(server, 'utf8'));
if (!cfg.development_origins?.includes('http://127.0.0.1:5196')) throw Error('Development configuration must explicitly allow http://127.0.0.1:5196.');
const children = [spawn(resolve('build/bin/infrapro-diagnostics-dev'), ['server', '--development', '--config', server], { stdio: 'inherit' }), spawn('pnpm', ['--filter', '@infrapro/diagnostics', 'dev'], { stdio: 'inherit', env: { ...process.env, DIAGNOSTICS_PROXY_TARGET: cfg.public_origin } })];
console.log(`独立诊断开发页面：http://127.0.0.1:5196\n配置：${server}\n首次使用请在另一终端执行 pnpm infrapro -- diagnostics auth init --server ${server} --username operator`);
let closed = false;
const stop = () => { if (closed) return; closed = true; for (const child of children) if (child.exitCode === null) child.kill('SIGTERM'); };
process.on('SIGINT', stop); process.on('SIGTERM', stop); for (const child of children) child.on('exit', stop);

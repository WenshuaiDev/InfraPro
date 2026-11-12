import { execFileSync, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseEnv } from 'node:util';

const root = fileURLToPath(new URL('../', import.meta.url));
process.chdir(root);
const directory = join(root, '.artifacts/monitoring-local');
const envFile = join(directory, '.env');
const command = process.argv[2];
if (!['setup', 'start', 'seed', 'cli', 'status'].includes(command)) throw new Error('Use setup | start | seed | cli <infrapro arguments> | status');
mkdirSync(directory, { recursive: true, mode: 0o700 });
if (command === 'setup' && !existsSync(envFile)) {
  const pg = randomBytes(24).toString('hex'), redis = randomBytes(24).toString('hex');
  const settings = {
    POSTGRES_PASSWORD: pg, REDIS_PASSWORD: redis, INFRAPRO_CONFIG: '', INFRAPRO_LOCAL_SYNTHETIC: '1',
    INFRAPRO_DATABASE_URL: `postgres://acceptance_local:${pg}@127.0.0.1:58432/acceptance_local?sslmode=disable`,
    INFRAPRO_REDIS_URL: `redis://:${redis}@127.0.0.1:58379/0`, INFRAPRO_REDIS_KEY_PREFIX: 'infrapro:monitoring-local:identity:v1:',
    INFRAPRO_CSRF_AUTH_KEY: randomBytes(32).toString('hex'), INFRAPRO_COOKIE_SECURE: 'false',
    INFRAPRO_HTTP_ADDR: '127.0.0.1:18280', INFRAPRO_PUBLIC_CONSOLE_URL: 'http://127.0.0.1:15273/',
    INFRAPRO_TRUSTED_ORIGINS: 'http://127.0.0.1:15273', INFRAPRO_SUPPORT_CONTACT: '本机合成验收环境维护人员',
    INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS: '127.0.0.1:61000,127.0.0.1:61001',
    INFRAPRO_MONITORING_CLICKHOUSE_DATABASE: 'monitoring_acceptance', INFRAPRO_MONITORING_CLICKHOUSE_USER: 'default',
    INFRAPRO_MONITORING_CLICKHOUSE_PASSWORD: '', INFRAPRO_MONITORING_CLICKHOUSE_POLICY: 'monitoring_tiered', INFRAPRO_MONITORING_CLICKHOUSE_TLS: 'false',
    INFRAPRO_MONITORING_OBJECT_ENDPOINT: 'http://127.0.0.1:61333', INFRAPRO_MONITORING_OBJECT_ACCESS_KEY: 'acceptance-source',
    INFRAPRO_MONITORING_OBJECT_SECRET_KEY: 'acceptance-source-test-only', INFRAPRO_MONITORING_SOURCE_BUCKET: 'acceptance-source',
    INFRAPRO_MONITORING_TEMP_DIRECTORY: directory, INFRAPRO_MONITORING_PHYSICAL_SHARDS: '[]',
    INFRAPRO_MONITORING_BACKUP_ENDPOINT: 'http://127.0.0.1:61334', INFRAPRO_MONITORING_BACKUP_ACCESS_KEY: 'acceptance-backup',
    INFRAPRO_MONITORING_BACKUP_SECRET_KEY: 'acceptance-backup-test-only', INFRAPRO_MONITORING_BACKUP_BUCKET: 'acceptance-backups',
    INFRAPRO_MONITORING_BACKUP_NAMED_COLLECTION: 'monitoring_backups',
    API_PROXY_TARGET: 'http://127.0.0.1:18280', CONSOLE_PORT: '15273', VITE_API_BASE_URL: '/api/v1',
  };
  writeFileSync(envFile, Object.entries(settings).map(([key, value]) => `${key}=${JSON.stringify(value)}`).join('\n') + '\n', { mode: 0o600, flag: 'wx' });
  writeFileSync(join(directory, 'login.json'), JSON.stringify({ username: 'monitoring_local', password: randomBytes(24).toString('base64url'), url: settings.INFRAPRO_PUBLIC_CONSOLE_URL }, null, 2) + '\n', { mode: 0o600, flag: 'wx' });
}
if (!existsSync(envFile)) throw new Error('Run pnpm monitoring:local setup first.');
const env = { ...process.env, ...parseEnv(readFileSync(envFile, 'utf8')) };
const run = (bin, args, extra = {}) => execFileSync(bin, args, { cwd: root, env: { ...env, ...extra }, stdio: 'inherit' });
const cli = (args) => run('go', ['run', './tools/infrapro/cmd/infrapro', ...args]);
if (command === 'setup') {
  run('docker', ['compose', '--env-file', envFile, '-f', 'deploy/monitoring-local/compose.yaml', 'up', '-d', '--wait', '--wait-timeout', '180']);
  cli(['database', 'migrate']); cli(['access', 'sync']);
  run('go', ['run', './tools/infrapro/cmd/monitoring-local-fixture']);
  cli(['monitoring', 'storage', 'init']);
  console.log(`本机存储已就绪。登录资料：${join(directory, 'login.json')}。运行 pnpm monitoring:local start，再在另一终端运行 pnpm monitoring:local seed。`);
} else if (command === 'cli') cli(process.argv.slice(3));
else if (command === 'status') cli(['monitoring', 'storage', 'status']);
else if (command === 'seed') {
  if (!existsSync(join(directory, 'contracts/samples/manifest.json'))) run('go', ['test', '-tags', 'monitoringintegration', '-count=1', '-run', '^TestSyntheticSampleFiles$', './packages/go/platform/monitoring/contracttest'], { INFRAPRO_CONTRACT_ARTIFACTS: join(directory, 'contracts') });
  const { seed } = await import('./monitoring-local-seed.mjs'); await seed(directory);
} else {
  mkdirSync(join(root, 'build/monitoring-local'), { recursive: true });
  for (const [name, source] of [['api', './services/api/cmd/api'], ['worker', './services/api/cmd/monitoring-worker']]) run('go', ['build', '-o', `build/monitoring-local/${name}`, source]);
  const children = [];
  let stopped = false;
  const stop = (code) => {
    if (stopped) return;
    stopped = true;
    for (const child of children) { try { process.kill(-child.pid, 'SIGTERM'); } catch { /* already stopped */ } }
    setTimeout(() => { for (const child of children) { try { process.kill(-child.pid, 'SIGKILL'); } catch { /* already stopped */ } } process.exit(code); }, 8000).unref();
  };
  for (const [bin, args] of [['build/monitoring-local/api', []], ['build/monitoring-local/worker', []], ['pnpm', ['--filter', '@infrapro/console', 'dev']]]) {
    const child = spawn(bin, args, { cwd: root, env, detached: true, stdio: 'inherit' }); children.push(child);
    child.on('error', () => stop(1)); child.on('exit', code => stop(code || 0));
  }
  process.on('SIGINT', () => stop(0)); process.on('SIGTERM', () => stop(0));
  console.log('合成本机验收入口：http://127.0.0.1:15273/；Ctrl+C 停止应用，持久卷保留。');
}

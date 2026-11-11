import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { parseEnv } from 'node:util';
import { fileURLToPath } from 'node:url';

export const root = fileURLToPath(new URL('../../', import.meta.url));
export const directory = resolve(root, '.artifacts/manual-acceptance');
export const origin = 'http://localhost:25173';
export const databaseName = 'infrapro_acceptance_20261110';
export const configFile = resolve(directory, 'environment.local.json');
export function savePrivate(name, value) {
  mkdirSync(directory, { recursive: true, mode: 0o700 });
  const path = resolve(directory, name);
  writeFileSync(path, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
  chmodSync(path, 0o600);
}
export function readPrivate(name) {
  return JSON.parse(readFileSync(resolve(directory, name), 'utf8'));
}
export function environment() {
  if (!existsSync(configFile)) throw new Error('Run node scripts/acceptance/setup.mjs first.');
  const own = readPrivate('environment.local.json');
  if (new URL(own.INFRAPRO_DATABASE_URL).pathname !== `/${databaseName}` || own.INFRAPRO_PUBLIC_CONSOLE_URL !== `${origin}/`) {
    throw new Error('Acceptance environment identity mismatch; refusing to proceed.');
  }
  const env = { ...process.env };
  // Never inherit a different deployment or optional config file into this environment.
  for (const key of Object.keys(env)) if (key.startsWith('INFRAPRO_') || key.startsWith('VITE_')) delete env[key];
  return { ...env, ...own, CONSOLE_PORT: '25173', API_PROXY_TARGET: 'http://127.0.0.1:28080', VITE_API_BASE_URL: '/api/v1', VITE_APP_BASE: '/' };
}
export function provision() {
  if (existsSync(configFile)) return environment();
  const local = parseEnv(readFileSync(resolve(root, '.env'), 'utf8'));
  const url = new URL(local.INFRAPRO_DATABASE_URL);
  if (!['127.0.0.1', 'localhost'].includes(url.hostname) || url.pathname !== '/infrapro') throw new Error('Expected the documented local PostgreSQL environment.');
  const compose = ['compose', '--env-file', '.env', '-f', 'deploy/compose/local.yaml'];
  const run = (args) => execFileSync('docker', [...compose, ...args], { cwd: root, encoding: 'utf8' });
  const found = run(['exec', '-T', 'postgres', 'psql', '-U', 'infrapro', '-d', 'infrapro', '-Atc', `SELECT 1 FROM pg_database WHERE datname='${databaseName}'`]).trim();
  if (found) throw new Error('The acceptance database already exists without its config. Refusing to adopt or reset it.');
  run(['exec', '-T', 'postgres', 'createdb', '-U', 'infrapro', '-T', 'template0', databaseName]);
  url.pathname = `/${databaseName}`;
  savePrivate('environment.local.json', {
    INFRAPRO_DATABASE_URL: url.href,
    INFRAPRO_REDIS_URL: local.INFRAPRO_REDIS_URL,
    INFRAPRO_REDIS_KEY_PREFIX: 'infrapro:acceptance20261110:identity:v1:',
    INFRAPRO_CSRF_AUTH_KEY: randomBytes(32).toString('hex'),
    INFRAPRO_HTTP_ADDR: '127.0.0.1:28080',
    INFRAPRO_PUBLIC_CONSOLE_URL: `${origin}/`,
    INFRAPRO_TRUSTED_ORIGINS: origin,
    INFRAPRO_COOKIE_SECURE: 'false',
    INFRAPRO_SUPPORT_CONTACT: '全功能本机人工验收环境，请反馈具体账号、页面和操作步骤。',
  });
  return environment();
}

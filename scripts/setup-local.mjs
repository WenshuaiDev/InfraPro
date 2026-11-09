import { randomBytes } from 'node:crypto';
import { existsSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
if (!existsSync('.env')) {
  const databasePassword = randomBytes(24).toString('hex'); const redisPassword = randomBytes(24).toString('hex');
  writeFileSync('.env', [
    `POSTGRES_PASSWORD=${databasePassword}`, `REDIS_PASSWORD=${redisPassword}`,
    `INFRAPRO_DATABASE_URL=postgres://infrapro:${databasePassword}@127.0.0.1:5432/infrapro`,
    `INFRAPRO_REDIS_URL=redis://:${redisPassword}@127.0.0.1:6379/0`,
    'INFRAPRO_REDIS_KEY_PREFIX=infrapro:local:identity:v1:',
    `INFRAPRO_CSRF_AUTH_KEY=${randomBytes(32).toString('hex')}`,
    'INFRAPRO_SUPPORT_CONTACT="本地开发环境：请联系此电脑的维护人员"',
    'INFRAPRO_PUBLIC_CONSOLE_URL=http://127.0.0.1:5173/',
    'INFRAPRO_TRUSTED_ORIGINS=http://127.0.0.1:5173',
    'INFRAPRO_COOKIE_SECURE=false', '',
  ].join('\n'), { mode: 0o600, flag: 'wx' });
}
process.loadEnvFile('.env');
execFileSync('docker', ['compose', '--env-file', '.env', '-f', 'deploy/compose/local.yaml', 'up', '-d', '--wait'], { stdio: 'inherit' });
execFileSync('go', ['run', './tools/infrapro/cmd/infrapro', 'database', 'migrate'], { stdio: 'inherit' });
execFileSync('go', ['run', './tools/infrapro/cmd/infrapro', 'access', 'sync'], { stdio: 'inherit' });
execFileSync('go', ['run', './tools/infrapro/cmd/infrapro', 'access', 'check'], { stdio: 'inherit' });
console.log('本地依赖、迁移与权限目录已就绪。运行 pnpm infrapro account bootstrap 创建首个账号，然后 pnpm dev。');

import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { directory, provision, root, savePrivate } from './environment.mjs';

const env = provision();
mkdirSync(resolve(root, 'build/project-resource-acceptance'), { recursive: true });
for (const [name, path] of [['infrapro', './tools/infrapro/cmd/infrapro'], ['api', './services/api/cmd/api']]) {
  execFileSync('go', ['build', '-o', `build/project-resource-acceptance/${name}`, path], { cwd: root, stdio: 'inherit' });
}
const cli = resolve(root, 'build/project-resource-acceptance/infrapro');
for (const args of [['database', 'migrate'], ['access', 'sync'], ['access', 'check']]) {
  execFileSync(cli, args, { cwd: root, env, stdio: 'inherit' });
}
if (!existsSync(resolve(directory, 'accounts.local.json'))) {
  savePrivate('accounts.local.json', { platform: { username: 'accept_platform', display_name: '验收·平台管理员', purpose: '平台治理、账号、角色、工程发布与审计', password: randomBytes(18).toString('base64url') + '!Aa7' } });
}
// The helper feeds the existing terminal-only CLI without echoing credentials.
execFileSync('python3', [resolve(root, 'scripts/project-resource-acceptance/bootstrap.py')], { cwd: root, env, stdio: 'inherit' });
console.log('Persistent acceptance database and administrator are ready. Start scripts/project-resource-acceptance/start.mjs --seed-mode, then run scripts/project-resource-acceptance/seed.mjs.');

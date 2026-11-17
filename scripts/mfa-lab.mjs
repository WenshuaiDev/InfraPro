import { spawn, execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync, rmSync, appendFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve, join } from 'node:path';
import { createServer } from 'node:net';
import { randomUUID } from 'node:crypto';
import { request } from '@playwright/test';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
import { requireAuthenticatedLogin } from './login-result.mjs';
import { syntheticTOTP } from '../tests/helpers/totp.ts';
import { prepareMFALabDisplay } from './mfa-lab-display.mjs';
import { verifyMFALabUI } from '../tests/mfa/lab-ui.mjs';

const directory = resolve('.artifacts/mfa/manual', new Date().toISOString().replaceAll(':', '-'));
const privateDirectory = mkdtempSync(join(tmpdir(), 'infrapro-mfa-acceptance-'));
mkdirSync(directory, { recursive: true });
const children = [];
async function freePort() { const s = createServer(); await new Promise(r => s.listen(0, '127.0.0.1', r)); const p = s.address().port; await new Promise(r => s.close(r)); return p; }
const [a, b, ui, runtime, assets] = [await freePort(), await freePort(), await freePort(), await freePort(), await freePort()];
const origin = `http://127.0.0.1:${ui}`, api = `http://127.0.0.1:${a}/api/v1`;
const session = { synthetic: true, origin, api_a: `http://127.0.0.1:${a}`, api_b: `http://127.0.0.1:${b}`, runtime_origin: `http://127.0.0.1:${runtime}`, asset_origin: `http://127.0.0.1:${assets}`, credentials_file: join(privateDirectory, 'credentials.json'), operator_config: join(privateDirectory, 'operator.json'), state: 'starting' };
let displayBrowser;
const save = () => writeFileSync(join(directory, 'session.json'), JSON.stringify(session, null, 2) + '\n');
function start(command, args, options = {}) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...options }); children.push(child);
  for (const stream of [child.stdout, child.stderr]) stream.on('data', data => appendFileSync(join(directory, 'server.log'), data));
  return child;
}
async function waitURL(url) { for (let i = 0; i < 160; i++) { try { if ((await fetch(url, { signal: AbortSignal.timeout(1000) })).ok) return; } catch { /* starting */ } if (children.some(c => c.exitCode !== null)) throw Error('MFA lab process exited'); await new Promise(r => setTimeout(r, 250)); } throw Error('MFA lab startup timed out'); }
async function client(account) {
  const http = await request.newContext(), csrf = await (await http.get(api + '/auth/csrf')).json();
  const call = async (path, data, method = data === undefined ? 'GET' : 'POST') => {
    const r = await http.fetch(api + path, { method, headers: { Origin: origin, 'X-CSRF-Token': csrf.token, 'X-Command-ID': randomUUID() }, data });
    if (![200, 201, 204].includes(r.status())) throw Error('MFA lab preparation failed: ' + method + ' ' + path.replace(/[0-9a-f]{8}-[0-9a-f-]+/g, ':id') + ' ' + r.status());
    return r.status() === 204 ? undefined : r.json();
  };
  requireAuthenticatedLogin(await call('/auth/login', { username: account.username, password: account.password }));
  const upload = async (path, data, sha) => {
    const response = await http.put(api + path, { headers: { Origin: origin, 'X-CSRF-Token': csrf.token, 'Content-Type': 'application/octet-stream', 'X-Content-SHA256': sha }, data });
    if (!response.ok()) throw Error('MFA display upload HTTP ' + response.status());
  };
  return { call, http, upload, close: () => http.dispose() };
}
async function seed() {
  const users = JSON.parse(readFileSync(session.credentials_file, 'utf8'));
  const p1 = await client(users.P1), ta = await client(users.TA);
  try {
    const owner = (await p1.call('/access/accounts?q=' + users.P1.username)).items[0];
    for (const [label, roleIds] of [['P2', owner.roles.map(r => r.id)], ['U6', []]]) {
      const target = (await p1.call('/access/accounts?q=' + users[label].username)).items[0];
      await p1.call('/access/accounts/' + target.id + '/roles', { expected_version: target.management_version, role_ids: roleIds, reason: '独立 MFA 验收角色' }, 'PUT');
    }
    const role = await p1.call('/access/roles', { name: 'MFA 策略只读', description: 'P3 合成验收', reason: 'MFA 验收' });
    await p1.call('/access/roles/' + role.id + '/permissions', { expected_version: role.version, permission_codes: ['platform.security_policy.read', 'platform.audit.read'], reason: 'MFA 验收' }, 'PUT');
    const p3 = (await p1.call('/access/accounts?q=' + users.P3.username)).items[0];
    await p1.call('/access/accounts/' + p3.id + '/roles', { expected_version: p3.management_version, role_ids: [...p3.roles.map(r => r.id), role.id], reason: '只读 MFA 验收' }, 'PUT');
    session.tenants = [];
    for (const code of ['a', 'b']) session.tenants.push((await p1.call('/platform/tenants', { name: 'MFA 验收租户 ' + code.toUpperCase(), code: 'mfa-' + code, administrator_account_id: users.TA.id, client_request_id: randomUUID(), reason: '隔离 MFA 验收' })).tenant_id);
    const member = (await ta.call('/tenants/' + session.tenants[0] + '/members')).items.find(m => m.account.id === users.TA.id);
    const project = await ta.call('/tenants/' + session.tenants[0] + '/projects', { name: 'MFA 验收项目', code: 'mfa-project', purpose: '跨空间人员认证验收', client_request_id: randomUUID(), reason: '隔离 MFA 验收', owner_tenant_membership_id: member.id, administrator_tenant_membership_id: member.id });
    session.project = project.project_id;
    for (const label of ['U1', 'U2', 'U3']) {
      const user = await client(users[label]);
      try {
        for (const tenant of label === 'U3' ? session.tenants : session.tenants.slice(0, 1)) {
          const invitation = await ta.call('/tenants/' + tenant + '/member-invitations', { target_account_id: users[label].id, client_request_id: randomUUID(), reason: '跨空间合成验收' });
          const accepted = await user.call('/me/tenant-invitations/' + invitation.invitation.id + '/accept', { expected_version: invitation.invitation.version });
          if (label === 'U3' && tenant === session.tenants[0]) await ta.call('/tenants/' + tenant + '/projects/' + project.project_id + '/members', { client_request_id: randomUUID(), reason: '跨项目 MFA 验收', expected_project_version: project.version, tenant_membership_id: accepted.membership_id });
        }
        if (label === 'U2') {
          const setup = await user.call('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
          await user.call('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
        }
        if (label === 'U3') {
          const setup = await user.call('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
          await user.call('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
          for (const base of [session.api_a, session.api_b]) {
            for (const path of ['/auth/me', ...session.tenants.map(id => `/tenants/${id}/context`), `/tenants/${session.tenants[0]}/projects/${project.project_id}/context`]) {
              const result = await user.http.get(base + '/api/v1' + path);
              if (result.status() !== 200) throw Error('MFA cross-workspace or cross-instance check failed: ' + result.status());
            }
            if ((await user.http.get(base + '/api/v1/platform/security-policy')).status() !== 403) throw Error('MFA expanded business permissions');
          }
          session.checks = { same_factor_across_two_tenants_and_project: true, two_api_instances_accept_same_mfa_session: true, mfa_does_not_grant_platform_permissions: true };
          // Return the manual fixture to its documented optional starting state.
          await user.call('/account/mfa/disable', { expected_factor_version: '2' });
          for (let attempt = 0; attempt < 40; attempt++) {
            const inbox = await user.call('/me/notifications');
            const enabled = inbox.items.find(item => item.outcome === 'mfa_enabled');
            const disabled = inbox.items.find(item => item.outcome === 'mfa_disabled');
            if (enabled?.body === '已启用多因素认证。请妥善保存恢复码。' && disabled?.body === '已关闭个人多因素认证。') {
              session.checks.mfa_success_notifications_delivered = true;
              break;
            }
            if (attempt === 39) throw Error('MFA success notifications were not delivered');
            await new Promise(resolve => setTimeout(resolve, 250));
          }
        }
        await user.call('/auth/logout', {});
      } finally { await user.close(); }
    }
    displayBrowser = await prepareMFALabDisplay({ administrator: p1, tenantAdmin: ta, session });
  } finally { await p1.close(); await ta.close(); }
}
try {
  ensureTestDependencies(); save();
  execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', 's3', 'audit-volume'], { stdio: 'inherit' });
  execFileSync('pnpm', ['display:pack'], { stdio: 'inherit' });
  const binary = join(privateDirectory, 'api-test');
  execFileSync('go', ['test', '-c', '-o', binary, './services/api/internal/transport/http'], { stdio: 'inherit' });
  start(binary, ['-test.run=^TestMFAAcceptanceServer$', '-test.timeout=0'], { cwd: resolve('services/api/internal/transport/http'), env: { ...process.env, ...testEnv, INFRAPRO_MFA_LAB: '1', INFRAPRO_MFA_LAB_PRIVATE: privateDirectory, INFRAPRO_MFA_API_A: `127.0.0.1:${a}`, INFRAPRO_MFA_API_B: `127.0.0.1:${b}`, INFRAPRO_E2E_CONSOLE_URL: origin, INFRAPRO_DISPLAY_E2E: '1', INFRAPRO_E2E_RUNTIME_ADDR: `127.0.0.1:${runtime}`, INFRAPRO_E2E_ASSET_ADDR: `127.0.0.1:${assets}`, INFRAPRO_E2E_RUNTIME_ORIGIN: session.runtime_origin, INFRAPRO_E2E_ASSET_ORIGIN: session.asset_origin } });
  start(process.execPath, [resolve('apps/console/node_modules/vite/bin/vite.js'), '--host', '127.0.0.1'], { cwd: resolve('apps/console'), env: { ...process.env, API_PROXY_TARGET: session.api_a, CONSOLE_PORT: String(ui), VITE_API_BASE_URL: '/api/v1', VITE_APP_BASE: '/' } });
  await waitURL(session.api_a + '/readyz'); await waitURL(session.api_b + '/readyz'); await waitURL(origin + '/api/v1/public/account-help');
  await seed(); await verifyMFALabUI(session, directory); session.state = 'running'; session.pids = children.map(c => c.pid); save();
  console.log('MFA 独立验收入口：' + origin); console.log('安全环境索引：' + join(directory, 'session.json'));
  if (!process.argv.includes('--check')) await new Promise(resolve => { process.once('SIGINT', resolve); process.once('SIGTERM', resolve); });
} finally {
  await displayBrowser?.close();
  for (const child of children.reverse()) { if (child.exitCode !== null) continue; const closed = new Promise(r => child.once('exit', r)); child.kill('SIGTERM'); const timeout = setTimeout(() => child.kill('SIGKILL'), 8000); await closed; clearTimeout(timeout); }
  rmSync(privateDirectory, { recursive: true, force: true }); session.state = 'stopped'; save();
}

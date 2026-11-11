import { execFileSync } from 'node:child_process';
import { randomUUID, randomBytes } from 'node:crypto';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { test, expect } from '@playwright/test';
import { roleAction } from '../helpers/roles';

const origin = `http://${process.env.INFRAPRO_HTTP_TEST_HOST}:18088`;
const base = `${origin}/console`;
const password = 'Real browser 河流 secret 592';
const args = ['compose', '-f', 'deploy/compose/http-test.yaml'];
const compose = (...extra: string[]) => execFileSync('docker', [...args, ...extra], { encoding: 'utf8', stdio: 'pipe' });
test.use({ trace: 'off' });

test('tenant invitation through /console HTTP and restored credentials are permanently revoked', async ({ page, request, browser }) => {
  const csrf = (await (await request.get(`${origin}/api/v1/auth/csrf`)).json()).token as string;
  const headers = { Origin: origin, 'X-CSRF-Token': csrf };
  expect((await request.post(`${origin}/api/v1/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const owner = (await (await request.get(`${origin}/api/v1/auth/me`)).json()).account.id as string;
  const createAccount = async () => {
    const link = await (await request.post(`${origin}/api/v1/invitations`, { headers, data: { note: 'HTTP membership' } })).json();
    const username = `http_${randomBytes(4).toString('hex')}`;
    const signup = await browser.newContext();
    try {
      const h = { Origin: origin, 'X-CSRF-Token': (await (await signup.request.get(`${origin}/api/v1/auth/csrf`)).json()).token as string };
      expect((await signup.request.post(`${origin}/api/v1/public/invitations/accept`, { headers: h, data: { token: new URLSearchParams(new URL(link.url).hash.slice(1)).get('token'), username, display_name: 'HTTP 成员', password } })).status()).toBe(204);
    } finally { await signup.close(); }
    const target = (await (await request.get(`${origin}/api/v1/access/accounts?q=${username}`)).json()).items[0].id as string; return { username, id: target };
  };
  const member = await createAccount(); const pending = await createAccount();
  const r = await request.post(`${origin}/api/v1/platform/tenants`, { headers, data: { name: 'HTTP 成员验收', code: `http-member-${randomBytes(3).toString('hex')}`, administrator_account_id: owner, client_request_id: randomUUID(), reason: 'HTTP membership' } }); expect(r.status()).toBe(201); const tenant = (await r.json()).tenant_id as string;
  const issue = async (target: string) => { const r = await request.post(`${origin}/api/v1/tenants/${tenant}/member-invitations`, { headers, data: { target_account_id: target, reason: 'HTTP membership', client_request_id: randomUUID() } }); expect(r.status()).toBe(201); return await r.json() as { link: string; invitation: { id: string } }; };
  const link = await issue(member.id); expect(link.link.startsWith(`${base}/tenant-invite#token=`)).toBe(true);
  await page.goto(link.link); await expect(page).not.toHaveURL(/token=/); await page.getByRole('link', { name: '登录并继续' }).click(); await page.getByLabel('用户名', { exact: true }).fill(member.username); await page.getByLabel('密码', { exact: true }).fill(password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: '确认加入租户' })).toBeVisible(); await page.getByRole('button', { name: '确认加入', exact: true }).click(); await page.getByRole('link', { name: '进入租户', exact: true }).click(); await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible(); expect(new URL(page.url()).pathname).toBe(`/console/t/${tenant}/profile`);
  const active = (await (await page.request.get(`${origin}/api/v1/tenants/${tenant}/context`)).json()).membership_id as string;
  const roleResponse = await request.post(`${origin}/api/v1/tenants/${tenant}/roles`, { headers, data: { name: 'HTTP 资料职责', permission_codes: [P.TenantProfileRead, P.TenantProfileUpdate], client_request_id: randomUUID(), reason: 'HTTP role acceptance' } }); expect(roleResponse.status()).toBe(201); const role = (await roleResponse.json()).role;
  const auth = await (await request.get(`${origin}/api/v1/tenants/${tenant}/members/${active}/authorization`)).json();
  const change = { expected_membership_version: auth.membership_version, role_ids: [role.id] };
  const impact = await (await request.post(`${origin}/api/v1/tenants/${tenant}/members/${active}/roles/preview`, { headers, data: change })).json();
  expect((await request.put(`${origin}/api/v1/tenants/${tenant}/members/${active}/roles`, { headers, data: { ...change, role_versions: impact.role_versions, preview_hash: impact.preview_hash, reason: 'HTTP role grant' } })).status()).toBe(200);
  await page.reload(); await expect(page.getByRole('button', { name: '编辑资料', exact: true })).toBeVisible();
  const operator = await browser.newContext(); const operatorPage = await operator.newPage();
  try {
    await operatorPage.goto(`${base}/t/${tenant}/roles/${role.id}`); await operatorPage.getByLabel('用户名', { exact: true }).fill('e2e_owner'); await operatorPage.getByLabel('密码', { exact: true }).fill(password); await operatorPage.getByRole('button', { name: '登录', exact: true }).click();
    await expect(operatorPage.getByRole('heading', { name: 'HTTP 资料职责', exact: true })).toBeVisible(); expect(new URL(operatorPage.url()).pathname).toBe(`/console/t/${tenant}/roles/${role.id}`);
    await roleAction(operatorPage, '停用角色'); await operatorPage.getByRole('button', { name: '预览变更影响' }).click(); await expect(operatorPage.getByRole('region', { name: '变更影响预览' })).toBeVisible(); await operatorPage.getByRole('button', { name: '取消', exact: true }).click();
  } finally { await operator.close(); }
  const dry = JSON.parse(compose('run', '--rm', '--no-deps', '-T', 'ops', 'access', 'sync', '--dry-run')); expect(dry.changed).toBe(false);
  const old = await issue(pending.id);
  // A real backup is restored into this dedicated disposable stack, with API stopped.
  compose('stop', 'api');
  const dump = execFileSync('docker', [...args, 'exec', '-T', 'postgres', 'pg_dump', '-U', 'infrapro_test', '-d', 'infrapro_test', '-Fc'], { stdio: ['ignore', 'pipe', 'pipe'] });
  execFileSync('docker', [...args, 'exec', '-T', 'postgres', 'pg_restore', '-U', 'infrapro_test', '-d', 'infrapro_test', '--clean', '--if-exists', '--exit-on-error'], { input: dump, stdio: ['pipe', 'pipe', 'pipe'] });
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'revoke-access', '--reason', 'membership restore drill');
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'access', 'check'); compose('start', 'api');
  await expect.poll(async () => (await request.get(`${origin}/readyz`)).status()).toBe(200);
  expect((await page.request.get(`${origin}/api/v1/auth/me`)).status()).toBe(401); expect((await request.get(`${origin}/api/v1/auth/me`)).status()).toBe(401);
  const recipient = await browser.newContext(); const targetPage = await recipient.newPage();
  try {
    await targetPage.goto(old.link); await targetPage.getByRole('link', { name: '登录并继续' }).click(); await targetPage.getByLabel('用户名', { exact: true }).fill(pending.username); await targetPage.getByLabel('密码', { exact: true }).fill(password); await targetPage.getByRole('button', { name: '登录', exact: true }).click(); await expect(targetPage.getByText('资格已失效', { exact: true })).toBeVisible();
    const rh = { Origin: origin, 'X-CSRF-Token': (await (await page.request.get(`${origin}/api/v1/auth/csrf`)).json()).token as string };
    expect((await page.request.post(`${origin}/api/v1/auth/login`, { headers: rh, data: { username: member.username, password } })).status()).toBe(200); const restored = await (await page.request.get(`${origin}/api/v1/tenants/${tenant}/context`)).json(); expect(restored.membership_id).toBe(active); expect(restored.authorization.roles.map((r: { id: string }) => r.id)).toEqual([role.id]); expect(restored.permissions).toContain(P.TenantProfileUpdate);
    const logs = compose('logs', '--no-color', 'api', 'web'); expect(logs.includes(new URLSearchParams(new URL(old.link).hash.slice(1)).get('token')!)).toBe(false); expect(logs.includes(new URLSearchParams(new URL(link.link).hash.slice(1)).get('token')!)).toBe(false);
  } finally { await recipient.close(); }
});

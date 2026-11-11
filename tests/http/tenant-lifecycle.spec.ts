import { randomBytes, randomUUID } from 'node:crypto';
import { test, expect } from '@playwright/test';

const origin = `http://${process.env.INFRAPRO_HTTP_TEST_HOST}:18088`; const base = `${origin}/console`;
const password = 'Real browser 河流 secret 592';
test.use({ trace: 'off' });
test('tenant lifecycle through ordinary HTTP /console with strict Origin and public restriction', async ({ page, request }) => {
  const headers = { Origin: origin, 'X-CSRF-Token': (await (await request.get(`${origin}/api/v1/auth/csrf`)).json()).token as string };
  expect((await request.post(`${origin}/api/v1/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const account = (await (await request.get(`${origin}/api/v1/auth/me`)).json()).account.id as string;
  const created = await request.post(`${origin}/api/v1/platform/tenants`, { headers, data: { name: 'HTTP 启停', code: `http-tl-${randomBytes(4).toString('hex')}`, administrator_account_id: account, client_request_id: randomUUID(), reason: '部署验证' } }); expect(created.status()).toBe(201); const tenant = (await created.json()).tenant_id as string;
  await page.goto(`${base}/platform/tenants/${tenant}`); await page.getByLabel('用户名', { exact: true }).fill('e2e_owner'); await page.getByLabel('密码', { exact: true }).fill(password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'HTTP 启停', exact: true })).toBeVisible(); expect(new URL(page.url()).pathname).toBe(`/console/platform/tenants/${tenant}`);
  const endpoint = `${origin}/api/v1/platform/tenants/${tenant}/disable/preview`;
  expect((await request.post(endpoint, { headers: { ...headers, Origin: 'http://untrusted.invalid' }, data: {} })).status()).toBe(403);
  expect((await request.post(endpoint, { headers: { Origin: origin }, data: {} })).status()).toBe(403);
  await page.getByRole('button', { name: '停用租户', exact: true }).click(); await page.getByLabel('平台内部操作原因').fill('HTTP 私有诊断'); await page.getByLabel('租户可见说明').fill('HTTP 维护说明'); await page.getByRole('button', { name: '确认停用租户', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0);
  const rejected = await request.get(`${origin}/api/v1/tenants/${tenant}/profile`); expect(rejected.status()).toBe(403); expect(rejected.headers()['content-type']).toContain('application/json'); expect((await rejected.json()).code).toBe('tenant_disabled');
  await page.goto(`${base}/t/${tenant}/profile`); await expect(page.getByRole('heading', { name: '租户已停用', exact: true })).toBeVisible(); await expect(page.getByText('HTTP 维护说明', { exact: true })).toBeVisible(); await expect(page.getByText('HTTP 私有诊断', { exact: true })).toHaveCount(0);
  await page.goto(`${base}/platform/tenants/${tenant}`); await page.getByRole('button', { name: '恢复租户', exact: true }).click(); await page.getByLabel('平台内部操作原因').fill('HTTP 恢复'); await page.getByRole('button', { name: '确认恢复租户', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); await page.goto(`${base}/t/${tenant}/profile`); await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
});

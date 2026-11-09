import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { randomBytes } from 'node:crypto';
export const consoleURL = process.env.INFRAPRO_E2E_CONSOLE_URL || 'http://127.0.0.1:15173';
export const password = 'Real browser 河流 secret 592';
export async function csrf(request: APIRequestContext, baseURL = consoleURL) {
  const response = await request.get(`${baseURL.replace(/\/$/, '')}/api/v1/auth/csrf`);
  expect(response.status()).toBe(200); const data = await response.json(); return data.token as string;
}
const apiRoot = () => new URL('/api/v1', consoleURL).href;
export async function createAccount(request: APIRequestContext, withInvitationRole = true) {
  const token = await csrf(request, new URL(consoleURL).origin);
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': token };
  let response = await request.post(`${apiRoot()}/auth/login`, { headers, data: { username: 'e2e_owner', password } }); expect(response.status()).toBe(200);
  response = await request.post(`${apiRoot()}/invitations`, { headers, data: { note: '隔离浏览器验收账号' } }); expect(response.status()).toBe(200);
  const link = await response.json(); const credential = new URLSearchParams(new URL(link.url).hash.slice(1)).get('token');
  response = await request.post(`${apiRoot()}/auth/logout`, { headers }); expect(response.status()).toBe(204);
  const username = `user_${randomBytes(6).toString('hex')}`;
  response = await request.post(`${apiRoot()}/public/invitations/accept`, { headers, data: { token: credential, username, display_name: '浏览器验收', password } }); expect(response.status()).toBe(204);
  if (withInvitationRole) {
    // Authentication regressions opt into invitation capability through real role APIs.
    response = await request.post(`${apiRoot()}/auth/login`, { headers, data: { username: 'e2e_owner', password } }); expect(response.status()).toBe(200);
    response = await request.get(`${apiRoot()}/access/accounts?q=${username}`); expect(response.status()).toBe(200);
    const target = (await response.json()).items[0] as { id: string; management_version: string; roles: { id: string }[] };
    response = await request.post(`${apiRoot()}/access/roles`, { headers, data: { name: `邀请回归_${username}`, description: '明确授权的邀请回归测试账号', reason: '测试准备' } }); expect(response.status()).toBe(200);
    const role = await response.json() as { id: string; version: string };
    response = await request.put(`${apiRoot()}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: [P.PlatformInvitationRead, P.PlatformInvitationCreate, P.PlatformInvitationRevoke, P.PlatformInvitationRegenerate], reason: '测试准备' } }); expect(response.status()).toBe(200);
    response = await request.put(`${apiRoot()}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [...target.roles.map((r) => r.id), role.id], reason: '测试准备' } }); expect(response.status()).toBe(200);
    response = await request.post(`${apiRoot()}/auth/logout`, { headers }); expect(response.status()).toBe(204);
  }
  return { username, password };
}
export async function login(page: Page, account: { username: string; password: string }, url = consoleURL) {
  await page.goto(`${url.replace(/\/$/, '')}/login`);
  await page.getByRole('textbox', { name: '用户名', exact: true }).fill(account.username);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
}
export const test = base.extend<{ account: { username: string; password: string } }>({
  account: async ({ request }, provide) => { await provide(await createAccount(request)); },
});
export { expect };

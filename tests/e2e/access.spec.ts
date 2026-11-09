import { test, expect, login, createAccount, consoleURL, password, csrf } from './fixtures';
import { navigateTo } from '../helpers/navigation';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import type { APIRequestContext, Page } from '@playwright/test';
import { randomBytes } from 'node:crypto';
const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  return headers;
}
async function refresh(page: Page) {
  const response = page.waitForResponse((r) => r.url().endsWith('/auth/me'));
  await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await response;
}
async function submitReason(page: Page) { const dialog = page.getByRole('dialog'); await dialog.getByLabel('变更原因').fill('浏览器授权闭环验证'); await dialog.getByRole('button', { name: '确认提交变更' }).click(); await expect(dialog).toBeHidden(); }

test('role UI grants and withdraws invitation capability without logging in again', async ({ page, request, browser }, testInfo) => {
  const member = await createAccount(request, false);
  const guestContext = await browser.newContext();
  try {
    const guest = await guestContext.newPage(); await login(guest, member);
    await guest.goto(`${consoleURL}/access/accounts`); await expect(guest.getByRole('heading', { name: '403 · 无权限' })).toBeVisible();
    await login(page, { username: 'e2e_owner', password }); await navigateTo(page, '角色管理');
    const name = `邀请办理_${randomBytes(4).toString('hex')}`;
    await page.getByRole('button', { name: '创建角色', exact: true }).click();
    await page.getByRole('dialog').getByLabel('角色名称').fill(name); await page.getByRole('dialog').getByLabel('角色说明').fill('仅允许办理基础成员邀请'); await submitReason(page);
    await page.getByLabel('搜索角色名称').fill(name); await page.getByRole('button', { name, exact: true }).click();
    await page.getByRole('checkbox', { name: /创建基础成员邀请/ }).focus(); await page.keyboard.press('Space'); await expect(page.getByRole('checkbox', { name: /创建基础成员邀请/ })).toBeChecked(); await page.getByRole('checkbox', { name: /查看本人邀请/ }).check();
    await expect(page.getByRole('group', { name: '邀请', exact: true }).getByRole('checkbox', { name: '全选此分组当前筛选结果（4 项可授权）' })).toHaveAttribute('aria-checked', 'mixed');
    await page.getByRole('button', { name: '保存权限配置' }).click(); await submitReason(page);
    await navigateTo(page, '账号管理'); await page.getByLabel('搜索用户名或显示名称').fill(member.username); await page.getByRole('button', { name: '浏览器验收', exact: true }).click();
    await page.getByRole('checkbox', { name: new RegExp(name) }).check(); await page.getByRole('button', { name: '保存角色绑定' }).click(); await submitReason(page);
    await refresh(guest); await guest.goto(`${consoleURL}/invitations`); await guest.getByRole('button', { name: '生成邀请链接', exact: true }).click(); await expect(guest.getByLabel('可手动复制的完整链接')).toBeVisible();
    const link = await guest.getByLabel('可手动复制的完整链接').inputValue();
    await navigateTo(page, '角色管理'); await page.getByLabel('搜索角色名称').fill(name); await page.getByRole('button', { name, exact: true }).click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath('role-permissions.png'), fullPage: true });
    await page.getByRole('button', { name: '停用角色', exact: true }).click(); await submitReason(page);
    // Existing cookies fail immediately, independently of the UI refresh interval.
    expect((await guestContext.request.get(`${api}/invitations`)).status()).toBe(403);
    await refresh(guest); await expect(guest.getByRole('heading', { name: '403 · 无权限' })).toBeVisible(); await expect(guest.getByLabel('可手动复制的完整链接')).toHaveCount(0);
    await page.getByRole('button', { name: '启用角色', exact: true }).click(); await submitReason(page); await refresh(guest); await expect(guest.getByRole('button', { name: '生成邀请链接', exact: true })).toBeVisible();
    await expect(guest.getByText('已撤销', { exact: true })).toBeVisible();
    const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
    const old = await request.post(`${api}/public/invitations/inspect`, { headers, data: { token: new URLSearchParams(new URL(link).hash.slice(1)).get('token') } }); expect(old.status()).toBe(404);
    await navigateTo(page, '权限目录'); await expect(page.getByRole('heading', { name: '权限目录', exact: true })).toBeVisible(); await expect(page.getByRole('button', { name: '新增权限' })).toHaveCount(0);
    await navigateTo(page, '平台审计'); await expect(page.getByText('浏览器授权闭环验证').first()).toBeVisible();
  } finally { await guestContext.close(); }
});

test('member API denies enumeration and strict writes reject direct grants', async ({ request, browser }) => {
  const member = await createAccount(request, false); const headers = await owner(request);
  let response = await request.get(`${api}/access/accounts?q=${member.username}`); const target = (await response.json()).items[0];
  response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [], permissions: [P.PlatformAuditRead], reason: '非法直接授权' } }); expect(response.status()).toBe(400);
  response = await request.post(`${api}/access/roles`, { headers, data: { name: '非法创建', reason: '非法权限', description: '', permission_codes: [P.PlatformAuditRead] } }); expect(response.status()).toBe(400);
  response = await request.post(`${api}/invitations`, { headers, data: { note: '', role_ids: ['anything'] } }); expect(response.status()).toBe(400);
  const l = await request.post(`${api}/invitations`, { headers, data: { note: '额外字段原子性' } }); const link = await l.json();
  const guest = await browser.newContext();
  try {
    const gh = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(guest.request) };
    const token = new URLSearchParams(new URL(link.url).hash.slice(1)).get('token');
    response = await guest.request.post(`${api}/public/invitations/accept`, { headers: gh, data: { token, username: 'bad_extra', password, is_admin: true } }); expect(response.status()).toBe(400);
    response = await guest.request.post(`${api}/public/invitations/inspect`, { headers: gh, data: { token } }); expect(response.status()).toBe(200);
    expect((await guest.request.post(`${api}/auth/login`, { headers: gh, data: member })).status()).toBe(200);
    for (const path of ['/access/accounts', `/access/accounts/${target.id}`, '/access/accounts/00000000-0000-0000-0000-000000000000', '/access/roles', '/invitations']) {
      response = await guest.request.get(`${api}${path}`); expect(response.status()).toBe(403); expect((await response.json()).code).toBe('permission_denied');
    }
    response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [], reason: '清空权限' } }); expect(response.status()).toBe(200);
    response = await guest.request.get(`${api}/auth/me`); expect(response.status()).toBe(200); expect((await response.json()).permissions).toEqual([]);
    response = await guest.request.get(`${api}/account/profile`); expect(response.status()).toBe(403);
    expect((await guest.request.post(`${api}/auth/logout`, { headers: gh })).status()).toBe(204);
  } finally { await guest.close(); }
});

test('authorization refresh preserves unrelated drafts and rejects late lost-page results', async ({ page, context, request }) => {
  const account = await createAccount(request); await login(page, account); const headers = await owner(request);
  const target = (await (await request.get(`${api}/access/accounts?q=${account.username}`)).json()).items[0];
  await navigateTo(page, '个人资料'); await page.getByLabel('显示名称', { exact: true }).fill('保留当前未提交资料');
  let response = await request.post(`${api}/access/roles`, { headers, data: { name: `无关变更_${randomBytes(4).toString('hex')}`, description: '', reason: '无关修订' } }); expect(response.status()).toBe(200);
  await refresh(page); await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('保留当前未提交资料');
  const tab = await context.newPage(); await tab.goto(`${consoleURL}/invitations`);
  let release = () => {}; let fetched = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; }); const ready = new Promise<void>((resolve) => { fetched = resolve; });
  await tab.route('**/api/v1/invitations', async (route) => { if (route.request().method() !== 'POST') { await route.continue(); return; } const response = await route.fetch(); fetched(); await held; await route.fulfill({ response }); });
  try {
    await tab.getByRole('button', { name: '生成邀请链接', exact: true }).click(); await ready;
    response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [], reason: '撤权验证' } }); expect(response.status()).toBe(200);
    await refresh(tab); await expect(tab.getByRole('heading', { name: '403 · 无权限' })).toBeVisible(); release();
    await refresh(page); await expect(page.getByRole('heading', { name: '403 · 无权限' })).toBeVisible();
    await tab.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expect(tab.getByLabel('可手动复制的完整链接')).toHaveCount(0); await expect(tab.getByLabel('用途备注')).toHaveCount(0);
    await page.goto(consoleURL); await expect(page.getByRole('heading', { name: '尚未分配可用权限' })).toBeVisible();
  } finally { release(); await tab.unrouteAll({ behavior: 'wait' }); }
});

import { test, expect, login, createAccount, consoleURL, password, csrf } from './fixtures';
import { navigateTo } from '../helpers/navigation';
import { closeRolePanel, roleAction, submitPlatformRole } from '../helpers/roles';
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
async function submitReason(page: Page, reason: string) { const dialog = page.getByRole('dialog'); await dialog.getByLabel('变更原因').fill(reason); await dialog.getByRole('button', { name: '保存角色绑定' }).click(); await expect(dialog.getByLabel('变更原因')).toHaveCount(0); await closeRolePanel(page); }

test('role UI grants and withdraws invitation capability without logging in again', async ({ page, request, browser }, testInfo) => {
  const member = await createAccount(request, false);
  const guestContext = await browser.newContext();
  try {
    const guest = await guestContext.newPage(); await login(guest, member);
    await guest.goto(`${consoleURL}/access/accounts`); await expect(guest.getByRole('heading', { name: '403 · 无权限' })).toBeVisible();
    await login(page, { username: 'e2e_owner', password }); await navigateTo(page, '角色管理');
    await expect(page.getByRole('link', { name: '权限目录', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: /搜索页面/ }).click();
    await page.getByPlaceholder('输入页面名称…').fill('权限目录');
    await expect(page.getByText('没有找到相关页面。', { exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    const name = `邀请办理_${randomBytes(4).toString('hex')}`;
    const reason = `浏览器授权闭环验证_${name}`;
    await page.getByRole('button', { name: '创建角色', exact: true }).click();
    await page.getByRole('dialog').getByLabel('角色名称').fill(name); await page.getByRole('dialog').getByLabel('角色说明').fill('仅允许办理基础成员邀请'); await submitPlatformRole(page, reason, '创建角色');
    await page.getByLabel('搜索角色名称').fill(name); await page.getByRole('button', { name, exact: true }).click();
    await page.getByRole('button', { name: '配置角色权限', exact: true }).click();
    const permissionEditor = page.getByRole('region', { name: '角色详情', exact: true });
    await permissionEditor.locator('summary').filter({ hasText: '权限说明' }).click();
    await expect(permissionEditor.getByText('角色将一组操作权限授予绑定的账号。', { exact: false })).toBeVisible();
    await expect(permissionEditor.getByRole('checkbox', { name: /查看项目资料/ })).toHaveCount(0);
    await expect(permissionEditor.locator('code').filter({ hasText: P.PlatformInvitationCreate })).toHaveCount(0);
    await permissionEditor.getByRole('switch', { name: '显示权限编码', exact: true }).check();
    await expect(permissionEditor.getByText(P.PlatformInvitationCreate, { exact: true })).toBeVisible();
    await permissionEditor.getByRole('switch', { name: '显示权限编码', exact: true }).uncheck();
    await page.getByLabel('搜索本角色可配置的权限').fill(P.PlatformInvitationCreate);
    await expect(page.getByRole('checkbox', { name: /创建基础成员邀请/ })).toBeVisible();
    await page.getByLabel('搜索本角色可配置的权限').fill('');
    await page.getByRole('checkbox', { name: /创建基础成员邀请/ }).focus(); await page.keyboard.press('Space'); await expect(page.getByRole('checkbox', { name: /创建基础成员邀请/ })).toBeChecked(); await page.getByRole('checkbox', { name: /查看本人邀请/ }).check();
    await expect(page.getByRole('group', { name: '邀请', exact: true }).getByRole('checkbox', { name: '全选此分组当前筛选结果（4 项可授权）' })).toHaveAttribute('aria-checked', 'mixed');
    await page.getByRole('button', { name: /仅看已选/ }).click();
    await expect(page.getByRole('checkbox', { name: /撤销本人邀请/ })).toHaveCount(0);
    await submitPlatformRole(page, reason, '保存权限配置');
    const invitationGroup = page.getByRole('dialog').getByRole('region', { name: '邀请权限', exact: true });
    await expect(invitationGroup.getByRole('listitem')).toHaveCount(2);
    await expect(invitationGroup).toContainText('创建基础成员邀请');
    await closeRolePanel(page);
    await navigateTo(page, '账号管理'); await page.getByLabel('搜索用户名或显示名称').fill(member.username); await page.getByRole('button', { name: '浏览器验收', exact: true }).click();
    await page.getByRole('button', { name: '配置角色', exact: true }).click(); await page.getByRole('checkbox', { name: new RegExp(name) }).check(); await submitReason(page, reason);
    await refresh(guest); await guest.goto(`${consoleURL}/invitations`); await guest.getByRole('button', { name: '创建邀请', exact: true }).click(); await guest.getByRole('button', { name: '生成邀请链接', exact: true }).click(); await expect(guest.getByLabel('可手动复制的完整链接')).toBeVisible();
    const link = await guest.getByLabel('可手动复制的完整链接').inputValue();
    await navigateTo(page, '角色管理'); await page.getByLabel('搜索角色名称').fill(name); await page.getByRole('button', { name, exact: true }).click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath('role-permissions.png') });
    await roleAction(page, '停用角色'); await submitPlatformRole(page, reason, '确认停用角色');
    // Existing cookies fail immediately, independently of the UI refresh interval.
    expect((await guestContext.request.get(`${api}/invitations`)).status()).toBe(403);
    await refresh(guest); await expect(guest.getByRole('heading', { name: '403 · 无权限' })).toBeVisible(); await expect(guest.getByLabel('可手动复制的完整链接')).toHaveCount(0);
    await roleAction(page, '启用角色'); await submitPlatformRole(page, reason, '确认启用角色'); await refresh(guest); await expect(guest.getByRole('button', { name: '创建邀请', exact: true })).toBeVisible();
    await expect(guest.getByRole('table', { name: '我的邀请', exact: true }).getByText('已撤销', { exact: true })).toBeVisible();
    const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
    const old = await request.post(`${api}/public/invitations/inspect`, { headers, data: { token: new URLSearchParams(new URL(link).hash.slice(1)).get('token') } }); expect(old.status()).toBe(404);
    await page.goto(`${consoleURL}/access/permissions`); await expect(page).toHaveURL(`${consoleURL}/access/roles`); await expect(page.getByRole('heading', { name: '角色管理', exact: true })).toBeVisible();
    await navigateTo(page, '平台审计'); await page.getByLabel('搜索名称或原因').fill(reason); await page.getByRole('table', { name: '平台审计记录', exact: true }).getByRole('row').nth(1).getByRole('button').first().click(); await expect(page.getByRole('dialog').getByText(reason, { exact: true })).toBeVisible();
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
    for (const path of ['/access/accounts', `/access/accounts/${target.id}`, '/access/accounts/00000000-0000-0000-0000-000000000000', '/access/roles', '/access/permissions', '/invitations']) {
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
    await tab.getByRole('button', { name: '创建邀请', exact: true }).click(); await tab.getByRole('button', { name: '生成邀请链接', exact: true }).click(); await ready;
    response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [], reason: '撤权验证' } }); expect(response.status()).toBe(200);
    await refresh(tab); await expect(tab.getByRole('heading', { name: '403 · 无权限' })).toBeVisible(); release();
    await refresh(page); await expect(page.getByRole('heading', { name: '403 · 无权限' })).toBeVisible();
    await tab.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expect(tab.getByLabel('可手动复制的完整链接')).toHaveCount(0); await expect(tab.getByLabel('用途备注')).toHaveCount(0);
    await page.goto(consoleURL); await expect(page.getByRole('heading', { name: '尚未分配可用权限' })).toBeVisible();
  } finally { release(); await tab.unrouteAll({ behavior: 'wait' }); }
});

test('platform role tables paginate, filter and separate preset scopes', async ({ page, request }, info) => {
  const headers = await owner(request); const prefix = `界面分页_${randomBytes(4).toString('hex')}`;
  for (let i = 0; i < 12; i++) {
    const created = await request.post(`${api}/access/roles`, { headers, data: { name: `${prefix}_${i + 1}`, description: '角色列表交互验收', reason: '准备分页验证' } });
    expect(created.status()).toBe(200);
    if (i === 0) { const role = await created.json(); expect((await request.put(`${api}/access/roles/${role.id}/status`, { headers, data: { expected_version: role.version, status: 'disabled', reason: '准备状态筛选' } })).status()).toBe(200); }
  }
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/access/roles`);
  await page.getByLabel('搜索角色名称', { exact: true }).fill(prefix);
  const table = page.getByRole('table', { name: '角色列表', exact: true }); const pager = page.getByRole('navigation', { name: '角色列表分页', exact: true });
  await expect(table.getByRole('row')).toHaveCount(11);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('platform-role-table-light.png'), fullPage: true });
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click(); await page.screenshot({ path: info.outputPath('platform-role-table-dark.png'), fullPage: true }); await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '浅色', exact: true }).click();
  await pager.getByRole('button', { name: '下一页', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(3); await expect(pager.getByRole('status')).toContainText('第 2 页');
  const roleButton = table.getByRole('row').nth(1).getByRole('button').first(); await roleButton.click();
  const drawer = page.locator('[data-slot="drawer-popup"]');
  await expect(drawer).toBeVisible(); await expect(drawer).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await expect(page.locator('[data-slot="drawer-overlay"]')).toHaveCSS('backdrop-filter', /blur\([1-9][\d.]*px\)/);
  if (info.project.name === 'desktop') { const bounds = await drawer.boundingBox(); expect(bounds!.y).toBe(12); expect(bounds!.x + bounds!.width).toBe(page.viewportSize()!.width - 12); await expect(drawer).toHaveCSS('border-top-right-radius', '16px'); }
  await closeRolePanel(page); await expect(roleButton).toBeFocused(); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(page.getByLabel('搜索角色名称', { exact: true })).toHaveValue(prefix);
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(13); await expect(pager.getByRole('status')).toContainText('第 1 页'); await expect(pager.getByRole('button', { name: '下一页', exact: true })).toBeDisabled();
  const rowMenu = table.getByRole('row').nth(1).getByRole('button', { name: /的操作$/ }); await rowMenu.click(); await page.getByRole('menuitem', { name: '查看详情', exact: true }).click(); await closeRolePanel(page); await expect(rowMenu).toBeFocused();
  if (info.project.name === 'desktop') { await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '绑定人数', exact: true }).click(); await expect(table.getByRole('columnheader', { name: '绑定人数', exact: true })).toHaveCount(0); }
  await page.getByRole('button', { name: '角色状态', exact: true }).click(); await page.getByRole('menuitemradio', { name: '停用', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(2); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByLabel('搜索角色名称', { exact: true }).fill('平台管理员'); await page.getByRole('button', { name: '平台管理员', exact: true }).click();
  for (const name of ['个人账号', '邀请', '账号管理', '角色管理', '平台审计', '租户治理']) await expect(page.getByRole('dialog').getByRole('heading', { name, exact: true })).toBeAttached();
  const platformAdmin = (await (await request.get(`${api}/access/roles?q=${encodeURIComponent('平台管理员')}`)).json()).items[0];
  await expect(page.getByRole('dialog').getByRole('listitem')).toHaveCount(platformAdmin.permission_codes.length);
  await page.screenshot({ path: info.outputPath('platform-role-drawer-grouped.png') }); await page.keyboard.press('Escape'); await expect(drawer).toHaveCount(0);
  await page.getByRole('tab', { name: '预置租户角色', exact: true }).click(); await expect(page.getByRole('table', { name: '预置租户角色', exact: true })).toBeVisible(); await expect(table).toHaveCount(0);
  await page.getByRole('button', { name: '租户管理员', exact: true }).click(); await expect(page.getByRole('dialog').getByRole('button', { name: '配置角色权限', exact: true })).toHaveCount(0); await expect(page.getByRole('dialog').getByRole('heading', { name: '管理员保留能力', exact: true })).toBeAttached(); await expect(page.getByRole('dialog').getByRole('region', { name: '资料权限', exact: true }).getByRole('listitem')).toHaveCount(2); await closeRolePanel(page);
  await page.getByRole('tab', { name: '预置项目角色', exact: true }).click(); await expect(page.getByRole('table', { name: '预置项目角色', exact: true })).toBeVisible(); await page.getByRole('button', { name: '项目管理员', exact: true }).click(); await expect(page.getByRole('dialog').getByText('当前项目', { exact: true }).first()).toBeVisible();
  await page.screenshot({ path: info.outputPath('project-preset-panel.png') });
});

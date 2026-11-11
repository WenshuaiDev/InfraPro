import { randomBytes } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import type { ManagedAccount, PermissionCode, Role } from '../../packages/ts/api-client/src';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { test, expect, login, consoleURL, password, csrf } from './fixtures';

const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  return headers;
}
async function createManaged(request: APIRequestContext, name = '账号界面验收') {
  const headers = await owner(request);
  const invitation = await request.post(`${api}/invitations`, { headers, data: { note: '账号界面隔离自验' } });
  expect(invitation.status()).toBe(200);
  const token = new URLSearchParams(new URL((await invitation.json()).url).hash.slice(1)).get('token');
  expect((await request.post(`${api}/auth/logout`, { headers })).status()).toBe(204);
  const username = `account_${randomBytes(6).toString('hex')}`;
  expect((await request.post(`${api}/public/invitations/accept`, { headers, data: { token, username, display_name: name, password } })).status()).toBe(204);
  await owner(request);
  return (await (await request.get(`${api}/access/accounts?q=${username}`)).json()).items[0] as ManagedAccount;
}
async function makeRole(request: APIRequestContext, name: string, permissions: PermissionCode[] = [P.PlatformAuditRead]) {
  const headers = await owner(request);
  const response = await request.post(`${api}/access/roles`, { headers, data: { name, description: '账号界面自验角色', reason: '账号界面自验' } });
  expect(response.status()).toBe(200);
  const role = await response.json() as Role;
  const configured = await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: permissions, reason: '账号界面自验' } });
  expect(configured.status()).toBe(200);
  return await configured.json() as Role;
}
async function openAccount(page: Page, target: ManagedAccount) {
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/access/accounts`);
  await page.getByLabel('搜索用户名或显示名称').fill(target.username);
  await page.getByRole('table', { name: '账号列表', exact: true }).getByRole('button', { name: target.display_name, exact: true }).click();
  await expect(page.getByRole('dialog').getByRole('tab', { name: '绑定角色', exact: true })).toBeVisible();
}
async function saveRoles(page: Page) {
  const panel = page.getByRole('dialog');
  await panel.getByLabel('变更原因', { exact: true }).fill('账号界面角色绑定自验');
  await panel.getByRole('button', { name: '保存角色绑定', exact: true }).click();
  await expect(panel.getByLabel('变更原因', { exact: true })).toHaveCount(0);
}

test('accounts paginate real rows, recover reads, filter and preserve drawer focus in both themes', async ({ page, request }, info) => {
  test.setTimeout(120_000);
  const prefix = `账号列表_${randomBytes(3).toString('hex')}`;
  const accounts: ManagedAccount[] = [];
  for (let i = 0; i < 12; i++) accounts.push(await createManaged(request, `${prefix} · ${i === 11 ? '一个需要完整查看的长显示名称和多项角色' : '资料维护员'} ${i}`));
  const disabled = accounts[0]!;
  const headers = await owner(request);
  expect((await request.post(`${api}/access/accounts/${disabled.id}/disable`, { headers, data: { expected_version: disabled.management_version, reason: '状态筛选自验' } })).status()).toBe(200);
  await login(page, { username: 'e2e_owner', password });
  let unavailable = true, failNext = true;
  await page.route('**/api/v1/access/accounts?*', async (route) => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor');
    if (unavailable || (cursor && failNext)) {
      if (cursor) failNext = false;
      await route.fulfill({ status: 503, json: { code: 'unavailable', message: '账号查询暂时不可用。' } });
    } else await route.continue();
  });
  await page.goto(`${consoleURL}/access/accounts`);
  await expect(page.getByRole('alert')).toContainText('账号查询暂时不可用');
  unavailable = false; await page.getByRole('button', { name: '重新加载', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByLabel('搜索用户名或显示名称').fill(prefix);
  const table = page.getByRole('table', { name: '账号列表', exact: true }), rows = table.locator('tbody tr');
  const pager = page.getByRole('navigation', { name: '账号列表分页', exact: true });
  await expect(rows).toHaveCount(10);
  const firstPage = await rows.locator('td:first-child').allTextContents();
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-list-light.png`, fullPage: true });
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-list-dark.png`, fullPage: true });
  await pager.getByRole('button', { name: '下一页' }).click();
  await expect(page.getByRole('alert')).toContainText('账号查询暂时不可用'); await expect(rows).toHaveCount(10); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('button', { name: '重新加载', exact: true }).click(); await expect(page.getByRole('alert')).toHaveCount(0);
  await pager.getByRole('button', { name: '下一页' }).click(); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(rows).toHaveCount(2);
  expect(new Set([...firstPage, ...await rows.locator('td:first-child').allTextContents()]).size).toBe(12);
  const trigger = rows.first().getByRole('button').first(); await trigger.focus(); await page.keyboard.press('Enter');
  const panel = page.getByRole('dialog');
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await expect(page.locator('[data-slot="drawer-overlay"]')).toHaveCSS('backdrop-filter', /blur\([1-9]/);
  await expect(panel.getByRole('heading', { name: '绑定的平台角色', exact: true })).toBeVisible();
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-drawer-dark.png` });
  await page.keyboard.press('Escape'); await expect(panel).toHaveCount(0); await expect(trigger).toBeFocused();
  await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(page.getByLabel('搜索用户名或显示名称')).toHaveValue(prefix);
  const menu = rows.first().getByRole('button', { name: /的操作$/ });
  await menu.click(); await page.getByRole('menuitem', { name: '查看详情', exact: true }).click(); await panel.getByRole('button', { name: '关闭面板' }).click(); await expect(menu).toBeFocused();
  await page.getByRole('tab', { name: '正常', exact: true }).click(); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('tab', { name: '全部账号', exact: true }).click(); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click(); await expect(rows).toHaveCount(12); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('tab', { name: '已停用', exact: true }).click(); await expect(rows).toHaveCount(1); await expect(table).toContainText(disabled.username);
  await page.getByRole('tab', { name: '正常', exact: true }).click(); await expect(rows).toHaveCount(11);
  await page.getByRole('button', { name: '绑定角色', exact: true }).click(); await page.getByRole('menuitemradio', { name: '基础成员', exact: true }).click(); await expect(rows).toHaveCount(11);
  if (info.project.name === 'desktop') {
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '创建时间', exact: true }).click(); await expect(table.getByRole('columnheader', { name: '创建时间', exact: true })).toHaveCount(0);
  }
  await page.getByLabel('搜索用户名或显示名称').fill(`${prefix}不存在`); await expect(page.getByText('没有符合条件的账号', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await expect(page.getByLabel('搜索用户名或显示名称')).toHaveValue(''); await expect(page.getByRole('tab', { name: '全部账号' })).toHaveAttribute('data-state', 'active');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('account roles save, explain their sources and reopen without stale selections', async ({ page, request }, info) => {
  const target = await createManaged(request), role = await makeRole(request, `账号审计_${randomBytes(4).toString('hex')}`);
  await openAccount(page, target);
  const panel = page.getByRole('dialog');
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-drawer-light.png` });
  await panel.getByRole('button', { name: '配置角色', exact: true }).click();
  await expect(panel.getByRole('button', { name: '保存角色绑定', exact: true })).toBeDisabled();
  await panel.getByLabel('搜索可选角色').fill(role.name); await panel.getByRole('checkbox', { name: new RegExp(role.name) }).check();
  await expect(panel.getByRole('region', { name: '角色变更摘要' })).toContainText(`新增：${role.name}`);
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-roles.png` });
  await saveRoles(page);
  await expect(panel.getByRole('heading', { name: role.name, exact: true })).toBeVisible();
  await panel.getByRole('tab', { name: '有效权限', exact: true }).click();
  await expect(panel.getByRole('region', { name: '平台审计权限', exact: true })).toContainText('查看平台审计');
  await expect(panel.getByText(`来自：${role.name}`, { exact: true })).toBeVisible();
  await expect(panel.locator('code').filter({ hasText: P.PlatformAuditRead })).toHaveCount(0);
  await panel.getByRole('switch', { name: '显示权限编码', exact: true }).check(); await expect(panel.getByText(P.PlatformAuditRead, { exact: true })).toBeVisible();
  await panel.getByLabel('搜索有效权限').fill(role.name); await expect(panel.getByRole('listitem')).toHaveCount(1);
  await page.screenshot({ path: `.artifacts/account-management-redesign/${info.project.name}-permissions.png` });
  await panel.getByRole('button', { name: '配置角色', exact: true }).click();
  await expect(panel.getByRole('checkbox', { name: new RegExp(role.name) })).toBeChecked();
  await panel.getByRole('checkbox', { name: new RegExp(role.name) }).uncheck(); await saveRoles(page);
  await expect(panel.getByRole('heading', { name: role.name, exact: true })).toHaveCount(0);
  await panel.getByRole('button', { name: '配置角色', exact: true }).click(); await expect(panel.getByRole('checkbox', { name: new RegExp(role.name) })).not.toBeChecked();
  await panel.getByRole('button', { name: '取消', exact: true }).click();
  await panel.getByRole('tab', { name: '租户任职', exact: true }).click(); await expect(panel.getByText('没有当前租户任职', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('account status confirms password when required and revokes existing sessions', async ({ page, request, browser }) => {
  const target = await createManaged(request), device = await browser.newContext();
  try {
    const member = await device.newPage(); await login(member, { username: target.username, password });
    await openAccount(page, target);
    let requirePassword = true;
    await page.route(`**/api/v1/access/accounts/${target.id}/disable`, async (route) => {
      if (requirePassword) { requirePassword = false; await route.fulfill({ status: 403, json: { code: 'reauthentication_required', message: '请确认当前密码后继续。' } }); }
      else await route.continue();
    });
    const panel = page.getByRole('dialog');
    await panel.getByRole('button', { name: '停用账号', exact: true }).click();
    await panel.getByRole('button', { name: '确认停用账号', exact: true }).click(); await expect(panel.getByText('请填写变更原因。')).toBeVisible();
    await panel.getByLabel('变更原因', { exact: true }).fill('账号启停与会话撤销自验'); await panel.getByRole('button', { name: '确认停用账号', exact: true }).click();
    await panel.getByLabel('当前密码', { exact: true }).fill(password); await panel.getByRole('button', { name: '确认密码', exact: true }).click();
    await expect(panel.getByText('密码已确认，请核对变更后再次提交。')).toBeVisible(); await panel.getByRole('button', { name: '确认停用账号', exact: true }).click();
    await expect(panel.getByRole('button', { name: '恢复账号', exact: true })).toBeVisible();
    expect((await device.request.get(`${api}/auth/me`)).status()).toBe(401);
    await panel.getByRole('button', { name: '恢复账号', exact: true }).click(); await panel.getByLabel('变更原因', { exact: true }).fill('恢复账号自验'); await panel.getByRole('button', { name: '确认恢复账号', exact: true }).click();
    await expect(panel.getByRole('button', { name: '停用账号', exact: true })).toBeVisible();
    expect((await device.request.get(`${api}/auth/me`)).status()).toBe(401);
    await login(member, { username: target.username, password });
  } finally { await device.close(); }
});

test('account editing rejects stale versions and reloads the real binding', async ({ page, request }) => {
  const target = await createManaged(request), role = await makeRole(request, `冲突验收_${randomBytes(4).toString('hex')}`);
  await openAccount(page, target);
  const panel = page.getByRole('dialog'); await panel.getByRole('button', { name: '配置角色', exact: true }).click();
  await panel.getByRole('checkbox', { name: new RegExp(role.name) }).check();
  const headers = await owner(request);
  expect((await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [], reason: '模拟另一位管理员修改' } })).status()).toBe(200);
  await panel.getByLabel('变更原因', { exact: true }).fill('过期草稿不能覆盖'); await panel.getByRole('button', { name: '保存角色绑定', exact: true }).click();
  await expect(panel.getByRole('button', { name: '重新加载并比较', exact: true })).toBeVisible(); await expect(panel.getByRole('button', { name: '保存角色绑定', exact: true })).toBeDisabled();
  await panel.getByRole('button', { name: '重新加载并比较', exact: true }).click(); await expect(panel.getByText('尚未绑定平台角色', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: '配置角色', exact: true }).click(); await expect(panel.getByRole('checkbox', { name: new RegExp(role.name) })).not.toBeChecked();
  await expect(panel.getByRole('checkbox', { name: /^基础成员/ })).not.toBeChecked();
});

test('account readers see allowed authorization without fetching protected directories', async ({ page, request }) => {
  const reader = await createManaged(request, '账号只读验收'), role = await makeRole(request, `只读账号_${randomBytes(4).toString('hex')}`, [P.PlatformAccountRead]);
  const headers = await owner(request);
  expect((await request.put(`${api}/access/accounts/${reader.id}/roles`, { headers, data: { expected_version: reader.management_version, role_ids: [...reader.roles.map((item) => item.id), role.id], reason: '只读账号自验' } })).status()).toBe(200);
  const protectedReads: string[] = [];
  page.on('request', (request) => { const path = new URL(request.url()).pathname; if (path === '/api/v1/access/roles' || path === '/api/v1/access/permissions' || /^\/api\/v1\/access\/accounts\/[^/]+\/tenant-memberships$/.test(path)) protectedReads.push(path); });
  await login(page, { username: reader.username, password }); await page.goto(`${consoleURL}/access/accounts`); await page.getByLabel('搜索用户名或显示名称').fill(reader.username);
  await expect(page.getByRole('button', { name: '绑定角色', exact: true })).toHaveCount(0);
  await page.getByRole('table', { name: '账号列表', exact: true }).getByRole('button', { name: reader.display_name, exact: true }).click();
  const panel = page.getByRole('dialog');
  await expect(panel.getByRole('button', { name: '配置角色', exact: true })).toHaveCount(0); await expect(panel.getByRole('button', { name: '停用账号', exact: true })).toHaveCount(0); await expect(panel.getByRole('tab', { name: '租户任职', exact: true })).toHaveCount(0);
  await panel.getByRole('tab', { name: '有效权限', exact: true }).click(); await expect(panel.getByText(P.PlatformAccountRead, { exact: true })).toBeVisible(); await expect(panel.getByText(`来自：${role.name}`, { exact: true })).toBeVisible();
  expect(protectedReads).toEqual([]);
});

test('partial role options retain existing disabled bindings and recover later pages without losing edits', async ({ page, request }) => {
  let target = await createManaged(request);
  const suffix = randomBytes(4).toString('hex');
  let bound = await makeRole(request, `已绑定停用_${suffix}`);
  let unbound = await makeRole(request, `未绑定停用_${suffix}`);
  const available = await makeRole(request, `后续页角色_${suffix}`);
  const headers = await owner(request);
  const assigned = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [...target.roles.map((role) => role.id), bound.id], reason: '部分角色页自验' } });
  expect(assigned.status()).toBe(200);
  for (const role of [bound, unbound]) {
    const response = await request.put(`${api}/access/roles/${role.id}/status`, { headers, data: { expected_version: role.version, status: 'disabled', reason: '停用角色候选自验' } });
    expect(response.status()).toBe(200);
    if (role.id === bound.id) bound = await response.json(); else unbound = await response.json();
  }
  target = await (await request.get(`${api}/access/accounts/${target.id}`)).json();
  let unavailable = true;
  let saves = 0;
  page.on('request', (request) => { if (request.method() === 'PUT' && new URL(request.url()).pathname === `/api/v1/access/accounts/${target.id}/roles`) saves++; });
  // Use real role records with controlled pagination to exercise options beyond
  // the loaded page, independently of other tests adding roles concurrently.
  await page.route('**/api/v1/access/roles?*', async (route) => {
    const later = new URL(route.request().url()).searchParams.get('cursor');
    if (later && unavailable) await route.fulfill({ status: 503, json: { code: 'unavailable', message: '后续角色暂时不可用。' } });
    else await route.fulfill({ json: { items: later ? [available, bound] : [unbound], next_cursor: later ? '' : 'options-next' } });
  });
  await openAccount(page, target);
  const panel = page.getByRole('dialog'); await panel.getByRole('button', { name: '配置角色', exact: true }).click();
  const existing = panel.getByRole('checkbox', { name: new RegExp(bound.name) });
  await expect(existing).toBeChecked(); await expect(existing).toBeEnabled();
  await expect(panel.getByRole('checkbox', { name: new RegExp(unbound.name) })).toBeDisabled();
  await existing.uncheck(); await panel.getByLabel('变更原因', { exact: true }).fill('保留编辑中的原因');
  await panel.getByRole('button', { name: '加载更多可选角色' }).click(); await expect(panel.getByRole('alert')).toContainText('后续角色暂时不可用');
  await expect(panel.getByRole('button', { name: '保存角色绑定' })).toBeDisabled(); await expect(existing).not.toBeChecked();
  unavailable = false; await panel.getByRole('button', { name: '重新加载', exact: true }).click(); await expect(panel.getByRole('alert')).toHaveCount(0);
  await panel.getByRole('button', { name: '加载更多可选角色' }).click(); await panel.getByRole('checkbox', { name: new RegExp(available.name) }).check();
  await expect(existing).not.toBeChecked(); await expect(panel.getByLabel('变更原因', { exact: true })).toHaveValue('保留编辑中的原因');
  expect(saves).toBe(0);
  await saveRoles(page); await expect(panel.getByRole('heading', { name: available.name, exact: true })).toBeVisible(); await expect(panel.getByRole('heading', { name: bound.name, exact: true })).toHaveCount(0);
  expect(saves).toBe(1);
  await panel.getByRole('tab', { name: '有效权限', exact: true }).click(); await expect(panel.getByText(`来自：${available.name}`, { exact: true })).toBeVisible();
});

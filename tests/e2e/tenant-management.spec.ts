import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { test, expect, login, createAccount, consoleURL, password, csrf } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const account = (await (await request.get(`${api}/access/accounts?q=e2e_owner`)).json()).items[0];
  return { headers, id: account.id as string };
}
async function create(request: APIRequestContext, headers: Record<string, string>, admin: string, name: string) {
  const code = `tm-${randomBytes(5).toString('hex')}`;
  const result = await request.post(`${api}/platform/tenants`, { headers, data: { name, code, description: '负责基础设施监测项目的日常管理与协作。', administrator_account_id: admin, client_request_id: randomUUID(), reason: '租户列表界面验收' } });
  expect(result.status()).toBe(201);
  return { id: (await result.json()).tenant_id as string, code };
}

test('tenant list filters, cursor pagination and panels preserve context and support edits', async ({ page, request }, info) => {
  test.setTimeout(90_000);
  const successor = await createAccount(request, false), { headers, id } = await owner(request), prefix = `租户界面_${randomBytes(3).toString('hex')}`;
  const tenants = [];
  for (let i = 0; i < 12; i++) tenants.push(await create(request, headers, id, `${prefix}_${i === 0 ? '长名称桥梁隧道基础设施监测运营管理中心'.repeat(3) : `监测项目 ${i + 1}`}`));
  const disabled = tenants[0]!;
  // Parallel fixture creation can change the global authorization revision.
  // Re-read only a stale preview; never replay a previously confirmed write.
  for (let attempt = 0; ; attempt++) {
    const preview = await request.post(`${api}/platform/tenants/${disabled.id}/disable/preview`, { headers, data: {} });
    expect(preview.status()).toBe(200); const impact = await preview.json();
    const response = await request.post(`${api}/platform/tenants/${disabled.id}/disable`, { headers, data: { expected_version: impact.expected_version, preview_hash: impact.preview_hash, reason: '筛选状态验收', tenant_notice: '维护期间暂停使用。' } });
    if (response.status() === 409 && attempt < 2) { expect((await response.json()).code).toBe('preview_changed'); continue; }
    expect(response.status(), await response.text()).toBe(200); break;
  }
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/platform/tenants`);
  const search = page.getByRole('textbox', { name: '搜索名称或当前 / 历史编号', exact: true });
  const table = page.getByRole('table', { name: '平台租户列表', exact: true });
  const pager = page.getByRole('navigation', { name: '平台租户列表分页', exact: true });
  await search.fill(prefix); await expect(table.getByRole('row')).toHaveCount(11);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: `.artifacts/tenant-management-redesign/${info.project.name}-list-light.png`, fullPage: true });
  if (info.project.name === 'desktop') {
    await page.setViewportSize({ width: 1024, height: 768 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: '.artifacts/tenant-management-redesign/tablet-list-light.png', fullPage: true }); await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '更新时间', exact: true }).click();
    await expect(table.getByRole('columnheader', { name: '更新时间', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '更新时间', exact: true }).click();
  }
  await pager.getByRole('button', { name: '下一页', exact: true }).click(); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(table.getByRole('row')).toHaveCount(3);
  const row = table.getByRole('row').nth(1), nameButton = row.locator('button[id$="-name"]');
  await nameButton.click();
  const panel = page.getByRole('dialog');
  await expect(panel.getByRole('region', { name: '租户详情', exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click(); await expect(nameButton).toBeFocused();
  await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(search).toHaveValue(prefix);
  const menu = row.getByRole('button', { name: /的操作$/ });
  await menu.click(); await page.getByRole('menuitem', { name: '编辑资料', exact: true }).click();
  await panel.getByLabel('租户说明', { exact: true }).fill('抽屉编辑后保存的说明');
  page.once('dialog', (dialog) => dialog.dismiss()); await panel.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(panel.getByLabel('租户说明', { exact: true })).toHaveValue('抽屉编辑后保存的说明');
  await panel.getByLabel('变更原因').fill('验证抽屉编辑保留分页'); await panel.getByRole('button', { name: '确认提交', exact: true }).click(); await expect(panel).toHaveCount(0);
  await expect(menu).toBeFocused(); await expect(pager.getByRole('status')).toContainText('第 2 页');
  await nameButton.click(); await expect(panel.getByText('抽屉编辑后保存的说明', { exact: true })).toBeVisible(); await panel.getByRole('button', { name: '关闭面板', exact: true }).click();
  await menu.click(); await page.getByRole('menuitem', { name: '交接管理员', exact: true }).click();
  await panel.getByLabel('搜索接任账号').fill(successor.username); await panel.getByRole('button', { name: `浏览器验收（${successor.username}）`, exact: true }).click();
  await panel.getByLabel('变更原因').fill('验证列表交接管理员'); await panel.getByRole('button', { name: '确认提交', exact: true }).click(); await expect(panel).toHaveCount(0);
  await expect(menu).toBeFocused(); await expect(pager.getByRole('status')).toContainText('第 2 页');
  await nameButton.click(); await expect(panel.getByText(`@${successor.username}`, { exact: true })).toBeVisible(); await panel.getByRole('button', { name: '关闭面板', exact: true }).click();
  for (const close of await page.locator('[data-sonner-toast]').getByRole('button', { name: '关闭提示', exact: true }).all()) await close.click();
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click();
  await expect(pager.getByRole('status')).toContainText('第 1 页'); await expect(table.getByRole('row')).toHaveCount(13);
  await page.getByRole('tab', { name: '已停用', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(2); await expect(table).toContainText(disabled.code);
  await table.getByRole('button', { name: /长名称/ }).first().click(); await expect(panel.getByText('维护期间暂停使用。', { exact: true })).toBeVisible();
  expect(await panel.locator('[data-slot="drawer-scroll-area"]').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: `.artifacts/tenant-management-redesign/${info.project.name}-drawer-light.png` });
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click();
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  await page.screenshot({ path: `.artifacts/tenant-management-redesign/${info.project.name}-list-dark.png`, fullPage: true });
  await table.getByRole('button', { name: /长名称/ }).first().click(); await page.screenshot({ path: `.artifacts/tenant-management-redesign/${info.project.name}-drawer-dark.png` }); await panel.getByRole('button', { name: '关闭面板', exact: true }).click();
  await page.getByRole('tab', { name: '已启用', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(12);
  await page.getByRole('button', { name: '管理员可用性', exact: true }).click(); await page.getByRole('menuitemradio', { name: '管理员不可用', exact: true }).click();
  await expect(page.getByText('没有符合筛选条件的租户', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '清除筛选', exact: true }).click();
  await expect(search).toHaveValue(''); await expect(page.getByRole('tab', { name: '全部租户', exact: true })).toHaveAttribute('aria-selected', 'true');
  const changedCode = `${disabled.code}-new`;
  const current = await (await request.get(`${api}/platform/tenants/${disabled.id}`)).json();
  expect((await request.patch(`${api}/platform/tenants/${disabled.id}`, { headers, data: { code: changedCode, expected_version: current.version, reason: '验证历史编号搜索' } })).status()).toBe(200);
  await search.fill(disabled.code); await expect(table.getByRole('row')).toHaveCount(2); await expect(table).toContainText(changedCode);
  await table.locator('button[id$="-name"]').click(); await panel.getByRole('link', { name: '打开完整档案', exact: true }).click();
  await expect(page).toHaveURL(`${consoleURL}/platform/tenants/${disabled.id}`); await expect(page.getByRole('button', { name: '恢复租户', exact: true })).toBeVisible();
});

test('tenant read-only list recovers from query errors and hides mutation controls', async ({ page, request }) => {
  const reader = await createAccount(request, false), { headers, id } = await owner(request);
  const tenant = await create(request, headers, id, `只读租户_${randomBytes(4).toString('hex')}`);
  const target = (await (await request.get(`${api}/access/accounts?q=${reader.username}`)).json()).items[0];
  const roleResponse = await request.post(`${api}/access/roles`, { headers, data: { name: `租户只读_${randomBytes(4).toString('hex')}`, description: '', reason: '列表权限验收' } }); expect(roleResponse.status()).toBe(200);
  const role = await roleResponse.json();
  expect((await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: [P.PlatformTenantRead], reason: '列表权限验收' } })).status()).toBe(200);
  expect((await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [role.id], reason: '列表权限验收' } })).status()).toBe(200);
  await login(page, reader);
  let unavailable = true;
  await page.route('**/api/v1/platform/tenants?*', (route) => unavailable ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '租户查询暂时不可用，请重试。' }) }) : route.continue());
  await page.goto(`${consoleURL}/platform/tenants`); await expect(page.getByRole('heading', { name: '加载失败', exact: true })).toBeVisible();
  unavailable = false; await page.getByRole('button', { name: '重新加载', exact: true }).click(); await expect(page.getByRole('heading', { name: '加载失败', exact: true })).toHaveCount(0);
  await page.getByLabel('搜索名称或当前 / 历史编号', { exact: true }).fill(tenant.code);
  const table = page.getByRole('table', { name: '平台租户列表', exact: true }); await expect(table.getByRole('row')).toHaveCount(2);
  await expect(page.getByRole('button', { name: '创建租户', exact: true })).toHaveCount(0);
  await table.getByRole('button', { name: /的操作$/ }).click(); await expect(page.getByRole('menuitem', { name: '编辑资料', exact: true })).toHaveCount(0); await expect(page.getByRole('menuitem', { name: '交接管理员', exact: true })).toHaveCount(0);
  await page.getByRole('menuitem', { name: '查看详情', exact: true }).click(); const panel = page.getByRole('dialog');
  await expect(panel.getByRole('button', { name: '编辑资料', exact: true })).toHaveCount(0); await expect(panel.getByRole('button', { name: '交接管理员', exact: true })).toHaveCount(0); await expect(panel.getByRole('link', { name: '打开完整档案', exact: true })).toBeVisible();
});

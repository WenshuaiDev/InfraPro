import { randomBytes } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { test, expect, login, createAccount, consoleURL, password, csrf } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  return headers;
}

test('audit explains real permission changes and preserves filtered pagination and focus', async ({ page, request }, info) => {
  const headers = await owner(request), prefix = `审计界面_${randomBytes(4).toString('hex')}`;
  let target = '';
  for (let i = 0; i < 12; i++) {
    const response = await request.post(`${api}/access/roles`, { headers, data: { name: `${prefix}_${i}`, description: '审计可读性验收', reason: prefix } });
    expect(response.status()).toBe(200);
    if (i === 11) {
      let role = await response.json(); target = role.id;
      role = await (await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: [P.PlatformInvitationRead, P.PlatformInvitationCreate], reason: prefix } })).json();
      expect((await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: [P.PlatformInvitationRead, P.PlatformAuditRead], reason: prefix } })).status()).toBe(200);
    }
  }
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/access/audit`);
  await page.getByLabel('搜索名称或原因').fill(prefix);
  const table = page.getByRole('table', { name: '平台审计记录', exact: true });
  const pager = page.getByRole('navigation', { name: '平台审计记录分页', exact: true });
  await expect(table.getByRole('row')).toHaveCount(11);
  await expect(table.getByRole('row').nth(1)).toContainText('新增 1 项权限，移除 1 项权限');
  await expect(table).not.toContainText(target);
  await expect(table).not.toContainText('RoleSetPermissions');
  await expect(table.getByRole('row').nth(1)).toContainText('成功');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: `.artifacts/platform-audit-redesign/${info.project.name}-list-light.png`, fullPage: true });
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  await page.screenshot({ path: `.artifacts/platform-audit-redesign/${info.project.name}-list-dark.png`, fullPage: true });
  const trigger = table.getByRole('row').nth(1).getByRole('button', { name: '配置角色权限', exact: true });
  await trigger.click();
  const panel = page.getByRole('dialog'), drawer = page.locator('[data-slot="drawer-popup"]');
  await expect(drawer).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await expect(page.locator('[data-slot="drawer-overlay"]')).toHaveCSS('backdrop-filter', /blur\([1-9][\d.]*px\)/);
  await expect(panel.getByRole('region', { name: '权限变化', exact: true })).toContainText('查看平台审计');
  await expect(panel.getByRole('region', { name: '权限变化', exact: true })).toContainText('创建基础成员邀请');
  await expect(panel.getByText(prefix, { exact: true })).toBeVisible();
  await expect(panel.locator('pre')).toHaveCount(0);
  await page.screenshot({ path: `.artifacts/platform-audit-redesign/${info.project.name}-drawer-dark.png` });
  await panel.getByRole('switch', { name: '显示完整前后内容', exact: true }).check();
  await expect(panel.getByText('操作前', { exact: true })).toBeVisible(); await expect(panel.getByText('操作后', { exact: true })).toBeVisible();
  await panel.getByRole('tab', { name: '技术信息', exact: true }).click();
  await expect(panel.getByText(target, { exact: true })).toBeVisible();
  await panel.getByRole('switch', { name: '显示原始内容', exact: true }).check();
  await expect(panel.getByLabel('原始审计内容')).toContainText(P.PlatformAuditRead);
  await page.keyboard.press('Escape'); await expect(panel).toHaveCount(0); await expect(trigger).toBeFocused();
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '浅色', exact: true }).click();
  await trigger.click(); await page.screenshot({ path: `.artifacts/platform-audit-redesign/${info.project.name}-drawer-light.png` }); await panel.getByRole('button', { name: '关闭面板' }).click();
  await pager.getByRole('button', { name: '下一页' }).click(); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(table.getByRole('row')).toHaveCount(5);
  const menu = table.getByRole('row').nth(1).getByRole('button', { name: /的操作$/ });
  await menu.click(); await page.getByRole('menuitem', { name: '查看详情', exact: true }).click(); await panel.getByRole('button', { name: '关闭面板' }).click();
  await expect(menu).toBeFocused(); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(page.getByLabel('搜索名称或原因')).toHaveValue(prefix);
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(15); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('button', { name: '操作类型', exact: true }).click(); await page.getByRole('menuitemradio', { name: '配置角色权限', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(3);
  if (info.project.name === 'desktop') {
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '原因', exact: true }).click(); await expect(table.getByRole('columnheader', { name: '原因', exact: true })).toBeVisible();
  }
  await page.getByRole('button', { name: '更多筛选', exact: true }).click();
  await panel.getByLabel('开始时间').fill('2099-01-02T00:00'); await panel.getByLabel('结束时间').fill('2099-01-01T00:00'); await panel.getByRole('button', { name: '应用筛选' }).click();
  await expect(panel.getByRole('alert')).toContainText('开始时间不能晚于结束时间');
  await panel.getByLabel('结束时间').fill('2100-01-01T00:00'); await panel.getByRole('button', { name: '应用筛选' }).click();
  await expect(page.getByText('没有符合条件的记录', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByLabel('搜索名称或原因').fill(`${prefix}_11`); await expect(table.getByRole('row')).toHaveCount(4);
  await page.getByRole('tab', { name: '登录与安全', exact: true }).click(); await expect(page.getByText('没有符合条件的记录', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: '管理变更', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(4);
});

test('audit presents failed login without attributing the visitor to the target account', async ({ page, request, browser }, info) => {
  const account = await createAccount(request, false), guest = await browser.newContext();
  try {
    const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(guest.request) };
    expect((await guest.request.post(`${api}/auth/login`, { headers, data: { username: account.username, password: 'wrong audit password' } })).status()).toBe(401);
    await login(page, { username: 'e2e_owner', password });
    let unavailable = true;
    await page.route('**/api/v1/access/audit-events?*', async (route) => {
      if (unavailable) await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '审计查询暂时不可用，请重试。' }) });
      else await route.continue();
    });
    await page.goto(`${consoleURL}/access/audit`);
    await expect(page.getByRole('heading', { name: '加载失败', exact: true })).toBeVisible();
    unavailable = false;
    await page.getByRole('button', { name: '重新加载', exact: true }).click();
    await expect(page.getByRole('heading', { name: '加载失败', exact: true })).toHaveCount(0);
    await page.getByRole('tab', { name: '登录与安全', exact: true }).click(); await page.getByLabel('搜索名称或原因').fill(account.username);
    await page.getByRole('button', { name: '操作结果', exact: true }).click(); await page.getByRole('menuitemradio', { name: '失败', exact: true }).click();
    const table = page.getByRole('table', { name: '平台审计记录', exact: true }); await expect(table.getByRole('row')).toHaveCount(2);
    await expect(table).toContainText('未认证的访问者'); await expect(table).toContainText('浏览器验收'); await expect(table).toContainText('登录验证未通过');
    await table.getByRole('button', { name: '登录账号', exact: true }).click();
    const panel = page.getByRole('dialog'); await expect(panel.getByText('这是一条账号安全事件，没有业务字段的前后差异。')).toBeVisible();
    await expect(panel.locator('pre')).toHaveCount(0); await expect(panel).not.toContainText('验证通过，建立了登录会话');
    await page.screenshot({ path: `.artifacts/platform-audit-redesign/${info.project.name}-failed-login.png` });
  } finally { await guest.close(); }
});

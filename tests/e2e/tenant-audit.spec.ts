import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import type { TenantAuditEvent } from '../../packages/ts/api-client/src';
import { test, expect, login, consoleURL, password, csrf } from './fixtures';

// Shanghai audit times must remain stable even when the browser uses another zone.
test.use({ timezoneId: 'America/Los_Angeles' });
const api = new URL('/api/v1', consoleURL).href;

async function prepare(request: APIRequestContext, updates = 0) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const actor = (await (await request.get(`${api}/access/accounts?q=e2e_owner`)).json()).items[0] as { id: string; display_name: string };
  const prefix = `审计回归_${randomBytes(5).toString('hex')}`, code = `audit-${randomBytes(6).toString('hex')}`;
  const response = await request.post(`${api}/platform/tenants`, { headers, data: { name: `${prefix}_0`, code, description: '', administrator_account_id: actor.id, reason: prefix, client_request_id: randomUUID() } });
  expect(response.status()).toBe(201);
  const tenant = (await response.json()) as { tenant_id: string; version: string };
  for (let i = 1; i <= updates; i++) {
    const result = await request.patch(`${api}/tenants/${tenant.tenant_id}/profile`, { headers, data: { expected_version: tenant.version, name: `${prefix}_${i}`, code, description: `第 ${i} 次资料修改`, reason: prefix } });
    expect(result.status()).toBe(200); tenant.version = (await result.json()).version;
  }
  return { ...tenant, prefix, actor };
}

test('tenant audit filters and paginates real history with readable details and focus restoration', async ({ page, request }, info) => {
  const tenant = await prepare(request, 12);
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/t/${tenant.tenant_id}/audit`);
  await page.getByLabel('搜索操作或原因').fill(tenant.prefix);
  const table = page.getByRole('table', { name: '租户操作记录', exact: true });
  const pager = page.getByRole('navigation', { name: '租户操作记录分页', exact: true });
  await expect(table.getByRole('row')).toHaveCount(11);
  await expect(table).toContainText(tenant.actor.display_name);
  await expect(table).not.toContainText(tenant.actor.id);
  await expect(table).not.toContainText('tenant.profile.updated');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('list-light.png'), fullPage: true });
  const trigger = table.getByRole('row').nth(1).getByRole('button', { name: '修改租户资料', exact: true });
  await trigger.click();
  const panel = page.getByRole('dialog');
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await expect(panel.getByRole('region', { name: '名称变化', exact: true })).toContainText(`${tenant.prefix}_11`);
  await expect(panel.getByRole('region', { name: '名称变化', exact: true })).toContainText(`${tenant.prefix}_12`);
  await expect(panel.getByLabel('原始审计内容')).toHaveCount(0);
  await page.screenshot({ path: info.outputPath('details-light.png') });
  await panel.getByRole('tab', { name: '技术信息', exact: true }).click();
  await expect(panel.getByText(tenant.actor.id, { exact: true })).toBeVisible();
  await panel.getByRole('switch', { name: '显示原始内容', exact: true }).check();
  await expect(panel.getByLabel('原始审计内容')).toContainText('e2e_owner');
  await page.keyboard.press('Escape'); await expect(panel).toHaveCount(0); await expect(trigger).toBeFocused();
  await pager.getByRole('button', { name: '下一页' }).click();
  await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(table.getByRole('row')).toHaveCount(4);
  const menu = table.getByRole('row').nth(1).getByRole('button', { name: /的操作$/ });
  await menu.click(); await page.getByRole('menuitem', { name: '查看技术信息', exact: true }).click();
  await expect(panel.getByRole('tab', { name: '技术信息', exact: true })).toHaveAttribute('aria-selected', 'true');
  await panel.getByRole('button', { name: '关闭面板' }).click(); await expect(menu).toBeFocused();
  await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(page.getByLabel('搜索操作或原因')).toHaveValue(tenant.prefix);
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click();
  await expect(table.getByRole('row')).toHaveCount(14); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('button', { name: '操作类型', exact: true }).click(); await page.getByRole('menuitemradio', { name: '修改租户资料', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(13);
  if (info.project.name === 'desktop') {
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '操作原因', exact: true }).click();
    await expect(table.getByRole('columnheader', { name: '操作原因', exact: true })).toHaveCount(0);
  }
  await page.getByRole('button', { name: '更多筛选', exact: true }).click();
  await panel.getByLabel('开始时间（上海）').fill('2099-01-02T00:00'); await panel.getByLabel('结束时间（上海）').fill('2099-01-01T00:00');
  await panel.getByRole('button', { name: '应用筛选' }).click(); await expect(panel.getByRole('alert')).toContainText('开始时间不能晚于结束时间');
  await panel.getByLabel('结束时间（上海）').fill('2100-01-01T00:00');
  const filtered = page.waitForRequest((r) => r.url().includes(`/tenants/${tenant.tenant_id}/audit-events?`) && new URL(r.url()).searchParams.get('from') === '2099-01-01T16:00:00.000Z');
  await panel.getByRole('button', { name: '应用筛选' }).click(); await filtered;
  await expect(page.getByText('没有符合条件的记录', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(14);
  await page.getByRole('button', { name: '更多筛选', exact: true }).click();
  await panel.getByLabel('开始时间（上海）').fill('2099-01-01T00:00'); await panel.getByRole('button', { name: '取消', exact: true }).click();
  await expect(table.getByRole('row')).toHaveCount(14);
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  await page.screenshot({ path: info.outputPath('list-dark.png'), fullPage: true });
  await trigger.click(); await page.screenshot({ path: info.outputPath('details-dark.png') }); await panel.getByRole('button', { name: '关闭面板' }).click();
  await page.setViewportSize({ width: 812, height: 375 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('landscape.png'), fullPage: true });
});

test('tenant audit handles failures, legacy snapshots, resource labels and Shanghai time without guessing results', async ({ page, request }, info) => {
  const tenant = await prepare(request);
  await login(page, { username: 'e2e_owner', password });
  let unavailable = true;
  const event: TenantAuditEvent = { id: '900001', tenant_id: tenant.tenant_id, event_type: 'resource_access.link.created', actor_id: tenant.actor.id, actor_type: 'account', actor_scope: 'tenant', reason: '关联监测设施', request_id: 'audit-request-example', created_at: '2026-11-12T01:15:40Z', details: {} };
  await page.route(`**/api/v1/tenants/${tenant.tenant_id}/audit-events?*`, async (route) => {
    if (unavailable) return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '操作记录暂时不可用，请重试。' }) });
    return route.fulfill({ json: { items: [event, { ...event, id: '900000', event_type: 'future.unknown', actor_id: '', actor_type: 'platform', actor_scope: 'platform' }, { ...event, id: '899999', event_type: 'resource.update', details: { before: null, after: { name: '历史设施', code: 'FAC-01', changed_fields: ['name'] } } }], next_cursor: '' } });
  });
  await page.goto(`${consoleURL}/t/${tenant.tenant_id}/audit`);
  await expect(page.getByRole('heading', { name: '加载失败', exact: true })).toBeVisible();
  unavailable = false; await page.getByRole('button', { name: '重新加载', exact: true }).click();
  const table = page.getByRole('table', { name: '租户操作记录', exact: true });
  await expect(table.getByRole('row')).toHaveCount(4);
  await expect(table).toContainText('账号名称未记录'); await expect(table).toContainText('平台管理员'); await expect(table).toContainText('09:15:40');
  await expect(table).not.toContainText('成功'); await expect(table).not.toContainText(event.actor_id); await expect(table).not.toContainText('future.unknown');
  await table.getByRole('button', { name: '关联项目设施', exact: true }).click();
  const panel = page.getByRole('dialog');
  await expect(panel.getByText('这条历史记录未保存字段快照，无法还原具体的前后变化。')).toBeVisible();
  await panel.getByRole('button', { name: '关闭面板' }).click();
  await table.getByRole('button', { name: '修改工程资源', exact: true }).click();
  await expect(panel).toContainText('历史设施');
  await expect(panel).toContainText('这条记录仅保留操作摘要，没有可还原的前后差异');
  await expect(panel.getByText('操作前', { exact: true })).toHaveCount(0);
  await panel.getByRole('button', { name: '关闭面板' }).click();
  await table.getByRole('button', { name: '其他操作', exact: true }).click();
  await panel.getByRole('tab', { name: '技术信息', exact: true }).click(); await expect(panel.getByText('future.unknown', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: '关闭面板' }).click();
  await page.getByRole('button', { name: '操作类型', exact: true }).click(); await page.getByRole('menuitemradio', { name: '关联项目设施', exact: true }).click();
  await expect(page.getByRole('button', { name: '操作类型', exact: true })).toContainText('关联项目设施');
  await page.getByRole('button', { name: '时间范围', exact: true }).click(); await page.getByRole('menuitemradio', { name: '自定义时间', exact: true }).click();
  await expect(panel.getByLabel('开始时间（上海）')).toBeVisible(); await panel.getByRole('button', { name: '取消', exact: true }).click();
  await expect(page.getByRole('button', { name: '时间范围', exact: true })).toBeFocused();
  await page.getByRole('button', { name: '时间范围', exact: true }).click();
  const today = page.waitForRequest((r) => r.url().includes('/audit-events?') && Boolean(new URL(r.url()).searchParams.get('from')));
  await page.getByRole('menuitemradio', { name: '今天', exact: true }).click();
  expect(new URL((await today).url()).searchParams.get('from')).toMatch(/T16:00:00\.000Z$/);
  await page.screenshot({ path: info.outputPath('legacy-records.png'), fullPage: true });
});

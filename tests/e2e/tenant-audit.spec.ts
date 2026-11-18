import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { test, expect, login, consoleURL, password, csrf } from './fixtures';

// Shanghai audit times must remain stable even when the browser uses another zone.
test.use({ timezoneId: 'America/Los_Angeles' });
const api = new URL('/api/v1', consoleURL).href;

async function prepare(request: APIRequestContext, updates = 0) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const actor = (await (await request.get(`${api}/access/accounts?q=e2e_owner`)).json()).items[0] as { id: string; display_name: string };
  const prefix = `审计回归_${randomBytes(5).toString('hex')}`, code = `audit-${randomBytes(6).toString('hex')}`;
  const response = await request.post(`${api}/platform/tenants`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { name: `${prefix}_0`, code, description: '', administrator_account_id: actor.id, reason: prefix, client_request_id: randomUUID() } });
  expect(response.status()).toBe(201);
  const tenant = (await response.json()) as { tenant_id: string; version: string };
  for (let i = 1; i <= updates; i++) {
    const result = await request.patch(`${api}/tenants/${tenant.tenant_id}/profile`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { expected_version: tenant.version, name: `${prefix}_${i}`, code, description: `第 ${i} 次资料修改`, reason: prefix } });
    expect(result.status()).toBe(200); tenant.version = (await result.json()).version;
  }
  // Platform creation is not copied into the tenant audit stream.
  return { ...tenant, prefix, actor };
}


test('tenant audit filters and paginates real history with readable details and focus restoration', async ({ page, request }, info) => {
  const tenant = await prepare(request, 12);
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/t/${tenant.tenant_id}/audit`);
  await page.getByLabel('搜索当前获准内容').fill(tenant.prefix); await page.getByRole('button', { name: '搜索', exact: true }).click();
  const records = page.locator('[data-audit-record]'); await expect(records).toHaveCount(10);
  const trigger = records.first(), id = await trigger.getAttribute('data-audit-record'); await trigger.click();
  const panel = page.getByRole('dialog'); await expect(panel).toContainText(`${tenant.prefix}_11`); await expect(panel).toContainText(`${tenant.prefix}_12`);
  await expect(panel.getByRole('heading', { name: /实际变更/ })).toBeVisible(); await page.screenshot({ path: info.outputPath('details.png') });
  await page.keyboard.press('Escape'); await expect(page.locator(`[data-audit-record="${id}"]`)).toBeFocused();
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(records).toHaveCount(2); await page.reload(); await expect(records).toHaveCount(2);
  await page.getByRole('combobox', { name: '每页条数' }).click(); await page.getByRole('option', { name: '25', exact: true }).click(); await expect(records).toHaveCount(12);
  await page.getByRole('button', { name: '更多筛选', exact: true }).click();
  await panel.getByLabel('开始时间（上海时区）').fill('2099-01-02T00:00'); await panel.getByLabel('结束时间（上海时区）').fill('2099-01-01T00:00');
  await panel.getByRole('button', { name: '应用筛选', exact: true }).click(); await expect(panel.getByRole('alert')).toBeVisible();
  await panel.getByLabel('结束时间（上海时区）').fill('2099-01-03T00:00');
  const filtered = page.waitForRequest(r => r.url().includes(`/tenants/${tenant.tenant_id}/audit/events?`) && new URL(r.url()).searchParams.get('from') === '2099-01-01T16:00:00.000Z');
  await panel.getByRole('button', { name: '应用筛选', exact: true }).click(); await filtered;
  await expect(page.getByText('当前筛选下没有可见记录。', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await expect(records).toHaveCount(12);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('tenant audit hides stale records during source failure and recovers the current authorized history', async ({ page, request }) => {
  const tenant = await prepare(request, 1);
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/t/${tenant.tenant_id}/audit`);
  const records = page.locator('[data-audit-record]'); await expect(records).toHaveCount(1);
  await page.route(`**/api/v1/tenants/${tenant.tenant_id}/audit/events?*`, route => route.fulfill({status:503,json:{code:'audit_source_unavailable',message:'操作来源暂时不可核实。'}}));
  await page.getByRole('button', { name: '刷新记录', exact: true }).click();
  await expect(page.getByText('操作来源暂时不可核实。', { exact: true })).toBeVisible(); await expect(records).toHaveCount(0);
  await page.unroute(`**/api/v1/tenants/${tenant.tenant_id}/audit/events?*`); await page.getByRole('button', { name: '刷新记录', exact: true }).click(); await expect(records).toHaveCount(1);
});

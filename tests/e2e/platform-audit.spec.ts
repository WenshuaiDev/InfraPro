import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { test, expect, login, createAccount, consoleURL, password, csrf } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  return headers;
}


test('audit explains real permission changes and preserves filtered pagination and focus', async ({ page, request }, info) => {
  const headers = await owner(request), prefix = `审计界面_${randomBytes(4).toString('hex')}`;
  for (let i = 0; i < 12; i++) {
    const response = await request.post(`${api}/access/roles`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { name: `${prefix}_${i}`, description: '审计回归', reason: prefix } });
    expect(response.status()).toBe(200);
    if (i === 11) {
      let role = await response.json();
      role = await (await request.put(`${api}/access/roles/${role.id}/permissions`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { expected_version: role.version, permission_codes: [P.PlatformInvitationRead, P.PlatformInvitationCreate], reason: prefix } })).json();
      expect((await request.put(`${api}/access/roles/${role.id}/permissions`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { expected_version: role.version, permission_codes: [P.PlatformInvitationRead, P.PlatformAuditRead], reason: prefix } })).status()).toBe(200);
    }
  }
  await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/access/audit`);
  await page.getByLabel('搜索当前获准内容').fill(prefix); await page.getByRole('button', { name: '搜索', exact: true }).click();
  const records = page.locator('[data-audit-record]'); await expect(records).toHaveCount(10);
  const trigger = records.first(), id = await trigger.getAttribute('data-audit-record');
  await trigger.click(); const panel = page.getByRole('dialog');
  await expect(panel.getByText(prefix, { exact: true })).toBeVisible();
  await expect(panel.getByRole('heading', { name: /实际变更/ })).toBeVisible();
  await expect(panel).toContainText('权限');
  await panel.getByRole('tab', { name: '技术信息', exact: true }).click();
  await expect(panel.getByText('技术排障信息', { exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('details.png') });
  await page.keyboard.press('Escape'); await expect(page.locator(`[data-audit-record="${id}"]`)).toBeFocused();
  const ids = await records.evaluateAll(nodes => nodes.map(n => n.getAttribute('data-audit-record')));
  await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(records).toHaveCount(4);
  for (const next of await records.evaluateAll(nodes => nodes.map(n => n.getAttribute('data-audit-record')))) expect(ids).not.toContain(next);
  await page.reload(); await expect(records).toHaveCount(4); await expect(page.getByLabel('搜索当前获准内容')).toHaveValue(prefix);
  await page.getByLabel('每页', { exact: true }).selectOption('25'); await expect(records).toHaveCount(14);
  await page.getByLabel('操作结果', { exact: true }).selectOption('failure'); await expect(page.getByText('当前筛选下没有可见记录。', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await expect(records.first()).toBeVisible();
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('audit presents failed login without attributing the visitor to the target account', async ({ page, request, browser }) => {
  const account = await createAccount(request, false), guest = await browser.newContext();
  try {
    const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(guest.request) };
    expect((await guest.request.post(`${api}/auth/login`, { headers, data: { username: account.username, password: 'wrong audit password' } })).status()).toBe(401);
    await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/access/audit`);
    await page.getByLabel('搜索当前获准内容').fill(account.username); await page.getByRole('button', { name: '搜索', exact: true }).click();
    await page.getByLabel('操作结果', { exact: true }).selectOption('failure');
    const records = page.locator('[data-audit-record]'); await expect(records).toHaveCount(1); await records.first().click();
    const panel = page.getByRole('dialog'); await expect(panel).toContainText('执行失败');
    // A rejected login has no authenticated actor; never substitute its target account.
    await expect(panel.locator('dl > div').filter({ has: page.getByText('操作者', { exact: true }) }).locator('dd')).toHaveText('未记录');
    await expect(panel.getByText('名称来源', { exact: true })).toBeVisible();
  } finally { await guest.close(); }
});

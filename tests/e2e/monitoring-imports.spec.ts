import { randomUUID } from 'node:crypto';
import { test, expect, consoleURL, csrf, login } from './fixtures';
import { seedResources } from '../helpers/resource-fixture';

test.use({ actionTimeout: 15000 });
test.skip(process.env.INFRAPRO_MONITORING_E2E !== '1', 'requires the disposable monitoring lab');

test('imports search the full authorized set, retain list context through drawers, and retry real commands', async ({ page, request }, info) => {
  test.setTimeout(180000);
  const f = await seedResources(request); await login(page, f.user);
  const errors: string[] = []; page.on('pageerror', e => errors.push(e.message));
  const origin = new URL(consoleURL).origin, api = `${origin}/api/v1/tenants/${f.tenant}/monitoring/imports`;
  const root = `${consoleURL}/t/${f.tenant}/monitoring/imports`;
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
  const batches: { id: string; revision: string; name: string }[] = [];
  for (let i = 0; i < 23; i++) {
    const response = await page.request.post(api, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { request_id: randomUUID(), expected_revision: '0', reason: '列表与 Drawer 合成自验', name: `列表样本-${String(i).padStart(2,'0')}.${i % 3 === 0 ? 'xlsx' : 'csv'}`, size_bytes: '4' } });
    expect(response.status(), await response.text()).toBe(201); batches.push(await response.json());
    if (i > 0 && i < 22) { const b = batches.at(-1)!; const cancel = await page.request.post(`${api}/${b.id}/cancel`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { request_id: randomUUID(), expected_revision: b.revision, reason: '合成历史批次' } }); expect(cancel.status(), await cancel.text()).toBe(200); }
  }
  for (const query of ['limit=11', 'states=unknown', 'formats=pdf', 'sort=bad', 'q=a&q=b']) {
    expect((await page.request.get(`${api}?${query}`)).status()).toBe(query === 'q=a&q=b' ? 400 : 422);
  }
  await page.goto(root);
  const table = page.getByRole('table', { name: '监测导入批次' });
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(10);
  await expect(table).toContainText('列表样本-22.csv');
  await page.getByRole('checkbox', { name: '选择当前页全部批次' }).check();
  await expect(page.getByText('本页 10 个批次 · 已选 10', { exact: false })).toBeVisible();
  await page.getByRole('button', { name: '下一页批次', exact: true }).click();
  await expect(table).toContainText('列表样本-12.xlsx');
  await expect(page.getByRole('button', { name: '清除选择' })).toHaveCount(0);
  const pageTwo = page.url(), file = page.getByRole('button', { name: '列表样本-12.xlsx', exact: true });
  await file.focus(); await page.keyboard.press('Enter');
  const drawer = page.getByRole('dialog');
  await expect(drawer.getByRole('heading', { name: '列表样本-12.xlsx', exact: true })).toBeVisible();
  await drawer.getByRole('tab', { name: '处理记录', exact: true }).click();
  await expect(drawer).toContainText('列表与 Drawer 合成自验');
  await expect(page).toHaveURL(/tab=history/);
  await page.reload();
  await expect(drawer.getByRole('tab', { name: '处理记录', exact: true })).toHaveAttribute('data-state', 'active');
  await page.keyboard.press('Escape');
  await expect(drawer).toHaveCount(0); expect(page.url()).toBe(pageTwo);
  await expect(file).toBeFocused();
  await page.getByRole('button', { name: '上一页批次', exact: true }).click();
  await expect(table).toContainText('列表样本-22.csv');
  await page.getByRole('textbox', { name: '搜索文件名或批次编号', exact: true }).fill('列表样本-00');
  await page.getByRole('button', { name: '搜索', exact: true }).click();
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(1);
  await expect(table).toContainText('列表样本-00.xlsx');
  await page.getByRole('button', { name: '列表样本-00.xlsx', exact: true }).click();
  await drawer.getByRole('button', { name: '取消此批次', exact: true }).click();
  await drawer.getByLabel('操作原因').fill('合成批次取消及重试自验');
  // A workspace focus check must hide unverified data while retaining the draft.
  let releaseContext!: () => void, contextSeen!: () => void;
  const contextGate = new Promise<void>(resolve => { releaseContext = resolve; });
  const contextRequest = new Promise<void>(resolve => { contextSeen = resolve; });
  const contextURL = `**/api/v1/tenants/${f.tenant}/context`;
  await page.route(contextURL, async route => { const response = await route.fetch(); contextSeen(); await contextGate; await route.fulfill({ response }); });
  try {
    await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await contextRequest;
    await expect(drawer.getByLabel('操作原因')).toHaveValue('合成批次取消及重试自验');
    await expect(drawer.getByLabel('操作原因')).toBeHidden();
  } finally { releaseContext(); }
  await expect(drawer.getByLabel('操作原因')).toBeVisible();
  await expect(drawer.getByLabel('操作原因')).toHaveValue('合成批次取消及重试自验');
  await page.unroute(contextURL);
  const requestIDs: string[] = [];
  await page.route(`**/monitoring/imports/${batches[0]!.id}/cancel`, async route => {
    requestIDs.push(route.request().postDataJSON().request_id);
    if (requestIDs.length === 1) await route.abort('failed'); else await route.continue();
  });
  await drawer.getByRole('button', { name: '确认取消该批次', exact: true }).click();
  await expect(drawer.getByRole('heading', { name: '未能完成请求', exact: true })).toBeVisible();
  await drawer.getByRole('button', { name: '确认取消该批次', exact: true }).click();
  await expect(drawer.getByText('已取消', { exact: true })).toBeVisible();
  expect(requestIDs).toHaveLength(2); expect(requestIDs[0]).toBe(requestIDs[1]);
  await drawer.getByRole('button', { name: '关闭批次详情', exact: true }).click();
  await expect(table).toContainText('已取消');
  await page.getByRole('button', { name: '状态', exact: true }).click();
  await page.getByRole('menuitemcheckbox', { name: '已取消', exact: true }).click(); await page.keyboard.press('Escape');
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(1);
  await page.getByRole('button', { name: '文件类型', exact: true }).click();
  await page.getByRole('menuitemcheckbox', { name: 'CSV', exact: true }).click(); await page.keyboard.press('Escape');
  await expect(page.getByText('没有符合条件的批次', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(10);
  // A failed refresh must not keep disclosing the previous successful list.
  await page.route('**/monitoring/imports?*', route => route.fulfill({ status: 503, json: { code: 'unavailable', message: '批次列表暂不可用' } }));
  await page.getByRole('button', { name: '刷新数据', exact: true }).click();
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(0);
  await expect(page.getByText('批次列表暂不可用', { exact: true })).toBeVisible();
  await page.unroute('**/monitoring/imports?*');
  await page.getByRole('button', { name: '刷新数据', exact: true }).click();
  await expect(table.locator('tbody tr[data-import-id]')).toHaveCount(10);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('imports-a-list.png'), fullPage: true });
  await page.getByRole('button', { name: '列表样本-22.csv', exact: true }).click();
  await drawer.getByRole('tab', { name: '映射配置', exact: true }).click();
  await expect(drawer.getByLabel('重新选择原文件以续传', { exact: true })).toBeVisible();
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: info.outputPath('imports-drawer-dark.png'), fullPage: true });
  await page.keyboard.press('Escape');
  await page.goto(`${consoleURL}/t/${f.tenant}/monitoring/batches/${batches[1]!.id}`);
  await expect(page).toHaveURL(new RegExp(`/imports/${batches[1]!.id}`));
  await expect(drawer.getByRole('heading', { name: '列表样本-01.csv' })).toBeVisible();
  expect(errors).toEqual([]);
});

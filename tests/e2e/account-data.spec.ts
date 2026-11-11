import { test, expect, login, createAccount, csrf, consoleURL } from './fixtures';
import { expectIdentity, logout, navigateTo } from '../helpers/navigation';

test('cursor tables retain rows on failure and append real invitation and event pages', async ({ page, context, account }) => {
  test.setTimeout(90_000);
  await login(page, account);
  const api = new URL('/api/v1', consoleURL).href;
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(context.request) };
  // Real writes in the isolated test schema; revoke each batch to respect the active-link cap.
  for (let offset = 0; offset < 51; offset += 10) {
    for (let index = offset; index < Math.min(offset + 10, 51); index++) {
      expect((await context.request.post(`${api}/invitations`, { headers, data: { note: `游标验收-${index}` } })).status()).toBe(200);
    }
    const response = await context.request.get(`${api}/invitations`);
    const result = await response.json() as { items: { id: string; status: string }[] };
    for (const item of result.items.filter((item) => item.status === 'pending')) expect((await context.request.post(`${api}/invitations/${item.id}/revoke`, { headers })).status()).toBe(204);
  }
  await navigateTo(page, '账号开户邀请');
  const rows = page.getByRole('table', { name: '我的邀请', exact: true }).locator('tbody tr');
  await expect(rows).toHaveCount(50);
  let failNext = true;
  await page.route('**/api/v1/invitations?cursor=*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('cursor') && failNext) {
      failNext = false;
      await route.fulfill({ status: 503, json: { code: 'unavailable', message: '下一页暂时不可用。', request_id: 'cursor-test' } });
    } else await route.continue();
  });
  await page.getByRole('button', { name: '加载更多邀请', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('下一页暂时不可用');
  await expect(rows).toHaveCount(50);
  await page.getByRole('button', { name: '重新加载', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.getByRole('button', { name: '加载更多邀请', exact: true }).click();
  await expect(rows).toHaveCount(51);
  const notes = await rows.locator('td:first-child').allTextContents();
  expect(new Set(notes).size).toBe(51);
  await expect(page.getByRole('button', { name: '加载更多邀请', exact: true })).toHaveCount(0);
  await expect(page.getByRole('tab', { name: '已撤销 51', exact: true })).toBeVisible();
  await page.getByLabel('搜索用途备注').fill('游标验收-0');
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText('游标验收-0');
  await expect(page.getByRole('tab', { name: '全部邀请 51', exact: true })).toBeVisible();
  await navigateTo(page, '账号安全');
  const records = page.getByRole('table', { name: '近期安全记录', exact: true }).locator('tbody tr');
  await expect(records).toHaveCount(50);
  await page.getByRole('button', { name: '加载更多记录', exact: true }).click();
  await expect(records).toHaveCount(100);
});

test('a late invitation query cannot populate the next account cache', async ({ page, context, request, account }) => {
  const nextAccount = await createAccount(request);
  await login(page, account);
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(context.request) };
  expect((await context.request.post(`${new URL('/api/v1', consoleURL).href}/invitations`, { headers, data: { note: '只能属于前一账号的延迟列表' } })).status()).toBe(200);
  let release = () => {};
  let notify = () => {};
  let delivered = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; });
  const fetched = new Promise<void>((resolve) => { notify = resolve; });
  const completed = new Promise<void>((resolve) => { delivered = resolve; });
  let first = true;
  await page.route('**/api/v1/invitations?cursor=*', async (route) => {
    if (!first) { await route.continue(); return; }
    first = false;
    const response = await route.fetch();
    notify();
    await held;
    try { await route.fulfill({ response }); } finally { delivered(); }
  });
  try {
    await navigateTo(page, '账号开户邀请');
    await fetched;
    const other = await context.newPage();
    await other.goto(consoleURL);
    await logout(other);
    await expect(other.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await login(other, nextAccount);
    const refreshed = page.waitForResponse((r) => r.url().endsWith('/auth/me'));
    await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await refreshed;
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expectIdentity(page, nextAccount.username, '浏览器验收');
    await expect(page.getByRole('heading', { name: '从第一条邀请开始' })).toBeVisible();
    release();
    await completed;
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expect(page.getByText('只能属于前一账号的延迟列表')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: '从第一条邀请开始' })).toBeVisible();
  } finally {
    release();
    await page.unrouteAll({ behavior: 'wait' });
  }
});

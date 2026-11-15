import { test, expect, login, consoleURL, password, createAccount } from './fixtures';
import { navigateTo, logout } from '../helpers/navigation';
import { prepareWorkspaces } from '../helpers/workspaces';

test('global task center can return directly to its platform source page', async ({ page }, info) => {
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/access/accounts?q=e2e&page=2#accounts`);
  await expect(page.getByRole('heading', { name: '账号管理', exact: true })).toBeVisible();
  const source = page.url();
  await page.getByRole('link', { name: /^任务中心/ }).click();
  await expect(page.getByRole('heading', { name: '任务中心', exact: true })).toBeVisible();
  const back = page.getByRole('link', { name: /^返回工作台/ });
  await expect(back).toHaveAttribute('href', source.replace(consoleURL, ''));
  await expect(page.getByRole('navigation', { name: '工作台返回导航' })).toContainText('平台管理工作台 · 账号管理');
  for (const name of ['个人资料', '账号安全', '账号首页']) {
    await navigateTo(page, name);
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
    await expect(back).toHaveAttribute('href', source.replace(consoleURL, ''));
  }
  await page.reload();
  await expect(back).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('workspace-return.png'), fullPage: true });
  await back.focus();
  await expect(back).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(source);
  await expect(page.getByRole('heading', { name: '账号管理', exact: true })).toBeVisible();
  await expect(back).toHaveCount(0);
  await page.goBack();
  await expect(page.getByRole('heading', { name: '账号首页', exact: true })).toBeVisible();
  await page.getByRole('link', { name: /^返回工作台/ }).click({ timeout: 5000 });
  await expect(page).toHaveURL(source);
  await expect(page.getByRole('heading', { name: '账号管理', exact: true })).toBeVisible();
});

test('tenant and project monitoring return survives notifications, settings and refresh', async ({ page, request }) => {
  const f = await prepareWorkspaces(request, false);
  await login(page, f.account);
  for (const [base, name] of [[`/t/${f.tenantIds[0]}`, '交通监测中心'], [`/t/${f.tenantIds[1]}/p/${f.projectIds[1]}`, '隧道运行监测']]) {
    await page.goto(`${consoleURL}${base}/monitoring?view=history&from=2026-11-01T00%3A00%3A00Z#observations`);
    await expect(page.getByRole('heading', { name: '监测浏览', exact: true })).toBeVisible();
    const source = page.url();
    await page.getByRole('button', { name: /^通知，/ }).click();
    await page.getByRole('link', { name: '查看全部通知', exact: true }).click();
    const back = page.getByRole('link', { name: /^返回工作台/ });
    await expect(back).toHaveAttribute('href', source.replace(consoleURL, ''));
    await expect(back).toHaveAccessibleName(`返回工作台：${name} · 监测浏览`);
    await page.getByRole('button', { name: /^通知，/ }).click();
    await page.getByRole('link', { name: '通知接收设置', exact: true }).click();
    await expect(page).toHaveURL(`${consoleURL}/notifications/preferences`);
    await page.getByRole('link', { name: /^任务中心/ }).click();
    await expect(page.getByRole('heading', { name: '任务中心', exact: true })).toBeVisible();
    await page.reload();
    await expect(back).toHaveAttribute('href', source.replace(consoleURL, ''));
    await back.click();
    await expect(page).toHaveURL(source);
    await expect(page.getByRole('heading', { name: '监测浏览', exact: true })).toBeVisible();
  }
});

test('direct global entry has a workspace picker and separate tabs keep separate origins', async ({ page, context, request }) => {
  const f = await prepareWorkspaces(request, false);
  await login(page, f.account);
  await expect(page.getByRole('link', { name: /^返回工作台/ })).toHaveCount(0);
  await page.getByRole('navigation', { name: '工作台返回导航' }).getByRole('button', { name: '选择工作空间', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '切换工作空间', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  const other = await context.newPage();
  const firstSource = `/t/${f.tenantIds[0]}/profile`;
  const secondSource = `/t/${f.tenantIds[1]}/p/${f.projectIds[1]}/profile`;
  await page.goto(consoleURL + firstSource);
  await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
  await other.goto(consoleURL + secondSource);
  await expect(other.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  for (const current of [page, other]) await navigateTo(current, '个人资料');
  await page.reload();
  await other.reload();
  await expect(page.getByRole('link', { name: /^返回工作台/ })).toHaveAttribute('href', firstSource);
  await expect(other.getByRole('link', { name: /^返回工作台/ })).toHaveAttribute('href', secondSource);
  await other.close();
});

test('account changes and untrusted stored targets do not reuse another workspace', async ({ page, request }) => {
  test.setTimeout(60_000);
  const other = await createAccount(request, false);
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/access/accounts`);
  await navigateTo(page, '个人资料');
  await expect(page.getByRole('link', { name: /^返回工作台/ })).toBeVisible();
  const key = await page.evaluate(() => Object.keys(sessionStorage).find(key => key.startsWith('infrapro:workspace-return:'))!);
  for (const href of ['https://example.com/', '//example.com/', '/\\example.com/', '/tasks', '/login', '/not-a-workspace']) {
    await page.evaluate(({ key, href }) => sessionStorage.setItem(key, JSON.stringify({ href, label: '无效来源' })), { key, href });
    await page.reload();
    await expect(page.getByRole('heading', { name: '个人资料', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: /^返回工作台/ })).toHaveCount(0);
  }
  await page.goto(`${consoleURL}/access/accounts`);
  await navigateTo(page, '个人资料');
  await expect(page.getByRole('link', { name: /^返回工作台/ })).toBeVisible();
  await logout(page);
  await expect(page.getByRole('heading', { name: '欢迎回来', exact: true })).toBeVisible();
  await login(page, other);
  await navigateTo(page, '个人资料');
  await expect(page.getByRole('heading', { name: '个人资料', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: /^返回工作台/ })).toHaveCount(0);
});

test('return still works in memory when browser storage is disabled', async ({ page }) => {
  await page.addInitScript(() => {
    const read = Storage.prototype.getItem, write = Storage.prototype.setItem;
    Storage.prototype.getItem = function (key) { if (key.startsWith('infrapro:workspace-return:')) throw new Error('Storage disabled'); return read.call(this, key); };
    Storage.prototype.setItem = function (key, value) { if (key.startsWith('infrapro:workspace-return:')) throw new Error('Storage disabled'); write.call(this, key, value); };
  });
  await login(page, { username: 'e2e_owner', password });
  await page.goto(`${consoleURL}/access/roles`);
  await expect(page.getByRole('heading', { name: '角色管理', exact: true })).toBeVisible();
  await page.getByRole('button', { name: /搜索页面/ }).click();
  await page.getByRole('option', { name: '个人资料', exact: true }).click();
  await expect(page.getByRole('heading', { name: '个人资料', exact: true })).toBeVisible();
  await page.getByRole('link', { name: /^任务中心/ }).click();
  await page.getByRole('link', { name: /^返回工作台/ }).click();
  await expect(page).toHaveURL(`${consoleURL}/access/roles`);
});

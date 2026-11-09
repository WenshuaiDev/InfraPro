import { logout, navigateTo, expectIdentity } from '../helpers/navigation';
import { type Page } from '@playwright/test';
import { test, expect, createAccount, login, consoleURL } from './fixtures';

async function revisit(page: Page) {
  // Headless tabs do not consistently emit native focus events when brought forward.
  const refreshed = page.waitForResponse((response) => response.url().endsWith('/api/v1/auth/me'));
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  expect((await refreshed).status()).toBe(200);
}

test('switching accounts clears profile drafts, invitation credentials and security state', async ({ page, context, request, account }) => {
  const nextAccount = await createAccount(request);
  await login(page, account);
  await navigateTo(page, '个人资料');
  await page.getByLabel('显示名称', { exact: true }).fill('前一账号尚未保存的资料');

  const invitations = await context.newPage();
  await invitations.goto(`${consoleURL}/invitations`);
  await invitations.getByLabel('用途备注').fill('前一账号的专属邀请');
  await invitations.getByRole('button', { name: '生成邀请链接', exact: true }).click();
  await expect(invitations.getByLabel('可手动复制的完整链接')).toBeVisible();
  await expect(invitations.getByText('前一账号的专属邀请', { exact: true })).toBeVisible();

  const security = await context.newPage();
  await security.goto(`${consoleURL}/security`);
  await expect(security.getByText('创建邀请', { exact: true })).toBeVisible();
  await security.getByLabel('当前密码', { exact: true }).fill(account.password);
  await security.getByLabel('新密码', { exact: true }).fill('前一账号的新密码草稿 2026');
  await security.getByLabel('确认新密码', { exact: true }).fill('前一账号的新密码草稿 2026');

  const other = await context.newPage();
  await other.goto(consoleURL);
  await logout(other);
  await expect(other.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await login(other, nextAccount);

  await revisit(page);
  await expectIdentity(page, nextAccount.username, '浏览器验收');
  await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('浏览器验收');

  await revisit(invitations);
  await expectIdentity(invitations, nextAccount.username, '浏览器验收');
  await expect(invitations.getByLabel('可手动复制的完整链接')).toHaveCount(0);
  await expect(invitations.getByLabel('用途备注')).toHaveValue('');
  await expect(invitations.getByText('还没有邀请。生成链接后会在这里显示状态。')).toBeVisible();
  await expect(invitations.getByText('前一账号的专属邀请', { exact: true })).toHaveCount(0);

  await revisit(security);
  await expectIdentity(security, nextAccount.username, '浏览器验收');
  await expect(security.getByLabel('当前密码', { exact: true })).toHaveValue('');
  await expect(security.getByLabel('新密码', { exact: true })).toHaveValue('');
  await expect(security.getByLabel('确认新密码', { exact: true })).toHaveValue('');
  await expect(security.getByRole('button', { name: '退出当前会话', exact: true })).toBeVisible();
  await expect(security.getByText('创建邀请', { exact: true })).toHaveCount(0);
});

test('current-session logout revokes a replacement cookie and keeps other devices signed in', async ({ page, context, browser, request, account }) => {
  await login(page, account);
  const otherDevice = await browser.newContext();
  try {
    const devicePage = await otherDevice.newPage();
    await login(devicePage, account);
    await navigateTo(page, '账号安全');
    await expect(page.getByRole('button', { name: '退出当前会话', exact: true })).toBeVisible();

    const otherTab = await context.newPage();
    await otherTab.goto(consoleURL);
    await logout(otherTab);
    await expect(otherTab.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await login(otherTab, account);
    await revisit(page);
    const currentCookie = (await context.cookies()).find((cookie) => cookie.name === 'infrapro_session');
    if (!currentCookie) throw new Error('The replacement session cookie is missing');

    await page.getByRole('button', { name: '退出当前会话', exact: true }).click();
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    expect((await context.request.get(`${consoleURL}/api/v1/auth/me`)).status()).toBe(401);
    // Replaying the credential must fail too, independently of the cleared browser cookie.
    expect((await request.get(`${consoleURL}/api/v1/auth/me`, {
      headers: { Cookie: `${currentCookie.name}=${currentCookie.value}` },
    })).status()).toBe(401);
    await page.reload();
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await otherTab.reload();
    await expect(otherTab.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    expect((await otherDevice.request.get(`${consoleURL}/api/v1/auth/me`)).status()).toBe(200);
  } finally {
    await otherDevice.close();
  }
});

test('a late profile save cannot restore the previous account after a tab switches identity', async ({ page, context, request, account }) => {
  const nextAccount = await createAccount(request);
  await login(page, account);
  await navigateTo(page, '个人资料');
  await page.getByLabel('显示名称', { exact: true }).fill('前一账号已保存的资料');

  let releaseResponse = () => {};
  let notifySaved = () => {};
  const held = new Promise<void>((resolve) => { releaseResponse = resolve; });
  const saved = new Promise<void>((resolve) => { notifySaved = resolve; });
  await page.route('**/api/v1/account/profile', async (route) => {
    if (route.request().method() !== 'PATCH') { await route.continue(); return; }
    const response = await route.fetch();
    notifySaved();
    await held;
    await route.fulfill({ response });
  });
  try {
    await page.getByRole('button', { name: '保存资料', exact: true }).click();
    await saved;
    const otherTab = await context.newPage();
    await otherTab.goto(consoleURL);
    await logout(otherTab);
    await expect(otherTab.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await login(otherTab, nextAccount);
    await revisit(page);
    await expectIdentity(page, nextAccount.username, '浏览器验收');

    const delivered = page.waitForResponse((response) => response.url().endsWith('/api/v1/account/profile') && response.request().method() === 'PATCH');
    releaseResponse();
    await (await delivered).finished();
    // Let the delivered response render without triggering another identity reload.
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await expectIdentity(page, nextAccount.username, '浏览器验收');
    await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('浏览器验收');
  } finally {
    releaseResponse();
    await page.unrouteAll({ behavior: 'wait' });
  }
});

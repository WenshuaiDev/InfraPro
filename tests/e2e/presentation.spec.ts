import { test, expect, login, consoleURL } from './fixtures';
import { navigateTo } from '../helpers/navigation';

test('real pages share responsive layouts, keyboard navigation and themes', async ({ page, account }, info) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.emulateMedia({ colorScheme: 'light' });
  await page.goto(`${consoleURL}/login`);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.screenshot({ path: info.outputPath('login-light.png'), fullPage: true });
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByLabel('用户名', { exact: true })).toBeFocused();
  await expect(page.getByRole('alert')).toHaveCount(2);
  await page.screenshot({ path: info.outputPath('login-errors.png'), fullPage: true });
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveClass('dark');
  await page.screenshot({ path: info.outputPath('login-dark.png'), fullPage: true });
  await login(page, account);
  for (const title of ['个人资料', '账号安全', '账号开户邀请']) {
    await navigateTo(page, title);
    await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
    if (title === '账号安全') await expect(page.getByRole('button', { name: '退出当前会话' })).toBeVisible();
    if (title === '账号开户邀请') await expect(page.getByRole('heading', { name: '从第一条邀请开始' })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: info.outputPath(`${title}-dark.png`), fullPage: true });
  }
  await page.keyboard.press('ControlOrMeta+k');
  await page.getByRole('combobox').fill('个人资料');
  await page.keyboard.press('Enter');
  await expect(page.getByRole('button', { name: '保存资料', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '浅色', exact: true }).click();
  await page.reload();
  await expect(page.locator('html')).not.toHaveClass(/dark/);
  await expect(page.getByRole('button', { name: '保存资料', exact: true })).toBeVisible();
  await page.goto(`${consoleURL}/missing-page`);
  await expect(page.getByRole('heading', { name: '页面未找到' })).toBeVisible();
  await page.getByRole('link', { name: '返回首页', exact: true }).click();
  await expect(page.getByRole('heading', { name: '账号首页' })).toBeVisible();
  expect(errors).toEqual([]);
});

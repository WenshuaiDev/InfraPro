import { test, expect, login } from './fixtures';
import { navigateTo } from '../helpers/navigation';

test('failed route chunks preserve the signed-in layout and recover on reload', async ({ page, account }, info) => {
  test.skip(!process.env.INFRAPRO_E2E_PREVIEW, 'Checks the production code-split asset, not the Vite source server.');
  await login(page, account);
  await page.route('**/assets/profile-*.js', (route) => route.abort('failed'));
  await navigateTo(page, '个人资料');
  await expect(page.getByRole('heading', { name: '页面暂时无法加载', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('route-error.png'), fullPage: true });
  await page.unroute('**/assets/profile-*.js');
  await page.getByRole('button', { name: '重新加载页面', exact: true }).click();
  await expect(page.getByRole('button', { name: '保存资料', exact: true })).toBeVisible();
});

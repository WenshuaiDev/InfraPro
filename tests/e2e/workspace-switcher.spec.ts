import { browseWorkspaceTenant, openWorkspacePicker } from '../helpers/navigation';
import { prepareWorkspaces as prepare } from '../helpers/workspaces';
import type { Page } from '@playwright/test';
import { test, expect, login, consoleURL } from './fixtures';

async function sidebar(page: Page) {
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  // Desktop navigation can briefly unmount while the new workspace loads.
  // Only mobile navigation needs its drawer opened; wait for desktop content.
  if ((page.viewportSize()?.width ?? 1440) < 768 && !await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  await expect(navigation).toBeVisible();
  return navigation;
}
const switcher = openWorkspacePicker;

test('workspace hierarchy switches routes and isolates platform, tenant and project navigation', async ({ page, request, context }, info) => {
  const f = await prepare(request);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await login(page, f.account);
  // Anonymous identity probes can return 401 during login; inspect the actual
  // workspace flow for console errors after authentication has completed.
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
  let menu = await switcher(page);
  await browseWorkspaceTenant(page, '交通监测中心');
  const tenantGroup = menu.getByRole('group', { name: '交通监测中心', exact: true });
  await expect(tenantGroup.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
  await expect(tenantGroup.getByRole('button', { name: /^隧道运行监测/ })).toHaveCount(0);
  await browseWorkspaceTenant(page, '城市基础设施中心');
  await expect(menu.getByRole('group', { name: '城市基础设施中心', exact: true }).getByRole('button', { name: /^隧道运行监测/ })).toBeVisible();
  await browseWorkspaceTenant(page, '交通监测中心');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('workspace-groups-light.png'), fullPage: true });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: info.outputPath('workspace-groups-dark.png'), fullPage: true });
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  await tenantGroup.getByRole('button', { name: /^桥梁健康监测/ }).click();
  if (info.project.name === 'mobile') await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  const projectURL = `${consoleURL}/t/${f.tenantIds[0]}/p/${f.projectIds[0]}/profile`;
  await expect(page).toHaveURL(projectURL);
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  let nav = await sidebar(page);
  await expect(nav.getByRole('link', { name: '项目成员', exact: true })).toBeVisible();
  for (const title of ['账号开户邀请', '租户管理', '租户资料', '项目治理']) await expect(nav.getByRole('link', { name: title, exact: true })).toHaveCount(0);
  if (info.project.name === 'mobile') await page.getByRole('button', { name: '关闭导航', exact: true }).click();
  await page.getByRole('button', { name: /搜索页面/ }).click();
  const search = page.getByRole('dialog', { name: '搜索页面', exact: true });
  await expect(search.getByRole('option', { name: '项目资料', exact: true })).toBeVisible();
  for (const title of ['账号开户邀请', '租户资料', '项目治理']) await expect(search.getByRole('option', { name: title, exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await page.reload();
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  menu = await switcher(page);
  await expect(menu.getByRole('button', { name: /^桥梁健康监测/ })).toHaveAttribute('aria-current', 'true');
  await menu.getByRole('button', { name: /^交通监测中心 租户/ }).click();
  if (info.project.name === 'mobile') await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  await expect(page).toHaveURL(`${consoleURL}/t/${f.tenantIds[0]}/profile`);
  nav = await sidebar(page);
  await expect(nav.getByRole('link', { name: '项目治理', exact: true })).toBeVisible();
  for (const title of ['账号开户邀请', '租户管理', '项目成员']) await expect(nav.getByRole('link', { name: title, exact: true })).toHaveCount(0);
  menu = await switcher(page);
  await menu.getByRole('button', { name: /^平台工作空间/ }).click();
  if (info.project.name === 'mobile') await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  await expect(page).toHaveURL(`${consoleURL}/invitations`);
  nav = await sidebar(page);
  await expect(nav.getByRole('link', { name: '账号开户邀请', exact: true })).toBeVisible();
  for (const title of ['租户资料', '项目治理', '项目成员']) await expect(nav.getByRole('link', { name: title, exact: true })).toHaveCount(0);
  if (info.project.name === 'mobile') await page.getByRole('button', { name: '关闭导航', exact: true }).click();
  await page.goBack();
  await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL(projectURL);
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  const other = await context.newPage();
  await other.goto(`${consoleURL}/t/${f.tenantIds[1]}/p/${f.projectIds[1]}/profile`);
  await expect(other.getByRole('main').getByText('隧道运行监测', { exact: true })).toBeVisible();
  await expect(page.getByRole('main').getByText('桥梁健康监测', { exact: true })).toBeVisible();
  await other.close();
  if (info.project.name === 'desktop') {
    await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
    menu = await switcher(page);
    await expect(menu.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
    await page.getByRole('button', { name: /^切换工作空间：/ }).press('Enter');
    await page.getByRole('dialog', { name: '切换工作空间', exact: true }).press('End');
    await expect(page.getByRole('button', { name: /^平台工作空间/ })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(`${consoleURL}/invitations`);
  }
  expect(errors).toEqual([]);
});

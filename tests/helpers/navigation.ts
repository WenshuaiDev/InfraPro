import { expect, type Page } from '@playwright/test';

export async function openWorkspacePicker(page: Page, filter?: 'tenants' | 'projects') {
  // Reload first restores identity; absence during that phase does not imply a mobile sidebar.
  await expect(page.getByRole('button', { name: '账号菜单', exact: true, includeHidden: true })).toBeVisible();
  const trigger = page.getByRole('button', { name: /^切换工作空间：/ });
  if (!await trigger.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  await trigger.click();
  const menu = page.getByRole('menu', { name: '切换工作空间', exact: true });
  await expect(menu).toBeVisible();
  if (filter) await menu.getByLabel('工作空间类型').selectOption(filter);
  return menu;
}

export async function navigateTo(page: Page, name: string) {
  if (['账号首页', '个人资料', '账号安全'].includes(name)) {
    await page.getByRole('button', { name: '账号菜单', exact: true }).click();
    await page.getByRole('menuitem', { name, exact: true }).click();
    return;
  }
  if (['切换租户', '切换项目', '我的租户', '我的项目'].includes(name)) {
    await openWorkspacePicker(page, name.endsWith('租户') ? 'tenants' : 'projects');
    return;
  }
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  const platform = ['租户管理', '账号管理', '账号开户邀请', '角色管理', '平台审计'].includes(name);
  if (platform && !await navigation.getByRole('link', { name, exact: true }).count()) {
    const menu = await openWorkspacePicker(page);
    await menu.getByRole('menuitem', { name: /^平台工作空间/ }).click();
    await expect(page.getByRole('menu', { name: '切换工作空间', exact: true })).toHaveCount(0);
    await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  }
  if ((page.viewportSize()?.width ?? 1440) < 768 && !await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  await navigation.getByRole('link', { name, exact: true }).click();
}

export async function logout(page: Page) {
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  await page.getByRole('menuitem', { name: '退出登录', exact: true }).click();
}

export async function expectIdentity(page: Page, username: string, displayName: string) {
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  const menu = page.getByRole('menu');
  await expect(menu.getByText(username, { exact: true })).toBeVisible();
  await expect(menu.getByText(displayName, { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
}

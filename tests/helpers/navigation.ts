import { expect, type Page } from '@playwright/test';

export async function navigateTo(page: Page, name: string) {
  if (['账号首页', '个人资料', '账号安全'].includes(name)) {
    await page.getByRole('button', { name: '账号菜单', exact: true }).click();
    await page.getByRole('menuitem', { name, exact: true }).click();
    return;
  }
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  if (!await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  if (name === '切换租户' || name === '切换项目') {
    await page.getByRole('button', { name: /^切换工作空间：/ }).click();
    await page.getByRole('menuitem', { name: name === '切换租户' ? /^我的租户/ : /^我的项目/ }).click();
    return;
  }
  if (['租户管理', '账号管理', '账号开户邀请', '角色管理', '平台审计'].includes(name) && !await navigation.getByRole('link', { name, exact: true }).isVisible()) {
    const mobileSheet = page.getByRole('dialog', { name: '主导航', exact: true });
    const mobile = await mobileSheet.isVisible();
    await page.getByRole('button', { name: /^切换工作空间：/ }).click();
    await page.getByRole('menuitem', { name: /^平台工作空间/ }).click();
    if (mobile) await expect(mobileSheet).toBeHidden();
    if (!await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  }
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

import { expect, type Page } from '@playwright/test';

export async function navigateTo(page: Page, name: string) {
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  if (!await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
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

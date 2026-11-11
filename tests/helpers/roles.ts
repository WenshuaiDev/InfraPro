import { expect, type Page } from '@playwright/test';

export async function submitPlatformRole(page: Page, reason: string, label: string) {
  const panel = page.getByRole('dialog');
  await panel.getByLabel('变更原因', { exact: true }).fill(reason);
  await panel.getByRole('button', { name: label, exact: true }).click();
  await expect(panel.getByLabel('变更原因', { exact: true })).toHaveCount(0);
}

export async function roleAction(page: Page, name: string) {
  await page.getByRole('dialog').getByRole('button', { name: /的操作$/ }).click();
  await page.getByRole('menuitem', { name, exact: true }).click();
}

export async function closeRolePanel(page: Page) {
  const panel = page.getByRole('dialog');
  await panel.getByRole('button', { name: /^关闭(面板)?$/ }).click();
  await expect(panel).toHaveCount(0);
}

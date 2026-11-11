import { test, expect } from '@playwright/test';

test('layout navigation, theme persistence, charts, states and dialog focus', async ({ page }, info) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto('/ui-preview.html');
  await expect(page.getByRole('heading', { name: '首页', exact: true })).toBeVisible();
  await expect(page.locator('.recharts-surface')).toHaveCount(5);
  const line = page.getByRole('figure', { name: 'line 图表', exact: true });
  await expect(line.locator('.recharts-legend-wrapper')).toContainText('测试系列一');
  await expect(line.locator('.recharts-legend-wrapper')).toContainText('测试系列二');
  await line.locator('.recharts-surface').focus();
  await page.keyboard.press('ArrowRight');
  await expect(line.locator('.recharts-tooltip-wrapper')).toBeVisible();
  await expect(line.locator('.recharts-tooltip-wrapper')).toContainText('测试系列一');
  await page.getByRole('heading', { name: '首页', exact: true }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('layout-light.png'), fullPage: true });

  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '深色' }).click();
  await expect(page.locator('html')).toHaveClass('dark');
  await page.reload();
  await expect(page.locator('html')).toHaveClass('dark');
  await page.screenshot({ path: info.outputPath('layout-dark.png'), fullPage: true });

  if (info.project.name === 'mobile') {
    await page.getByRole('banner').getByRole('button', { name: '切换侧栏' }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await page.getByRole('navigation', { name: '主导航' }).getByRole('link', { name: '设置', exact: true }).click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: '设置内容' })).toBeVisible();
  } else {
    await page.getByRole('banner').getByRole('button', { name: '切换侧栏' }).click();
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'collapsed');
    await page.reload();
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'collapsed');
    await page.getByRole('button', { name: '重置布局偏好' }).click();
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'expanded');
  }
  await page.getByRole('button', { name: /搜索页面/ }).click();
  await page.getByRole('combobox').fill('图表');
  await page.getByRole('option', { name: '图表组件' }).click();
  await expect(page.getByRole('heading', { name: '图表组件' })).toBeVisible();
  await page.keyboard.press('ControlOrMeta+k');
  await expect(page.getByRole('combobox')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: /搜索页面/ })).toBeFocused();
  await page.getByRole('button', { name: '空数据', exact: true }).click();
  await expect(page.getByText('暂无图表数据')).toHaveCount(5);
  await page.getByRole('button', { name: '加载中', exact: true }).click();
  await expect(page.getByRole('status')).toHaveCount(5);
  await page.getByRole('button', { name: '错误', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveCount(5);
  await page.getByRole('button', { name: '重新加载' }).first().click();
  await expect(page.locator('.recharts-surface')).toHaveCount(5);
  await page.getByRole('button', { name: '测试确认弹窗' }).click();
  await expect(page.getByRole('alertdialog')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('alertdialog')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '测试确认弹窗' })).toBeFocused();
  await page.emulateMedia({ colorScheme: 'light' });
  await page.getByRole('button', { name: '切换主题', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '跟随系统' }).click();
  await expect(page.locator('html')).not.toHaveClass(/dark/);
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveClass('dark');
  expect(errors).toEqual([]);
});

test('auth layout and password visibility work at every viewport', async ({ page }, info) => {
  await page.goto('/ui-preview.html?view=auth');
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  const input = page.getByLabel('密码', { exact: true });
  await input.fill('test-only password');
  await page.getByRole('button', { name: '显示密码', exact: true }).click();
  await expect(input).toHaveAttribute('type', 'text');
  await page.getByRole('button', { name: '隐藏密码', exact: true }).click();
  await expect(input).toHaveAttribute('type', 'password');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('auth.png'), fullPage: true });
});

test('plain shared layout works without sidebar context and has one account menu', async ({ page }) => {
  await page.goto('/ui-preview.html?view=plain');
  await expect(page.getByRole('heading', { name: '首页', exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="sidebar-wrapper"], [data-slot="sidebar"], [data-slot="sidebar-trigger"]')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toHaveCount(1);
  await expect(page.getByRole('button', { name: '侧栏账号菜单' })).toHaveCount(0);
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  await page.getByRole('menuitem', { name: '设置', exact: true }).click();
  await expect(page.getByRole('heading', { name: '设置内容', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  await page.getByRole('menuitem', { name: '退出登录', exact: true }).click();
  await expect(page.getByRole('status')).toHaveText('操作已确认');
  await page.getByRole('button', { name: /搜索页面/ }).click();
  await expect(page.getByRole('option', { name: '设置', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

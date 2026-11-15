import { expect, type Page } from '@playwright/test';

export const workspaceTypeLabels = { all: '全部类型', tenants: '租户', projects: '项目' } as const;
export async function selectWorkspaceType(page: Page, filter: keyof typeof workspaceTypeLabels) {
  await page.getByRole('dialog', { name: '切换工作空间', exact: true }).getByRole('combobox', { name: '工作空间类型', exact: true }).click();
  await page.getByRole('option', { name: workspaceTypeLabels[filter], exact: true }).click();
}

export async function openWorkspacePicker(page: Page, filter?: 'tenants' | 'projects') {
  // Reload first restores identity; absence during that phase does not imply a mobile sidebar.
  await expect(page.getByRole('button', { name: '账号菜单', exact: true, includeHidden: true })).toBeVisible();
  const trigger = page.getByRole('button', { name: /^切换工作空间：/ });
  if ((page.viewportSize()?.width ?? 1440) < 768) {
    const sidebar = page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true });
    // A closing modal still hides the page from accessibility queries. Wait for
    // its focus/visibility handoff before deciding whether the sidebar is needed.
    await expect(trigger.or(sidebar).first()).toBeVisible();
    if (!await trigger.isVisible()) await sidebar.click();
  }
  await trigger.click();
  const menu = page.getByRole('dialog', { name: '切换工作空间', exact: true });
  await expect(menu).toBeVisible();
  if (filter) await selectWorkspaceType(page, filter);
  return menu;
}

export async function browseWorkspaceTenant(page: Page, name: string) {
  const dialog = page.getByRole('dialog', { name: '切换工作空间', exact: true });
  await expect(dialog).toBeVisible();
  const mobile = dialog.getByLabel('选择租户', { exact: true });
  if ((page.viewportSize()?.width ?? 1440) < 768) await mobile.selectOption({ label: name });
  else await dialog.getByRole('button', { name: `浏览租户：${name}`, exact: true }).click();
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
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true, includeHidden: true });
  const platform = ['租户管理', '账号管理', '账号开户邀请', '角色管理', '平台审计'].includes(name);
  if ((page.viewportSize()?.width ?? 1440) < 768) {
    const sidebar = page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true });
    await expect.poll(async () => await sidebar.isVisible() || await navigation.isVisible() || await page.getByRole('button', { name: /^(进入工作空间：|切换工作空间：)/ }).isVisible()).toBe(true);
    if (!await navigation.isVisible() && await sidebar.isVisible()) {
      await sidebar.click();
      await expect(navigation).toBeVisible();
    }
  }
  if (platform) await expect.poll(async () => await navigation.getByRole('link', { name, exact: true, includeHidden: true }).count() + await page.getByRole('button', { name: /^(进入工作空间：平台工作空间|切换工作空间：)/, includeHidden: true }).count()).toBeGreaterThan(0);
  if (platform && !await navigation.getByRole('link', { name, exact: true, includeHidden: true }).count()) {
    const single = page.getByRole('button', { name: '进入工作空间：平台工作空间', exact: true });
    if (await single.isVisible()) await single.click();
    else { const menu = await openWorkspacePicker(page); await menu.getByRole('button', { name: /^平台工作空间/ }).click(); }
    await expect(page.getByRole('dialog', { name: '切换工作空间', exact: true })).toHaveCount(0);
    await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  }
  if ((page.viewportSize()?.width ?? 1440) < 768 && !await navigation.isVisible()) {
    await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
    await expect(navigation).toBeVisible();
  }
  const section = ({ '监测浏览': '监测数据', '导入与批次': '监测数据', '监测任务': '监测数据', '派生指标定义': '监测数据', '派生通道': '监测数据', '历史分析': '监测数据', '计算运行': '监测数据', '设施管理': '工程资源', '项目资源授权': '工程资源', '设备产品目录': '设备管理', '设备台账': '设备管理', '设施类型定义': '工程配置', '指标目录': '工程配置', '单位目录': '工程配置', '公共监测模板': '工程配置' } as Record<string, string>)[name];
  if (section) {
    const trigger = navigation.getByRole('button', { name: section, exact: true });
    await expect(trigger.or(navigation.getByRole('link', { name, exact: true })).first()).toBeVisible();
    // A single authorized destination is flat; multiple destinations use a
    // collapsible section or, in icon mode, a menu beside the sidebar.
    if (await trigger.count()) {
      if (await trigger.getAttribute('aria-haspopup') === 'menu') {
        await trigger.click();
        await page.getByRole('menu').getByRole('menuitem', { name, exact: true }).click();
        return;
      }
      if (await trigger.getAttribute('aria-expanded') === 'false') await trigger.click();
    }
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

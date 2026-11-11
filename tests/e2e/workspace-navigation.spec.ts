import type { Page } from '@playwright/test';
import { test, expect, login, createAccount, consoleURL } from './fixtures';
import { navigateTo, openWorkspacePicker, logout } from '../helpers/navigation';
import { prepareWorkspaces } from '../helpers/workspaces';

const picker = (page: Page) => page.getByRole('menu', { name: '切换工作空间', exact: true });
async function expectPlain(page: Page) {
  await expect(page.locator('[data-slot="sidebar-wrapper"], [data-slot="sidebar"], [data-slot="sidebar-gap"], [data-slot="sidebar-rail"], [data-slot="sidebar-trigger"]')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toHaveCount(1);
  await expect(page.getByRole('button', { name: '侧栏账号菜单' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeVisible();
}

test('plain account pages retain content and all toolbar actions without sidebar preferences', async ({ page, account }, info) => {
  await page.addInitScript(() => {
    localStorage.setItem('infrapro:layout', JSON.stringify({ state: { sidebarOpen: false }, version: 1 }));
    const read = Storage.prototype.getItem;
    Object.assign(window, { sidebarReads: 0 });
    Storage.prototype.getItem = function (key) { if (key === 'infrapro:layout') Reflect.set(window, 'sidebarReads', Number(Reflect.get(window, 'sidebarReads')) + 1); return read.call(this, key); };
  });
  await login(page, account);
  await expect(page).toHaveURL(`${consoleURL}/`);
  await expect(page.getByRole('heading', { name: '账号首页', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: /我的租户/ })).toBeVisible();
  await expectPlain(page);
  expect(await page.evaluate(() => Reflect.get(window, 'sidebarReads'))).toBe(0);
  const preference = await page.evaluate(() => localStorage.getItem('infrapro:layout'));
  await page.keyboard.press('ControlOrMeta+b');
  expect(await page.evaluate(() => localStorage.getItem('infrapro:layout'))).toBe(preference);
  await page.getByRole('button', { name: /搜索页面/ }).click();
  for (const name of ['账号首页', '个人资料', '账号安全']) await expect(page.getByRole('option', { name, exact: true })).toBeVisible();
  for (const name of ['我的租户', '我的项目']) await expect(page.getByRole('option', { name, exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  for (const name of ['个人资料', '账号安全', '账号首页']) {
    await navigateTo(page, name);
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
    await expectPlain(page);
  }
  for (const theme of ['浅色', '深色']) {
    await page.getByRole('button', { name: '切换主题', exact: true }).click();
    await page.getByRole('menuitemradio', { name: theme, exact: true }).click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`account-${theme}.png`), fullPage: true });
  }
  await page.route('**/api/v1/auth/logout', (route) => route.fulfill({ status: 503, json: { code: 'unavailable', message: '退出暂时失败，请重试' } }));
  await logout(page);
  await expect(page.getByRole('alert')).toContainText('退出暂时失败');
  await expectPlain(page);
  await page.unroute('**/api/v1/auth/logout');
  await logout(page);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
});

test('legacy aliases consume their intent once, preserve history and keep the home card', async ({ page, account }) => {
  await login(page, account);
  for (const filter of ['tenants', 'projects']) {
    await page.goto(`${consoleURL}/${filter}`);
    await expect(picker(page)).toBeVisible();
    await expect(picker(page).getByLabel('工作空间类型')).toHaveValue(filter);
    await expect(page).toHaveURL(`${consoleURL}/`);
    await page.keyboard.press('Escape');
    await expect(picker(page)).toHaveCount(0);
    await expectPlain(page);
    await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
    await navigateTo(page, '个人资料');
    await page.goBack();
    await expect(page.getByRole('heading', { name: '账号首页' })).toBeVisible();
    await expect(picker(page)).toHaveCount(0);
    await page.goForward();
    await expect(page.getByRole('heading', { name: '个人资料', exact: true })).toBeVisible();
  }
  await navigateTo(page, '账号首页');
  await page.getByRole('link', { name: /我的租户/ }).click();
  await expect(picker(page).getByLabel('工作空间类型')).toHaveValue('tenants');
  await page.keyboard.press('Escape');
  await page.goto(`${consoleURL}/?workspace=invalid`);
  await expectPlain(page);
  await expect(picker(page)).toHaveCount(0);
});

for (const alias of ['tenants', 'projects']) test(`anonymous ${alias} alias survives login and opens only its requested source`, async ({ page, account }) => {
  const reads: string[] = [];
  page.on('request', (request) => { if (/\/api\/v1\/(account\/tenants|me\/projects)\?/.test(request.url())) reads.push(request.url()); });
  await page.goto(`${consoleURL}/${alias}`);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.getByLabel('用户名', { exact: true }).fill(account.username);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(picker(page).getByLabel('工作空间类型')).toHaveValue(alias);
  await expect(picker(page).getByText(alias === 'tenants' ? '尚未加入租户，请联系平台管理人员' : '当前没有可显示的项目。')).toBeVisible();
  await expect(page).toHaveURL(`${consoleURL}/`);
  expect(reads.length).toBeGreaterThan(0);
  expect(reads.every((url) => url.includes(alias === 'tenants' ? '/account/tenants?' : '/me/projects?'))).toBe(true);
});

test('business deep links, sidebar preference, body picker focus and same-target history', async ({ page, request }, info) => {
  const f = await prepareWorkspaces(request);
  const target = `${consoleURL}/t/${f.tenantIds[0]}/p/${f.projectIds[0]}/profile`;
  await page.goto(target);
  await page.getByLabel('用户名', { exact: true }).fill(f.account.username);
  await page.getByLabel('密码', { exact: true }).fill(f.account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page).toHaveURL(target);
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  if (info.project.name === 'desktop') {
    await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'collapsed');
    await navigateTo(page, '账号首页');
    await expectPlain(page);
    const menu = await openWorkspacePicker(page);
    await menu.getByRole('menuitem', { name: /^桥梁健康监测/ }).click();
    await expect(page).toHaveURL(target);
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'collapsed');
  }
  await page.getByRole('button', { name: '切换项目', exact: true }).click();
  await expect(picker(page).getByLabel('工作空间类型')).toHaveValue('projects');
  await expect(picker(page).getByLabel('搜索名称或编号')).toBeFocused();
  await picker(page).getByLabel('搜索名称或编号').fill('桥梁');
  await page.keyboard.press('ArrowLeft');
  await expect(picker(page).getByLabel('搜索名称或编号')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(picker(page)).toHaveCount(0);
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
  if (info.project.name === 'mobile') {
    await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeVisible();
    await page.getByRole('button', { name: '关闭导航', exact: true }).click();
  }
  await page.getByRole('button', { name: '切换项目', exact: true }).click();
  await expect(picker(page).getByLabel('搜索名称或编号')).toHaveValue('');
  const length = await page.evaluate(() => history.length);
  await picker(page).getByRole('menuitem', { name: /^桥梁健康监测/ }).click();
  await expect(picker(page)).toHaveCount(0);
  await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  expect(await page.evaluate(() => history.length)).toBe(length);
  await expect(page.getByRole('main')).toBeFocused();
  await page.getByRole('button', { name: '切换项目', exact: true }).click();
  await picker(page).getByRole('menuitem', { name: /^隧道运行监测/ }).click();
  await expect(page).toHaveURL(new RegExp(`${f.projectIds[1]}/profile$`));
  await expect(page.getByRole('main')).toBeFocused();
  await page.getByRole('button', { name: '编辑项目资料', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '编辑项目资料', exact: true })).toBeVisible();
});

test('picker searches the server beyond loaded pages and keeps tenant pages when project status changes', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  const api = new URL('/api/v1', consoleURL).href;
  const tenants = (await (await request.get(`${api}/account/tenants`)).json()).items;
  const projects = (await (await request.get(`${api}/me/projects?status=all`)).json()).items;
  const firstTenant = tenants.find((t: { id: string }) => t.id === f.tenantIds[0]);
  const laterTenant = tenants.find((t: { id: string }) => t.id === f.tenantIds[1]);
  const firstProject = projects.find((p: { project: { id: string } }) => p.project.id === f.projectIds[0]);
  const laterProject = projects.find((p: { project: { id: string } }) => p.project.id === f.projectIds[1]);
  const calls: URL[] = [];
  await page.route('**/api/v1/account/tenants?*', (route) => {
    const url = new URL(route.request().url()); calls.push(url);
    const q = url.searchParams.get('q'); const more = url.searchParams.get('cursor');
    return route.fulfill({ json: { items: q ? (q === laterTenant.code ? [laterTenant] : []) : more ? [laterTenant] : [firstTenant], next_cursor: q || more ? '' : 'tenant-page-2' } });
  });
  await page.route('**/api/v1/me/projects?*', (route) => {
    const url = new URL(route.request().url()); calls.push(url);
    const q = url.searchParams.get('q'); const more = url.searchParams.get('cursor');
    const archived = { ...laterProject, project: { ...laterProject.project, status: 'archived' } };
    return route.fulfill({ json: { items: q ? (q === laterProject.project.code || q === laterProject.project.name ? [archived] : []) : url.searchParams.get('status') === 'archived' ? [archived] : more ? [firstProject, archived] : [firstProject], next_cursor: q || more || url.searchParams.get('status') === 'archived' ? '' : 'project-page-2' } });
  });
  await login(page, f.account);
  const menu = await openWorkspacePicker(page);
  await menu.getByRole('menuitem', { name: '加载更多租户', exact: true }).click();
  await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心.*进入租户/ })).toBeVisible();
  await menu.getByRole('menuitem', { name: '加载更多项目', exact: true }).click();
  await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测/ })).toHaveCount(1);
  const tenantReads = calls.filter((u) => u.pathname.endsWith('/account/tenants')).length;
  await menu.getByLabel('项目状态').selectOption('archived');
  await expect(menu.getByRole('menuitem', { name: /^隧道运行监测 已归档/ })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测/ })).toHaveCount(0);
  await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心.*进入租户/ })).toBeVisible();
  expect(calls.filter((u) => u.pathname.endsWith('/account/tenants')).length).toBe(tenantReads);
  await menu.getByLabel('工作空间类型').selectOption('tenants');
  await expect(menu.getByLabel('项目状态')).toHaveCount(0);
  await menu.getByLabel('搜索名称或编号').fill(laterTenant.code);
  await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心/ })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: /^交通监测中心/ })).toHaveCount(0);
  expect(calls.some((u) => u.searchParams.get('q') === laterTenant.code && !u.searchParams.get('cursor'))).toBe(true);
  await menu.getByRole('button', { name: '清除', exact: true }).click();
  await menu.getByLabel('工作空间类型').selectOption('projects');
  for (const q of [laterProject.project.name, laterProject.project.code]) {
    await menu.getByLabel('搜索名称或编号').fill(q);
    await expect.poll(() => calls.some((u) => u.searchParams.get('q') === q && !u.searchParams.get('cursor'))).toBe(true);
    await expect(menu.getByRole('menuitem', { name: /^隧道运行监测 已归档/ })).toBeVisible();
  }
  await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心/ })).toHaveCount(0);
  expect(calls.every((u) => u.searchParams.get('limit') === '50')).toBe(true);
  await page.keyboard.press('Escape');
  const reopened = await openWorkspacePicker(page);
  await expect(reopened.getByLabel('搜索名称或编号')).toHaveValue('');
  await expect(reopened.getByLabel('工作空间类型')).toHaveValue('all');
  await expect(reopened.getByLabel('项目状态')).toHaveValue('all');
  await expect(reopened.getByRole('menuitem', { name: /^隧道运行监测/ })).toHaveCount(0);
});

test('unavailable sources and status actions retain independent access and recovery', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  const api = new URL('/api/v1', consoleURL).href;
  const list = (await (await request.get(`${api}/me/projects?status=all`)).json()).items;
  let failed = true;
  await page.route('**/api/v1/account/tenants?*', (route) => failed ? route.fulfill({ status: 503, json: { code: 'unavailable', message: '租户读取失败' } }) : route.continue());
  await page.route('**/api/v1/me/projects?*', (route) => route.fulfill({ json: { items: list.map((item: { authorization: object }) => ({ ...item, can_enter: false, authorization: { ...item.authorization, membership_status: 'suspended', inactive_reasons: ['membership_suspended'] } })), next_cursor: '' } }));
  await page.route(`**/api/v1/tenants/${f.tenantIds[0]}/projects/${f.projectIds[0]}/context`, (route) => route.fulfill({ status: 404, json: { code: 'project_not_found', message: '项目不可访问' } }));
  await login(page, f.account);
  const menu = await openWorkspacePicker(page);
  await expect(menu.getByRole('menuitem', { name: '租户加载失败，点击重试', exact: true })).toBeVisible();
  await expect(menu.getByText('尚未加入租户或项目。', { exact: true })).toHaveCount(0);
  await expect(menu.getByRole('menuitem', { name: /^交通监测中心/ })).toHaveCount(0);
  await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测.*项目成员已暂停.*查看本人状态/ })).toBeEnabled();
  failed = false;
  await menu.getByRole('menuitem', { name: '租户加载失败，点击重试', exact: true }).click();
  await expect(menu.getByRole('menuitem', { name: /^交通监测中心.*本人角色：.*进入租户/ })).toBeVisible();
  await menu.getByRole('menuitem', { name: /^桥梁健康监测.*查看本人状态/ }).click();
  await expect(page.getByRole('heading', { name: '当前项目不可访问', exact: true })).toBeVisible();
  const url = page.url();
  await page.getByRole('button', { name: '重新选择项目', exact: true }).click();
  await expect(picker(page).getByLabel('工作空间类型')).toHaveValue('projects');
  expect(page.url()).toBe(url);
  await picker(page).getByRole('menuitem', { name: /^隧道运行监测/ }).click();
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
});

test('768px breakpoint and keyboard picker remain usable in both themes', async ({ page, account }, info) => {
  test.skip(info.project.name !== 'desktop', 'Explicit tablet breakpoint coverage is independent of the presentation project.');
  await login(page, account);
  for (const width of [768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await expectPlain(page);
    for (const theme of ['浅色', '深色']) {
      await page.getByRole('button', { name: '切换主题', exact: true }).click();
      await page.getByRole('menuitemradio', { name: theme, exact: true }).click();
      await page.getByRole('button', { name: /^切换工作空间：/ }).press('Space');
      await expect(picker(page).getByLabel('搜索名称或编号')).toBeFocused();
      await picker(page).press('Home');
      await expect(picker(page).getByRole('menuitem', { name: /^平台工作空间/ })).toBeFocused();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: info.outputPath(`picker-${width}-${theme}.png`), fullPage: true });
      await page.keyboard.press('Escape');
    }
  }
});

test('late searches and an identity change cannot resurrect previous picker results', async ({ page, request, context }) => {
  const f = await prepareWorkspaces(request);
  const api = new URL('/api/v1', consoleURL).href;
  const list = await (await request.get(`${api}/account/tenants`)).json();
  const next = await createAccount(request, false);
  await login(page, f.account);
  let release!: () => void;
  let held = new Promise<void>((resolve) => { release = resolve; });
  let captured!: () => void;
  let capturedRequest = new Promise<void>((resolve) => { captured = resolve; });
  await page.route('**/api/v1/account/tenants?*', async (route) => {
    if (!new URL(route.request().url()).searchParams.get('q')?.startsWith('slow')) return route.continue();
    captured(); await held;
    await route.fulfill({ json: list }).catch(() => {});
  });
  try {
    let menu = await openWorkspacePicker(page, 'tenants');
    await expect(menu.getByRole('menuitem', { name: /^交通监测中心/ })).toBeVisible();
    await menu.getByLabel('搜索名称或编号').fill('slow');
    await capturedRequest;
    await menu.getByLabel('搜索名称或编号').fill('no-match-unique');
    await expect(menu.getByText('没有符合筛选条件的工作空间。')).toBeVisible();
    release();
    await menu.getByLabel('搜索名称或编号').fill('交通监测中心');
    await expect(menu.getByRole('menuitem', { name: /^交通监测中心/ })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心/ })).toHaveCount(0);
    held = new Promise<void>((resolve) => { release = resolve; });
    capturedRequest = new Promise<void>((resolve) => { captured = resolve; });
    await menu.getByLabel('搜索名称或编号').fill('slow-again');
    await capturedRequest;
    const other = await context.newPage();
    await other.goto(consoleURL);
    await logout(other);
    await expect(other.getByRole('heading', { name: '欢迎回来', exact: true })).toBeVisible();
    await login(other, next);
    const refreshed = page.waitForResponse((response) => response.url().endsWith('/auth/me'));
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await refreshed;
    await expect(picker(page)).toHaveCount(0);
    release();
    menu = await openWorkspacePicker(page);
    await expect(menu.getByText('尚未加入租户或项目。', { exact: true })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: /交通监测中心|城市基础设施中心/ })).toHaveCount(0);
    await other.close();
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }); }
});

test('tenant governance keeps creation, owner, management-gap, status filters and details', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  await login(page, f.account);
  await page.goto(`${consoleURL}/t/${f.tenantIds[0]}/projects`);
  await expect(page.getByRole('heading', { name: '项目治理', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: '创建项目', exact: true })).toHaveAttribute('href', `/t/${f.tenantIds[0]}/projects/new`);
  await expect(page.getByLabel('项目状态', { exact: true })).toHaveValue('active');
  await page.getByRole('button', { name: '按此负责人筛选', exact: true }).click();
  await expect(page.getByRole('button', { name: '清除负责人筛选', exact: true })).toBeVisible();
  await page.getByLabel('仅管理员不可用或空缺').check();
  await expect(page.getByText('当前没有可显示的项目', { exact: true })).toBeVisible();
  await page.getByLabel('仅管理员不可用或空缺').uncheck();
  await page.getByLabel('项目状态', { exact: true }).selectOption('archived');
  await expect(page.getByText('当前没有可显示的项目', { exact: true })).toBeVisible();
  await page.getByLabel('项目状态', { exact: true }).selectOption('all');
  await page.getByRole('button', { name: '清除负责人筛选', exact: true }).click();
  await page.getByRole('link', { name: '治理详情', exact: true }).click();
  await expect(page.getByRole('heading', { name: '桥梁健康监测', exact: true })).toBeVisible();
  await expect(page.getByText('这里只提供治理摘要，项目成员通过左上角工作空间切换器进入项目。', { exact: true })).toBeVisible();
});

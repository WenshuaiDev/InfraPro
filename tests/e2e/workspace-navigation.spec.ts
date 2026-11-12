import type { Page } from '@playwright/test';
import { test, expect, login, createAccount, consoleURL } from './fixtures';
import { browseWorkspaceTenant, navigateTo, openWorkspacePicker, logout, selectWorkspaceType, workspaceTypeLabels } from '../helpers/navigation';
import { prepareWorkspaces } from '../helpers/workspaces';

const picker = (page: Page) => page.getByRole('dialog', { name: '切换工作空间', exact: true });
async function expectPlain(page: Page) {
  await expect(page.locator('[data-slot="sidebar-wrapper"], [data-slot="sidebar"], [data-slot="sidebar-gap"], [data-slot="sidebar-rail"], [data-slot="sidebar-trigger"]')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toHaveCount(1);
  await expect(page.getByRole('button', { name: '侧栏账号菜单' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeVisible();
}

test('plain account pages retain content and all toolbar actions without sidebar preferences', async ({ page, request }, info) => {
  const { account } = await prepareWorkspaces(request);
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

test('legacy aliases consume their intent once, preserve history and keep the home card', async ({ page, request }) => {
  const { account } = await prepareWorkspaces(request);
  await login(page, account);
  for (const filter of ['tenants', 'projects'] as const) {
    await page.goto(`${consoleURL}/${filter}`);
    await expect(picker(page)).toBeVisible();
    await expect(picker(page).getByRole('combobox', { name: '工作空间类型' })).toHaveText(workspaceTypeLabels[filter]);
    await expect(page).toHaveURL(`${consoleURL}/?view=account`);
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
  await expect(picker(page).getByRole('combobox', { name: '工作空间类型' })).toHaveText('租户');
  await page.keyboard.press('Escape');
  await page.goto(`${consoleURL}/?workspace=invalid`);
  await expectPlain(page);
  await expect(picker(page)).toHaveCount(0);
});

for (const alias of ['tenants', 'projects'] as const) test(`anonymous ${alias} alias survives login and opens only its requested source`, async ({ page, request }) => {
  const { account } = await prepareWorkspaces(request);
  const reads: string[] = [];
  page.on('request', (request) => { if (request.url().includes('/api/v1/me/workspaces?') && new URL(request.url()).searchParams.has('kind')) reads.push(request.url()); });
  await page.goto(`${consoleURL}/${alias}`);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.getByLabel('用户名', { exact: true }).fill(account.username);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(picker(page).getByRole('combobox', { name: '工作空间类型' })).toHaveText(workspaceTypeLabels[alias]);
  await expect(picker(page).getByRole('button', { name: alias === 'tenants' ? /^交通监测中心/ : /^桥梁健康监测/ })).toBeVisible();
  await expect(page).toHaveURL(`${consoleURL}/?view=account`);
  expect(reads.length).toBeGreaterThan(0);
  expect(reads.every((url) => new URL(url).searchParams.get('kind') === (alias === 'tenants' ? 'tenant' : 'project'))).toBe(true);
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
    await browseWorkspaceTenant(page, '交通监测中心');
    await menu.getByRole('button', { name: /^桥梁健康监测/ }).click();
    await expect(page).toHaveURL(target);
    await expect(page.locator('[data-slot="sidebar"]').first()).toHaveAttribute('data-state', 'collapsed');
  }
  await page.getByRole('button', { name: '切换项目', exact: true }).click();
  await expect(picker(page).getByRole('combobox', { name: '工作空间类型' })).toHaveText('项目');
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
  await picker(page).getByRole('button', { name: /^桥梁健康监测/ }).click();
  await expect(picker(page)).toHaveCount(0);
  await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  expect(await page.evaluate(() => history.length)).toBe(length);
  await expect(page.getByRole('main')).toBeFocused();
  await page.getByRole('button', { name: '切换项目', exact: true }).click();
  await picker(page).getByRole('button', { name: /^隧道运行监测/ }).click();
  await expect(page).toHaveURL(new RegExp(`${f.projectIds[1]}/profile$`));
  await expect(page.getByRole('main')).toBeFocused();
  await page.getByRole('button', { name: '编辑项目资料', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '编辑项目资料', exact: true })).toBeVisible();
});

test('authorized lists paginate independently and search beyond loaded pages', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  const api = new URL('/api/v1', consoleURL).href;
  const snapshot = await (await request.get(`${api}/me/workspaces?limit=100`)).json();
  const tenants = snapshot.items.filter((v: {kind:string}) => v.kind === 'tenant');
  const projects = snapshot.items.filter((v: {kind:string}) => v.kind === 'project');
  const calls: URL[] = [];
  await page.route('**/api/v1/me/workspaces?*', (route) => {
    const url = new URL(route.request().url()); calls.push(url);
    const kind = url.searchParams.get('kind'); if (!kind) return route.continue();
    const q = url.searchParams.get('q'); const more = url.searchParams.get('cursor');
    const source = kind === 'tenant' ? tenants : projects;
    const items = q ? source.filter((v: {name:string;code:string}) => v.name.includes(q) || v.code.includes(q)) : more ? source.slice(1) : source.slice(0, 1);
    return route.fulfill({ json: { ...snapshot, items, next_cursor: !q && !more ? 'next-page' : '' } });
  });
  await login(page, f.account);
  const dialog = await openWorkspacePicker(page);
  await dialog.getByRole('button', { name: '加载更多租户', exact: true }).click();
  await expect(dialog.getByRole('button', { name: /^浏览租户：/ }).or(dialog.getByLabel('选择租户'))).not.toHaveCount(0);
  const tenantReads = calls.filter((u) => u.searchParams.get('kind') === 'tenant').length;
  await dialog.getByLabel('项目状态').selectOption('archived');
  await expect(dialog.getByRole('button', { name: '加载更多租户', exact: true })).toHaveCount(0);
  expect(calls.filter((u) => u.searchParams.get('kind') === 'tenant').length).toBe(tenantReads);
  await selectWorkspaceType(page, 'projects');
  await dialog.getByRole('button', { name: '加载更多项目', exact: true }).click();
  await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toHaveCount(1);
  await expect(dialog.getByRole('button', { name: /^隧道运行监测/ })).toHaveCount(1);
  await dialog.getByLabel('搜索名称或编号').fill(projects[1].name);
  await expect(dialog.getByRole('button', { name: new RegExp(`^${projects[0].name}`) })).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: new RegExp(`^${projects[1].name}`) })).toBeVisible();
  expect(calls.some((u) => u.searchParams.get('q') === projects[1].name && !u.searchParams.get('cursor'))).toBe(true);
  await expect(page).toHaveURL(`${consoleURL}/`);
  await page.keyboard.press('Escape');
  const reopened = await openWorkspacePicker(page);
  await expect(reopened.getByLabel('搜索名称或编号')).toHaveValue('');
  await expect(reopened.getByRole('combobox', { name: '工作空间类型' })).toHaveText('全部类型');
  await expect(reopened.getByLabel('项目状态')).toHaveValue('all');
});

test('a failed tenant query does not block authorized projects or fabricate parent access', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  let failed = true;
  await page.route('**/api/v1/me/workspaces?*', (route) => new URL(route.request().url()).searchParams.get('kind') === 'tenant' && failed ? route.fulfill({ status:503, json:{code:'unavailable',message:'租户读取失败'} }) : route.continue());
  await login(page, f.account);
  const dialog = await openWorkspacePicker(page);
  await expect(dialog.getByRole('button', { name: '租户加载失败，点击重试', exact:true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: /^交通监测中心/ })).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
  failed = false;
  await dialog.getByRole('button', { name: '租户加载失败，点击重试', exact:true }).click();
  await browseWorkspaceTenant(page, '交通监测中心');
  await expect(dialog.getByRole('button', { name: /^交通监测中心.*进入租户/ })).toBeVisible();
  await dialog.getByRole('button', { name: /^桥梁健康监测/ }).click();
  await expect(page.getByRole('heading', { name:'项目资料',exact:true })).toBeVisible();
});

test('768px breakpoint and keyboard picker remain usable in both themes', async ({ page, request }, info) => {
  const { account } = await prepareWorkspaces(request);
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
      await picker(page).press('End');
      await expect(picker(page).getByRole('button', { name: /^平台工作空间/ })).toBeFocused();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: info.outputPath(`picker-${width}-${theme}.png`), fullPage: true });
      await page.keyboard.press('Escape');
    }
  }
});

test('late searches and an identity change cannot resurrect previous picker results', async ({ page, request, context }) => {
  test.setTimeout(90_000);
  const f = await prepareWorkspaces(request);
  const api = new URL('/api/v1', consoleURL).href;
  const list = await (await request.get(`${api}/me/workspaces?kind=tenant`)).json();
  const next = await createAccount(request, false);
  await login(page, f.account);
  let release!: () => void;
  let held = new Promise<void>((resolve) => { release = resolve; });
  let captured!: () => void;
  let capturedRequest = new Promise<void>((resolve) => { captured = resolve; });
  await page.route('**/api/v1/me/workspaces?*', async (route) => {
    if (new URL(route.request().url()).searchParams.get('kind') !== 'tenant' || !new URL(route.request().url()).searchParams.get('q')?.startsWith('slow')) return route.continue();
    captured(); await held;
    await route.fulfill({ json: list }).catch(() => {});
  });
  try {
    const menu = await openWorkspacePicker(page, 'tenants');
    await expect(menu.getByRole('button', { name: /^交通监测中心/ })).toBeVisible();
    await menu.getByLabel('搜索名称或编号').fill('slow');
    await capturedRequest;
    await menu.getByLabel('搜索名称或编号').fill('no-match-unique');
    await expect(menu.getByText('当前没有符合条件的租户工作空间。')).toBeVisible();
    release();
    await menu.getByLabel('搜索名称或编号').fill('交通监测中心');
    await expect(menu.getByRole('button', { name: /^交通监测中心/ })).toBeVisible();
    await expect(menu.getByRole('button', { name: /^城市基础设施中心/ })).toHaveCount(0);
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
    await expect(page.getByRole('heading', { name: '暂无工作空间授权' })).toBeVisible();
    await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toHaveCount(0);
    await expect(page.getByText(/交通监测中心|城市基础设施中心/)).toHaveCount(0);
    await other.close();
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }); }
});

test('tenant governance keeps creation, owner, management-gap, status filters and details', async ({ page, request }) => {
  test.setTimeout(60_000);
  const f = await prepareWorkspaces(request);
  await login(page, f.account);
  await page.goto(`${consoleURL}/t/${f.tenantIds[0]}/projects`);
  await expect(page.getByRole('heading', { name: '项目治理', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: '创建项目', exact: true })).toHaveAttribute('href', `/t/${f.tenantIds[0]}/projects/new`);
  await expect(page.getByRole('link', { name: '资源授权', exact: true })).toHaveAttribute('href', `/t/${f.tenantIds[0]}/resource-access/${f.projectIds[0]}`);
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

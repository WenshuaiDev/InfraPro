import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, createAccount, csrf, login, password, consoleURL } from './fixtures';

const api = new URL('/api/v1', consoleURL).href;
async function signIn(request: APIRequestContext, username: string) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200);
  return headers;
}

async function prepare(request: APIRequestContext) {
  const account = await createAccount(request);
  const ownerHeaders = await signIn(request, 'e2e_owner');
  const accounts = await (await request.get(`${api}/access/accounts?q=${account.username}`)).json();
  const tenantIds: string[] = [];
  for (const name of ['交通监测中心', '城市基础设施中心']) {
    const response = await request.post(`${api}/platform/tenants`, { headers: ownerHeaders, data: { name, code: `ws-${randomBytes(5).toString('hex')}`, administrator_account_id: accounts.items[0].id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(201);
    tenantIds.push((await response.json()).tenant_id);
  }
  const headers = await signIn(request, account.username);
  const projectIds: string[] = [];
  for (const [index, name] of ['桥梁健康监测', '隧道运行监测'].entries()) {
    const tenantId = tenantIds[index]!;
    const ctx = await (await request.get(`${api}/tenants/${tenantId}/context`)).json();
    const response = await request.post(`${api}/tenants/${tenantId}/projects`, { headers, data: { name, code: `project-${index}`, purpose: '工作空间切换验收', owner_tenant_membership_id: ctx.membership_id, administrator_tenant_membership_id: ctx.membership_id, reason: '工作空间切换验收', client_request_id: randomUUID() } });
    expect(response.status()).toBe(200);
    projectIds.push((await response.json()).project_id);
  }
  return { account, tenantIds, projectIds };
}

async function sidebar(page: Page) {
  const navigation = page.getByRole('navigation', { name: '主导航', exact: true });
  // Desktop navigation can briefly unmount while the new workspace loads.
  // Only mobile navigation needs its drawer opened; wait for desktop content.
  if ((page.viewportSize()?.width ?? 1440) < 768 && !await navigation.isVisible()) await page.getByRole('banner').getByRole('button', { name: '切换侧栏', exact: true }).click();
  await expect(navigation).toBeVisible();
  return navigation;
}
async function switcher(page: Page) {
  await sidebar(page);
  await page.getByRole('button', { name: /^切换工作空间：/ }).click();
  return page.getByRole('menu');
}

test('workspace hierarchy switches routes and isolates platform, tenant and project navigation', async ({ page, request, context }, info) => {
  const f = await prepare(request);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await login(page, f.account);
  // Anonymous identity probes can return 401 during login; inspect the actual
  // workspace flow for console errors after authentication has completed.
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
  let menu = await switcher(page);
  const tenantGroup = menu.getByRole('group', { name: '交通监测中心', exact: true });
  await expect(tenantGroup.getByRole('menuitem', { name: /^桥梁健康监测/ })).toBeVisible();
  await expect(tenantGroup.getByRole('menuitem', { name: /^隧道运行监测/ })).toHaveCount(0);
  await expect(menu.getByRole('group', { name: '城市基础设施中心', exact: true }).getByRole('menuitem', { name: /^隧道运行监测/ })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('workspace-groups-light.png'), fullPage: true });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: info.outputPath('workspace-groups-dark.png'), fullPage: true });
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  await tenantGroup.getByRole('menuitem', { name: /^桥梁健康监测/ }).click();
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
  await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测/ })).toHaveAttribute('aria-current', 'true');
  await menu.getByRole('menuitem', { name: /^交通监测中心 租户/ }).click();
  if (info.project.name === 'mobile') await expect(page.getByRole('dialog', { name: '主导航', exact: true })).toBeHidden();
  await expect(page).toHaveURL(`${consoleURL}/t/${f.tenantIds[0]}/profile`);
  nav = await sidebar(page);
  await expect(nav.getByRole('link', { name: '项目治理', exact: true })).toBeVisible();
  for (const title of ['账号开户邀请', '租户管理', '项目成员']) await expect(nav.getByRole('link', { name: title, exact: true })).toHaveCount(0);
  menu = await switcher(page);
  await menu.getByRole('menuitem', { name: /^平台工作空间/ }).click();
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
    await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测/ })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
    await page.getByRole('button', { name: /^切换工作空间：/ }).press('Enter');
    await page.getByRole('menu').press('Home');
    await expect(page.getByRole('menuitem', { name: /^平台工作空间/ })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(`${consoleURL}/invitations`);
  }
  expect(errors).toEqual([]);
});

test('workspace menu handles empty memberships, missing platform access and independent retry', async ({ page, request }, info) => {
  const account = await createAccount(request, false);
  await login(page, account);
  let menu = await switcher(page);
  await expect(menu.getByText('尚未加入租户或项目。', { exact: true })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: /^平台工作空间/ })).toHaveCount(0);
  await page.keyboard.press('Escape');
  let failing = true;
  await page.route('**/api/v1/me/projects?*', (route) => failing ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '测试暂时故障' }) }) : route.continue());
  menu = await switcher(page);
  await expect(menu.getByRole('menuitem', { name: '项目加载失败，点击重试', exact: true })).toBeVisible();
  await expect(menu.getByText('尚未加入租户或项目。', { exact: true })).toHaveCount(0);
  failing = false;
  await menu.getByRole('menuitem', { name: '项目加载失败，点击重试', exact: true }).click();
  await expect(menu.getByText('尚未加入租户或项目。', { exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('workspace-empty.png'), fullPage: true });
});

test('workspace menu paginates both lists and preserves independent project access', async ({ page, request }, info) => {
  const f = await prepare(request);
  const tenantList = await (await request.get(`${api}/account/tenants`)).json();
  const projectList = await (await request.get(`${api}/me/projects?status=all`)).json();
  const tenantA = tenantList.items.find((t: { id: string }) => t.id === f.tenantIds[0]);
  const tenantB = tenantList.items.find((t: { id: string }) => t.id === f.tenantIds[1]);
  const projectA = projectList.items.find((p: { project: { id: string } }) => p.project.id === f.projectIds[0]);
  const projectB = projectList.items.find((p: { project: { id: string } }) => p.project.id === f.projectIds[1]);
  await page.route('**/api/v1/account/tenants?*', (route) => {
    const more = new URL(route.request().url()).searchParams.get('cursor');
    return route.fulfill({ json: { items: more ? [tenantB] : [{ ...tenantA, can_enter: false }], next_cursor: more ? '' : 'next-tenant' } });
  });
  await page.route('**/api/v1/me/projects?*', (route) => {
    const more = new URL(route.request().url()).searchParams.get('cursor');
    return route.fulfill({ json: { items: more ? [{ ...projectB, can_enter: false }] : [{ ...projectA, project: { ...projectA.project, status: 'archived' } }], next_cursor: more ? '' : 'next-project' } });
  });
  await login(page, f.account);
  const menu = await switcher(page);
  await expect(menu.getByRole('menuitem', { name: /^交通监测中心/ })).toBeDisabled();
  await expect(menu.getByRole('menuitem', { name: /^桥梁健康监测 已归档/ })).toBeEnabled();
  await menu.getByRole('menuitem', { name: '加载更多项目', exact: true }).click();
  await expect(menu.getByRole('group', { name: '城市基础设施中心', exact: true }).getByRole('menuitem', { name: /^隧道运行监测/ })).toBeDisabled();
  await menu.getByRole('menuitem', { name: '加载更多租户', exact: true }).click();
  await expect(menu.getByRole('menuitem', { name: /^城市基础设施中心 租户 ·/ })).toBeEnabled();
  await expect(menu.getByRole('menuitem', { name: /^加载更多/ })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath('workspace-pagination.png'), fullPage: true });
  await menu.getByRole('menuitem', { name: /^桥梁健康监测/ }).click();
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
});

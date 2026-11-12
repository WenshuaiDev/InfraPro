import { test, expect, login, createAccount, consoleURL, csrf, password } from './fixtures';
import { prepareProjectOnly } from '../helpers/workspaces';
import { navigateTo, openWorkspacePicker } from '../helpers/navigation';

test.use({ trace: 'off' });
const api = new URL('/api/v1', consoleURL).href;

test('a platform device-catalog-only role enters its readable workspace after login', async ({ page, request }) => {
  const account = await createAccount(request, false);
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const accounts = await (await request.get(`${api}/access/accounts?q=${account.username}`)).json();
  const target = accounts.items[0];
  const created = await request.post(`${api}/access/roles`, { headers, data: { name: `设备目录_${account.username}`, description: '仅目录读取', reason: '工作空间入口回归' } });
  expect(created.status()).toBe(200); const role = await created.json();
  const permissions = await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: ['device.definition.read'], reason: '目录读取回归' } });
  expect(permissions.status(), await permissions.text()).toBe(200);
  const assigned = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [role.id], reason: '仅授权设备产品目录' } });
  expect(assigned.status(), await assigned.text()).toBe(200);
  await login(page, account);
  await expect(page).toHaveURL(`${consoleURL}/platform/device-products`);
  await expect(page.getByRole('heading', { name: '公共设备产品', exact: true })).toBeVisible();
  const result = await (await page.request.get(`${api}/me/workspaces`)).json();
  expect(result.summary.mode).toBe('single');
  expect(result.summary.single.default_path).toBe('/platform/device-products');
});

test('one project enters directly, preserves personal pages and rejects ungranted parent and sibling', async ({ page, request }) => {
  const f = await prepareProjectOnly(request);
  await login(page, f.member);
  const target = `${consoleURL}/t/${f.tenantIds[0]}/p/${f.projectIds[0]}/profile`;
  await expect(page).toHaveURL(target);
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /^切换工作空间：|^切换项目$/ })).toHaveCount(0);
  const list = await (await page.request.get(`${api}/me/workspaces?limit=1`)).json();
  expect(list.summary.mode).toBe('single'); expect(list.summary.tenant_count).toBe(0); expect(list.items.map((v: { id: string }) => v.id)).toEqual([f.projectIds[0]]);
  for (const name of ['个人资料', '账号安全', '账号首页']) { await navigateTo(page, name); await expect(page.getByRole('heading', { name, exact: true })).toBeVisible(); }
  await page.reload(); await expect(page.getByRole('heading', { name: '账号首页', exact: true })).toBeVisible();
  await page.goto(`${consoleURL}/t/${f.tenantIds[0]}/profile`);
  await expect(page.getByRole('heading', { name: '当前租户不可访问' })).toBeVisible();
  await page.goto(`${consoleURL}/t/${f.tenantIds[1]}/p/${f.projectIds[1]}/profile`);
  await expect(page.getByRole('heading', { name: '当前项目不可访问' })).toBeVisible();
  expect((await page.request.get(`${api}/tenants/${f.tenantIds[1]}/projects/${f.projectIds[1]}/profile`)).status()).toBe(404);
  await page.goto(consoleURL); await expect(page).toHaveURL(target);
  await f.suspend();
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByRole('heading', { name: '当前项目不可访问' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toHaveCount(0);
  await page.goto(consoleURL);
  await expect(page.getByRole('heading', { name: '暂无工作空间授权' })).toBeVisible();
  expect((await (await page.request.get(`${api}/me/workspaces?q=桥梁`)).json()).items).toEqual([]);
});

test('project-only multiple entries use a flat list and filtered single results do not redirect', async ({ page, request }, info) => {
  const f = await prepareProjectOnly(request, 2);
  await login(page, f.member);
  await expect(page).toHaveURL(`${consoleURL}/`);
  const dialog = await openWorkspacePicker(page);
  await expect(dialog.getByRole('group', { name: '租户列表' })).toHaveCount(0);
  await expect(dialog.getByLabel('选择租户')).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: /^浏览租户：|进入租户/ })).toHaveCount(0);
  for (const name of ['桥梁健康监测', '隧道运行监测']) await expect(dialog.getByRole('button', { name: new RegExp(`^${name}`) })).toBeVisible();
  await dialog.getByLabel('搜索名称或编号').fill('桥梁');
  await expect(dialog.getByRole('button', { name: /^隧道运行监测/ })).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
  await expect(page).toHaveURL(`${consoleURL}/`);
  const result = await (await page.request.get(`${api}/me/workspaces?q=桥梁&limit=1`)).json();
  expect(result.items).toHaveLength(1); expect(result.summary.mode).toBe('multiple'); expect(result.summary.single).toBeUndefined();
  await page.screenshot({ path: info.outputPath('project-only-dialog.png') });
  await dialog.getByRole('button', { name: /^桥梁健康监测/ }).click();
  await expect(page).toHaveURL(new RegExp(`${f.projectIds[0]}/profile$`));
  await expect(page.getByRole('heading', { name: '项目资料', exact: true })).toBeVisible();
});

test('no business entry keeps personal access and failed resolution is retryable', async ({ page, request }) => {
  const account = await createAccount(request, false);
  let failed = true;
  await page.route('**/api/v1/me/workspaces?*', (route) => failed ? route.fulfill({ status:503, json:{code:'unavailable',message:'工作空间查询暂时失败'} }) : route.continue());
  await login(page, account);
  await expect(page.getByRole('heading', { name: '工作空间核验失败' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '暂无工作空间授权' })).toHaveCount(0);
  failed = false;
  await page.getByRole('button', { name: '重新加载', exact: true }).click();
  await expect(page.getByRole('heading', { name: '暂无工作空间授权' })).toBeVisible();
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toHaveCount(0);
  await navigateTo(page, '个人资料');
  await expect(page.getByRole('heading', { name: '个人资料', exact: true })).toBeVisible();
});

test('retrying an explicit entry request enters the only project and dismisses the error dialog', async ({ page, request }) => {
  const f = await prepareProjectOnly(request);
  await login(page, f.member);
  await expect(page).toHaveURL(new RegExp(`${f.projectIds[0]}/profile$`));
  await navigateTo(page, '个人资料');
  let failed = true;
  await page.route('**/api/v1/me/workspaces?*', (route) => failed ? route.fulfill({ status: 503, json: { code: 'unavailable', message: '暂时无法核验授权' } }) : route.continue());
  const entry = page.getByRole('button', { name: '进入工作空间：桥梁健康监测', exact: true });
  await expect(entry).toContainText('桥梁健康监测');
  await entry.click();
  const dialog = page.getByRole('dialog', { name: '切换工作空间', exact: true });
  await expect(dialog.getByRole('button', { name: '工作空间核验失败，点击重试', exact: true })).toBeVisible();
  failed = false;
  await dialog.getByRole('button', { name: '工作空间核验失败，点击重试', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`${f.projectIds[0]}/profile$`));
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: /^切换工作空间：|^切换项目$/ })).toHaveCount(0);
});

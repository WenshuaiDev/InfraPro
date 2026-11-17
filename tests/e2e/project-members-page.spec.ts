import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Browser, Page } from '@playwright/test';
import { test, expect, consoleURL, createAccount, csrf, login, password } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import type { ProjectMember } from '../../packages/ts/api-client/src/projects';

const origin = new URL(consoleURL).origin, api = `${origin}/api/v1`;
test.use({ trace: 'off', actionTimeout: 15_000 });

async function prepare(request: APIRequestContext, browser: Browser) {
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request) };
  const owner = async () => expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const accounts = [];
  for (let i = 0; i < 10; i++) accounts.push(await createAccount(request, false));
  await owner();
  const ownerId = (await (await request.get(`${api}/access/accounts?q=e2e_owner`)).json()).items[0].id;
  const tenant = await request.post(`${api}/platform/tenants`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { name: '项目成员界面验收', code: `members-${randomBytes(5).toString('hex')}`, administrator_account_id: ownerId, client_request_id: randomUUID(), reason: '隔离界面验收' } });
  expect(tenant.status()).toBe(201);
  const tid = (await tenant.json()).tenant_id;
  const tenantAdmin = (await (await request.get(`${api}/tenants/${tid}/members`)).json()).items[0].id;
  const project = await request.post(`${api}/tenants/${tid}/projects`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { name: '成员工作台验收项目', code: 'members-ui', purpose: '项目成员界面与授权回归', owner_tenant_membership_id: tenantAdmin, administrator_tenant_membership_id: tenantAdmin, client_request_id: randomUUID(), reason: '隔离项目' } });
  expect(project.status()).toBe(200);
  const pid = (await project.json()).project_id, base = `${api}/tenants/${tid}/projects/${pid}`;
  for (const account of accounts) {
    const target = (await (await request.get(`${api}/access/accounts?q=${account.username}`)).json()).items[0];
    const invitation = await request.post(`${api}/tenants/${tid}/member-invitations`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { target_account_id: target.id, client_request_id: randomUUID(), reason: '隔离成员' } });
    expect(invitation.status()).toBe(201);
    const value = await invitation.json();
    const token = new URLSearchParams(new URL(value.link).hash.slice(1)).get('token');
    const viewer = await browser.newContext();
    let membership: string;
    try {
      const h = { Origin: origin, 'X-CSRF-Token': await csrf(viewer.request) };
      expect((await viewer.request.post(`${api}/auth/login`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: account })).status()).toBe(200);
      const accepted = await viewer.request.post(`${api}/account/tenant-invitations/accept`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { token, expected_version: value.invitation.version } });
      expect(accepted.status()).toBe(200);
      membership = (await accepted.json()).membership_id;
    } finally { await viewer.close(); }
    const context = await (await request.get(base + '/context')).json();
    expect((await request.post(base + '/members', { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { tenant_membership_id: membership, expected_project_version: context.project.version, client_request_id: randomUUID(), reason: '分页数据' } })).status()).toBe(200);
  }
  const members = (await (await request.get(base + '/members?limit=100')).json()).items as ProjectMember[];
  return { tid, pid, base, headers, accounts, members, path: `${consoleURL}/t/${tid}/p/${pid}/members` };
}

async function confirmMemberAction(page: Page, label: string) {
  const panel = page.getByRole('dialog');
  await expect(panel.getByRole('button', { name: `确认${label}`, exact: true })).toBeDisabled();
  await panel.getByRole('button', { name: '预览变更影响', exact: true }).click();
  await expect(panel.getByRole('region', { name: '项目变更影响', exact: true })).toBeVisible();
  await panel.getByLabel('变更原因', { exact: true }).fill('项目成员界面回归验收');
  await panel.getByRole('button', { name: `确认${label}`, exact: true }).click();
  await expect(panel.getByRole('heading', { name: '成员详情', exact: true })).toBeVisible();
}

test('project members use the account table and drawer with real pagination, authorization and lifecycle actions', async ({ page, request, browser }, info) => {
  test.setTimeout(240_000);
  const f = await prepare(request, browser), target = f.members.find((member) => member.username === f.accounts[0]!.username)!;
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page, { username: 'e2e_owner', password });
  await page.goto(f.path);
  const table = page.getByRole('table', { name: '项目成员列表', exact: true }), rows = table.locator('tbody tr');
  const pager = page.getByRole('navigation', { name: '项目成员列表分页', exact: true }), search = page.getByRole('textbox', { name: '搜索项目成员', exact: true });
  await expect(rows).toHaveCount(10);
  const first = await rows.locator('td:first-child').allTextContents();
  await pager.getByRole('button', { name: '下一页', exact: true }).click();
  await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(rows).toHaveCount(1);
  expect(new Set([...first, ...await rows.locator('td:first-child').allTextContents()]).size).toBe(11);
  const trigger = rows.first().getByRole('button').first();
  await trigger.focus(); await page.keyboard.press('Enter');
  const panel = page.getByRole('dialog');
  await expect(panel.getByRole('tab', { name: '绑定角色', exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveAttribute('data-swipe-direction', info.project.name === 'mobile' ? 'down' : 'right');
  await expect(page.locator('[data-slot="drawer-overlay"]')).toHaveCSS('backdrop-filter', /blur\([1-9]/);
  await page.keyboard.press('Escape'); await expect(panel).toHaveCount(0); await expect(trigger).toBeFocused();
  await expect(pager.getByRole('status')).toContainText('第 2 页');
  await page.getByRole('tab', { name: '正常', exact: true }).click(); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('tab', { name: '当前成员', exact: true }).click(); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click(); await expect(rows).toHaveCount(11);
  if (info.project.name === 'desktop') {
    await page.getByRole('button', { name: '显示列', exact: true }).click(); await page.getByRole('menuitemcheckbox', { name: '加入时间', exact: true }).click(); await page.keyboard.press('Escape');
    await expect(table.getByRole('columnheader', { name: '加入时间', exact: true })).toHaveCount(0);
  }
  await search.fill('不存在的项目成员'); await expect(page.getByText('没有符合条件的成员', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click(); await expect(rows).toHaveCount(11);
  // A real list read that fails must show retry, then recover the same filter.
  let failRead = true;
  const listPattern = `**/tenants/${f.tid}/projects/${f.pid}/members?*`;
  await page.route(listPattern, route => failRead ? route.fulfill({ status: 503, json: { code: 'unavailable', message: '成员查询暂时不可用' } }) : route.continue());
  await search.fill(target.username); await expect(page.getByRole('alert')).toContainText('成员查询暂时不可用');
  failRead = false; await page.getByRole('button', { name: '重新加载', exact: true }).click(); await expect(rows).toHaveCount(1); await expect(page.getByRole('alert')).toHaveCount(0);
  await page.unroute(listPattern);
  const menu = rows.first().getByRole('button', { name: /的操作$/ });
  await menu.click(); await page.getByRole('menuitem', { name: '查看详情', exact: true }).click();
  await panel.getByRole('tab', { name: '有效权限', exact: true }).click(); await expect(panel.getByRole('region', { name: '有效权限及来源', exact: true })).toBeVisible();
  await expect(panel.getByText('来自：项目普通成员', { exact: true }).first()).toBeVisible();
  await panel.getByRole('switch', { name: '显示权限编码', exact: true }).check(); await expect(panel.getByText(P.ProjectProfileRead, { exact: true })).toBeVisible();
  await panel.getByRole('tab', { name: '授权历史', exact: true }).click(); await expect(panel.getByText('当前绑定', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape'); await expect(menu).toBeFocused(); await expect(search).toHaveValue(target.username);

  // Original detail URLs open the same drawer, and closing restores the list route.
  await page.goto(`${f.path}/${target.id}`); await expect(panel.getByRole('heading', { name: '成员详情', exact: true })).toBeVisible();
  await panel.getByRole('button', { name: '配置项目角色', exact: true }).click();
  await panel.getByRole('checkbox', { name: /项目普通成员/ }).uncheck();
  await confirmMemberAction(page, '配置项目角色'); await expect(panel.getByText('尚未绑定项目角色', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: '配置项目角色', exact: true }).click(); await panel.getByRole('checkbox', { name: /项目普通成员/ }).check(); await confirmMemberAction(page, '配置项目角色');
  expect((await (await request.get(`${f.base}/members/${target.id}/authorization`)).json()).roles.length).toBe(1);
  await panel.getByRole('button', { name: '暂停项目成员', exact: true }).click(); await confirmMemberAction(page, '暂停项目成员'); await expect(panel.getByRole('button', { name: '恢复项目成员', exact: true })).toBeVisible();
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click(); await expect(panel).toHaveCount(0); await expect(page).toHaveURL(f.path);
  await page.getByRole('tab', { name: '已暂停', exact: true }).click(); await expect(rows).toHaveCount(1); await expect(table).toContainText(target.username);
  await rows.first().getByRole('button', { name: /的操作$/ }).click(); await page.getByRole('menuitem', { name: '恢复项目成员', exact: true }).click(); await confirmMemberAction(page, '恢复项目成员');
  await panel.getByRole('button', { name: '移除项目成员', exact: true }).click(); await confirmMemberAction(page, '移除项目成员');
  await expect(panel.getByRole('button', { name: '配置项目角色', exact: true })).toHaveCount(0);
  await panel.getByRole('tab', { name: '授权历史', exact: true }).click(); await expect(panel.getByText('已撤销', { exact: true }).first()).toBeVisible();
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click(); await page.getByRole('tab', { name: '已结束', exact: true }).click(); await expect(rows).toHaveCount(1);
  await page.getByRole('button', { name: '添加项目成员', exact: true }).click();
  await expect(panel.getByRole('button', { name: '确认添加成员', exact: true })).toBeDisabled();
  await panel.getByLabel('选择项目成员', { exact: true }).fill(target.username); await panel.getByRole('button', { name: `${target.display_name}（${target.username}）`, exact: true }).click();
  await panel.getByLabel('变更原因', { exact: true }).fill('重新加入保留历史关系'); await panel.getByRole('button', { name: '确认添加成员', exact: true }).click(); await expect(panel).toHaveCount(0);
  await page.getByRole('tab', { name: '全部历史', exact: true }).click(); await search.fill(target.username); await expect(rows).toHaveCount(2);
  const history = (await (await request.get(`${f.base}/members?status=all&q=${target.username}`)).json()).items as ProjectMember[];
  expect(history.map(member => member.status).sort()).toEqual(['active', 'ended']);
  const replacement = history.find(member => member.status === 'active')!; expect(replacement.id).not.toBe(target.id);

  // Self/current-admin protection also applies to menu shortcuts.
  await search.fill('e2e_owner'); await rows.first().getByRole('button', { name: /的操作$/ }).click(); await expect(page.getByRole('menuitem')).toHaveCount(1); await page.getByRole('menuitem', { name: '查看详情' }).click();
  await expect(panel.getByText('当前项目管理员受保护。如需变更，请在项目资料中交接管理员。', { exact: true })).toBeVisible();
  await expect(panel.getByRole('button', { name: '移除项目成员', exact: true })).toHaveCount(0);
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click(); await page.getByRole('button', { name: '重置', exact: true }).click();

  for (const width of info.project.name === 'desktop' ? [1440, 1024] : [375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => document.documentElement.classList.toggle('dark', value === 'dark'), theme);
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: info.outputPath(`members-${width}-${theme}.png`), fullPage: true });
    }
  }
  await rows.first().getByRole('button', { name: /的操作$/ }).click(); await page.getByRole('menuitem', { name: '查看详情', exact: true }).click();
  await expect(panel.getByRole('tab', { name: '绑定角色', exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('member-drawer-dark.png'), fullPage: true });
  // The shared Drawer has an off-screen swipe bleed; check the actual content scroll area.
  await expect.poll(() => panel.locator('[data-slot="drawer-scroll-area"]').evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  await panel.getByRole('button', { name: '关闭面板', exact: true }).click();

  // A real reader role can see members but must not fetch protected authorization.
  const currentProject = await (await request.get(`${f.base}/context`)).json();
  const readerRole = await request.post(`${f.base}/roles`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { name: '成员只读', description: '', permission_codes: [P.ProjectMemberRead, P.ProjectProfileRead], expected_project_version: currentProject.project.version, reason: '权限边界', client_request_id: randomUUID() } });
  expect(readerRole.status()).toBe(200);
  const roleId = (await readerRole.json()).object_id;
  const change = async (path: string, data: object, method = 'POST') => {
    const preview = await request.post(path + '/preview', { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data }); expect(preview.status()).toBe(200);
    const value = await preview.json();
    const response = await request.fetch(path, { method, headers: f.headers, data: { ...data, expected_version: value.expected_version, expected_project_version: value.expected_project_version, preview_hash: value.preview_hash, ...(method === 'PUT' ? { role_versions: value.role_versions } : {}), reason: '权限边界验收', client_request_id: randomUUID() } }); expect(response.status()).toBe(200);
  };
  await change(`${f.base}/members/${replacement.id}/roles`, { role_ids: [roleId] }, 'PUT');
  const viewer = await browser.newContext();
  try {
    const reader = await viewer.newPage(), protectedReads: string[] = [];
    reader.on('request', request => { if (request.url().includes(`/projects/${f.pid}/`) && /\/(authorization|permissions)(\?|$)/.test(request.url())) protectedReads.push(request.url()); });
    await login(reader, f.accounts[0]!); await reader.goto(f.path); await expect(reader.getByRole('table', { name: '项目成员列表', exact: true })).toBeVisible();
    await expect(reader.getByRole('button', { name: '添加项目成员', exact: true })).toHaveCount(0);
    await reader.getByRole('textbox', { name: '搜索项目成员', exact: true }).fill('e2e_owner');
    await reader.getByRole('table').getByRole('button', { name: /的操作$/ }).click(); await expect(reader.getByRole('menuitem')).toHaveCount(1); await reader.getByRole('menuitem', { name: '查看详情', exact: true }).click();
    await expect(reader.getByText('没有项目授权查看权限。', { exact: true })).toBeVisible(); expect(protectedReads).toEqual([]);
  } finally { await viewer.close(); }
  await change(`${f.base}/archive`, {}); await page.reload();
  await expect(page.getByRole('button', { name: '添加项目成员', exact: true })).toHaveCount(0);
  expect(errors).toEqual([]);
});

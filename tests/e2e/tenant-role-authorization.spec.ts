import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext } from '@playwright/test';
import { test, expect, consoleURL, createAccount, csrf, login, password } from './fixtures';
import { closeRolePanel, roleAction } from '../helpers/roles';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

test.use({ trace: 'off' });
const origin = new URL(consoleURL).origin;
const api = `${origin}/api/v1`;
async function signIn(request: APIRequestContext, username: string) {
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200); return headers;
}
async function prepare(request: APIRequestContext) {
  const administrator = await createAccount(request, false); const member = await createAccount(request, false); const headers = await signIn(request, 'e2e_owner');
  const find = async (username: string) => (await (await request.get(`${api}/access/accounts?q=${username}`)).json()).items[0].id as string;
  const admin = await find(administrator.username); const target = await find(member.username);
  const response = await request.post(`${api}/platform/tenants`, { headers, data: { name: '角色授权验收', code: `tra-${randomBytes(5).toString('hex')}`, administrator_account_id: admin, client_request_id: randomUUID(), reason: '角色授权测试准备' } }); expect(response.status()).toBe(201); const tenant = (await response.json()).tenant_id as string;
  const h = await signIn(request, administrator.username);
  const link = await (await request.post(`${api}/tenants/${tenant}/member-invitations`, { headers: h, data: { target_account_id: target, client_request_id: randomUUID(), reason: '加入测试' } })).json();
  const recipient = await signIn(request, member.username); const token = new URLSearchParams(new URL(link.link).hash.slice(1)).get('token'); const accepted = await request.post(`${api}/account/tenant-invitations/accept`, { headers: recipient, data: { token, expected_version: link.invitation.version } }); expect(accepted.status()).toBe(200); const membership = (await accepted.json()).membership_id as string;
  return { administrator, member, tenant, membership };
}

test('role UI: create, copy, multi-role union, impact, disable, empty authorization and keyboard', async ({ page, request, browser }, info) => {
  const f = await prepare(request); await login(page, f.administrator); await page.goto(`${consoleURL}/t/${f.tenant}/permissions`);
  await expect(page).toHaveURL(`${consoleURL}/t/${f.tenant}/roles`); await expect(page.getByRole('heading', { name: '租户角色', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: '租户权限目录', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: /搜索页面/ }).click(); await page.getByPlaceholder('输入页面名称…').fill('权限目录'); await expect(page.getByText('没有找到相关页面。', { exact: true })).toBeVisible(); await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '创建租户角色', exact: true }).click(); await page.getByLabel('角色名称', { exact: true }).fill('本地资料维护'); await page.getByLabel('角色说明', { exact: true }).fill('角色授权浏览器验收');
  const editor = page.getByRole('dialog');
  await expect(editor.getByRole('heading', { name: '可配置业务权限' })).toBeVisible();
  await expect(editor.getByRole('checkbox', { name: /创建项目/ })).toBeVisible();
  await expect(editor.getByText(P.TenantProfileRead, { exact: true })).toHaveCount(0);
  await editor.getByRole('switch', { name: '显示权限编码', exact: true }).check(); await expect(editor.getByText(P.TenantProfileRead, { exact: true })).toBeVisible(); await editor.getByRole('switch', { name: '显示权限编码', exact: true }).uncheck();
  await editor.locator('summary').filter({ hasText: '当前管理员保留能力' }).click();
  await expect(editor.getByText('以下能力由当前租户管理员持有，不可下放给自定义角色。', { exact: true })).toBeVisible();
  await expect(editor.getByText('仅当前租户管理员持有，不可下放。', { exact: true }).first()).toBeVisible();
  await expect(editor.getByRole('checkbox', { name: /配置租户成员角色/ })).toHaveCount(0);
  await editor.locator('summary').filter({ hasText: '当前管理员保留能力' }).click();
  await page.getByRole('checkbox', { name: '查看租户资料', exact: false }).check(); await page.getByRole('checkbox', { name: '修改租户资料', exact: false }).check(); await page.getByLabel('搜索权限', { exact: true }).fill('邀请'); await expect(page.getByText('已选择 2 项权限。', { exact: false })).toBeVisible(); await page.getByLabel('搜索权限', { exact: true }).fill('');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('tenant-permission-picker.png') });
  await page.getByLabel('操作原因').fill('创建职责'); await page.getByRole('button', { name: '创建角色', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); await page.getByRole('link', { name: '本地资料维护', exact: true }).click(); await expect(page.getByRole('heading', { name: '本地资料维护', exact: true })).toBeVisible();
  const roleId = new URL(page.url()).pathname.split('/').at(-1)!;
  await expect(page.getByRole('dialog').getByRole('region', { name: '资料权限', exact: true }).getByRole('listitem')).toHaveCount(2);
  await page.screenshot({ path: info.outputPath('tenant-role-drawer-grouped.png') });
  await page.getByRole('tab', { name: '操作记录', exact: true }).click();
  await page.getByLabel('事件类型', { exact: true }).selectOption('tenant.role.created'); await expect(page.getByRole('heading', { name: '创建租户角色', exact: true })).toBeVisible(); await page.getByLabel('事件类型', { exact: true }).selectOption('');
  const deep = await browser.newContext({ viewport: page.viewportSize()! });
  try { const deepPage = await deep.newPage(); await deepPage.goto(`${consoleURL}/t/${f.tenant}/roles/${roleId}`); await deepPage.getByLabel('用户名', { exact: true }).fill(f.administrator.username); await deepPage.getByLabel('密码', { exact: true }).fill(password); await deepPage.getByRole('button', { name: '登录', exact: true }).click(); await expect(deepPage.getByRole('heading', { name: '本地资料维护', exact: true })).toBeVisible(); } finally { await deep.close(); }
  await page.getByRole('button', { name: '复制为本租户角色', exact: true }).click(); await page.getByLabel('角色名称', { exact: true }).fill('独立资料副本'); await page.getByLabel('操作原因').fill('复制独立职责'); await page.getByRole('button', { name: '创建角色', exact: true }).click(); await expect(page.getByRole('heading', { name: '本地资料维护', exact: true })).toBeVisible();
  await page.goto(`${consoleURL}/t/${f.tenant}/members/${f.membership}`); await page.getByRole('button', { name: '配置成员角色' }).click(); await page.getByRole('checkbox', { name: '本地资料维护', exact: false }).check(); await page.getByRole('checkbox', { name: '租户邀请协作员', exact: false }).check(); await page.getByRole('button', { name: '预览成员授权影响' }).click(); await expect(page.getByRole('region', { name: '变更影响预览' })).toBeVisible(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('tenant-member-impact.png'), fullPage: true }); await page.getByLabel('操作原因').fill('安排资料及邀请工作'); await page.getByRole('button', { name: '确认成员授权', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0);
  const recipient = await browser.newContext({ viewport: page.viewportSize()! }); const memberPage = await recipient.newPage();
  try {
    await login(memberPage, f.member); await memberPage.goto(`${consoleURL}/t/${f.tenant}/profile`); await expect(memberPage.getByRole('button', { name: '编辑资料', exact: true })).toBeVisible();
    const snap = await (await recipient.request.get(`${api}/tenants/${f.tenant}/context`)).json(); expect(snap.permission_sources[P.TenantProfileRead]).toHaveLength(3);
    await page.goto(`${consoleURL}/t/${f.tenant}/roles/${roleId}`); await roleAction(page, '停用角色'); await page.getByRole('button', { name: '预览变更影响' }).click(); await expect(page.getByRole('region', { name: '变更影响预览' })).toContainText('实际失权人数'); await page.getByLabel('操作原因').fill('暂停此角色'); await page.getByRole('button', { name: '确认授权变更', exact: true }).click(); await expect(page.getByRole('heading', { name: '本地资料维护', exact: true })).toBeVisible();
    expect((await (await recipient.request.get(`${api}/tenants/${f.tenant}/context`)).json()).permissions).not.toContain(P.TenantProfileUpdate); expect((await (await recipient.request.get(`${api}/tenants/${f.tenant}/context`)).json()).permissions).toContain(P.TenantProfileRead);
    await roleAction(page, '删除租户角色'); await page.getByRole('button', { name: '预览变更影响' }).click(); await expect(page.getByText(/角色仍有未撤销绑定/)).toBeVisible(); await expect(page.getByRole('button', { name: '确认授权变更', exact: true })).toBeDisabled(); await page.getByRole('button', { name: '取消', exact: true }).click(); await expect(page.getByRole('dialog').getByRole('button', { name: '本地资料维护的操作', exact: true })).toBeFocused();
    await page.goto(`${consoleURL}/t/${f.tenant}/members/${f.membership}`); await page.getByRole('button', { name: '配置成员角色' }).click(); await page.getByRole('button', { name: '清空全部角色' }).click(); await page.getByRole('button', { name: '预览成员授权影响' }).click(); await page.getByLabel('操作原因').fill('明确清空职责'); await page.getByRole('button', { name: '确认成员授权', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0);
    await memberPage.reload(); await expect(memberPage.getByRole('heading', { name: '本人租户授权', exact: true })).toBeVisible(); await expect(memberPage.getByText('当前未获业务授权，请联系租户管理员。')).toBeVisible(); expect((await recipient.request.get(`${api}/tenants/${f.tenant}/profile`)).status()).toBe(403);
    await memberPage.goto(`${consoleURL}/t/${f.tenant}/roles`); await expect(memberPage.getByText('无权查看租户角色目录。', { exact: false })).toBeVisible();
    await page.goto(`${consoleURL}/t/${f.tenant}/roles`); await page.getByRole('button', { name: '创建租户角色', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('dialog')).toBeVisible(); await page.getByRole('button', { name: '取消', exact: true }).click(); await expect(page.getByRole('button', { name: '创建租户角色', exact: true })).toBeFocused();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('tenant-roles-light.png'), fullPage: true }); await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色', exact: true }).click(); await page.screenshot({ path: info.outputPath('tenant-roles-dark.png'), fullPage: true });
  } finally { await recipient.close(); }
});

test('role HTTP: strict bodies, presets scope, stale preview and immediate cookie authorization', async ({ request }) => {
  const f = await prepare(request); const h = await signIn(request, f.administrator.username); const base = `${api}/tenants/${f.tenant}`;
  const input = { name: '严格校验职责', permission_codes: [], client_request_id: randomUUID(), reason: '严格校验' };
  for (const field of ['builtin_key', 'owner_tenant_id', 'binding_scope']) expect((await request.post(`${base}/roles`, { headers: h, data: { ...input, [field]: 'forged' } })).status()).toBe(400);
  for (const permission_codes of [[P.AccountProfileRead], [P.TenantRoleRead], [P.TenantProfileRead, P.TenantProfileRead], ['unknown.*'], null]) expect((await request.post(`${base}/roles`, { headers: h, data: { ...input, permission_codes } })).status()).toBe(400);
  expect((await request.post(`${base}/roles`, { headers: { Origin: origin }, data: input })).status()).toBe(403);
  const created = await request.post(`${base}/roles`, { headers: h, data: input }); expect(created.status()).toBe(201); const r = (await created.json()).role;
  expect((await request.post(`${base}/roles`, { headers: h, data: input })).status()).toBe(200);
  expect((await request.post(`${base}/roles`, { headers: h, data: { ...input, name: '不同内容' } })).status()).toBe(409);
  expect((await request.put(`${base}/roles/${r.id}/permissions`, { headers: h, data: { expected_version: r.version, permission_codes: [], preview_hash: 'forged', reason: '没有预览' } })).status()).toBe(409);
  const platform = await signIn(request, 'e2e_owner'); expect((await request.patch(`${api}/access/roles/${r.id}`, { headers: platform, data: { name: '越权', description: '', reason: '越权', expected_version: r.version } })).status()).toBe(404);
  const presets = await (await request.get(`${api}/access/tenant-role-presets`)).json(); expect(presets.items).toHaveLength(6); expect(presets.items.every((p: { owner_tenant_id: string }) => !p.owner_tenant_id)).toBe(true);
});

test('tenant role drafts and late previews disappear after administrator handover', async ({ page, request }) => {
  const f = await prepare(request); const h = await signIn(request, f.administrator.username); const base = `${api}/tenants/${f.tenant}`;
  const r = await (await request.post(`${base}/roles`, { headers: h, data: { name: '旧上下文职责', permission_codes: [P.TenantProfileRead], client_request_id: randomUUID(), reason: '迟到预览测试' } })).json();
  await login(page, f.administrator); await page.goto(`${consoleURL}/t/${f.tenant}/roles/${r.role.id}`); await page.getByRole('button', { name: '配置角色权限', exact: true }).click(); await page.getByRole('checkbox', { name: '修改租户资料', exact: false }).check(); await page.getByLabel('操作原因').fill('旧租户草稿不可恢复');
  let release!: () => void; const held = new Promise<void>((resolve) => { release = resolve; }); let fetched!: () => void; const ready = new Promise<void>((resolve) => { fetched = resolve; });
  await page.route(`**/tenants/${f.tenant}/roles/${r.role.id}/permissions/preview?*`, async (route) => { const response = await route.fetch(); fetched(); await held; try { await route.fulfill({ response }); } catch { /* Navigation may cancel this completed response. */ } });
  await page.getByRole('button', { name: '预览变更影响' }).click(); await ready;
  const owner = await signIn(request, 'e2e_owner'); const target = (await (await request.get(`${api}/access/accounts?q=${f.member.username}`)).json()).items[0].id as string; const tenant = await (await request.get(`${api}/platform/tenants/${f.tenant}`)).json();
  expect((await request.post(`${api}/platform/tenants/${f.tenant}/administrator-transfer`, { headers: owner, data: { new_administrator_account_id: target, expected_version: tenant.version, reason: '验证交接后清理草稿' } })).status()).toBe(200);
  await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('heading', { name: '当前租户不可访问', exact: true })).toBeVisible(); release();
  await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByText('旧租户草稿不可恢复', { exact: true })).toHaveCount(0); await expect(page.getByRole('region', { name: '变更影响预览' })).toHaveCount(0);
});

test('tenant role table keeps filters and pagination when its detail panel closes', async ({ page, request }, info) => {
  const f = await prepare(request); const headers = await signIn(request, f.administrator.username); const prefix = `分页职责_${randomBytes(3).toString('hex')}`;
  for (let i = 0; i < 12; i++) expect((await request.post(`${api}/tenants/${f.tenant}/roles`, { headers, data: { name: `${prefix}_${i + 1}`, description: '租户职责列表验收', permission_codes: [], client_request_id: randomUUID(), reason: '准备分页验证' } })).status()).toBe(201);
  await login(page, f.administrator); await page.goto(`${consoleURL}/t/${f.tenant}/roles`); await page.getByLabel('搜索角色名称', { exact: true }).fill(prefix);
  const table = page.getByRole('table', { name: '租户角色列表', exact: true }); const pager = page.getByRole('navigation', { name: '租户角色列表分页', exact: true });
  await expect(table.getByRole('row')).toHaveCount(11); await pager.getByRole('button', { name: '下一页', exact: true }).click(); await expect(table.getByRole('row')).toHaveCount(3);
  const link = table.getByRole('row').nth(1).getByRole('link'); await link.click(); await expect(page.getByRole('dialog')).toBeVisible();
  await page.getByRole('tab', { name: '绑定成员', exact: true }).click(); await expect(page.getByText('该角色尚未绑定成员。', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: '基本资料', exact: true }).click(); await expect(page.getByRole('dialog').getByRole('button', { name: '编辑名称与说明', exact: true })).toBeVisible();
  await closeRolePanel(page); await expect(page).toHaveURL(`${consoleURL}/t/${f.tenant}/roles`); await expect(pager.getByRole('status')).toContainText('第 2 页'); await expect(page.getByLabel('搜索角色名称', { exact: true })).toHaveValue(prefix);
  await page.getByRole('button', { name: '角色来源', exact: true }).click(); await page.getByRole('menuitemradio', { name: '平台预置', exact: true }).click(); await expect(page.getByText('没有符合条件的角色。', { exact: true })).toBeVisible(); await expect(pager.getByRole('status')).toContainText('第 1 页');
  await page.getByLabel('搜索角色名称', { exact: true }).fill(''); await expect(table.getByRole('row')).toHaveCount(7);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('tenant-role-table.png'), fullPage: true });
});

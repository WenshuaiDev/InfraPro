import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, createAccount, login, password, consoleURL, csrf } from './fixtures';
import { navigateTo } from '../helpers/navigation';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
const api = new URL('/api/v1', consoleURL).href;
async function owner(request: APIRequestContext) { const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request) }; expect((await request.post(`${api}/auth/login`, { headers, data: { username: 'e2e_owner', password } })).status()).toBe(200); return headers; }
async function accountID(request: APIRequestContext, username: string) { const response = await request.get(`${api}/access/accounts?q=${username}`); expect(response.status()).toBe(200); return (await response.json()).items[0] as { id: string; management_version: string }; }
async function create(request: APIRequestContext, headers: Record<string, string>, admin: string, label: string) { const code = `t-${randomBytes(5).toString('hex')}`; const response = await request.post(`${api}/platform/tenants`, { headers, data: { name: label, code, description: '', reason: '浏览器租户验收', administrator_account_id: admin, client_request_id: randomUUID() } }); expect(response.status()).toBe(201); return { ...(await response.json()) as { tenant_id: string; version: string }, code }; }
async function refresh(page: Page) { const response = page.waitForResponse((r) => r.url().endsWith('/auth/me')); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await response; }
async function save(page: Page) { const dialog = page.getByRole('dialog'); await dialog.getByLabel('变更原因').fill('租户浏览器验收原因'); await dialog.getByLabel('变更原因').press('Enter'); await expect(dialog).toBeHidden(); }
async function select(page: Page, username: string) { const dialog = page.getByRole('dialog'); await dialog.getByLabel('搜索接任账号').fill(username); await dialog.getByRole('button', { name: `浏览器验收（${username}）`, exact: true }).focus(); await page.keyboard.press('Enter'); }

test('tenant UI creates, edits, transfers with old sessions and protects account disable', async ({ page, request, browser }, info) => {
  const alice = await createAccount(request, false); const bob = await createAccount(request, false); const headers = await owner(request); const a = await accountID(request, alice.username); const b = await accountID(request, bob.username);
  const guestContext = await browser.newContext({ viewport: info.project.use.viewport }); const bobContext = await browser.newContext({ viewport: info.project.use.viewport });
  try {
    const guest = await guestContext.newPage(); const successor = await bobContext.newPage(); await login(successor, bob); await login(guest, alice); await navigateTo(guest, '我的租户'); await expect(guest.getByText('尚未加入租户，请联系平台管理人员')).toBeVisible();
    await login(page, { username: 'e2e_owner', password }); await navigateTo(page, '租户管理'); await page.getByRole('button', { name: '创建租户', exact: true }).click();
    const name = `租户甲_${randomBytes(4).toString('hex')}`; const code = `ui-${randomBytes(5).toString('hex')}`;
    await page.getByLabel('租户名称', { exact: true }).fill(name); await page.getByLabel('租户编号', { exact: true }).fill(code); await page.getByLabel('租户说明', { exact: true }).fill('真实租户资料'); await select(page, alice.username); await save(page);
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible(); const tenantId = new URL(page.url()).pathname.split('/').at(-1)!;
    const beta = await create(request, headers, b.id, '租户乙');
    await guest.reload(); await guest.getByRole('link', { name: '进入租户', exact: true }).click(); await expect(guest.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
    await guest.getByText('当前租户权限', { exact: true }).click(); await expect(guest.getByText('角色：租户管理员（租户范围）')).toBeVisible();
    expect((await guestContext.request.get(`${api}/tenants/${beta.tenant_id}/profile`)).status()).toBe(404); expect((await request.get(`${api}/tenants/${tenantId}/profile`)).status()).toBe(404);
    await guest.getByRole('button', { name: '编辑资料', exact: true }).click(); await guest.getByLabel('租户名称', { exact: true }).fill(`${name}已改`); await save(guest); await guest.reload(); await expect(guest.getByText(`${name}已改`, { exact: true })).toBeVisible();
    await guest.screenshot({ path: info.outputPath('tenant-profile.png'), fullPage: true }); expect(await guest.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.reload(); await page.getByRole('button', { name: '交接管理员', exact: true }).click(); await select(page, bob.username); await save(page);
    expect((await guestContext.request.get(`${api}/tenants/${tenantId}/profile`)).status()).toBe(404); await refresh(guest); await expect(guest.getByRole('heading', { name: '当前租户不可访问' })).toBeVisible();
    expect((await bobContext.request.get(`${api}/tenants/${tenantId}/profile`)).status()).toBe(200); await successor.goto(`${consoleURL}/t/${tenantId}/profile`); await expect(successor.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
    await navigateTo(successor, '租户操作记录'); await expect(successor.getByRole('heading', { name: '交接管理员', exact: true })).toBeVisible(); await successor.getByLabel('搜索操作或原因').fill('租户浏览器验收原因'); await expect(successor.getByRole('heading', { name: '交接管理员', exact: true })).toBeVisible();
    await navigateTo(page, '账号管理'); await page.getByLabel('搜索用户名或显示名称').fill(bob.username); await page.getByRole('button', { name: '浏览器验收', exact: true }).click(); await expect(page.getByRole('heading', { name: '当前租户任职' })).toBeVisible();
    await page.getByRole('button', { name: '停用账号', exact: true }).click(); await page.getByRole('dialog').getByLabel('变更原因').fill('必须先交接'); await page.getByRole('button', { name: '确认提交变更' }).click(); await expect(page.getByRole('dialog').getByText(/请先交接该账号负责的 2 个租户/)).toBeVisible();
    expect((await guestContext.request.get(`${api}/auth/me`)).status()).toBe(200);
    expect(a.id).not.toBe(b.id);
  } finally { await guestContext.close(); await bobContext.close(); }
});

test('tenant navigation rejects late reads and writes and keeps tabs independent', async ({ page, context, request }, info) => {
  const account = await createAccount(request, false); const headers = await owner(request); const target = await accountID(request, account.username); const a = await create(request, headers, target.id, '隔离租户 A'); await create(request, headers, target.id, '隔离租户 B');
  await login(page, account);
  let release = () => {}; let ready = () => {}; const held = new Promise<void>((r) => { release = r; }); const fetched = new Promise<void>((r) => { ready = r; });
  await page.route(`**/api/v1/tenants/${a.tenant_id}/profile`, async (route) => { const response = await route.fetch(); ready(); await held; await route.fulfill({ response }).catch(() => undefined); });
  try {
    await page.goto(`${consoleURL}/t/${a.tenant_id}/profile`); await fetched; await navigateTo(page, '切换租户'); await page.getByRole('article').filter({ hasText: '隔离租户 B' }).getByRole('link', { name: '进入租户' }).click();
    await expect(page.getByText('隔离租户 B', { exact: true })).toBeVisible(); release(); await expect(page.getByText('隔离租户 A', { exact: true })).toHaveCount(0);
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }); }
  const tab = await context.newPage(); await tab.goto(`${consoleURL}/t/${a.tenant_id}/profile`); await expect(tab.getByText('隔离租户 A', { exact: true })).toBeVisible(); await expect(page.getByText('隔离租户 B', { exact: true })).toBeVisible();
  await tab.getByRole('button', { name: '编辑资料', exact: true }).click(); await tab.getByLabel('租户名称', { exact: true }).fill('A 未保存输入');
  tab.once('dialog', (dialog) => dialog.dismiss()); await tab.keyboard.press('Escape'); await expect(tab.getByLabel('租户名称', { exact: true })).toHaveValue('A 未保存输入');
  // Unrelated graph revisions and focus refresh preserve this tenant's draft.
  await create(request, headers, target.id, '无关租户'); await refresh(tab); await expect(tab.getByLabel('租户名称', { exact: true })).toHaveValue('A 未保存输入');
  let writeRelease = () => {}; let writeReady = () => {}; const writeHeld = new Promise<void>((r) => { writeRelease = r; }); const committed = new Promise<void>((r) => { writeReady = r; });
  await tab.route(`**/api/v1/tenants/${a.tenant_id}/profile`, async (route) => { if (route.request().method() !== 'PATCH') return route.continue(); const response = await route.fetch(); writeReady(); await writeHeld; await route.fulfill({ response }).catch(() => undefined); });
  try {
    await tab.getByLabel('变更原因').fill('迟到写响应'); await tab.getByRole('button', { name: '确认提交', exact: true }).click(); await committed;
    tab.once('dialog', (dialog) => dialog.accept()); await tab.keyboard.press('Escape'); await navigateTo(tab, '切换租户'); await tab.getByRole('article').filter({ hasText: '隔离租户 B' }).getByRole('link', { name: '进入租户' }).click(); writeRelease();
    await expect(tab.getByText('隔离租户 B', { exact: true })).toBeVisible(); await expect(tab.getByText('变更已保存。')).toHaveCount(0); await expect(tab.getByText('A 未保存输入', { exact: true })).toHaveCount(0);
    const actual = await (await context.request.get(`${api}/tenants/${a.tenant_id}/profile`)).json(); expect(actual.name).toBe('A 未保存输入');
    expect(await tab.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await tab.screenshot({ path: info.outputPath('tenant-switch.png'), fullPage: true });
  } finally { writeRelease(); await tab.unrouteAll({ behavior: 'wait' }); await tab.close(); }
});

test('tenant HTTP contracts enforce scoped permissions, narrow candidates and strict payloads', async ({ page, request, browser }) => {
  const member = await createAccount(request, false); const headers = await owner(request); const target = await accountID(request, member.username);
  let response = await request.post(`${api}/access/roles`, { headers, data: { name: `仅建档_${randomBytes(4).toString('hex')}`, description: '', reason: '范围测试' } }); const role = await response.json();
  response = await request.put(`${api}/access/roles/${role.id}/permissions`, { headers, data: { expected_version: role.version, permission_codes: [P.PlatformTenantCreate], reason: '范围测试' } }); expect(response.status()).toBe(200);
  response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: target.management_version, role_ids: [role.id], reason: '范围测试' } }); expect(response.status()).toBe(200);
  const isolated = await browser.newContext();
  try {
    const r = isolated.request; const h = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(r) }; expect((await r.post(`${api}/auth/login`, { headers: h, data: member })).status()).toBe(200);
    for (const path of ['/platform/tenants', '/access/accounts', '/access/audit-events']) expect((await r.get(api + path)).status()).toBe(403);
    response = await r.get(`${api}/platform/tenant-admin-candidates?q=${member.username}`); expect(response.status()).toBe(200); const candidates = (await response.json()).items; expect(Object.keys(candidates[0]).sort()).toEqual(['display_name', 'id', 'username']);
    expect((await r.get(`${api}/platform/tenant-admin-candidates?q=x`)).status()).toBe(400);
    const input = { name: '无平台查看权限建档', code: `n-${randomBytes(4).toString('hex')}`, description: '', reason: '最小授权', administrator_account_id: target.id, client_request_id: randomUUID() };
    for (const extra of [{ status: 'active' }, { roles: [] }, { tenant_id: randomUUID() }]) expect((await r.post(`${api}/platform/tenants`, { headers: h, data: { ...input, ...extra } })).status()).toBe(400);
    response = await r.post(`${api}/platform/tenants`, { headers: h, data: input }); expect(response.status()).toBe(201); const result = await response.json(); expect(Object.keys(result).sort()).toEqual(['authorization_revision', 'replayed', 'tenant_id', 'version']);
    expect((await r.get(`${api}/platform/tenants/${result.tenant_id}`)).status()).toBe(403); expect((await r.get(`${api}/tenants/${result.tenant_id}/profile`)).status()).toBe(200);
    expect((await r.get(`${api}/tenants/${result.tenant_id}/profile?tenant_id=${randomUUID()}`)).status()).toBe(400);
    expect((await r.patch(`${api}/tenants/${result.tenant_id}/profile`, { headers: h, data: { name: 'X', code: input.code, description: '', expected_version: '1', reason: '伪造', tenant_id: randomUUID() } })).status()).toBe(400);
    expect((await r.get(`${api}/tenants/${randomUUID()}/profile`)).status()).toBe(404);
    expect((await r.post(`${api}/platform/tenants/${result.tenant_id}/administrator-transfer`, { headers: h, data: { new_administrator_account_id: target.id, expected_version: '1', reason: '无权' } })).status()).toBe(403);
    await login(page, member); await page.goto(`${consoleURL}/platform/tenants`); await expect(page.getByText(/当前未配置租户查看权限/)).toBeVisible();
    await page.getByRole('button', { name: '创建租户', exact: true }).click(); await page.getByLabel('租户名称', { exact: true }).fill('仅创建权限结果'); const narrowCode = `only-${randomBytes(4).toString('hex')}`; await page.getByLabel('租户编号', { exact: true }).fill(narrowCode); await select(page, member.username);
    let releaseCreate = () => {}; let createReady = () => {}; const createHeld = new Promise<void>((r) => { releaseCreate = r; }); const createFetched = new Promise<void>((r) => { createReady = r; });
    await page.route('**/api/v1/platform/tenants', async (route) => { if (route.request().method() !== 'POST') return route.continue(); const response = await route.fetch(); createReady(); await createHeld; await route.fulfill({ response }).catch(() => undefined); });
    try {
      await page.getByLabel('变更原因').fill('慢响应成功结果'); await page.getByLabel('变更原因').press('Enter'); await createFetched;
      await expect(page.getByLabel('租户名称', { exact: true })).toBeDisabled(); await expect(page.getByLabel('搜索接任账号')).toBeDisabled(); releaseCreate(); await expect(page.getByRole('dialog')).toBeHidden();
    } finally { releaseCreate(); await page.unrouteAll({ behavior: 'wait' }); }
    await expect(page.getByRole('status').filter({ hasText: `仅创建权限结果（${narrowCode}）` })).toContainText(`管理员：浏览器验收（${member.username}）`);
    const auditor = await request.post(`${api}/access/roles`, { headers, data: { name: `审计_${randomBytes(4).toString('hex')}`, description: '', reason: '审计隔离' } }); const ar = await auditor.json();
    expect((await request.put(`${api}/access/roles/${ar.id}/permissions`, { headers, data: { expected_version: ar.version, permission_codes: [P.PlatformAuditRead], reason: '审计隔离' } })).status()).toBe(200);
    const latest = await accountID(request, member.username); expect((await request.put(`${api}/access/accounts/${target.id}/roles`, { headers, data: { expected_version: latest.management_version, role_ids: [ar.id], reason: '审计隔离' } })).status()).toBe(200);
    response = await r.get(`${api}/access/audit-events`); expect(response.status()).toBe(200); for (const ev of (await response.json()).items) expect(ev.tenant_id).toBe('');
    expect((await r.get(`${api}/access/audit-events?tenant_id=${result.tenant_id}`)).status()).toBe(403);
  } finally { await isolated.close(); }
});

test('conflicts and temporary tenant outages preserve drafts, confirmed loss clears them', async ({ page, request }) => {
  const account = await createAccount(request, false); const successor = await createAccount(request, false); const headers = await owner(request); const target = await accountID(request, account.username); const next = await accountID(request, successor.username); const a = await create(request, headers, target.id, '草稿租户');
  await login(page, account); await page.goto(`${consoleURL}/t/${a.tenant_id}/profile`); await page.getByRole('button', { name: '编辑资料', exact: true }).click(); await page.getByLabel('租户名称', { exact: true }).fill('本地草稿'); await page.getByLabel('变更原因').fill('冲突保留输入');
  let response = await request.patch(`${api}/platform/tenants/${a.tenant_id}`, { headers, data: { expected_version: '1', name: '服务器新名称', reason: '制造并发版本' } }); expect(response.status()).toBe(200);
  await page.getByRole('button', { name: '确认提交', exact: true }).click(); await expect(page.getByRole('dialog').getByText(/记录已被修改/)).toBeVisible(); await expect(page.getByLabel('租户名称', { exact: true })).toHaveValue('本地草稿');
  await page.getByRole('button', { name: '加载最新内容并比较' }).click(); await expect(page.getByRole('dialog').getByText('服务器最新内容（本地输入已保留）')).toBeVisible(); await page.getByRole('button', { name: '确认提交', exact: true }).click(); await expect(page.getByRole('dialog')).toBeHidden(); await expect(page.getByRole('button', { name: '编辑资料', exact: true })).toBeFocused();
  await page.getByRole('button', { name: '编辑资料', exact: true }).click(); await page.getByLabel('租户名称', { exact: true }).fill('故障期间保留');
  let unavailable = true;
  await page.route(`**/api/v1/tenants/${a.tenant_id}/context`, async (route) => { if (unavailable) await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '测试暂时故障', request_id: 'fixture' }) }); else await route.continue(); });
  await refresh(page); await expect(page.getByRole('dialog').getByText('租户状态暂不可确认，输入已保留，恢复后可继续提交。')).toBeVisible(); await expect(page.getByLabel('租户名称', { exact: true })).toHaveValue('故障期间保留'); await expect(page.getByRole('button', { name: '确认提交', exact: true })).toBeDisabled();
  unavailable = false; await refresh(page); await expect(page.getByRole('button', { name: '确认提交', exact: true })).toBeEnabled(); await expect(page.getByLabel('租户名称', { exact: true })).toHaveValue('故障期间保留');
  response = await request.post(`${api}/platform/tenants/${a.tenant_id}/administrator-transfer`, { headers, data: { expected_version: '3', new_administrator_account_id: next.id, reason: '编辑中撤权' } }); expect(response.status()).toBe(200);
  await refresh(page); await expect(page.getByRole('heading', { name: '当前租户不可访问' })).toBeVisible(); await expect(page.getByLabel('租户名称', { exact: true })).toHaveCount(0); await expect(page.getByText('故障期间保留', { exact: true })).toHaveCount(0);
});

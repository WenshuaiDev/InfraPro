import { randomBytes, randomUUID } from 'node:crypto';
import { test, expect, consoleURL, createAccount, csrf, login, password } from './fixtures';
import type { APIRequestContext, Page } from '@playwright/test';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { navigateTo } from '../helpers/navigation';

test.use({ trace: 'off' }); // One-time test credentials must not be retained in network traces.
const api = new URL('/api/v1', consoleURL).href;
async function signIn(request: APIRequestContext, username: string) {
  const headers = { Origin: new URL(consoleURL).origin, 'X-CSRF-Token': await csrf(request, new URL(consoleURL).origin) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200); return headers;
}
async function prepare(request: APIRequestContext) {
  const administrator = await createAccount(request, false); const member = await createAccount(request, false); const outsider = await createAccount(request, false);
  const headers = await signIn(request, 'e2e_owner');
  const account = async (username: string) => (await (await request.get(`${api}/access/accounts?q=${username}`)).json()).items[0] as { id: string; management_version: string };
  const admin = await account(administrator.username); const target = await account(member.username); const other = await account(outsider.username);
  const create = async (name: string) => { const r = await request.post(`${api}/platform/tenants`, { headers, data: { name, code: `tm-${randomBytes(6).toString('hex')}`, description: '成员管理验收', administrator_account_id: admin.id, client_request_id: randomUUID(), reason: '浏览器准备' } }); expect(r.status()).toBe(201); return (await r.json()).tenant_id as string; };
  return { administrator, member, outsider, headers, admin, target, other, a: await create('成员验收 A'), b: await create('成员验收 B') };
}
async function invite(request: APIRequestContext, headers: Record<string, string>, tenant: string, target: string) {
  const r = await request.post(`${api}/tenants/${tenant}/member-invitations`, { headers, data: { target_account_id: target, client_request_id: randomUUID(), reason: '测试邀请' } }); expect(r.status()).toBe(201); return await r.json() as { link: string; invitation: { id: string; version: string } };
}
function credential(link: string) { return new URLSearchParams(new URL(link).hash.slice(1)).get('token'); }
function row(page: Page, name: string) { return page.getByRole(page.viewportSize()!.width < 768 ? 'article' : 'row').filter({ hasText: name }); }
async function uiInvite(page: Page, tenant: string, username: string) {
  await page.goto(`${consoleURL}/t/${tenant}/invitations`); await page.getByRole('button', { name: '邀请成员', exact: true }).click();
  await page.getByLabel('完整用户名').fill(username); await page.getByRole('button', { name: '定位账号' }).click(); await expect(page.getByRole('status').filter({ hasText: username })).toBeVisible();
  await page.getByLabel('操作原因').fill('人工交付链接'); await page.getByRole('button', { name: '创建成员邀请', exact: true }).click();
  const field = page.getByLabel('完整邀请链接'); await expect(field).toBeVisible(); const link = await field.inputValue();
  await page.getByRole('button', { name: '关闭链接', exact: true }).click(); await expect(field).toHaveCount(0); return link;
}

test('member journey: explicit login confirmation, two tenants, suspension, removal and new relation', async ({ page, request, browser }, info) => {
  const f = await prepare(request); await login(page, f.administrator);
  const link = await uiInvite(page, f.a, f.member.username);
  const recipient = await browser.newContext({ viewport: page.viewportSize()! }); const member = await recipient.newPage();
  const wrong = await browser.newContext({ viewport: page.viewportSize()! }); const wrongPage = await wrong.newPage();
  try {
    await login(wrongPage, f.outsider); await wrongPage.goto(link); await expect(wrongPage.getByRole('heading', { name: '邀请当前不可使用' })).toBeVisible(); await expect(wrongPage.getByText('成员验收 A', { exact: true })).toHaveCount(0); await expect(wrongPage.getByText(f.member.username, { exact: true })).toHaveCount(0);
    await member.goto(link); await expect(member).not.toHaveURL(/token=/); await member.getByRole('link', { name: '登录并继续' }).click(); await member.getByLabel('用户名', { exact: true }).fill(f.member.username); await member.getByLabel('密码', { exact: true }).fill(password); await member.getByRole('button', { name: '登录', exact: true }).click();
    await expect(member.getByRole('button', { name: '确认加入', exact: true })).toBeVisible();
    expect((await (await recipient.request.get(`${api}/account/tenants`)).json()).items).toHaveLength(0);
    await member.getByRole('link', { name: '取消并选择工作空间', exact: true }).click();
    await expect(member.getByRole('menu', { name: '切换工作空间', exact: true }).getByLabel('工作空间类型')).toHaveValue('tenants');
    expect((await (await recipient.request.get(`${api}/account/tenants`)).json()).items).toHaveLength(0);
    await member.goto(`${consoleURL}/tenant-invite`);
    await expect(member.getByRole('heading', { name: '请重新打开原邀请链接', exact: true })).toBeVisible();
    await member.goto(link);
    await expect(member.getByRole('button', { name: '确认加入', exact: true })).toBeVisible();
    const persistent = await member.evaluate(() => [...Object.values(localStorage), ...Object.values(sessionStorage)].join('')); expect(persistent.includes(credential(link)!)).toBe(false);
    await member.getByRole('button', { name: '确认加入', exact: true }).click(); await member.getByRole('link', { name: '进入租户', exact: true }).click(); await expect(member.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
    for (const label of ['编辑资料', '成员管理', '成员邀请', '租户操作记录']) await expect(member.getByRole('button', { name: label, exact: true }).or(member.getByRole('link', { name: label, exact: true }))).toHaveCount(0);
    await member.getByText('当前租户权限', { exact: true }).click(); await expect(member.getByText('角色：租户普通成员（租户范围）')).toBeVisible();
    const profile = await (await recipient.request.get(`${api}/tenants/${f.a}/profile`)).json(); expect(Object.keys(profile).sort()).toEqual(['administrator', 'administrator_available', 'code', 'description', 'id', 'name', 'updated_at', 'version']);
    const first = (await (await recipient.request.get(`${api}/tenants/${f.a}/context`)).json()).membership_id as string;
    const secondLink = await uiInvite(page, f.b, f.member.username); await member.goto(secondLink); await member.getByRole('button', { name: '确认加入', exact: true }).click();
    await page.goto(`${consoleURL}/t/${f.a}/members`); const target = row(page, f.member.username); await target.getByRole('button', { name: '暂停成员', exact: true }).click(); await expect(page.getByRole('alertdialog')).toContainText('将失去本租户访问'); await page.getByLabel('操作原因').fill('重新核验时保留的暂停原因'); const checked = page.waitForResponse((r) => r.url().endsWith(`/tenants/${f.a}/context`)); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await checked; await expect(page.getByLabel('操作原因')).toHaveValue('重新核验时保留的暂停原因'); await page.getByRole('button', { name: '取消', exact: true }).click(); await expect(target.getByRole('button', { name: '暂停成员', exact: true })).toBeFocused();
    await target.getByRole('button', { name: '暂停成员', exact: true }).click(); await page.getByLabel('操作原因').fill('暂停 A，保留 B'); await page.getByRole('button', { name: '确认操作', exact: true }).click(); await expect(target).toContainText('已暂停');
    expect((await recipient.request.get(`${api}/tenants/${f.a}/profile`)).status()).toBe(404); expect((await recipient.request.get(`${api}/tenants/${f.b}/profile`)).status()).toBe(200);
    await target.getByRole('button', { name: '恢复成员', exact: true }).click(); await page.getByLabel('操作原因').fill('恢复原关系'); await page.getByRole('button', { name: '确认操作', exact: true }).click(); await expect(target).toContainText('有效');
    expect((await (await recipient.request.get(`${api}/tenants/${f.a}/context`)).json()).membership_id).toBe(first);
    await target.getByRole('button', { name: '移除成员', exact: true }).click(); await page.getByLabel('操作原因').fill('结束本次关系'); const removal = page.waitForResponse((r) => r.url().endsWith(`/members/${first}/remove`) && r.request().method() === 'POST'); await page.getByRole('button', { name: '确认操作', exact: true }).click(); expect((await removal).status()).toBe(200); await expect(page.getByRole('alertdialog')).toHaveCount(0); await expect(target).toHaveCount(0); expect((await recipient.request.get(`${api}/tenants/${f.a}/profile`)).status()).toBe(404);
    const newLink = await uiInvite(page, f.a, f.member.username); await member.goto(newLink); await member.getByRole('button', { name: '确认加入', exact: true }).click();
    const next = (await (await recipient.request.get(`${api}/tenants/${f.a}/context`)).json()).membership_id as string; expect(next).not.toBe(first);
    await page.goto(`${consoleURL}/t/${f.a}/members/${first}`); await expect(page.getByRole('heading', { name: '本次角色授予记录' })).toBeVisible(); await expect(page.getByText(/已撤销于/)).toBeVisible(); await expect(page.getByText(/结束本次关系/).first()).toBeVisible();
    await page.goto(`${consoleURL}/t/${f.a}/members`); await page.getByLabel('成员状态', { exact: true }).selectOption('all'); await page.getByLabel('搜索本租户成员').fill(f.member.username); await expect(row(page, f.member.username)).toHaveCount(2);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('membership-history-light.png'), fullPage: true, animations: 'disabled' });
    await page.evaluate(() => document.documentElement.classList.add('dark')); await page.screenshot({ path: info.outputPath('membership-history-dark.png'), fullPage: true, animations: 'disabled' });
    const suspend = row(page, f.member.username).filter({ hasText: '有效' }).getByRole('button', { name: '暂停成员', exact: true });
    await suspend.focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('alertdialog')).toBeVisible(); await page.getByRole('button', { name: '取消', exact: true }).focus(); await page.keyboard.press('Enter'); await expect(suspend).toBeFocused();
    if (info.project.name === 'mobile') {
      await page.setViewportSize({ width: 667, height: 375 }); await page.evaluate(() => { document.documentElement.style.fontSize = '24px'; });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('membership-landscape-large-text.png'), fullPage: true, animations: 'disabled' });
    }
  } finally { await recipient.close().catch(() => undefined); await wrong.close().catch(() => undefined); }
});

test('membership HTTP matrix rejects forged scope and fields, protects administrator and binds recipient', async ({ request, browser }) => {
  const f = await prepare(request); const administrator = await browser.newContext(); const recipient = await browser.newContext();
  try {
    const h = await signIn(administrator.request, f.administrator.username); const rh = await signIn(recipient.request, f.member.username);
    const base = `${api}/tenants/${f.a}`; const adminContext = await (await administrator.request.get(`${base}/context`)).json();
    const l = await invite(administrator.request, h, f.a, f.target.id); const body = { token: credential(l.link) };
    const routes: [string, object][] = [[`${base}/member-candidates/resolve`, { username: f.member.username }], [`${base}/member-invitations`, { target_account_id: f.target.id, reason: 'x', client_request_id: randomUUID() }], [`${base}/member-invitations/${l.invitation.id}/revoke`, { expected_version: '1', reason: 'x' }], [`${base}/member-invitations/${l.invitation.id}/regenerate`, { expected_version: '1', reason: 'x', client_request_id: randomUUID() }], ...['suspend', 'resume', 'remove'].map((action): [string, object] => [`${base}/members/${adminContext.membership_id}/${action}`, { expected_version: '1', reason: 'x' }])];
    for (const [path, data] of routes) {
      expect((await administrator.request.post(path, { headers: { Origin: h.Origin! }, data })).status()).toBe(403);
      expect((await administrator.request.post(path, { headers: { ...h, Origin: 'http://invalid.example' }, data })).status()).toBe(403);
      expect((await administrator.request.post(path, { headers: h, data: { ...data, tenant_id: f.b, force: true } })).status()).toBe(400);
      expect((await recipient.request.post(path, { headers: rh, data })).status()).toBe(404);
    }
    for (const action of ['suspend', 'remove']) { const r = await administrator.request.post(`${base}/members/${adminContext.membership_id}/${action}`, { headers: h, data: { expected_version: '1', reason: 'protected' } }); expect((await r.json()).code).toBe('tenant_admin_transfer_required'); }
    for (const action of ['inspect', 'accept']) {
      const path = `${api}/account/tenant-invitations/${action}`; const data = action === 'inspect' ? body : { ...body, expected_version: '1' };
      expect((await recipient.request.post(path, { headers: { Origin: rh.Origin! }, data })).status()).toBe(403);
      expect((await recipient.request.post(path, { headers: rh, data: { ...data, roles: ['tenant_admin'] } })).status()).toBe(400);
      expect((await administrator.request.post(path, { headers: h, data })).status()).toBe(404);
    }
    for (const path of ['/members', `/members/${adminContext.membership_id}`, '/member-invitations', `/member-invitations/${l.invitation.id}`]) expect((await request.get(base + path)).status()).toBe(404);
    expect((await administrator.request.get(`${api}/tenants/${f.b}/member-invitations/${l.invitation.id}`)).status()).toBe(404);
    let r = await recipient.request.post(`${api}/account/tenant-invitations/accept`, { headers: rh, data: { ...body, expected_version: '1' } }); expect(r.status()).toBe(200); const accepted = await r.json();
    r = await recipient.request.post(`${api}/account/tenant-invitations/accept`, { headers: rh, data: { ...body, expected_version: '1' } }); expect((await r.json()).code).toBe('invitation_already_accepted');
    for (const path of ['/members', '/member-invitations', '/audit-events']) expect((await recipient.request.get(base + path)).status()).toBe(403);
    const audit = await (await administrator.request.get(`${base}/audit-events?object_type=tenant_member&object_id=${accepted.membership_id}`)).json(); expect(audit.items).toHaveLength(1); expect(audit.items[0].event_type).toBe('tenant.member.joined');
    const safe = JSON.stringify({ audit, members: await (await administrator.request.get(`${base}/members`)).json(), invites: await (await administrator.request.get(`${base}/member-invitations`)).json() }); expect(safe.includes(credential(l.link)!)).toBe(false); expect(safe.includes('token_hash')).toBe(false);
    const candidates = await (await request.get(`${api}/platform/tenant-admin-candidates?q=${f.member.username}&tenant_id=${f.a}`)).json(); expect(candidates.items[0].transfer_mode).toBe('reuse');
    r = await request.post(`${api}/platform/tenants/${f.a}/administrator-transfer`, { headers: f.headers, data: { expected_version: '1', new_administrator_account_id: f.target.id, reason: 'reuse member' } }); expect(r.status()).toBe(200);
    const promoted = await (await recipient.request.get(`${base}/context`)).json(); expect(promoted.membership_id).toBe(accepted.membership_id); expect(promoted.permissions).toContain(P.TenantInvitationCreate); expect((await administrator.request.get(`${base}/profile`)).status()).toBe(404);
  } finally { await administrator.close(); await recipient.close(); }
});

test('invitation replacement, revocation and refresh clear credentials without automatic acceptance', async ({ page, request, browser }) => {
  const f = await prepare(request); await login(page, f.administrator); const first = await uiInvite(page, f.a, f.member.username);
  const recipient = await browser.newContext(); const p = await recipient.newPage();
  try {
    await login(p, f.member); await p.goto(first); await expect(p.getByRole('button', { name: '确认加入', exact: true })).toBeVisible(); await p.reload(); await expect(p.getByRole('heading', { name: '请重新打开原邀请链接' })).toBeVisible();
    await row(page, f.member.username).getByRole('button', { name: '重新生成', exact: true }).click(); await page.getByLabel('操作原因').fill('替换失去的链接'); const checked = page.waitForResponse((r) => r.url().endsWith(`/tenants/${f.a}/context`)); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await checked; await expect(page.getByLabel('操作原因')).toHaveValue('替换失去的链接'); await page.getByRole('button', { name: '确认操作', exact: true }).click();
    await expect(page.getByLabel('完整邀请链接')).toBeVisible(); const replacement = await page.getByLabel('完整邀请链接').inputValue(); await page.getByRole('button', { name: '关闭链接', exact: true }).click();
    await p.goto(first); await expect(p.getByText('已替换', { exact: true })).toBeVisible();
    await p.goto(replacement); await expect(p.getByRole('button', { name: '确认加入', exact: true })).toBeVisible();
    await row(page, f.member.username).filter({ hasText: '待接受' }).getByRole('button', { name: '撤销邀请', exact: true }).click(); await page.getByLabel('操作原因').fill('撤销');
    // Acceptance in the other browser must happen after revocation has committed, not race the write.
    const revoked = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().includes(`/tenants/${f.a}/member-invitations/`) && r.url().endsWith('/revoke'));
    await page.getByRole('button', { name: '确认操作', exact: true }).click(); expect((await revoked).status()).toBe(200);
    await expect(page.getByRole('alertdialog')).toHaveCount(0);
    await p.getByRole('button', { name: '确认加入', exact: true }).click(); await expect(p.getByText(/邀请已结束或过期/)).toBeVisible(); await p.getByRole('button', { name: '查验当前邀请结果' }).click(); await expect(p.getByText('已撤销', { exact: true })).toBeVisible();
    expect((await (await recipient.request.get(`${api}/account/tenants`)).json()).items).toHaveLength(0);
  } finally { await recipient.close(); }
});

test('membership dialogs preserve drafts, reauthenticate explicitly and discard late cross-tenant links', async ({ page, request }) => {
  const f = await prepare(request); await login(page, f.administrator); await page.goto(`${consoleURL}/t/${f.a}/invitations`);
  let attempts = 0; let mode = 'reauth'; let release = () => {}; let fetched = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; }); const ready = new Promise<void>((resolve) => { fetched = resolve; });
  await page.route(`**/api/v1/tenants/${f.a}/member-invitations`, async (route) => {
    if (route.request().method() !== 'POST') return route.continue(); attempts++;
    if (mode === 'reauth') return route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'reauthentication_required', message: '请先确认当前密码。', request_id: 'test' }) });
    const response = await route.fetch(); fetched(); await held; await route.fulfill({ response }).catch(() => undefined);
  });
  try {
    await page.getByRole('button', { name: '邀请成员', exact: true }).click(); await page.getByLabel('完整用户名').fill(f.member.username); await page.getByRole('button', { name: '定位账号' }).click(); await expect(page.getByRole('status').filter({ hasText: f.member.username })).toBeVisible(); await page.getByLabel('操作原因').fill('保留的邀请原因');
    await page.getByRole('button', { name: '创建成员邀请', exact: true }).click(); await page.getByLabel('当前密码', { exact: true }).fill(password); await page.getByRole('button', { name: '确认密码', exact: true }).click(); await expect(page.getByText('密码已确认，请核对内容后再次确认。')).toBeVisible(); expect(attempts).toBe(1); await expect(page.getByLabel('操作原因')).toHaveValue('保留的邀请原因');
    let unavailable = true;
    await page.route(`**/api/v1/tenants/${f.a}/context`, (route) => unavailable ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '短暂故障', request_id: 'test' }) }) : route.continue());
    await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('button', { name: '创建成员邀请', exact: true })).toBeDisabled(); await expect(page.getByLabel('操作原因')).toHaveValue('保留的邀请原因'); unavailable = false; await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('button', { name: '创建成员邀请', exact: true })).toBeEnabled();
    mode = 'held'; await page.getByRole('button', { name: '创建成员邀请', exact: true }).click(); await ready;
    await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click(); await navigateTo(page, '切换租户'); await page.getByRole('menuitem', { name: /^成员验收 B.*进入租户/ }).click(); release();
    await expect(page.getByRole('heading', { name: '租户资料' })).toBeVisible(); await expect(page.getByLabel('完整邀请链接')).toHaveCount(0); await expect(page.getByRole('heading', { name: '成员邀请链接已生成' })).toHaveCount(0);
    const invitations = await (await page.request.get(`${api}/tenants/${f.a}/member-invitations`)).json(); expect(invitations.items).toHaveLength(1); expect((await (await page.request.get(`${api}/tenants/${f.b}/member-invitations`)).json()).items).toHaveLength(0);
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }); }
});

test('rejoining replaces the membership context and rejects delayed reads from the former relation', async ({ request, browser }, info) => {
  const f = await prepare(request); const admin = await browser.newContext(); const recipient = await browser.newContext({ viewport: info.project.use.viewport }); const memberPage = await recipient.newPage();
  let release = () => {}; let fetched = () => {}; const held = new Promise<void>((resolve) => { release = resolve; }); const ready = new Promise<void>((resolve) => { fetched = resolve; });
  try {
    const ah = await signIn(admin.request, f.administrator.username); const rh = await signIn(recipient.request, f.member.username);
    const first = await invite(admin.request, ah, f.a, f.target.id); const accepted = await (await recipient.request.post(`${api}/account/tenant-invitations/accept`, { headers: rh, data: { token: credential(first.link), expected_version: '1' } })).json();
    let block = true;
    await memberPage.route(`**/api/v1/tenants/${f.a}/profile`, async (route) => { if (!block) return route.continue(); block = false; const response = await route.fetch(); fetched(); await held; await route.fulfill({ response }).catch(() => undefined); });
    await memberPage.goto(`${consoleURL}/t/${f.a}/profile`); await ready;
    expect((await admin.request.post(`${api}/tenants/${f.a}/members/${accepted.membership_id}/remove`, { headers: ah, data: { expected_version: '1', reason: '重新加入隔离' } })).status()).toBe(200);
    // Ensure this tab observes the ended relation before the new relation exists.
    // Previously a concurrent context refresh made this path nondeterministic.
    await memberPage.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(memberPage.getByRole('heading', { name: '当前租户不可访问', exact: true })).toBeVisible();
    const next = await invite(admin.request, ah, f.a, f.target.id); const joined = await (await recipient.request.post(`${api}/account/tenant-invitations/accept`, { headers: rh, data: { token: credential(next.link), expected_version: '1' } })).json(); expect(joined.membership_id).not.toBe(accepted.membership_id);
    const observedContext = await recipient.request.get(`${api}/tenants/${f.a}/context`);
    expect(observedContext.status()).toBe(200);
    expect((await observedContext.json()).membership_id).toBe(joined.membership_id);
    let refreshedContext = 0;
    memberPage.on('response', (response) => { if (response.url().endsWith(`/tenants/${f.a}/context`)) refreshedContext++; });
    expect((await admin.request.patch(`${api}/tenants/${f.a}/profile`, { headers: ah, data: { expected_version: '1', description: '仅新关系应读取的资料', reason: '隔离旧响应' } })).status()).toBe(200);
    await memberPage.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect.poll(() => refreshedContext, { message: 'New membership must trigger a fresh context check' }).toBeGreaterThan(0);
    await expect(memberPage.getByText('仅新关系应读取的资料', { exact: true })).toBeVisible(); release();
    await memberPage.getByText('当前租户权限', { exact: true }).click(); await expect(memberPage.getByText(`成员关系：${joined.membership_id}`, { exact: true })).toBeVisible(); await expect(memberPage.getByText(`成员关系：${accepted.membership_id}`, { exact: true })).toHaveCount(0); await expect(memberPage.getByText('成员管理验收', { exact: true })).toHaveCount(0);
  } finally { release(); await memberPage.unrouteAll({ behavior: 'wait' }); await admin.close(); await recipient.close(); }
});

import { randomBytes, randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, consoleURL, createAccount, csrf, login, password } from './fixtures';

test.use({ trace: 'off' });
const origin = new URL(consoleURL).origin; const api = `${origin}/api/v1`;
async function signIn(request: APIRequestContext, username = 'e2e_owner') {
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) };
  expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200); return headers;
}
async function prepare(request: APIRequestContext) {
  const admin = await createAccount(request, false); const recipient = await createAccount(request, false); const h = await signIn(request);
  const find = async (username: string) => (await (await request.get(`${api}/access/accounts?q=${username}`)).json()).items[0].id as string;
  const adminId = await find(admin.username); const recipientId = await find(recipient.username);
  const create = async () => { const r = await request.post(`${api}/platform/tenants`, { headers: h, data: { name: '租户启停验收', code: `tl-${randomBytes(5).toString('hex')}`, administrator_account_id: adminId, client_request_id: randomUUID(), reason: '启停测试准备' } }); expect(r.status()).toBe(201); return (await r.json()).tenant_id as string; };
  return { admin, recipient, recipientId, tenant: await create(), other: await create(), h };
}
// Concurrent tests share the global authorization revision. Exercise the prescribed
// explicit re-preview flow on that conflict; never silently replay an old write.
async function change(request: APIRequestContext, h: Record<string, string>, tenant: string, action: 'disable' | 'enable') {
  for (let review = 0; review < 5; review++) {
    const pre = await request.post(`${api}/platform/tenants/${tenant}/${action}/preview`, { headers: h, data: {} }); expect(pre.status()).toBe(200); const p = await pre.json();
    const r = await request.post(`${api}/platform/tenants/${tenant}/${action}`, { headers: h, data: { expected_version: p.expected_version, preview_hash: p.preview_hash, reason: '内部原因-lifecycle-secret', ...(action === 'disable' ? { tenant_notice: '维护中，请联系维护人员。' } : {}) } });
    if (r.status() === 200) return r.json();
    expect(r.status()).toBe(409); expect((await r.json()).code).toBe('preview_changed');
  }
  throw new Error('Global authorization remained unstable across five explicit previews');
}
async function confirmLifecycle(page: Page, tenant: string, action: 'disable' | 'enable') {
  for (let review = 0; review < 5; review++) {
    const response = page.waitForResponse((r) => r.url() === `${api}/platform/tenants/${tenant}/${action}` && r.request().method() === 'POST');
    await page.getByRole('button', { name: action === 'disable' ? '确认停用租户' : '确认恢复租户', exact: true }).click(); const r = await response;
    if (r.status() === 200) { await expect(page.getByRole('dialog')).toHaveCount(0); return; }
    expect(r.status()).toBe(409); expect((await r.json()).code).toBe('preview_changed');
    await expect(page.getByText('操作影响已变化，请重新查看并确认。', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: '重新预览并核对' }).click();
    await expect(page.getByText('正在读取操作影响…')).toHaveCount(0);
  }
  throw new Error('Could not confirm after explicit review');
}

test('lifecycle UI: preview, cancel, stop old tabs, preserve invite, restore and public audit', async ({ page, request, browser }, info) => {
  const f = await prepare(request); await login(page, f.admin); await page.goto(`${consoleURL}/t/${f.tenant}/profile`);
  const oldMembership = (await (await page.request.get(`${api}/tenants/${f.tenant}/context`)).json()).membership_id;
  const adminHeaders = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
  const issued = await page.request.post(`${api}/tenants/${f.tenant}/member-invitations`, { headers: adminHeaders, data: { target_account_id: f.recipientId, reason: '保留待领取邀请', client_request_id: randomUUID() } }); expect(issued.status()).toBe(201); const invitation = await issued.json();
  const operator = await browser.newContext({ viewport: page.viewportSize()! }); const recipient = await browser.newContext({ viewport: page.viewportSize()! });
  try {
    const op = await operator.newPage(); await login(op, { username: 'e2e_owner', password }); await op.goto(`${consoleURL}/platform/tenants/${f.tenant}`);
    const target = await recipient.newPage(); await login(target, f.recipient); await target.goto(invitation.link); await expect(target.getByRole('heading', { name: '确认加入租户', exact: true })).toBeVisible();
    await page.getByRole('button', { name: '编辑资料', exact: true }).click(); await page.getByLabel('租户名称', { exact: true }).fill('停用后应丢弃的草稿');
    await op.getByRole('button', { name: '停用租户', exact: true }).click(); await expect(op.getByRole('button', { name: '取消', exact: true })).toBeFocused();
    await expect(op.getByRole('region', { name: '租户启停影响' })).toContainText('待领取邀请 1 条');
    await op.getByRole('button', { name: '取消', exact: true }).click(); await expect(op.getByRole('button', { name: '停用租户', exact: true })).toBeFocused();
    await op.getByRole('button', { name: '停用租户', exact: true }).click(); await op.getByLabel('平台内部操作原因').fill('内部原因-lifecycle-secret'); await op.getByLabel('租户可见说明').fill('维护中，请联系维护人员。');
    for (const width of info.project.name === 'desktop' ? [1440, 768] : [375]) { await op.setViewportSize({ width, height: 1000 }); for (const theme of ['light', 'dark']) { await op.evaluate((t) => document.documentElement.classList.toggle('dark', t === 'dark'), theme); expect(await op.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await op.screenshot({ path: info.outputPath(`tenant-disable-${width}-${theme}.png`), fullPage: true }); } }
    await confirmLifecycle(op, f.tenant, 'disable'); await expect(op.getByRole('button', { name: '恢复租户', exact: true })).toBeVisible();
    await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('heading', { name: '租户已停用', exact: true })).toBeVisible(); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByText('停用后应丢弃的草稿', { exact: true })).toHaveCount(0); await expect(page.getByText('内部原因-lifecycle-secret')).toHaveCount(0);
    await target.getByRole('button', { name: '确认加入', exact: true }).click(); await target.getByRole('button', { name: '查验当前邀请结果' }).click(); await expect(target.getByRole('heading', { name: '租户已停用，暂时无法加入' })).toBeVisible(); await expect(target.getByRole('button', { name: '确认加入', exact: true })).toBeDisabled();
    await page.getByRole('link', { name: '返回我的租户' }).click(); await expect(page.getByRole('link', { name: '查看状态', exact: true })).toBeVisible(); await page.goto(`${consoleURL}/t/${f.other}/profile`); await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible();
    expect((await page.request.get(`${api}/account/profile`)).status()).toBe(200);
    await op.getByRole('button', { name: '恢复租户', exact: true }).click(); await op.getByLabel('平台内部操作原因').fill('恢复核验完成'); await confirmLifecycle(op, f.tenant, 'enable');
    await target.getByRole('button', { name: '重新检查', exact: true }).click(); await expect(target.getByRole('heading', { name: '确认加入租户', exact: true })).toBeVisible(); await target.getByRole('button', { name: '确认加入', exact: true }).click(); await expect(target.getByRole('heading', { name: '已加入租户', exact: true })).toBeVisible();
    await page.goto(`${consoleURL}/t/${f.tenant}/profile`); await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible(); expect((await (await page.request.get(`${api}/tenants/${f.tenant}/context`)).json()).membership_id).toBe(oldMembership);
    await page.goto(`${consoleURL}/t/${f.tenant}/audit`); await expect(page.getByRole('heading', { name: '停用租户', exact: true })).toBeVisible(); await page.getByLabel('搜索操作或原因').fill('内部原因-lifecycle-secret'); await expect(page.getByText('没有符合条件的操作记录。')).toBeVisible();
  } finally { await operator.close(); await recipient.close(); }
});

test('all tenant HTTP operations deny disabled access; strict status API and privacy', async ({ request, browser }) => {
  const f = await prepare(request); const ctx = await browser.newContext();
  try {
    const h = await signIn(ctx.request, f.admin.username); await change(request, f.h, f.tenant, 'disable');
    const source = readFileSync('packages/go/platform/access/policy.go', 'utf8');
    const policies = [...source.matchAll(/\{Op\w+, "(GET|POST|PATCH|PUT)", "(\/api\/v1\/tenants\/[^"]+)", "tenant"/g)]; expect(policies.length).toBeGreaterThan(25);
    for (const [, method, path] of policies) { const url = path!.replace(':tenant_id', f.tenant).replace(/:[a-z_]+/g, randomUUID()); const response = await ctx.request.fetch(`${origin}${url}`, { method, headers: h, ...(method === 'GET' ? {} : { data: {} }) }); expect(response.status(), url).toBe(403); expect((await response.json()).code).toBe('tenant_disabled'); }
    expect((await ctx.request.get(`${api}/account/tenants/${f.tenant}/availability`)).status()).toBe(200); expect((await ctx.request.get(`${api}/tenants/${f.other}/profile`)).status()).toBe(200);
    expect((await request.get(`${api}/tenants/${f.tenant}/profile`)).status()).toBe(404);
    const base = `${api}/platform/tenants/${f.tenant}/enable`;
    for (const data of [null, [], { extra: true }]) expect((await request.post(`${base}/preview`, { headers: f.h, data })).status()).toBe(400);
    expect((await request.post(`${base}/preview`, { headers: { Origin: origin }, data: {} })).status()).toBe(403);
    const p = await (await request.post(`${base}/preview`, { headers: f.h, data: {} })).json(); const input = { expected_version: p.expected_version, preview_hash: p.preview_hash, reason: '恢复' };
    for (const extra of [{ tenant_notice: null }, { status: 'active' }, { reason: '　 ' }, { reason: '桥'.repeat(501) }, { expected_version: '0' }]) expect((await request.post(base, { headers: f.h, data: { ...input, ...extra } })).status()).toBe(400);
    expect((await ctx.request.post(base, { headers: h, data: input })).status()).toBe(403);
    const filtered = await request.get(`${api}/platform/tenants?status=disabled&q=${(await (await request.get(`${api}/platform/tenants/${f.tenant}`)).json()).code}&administrator_unavailable=false&limit=1`); expect((await filtered.json()).items).toHaveLength(1);
    expect((await request.get(`${api}/platform/tenants?status=bogus`)).status()).toBe(400);
    await change(request, f.h, f.tenant, 'enable');
    const audit = await ctx.request.get(`${api}/tenants/${f.tenant}/audit-events`); expect(await audit.text()).not.toContain('lifecycle-secret'); expect(await audit.text()).not.toContain('status_change_reason');
  } finally { await ctx.close(); }
});

test('disabled boundary rejects late reads and writes, retains drafts on outages, and polls within 35 seconds', async ({ page, request }) => {
  test.setTimeout(90_000); const f = await prepare(request); await login(page, f.admin); await page.goto(`${consoleURL}/t/${f.tenant}/profile`); await page.getByRole('button', { name: '编辑资料', exact: true }).click(); await page.getByLabel('租户名称', { exact: true }).fill('故障期间保留的草稿');
  await page.route(`**/tenants/${f.tenant}/context`, (route) => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '隔离依赖故障' }) })); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('dialog').getByText('租户状态暂不可确认，输入已保留，恢复后可继续提交。')).toBeVisible(); await expect(page.getByLabel('租户名称', { exact: true })).toHaveValue('故障期间保留的草稿'); await expect(page.getByRole('button', { name: '确认提交', exact: true })).toBeDisabled();
  await page.unroute(`**/tenants/${f.tenant}/context`); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('button', { name: '确认提交', exact: true })).toBeEnabled();
  let release!: () => void; const held = new Promise<void>((r) => { release = r; }); let ready!: () => void; const fetched = new Promise<void>((r) => { ready = r; });
  await page.route(`**/tenants/${f.tenant}/profile`, async (route) => { if (route.request().method() !== 'PATCH') return route.continue(); const response = await route.fetch(); ready(); await held; try { await route.fulfill({ response }); } catch { /* canceled old boundary */ } });
  await page.getByLabel('变更原因').fill('迟到写响应'); await page.getByRole('button', { name: '确认提交', exact: true }).click(); await fetched;
  await change(request, f.h, f.tenant, 'disable');
  await expect(page.getByRole('heading', { name: '租户已停用', exact: true })).toBeVisible({ timeout: 35_000 }); release(); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByText('变更已保存。', { exact: true })).toHaveCount(0);
  await change(request, f.h, f.tenant, 'enable'); await page.getByRole('button', { name: '重新检查', exact: true }).click(); await expect(page.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible(); await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('status confirmation preserves inputs on preview failure and reauthentication never replays a write', async ({ page, request }) => {
  const f = await prepare(request); await login(page, { username: 'e2e_owner', password }); await page.goto(`${consoleURL}/platform/tenants/${f.tenant}`);
  const previewPath = `**/platform/tenants/${f.tenant}/disable/preview`;
  await page.route(previewPath, (route) => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '预览依赖不可用' }) }));
  await page.getByRole('button', { name: '停用租户', exact: true }).click(); await expect(page.getByRole('button', { name: '确认停用租户', exact: true })).toBeDisabled(); await expect(page.getByText('预览依赖不可用', { exact: true })).toBeVisible(); await page.getByLabel('平台内部操作原因').fill('保留的内部原因');
  await page.unroute(previewPath); await page.getByRole('button', { name: '重新预览并核对' }).click(); await expect(page.getByRole('button', { name: '确认停用租户', exact: true })).toBeEnabled();
  let writes = 0;
  await page.route(`**/platform/tenants/${f.tenant}/disable`, (route) => { writes++; return route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'reauthentication_required', message: '请重新确认密码。' }) }); });
  await page.getByRole('button', { name: '确认停用租户', exact: true }).click(); await page.getByLabel('当前密码', { exact: true }).fill(password); await page.getByRole('button', { name: '确认密码', exact: true }).click(); await expect(page.getByRole('button', { name: '确认停用租户', exact: true })).toBeEnabled(); expect(writes).toBe(1); await expect(page.getByLabel('平台内部操作原因')).toHaveValue('保留的内部原因');
  expect((await (await request.get(`${api}/platform/tenants/${f.tenant}`)).json()).status).toBe('active');
  await page.unroute(`**/platform/tenants/${f.tenant}/disable`); await confirmLifecycle(page, f.tenant, 'disable');
});

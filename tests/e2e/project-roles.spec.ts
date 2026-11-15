import { randomUUID } from 'node:crypto';
import type { Page } from '@playwright/test';
import { test, expect, consoleURL, login, password } from './fixtures';
import { prepareProject } from './project-fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

test.use({ trace: 'off', actionTimeout: 15_000 });
test.setTimeout(180_000);
const api = `${new URL(consoleURL).origin}/api/v1`;
function projectAPI(f: Awaited<ReturnType<typeof prepareProject>>, pid: string) {
  const base = `${api}/tenants/${f.tid}/projects/${pid}`;
  const read = async (path: string) => { const response = await f.pb.request.get(base + path); expect(response.status(), await response.text()).toBe(200); return response.json(); };
  const write = async (path: string, data: Record<string, unknown>, method = 'POST') => { const response = await f.pb.request.fetch(base + path, { method, headers: f.pb.h, data }); expect(response.status(), await response.text()).toBe(200); return response.json(); };
  const createRole = async (name: string, codes: string[] = [P.ProjectProfileRead]) => write('/roles', { name, description: '角色回归测试', permission_codes: codes, expected_project_version: (await read('/context')).project.version, reason: '隔离测试准备', client_request_id: randomUUID() });
  const change = async (path: string, data: Record<string, unknown> = {}, method = 'POST') => { const preview = await write(path + '/preview', data); expect(preview.can_submit, preview.blocked_reason).toBe(true); return write(path, { ...data, expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, ...(path.includes('/members/') && path.endsWith('/roles') ? { role_versions: preview.role_versions } : {}), reason: '隔离测试变更', client_request_id: randomUUID() }, method); };
  const update = async (id: string, name: string) => write(`/roles/${id}`, { name, description: '另一个会话更新', expected_version: (await read(`/roles/${id}`)).version, expected_project_version: (await read('/context')).project.version, reason: '并发更新', client_request_id: randomUUID() }, 'PATCH');
  const add = async () => write('/members', { tenant_membership_id: f.cid, expected_project_version: (await read('/context')).project.version, reason: '绑定测试成员', client_request_id: randomUUID() });
  return { base, read, write, createRole, change, update, add };
}
async function menu(page: Page, role: string, action: string) { await page.getByRole('dialog').getByRole('button', { name: `${role}的操作`, exact: true }).click(); await page.getByRole('menuitem', { name: action, exact: true }).click(); }
async function preview(page: Page) { await page.getByRole('dialog').getByRole('button', { name: '预览变更影响', exact: true }).click(); await expect(page.getByRole('region', { name: '项目变更影响' })).toBeVisible(); }
async function submit(page: Page, label: string) { const dialog = page.getByRole('dialog'); await dialog.getByLabel('变更原因', { exact: true }).fill('浏览器真实操作验证'); await dialog.getByRole('button', { name: label, exact: true }).click(); }
async function close(page: Page) { await page.getByRole('dialog').getByRole('button', { name: '关闭面板', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); }

test('project roles A: real lifecycle, integrated permissions, bound members and protected presets', async ({ page, request, browser }) => {
  const f = await prepareProject(request, browser); const pid = await f.create('role-lifecycle'); const p = projectAPI(f, pid); const root = `${consoleURL}/t/${f.tid}/p/${pid}`;
  try {
    const linked = await p.createRole('协作职责', [P.ProjectProfileRead, P.ProjectProfileUpdate]); const member = await p.add();
    const ordinary = (await p.read('/roles')).items.find((r: { builtin_key: string }) => r.builtin_key === 'project_member');
    await p.change(`/members/${member.object_id}/roles`, { role_ids: [ordinary.id, linked.object_id] }, 'PUT');
    await login(page, f.bob); await page.goto(root + '/permissions'); await expect(page).toHaveURL(root + '/roles');
    await expect(page.getByRole('link', { name: '项目权限目录', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: '创建项目角色', exact: true }).click();
    let dialog = page.getByRole('dialog'); await dialog.getByLabel('角色名称', { exact: true }).fill('新建职责');
    await dialog.getByRole('checkbox', { name: '查看项目资料', exact: true }).check();
    await expect(dialog.getByRole('checkbox', { name: '配置项目角色权限', exact: true })).toBeDisabled();
    const created = page.waitForResponse(r => r.url() === p.base + '/roles' && r.request().method() === 'POST'); await submit(page, '创建项目角色');
    const id = (await (await created).json()).object_id as string; await expect(page.getByRole('dialog')).toHaveCount(0);
    expect((await p.read(`/roles/${id}`)).permission_codes).toEqual([P.ProjectProfileRead]);
    await page.getByRole('link', { name: '新建职责', exact: true }).click(); dialog = page.getByRole('dialog');
    await expect(dialog.getByText('查看项目资料', { exact: true })).toBeVisible(); await expect(dialog.locator('code')).toHaveCount(0);
    await dialog.getByRole('switch', { name: '显示权限编码', exact: true }).check(); await expect(dialog.locator('code').filter({ hasText: P.ProjectProfileRead })).toBeVisible();
    await menu(page, '新建职责', '编辑名称与说明'); await dialog.getByLabel('角色名称', { exact: true }).fill('已更新职责'); await dialog.getByLabel('角色说明', { exact: true }).fill('多行说明\n保持业务语义'); await submit(page, '保存角色资料');
    await expect(page.getByRole('heading', { name: '已更新职责', exact: true })).toBeVisible(); expect((await p.read(`/roles/${id}`)).description).toBe('多行说明\n保持业务语义');
    await menu(page, '已更新职责', '复制为本项目角色'); await dialog.getByLabel('角色名称', { exact: true }).fill('独立副本'); await submit(page, '复制为本项目角色');
    await expect(page.getByRole('heading', { name: '已更新职责', exact: true })).toBeVisible();
    const copy = (await p.read('/roles?q=独立副本')).items[0]; expect(copy.source_role_id).toBe(id); expect(copy.account_count).toBe(0); expect(copy.permission_codes).toEqual([P.ProjectProfileRead]);
    await close(page); await page.getByRole('link', { name: '协作职责', exact: true }).click();
    await dialog.getByRole('button', { name: '配置角色权限', exact: true }).click(); await dialog.getByRole('checkbox', { name: '编辑项目资料', exact: true }).uncheck(); await preview(page);
    await expect(page.getByRole('region', { name: '项目变更影响' })).toContainText('实际减少：编辑项目资料');
    await dialog.getByRole('checkbox', { name: '查看项目资料', exact: true }).uncheck(); await expect(page.getByRole('region', { name: '项目变更影响' })).toHaveCount(0); await expect(dialog.getByRole('button', { name: '确认配置角色权限' })).toBeDisabled();
    await dialog.getByRole('checkbox', { name: '查看项目资料', exact: true }).check(); await preview(page); await submit(page, '确认配置角色权限'); await expect(page.getByRole('heading', { name: '协作职责', exact: true })).toBeVisible();
    const auth = await p.read(`/members/${member.object_id}/authorization`); expect(auth.permissions).not.toContain(P.ProjectProfileUpdate); expect(auth.permissions).toContain(P.ProjectProfileRead);
    await p.change(`/members/${member.object_id}/suspend`); await dialog.getByRole('tab', { name: '绑定成员', exact: true }).click(); await expect(dialog.getByRole('link', { name: '浏览器验收', exact: true })).toBeVisible(); await expect(dialog.getByText('暂停', { exact: true })).toBeVisible();
    await menu(page, '协作职责', '删除项目角色'); await dialog.getByRole('button', { name: '预览变更影响', exact: true }).click(); await expect(dialog.getByText('仍有当前绑定（包括暂停成员），请先全部解绑。', { exact: true })).toBeVisible(); await expect(dialog.getByRole('button', { name: '确认删除角色' })).toBeDisabled(); await dialog.getByRole('button', { name: '取消', exact: true }).click();
    await menu(page, '协作职责', '停用角色'); await preview(page); await submit(page, '确认停用角色'); await expect(dialog.getByText('停用', { exact: true })).toBeVisible();
    await menu(page, '协作职责', '启用角色'); await preview(page); await submit(page, '确认启用角色'); await expect(dialog.getByText('启用', { exact: true })).toBeVisible();
    await p.change(`/members/${member.object_id}/roles`, { role_ids: [] }, 'PUT');
    await menu(page, '协作职责', '删除项目角色'); await preview(page); await submit(page, '确认删除角色'); await expect(dialog).toHaveCount(0); expect((await f.pb.request.get(p.base + `/roles/${linked.object_id}`)).status()).toBe(404);
    await page.getByRole('link', { name: '项目管理员', exact: true }).click(); await expect(dialog.getByText(/项目管理员为受保护角色/)).toBeVisible(); await expect(dialog.getByRole('button', { name: '项目管理员的操作', exact: true })).toHaveCount(0); await expect(dialog.getByRole('button', { name: '配置角色权限', exact: true })).toHaveCount(0);
    await dialog.getByRole('tab', { name: '操作记录', exact: true }).click(); await expect(dialog.getByRole('link', { name: '查看角色操作记录', exact: true })).toHaveAttribute('href', /object_type=project_role/); await close(page);
    let writes = 0; page.on('request', r => { if (r.url().startsWith(p.base + '/roles') && r.method() !== 'GET') writes++; });
    await page.getByRole('button', { name: '创建项目角色', exact: true }).click(); await dialog.getByLabel('角色名称').fill('不会创建'); await dialog.getByRole('button', { name: '取消', exact: true }).click(); await expect(dialog).toHaveCount(0); expect(writes).toBe(0);
  } finally { await f.close(); }
});

test('project roles A: server filters, cursor continuity, deep links, keyboard and responsive layout', async ({ page, request, browser }, info) => {
  const f = await prepareProject(request, browser); const pid = await f.create('role-list'); const p = projectAPI(f, pid); const root = `${consoleURL}/t/${f.tid}/p/${pid}/roles`;
  try {
    const ids: string[] = []; for (let n = 0; n < 12; n++) ids.push((await p.createRole(`分页职责 ${String(n).padStart(2, '0')}`)).object_id);
    await p.change(`/roles/${ids[0]}/status`, { status: 'disabled' });
    await login(page, f.bob); await page.goto(root + '?source=custom'); const table = page.getByRole('table', { name: '项目角色列表', exact: true });
    await expect(table.locator('tbody tr')).toHaveCount(10); await page.getByRole('button', { name: '下一页', exact: true }).click(); await expect(table.locator('tbody tr')).toHaveCount(2); await expect(page.getByText('第 2 页 · 末页', { exact: true })).toBeVisible();
    const link = page.getByRole('link', { name: '分页职责 00', exact: true }); await link.focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('dialog')).toBeVisible(); await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(page.getByText('第 2 页 · 末页', { exact: true })).toBeVisible(); await expect(link).toBeFocused();
    await page.getByLabel('搜索角色名称', { exact: true }).fill('分页职责 11'); await expect(table.locator('tbody tr')).toHaveCount(1); await expect(page.getByRole('link', { name: '分页职责 11', exact: true })).toBeVisible(); await expect(page.getByText('第 1 页 · 末页', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByRole('button', { name: '角色状态', exact: true }).click(); await page.getByRole('menuitemradio', { name: '停用', exact: true }).click(); await expect(table.locator('tbody tr')).toHaveCount(1); await expect(page.getByRole('link', { name: '分页职责 00', exact: true })).toBeVisible();
    await page.getByRole('button', { name: '重置', exact: true }).click(); await page.getByRole('tab', { name: '平台预置', exact: true }).click(); await expect(table.getByText('本项目自定义', { exact: true })).toHaveCount(0); await expect(page.getByRole('link', { name: '项目管理员', exact: true })).toBeVisible();
    await page.getByRole('tab', { name: '本项目自定义', exact: true }).click(); await page.getByRole('combobox', { name: '每页条数', exact: true }).click(); await page.getByRole('option', { name: '20', exact: true }).click(); await expect(table.locator('tbody tr')).toHaveCount(12);
    for (const width of info.project.name === 'desktop' ? [1440, 768] : [375, 812]) {
      await page.setViewportSize({ width, height: width === 812 ? 375 : 1000 });
      for (const dark of [false, true]) { await page.evaluate(value => document.documentElement.classList.toggle('dark', value), dark); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath(`roles-${width}-${dark ? 'dark' : 'light'}.png`), fullPage: true }); }
    }
    await page.setViewportSize(info.project.use.viewport!); await page.evaluate(() => document.documentElement.classList.remove('dark'));
    await page.getByRole('link', { name: '分页职责 11', exact: true }).click(); await page.reload(); await expect(page.getByRole('heading', { name: '分页职责 11', exact: true })).toBeVisible(); await page.getByRole('dialog').getByRole('button', { name: '配置角色权限', exact: true }).click();
    await expect(page.getByRole('dialog').getByRole('checkbox', { name: '查看项目资料', exact: true })).toBeChecked(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath('roles-permission-editor.png'), fullPage: true });
    await close(page); await expect(page).toHaveURL(/source=custom.*limit=20/);
    const pattern = `**/tenants/${f.tid}/projects/${pid}/roles?*`; await page.route(pattern, r => r.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '列表暂不可用' }) })); await page.getByLabel('搜索角色名称', { exact: true }).fill('失败'); await expect(page.getByText('列表暂不可用', { exact: true })).toBeVisible(); await expect(table.getByRole('link')).toHaveCount(0); await page.unroute(pattern); await page.getByRole('button', { name: '重新加载', exact: true }).click(); await expect(page.getByText('列表暂不可用', { exact: true })).toHaveCount(0);
  } finally { await f.close(); }
});

test('project roles A: conflict recovery, password confirmation and exact request replay', async ({ page, request, browser }) => {
  const f = await prepareProject(request, browser); const pid = await f.create('role-recovery'); const p = projectAPI(f, pid); const role = await p.createRole('恢复测试'); const path = `/roles/${role.object_id}`;
  try {
    await login(page, f.bob); await page.goto(`${consoleURL}/t/${f.tid}/p/${pid}${path}`); await menu(page, '恢复测试', '编辑名称与说明'); const dialog = page.getByRole('dialog'); await dialog.getByLabel('角色名称', { exact: true }).fill('保留草稿'); await p.update(role.object_id, '并发名称'); await submit(page, '保存角色资料');
    await dialog.getByRole('button', { name: '重新读取并比较', exact: true }).click(); await expect(dialog.getByLabel('角色名称', { exact: true })).toHaveValue('保留草稿'); await expect(dialog.getByText(/当前名称：并发名称/)).toBeVisible(); await dialog.getByRole('button', { name: '保存角色资料', exact: true }).click(); await expect(page.getByRole('heading', { name: '保留草稿', exact: true })).toBeVisible();
    await dialog.getByRole('button', { name: '配置角色权限', exact: true }).click(); await dialog.getByRole('checkbox', { name: '编辑项目资料', exact: true }).check(); await preview(page); await p.update(role.object_id, '预览已过期'); await submit(page, '确认配置角色权限'); await dialog.getByRole('button', { name: '重新读取并比较', exact: true }).click(); await expect(dialog.getByRole('checkbox', { name: '编辑项目资料', exact: true })).toBeChecked(); await dialog.getByRole('button', { name: '确认配置角色权限', exact: true }).click(); await expect(page.getByRole('heading', { name: '预览已过期', exact: true })).toBeVisible();
    await menu(page, '预览已过期', '编辑名称与说明'); await dialog.getByLabel('角色名称', { exact: true }).fill('密码确认后保存'); let attempts = 0;
    const pattern = `**/tenants/${f.tid}/projects/${pid}${path}`;
    await page.route(pattern, route => { if (route.request().method() !== 'PATCH') return route.continue(); attempts++; return attempts === 1 ? route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'reauthentication_required', message: '请确认密码。' }) }) : route.continue(); });
    await submit(page, '保存角色资料'); await dialog.getByLabel('当前密码', { exact: true }).fill(password); await dialog.getByRole('button', { name: '确认密码', exact: true }).click(); await expect(dialog.getByRole('button', { name: '保存角色资料', exact: true })).toBeEnabled(); expect(attempts).toBe(1); await dialog.getByRole('button', { name: '保存角色资料', exact: true }).click(); await expect(page.getByRole('heading', { name: '密码确认后保存', exact: true })).toBeVisible(); expect(attempts).toBe(2); await page.unroute(pattern);
    await menu(page, '密码确认后保存', '编辑名称与说明'); await dialog.getByLabel('角色名称', { exact: true }).fill('响应丢失后恢复'); const payloads: string[] = []; let release!: () => void; let received!: () => void; const hold = new Promise<void>(resolve => { release = resolve; }); const fetched = new Promise<void>(resolve => { received = resolve; });
    await page.route(pattern, async route => { if (route.request().method() !== 'PATCH') return route.continue(); payloads.push(route.request().postData()!); const response = await route.fetch(); expect(response.status()).toBe(200); if (payloads.length === 1) { received(); await hold; await route.abort('failed'); } else await route.fulfill({ response }); });
    const before = (await p.read(path)).version; await submit(page, '保存角色资料'); await fetched; await page.keyboard.press('Escape'); await expect(dialog).toBeVisible(); await expect(dialog.getByLabel('角色名称', { exact: true })).toBeDisabled(); await expect(dialog.getByRole('button', { name: '正在提交…' })).toBeDisabled(); release();
    await expect(dialog.getByRole('button', { name: '使用原请求重试', exact: true })).toBeEnabled(); await expect(dialog.getByLabel('角色名称', { exact: true })).toBeDisabled(); await dialog.getByRole('button', { name: '使用原请求重试', exact: true }).click(); await expect(page.getByRole('heading', { name: '响应丢失后恢复', exact: true })).toBeVisible(); expect(payloads).toHaveLength(2); expect(payloads[0]).toBe(payloads[1]); expect(Number((await p.read(path)).version)).toBe(Number(before) + 1);
    await page.unroute(pattern); await menu(page, '响应丢失后恢复', '编辑名称与说明'); await dialog.getByLabel('角色名称', { exact: true }).fill('无效响应后恢复'); const malformed: string[] = []; await page.route(pattern, async route => { if (route.request().method() !== 'PATCH') return route.continue(); malformed.push(route.request().postData()!); const response = await route.fetch(); expect(response.status()).toBe(200); if (malformed.length === 1) await route.fulfill({ status: 200, contentType: 'application/json', body: '{' }); else await route.fulfill({ response }); }); await submit(page, '保存角色资料'); await expect(dialog.getByRole('button', { name: '使用原请求重试', exact: true })).toBeEnabled(); await expect(dialog.getByLabel('角色名称', { exact: true })).toBeDisabled(); await dialog.getByRole('button', { name: '使用原请求重试', exact: true }).click(); await expect(page.getByRole('heading', { name: '无效响应后恢复', exact: true })).toBeVisible(); expect(malformed).toHaveLength(2); expect(malformed[0]).toBe(malformed[1]); expect(Number((await p.read(path)).version)).toBe(Number(before) + 2);
  } finally { await f.close(); }
});

test('project roles A: read boundaries, old-tab revocation and archived restrictions', async ({ page, request, browser }) => {
  const f = await prepareProject(request, browser); const pid = await f.create('role-boundaries'); const p = projectAPI(f, pid); const role = await p.createRole('受限查看职责', [P.ProjectProfileRead, P.ProjectRoleRead]); const member = await p.add();
  try {
    await p.change(`/members/${member.object_id}/roles`, { role_ids: [role.object_id] }, 'PUT'); await login(page, f.carol); const root = `${consoleURL}/t/${f.tid}/p/${pid}/roles`; const reads: string[] = []; page.on('request', r => reads.push(r.url()));
    await page.goto(root + `/${role.object_id}`); const dialog = page.getByRole('dialog'); await expect(page.getByRole('heading', { name: '受限查看职责', exact: true })).toBeVisible(); await expect(dialog.getByRole('tab', { name: '绑定成员', exact: true })).toHaveCount(0); await expect(dialog.getByRole('tab', { name: '操作记录', exact: true })).toHaveCount(0); await expect(dialog.getByRole('button', { name: '配置角色权限', exact: true })).toHaveCount(0); await expect(dialog.getByText(/暂无权限说明读取权限/)).toBeVisible(); expect(reads.some(url => url.startsWith(p.base + '/permissions') || url.startsWith(p.base + '/members'))).toBe(false);
    await p.change(`/roles/${role.object_id}/permissions`, { permission_codes: [P.ProjectProfileRead] }, 'PUT'); await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByText('无权查看项目角色。', { exact: true })).toBeVisible(); await expect(page.getByRole('heading', { name: '受限查看职责', exact: true })).toHaveCount(0);
    await page.context().clearCookies(); await login(page, f.bob); await p.change('/archive'); await page.goto(root); await expect(page.getByRole('button', { name: '创建项目角色', exact: true })).toHaveCount(0); await page.getByRole('link', { name: '受限查看职责', exact: true }).click(); await dialog.getByRole('button', { name: '受限查看职责的操作', exact: true }).click(); await expect(page.getByRole('menuitem', { name: '编辑名称与说明', exact: true })).toHaveCount(0); await expect(page.getByRole('menuitem', { name: '复制为本项目角色', exact: true })).toHaveCount(0); await page.keyboard.press('Escape');
    await dialog.getByRole('button', { name: '配置角色权限', exact: true }).click(); await expect(dialog.getByRole('checkbox', { name: '编辑项目资料', exact: true })).toBeDisabled(); await dialog.getByRole('checkbox', { name: '查看项目资料', exact: true }).uncheck(); await preview(page); await submit(page, '确认配置角色权限'); await expect(dialog.getByText('该角色暂未配置权限。', { exact: true })).toBeVisible();
    await menu(page, '受限查看职责', '停用角色'); await preview(page); await submit(page, '确认停用角色'); await dialog.getByRole('button', { name: '受限查看职责的操作', exact: true }).click(); await expect(page.getByRole('menuitem', { name: '启用角色', exact: true })).toHaveCount(0);
  } finally { await f.close(); }
});

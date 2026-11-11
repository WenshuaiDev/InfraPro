import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, consoleURL, csrf, login, password, createAccount } from './fixtures';
import type { EngineeringContent, EngineeringDefinition, EngineeringResult, EngineeringVersion } from '../../packages/ts/api-client/src/engineering';
const origin = new URL(consoleURL).origin; const api = `${origin}/api/v1`; const owner = { username: 'e2e_owner', password };
const wm = () => ({ request_id: randomUUID(), reason: '工程配置验收' });
async function signIn(request: APIRequestContext, username = owner.username) { const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) }; expect((await request.post(`${api}/auth/login`, { headers, data: { username, password } })).status()).toBe(200); return headers; }
const content = (name: string): EngineeringContent => ({ metamodel_version: '1', name, description: '', release_notes: '' });
async function seed(request: APIRequestContext) { const headers = await signIn(request); const suffix = randomBytes(5).toString('hex'); const root = `${api}/platform/engineering`; const post = async <T>(path: string, data: unknown) => { const r = await request.post(root + path, { headers, data }); expect(r.status(), await r.text()).toBe(200); return await r.json() as T; }; const publish = async (kind: string, code: string, c: EngineeringContent) => { const created = await post<EngineeringResult>(`/${kind}`, { ...wm(), code, content: c }); const v = await post<EngineeringResult>(`/${kind}/${created.definition_id}/publish`, { ...wm(), revision: created.revision }); return v; };
 await post('/unit-categories', { ...wm(), key: `length_${suffix}`, name: '长度', description: '空间长度', revision: 0 });
 const unit = await publish('units', `mm_${suffix}`, { ...content('毫米'), unit: { symbol: 'mm', category: `length_${suffix}` } });
 const metric = await publish('metrics', `movement_${suffix}`, { ...content('路面位移'), metric: { shape: 'decimal', category: `length_${suffix}`, standard_unit: unit.version_id, allowed_units: [unit.version_id], options: [] } });
 return { headers, suffix, unit, metric, publish, post };
}
function section(page: Page, title: string) { return page.getByRole('heading', { name: title, exact: true }).locator('../..'); }
async function createUI(page: Page, kind: string, code: string, name: string) { await page.goto(`${consoleURL}/platform/engineering/${kind}`); await page.getByRole('button', { name: '新建定义', exact: true }).click(); await page.getByLabel('稳定编号', { exact: true }).fill(code); await page.getByLabel('定义名称', { exact: true }).fill(name); await page.getByLabel('创建原因').fill('同一构建新增业务类型'); await page.getByRole('button', { name: '创建草稿', exact: true }).click(); await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeVisible(); await page.getByLabel('本次变更原因').fill('编制并验证定义'); }
async function save(page: Page) { const response = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().endsWith('/draft')); await page.getByRole('button', { name: '保存草稿', exact: true }).click(); const r = await response; expect(r.status(), await r.text()).toBe(200); await expect(page.getByText('操作成功。', { exact: true })).toBeVisible(); }
async function publishUI(page: Page) { await page.getByRole('button', { name: '发布当前草稿', exact: true }).click(); const response = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/publish')); await page.getByRole('alertdialog').getByRole('button', { name: '确认', exact: true }).click(); const r = await response; expect(r.status(), await r.text()).toBe(200); await expect(page.getByRole('alertdialog')).toHaveCount(0); await expect(page.getByText('发布成功，已生成不可变版本。', { exact: true })).toBeVisible(); return await r.json() as EngineeringResult; }
async function pick(page: Page, label: string, definition: string, version: string, q: string) { await page.getByLabel(`${label}搜索`, { exact: true }).fill(q); await page.getByLabel(`${label}定义`, { exact: true }).selectOption(definition); await page.getByLabel(`${label}版本`, { exact: true }).selectOption(version); }

test('engineering same build: author arbitrary road type, sample tree, templates, tenant copy and V2', async ({ page, request, browser }, info) => {
 test.setTimeout(180_000); const tenantUser = await createAccount(request, false); const f = await seed(request); const errors: string[] = []; page.on('pageerror', (e) => errors.push(e.message));
 const accounts = await (await request.get(`${api}/access/accounts?q=${tenantUser.username}`)).json(); const tenantResponse = await request.post(`${api}/platform/tenants`, { headers: f.headers, data: { name: '工程模板租户', code: `eng-${f.suffix}`, administrator_account_id: accounts.items[0].id, client_request_id: randomUUID(), reason: '工程验收' } }); expect(tenantResponse.status()).toBe(201); const tid = (await tenantResponse.json()).tenant_id as string;
 await login(page, owner); await createUI(page, 'type-definitions', `road_subgrade_pavement_${f.suffix}`, '路基路面');
 await page.getByRole('tab', { name: '属性字段', exact: true }).click(); await section(page, '设施属性').getByRole('button', { name: '添加属性', exact: true }).click(); let field = section(page, '属性 1 · 未命名'); await field.getByLabel('属性键', { exact: true }).fill('segment_name'); await field.getByLabel('属性名称', { exact: true }).fill('路段名称'); field = section(page, '属性 1 · 路段名称'); await field.getByLabel('必填属性').check();
 await page.getByRole('tab', { name: '构件规则', exact: true }).click(); await page.getByRole('button', { name: '添加构件类别' }).click(); const component = section(page, '构件类别 1'); await component.getByLabel('类别键', { exact: true }).fill('layer'); await component.getByLabel('类别名称', { exact: true }).fill('路面结构层'); await component.getByRole('button', { name: '添加属性', exact: true }).click(); field = component.getByRole('heading', { name: '属性 1 · 未命名', exact: true }).locator('../..'); await field.getByLabel('属性键', { exact: true }).fill('thickness'); await field.getByLabel('属性名称', { exact: true }).fill('厚度'); field = section(page, '属性 1 · 厚度'); await field.getByLabel('数据形态', { exact: true }).selectOption('decimal'); await field.getByLabel('最小数值').fill('0'); await field.getByLabel('必填属性').check(); await pick(page, '属性单位', f.unit.definition_id, f.unit.version_id, `mm_${f.suffix}`);
 await page.getByRole('button', { name: '添加组成关系' }).click(); await page.getByLabel('子级类别').selectOption('layer'); await page.getByLabel('最少构件数').fill('1'); await page.getByLabel('最多构件数（可空）').fill('3'); await save(page);
 await page.getByRole('tab', { name: '样例预览', exact: true }).click(); await page.getByLabel('路段名称 *', { exact: true }).fill('北段'); await page.getByLabel('新增样例构件类别').selectOption('layer'); await page.getByRole('button', { name: '添加样例构件' }).click(); await page.getByLabel('厚度 *', { exact: true }).fill('0'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByText('服务端校验通过；该结果仅验证当前样例，不产生工程数据。', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '添加样例构件' }).click(); await page.getByLabel('厚度 *', { exact: true }).nth(1).fill('20'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByText('服务端校验通过；该结果仅验证当前样例，不产生工程数据。', { exact: true })).toBeVisible(); await page.getByLabel('厚度 *', { exact: true }).nth(1).fill('-1'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByRole('heading', { name: /需要处理的问题/ })).toBeVisible();
 for (const theme of ['light', 'dark']) { await page.evaluate((theme) => document.documentElement.classList.toggle('dark', theme === 'dark'), theme); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath(`engineering-sample-${theme}.png`), fullPage: true }); }
 const v1 = await publishUI(page); await page.getByRole('tab', { name: '操作记录', exact: true }).click(); await expect(page.getByRole('heading', { name: '发布工程版本', exact: true })).toBeVisible(); await page.reload(); await expect(page.getByRole('heading', { name: '路基路面', exact: true })).toBeVisible(); await page.getByRole('tab', { name: '属性字段', exact: true }).click(); await expect(page.getByRole('table', { name: '属性字段', exact: true })).toContainText('路段名称'); await expect(page.getByLabel('属性键', { exact: true })).toHaveCount(0); await page.getByRole('button', { name: '返回目录', exact: true }).click(); await page.getByLabel('搜索名称或编号', { exact: true }).fill(f.suffix); await page.getByRole('button', { name: '路基路面', exact: true }).click(); await page.getByRole('button', { name: '返回目录', exact: true }).click(); await expect(page.getByLabel('搜索名称或编号', { exact: true })).toHaveValue(f.suffix);
 await createUI(page, 'monitoring-templates', `road_template_${f.suffix}`, '路面日常监测'); await page.getByRole('tab', { name: '适用范围', exact: true }).click(); await page.getByRole('button', { name: '添加适用类型' }).click(); await pick(page, '适用类型 1', v1.definition_id, v1.version_id, f.suffix); await page.getByRole('tab', { name: '监测项', exact: true }).click(); await page.getByRole('button', { name: '添加监测项', exact: true }).click(); await page.getByLabel('监测项键').fill('movement'); await page.getByLabel('监测项名称').fill('路面位移'); await page.getByLabel('路基路面 V1 的目标').selectOption('layer'); await pick(page, '绑定指标 1', f.metric.definition_id, f.metric.version_id, f.suffix); await pick(page, '绑定单位 1', f.unit.definition_id, f.unit.version_id, f.suffix); await save(page); const tpl = await publishUI(page);
 await page.context().clearCookies(); await login(page, tenantUser); await page.goto(`${consoleURL}/t/${tid}/engineering/catalog/monitoring-templates?definition=${tpl.definition_id}`); await expect(page).toHaveURL(`${consoleURL}/t/${tid}/engineering/templates`); await expect(page.getByRole('link', { name: /公共设施类型|公共指标|公共单位|公共监测模板/ })).toHaveCount(0); await page.getByRole('button', { name: '从公共模板复制', exact: true }).click(); await pick(page, '公共监测模板', tpl.definition_id, tpl.version_id, f.suffix); await page.getByLabel('复制原因').fill('租户独立编制'); await page.getByLabel('副本新编号').fill(`local_${f.suffix}`); await page.getByRole('button', { name: '复制到本租户' }).click(); await expect(page).toHaveURL(new RegExp(`/engineering/templates\\?definition=`)); await page.getByLabel('定义名称', { exact: true }).fill('租户私有日常监测'); await page.getByLabel('本次变更原因').fill('本租户发布'); await save(page); const privateVersion = await publishUI(page); await page.getByRole('tab', { name: '操作记录', exact: true }).click(); await expect(page.getByRole('heading', { name: '发布工程版本', exact: true })).toBeVisible(); await page.getByRole('tab', { name: '基本资料', exact: true }).click();
 const tenantContext = await browser.newContext(); const tenantHeaders = await signIn(tenantContext.request, tenantUser.username);
 try { const privateRead = await tenantContext.request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`); expect(privateRead.status()).toBe(200); expect((await request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`)).status()).toBe(404);
 await page.context().clearCookies(); await login(page, owner); await page.goto(`${consoleURL}/platform/engineering/type-definitions?definition=${v1.definition_id}`); await page.getByRole('button', { name: '从此版本建立草稿' }).click(); await page.getByLabel('本次变更原因').fill('编制 V2'); await page.getByRole('button', { name: '建立草稿', exact: true }).click(); await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeVisible(); await page.getByLabel('定义名称', { exact: true }).fill('路基路面 V2'); await save(page); await publishUI(page);
 const d = await (await tenantContext.request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`)).json() as EngineeringDefinition; expect(d.dependencies.find((v) => v.id === v1.version_id)?.content.name).toBe('路基路面');
 // Keep the same server/build running and introduce a new, unpredictable type identity.
 const arbitrary = await f.publish('type-definitions', `new_${randomBytes(8).toString('hex')}`, content('任意类型')); expect(arbitrary.version_id).toBeTruthy();
 const stop = await request.post(`${api}/platform/engineering/units/${f.unit.definition_id}/versions/${f.unit.version_id}/state`, { headers: f.headers, data: { ...wm(), revision: 1, enabled: false } }); expect(stop.status()).toBe(200);
 const blocked = await tenantContext.request.post(`${api}/tenants/${tid}/engineering/monitoring-templates/copy`, { headers: tenantHeaders, data: { ...wm(), code: `blocked_${f.suffix}`, source_version: privateVersion.version_id } }); expect(blocked.status()).toBe(409);
 } finally { await tenantContext.close(); }
 expect(errors).toEqual([]);
});

test('engineering strict HTTP, concurrent draft conflicts and idempotent publish', async ({ request }) => {
 const f = await seed(request); const path = `${api}/platform/engineering/type-definitions`; const c = content('HTTP arbitrary');
 for (const data of [{ ...wm(), code: `bad_${f.suffix}`, content: { ...c, script: 'execute' } }, { ...wm(), code: `bad_${f.suffix}`, tenant_id: randomUUID(), content: c }, { ...wm(), code: `bad_${f.suffix}`, content: { ...c, fields: [{ key: 'x', name: 'x', shape: 'script' }] } }]) expect((await request.post(path, { headers: f.headers, data })).status()).toBe(400);
 expect((await request.post(path, { headers: { Origin: origin }, data: { ...wm(), code: `csrf_${f.suffix}`, content: c } })).status()).toBe(403);
 const created = await f.post<EngineeringResult>('/type-definitions', { ...wm(), code: `http_${f.suffix}`, content: c }); const id = created.definition_id;
 const edits = await Promise.all(['A', 'B'].map((name) => request.put(`${path}/${id}/draft`, { headers: f.headers, data: { ...wm(), revision: 1, content: { ...c, name } } }))); expect(edits.map((r) => r.status()).sort()).toEqual([200, 409]);
 const input = { ...wm(), revision: 2 }; const published = await Promise.all([1, 2].map(() => request.post(`${path}/${id}/publish`, { headers: f.headers, data: input }))); for (const r of published) expect(r.status(), await r.text()).toBe(200); expect((await published[0]!.json()).version_id).toBe((await published[1]!.json()).version_id);
 const d = await (await request.get(`${path}/${id}`)).json() as EngineeringDefinition; expect(d.draft).toBeNull(); expect(d.versions).toHaveLength(1); expect((d.versions[0] as EngineeringVersion).number).toBe(1);
 expect((await request.post(`${path}/${id}/publish`, { headers: f.headers, data: { ...input, reason: '冲突载荷' } })).status()).toBe(409);
});

test('engineering editor preserves drafts through outage and clears content after real permission revocation', async ({ page, request }) => {
 const user = await createAccount(request, false); const h = await signIn(request); const suffix = randomBytes(5).toString('hex');
 const { Permissions: P } = await import('../../packages/ts/api-client/src/permissions.generated');
 const target = (await (await request.get(`${api}/access/accounts?q=${user.username}`)).json()).items[0];
 let response = await request.post(`${api}/access/roles`, { headers: h, data: { name: `工程编辑_${suffix}`, description: '只编制不发布', reason: '权限自验' } }); expect(response.status()).toBe(200); let role = await response.json();
 response = await request.put(`${api}/access/roles/${role.id}/permissions`, { headers: h, data: { expected_version: role.version, permission_codes: [P.EngineeringTypeRead, P.EngineeringTypeEdit], reason: '权限自验' } }); expect(response.status()).toBe(200); role = await response.json();
 response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers: h, data: { expected_version: target.management_version, role_ids: [...target.roles.map((r: { id: string }) => r.id), role.id], reason: '权限自验' } }); expect(response.status()).toBe(200);
 await login(page, user); await createUI(page, 'type-definitions', `limited_${suffix}`, '有限编制定义'); await expect(page.getByRole('button', { name: '发布当前草稿', exact: true })).toHaveCount(0); await page.getByLabel('定义名称', { exact: true }).fill('故障仍保留的私有编辑');
 const route = '**/platform/engineering/type-definitions/*/draft'; let writes = 0; await page.route(route, (r) => { if (r.request().method() !== 'PUT') return r.continue(); writes++; return r.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '暂时无法写入' }) }); });
 await page.getByRole('button', { name: '保存草稿', exact: true }).click(); await expect(page.getByText('暂时无法写入', { exact: true })).toBeVisible(); await expect(page.getByLabel('定义名称', { exact: true })).toHaveValue('故障仍保留的私有编辑'); await page.unroute(route); expect(writes).toBe(1);
 response = await request.put(`${api}/access/roles/${role.id}/status`, { headers: h, data: { expected_version: role.version, status: 'disabled', reason: '即时撤销工程权限' } }); expect(response.status(), await response.text()).toBe(200);
 await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByText('故障仍保留的私有编辑', { exact: true })).toHaveCount(0); await expect(page.getByLabel('定义名称', { exact: true })).toHaveCount(0);
});

test('engineering directory keeps filters and page when returning from a definition', async ({ page, request }) => {
 const f = await seed(request);
 for (let i = 0; i < 12; i++) await f.post('/type-definitions', { ...wm(), code: `list_${f.suffix}_${i}`, content: content(`目录分页 ${i}`) });
 await login(page, owner);
 await page.goto(`${consoleURL}/platform/engineering/type-definitions`);
 await page.getByLabel('搜索名称或编号', { exact: true }).fill(`list_${f.suffix}`);
 await page.getByRole('button', { name: '可用状态', exact: true }).click();
 await page.getByRole('menuitemradio', { name: '存在草稿', exact: true }).click();
 await page.getByRole('combobox', { name: '每页条数', exact: true }).click();
 await page.getByRole('option', { name: '10', exact: true }).click();
 const table = page.getByRole('table', { name: '设施类型定义', exact: true });
 await expect(table.getByRole('row')).toHaveCount(11);
 await page.getByRole('button', { name: '下一页', exact: true }).click();
 await expect(table.getByRole('row')).toHaveCount(3);
 const item = table.getByRole('button', { name: /^目录分页 \d+$/ }).first();
 const name = await item.innerText();
 await item.click();
 await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
 await page.getByRole('button', { name: '返回目录', exact: true }).click();
 await expect(page.getByLabel('搜索名称或编号', { exact: true })).toHaveValue(`list_${f.suffix}`);
 await expect(table.getByRole('row')).toHaveCount(3);
 await expect(page.getByText('第 2 页 · 末页', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '第一页', exact: true }).click();
 await expect(table.getByRole('row')).toHaveCount(11);
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

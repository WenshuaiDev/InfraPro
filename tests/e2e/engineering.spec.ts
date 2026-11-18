import { randomBytes, randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';
import { test, expect, consoleURL, csrf, login, password, createAccount } from './fixtures';
import type { EngineeringContent, EngineeringDefinition, EngineeringResult, EngineeringVersion } from '../../packages/ts/api-client/src/engineering';
const origin = new URL(consoleURL).origin; const api = `${origin}/api/v1`; const owner = { username: 'e2e_owner', password };
const wm = () => ({ request_id: randomUUID(), reason: '工程配置验收' });
async function signIn(request: APIRequestContext, username = owner.username) { const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) }; expect((await request.post(`${api}/auth/login`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { username, password } })).status()).toBe(200); return headers; }
const content = (name: string): EngineeringContent => ({ metamodel_version: '1', name, description: '', release_notes: '' });
async function seed(request: APIRequestContext) { const headers = await signIn(request); const suffix = randomBytes(5).toString('hex'); const root = `${api}/platform/engineering`; const post = async <T>(path: string, data: unknown) => { const r = await request.post(root + path, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data }); expect(r.status(), await r.text()).toBe(200); return await r.json() as T; }; const publish = async (kind: string, code: string, c: EngineeringContent) => { const created = await post<EngineeringResult>(`/${kind}`, { ...wm(), code, content: c }); const v = await post<EngineeringResult>(`/${kind}/${created.definition_id}/publish`, { ...wm(), revision: created.revision }); return v; };
 await post('/unit-categories', { ...wm(), key: `length_${suffix}`, name: '长度', description: '空间长度', revision: 0 });
 const unit = await publish('units', `mm_${suffix}`, { ...content('毫米'), unit: { symbol: 'mm', category: `length_${suffix}` } });
 const metric = await publish('metrics', `movement_${suffix}`, { ...content('路面位移'), metric: { shape: 'decimal', category: `length_${suffix}`, standard_unit: unit.version_id, allowed_units: [unit.version_id], options: [] } });
 return { headers, suffix, unit, metric, publish, post };
}
const panel = (page: Page) => page.getByRole('dialog');
async function createUI(page: Page, kind: string, code: string, name: string) {
 await page.goto(`${consoleURL}/platform/engineering/${kind}`);
 await page.getByRole('button', { name: '新建定义', exact: true }).click();
 await page.getByLabel('稳定编号', { exact: true }).fill(code);
 await page.getByLabel('定义名称', { exact: true }).fill(name);
 await page.getByLabel('创建原因').fill('同一构建新增业务类型');
 await page.getByRole('button', { name: '创建草稿', exact: true }).click();
 if (kind === 'type-definitions') {
  await expect(page.getByRole('heading', { level: 1, name, exact: true })).toBeVisible();
  await page.getByRole('button', { name: '版本与管理' }).click();
  await expect(panel(page).getByRole('heading', { name: '当前草稿' })).toBeVisible();
 } else {
  await expect(page.getByRole('button', { name: '保存草稿', exact: true })).toBeVisible();
  await page.getByLabel('本次变更原因').fill('编制并验证定义');
 }
}
async function saveType(page: Page, reason: string) {
 await panel(page).getByLabel('本次变更原因').fill(reason);
 const response = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().endsWith('/draft'));
 await panel(page).getByRole('button', { name: '保存草稿' }).click();
 const r = await response;
 expect(r.status(), await r.text()).toBe(200);
 await expect(panel(page).getByText('草稿已保存。')).toBeVisible();
}
async function publishType(page: Page, number: number) {
 await panel(page).getByRole('button', { name: '校验草稿' }).click();
 await expect(panel(page).getByText('定义校验通过。')).toBeVisible();
 await panel(page).getByRole('button', { name: '发布草稿' }).click();
 await panel(page).getByLabel('本次变更原因').fill('发布设施类型 V' + number);
 const response = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/publish'));
 await panel(page).getByRole('button', { name: '确认发布 V' + number }).click();
 const r = await response;
 expect(r.status(), await r.text()).toBe(200);
 await expect(panel(page).getByRole('heading', { name: '已发布版本 V' + number })).toBeVisible();
 return await r.json() as EngineeringResult;
}
async function save(page: Page) { const response = page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().endsWith('/draft')); await page.getByRole('button', { name: '保存草稿', exact: true }).click(); const r = await response; expect(r.status(), await r.text()).toBe(200); await expect(page.getByText('操作成功。', { exact: true })).toBeVisible(); }
async function publishUI(page: Page) { await page.getByRole('button', { name: '发布当前草稿', exact: true }).click(); const response = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/publish')); await page.getByRole('alertdialog').getByRole('button', { name: '确认', exact: true }).click(); const r = await response; expect(r.status(), await r.text()).toBe(200); await expect(page.getByRole('alertdialog')).toHaveCount(0); await expect(page.getByText('发布成功，已生成不可变版本。', { exact: true })).toBeVisible(); return await r.json() as EngineeringResult; }
async function pick(page: Page, label: string, definition: string, version: string, q: string) { await page.getByLabel(`${label}搜索`, { exact: true }).fill(q); await page.getByLabel(`${label}定义`, { exact: true }).selectOption(definition); await page.getByLabel(`${label}版本`, { exact: true }).selectOption(version); }

test('engineering same build: author arbitrary road type, sample tree, templates, tenant copy and V2', async ({ page, request, browser }, info) => {
 test.setTimeout(180_000); const tenantUser = await createAccount(request, false); const f = await seed(request); const errors: string[] = []; page.on('pageerror', (e) => errors.push(e.message));
 const accounts = await (await request.get(`${api}/access/accounts?q=${tenantUser.username}`)).json(); const tenantResponse = await request.post(`${api}/platform/tenants`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { name: '工程模板租户', code: `eng-${f.suffix}`, administrator_account_id: accounts.items[0].id, client_request_id: randomUUID(), reason: '工程验收' } }); expect(tenantResponse.status()).toBe(201); const tid = (await tenantResponse.json()).tenant_id as string;
 await login(page, owner); await createUI(page, 'type-definitions', `road_subgrade_pavement_${f.suffix}`, '路基路面');
 const drawer = panel(page);
 await drawer.getByRole('button', { name: /设施建档字段/ }).click();
 await drawer.getByRole('button', { name: '添加字段' }).click();
 await drawer.getByLabel('字段编号').fill('segment_name');
 await drawer.getByLabel('字段名称').fill('路段名称');
 await drawer.getByLabel('建档时必填').check();
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await drawer.getByRole('button', { name: /工程结构规则/ }).click();
 await drawer.getByRole('button', { name: '添加构件' }).click();
 await drawer.getByLabel('新构件名称').fill('路面结构层');
 await drawer.getByLabel('新构件编号').fill('layer');
 await drawer.getByRole('button', { name: '加入草稿' }).click();
 await drawer.getByRole('button', { name: /建档资料/ }).click();
 await drawer.getByRole('button', { name: '添加字段' }).click();
 await drawer.getByLabel('字段编号').fill('thickness');
 await drawer.getByLabel('字段名称').fill('厚度');
 await drawer.getByLabel('填写方式').selectOption('decimal');
 await drawer.getByLabel('建档时必填').check();
 await drawer.getByText('填写规则与显示设置', { exact: true }).click();
 await drawer.getByLabel('最小数值').fill('0');
 await pick(page, '计量单位', f.unit.definition_id, f.unit.version_id, `mm_${f.suffix}`);
 await drawer.getByRole('button', { name: '编辑规则' }).click();
 await drawer.getByLabel('最少数量').fill('1');
 await drawer.getByLabel('最多数量').fill('3');
 await saveType(page, '配置路面结构层与字段');
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await drawer.getByRole('button', { name: '更多操作' }).click();
 await drawer.getByRole('button', { name: '试验建档样例' }).click();
 await page.getByLabel('路段名称 *', { exact: true }).fill('北段'); await page.getByLabel('新增样例构件类别').selectOption('layer'); await page.getByRole('button', { name: '添加样例构件' }).click(); await page.getByLabel('厚度 *', { exact: true }).fill('0'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByText('服务端校验通过；该结果仅验证当前样例，不产生工程数据。', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '添加样例构件' }).click(); await page.getByLabel('厚度 *', { exact: true }).nth(1).fill('20'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByText('服务端校验通过；该结果仅验证当前样例，不产生工程数据。', { exact: true })).toBeVisible(); await page.getByLabel('厚度 *', { exact: true }).nth(1).fill('-1'); await page.getByRole('button', { name: '验证样例与定义' }).click(); await expect(page.getByRole('heading', { name: /需要处理的问题/ })).toBeVisible();
 for (const theme of ['light', 'dark']) { await page.evaluate((theme) => document.documentElement.classList.toggle('dark', theme === 'dark'), theme); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true); await page.screenshot({ path: info.outputPath(`engineering-sample-${theme}.png`), fullPage: true }); }
 await drawer.getByRole('button', { name: '当前草稿', exact: true }).click();
 const v1 = await publishType(page, 1);
 await drawer.getByRole('button', { name: '操作记录' }).click();
 await expect(drawer.getByRole('listitem').filter({ hasText: '发布设施类型 V1' })).toContainText('发布');
 await page.reload();
 await expect(page.getByRole('heading', { name: '路基路面', level: 1, exact: true })).toBeVisible();
 await page.getByRole('button', { name: '查看建档资料' }).click();
 await expect(drawer.getByText('路段名称', { exact: true })).toBeVisible();
 await expect(drawer.getByLabel('字段编号')).toHaveCount(0);
 await drawer.getByRole('button', { name: '关闭面板' }).click();
 await page.getByRole('button', { name: '返回目录' }).click();
 await page.getByLabel('搜索名称或编号', { exact: true }).fill(f.suffix);
 await page.getByRole('button', { name: '路基路面', exact: true }).click();
 await page.getByRole('button', { name: '返回目录' }).click();
 await expect(page.getByLabel('搜索名称或编号', { exact: true })).toHaveValue(f.suffix);
 await createUI(page, 'monitoring-templates', `road_template_${f.suffix}`, '路面日常监测'); await page.getByRole('tab', { name: '适用范围', exact: true }).click(); await page.getByRole('button', { name: '添加适用类型' }).click(); await pick(page, '适用类型 1', v1.definition_id, v1.version_id, f.suffix); await page.getByRole('tab', { name: '监测项', exact: true }).click(); await page.getByRole('button', { name: '添加监测项', exact: true }).click(); await page.getByLabel('监测项键').fill('movement'); await page.getByLabel('监测项名称').fill('路面位移'); await page.getByLabel('路基路面 V1 的监测锚点').selectOption('component'); await page.getByLabel('构件类别', { exact: true }).selectOption('layer'); await pick(page, '绑定指标 1', f.metric.definition_id, f.metric.version_id, f.suffix); await expect(page.getByLabel('绑定单位 1', { exact: true })).toHaveValue(f.unit.version_id); await save(page); const tpl = await publishUI(page);
 await page.context().clearCookies(); await login(page, tenantUser); await page.goto(`${consoleURL}/t/${tid}/engineering/catalog/monitoring-templates?definition=${tpl.definition_id}`); await expect(page).toHaveURL(`${consoleURL}/t/${tid}/engineering/templates`); await expect(page.getByRole('link', { name: /公共设施类型|公共指标|公共单位|公共监测模板/ })).toHaveCount(0); await page.getByRole('button', { name: '从公共模板复制', exact: true }).click(); await pick(page, '公共监测模板', tpl.definition_id, tpl.version_id, f.suffix); await page.getByLabel('复制原因').fill('租户独立编制'); await page.getByLabel('副本新编号').fill(`local_${f.suffix}`); await page.getByRole('button', { name: '复制到本租户' }).click(); await expect(page).toHaveURL(new RegExp(`/engineering/templates\\?definition=`)); await page.getByLabel('定义名称', { exact: true }).fill('租户私有日常监测'); await page.getByLabel('本次变更原因').fill('本租户发布'); await save(page); const privateVersion = await publishUI(page); await page.getByRole('tab', { name: '操作记录', exact: true }).click(); await expect(page.getByRole('heading', { name: '发布工程版本', exact: true })).toBeVisible(); await page.getByRole('tab', { name: '基本资料', exact: true }).click();
 const tenantContext = await browser.newContext(); const tenantHeaders = await signIn(tenantContext.request, tenantUser.username);
 try { const privateRead = await tenantContext.request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`); expect(privateRead.status()).toBe(200); expect((await request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`)).status()).toBe(404);
 await page.context().clearCookies(); await login(page, owner);
 await page.goto(`${consoleURL}/platform/engineering/type-definitions?definition=${v1.definition_id}&panel=versions`);
 await drawer.getByRole('button', { name: '建立草稿', exact: true }).click();
 await drawer.getByLabel('本次变更原因').fill('编制 V2');
 await drawer.getByRole('button', { name: '确认建立草稿' }).click();
 await drawer.getByRole('button', { name: /基本资料/ }).click();
 await drawer.getByLabel('类型名称').fill('路基路面 V2');
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await drawer.getByRole('button', { name: /工程结构规则/ }).click();
 await drawer.getByRole('button', { name: '编辑规则' }).click();
 await drawer.getByLabel('最少数量').fill('2');
 await drawer.getByText('构件编号 · layer', { exact: true }).click();
 await drawer.getByLabel('新的构件编号').fill('pavement_layer');
 await drawer.getByRole('button', { name: '应用新编号' }).click();
 await saveType(page, '提高路面结构层数量下限并更新编号');
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await publishType(page, 2);
 await drawer.getByRole('button', { name: '版本对比' }).click();
 await expect(drawer.getByRole('listitem').filter({ hasText: '名称' })).toContainText('路基路面 V2');
 await expect(drawer.getByRole('listitem').filter({ hasText: '最少数量' })).toContainText('2');
 const published = await (await request.get(api + '/platform/engineering/type-definitions/' + v1.definition_id)).json() as EngineeringDefinition;
 const first = published.versions.find((v) => v.number === 1)!.content;
 const second = published.versions.find((v) => v.number === 2)!.content;
 expect(first.relations?.[0]).toMatchObject({ parent: 'root', child: 'layer', min_count: 1, max_count: 3 });
 expect(second.relations?.[0]).toMatchObject({ parent: 'root', child: 'pavement_layer', min_count: 2, max_count: 3 });
 expect(second.components?.[0]?.fields?.[0]).toMatchObject({ name: '厚度', required: true, unit_version: f.unit.version_id });
 const d = await (await tenantContext.request.get(`${api}/tenants/${tid}/engineering/monitoring-templates/${privateVersion.definition_id}`)).json() as EngineeringDefinition; expect(d.dependencies.find((v) => v.id === v1.version_id)?.content.name).toBe('路基路面');
 // Keep the same server/build running and introduce a new, unpredictable type identity.
 const arbitrary = await f.publish('type-definitions', `new_${randomBytes(8).toString('hex')}`, content('任意类型')); expect(arbitrary.version_id).toBeTruthy();
 const stop = await request.post(`${api}/platform/engineering/units/${f.unit.definition_id}/versions/${f.unit.version_id}/state`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { ...wm(), revision: 1, enabled: false } }); expect(stop.status()).toBe(200);
 const blocked = await tenantContext.request.post(`${api}/tenants/${tid}/engineering/monitoring-templates/copy`, { headers: { ...tenantHeaders, 'X-Command-ID': randomUUID() }, data: { ...wm(), code: `blocked_${f.suffix}`, source_version: privateVersion.version_id } }); expect(blocked.status()).toBe(409);
 } finally { await tenantContext.close(); }
 expect(errors).toEqual([]);
});

test('engineering strict HTTP, concurrent draft conflicts and idempotent publish', async ({ request }) => {
 const f = await seed(request); const path = `${api}/platform/engineering/type-definitions`; const c = content('HTTP arbitrary');
 for (const data of [{ ...wm(), code: `bad_${f.suffix}`, content: { ...c, script: 'execute' } }, { ...wm(), code: `bad_${f.suffix}`, tenant_id: randomUUID(), content: c }, { ...wm(), code: `bad_${f.suffix}`, content: { ...c, fields: [{ key: 'x', name: 'x', shape: 'script' }] } }]) expect((await request.post(path, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data })).status()).toBe(400);
 expect((await request.post(path, { headers: { Origin: origin }, data: { ...wm(), code: `csrf_${f.suffix}`, content: c } })).status()).toBe(403);
 const created = await f.post<EngineeringResult>('/type-definitions', { ...wm(), code: `http_${f.suffix}`, content: c }); const id = created.definition_id;
 const edits = await Promise.all(['A', 'B'].map((name) => request.put(`${path}/${id}/draft`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { ...wm(), revision: 1, content: { ...c, name } } }))); expect(edits.map((r) => r.status()).sort()).toEqual([200, 409]);
 const input = { ...wm(), revision: 2 }; const published = await Promise.all([1, 2].map(() => request.post(`${path}/${id}/publish`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: input }))); for (const r of published) expect(r.status(), await r.text()).toBe(200); expect((await published[0]!.json()).version_id).toBe((await published[1]!.json()).version_id);
 const d = await (await request.get(`${path}/${id}`)).json() as EngineeringDefinition; expect(d.draft).toBeNull(); expect(d.versions).toHaveLength(1); expect((d.versions[0] as EngineeringVersion).number).toBe(1);
 expect((await request.post(`${path}/${id}/publish`, { headers: { ...f.headers, 'X-Command-ID': randomUUID() }, data: { ...input, reason: '冲突载荷' } })).status()).toBe(409);
});

test('engineering editor preserves drafts through outage and clears content after real permission revocation', async ({ page, request }) => {
 const user = await createAccount(request, false); const h = await signIn(request); const suffix = randomBytes(5).toString('hex');
 const { Permissions: P } = await import('../../packages/ts/api-client/src/permissions.generated');
 const target = (await (await request.get(`${api}/access/accounts?q=${user.username}`)).json()).items[0];
 let response = await request.post(`${api}/access/roles`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { name: `工程编辑_${suffix}`, description: '只编制不发布', reason: '权限自验' } }); expect(response.status()).toBe(200); let role = await response.json();
 response = await request.put(`${api}/access/roles/${role.id}/permissions`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { expected_version: role.version, permission_codes: [P.EngineeringTypeRead, P.EngineeringTypeEdit], reason: '权限自验' } }); expect(response.status()).toBe(200); role = await response.json();
 response = await request.put(`${api}/access/accounts/${target.id}/roles`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { expected_version: target.management_version, role_ids: [...target.roles.map((r: { id: string }) => r.id), role.id], reason: '权限自验' } }); expect(response.status()).toBe(200);
 await login(page, user); await createUI(page, 'type-definitions', `limited_${suffix}`, '有限编制定义');
 await panel(page).getByRole('button', { name: '校验草稿' }).click();
 await expect(panel(page).getByText('定义校验通过。')).toBeVisible();
 await expect(panel(page).getByRole('button', { name: '发布草稿', exact: true })).toHaveCount(0);
 await panel(page).getByRole('button', { name: /基本资料/ }).click();
 await page.getByLabel('类型名称').fill('故障仍保留的私有编辑');
 await page.getByLabel('本次变更原因').fill('保留未提交的输入');
 const route = '**/platform/engineering/type-definitions/*/draft'; let writes = 0; await page.route(route, (r) => { if (r.request().method() !== 'PUT') return r.continue(); writes++; return r.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '暂时无法写入' }) }); });
 await page.getByRole('button', { name: '保存草稿', exact: true }).click(); await expect(page.getByText('暂时无法写入', { exact: true })).toBeVisible(); await expect(page.getByLabel('类型名称', { exact: true })).toHaveValue('故障仍保留的私有编辑'); await page.unroute(route); expect(writes).toBe(1);
 response = await request.put(`${api}/access/roles/${role.id}/status`, { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { expected_version: role.version, status: 'disabled', reason: '即时撤销工程权限' } }); expect(response.status(), await response.text()).toBe(200);
 await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByText('故障仍保留的私有编辑', { exact: true })).toHaveCount(0); await expect(page.getByLabel('类型名称', { exact: true })).toHaveCount(0);
});

test('engineering directory keeps filters and page when returning from a definition', async ({ page, request, isMobile }) => {
 const f = await seed(request);
 for (let i = 0; i < 12; i++) await f.post('/type-definitions', { ...wm(), code: `list_${f.suffix}_${i}`, content: content(`目录分页 ${i}`) });
 await login(page, owner);
 await page.goto(`${consoleURL}/platform/engineering/type-definitions`);
 await page.getByLabel('搜索名称或编号', { exact: true }).fill(`list_${f.suffix}`);
 await page.getByRole('button', { name: '可用状态', exact: true }).click();
 await page.getByRole('menuitemradio', { name: '存在草稿', exact: true }).click();
 const perPage = page.getByRole('combobox', { name: '每页条数', exact: true });
 await perPage.scrollIntoViewIfNeeded();
 const tenRows = page.getByRole('option', { name: '10', exact: true });
 if (isMobile) { await perPage.tap(); await tenRows.tap(); }
 else { await perPage.click(); await tenRows.click(); }
 const table = page.getByRole('table', { name: '设施类型定义', exact: true });
 await expect(table.getByRole('row')).toHaveCount(11);
 await page.getByRole('button', { name: '下一页', exact: true }).click();
 await expect(table.getByRole('row')).toHaveCount(3);
 const item = table.getByRole('button', { name: /^目录分页 \d+$/ }).first();
 const name = await item.innerText();
 await item.click();
 await expect(page.getByRole('heading', { level: 1, name, exact: true })).toBeVisible();
 await page.getByRole('button', { name: '返回目录', exact: true }).click();
 await expect(page.getByLabel('搜索名称或编号', { exact: true })).toHaveValue(`list_${f.suffix}`);
 await expect(table.getByRole('row')).toHaveCount(3);
 await expect(page.getByText('第 2 页 · 末页', { exact: true })).toBeVisible();
 await page.getByRole('button', { name: '第一页', exact: true }).click();
 await expect(table.getByRole('row')).toHaveCount(11);
 expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('facility type keeps local edits on a concurrent save and reconciles a lost acknowledgement', async ({ page, request }) => {
 const f = await seed(request);
 const original = content('边坡旧稿');
 const result = await f.post<EngineeringResult>('/type-definitions', { ...wm(), code: 'slope_' + f.suffix, content: original });
 const detail = api + '/platform/engineering/type-definitions/' + result.definition_id;
 await login(page, owner);
 await page.goto(consoleURL + '/platform/engineering/type-definitions?definition=' + result.definition_id + '&panel=versions');
 const drawer = panel(page);
 await drawer.getByRole('button', { name: /基本资料/ }).click();
 await drawer.getByLabel('类型名称').fill('本页的编辑');
 await drawer.getByLabel('本次变更原因').fill('更新类型名称');
 const competitor = await request.put(detail + '/draft', {
  headers: { ...f.headers, 'X-Command-ID': randomUUID() },
  data: { ...wm(), revision: result.revision, content: { ...original, name: '另一位编辑的草稿' } },
 });
 expect(competitor.status(), await competitor.text()).toBe(200);
 await drawer.getByRole('button', { name: '保存草稿' }).click();
 await expect(drawer.getByText('服务端草稿已有变更，本页的输入已保留。')).toBeVisible();
 await expect(drawer.getByLabel('类型名称')).toHaveValue('本页的编辑');
 await drawer.getByRole('button', { name: '重新读取', exact: true }).click();
 await drawer.getByRole('button', { name: '确认重新读取' }).click();
 await drawer.getByRole('button', { name: /基本资料/ }).click();
 await expect(drawer.getByLabel('类型名称')).toHaveValue('另一位编辑的草稿');
 await drawer.getByLabel('类型名称').fill('最终边坡名称');
 await drawer.getByLabel('本次变更原因').fill('采用最终名称');
 let committed = 0;
 await page.route(detail + '/draft', async (route) => {
  const reply = await route.fetch();
  expect(reply.status()).toBe(200);
  committed++;
  await route.fulfill({ status: 503, json: { code: 'unavailable', message: '保存回执在返回时中断' } });
 });
 await drawer.getByRole('button', { name: '保存草稿' }).click();
 await drawer.getByRole('button', { name: '核对操作回执' }).click();
 await expect(drawer.getByText('草稿已保存。')).toBeVisible();
 const current = await (await request.get(detail)).json() as EngineeringDefinition;
 expect(current.draft?.content.name).toBe('最终边坡名称');
 expect(current.revision).toBe(result.revision + 2);
 expect(committed).toBe(1);
 await page.reload();
 await expect(drawer.getByRole('heading', { name: '当前草稿' })).toBeVisible();
 await drawer.getByRole('button', { name: /基本资料/ }).click();
 await expect(drawer.getByLabel('类型名称')).toHaveValue('最终边坡名称');
});

test('facility version can be disabled, restored and copied; deleting the independent draft returns to its directory', async ({ page, request }) => {
 const f = await seed(request);
 const source = await f.publish('type-definitions', 'weather_' + f.suffix, content('气象观测设施'));
 const typesURL = consoleURL + '/platform/engineering/type-definitions';
 const dataURL = api + '/platform/engineering/type-definitions/';
 await login(page, owner);
 await page.goto(typesURL + '?definition=' + source.definition_id + '&panel=versions');
 const drawer = panel(page);
 for (const enabled of [false, true]) {
  const action = enabled ? '恢复' : '停用';
  await drawer.getByRole('button', { name: '更多操作' }).click();
  await drawer.getByRole('button', { name: action + '版本' }).click();
  await drawer.getByLabel('变更原因').fill(action + '新建设施选用');
  await drawer.getByRole('button', { name: '确认' + action }).click();
  await expect(drawer.getByText('版本使用状态已更新。')).toBeVisible();
  await expect(drawer.getByText(enabled ? '新建设施可选' : '已停用', { exact: true })).toBeVisible();
  const server = await (await request.get(dataURL + source.definition_id)).json() as EngineeringDefinition;
  expect(server.versions[0]?.enabled).toBe(enabled);
 }
 await drawer.getByRole('button', { name: '更多操作' }).click();
 await drawer.getByRole('button', { name: '复制为独立定义' }).click();
 await drawer.getByLabel('副本编号').fill('independent_' + f.suffix);
 await drawer.getByLabel('复制原因').fill('独立维护观测设施类型');
 const copied = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/type-definitions/copy'));
 await drawer.getByRole('button', { name: '创建副本草稿' }).click();
 const copy = await (await copied).json() as EngineeringResult;
 expect(copy.definition_id).not.toBe(source.definition_id);
 await expect(page).toHaveURL(typesURL + '?definition=' + copy.definition_id);
 await page.getByRole('button', { name: '版本与管理' }).click();
 await expect(drawer.getByRole('heading', { name: '当前草稿' })).toBeVisible();
 await drawer.getByRole('button', { name: /基本资料/ }).click();
 await drawer.getByLabel('类型名称').fill('仅副本的未保存编辑');
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await drawer.getByRole('button', { name: '更多操作' }).click();
 await drawer.getByRole('button', { name: '删除草稿' }).click();
 await drawer.getByLabel('本次变更原因').fill('撤销测试副本');
 await drawer.getByRole('button', { name: '确认删除' }).click();
 await expect(page).toHaveURL(typesURL);
 await expect(page.getByRole('heading', { name: '设施类型定义', exact: true })).toBeVisible();
 expect((await request.get(dataURL + copy.definition_id)).status()).toBe(404);
 const original = await (await request.get(dataURL + source.definition_id)).json() as EngineeringDefinition;
 expect(original.draft).toBeNull();
 expect(original.versions.map((v) => v.id)).toEqual([source.version_id]);
});

test('legacy types can add borehole fields in a contextual draft and keep the selected published snapshot', async ({ page, request, isMobile }) => {
 const f = await seed(request);
 const source = await f.publish('type-definitions', 'legacy_cut_' + f.suffix, content('旧版路堑'));
 const route = consoleURL + '/platform/engineering/type-definitions?definition=' + source.definition_id;
 await login(page, owner);
 await page.goto(route + '&panel=structure');
 const drawer = panel(page);
 if (isMobile) await drawer.getByLabel('选择工程对象').selectOption('borehole');
 else await drawer.getByRole('navigation', { name: '工程对象目录' }).getByRole('button', { name: '孔位' }).click();
 await expect(drawer.getByRole('heading', { name: '此版本尚未声明孔位能力' })).toBeVisible();
 await drawer.getByRole('button', { name: '建草稿编辑规则' }).click();
 await drawer.getByLabel('本次变更原因').fill('补充孔位定义');
 await drawer.getByRole('button', { name: '确认建立草稿' }).click();
 await expect(drawer.getByRole('heading', { name: '孔位', exact: true })).toBeVisible();
 await drawer.getByRole('button', { name: '启用断面与孔位设置' }).click();
 await drawer.getByRole('radio', { name: '按需建立' }).check();
 await drawer.getByRole('button', { name: /建档资料/ }).click();
 await drawer.getByRole('button', { name: '添加字段' }).click();
 await drawer.getByLabel('字段名称').fill('孔号');
 await drawer.getByLabel('字段编号').fill('hole_number');
 await saveType(page, '按需建立孔位并记录孔号');
 await drawer.getByRole('button', { name: '草稿概览' }).click();
 await publishType(page, 2);
 if (isMobile) await drawer.getByLabel('查看版本').selectOption(source.version_id);
 else await drawer.getByRole('navigation', { name: '版本时间线' }).getByRole('button', { name: /^V1\b/ }).click();
 await expect(drawer.getByRole('heading', { name: '已发布版本 V1' })).toBeVisible();
 await drawer.getByRole('button', { name: /工程结构规则/ }).click();
 if (isMobile) await drawer.getByLabel('选择工程对象').selectOption('borehole');
 else await drawer.getByRole('navigation', { name: '工程对象目录' }).getByRole('button', { name: '孔位' }).click();
 await expect(drawer.getByRole('heading', { name: '此版本尚未声明孔位能力' })).toBeVisible();
 const server = await (await request.get(api + '/platform/engineering/type-definitions/' + source.definition_id)).json() as EngineeringDefinition;
 expect(server.versions.find(v => v.number === 2)?.content.structure?.borehole).toMatchObject({ supported: true, fields: [{ key: 'hole_number', name: '孔号' }] });
 expect(server.versions.find(v => v.number === 1)?.content.metamodel_version).toBe('1');
});

test('a reader can compare and inspect a published type without maintenance controls', async ({ page, request }) => {
 const account = await createAccount(request, false);
 const f = await seed(request);
 const h = { ...f.headers, 'X-Command-ID': randomUUID() };
 const { Permissions: P } = await import('../../packages/ts/api-client/src/permissions.generated');
 const target = (await (await request.get(api + '/access/accounts?q=' + account.username)).json()).items[0];
 const newRole = await request.post(api + '/access/roles', { headers: h, data: { name: '类型查看_' + f.suffix, description: '查看设施定义', reason: '只读验收' } });
 expect(newRole.status()).toBe(200);
 const role = await newRole.json();
 const grant = await request.put(api + '/access/roles/' + role.id + '/permissions', { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { expected_version: role.version, permission_codes: [P.EngineeringTypeRead], reason: '只读验收' } });
 expect(grant.status()).toBe(200);
 const assign = await request.put(api + '/access/accounts/' + target.id + '/roles', { headers: { ...h, 'X-Command-ID': randomUUID() }, data: { expected_version: target.management_version, role_ids: [...target.roles.map((r: { id: string }) => r.id), role.id], reason: '只读验收' } });
 expect(assign.status()).toBe(200);
 const v = await f.publish('type-definitions', 'public_read_' + f.suffix, content('泵站'));
 const d = await f.post<EngineeringResult>('/type-definitions/' + v.definition_id + '/draft', { ...wm(), revision: v.revision, version_id: v.version_id });
 await f.post('/type-definitions/' + v.definition_id + '/publish', { ...wm(), revision: d.revision });
 await login(page, account);
 await page.goto(consoleURL + '/platform/engineering/type-definitions?definition=' + v.definition_id + '&panel=versions');
 const drawer = panel(page);
 await expect(drawer.getByRole('heading', { name: '已发布版本 V2' })).toBeVisible();
 await expect(drawer.getByRole('button', { name: '建立草稿' })).toHaveCount(0);
 await expect(drawer.getByRole('button', { name: '操作记录' })).toHaveCount(0);
 await drawer.getByRole('button', { name: '更多操作' }).click();
 await expect(drawer.getByRole('button', { name: '停用版本' })).toHaveCount(0);
 await expect(drawer.getByRole('button', { name: '复制为独立定义' })).toHaveCount(0);
 await drawer.getByRole('button', { name: '版本对比' }).click();
 await expect(drawer.getByText(/定义内容一致/)).toBeVisible();
});

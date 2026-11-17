import { randomUUID } from 'node:crypto';
import { test, expect, consoleURL, csrf, login, createAccount, password } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { seedResources } from '../helpers/resources';
const origin = new URL(consoleURL).origin;

test('facility tree loads pages lazily, searches deep paths and keeps URL and mobile selection', async ({ page, request, isMobile }, info) => {
  test.setTimeout(180_000);
  const f = await seedResources(request); await login(page, f.user);
  const errors: string[] = []; page.on('pageerror', (e) => errors.push(e.message));
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
  const base = `${origin}/api/v1/tenants/${f.tenant}/engineering/resources`;
  const post = async (path: string, data: object) => { const response = await page.request.post(base + path, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { request_id: randomUUID(), ...data } }); expect(response.status(), await response.text()).toBe(201); return response.json(); };
  const facilities: { id: string; name: string }[] = [];
  for (let index = 0; index < 27; index++) { const name = `分页设施${String(index).padStart(2, '0')}`; const created = await post('/facilities', { name, code: `F${index}`, type_version_id: f.type.version_id, values: { length: 0 } }); facilities.push({ id: created.id, name }); }
  const facility = facilities[26]!; const root = `/facilities/${facility.id}`;
  const span = await post(root + '/components', { code: 'SPAN', name: '主跨', category_key: 'span', expected_structure_revision: 1 });
  const beam = await post(root + '/components', { code: 'BEAM', name: '跨中主梁', category_key: 'beam', parent_component_id: span.id, expected_structure_revision: 2 });
  const point = await post(root + '/points', { code: 'POINT', name: '梁底测点', component_id: beam.id });
  const channel = await post(root + `/points/${point.id}/channels`, { code: 'CH', name: '深层_目标通道', metric_version_id: f.metric.version_id, unit_version_id: f.unit.version_id, timestamp_meaning: 'observation_time' });
  const pointIDs: string[] = [];
  for (let index = 0; index < 27; index++) pointIDs.push((await post(root + '/points', { code: `P${index}`, name: `直属测点${String(index).padStart(2, '0')}` })).id);
  const queriedParents = new Set<string>();
  page.on('request', (r) => { const url = new URL(r.url()); if (url.pathname === new URL(base).pathname + '/tree' && url.searchParams.has('parent_id')) queriedParents.add(url.searchParams.get('parent_id')!); });
  const indexURL = `${consoleURL}/t/${f.tenant}/resources/facilities`;
  await page.goto(indexURL);
  const tree = page.getByRole('tree', { name: '设施、构件、测点与通道' });
  await expect(tree.getByRole('treeitem', { name: '分页设施00，设施', exact: true })).toBeVisible();
  expect(queriedParents.has(facility.id)).toBe(false);
  await expect(tree.getByRole('treeitem', { name: `${facility.name}，设施`, exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '加载更多设施', exact: true }).click();
  await expect(tree.getByRole('treeitem', { name: `${facility.name}，设施`, exact: true })).toBeVisible();
  expect(queriedParents.has(facility.id)).toBe(false);
  await tree.getByRole('treeitem', { name: `${facility.name}，设施`, exact: true }).press('Enter');
  await expect(page.getByRole('heading', { name: facility.name, exact: true })).toBeVisible();
  await expect(page.getByLabel('直属资源数量').getByText('28', { exact: true })).toBeVisible();
  await page.getByRole('tab', { name: '下级资源', exact: true }).click();
  await expect(page.getByText('已显示 25 / 28 项', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '加载更多下级资源', exact: true }).click();
  await expect(page.getByText('已显示 28 / 28 项', { exact: true })).toBeVisible();
  const directory = async () => { if (isMobile) await page.getByRole('button', { name: '返回资源目录', exact: true }).click(); };
  await directory();
  await page.getByLabel('搜索设施、构件、断面、孔位、测点或通道', { exact: true }).fill('_目标');
  await expect(tree.getByRole('treeitem')).toHaveCount(5);
  for (const name of ['主跨，构件', '跨中主梁，构件', '梁底测点，测点']) await expect(tree.getByRole('treeitem', { name, exact: true })).toBeVisible();
  await tree.getByRole('treeitem', { name: '深层_目标通道，业务通道', exact: true }).press('Enter');
  await expect(page.getByRole('heading', { name: '深层_目标通道', exact: true })).toBeVisible();
  expect(new URL(page.url()).searchParams.get('channel')).toBe(channel.id);
  await page.reload();
  await expect(page.getByRole('navigation', { name: '当前资源路径' }).getByRole('button')).toHaveCount(5);
  await page.getByRole('navigation', { name: '当前资源路径' }).getByRole('button', { name: '梁底测点', exact: true }).click();
  await expect(page.getByRole('heading', { name: '梁底测点', exact: true })).toBeVisible();
  await page.goBack(); await expect(page.getByRole('heading', { name: '深层_目标通道', exact: true })).toBeVisible();
  await page.goForward(); await expect(page.getByRole('heading', { name: '梁底测点', exact: true })).toBeVisible();
  await directory();
  await expect(page.getByLabel('搜索设施、构件、断面、孔位、测点或通道', { exact: true })).toHaveValue('_目标');
  await page.getByRole('button', { name: '重置筛选', exact: true }).click();
  await expect(tree.getByRole('treeitem', { name: '梁底测点，测点', exact: true })).toHaveAttribute('aria-selected', 'true');
  await page.getByRole('button', { name: '收起全部资源', exact: true }).click();
  await expect(tree.getByRole('treeitem', { name: '梁底测点，测点', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '定位当前资源', exact: true }).click();
  await expect(tree.getByRole('treeitem', { name: '梁底测点，测点', exact: true })).toBeFocused();
  await tree.getByRole('treeitem', { name: '梁底测点，测点', exact: true }).press('ArrowLeft');
  await tree.getByRole('treeitem', { name: '梁底测点，测点', exact: true }).press('ArrowLeft');
  await expect(tree.getByRole('treeitem', { name: '跨中主梁，构件', exact: true })).toBeFocused();
  // A fresh deep link supplements both the root page and the child page with its current path.
  await page.goto(`${indexURL}/${facility.id}?point=${pointIDs.at(-1)}`);
  await expect(page.getByRole('heading', { name: '直属测点26', exact: true })).toBeVisible();
  await directory();
  await expect(tree.getByRole('treeitem', { name: '直属测点26，测点', exact: true })).toHaveAttribute('aria-selected', 'true');
  for (const theme of ['light', 'dark']) {
    await page.evaluate((theme) => document.documentElement.classList.toggle('dark', theme === 'dark'), theme);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`facility-tree-${theme}.png`), fullPage: true });
  }
  expect(errors).toEqual([]);
});

test('facility workspace deletes the selected leaf, retains parent and reads deleted facility history', async ({ page, request, isMobile }) => {
  const f = await seedResources(request); await login(page, f.user);
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
  const base = `${origin}/api/v1/tenants/${f.tenant}/engineering/resources`;
  const post = async (path: string, data: object) => { const r = await page.request.post(base + path, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { request_id: randomUUID(), ...data } }); expect(r.status(), await r.text()).toBe(201); return r.json(); };
  const facility = await post('/facilities', { name: '可删除设施', code: 'REMOVE', type_version_id: f.type.version_id, values: { length: 0 } });
  const point = await post(`/facilities/${facility.id}/points`, { name: '误建测点', code: 'POINT' });
  await page.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}?point=${point.id}`);
  const remove = async (name: string, kind: string) => {
    await page.getByRole('button', { name: `${name} 操作`, exact: true }).click();
    await page.getByRole('menuitem', { name: `删除${kind}`, exact: true }).click();
    await page.getByLabel('原因 *', { exact: true }).fill('移除误建资源');
    await page.getByRole('button', { name: '预览删除影响', exact: true }).click();
    await page.getByRole('button', { name: '确认保存', exact: true }).click();
    await expect(page.getByRole('dialog')).toBeHidden();
  };
  await remove('误建测点', '测点');
  await expect(page.getByRole('heading', { name: '可删除设施', exact: true })).toBeVisible();
  await expect(page.getByRole('treeitem', { name: '误建测点，测点', exact: true })).toHaveCount(0);
  await remove('可删除设施', '设施');
  await expect(page.getByText('尚未创建设施', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '设施状态', exact: true }).click();
  await page.getByRole('menuitemradio', { name: '已删除记录', exact: true }).click();
  await page.getByRole('treeitem', { name: '可删除设施，设施', exact: true }).press('Enter');
  await expect(page.getByRole('heading', { name: '变更记录', exact: true })).toBeVisible();
  await expect(page.getByText('可删除设施 · REMOVE', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: '可删除设施 操作', exact: true })).toHaveCount(0);
  if (isMobile) await page.getByRole('button', { name: '返回资源目录', exact: true }).click();
  await expect(page.getByRole('treeitem', { name: '可删除设施，设施', exact: true })).not.toHaveAttribute('aria-expanded');
});

test('read-only facility workspace hides writes and clears cached data after permission removal', async ({ page, request, browser }) => {
  const f = await seedResources(request);
  const reader = await createAccount(request, false);
  const platformHeaders = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) };
  expect((await request.post(`${origin}/api/v1/auth/login`, { headers: { ...platformHeaders, 'X-Command-ID': randomUUID() }, data: { username: 'e2e_owner', password } })).status()).toBe(200);
  const accountID = (await (await request.get(`${origin}/api/v1/access/accounts?q=${reader.username}`)).json()).items[0].id;
  await login(page, f.user);
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(page.request, origin) };
  const base = `${origin}/api/v1/tenants/${f.tenant}`;
  const post = async (path: string, data: object, status = 200) => { const r = await page.request.post(base + path, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data }); expect(r.status(), await r.text()).toBe(status); return r.json(); };
  const facility = await post('/engineering/resources/facilities', { request_id: randomUUID(), code: 'READ', name: '仅查看设施', type_version_id: f.type.version_id, values: { length: 0 } }, 201);
  const invitation = await post('/member-invitations', { target_account_id: accountID, client_request_id: randomUUID(), reason: '树形界面只读验收' }, 201);
  const readerContext = await browser.newContext({ viewport: page.viewportSize()! });
  try {
    const token = await csrf(readerContext.request, origin);
    const readerHeaders = { Origin: origin, 'X-CSRF-Token': token };
    expect((await readerContext.request.post(`${origin}/api/v1/auth/login`, { headers: { ...readerHeaders, 'X-Command-ID': randomUUID() }, data: reader })).status()).toBe(200);
    const accepted = await readerContext.request.post(`${origin}/api/v1/account/tenant-invitations/accept`, { headers: { ...readerHeaders, 'X-Command-ID': randomUUID() }, data: { token: new URLSearchParams(new URL(invitation.link).hash.slice(1)).get('token'), expected_version: invitation.invitation.version } });
    expect(accepted.status()).toBe(200); const membership = (await accepted.json()).membership_id;
    const role = (await post('/roles', { name: '设施只读角色', permission_codes: [P.TenantProfileRead, P.TenantEngineeringResourceRead], client_request_id: randomUUID(), reason: '只读范围' }, 201)).role;
    const assign = async (roles: string[]) => {
      const authorization = await (await page.request.get(`${base}/members/${membership}/authorization`)).json();
      const input = { expected_membership_version: authorization.membership_version, role_ids: roles };
      const preview = await post(`/members/${membership}/roles/preview`, input);
      const response = await page.request.put(`${base}/members/${membership}/roles`, { headers: { ...headers, 'X-Command-ID': randomUUID() }, data: { ...input, role_versions: preview.role_versions, preview_hash: preview.preview_hash, reason: '验证读取权限边界' } });
      expect(response.status(), await response.text()).toBe(200);
    };
    await assign([role.id]);
    const readerPage = await readerContext.newPage();
    await readerPage.goto(`${consoleURL}/t/${f.tenant}/resources/facilities/${facility.id}`);
    await expect(readerPage.getByRole('heading', { name: '仅查看设施', exact: true })).toBeVisible();
    for (const name of ['新增设施', '编辑资料', '添加下级', '仅查看设施 操作']) await expect(readerPage.getByRole('button', { name, exact: true })).toHaveCount(0);
    expect((await readerContext.request.get(base + '/engineering/resources/tree')).status()).toBe(200);
    await assign([]);
    expect((await readerContext.request.get(base + '/engineering/resources/tree')).status()).toBe(403);
    await readerPage.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(readerPage.getByRole('heading', { name: '仅查看设施', exact: true })).toHaveCount(0);
    await expect(readerPage.getByRole('treeitem', { name: '仅查看设施，设施', exact: true })).toHaveCount(0);
  } finally { await readerContext.close(); }
});

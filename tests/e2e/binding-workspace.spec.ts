import { randomUUID } from 'node:crypto';
import type { Page } from '@playwright/test';
import { test, expect, consoleURL, csrf, login } from './fixtures';
import { seedResources } from '../helpers/resources';

const origin = new URL(consoleURL).origin;
const meta = () => ({ request_id: randomUUID(), reason: '绑定工作台回归' });

async function submitBinding(page: Page, reason: string) {
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('变更原因 *', { exact: true }).fill(reason);
  await dialog.getByRole('button', { name: '预览并核对', exact: true }).click();
  const confirm = dialog.getByRole('button', { name: '确认提交', exact: true });
  await expect.poll(async () => await dialog.count() === 0 || await confirm.isVisible()).toBe(true);
  if (await confirm.isVisible()) await confirm.click();
  await expect(dialog).toHaveCount(0);
}

test('binding workbench preserves complete paths, paged selection and real bind/switch/unbind', async ({ page, request, isMobile }, info) => {
  test.setTimeout(180_000);
  page.setDefaultTimeout(15_000);
  const f = await seedResources(request);
  const manufacturer = await f.post('/platform/device-definitions/manufacturers', { ...meta(), code: `binding_${f.suffix}`, name: '绑定工作台厂家' });
  const draft = await f.post('/platform/device-definitions/products', { ...meta(), manufacturer_id: manufacturer.id, code: `binding_${f.suffix}`, name: '静力水准仪', model: 'BIND-01', output_slots: [{ key: 'value', name: '位移输出', mode: 'fixed', metric_version_id: f.metric.version_id, unit_version_id: f.unit.version_id, timestamp_meaning: 'observation_time' }] });
  const product = await f.post(`/platform/device-definitions/products/${draft.id}/publish`, { ...meta(), expected_draft_revision: draft.revision, classification: { mode: 'unclassified' } });
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request, origin) };
  expect((await request.post(`${origin}/api/v1/auth/logout`, { headers })).status()).toBe(204);
  expect((await request.post(`${origin}/api/v1/auth/login`, { headers, data: f.user })).status()).toBe(200);
  const base = `/tenants/${f.tenant}`;
  const facility = await f.post(base + '/engineering/resources/facilities', { request_id: randomUUID(), code: 'BIND-F', name: '回归桥梁', type_version_id: f.type.version_id, values: { length: 0 } }, 201);
  const root = base + `/engineering/resources/facilities/${facility.id}`;
  const span = await f.post(root + '/components', { request_id: randomUUID(), code: 'SPAN', name: '主跨', category_key: 'span', expected_structure_revision: 1 }, 201);
  const beam = await f.post(root + '/components', { request_id: randomUUID(), code: 'BEAM-PATH', name: '跨中主梁', category_key: 'beam', parent_component_id: span.id, expected_structure_revision: 2 }, 201);
  const point = await f.post(root + '/points', { request_id: randomUUID(), code: 'P1', name: '梁底测点', component_id: beam.id }, 201);
  const directPoint = await f.post(root + '/points', { request_id: randomUUID(), code: 'P2', name: '桥面环境点' }, 201);
  const channelInput = { metric_version_id: f.metric.version_id, unit_version_id: f.unit.version_id, timestamp_meaning: 'observation_time' };
  const channel = await f.post(root + `/points/${point.id}/channels`, { request_id: randomUUID(), code: 'CH1', name: '竖向位移', ...channelInput }, 201);
  const direct = await f.post(root + `/points/${directPoint.id}/channels`, { request_id: randomUUID(), code: 'CH2', name: '桥面位移', ...channelInput }, 201);
  let deep = channel;
  for (let index = 0; index < 51; index++) deep = await f.post(root + `/points/${point.id}/channels`, { request_id: randomUUID(), code: `CH-${index}`, name: `分页通道${index}`, ...channelInput }, 201);
  const devices: { id: string; output: string; name: string; code: string }[] = [];
  for (let index = 0; index < 11; index++) {
    const code = `BIND-${String(index + 1).padStart(2, '0')}`, name = `位移设备${index + 1}`;
    const device = await f.post(base + '/devices', { ...meta(), code, name, product_version_id: product.version_id });
    const slots = await (await request.get(`${origin}/api/v1${base}/devices/${device.id}/outputs`)).json();
    devices.push({ id: device.id, output: slots.items[0].current.id, name, code });
  }
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page, f.user);
  const url = `${consoleURL}/t/${f.tenant}/bindings`;
  await page.goto(url + `?channel=${deep.id}`);
  const tree = page.getByRole('region', { name: '业务通道层级列表', exact: true });
  const pairing = page.getByRole('region', { name: '设备通道配对', exact: true });
  await expect(tree.getByText('当前定位 · 不在已加载结果中', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '定位已选', exact: true }).click();
  await expect(tree.locator(`[data-binding-channel="${deep.id}"]`)).toBeFocused();
  await expect(tree.getByText('主跨', { exact: true }).first()).toBeVisible();
  await expect(tree.getByText('跨中主梁', { exact: true }).first()).toBeVisible();

  let failMore = true;
  await page.route('**/monitoring-source-bindings?**', async route => {
    if (failMore && new URL(route.request().url()).searchParams.get('cursor')) return route.fulfill({ status: 503, json: { code: 'unavailable', message: '后续通道暂时无法读取' } });
    return route.continue();
  });
  await page.getByRole('navigation', { name: '业务通道分页' }).getByRole('button', { name: '下一页' }).click();
  await expect(tree.getByText('更多通道暂时无法读取', { exact: true })).toBeVisible();
  await expect(tree.locator(`[data-binding-channel="${deep.id}"]`)).toBeVisible();
  failMore = false;
  await tree.getByRole('button', { name: '重新加载', exact: true }).click();
  const pager = page.getByRole('navigation', { name: '业务通道分页' });
  await pager.getByRole('button', { name: '下一页' }).click();
  await expect(pager.getByRole('button', { name: '第 2 页' })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByText('共 53 条', { exact: true })).toBeVisible();
  await expect(tree.getByText('当前定位 · 不在已加载结果中', { exact: true })).toHaveCount(0);
  await page.unroute('**/monitoring-source-bindings?**');

  await page.getByRole('textbox', { name: '搜索设施、构件、测点或业务通道', exact: true }).fill('BEAM-PATH');
  await expect(pairing.getByText('请先选择业务通道', { exact: true })).toBeVisible();
  const leaf = tree.getByRole('button', { name: '选择 回归桥梁 / 主跨 / 跨中主梁 / 梁底测点 / 竖向位移', exact: true });
  await leaf.click();
  await expect(pairing.getByText('回归桥梁 / 主跨 / 跨中主梁 / 梁底测点', { exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: '当前绑定设备', exact: true }).getByText('尚未绑定设备通道', { exact: true })).toBeVisible();
  await pairing.getByRole('button', { name: '选择 位移设备1 / 位移输出', exact: true }).click();
  await page.getByRole('button', { name: '核对并绑定', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '预览并核对', exact: true }).click();
  await expect(page.getByRole('dialog').getByText('请填写本次绑定变更原因。', { exact: true }).first()).toBeVisible();
  await submitBinding(page, '现场接线核对完成');
  const current = page.getByRole('region', { name: '当前绑定设备', exact: true });
  await expect(current.locator('[data-current-device-code]')).toHaveText('BIND-01');
  const candidates = pairing.getByRole('region', { name: '候选设备通道', exact: true });
  await expect(candidates.getByText('BIND-01', { exact: true })).toHaveCount(0);
  const sourcePath = `${origin}/api/v1${root}/points/${point.id}/channels/${channel.id}/source`;
  expect((await (await request.get(sourcePath)).json()).source.output.id).toBe(devices[0]!.output);

  if (!isMobile) {
    const before = await current.boundingBox();
    await candidates.evaluate(node => { node.scrollTop = node.scrollHeight; });
    expect((await current.boundingBox())?.y).toBe(before?.y);
    const layout = await page.evaluate(() => {
      const left = document.querySelector('.binding-tree-panel')!.getBoundingClientRect();
      const right = document.querySelector('.binding-device-panel')!.getBoundingClientRect();
      const review = document.querySelector('.binding-review')!.getBoundingClientRect();
      return { difference: Math.abs(left.bottom - right.bottom), gap: Math.abs(review.top - left.bottom) };
    });
    expect(layout.difference).toBeLessThan(2); expect(layout.gap).toBeLessThan(2);
  }
  await candidates.evaluate(node => { node.scrollTop = 0; });
  await page.screenshot({ path: info.outputPath('binding-workbench.png'), fullPage: true });
  await pairing.getByRole('button', { name: '下一页设备', exact: true }).click();
  await expect(candidates.getByText('BIND-11', { exact: true })).toBeVisible();
  await pairing.getByRole('button', { name: '上一页设备', exact: true }).click();
  await pairing.getByRole('button', { name: '选择 位移设备2 / 位移输出', exact: true }).click();
  await page.getByRole('button', { name: '核对并更换', exact: true }).click();
  await submitBinding(page, '原设备检修，启用备用设备');
  await expect(current.locator('[data-current-device-code]')).toHaveText('BIND-02');
  expect((await (await request.get(sourcePath)).json()).source.output.id).toBe(devices[1]!.output);
  await current.getByRole('button', { name: '关系与历史', exact: true }).click();
  await expect(page.getByRole('dialog').getByText('原设备检修，启用备用设备', { exact: false }).first()).toBeVisible();
  await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
  await current.getByRole('button', { name: '解除绑定', exact: true }).click();
  await submitBinding(page, '拆除临时接线');
  await expect(current.getByText('尚未绑定设备通道', { exact: true })).toBeVisible();
  expect((await (await request.get(sourcePath)).json()).source.configured).toBe(false);
  const directSourcePath = '/api/v1' + root + `/points/${directPoint.id}/channels/${direct.id}/source`;
  const [directSourceResponse] = await Promise.all([
    page.waitForResponse(response => {
      const target = new URL(response.url());
      return response.request().method() === 'GET' && target.origin === origin &&
        target.pathname === directSourcePath && target.search === '' && target.hash === '';
    }),
    page.goto(url + `?channel=${direct.id}`),
  ]);
  expect(directSourceResponse.status()).toBe(200);
  await expect(pairing.getByText('回归桥梁 / 设施直属（无构件） / 桥面环境点', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '设计要求', exact: true }).click();
  await expect(page.getByRole('dialog').getByText('参考基准', { exact: true })).toBeVisible();
  await page.getByRole('dialog').getByRole('button', { name: '关闭', exact: true }).click();
  await expect(candidates.getByRole('button', { name: '选择 位移设备1 / 位移输出', exact: true })).toBeVisible();
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: info.outputPath('binding-workbench-dark.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});

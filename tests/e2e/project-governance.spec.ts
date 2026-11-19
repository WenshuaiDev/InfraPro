import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { prepareProject } from './project-fixtures';
import { test, expect, consoleURL, login, password } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';

const api = new URL('/api/v1', consoleURL).href;
test.use({ trace: 'off', actionTimeout: 15_000 });

test('governance B uses real summaries, filters, pagination and accessible responsive drawers', async ({ page, request, browser }, info) => {
  test.setTimeout(180_000);
  const f = await prepareProject(request, browser);
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  try {
    const first = await f.create('bridge-governance');
    const second = await f.create('corridor-governance', f.tid, f.bid, f.bid);
    await f.create('other-tenant', f.other, f.otherBid, f.otherBid);
    const path = `${api}/tenants/${f.tid}/projects/${second}/archive`;
    const preview = await (await f.pb.request.post(path + '/preview', { headers: f.pb.h, data: {} })).json();
    expect((await f.pb.request.post(path, { headers: { ...f.pb.h, 'X-Command-ID': randomUUID() }, data: { expected_version: preview.expected_version, expected_project_version: preview.expected_project_version, preview_hash: preview.preview_hash, reason: '治理统计验证', client_request_id: randomUUID() } })).status()).toBe(200);
    const response = await f.ta.request.get(`${api}/tenants/${f.tid}/project-governance?status=all`);
    expect(response.status(), await response.text()).toBe(200);
    const real = await response.json();
    expect(real.overview).toMatchObject({ total: 2, active: 1, archived: 1, attention: 0, filtered_total: 2 });
    expect(real.items[0].project.statistics).toEqual({ facilities: 0, points: 0, devices: 0, members: null });
    await login(page, f.admin);
    const url = `${consoleURL}/t/${f.tid}/projects`;
    await page.goto(url);
    const cards = page.locator('[data-project-card]');
    await expect(cards).toHaveCount(2);
    await expect(page.getByRole('button', { name: '查看全部项目', exact: true })).toContainText('2');
    await expect(page.getByRole('button', { name: '查看运行中', exact: true })).toContainText('1');
    await expect(cards.first().getByText('无查看权限', { exact: true })).toHaveCount(1);
    await expect(page.getByText('业务资料内容仅项目可读', { exact: true })).toHaveCount(0);
    await expect(page.getByText('项目 other-tenant', { exact: true })).toHaveCount(0);

    for (const width of info.project.name === 'desktop' ? [1440, 1024, 768] : [375]) {
      await page.setViewportSize({ width, height: 1000 });
      for (const theme of ['light', 'dark']) {
        await page.evaluate(value => document.documentElement.classList.toggle('dark', value === 'dark'), theme);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
        await page.screenshot({ path: info.outputPath(`governance-${width}-${theme}.png`), fullPage: true });
      }
    }
    await page.evaluate(() => document.documentElement.classList.remove('dark'));
    await page.getByLabel('项目状态', { exact: true }).selectOption('archived');
    await expect(cards).toHaveCount(1);
    await expect(cards).toContainText('corridor-governance');
    await page.reload();
    await expect(page.getByLabel('项目状态', { exact: true })).toHaveValue('archived');
    await page.getByRole('button', { name: '清除筛选', exact: true }).click();
    await page.getByLabel('项目负责人', { exact: true }).selectOption(f.cid);
    await expect(cards).toHaveCount(1);
    await expect(cards).toContainText('bridge-governance');
    await page.getByRole('button', { name: '清除筛选', exact: true }).click();
    await page.getByLabel('搜索项目名称或编号', { exact: true }).fill('no-match');
    await expect(page.getByText('没有匹配的项目', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '查看全部项目', exact: true })).toContainText('2');
    await page.getByRole('button', { name: '清除全部筛选', exact: true }).click();
    const trigger = page.locator('#project-detail-' + first);
    await trigger.click();
    let drawer = page.getByRole('dialog');
    await expect(drawer.getByRole('heading', { name: '项目 bridge-governance', exact: true })).toBeVisible();
    await expect(drawer.getByText('管理员可履职，当前没有管理空缺。', { exact: true })).toBeVisible();
    await expect(drawer.getByRole('button', { name: '恢复项目管理员', exact: true })).toHaveCount(0);
    const detailPath = `**/api/v1/tenants/${f.tid}/project-governance/${first}`;
    await page.route(detailPath, route => route.fulfill({ status: 503, json: { code: 'unavailable', message: '治理详情暂时不可用' } }));
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(drawer.getByRole('alert')).toContainText('治理详情暂时不可用');
    await expect(drawer.getByText('管理员可履职，当前没有管理空缺。', { exact: true })).toHaveCount(0);
    await page.unroute(detailPath);
    await drawer.getByRole('button', { name: '重新加载', exact: true }).click();
    await expect(drawer.getByText('管理员可履职，当前没有管理空缺。', { exact: true })).toBeVisible();
    await drawer.getByRole('button', { name: '授权概览', exact: true }).click();
    for (const label of ['资源授权', '设备授权', '空间资料授权', '监测数据授权']) {
      await expect(drawer.getByRole('link', { name: new RegExp(label) })).toHaveAttribute('href', new RegExp(first));
    }
    await page.reload();
    drawer = page.getByRole('dialog');
    await expect(drawer.getByRole('button', { name: '授权概览', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await drawer.getByRole('button', { name: '关闭面板', exact: true }).click();
    await expect(drawer).toHaveCount(0);
    await expect(page.getByLabel('搜索项目名称或编号', { exact: true })).toBeFocused();
    await trigger.click();
    await page.getByRole('dialog').press('Escape');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await page.goto(`${url}/${first}`);
    await expect(page.getByRole('heading', { name: '项目 bridge-governance', exact: true })).toBeVisible();
    await page.getByRole('link', { name: '返回项目治理', exact: true }).click();

    // Cross the real page boundary; totals remain tenant-wide on every page.
    const expectedProjects = [first, second];
    for (let index = 0; index < 19; index++) expectedProjects.push(await f.create('extra-' + index));
    await page.reload();
    await expect(cards).toHaveCount(20);
    await expect(page.getByRole('button', { name: '查看全部项目', exact: true })).toContainText('21');
    const pager = page.getByRole('navigation', { name: '项目治理分页', exact: true });
    const cardIDs = () => cards.evaluateAll(rows => rows.map(row => row.getAttribute('data-project-card')));
    const firstPage = await cardIDs();
    const next = pager.getByRole('button', { name: '下一页', exact: true });
    await next.click();
    await expect(cards).toHaveCount(1);
    const lastPage = await cardIDs();
    expect([...firstPage, ...lastPage].sort()).toEqual([...expectedProjects].sort());
    await expect(pager.getByRole('status')).toContainText('第 2 页 · 末页');
    await expect(page.getByText('已显示 1 / 21 个项目', { exact: false })).toBeVisible();
    await expect(next).toBeDisabled();
    await pager.getByRole('button', { name: '上一页', exact: true }).click();
    await expect(cards).toHaveCount(20);
    expect(await cardIDs()).toEqual(firstPage);
    await page.route(`**/api/v1/tenants/${f.tid}/project-governance?*`, route => route.fulfill({ status: 503, json: { code: 'unavailable', message: '项目列表暂时不可用' } }));
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(page.getByRole('alert')).toContainText('项目列表暂时不可用');
    await expect(cards).toHaveCount(0);
    await page.unroute(`**/api/v1/tenants/${f.tid}/project-governance?*`);
    await page.getByRole('button', { name: '重新加载', exact: true }).click();
    await expect(cards).toHaveCount(20);
    expect(await cardIDs()).toEqual(firstPage);
    // A changed qualification response must dispose of already-rendered cards.
    // Server-side role boundaries are independently verified by the Go tests.
    await page.route(`**/api/v1/tenants/${f.tid}/context`, async route => {
      const response = await route.fetch();
      const context = await response.json();
      await route.fulfill({ response, json: { ...context, permissions: context.permissions.filter((code: string) => code !== P.TenantProjectRead) } });
    });
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(page.getByText('没有项目治理读取权限。独立项目授权可通过左上角工作空间切换器进入项目。', { exact: true })).toBeVisible();
    await expect(cards).toHaveCount(0);
    expect(errors).toEqual([]);
  } finally { await f.close(); }
});

test('governance attention leads to the real administrator recovery flow', async ({ page }, info) => {
  test.setTimeout(120_000);
  test.skip(process.env.INFRAPRO_GOVERNANCE_E2E !== '1', 'Requires the isolated emergency-maintenance fixture');
  const fixture = JSON.parse(readFileSync(process.env.INFRAPRO_E2E_FIXTURE_FILE || 'build/e2e/fixture.json', 'utf8'));
  const prefix = 'governance_' + info.project.name + '_';
  const pid = fixture[prefix + 'project'] as string;
  await login(page, { username: fixture[prefix + 'admin'], password });
  await page.goto(`${consoleURL}/t/${fixture[prefix + 'tenant']}/projects`);
  await page.getByRole('button', { name: '查看需要关注', exact: true }).click();
  await expect(page.getByLabel('仅管理员不可用或空缺')).toBeChecked();
  await expect(page.locator('[data-project-card]')).toHaveCount(1);
  await page.locator('#project-detail-' + pid).click();
  await page.getByRole('dialog').getByRole('button', { name: '恢复项目管理员', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '恢复项目管理员', exact: true });
  await dialog.getByLabel('选择接任管理员').fill(fixture[prefix + 'successor']);
  await dialog.getByRole('button', { name: `治理验收（${fixture[prefix + 'successor']}）`, exact: true }).click();
  await dialog.getByRole('button', { name: '预览变更影响', exact: true }).click();
  await expect(dialog.getByRole('region', { name: '项目变更影响' })).toBeVisible();
  await dialog.getByLabel('变更原因', { exact: true }).fill('恢复可履职管理员');
  await dialog.getByRole('button', { name: '确认恢复项目管理员', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('dialog').getByText('管理员可履职，当前没有管理空缺。', { exact: true })).toBeVisible();
  await page.getByRole('dialog').getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(page.locator('[data-project-card]')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '查看需要关注', exact: true })).toContainText('0');
});

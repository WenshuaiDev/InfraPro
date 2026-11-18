import { randomUUID } from 'node:crypto';
import type { EngineeringCategory, EngineeringSummary } from '../../packages/ts/api-client/src/engineering';
import { consoleURL, expect, login, password, test } from './fixtures';

const catalog = consoleURL + '/platform/engineering/units';
const owner = { username: 'e2e_owner', password };

test('unit cards keep their height and scroll every page of units on hover', async ({ page, isMobile }) => {
  const categories: EngineeringCategory[] = [
    { key: 'length', name: '长度', description: '', revision: 1, used: true },
    { key: 'area', name: '面积', description: '', revision: 1, used: true },
    { key: 'volume', name: '体积', description: '', revision: 1, used: false },
  ];
  const units: EngineeringSummary[] = Array.from({ length: 105 }, (_, i) => ({
    id: randomUUID(), kind: 'units', code: 'unit_' + i, name: '单位 ' + i, has_draft: false,
    latest: 1, usable: true, source_version: '', unit: { symbol: 'u' + i, category: i === 0 ? 'area' : 'length' },
  }));
  const offsets: number[] = [];
  await login(page, owner);
  await page.route('**/api/v1/platform/engineering/unit-categories', route => route.fulfill({ json: categories }));
  await page.route('**/api/v1/platform/engineering/units?*', route => {
    const search = new URL(route.request().url()).searchParams;
    const offset = Number(search.get('offset')), limit = Number(search.get('limit'));
    offsets.push(offset);
    return route.fulfill({ json: { items: units.slice(offset, offset + limit), offset, limit, total: units.length } });
  });
  await page.goto(catalog);
  const cards = page.locator('.unit-category-card');
  await expect(cards).toHaveCount(3);
  expect(offsets).toEqual([0, 100]);
  const heights = await cards.evaluateAll(elements => elements.map(el => el.getBoundingClientRect().height));
  expect(heights).toEqual([176, 176, 176]);
  const long = cards.filter({ has: page.getByRole('heading', { name: /长度/ }) });
  const area = cards.filter({ has: page.getByRole('heading', { name: /面积/ }) });
  const empty = cards.filter({ has: page.getByRole('heading', { name: /体积/ }) });
  await expect(long.getByRole('listitem')).toHaveCount(104);
  await expect(area.getByRole('listitem')).toHaveCount(1);
  await expect(empty.getByRole('listitem')).toHaveCount(0);
  const viewport = long.getByRole('group', { name: '长度的单位' });
  const bar = long.locator('[data-slot="scroll-area-scrollbar"]');
  await expect(bar).toHaveCSS('opacity', '0');
  await expect(area.locator('[data-slot="scroll-area-scrollbar"]')).toHaveCount(0);
  await expect(empty.locator('[data-slot="scroll-area-scrollbar"]')).toHaveCount(0);
  if (!isMobile) {
    await long.getByRole('heading').hover();
    await expect(bar).toHaveCSS('opacity', '1');
    const top = await long.getByRole('heading').boundingBox();
    await viewport.hover();
    await page.mouse.wheel(0, 80);
    await expect.poll(() => viewport.evaluate(el => el.scrollTop)).toBeGreaterThan(0);
    expect((await long.getByRole('heading').boundingBox())?.y).toBe(top?.y);
    await page.mouse.move(1, 1);
    await expect(bar).toHaveCSS('opacity', '0');
  }
  await viewport.focus();
  await viewport.press('End');
  await expect.poll(() => viewport.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
  await expect(bar).toHaveCSS('opacity', '0');
  await page.getByRole('textbox', { name: '搜索类别、单位、符号或编号' }).fill('u104');
  await expect(cards).toHaveCount(1);
  await expect(long.getByRole('listitem')).toHaveCount(1);
  await expect(long).toContainText('单位 104');
  expect((await long.boundingBox())?.height).toBe(176);
});

test('platform unit catalog writes categories and units and returns to the current class', async ({ page }) => {
  test.setTimeout(90_000);
  const suffix = randomUUID().slice(0, 8);
  const categoryKey = 'cat_' + suffix, unitCode = 'unit_' + suffix;
  await login(page, owner);
  await page.goto(catalog);
  const button = (name: string) => page.getByRole('button', { name, exact: true });
  const dialog = page.getByRole('dialog');
  async function createCategory(key: string, name: string) {
    await button('新建类别').click();
    await dialog.getByLabel('类别名称').fill(name);
    await dialog.getByLabel('类别编号').fill(key);
    await dialog.getByLabel('类别含义').fill('目录交互验证');
    await dialog.getByLabel('变更原因').fill('建立验收记录');
    await dialog.getByRole('button', { name: '保存类别' }).click();
    await expect(page.getByRole('heading', { name, level: 1, exact: true })).toBeVisible();
  }
  await createCategory(categoryKey, '新类别' + suffix);
  await button('类别资料').click();
  await dialog.getByLabel('类别名称').fill('修改类别' + suffix);
  await dialog.getByLabel('变更原因').fill('修改未引用类别');
  await dialog.getByRole('button', { name: '保存类别' }).click();
  await expect(page.getByRole('heading', { name: '修改类别' + suffix, exact: true })).toBeVisible();
  await button('新建单位').click();
  await expect(dialog.getByRole('combobox', { name: '单位类别' })).toHaveText('修改类别' + suffix);
  await dialog.getByLabel('单位名称').fill('测试尺');
  await dialog.getByLabel('单位符号').fill('尺');
  await dialog.getByLabel('单位编号').fill(unitCode);
  await dialog.getByLabel('创建原因').fill('验证类别归属');
  await dialog.getByRole('button', { name: '创建草稿' }).click();
  await expect(button('保存草稿')).toBeVisible();
  const unitId = new URL(page.url()).searchParams.get('definition');
  const readback = await page.request.get(consoleURL + '/api/v1/platform/engineering/units/' + unitId);
  expect((await readback.json()).draft.content.unit).toEqual({ category: categoryKey, symbol: '尺' });
  await page.getByLabel('本次变更原因').fill('验证正常发布');
  await button('发布当前草稿').click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认', exact: true }).click();
  await expect(button('从此版本建立草稿')).toBeVisible();
  await button('返回目录').click();
  const table = page.getByRole('table', { name: '修改类别' + suffix + '的单位' });
  await expect(table).toContainText('测试尺');
  await expect(table).toContainText('可新使用');
  await button('类别资料').click();
  await expect(dialog.getByText('已引用', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '保存类别' })).toHaveCount(0);
  await dialog.getByRole('button', { name: '关闭面板' }).click();
  await button('返回类别总览').click();
  const search = page.getByRole('textbox', { name: '搜索类别、单位、符号或编号' });
  await search.fill(unitCode);
  await page.getByRole('button', { name: '浏览修改类别' + suffix + '，1 个单位' }).click();
  const within = page.getByRole('textbox', { name: '搜索本类单位名称、符号或编号' });
  await within.fill('尺');
  await table.getByRole('button', { name: '测试尺', exact: true }).click();
  await button('返回目录').click();
  await expect(within).toHaveValue('尺');
  await button('返回类别总览').click();
  await expect(search).toHaveValue(unitCode);
  await button('重置').click();
  await createCategory('unused_' + suffix, '空类别' + suffix);
  await button('类别资料').click();
  await dialog.getByLabel('变更原因').fill('删除空类别');
  await dialog.getByRole('button', { name: '删除未引用类别' }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认', exact: true }).click();
  await expect(search).toBeVisible();
  await expect(page.getByRole('button', { name: new RegExp('浏览空类别' + suffix) })).toHaveCount(0);
});

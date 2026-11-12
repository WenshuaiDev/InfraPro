import { test, expect, login, consoleURL } from './fixtures';
import { browseWorkspaceTenant, openWorkspacePicker, selectWorkspaceType } from '../helpers/navigation';
import { prepareWorkspaces } from '../helpers/workspaces';

test('workspace type options stay inside the dialog and accept pointer input', async ({ page, request }, info) => {
  const f = await prepareWorkspaces(request);
  await login(page, f.account);
  const dialog = await openWorkspacePicker(page);
  const trigger = dialog.getByRole('combobox', { name: '工作空间类型', exact: true });
  for (const label of ['租户', '项目', '全部类型']) {
    await trigger.click();
    const option = page.getByRole('option', { name: label, exact: true });
    await expect(option).toBeVisible();
    expect(await option.evaluate((node) => Boolean(node.closest('[role="dialog"]')))).toBe(true);
    if (label === '租户') {
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: info.outputPath('workspace-type-popup.png') });
    }
    await option.click();
    await expect(trigger).toHaveText(label);
    await expect(page.getByRole('listbox')).toHaveCount(0);
  }
  await trigger.click();
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(page.getByRole('option', { name: '租户', exact: true })).toHaveCount(0);
});

test('tenant browsing keeps the picker layout stable while the project request is pending', async ({ page, request }, info) => {
  const f = await prepareWorkspaces(request);
  await login(page, f.account);
  await page.goto(`${consoleURL}/t/${f.tenantIds[0]}/profile`);
  const dialog = await openWorkspacePicker(page);
  await browseWorkspaceTenant(page, '交通监测中心');
  await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
  const anchor = info.project.name === 'mobile' ? dialog.getByLabel('选择租户', { exact: true }) : dialog.getByRole('group', { name: '租户列表' });
  const before = await anchor.boundingBox();
  let release = () => {};
  let started = () => {};
  const held = new Promise<void>((resolve) => { release = resolve; });
  const pending = new Promise<void>((resolve) => { started = resolve; });
  await page.route('**/api/v1/me/workspaces?*', async (route) => {
    const url = new URL(route.request().url());
    if (url.searchParams.get('kind') === 'project' && url.searchParams.get('tenant_id') === f.tenantIds[1]) { started(); await held; }
    await route.continue();
  });
  try {
    await browseWorkspaceTenant(page, '城市基础设施中心');
    await pending;
    await expect(dialog.getByRole('heading', { name: '城市基础设施中心', exact: true })).toBeVisible();
    const during = await anchor.boundingBox();
    expect(during?.y, 'pending project data must not push the tenant pane down').toBeCloseTo(before?.y ?? 0, 0);
    await expect(dialog.getByText('正在加载工作空间…', { exact: true })).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toHaveCount(0);
    await expect(dialog.getByRole('status', { name: '正在加载项目…', exact: true })).toBeVisible();
    release();
    await expect(dialog.getByRole('button', { name: /^隧道运行监测/ })).toBeVisible();
    const after = await anchor.boundingBox();
    expect(after?.y, 'settled project data must preserve the tenant pane position').toBeCloseTo(before?.y ?? 0, 0);
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }).catch(() => undefined); }
});

test('reopening reuses verified entries and a newer authorization revision discards them', async ({ page, request }) => {
  const f = await prepareWorkspaces(request);
  const snapshot = await (await request.get(`${consoleURL}/api/v1/me/workspaces?limit=100`)).json();
  let revision = snapshot.authorization_revision as string;
  let release = () => {};
  let hold: Promise<void> | undefined;
  let listReads = 0;
  // Freeze the revision until the deliberate change below: other parallel
  // fixtures may advance the database-wide revision while this test is running.
  await page.route('**/api/v1/auth/me', async (route) => {
    const response = await route.fetch();
    await route.fulfill({ response, headers: { ...response.headers(), 'x-authorization-revision': revision }, json: { ...await response.json(), authorization_revision: revision } });
  });
  await page.route('**/api/v1/me/workspaces?*', async (route) => {
    const kind = new URL(route.request().url()).searchParams.get('kind');
    if (!kind) return route.fulfill({ headers: { 'x-authorization-revision': revision }, json: { ...snapshot, authorization_revision: revision } });
    listReads++;
    await hold;
    const response = await route.fetch();
    await route.fulfill({ json: { ...await response.json(), authorization_revision: revision } });
  });
  try {
    await login(page, f.account);
    await page.goto(`${consoleURL}/t/${f.tenantIds[0]}/profile`);
    const dialog = await openWorkspacePicker(page);
    await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    hold = new Promise<void>((resolve) => { release = resolve; });
    listReads = 0;
    await openWorkspacePicker(page);
    await expect.poll(() => listReads).toBeGreaterThan(0);
    await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
    await expect(dialog.getByRole('status', { name: /^正在加载/ })).toHaveCount(0);
    await page.keyboard.press('Escape');
    release();
    await expect(dialog).toBeHidden();
    revision = (BigInt(revision) + 1n).toString();
    hold = new Promise<void>((resolve) => { release = resolve; });
    await openWorkspacePicker(page);
    await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: /^浏览租户：交通监测中心/ })).toHaveCount(0);
    release();
    await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toBeVisible();
  } finally { release(); await page.unrouteAll({ behavior: 'wait' }).catch(() => undefined); }
});

test('centered workspace dialog blurs the backdrop, contains focus and restores its trigger', async ({ page, request }, info) => {
  const f = await prepareWorkspaces(request);
  await login(page, f.account);
  const dialog = await openWorkspacePicker(page);
  await expect(dialog).toHaveAttribute('aria-modal', 'true');
  await expect(dialog.getByLabel('搜索名称或编号')).toBeFocused();
  const type = dialog.getByRole('combobox', { name: '工作空间类型', exact: true });
  await type.focus();
  await page.keyboard.press('ArrowDown');
  await expect(page.getByRole('listbox')).toBeVisible();
  await expect(page.getByRole('option', { name: '全部类型', exact: true })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByRole('option', { name: '全部类型', exact: true })).toBeFocused();
  await page.keyboard.press('End');
  await expect(page.getByRole('option', { name: '项目', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(type).toHaveText('项目');
  await expect(page.getByRole('listbox')).toHaveCount(0);
  await expect(dialog).toBeVisible();
  await type.click();
  await expect(page.getByRole('listbox')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('listbox')).toHaveCount(0);
  await expect(dialog).toBeVisible();
  await expect(type).toBeFocused();
  await selectWorkspaceType(page, 'all');
  const geometry = await dialog.evaluate((element) => {
    const box = element.getBoundingClientRect();
    return { x: box.x + box.width / 2 - innerWidth / 2, y: box.y + box.height / 2 - innerHeight / 2, fits: box.x >= 0 && box.y >= 0 && box.right <= innerWidth && box.bottom <= innerHeight };
  });
  expect(Math.abs(geometry.x)).toBeLessThan(2);
  expect(Math.abs(geometry.y)).toBeLessThan(2);
  expect(geometry.fits).toBe(true);
  await expect(page.locator('[data-slot="dialog-overlay"]').last()).toHaveCSS('backdrop-filter', 'blur(8px)');
  await browseWorkspaceTenant(page, '城市基础设施中心');
  await expect(dialog.getByRole('button', { name: /^隧道运行监测/ })).toBeVisible();
  await expect(dialog.getByRole('button', { name: /^桥梁健康监测/ })).toHaveCount(0);
  for (const key of [...Array<string>(16).fill('Tab'), ...Array<string>(16).fill('Shift+Tab')]) {
    await page.keyboard.press(key);
    expect(await dialog.evaluate((element) => element.contains(document.activeElement))).toBe(true);
  }
  await page.screenshot({ path: info.outputPath('workspace-dialog-blur-light.png'), fullPage: true });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: info.outputPath('workspace-dialog-blur-dark.png'), fullPage: true });
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
  await openWorkspacePicker(page);
  await page.mouse.click(2, 2);
  await expect(dialog).toBeHidden();
  await expect(page.getByRole('button', { name: /^切换工作空间：/ })).toBeFocused();
  await openWorkspacePicker(page);
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(dialog).toBeHidden();
  // Blur is scoped to this picker; ordinary account dialogs keep their overlay.
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: '账号安全', exact: true })).toBeVisible();
});

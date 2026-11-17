import { expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { join } from 'node:path';

/** Real management commands run only against display-browser-check's isolated fixture. */
export async function checkDisplayWorkspace({ page, call, workspace, consoleOrigin, tenant, project, instance, out }) {
  const root = `/tenants/${tenant}/projects/${project}/displays`, path = `${root}/instances/${instance.id}`;
  const wm = revision => ({ request_id: randomUUID(), reason: 'B 方案界面回归', expected_revision: revision });
  const checks = [];
  const url = consoleOrigin + workspace;
  const drawer = () => page.getByRole('dialog');
  const navigate = async (id = instance.id, packageID) => { await page.goto(`${url}?instance=${id}${packageID ? `&package=${packageID}` : ''}`); await expect(page.getByRole('button', { name: '关闭面板', exact: true })).toBeVisible(); };
  const tab = label => drawer().getByRole('button', { name: label, exact: true }).click();
  const close = async () => { await tab('关闭面板'); await expect(drawer()).toHaveCount(0); };
  const confirm = async label => { await page.getByRole('alertdialog').getByRole('button', { name: `确认${label}`, exact: true }).click(); await expect(page.getByRole('alertdialog')).toHaveCount(0); };
  await page.setViewportSize({ width: 1440, height: 1000 });
  await navigate();
  await tab('复制访问地址'); await expect(drawer().getByText('请选中地址后手动复制', { exact: true })).toBeVisible(); checks.push('HTTP clipboard fallback');
  await tab('管理授权'); await drawer().getByRole('button', { name: '保存读取授权', exact: true }).click(); await expect(drawer().getByText('已保存', { exact: true })).toBeVisible(); await tab('返回概览');
  const before = await call(path);
  await tab('编辑配置'); const timeMode = drawer().getByLabel('时间模式', { exact: true }); await timeMode.selectOption('follow');
  const refreshURL = `**/api/v1/tenants/${tenant}/projects/${project}/context`;
  await page.route(refreshURL, async route => { await new Promise(resolve => setTimeout(resolve, 600)); await route.continue(); });
  await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(timeMode).toBeDisabled(); await expect(timeMode).toHaveValue('follow'); await expect(timeMode).toBeEnabled(); await expect(timeMode).toHaveValue('follow'); await page.unroute(refreshURL);
  checks.push('background identity refresh preserves unsaved input');
  await tab('保存草稿'); await expect(drawer().getByRole('button', { name: '编辑配置', exact: true })).toBeVisible();
  const saved = await call(path); expect(saved.instance.current_release_id).toBe(before.instance.current_release_id); expect(saved.instance.draft_config_id).not.toBe(before.instance.draft_config_id);
  checks.push('saving a draft preserves the current release'); console.log('Display workspace: draft and identity refresh passed.');
  await tab('发布更改'); await tab('核验版本'); await expect(drawer().getByText('核验通过', { exact: true })).toBeVisible();
  const concurrent = await call(path); await call(path, { ...wm(concurrent.instance.revision), name: concurrent.instance.name, state: concurrent.instance.state, viewers: concurrent.instance.viewers }, 'PATCH');
  await expect(drawer().getByText('大屏配置或授权已更新，请重新核验。', { exact: true })).toBeVisible({ timeout: 15000 }); await expect(drawer().getByRole('button', { name: '确认发布', exact: true })).toHaveCount(0);
  await tab('重新核验'); await tab('确认发布'); await expect(drawer().getByText('当前发布', { exact: true })).toBeVisible();
  const published = await call(path); expect(published.instance.current_release_id).not.toBe(saved.instance.current_release_id); checks.push('revision change invalidates preparation before confirmation');
  const historyRoute = `**/api/v1${path}`, historicalConfig = published.releases.find(r => r.id !== published.instance.current_release_id).config_id;
  await page.route(historyRoute, async route => { const response = await route.fetch(); const data = await response.json(); await route.fulfill({ response, json: { ...data, configs: data.configs.filter(c => c.id !== historicalConfig) } }); });
  await navigate(); await tab('发布记录');
  await drawer().getByRole('button', { name: '回退至此版本', exact: true }).filter({ visible: true }).nth(1).click(); await tab('核验版本'); await tab('确认回退'); await expect(drawer().getByText('当前发布', { exact: true })).toBeVisible();
  const rolled = await call(path); expect(rolled.instance.current_release_id).not.toBe(published.instance.current_release_id); expect(rolled.releases.length).toBe(published.releases.length + 1); checks.push('rollback creates a new release even when historical config is outside the detail window'); await page.unroute(historyRoute);
  await drawer().getByRole('button', { name: '撤回发布', exact: true }).last().click(); await confirm('撤回发布'); await expect(drawer().getByText('已撤回', { exact: true }).first()).toBeVisible(); checks.push('withdraw confirmation and release status');
  await tab('运行与终端'); await drawer().getByLabel('终端名称', { exact: true }).fill('B 方案墙屏'); await tab('签发配对码（90 天）'); await expect(drawer().getByText(/一次性配对码：/)).toBeVisible();
  let terminal = (await call(path)).terminals.find(t => t.name === 'B 方案墙屏'); expect(terminal).toBeTruthy();
  const terminalCard = drawer().locator('div.rounded-lg.border.p-4').filter({ has: page.getByText('B 方案墙屏 · 待配对', { exact: true }) });
  await terminalCard.getByRole('button', { name: '重置配对', exact: true }).click(); await expect.poll(async () => (await call(path)).terminals.find(t => t.id === terminal.id).generation).not.toBe(terminal.generation);
  const other = await call(root + '/instances', { ...wm('0'), name: 'B 方案备用屏' });
  await expect(drawer().getByLabel('改绑 B 方案墙屏')).toContainText('B 方案备用屏', { timeout: 20000 }); await drawer().getByLabel('改绑 B 方案墙屏').selectOption(other.id);
  await expect.poll(async () => (await call(`${root}/instances/${other.id}`)).terminals.some(t => t.id === terminal.id)).toBe(true);
  await navigate(other.id); await tab('运行与终端'); await drawer().getByRole('button', { name: '撤销', exact: true }).click(); await expect.poll(async () => (await call(`${root}/instances/${other.id}`)).terminals[0].state).toBe('revoked'); checks.push('terminal issue, reset, reassign and revoke'); console.log('Display workspace: publication and terminals passed.');
  await tab('大屏设置'); await tab('停用大屏'); await confirm('停用大屏'); await expect(drawer().getByText('已停用', { exact: true })).toBeVisible(); await tab('启用大屏'); await confirm('启用大屏');
  await drawer().getByLabel('实例名称', { exact: true }).fill('B 方案归档屏'); await tab('保存实例信息'); await expect(drawer().getByRole('heading', { name: 'B 方案归档屏', exact: true })).toBeVisible();
  await tab('归档大屏'); await confirm('归档大屏'); await tab('返回概览'); await expect(drawer().getByRole('button', { name: '发布', exact: true })).toHaveCount(0); await expect(drawer().getByRole('button', { name: '打开大屏', exact: true })).toBeDisabled(); checks.push('rename, stop, enable and archive retain read-only detail');
  await close(); await page.getByLabel('筛选大屏状态').selectOption('archived'); await expect(page.getByRole('article')).toHaveCount(1); await page.getByLabel('搜索大屏名称').fill('不存在'); await expect(page.getByText('没有匹配的大屏', { exact: true })).toBeVisible(); await page.getByRole('button', { name: '清除筛选', exact: true }).click(); checks.push('gallery search and status filtering');
  const trigger = page.locator(`#display-manage-${instance.id}`); await trigger.click(); await expect(drawer().getByRole('heading', { name: instance.name, exact: true })).toBeVisible(); await page.keyboard.press('Escape'); await expect(drawer()).toHaveCount(0); await expect(trigger).toBeFocused();
  await trigger.click(); await page.goBack(); await expect(drawer()).toHaveCount(0); await page.goForward(); await expect(drawer()).toBeVisible(); checks.push('Escape, focus restoration and browser history');
  const pack = (await call(path)).packages[0]; await navigate(instance.id, pack.id); await expect(drawer().getByRole('button', { name: '程序版本', exact: true })).toHaveAttribute('aria-pressed', 'true'); await expect(page.locator(`#display-package-${pack.id}`)).toBeFocused(); checks.push('instance and package deep links');
  await close();
  for (const width of [1440, 1024, 768, 390]) {
    await page.setViewportSize({ width, height: width < 500 ? 844 : 1000 });
    for (const theme of ['light', 'dark']) {
      await page.evaluate(theme => document.documentElement.classList.toggle('dark', theme === 'dark'), theme);
      await expect(page.getByRole('article').first()).toBeVisible(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      const clipped = await page.getByRole('article').evaluateAll(cards => cards.some(card => { const bounds = card.getBoundingClientRect(); return [...card.querySelectorAll('footer button, footer a')].some(button => button.getBoundingClientRect().right > bounds.right + 1); })); expect(clipped).toBe(false);
      await page.screenshot({ path: join(out, `gallery-${width}-${theme}.png`), fullPage: true, animations: 'disabled' });
      await trigger.click(); await expect(drawer()).toBeVisible(); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await page.screenshot({ path: join(out, `drawer-${width}-${theme}.png`), fullPage: false, animations: 'disabled' }); await close();
    }
  }
  checks.push('four viewport widths and light/dark themes'); console.log('Display workspace: gallery, navigation and responsive layout passed.');
  await page.evaluate(() => document.documentElement.classList.remove('dark')); await page.setViewportSize({ width: 1440, height: 1000 });
  // Fault injection verifies fail-closed rendering; authorization itself remains server-owned.
  const detailURL = `**/api/v1${path}`;
  await page.route(detailURL, route => route.fulfill({ status: 503, json: { code: 'unavailable', message: '大屏详情暂时不可用' } })); await navigate(); await expect(drawer().getByRole('alert')).toContainText('大屏详情暂时不可用'); await expect(drawer().getByRole('link', { name: '打开大屏' })).toHaveCount(0); await page.unroute(detailURL); await tab('重新加载'); await expect(drawer().getByRole('heading', { name: '线上版本', exact: true })).toBeVisible(); checks.push('detail failure and retry'); await close();
  const contextURL = `**/api/v1/tenants/${tenant}/projects/${project}/context`;
  await page.route(contextURL, async route => { const response = await route.fetch(); const data = await response.json(); data.authorization.permissions = ['project.display.view']; await route.fulfill({ response, json: data }); });
  await page.route(detailURL, async route => { const response = await route.fetch(); const data = await response.json(); await route.fulfill({ response, json: { instance: data.instance, access_url: data.access_url } }); });
  await navigate(); await expect(drawer().getByRole('heading', { name: '访问', exact: true })).toBeVisible(); for (const label of ['程序版本', '编辑配置', '大屏设置', '管理授权', '发布更改']) await expect(drawer().getByRole('button', { name: label, exact: true })).toHaveCount(0); await expect(drawer().getByRole('link', { name: '打开大屏', exact: true })).toBeVisible(); checks.push('view-only permission and restricted DTO presentation'); await page.unroute(contextURL); await page.unroute(detailURL);
  await navigate(); await close(); await page.getByRole('button', { name: '项目大屏设置', exact: true }).click(); await page.getByRole('button', { name: '停用项目大屏', exact: true }).click(); await expect(page.getByText('项目大屏已停用', { exact: true }).first()).toBeVisible(); await expect(page.getByRole('article')).toHaveCount((await call(root)).items.length); await expect(page.getByRole('link', { name: '打开大屏', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '启用项目大屏', exact: true }).click(); await page.getByRole('button', { name: '确认启用', exact: true }).click(); await expect.poll(async () => (await call(root)).settings.enabled).toBe(true); checks.push('project stop and enable keep existing cards');
  return checks;
}

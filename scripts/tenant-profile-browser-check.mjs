import { chromium, expect as baseExpect } from '@playwright/test';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { startFilesLab } from './fixtures/files-lab.mjs';
import { seedFilesAcceptance, filesClient, filesLogin } from './fixtures/files-acceptance.mjs';

const expect = baseExpect.configure({ timeout: 20000 });
const out = resolve('.artifacts/tenant-profile/browser');
mkdirSync(out, { recursive: true });
const checks = [], errors = [];
let lab, browser, page, api;
const mark = note => { checks.push(note); console.log(note); };
const hash = bytes => createHash('sha256').update(bytes).digest('hex');

async function signIn(target, credentials) {
  await target.goto(lab.origin + '/login');
  await target.getByLabel('用户名', { exact: true }).fill(credentials.username);
  await target.getByLabel('密码', { exact: true }).fill(credentials.password);
  await target.getByRole('button', { name: '登录', exact: true }).click();
  await target.waitForURL(url => url.pathname !== '/login');
}

try {
  lab = await startFilesLab({ build: process.env.INFRAPRO_FILES_SKIP_BUILD !== '1', consolePort: 15694, apiPort: 18694, directory: join(out, 'lab'), consoleDist: process.env.INFRAPRO_PROFILE_CONSOLE_DIST });
  const state = await seedFilesAcceptance(lab);
  api = await filesClient(lab.origin);
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce', acceptDownloads: true });
  page = await context.newPage();
  page.on('pageerror', error => errors.push(error.message));
  await signIn(page, filesLogin);
  const tenant = state.tenants[0], profilePath = `/t/${tenant}/profile`, base = `/tenants/${tenant}`, go = () => page.goto(lab.origin + profilePath);
  await go();
  await expect(page.getByRole('heading', { name: '关于本租户', exact: true })).toBeVisible();
  await expect(page.getByText('尚未填写租户说明。', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '资料附件', exact: true })).toBeVisible();
  await expect(page.getByText('尚未添加普通附件。', { exact: true })).toBeVisible();
  await expect(page.getByText('当前租户权限', { exact: true })).toHaveCount(0);
  await expect(page.getByText('管理员可用性', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('complementary', { name: '租户管理信息', exact: true })).toBeVisible();
  mark('B layout: tenant identity, empty description/attachments, management rail, no permission or normal-availability dump');

  await page.getByRole('button', { name: '编辑资料', exact: true }).click();
  const editor = page.getByRole('dialog', { name: '编辑租户资料', exact: true });
  await editor.getByLabel('租户名称', { exact: true }).fill('华东交通监测中心 · B 方案验收');
  await editor.getByLabel('租户说明', { exact: true }).fill('组织资料与附件查阅验收，独立测试数据。');
  await editor.getByRole('button', { name: '确认提交', exact: true }).click();
  await expect(editor.getByText('请填写变更原因。', { exact: true })).toBeVisible();
  await editor.getByLabel('变更原因', { exact: true }).fill('验证 B 方案资料编辑与审计');
  await editor.getByRole('button', { name: '确认提交', exact: true }).click();
  await expect(editor).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('main').getByRole('heading', { name: '华东交通监测中心 · B 方案验收', exact: true })).toBeVisible();
  const saved = await api.call(base + '/profile');
  expect(saved.description).toBe('组织资料与附件查阅验收，独立测试数据。');
  await page.getByRole('link', { name: '变更记录', exact: true }).click();
  await page.waitForURL(url => url.pathname.endsWith('/audit'));
  expect(new URL(page.url()).searchParams.get('object_type')).toBe('tenant');
  expect(new URL(page.url()).searchParams.get('object_id')).toBe(tenant);
  await go();
  await page.getByRole('link', { name: '查看成员', exact: true }).click();
  await page.waitForURL(url => url.pathname.endsWith('/members'));
  await go();
  mark('Real profile edit persists after reload, still requires a reason, and member/history links keep tenant scope');

  await page.getByRole('button', { name: '添加附件', exact: true }).click();
  await page.getByLabel('选择普通附件', { exact: true }).setInputFiles(join(state.samples, '合成平面图.png'));
  await page.getByLabel('显示名称', { exact: true }).fill('B 方案原位预览.png');
  await page.getByLabel('操作原因', { exact: true }).fill('验证资料页上传与预览闭环');
  await page.getByRole('button', { name: '上传并提交检查', exact: true }).click();
  await expect(page.getByRole('button', { name: '查看本代附件修订', exact: true })).toBeVisible({ timeout: 45000 });
  expect(new URL(page.url()).pathname).toBe(profilePath);
  await page.getByRole('button', { name: '查看本代附件修订', exact: true }).click();
  await expect(page.getByRole('img', { name: 'B 方案原位预览.png 修订 1 的安全预览', exact: true })).toBeVisible();
  const attachment = new URL(page.url()).searchParams.get('attachment');
  expect(attachment).toBeTruthy();
  const downloadEvent = page.waitForEvent('download');
  await page.getByRole('link', { name: '下载原件', exact: true }).click();
  const download = await downloadEvent;
  await download.saveAs(join(out, 'download.png'));
  expect(hash(readFileSync(join(out, 'download.png')))).toBe(hash(readFileSync(join(state.samples, '合成平面图.png'))));
  await page.reload();
  await expect(page.getByRole('img', { name: 'B 方案原位预览.png 修订 1 的安全预览', exact: true })).toBeVisible();
  expect(new URL(page.url()).pathname).toBe(profilePath);
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveCount(0);
  const fileTrigger = page.getByRole('button', { name: '查看附件 B 方案原位预览.png', exact: true });
  await expect(fileTrigger).toBeVisible();
  await expect(page.getByRole('list', { name: '附件列表', exact: true })).toBeVisible();
  await expect(page.getByRole('table', { name: '附件列表', exact: true })).toHaveCount(0);
  await fileTrigger.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-slot="drawer-popup"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveCount(0);
  await expect(fileTrigger).toBeFocused();
  mark('Real upload/checker/preview/download work in the profile route; deep links reload and keyboard close restores list focus');

  await page.getByRole('button', { name: 'B 方案原位预览.png 的更多操作', exact: true }).click();
  await page.getByRole('menuitem', { name: '编辑信息', exact: true }).click();
  await expect(page.locator('#attachment-name')).toBeVisible();
  await page.locator('#attachment-name').fill('B 方案已改名.png');
  await page.locator('#attachment-reason').fill('验证紧凑列表的真实管理操作');
  await page.getByRole('button', { name: '保存名称与说明', exact: true }).click();
  await expect(page.locator('#attachment-name')).toHaveCount(0);
  await page.getByRole('button', { name: '关闭面板', exact: true }).click();
  await expect(page.getByRole('button', { name: '查看附件 B 方案已改名.png', exact: true })).toBeVisible();
  expect((await api.call(base + '/files/attachments/' + attachment)).display_name).toBe('B 方案已改名.png');
  mark('Compact attachment menu edits real metadata and refreshes the list without losing the profile');

  for (const width of [1440, 1024, 720, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: join(out, `profile-${width}.png`), fullPage: true, animations: 'disabled' });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: join(out, 'profile-dark.png'), fullPage: true, animations: 'disabled' });
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  mark('Light/dark layouts and 1440/1024/720/375/320px widths remain usable without horizontal overflow');

  const profilePattern = '**/tenants/' + tenant + '/profile';
  await page.route(profilePattern, async route => {
    if (route.request().method() !== 'GET') return route.continue();
    const response = await route.fetch(), value = await response.json();
    await route.fulfill({ response, json: { ...value, administrator_available: false, name: '交通基础设施监测组织'.repeat(10), description: '验收说明'.repeat(100) } });
  });
  await go();
  await expect(page.getByText('租户管理员暂不可用', { exact: true })).toBeVisible();
  for (const width of [1440, 375, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  await page.screenshot({ path: join(out, 'long-content-admin-unavailable.png'), fullPage: true, animations: 'disabled' });
  await page.unroute(profilePattern);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await go();
  mark('Injected unavailable-administrator and long-content responses show contextual guidance and wrap at narrow widths');

  for (const role of ['profile', 'reader', 'downloader', 'manager']) {
    const viewer = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
    try {
      const target = await viewer.newPage();
      target.on('pageerror', error => errors.push(error.message));
      await signIn(target, state.accounts[role]);
      await target.goto(lab.origin + profilePath);
      await expect(target.getByRole('heading', { name: '关于本租户', exact: true })).toBeVisible();
      await expect(target.getByText('当前租户权限', { exact: true })).toHaveCount(0);
      await expect(target.getByRole('button', { name: '编辑资料', exact: true })).toHaveCount(role === 'manager' ? 1 : 0);
      await expect(target.getByRole('link', { name: '查看成员', exact: true })).toHaveCount(0);
      await expect(target.getByRole('link', { name: '变更记录', exact: true })).toHaveCount(0);
      if (role === 'profile') {
        await expect(target.getByText('当前没有本空间普通附件查看权限；资料查看权与附件权限分别授予。', { exact: true })).toBeVisible();
        await expect(target.locator('[data-file-row]')).toHaveCount(0);
      } else {
        await expect(target.getByRole('button', { name: '添加附件', exact: true })).toHaveCount(role === 'manager' ? 1 : 0);
        await target.getByRole('button', { name: '查看附件 B 方案已改名.png', exact: true }).click();
        await expect(target.getByRole('heading', { name: 'B 方案已改名.png', exact: true })).toBeVisible();
        await expect(target.getByRole('link', { name: '下载原件', exact: true })).toHaveCount(role === 'downloader' ? 1 : 0);
        await expect(target.getByRole('img', { name: 'B 方案已改名.png 修订 1 的安全预览', exact: true })).toHaveCount(role === 'downloader' ? 1 : 0);
        await target.getByRole('button', { name: '附件更多操作', exact: true }).click();
        await expect(target.getByRole('menuitem', { name: '编辑信息', exact: true })).toHaveCount(role === 'manager' ? 1 : 0);
      }
    } finally { await viewer.close(); }
  }
  mark('Real profile-only/read/download/manage accounts retain independent permissions and no unauthorized member/history entry');

  await page.getByRole('button', { name: '查看附件 B 方案已改名.png', exact: true }).click();
  const contextPattern = '**/tenants/' + tenant + '/context';
  await page.route(contextPattern, async route => {
    const response = await route.fetch(), value = await response.json();
    await route.fulfill({ response, json: { ...value, permissions: [] } });
  });
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByText('当前未获资料查看权限，请联系租户管理员。', { exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="drawer-popup"]')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: '华东交通监测中心 · B 方案验收', exact: true })).toHaveCount(0);
  await expect(page.getByText('当前租户权限', { exact: true })).toHaveCount(0);
  await page.unroute(contextPattern);
  await go();
  await expect(page.getByRole('heading', { name: '关于本租户', exact: true })).toBeVisible();
  mark('Injected profile permission loss clears the open preview and profile details; permission internals are not substituted');

  await page.route(profilePattern, route => route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ code: 'permission_denied', message: '资料读取资格已变化' }) }));
  // Profile reads poll every 30 seconds; a window focus event only rechecks
  // tenant membership, so wait for the actual scheduled profile response.
  await page.waitForResponse(response => response.url().endsWith(base + '/profile') && response.status() === 403, { timeout: 45000 });
  // A permission_denied response is handled first by TenantBoundary: it
  // clears tenant queries and rechecks membership before the page can render
  // a local error. Verify that protected content disappears in either path.
  await expect(page.getByRole('heading', { name: '关于本租户', exact: true })).toHaveCount(0);
  await expect(page.locator('[data-file-row]')).toHaveCount(0);
  await page.unroute(profilePattern);
  await go();
  await expect(page.getByRole('heading', { name: '关于本租户', exact: true })).toBeVisible();
  mark('A denied profile refresh hides cached data and attachments; a fresh allowed read recovers');

  if (errors.length) throw Error(errors.join('; '));
  writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: true, synthetic: true, checks, errors, note: 'Writes use isolated real backend data. Administrator/long-content/revocation/error scenarios explicitly inject read responses.' }, null, 2));
  console.log(`Passed ${checks.length} tenant-profile browser journeys.`);
} catch (error) {
  if (page) await page.screenshot({ path: join(out, 'failure.png'), fullPage: true }).catch(() => {});
  writeFileSync(join(out, 'result.json'), JSON.stringify({ passed: false, checks, errors, error: String(error) }, null, 2));
  throw error;
} finally {
  await api?.client.dispose();
  await browser?.close();
  await lab?.close();
}

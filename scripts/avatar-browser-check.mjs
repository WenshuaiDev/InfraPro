import { spawn, execFileSync } from 'node:child_process';
import { mkdirSync, appendFileSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { createServer } from 'node:net';
import { randomUUID } from 'node:crypto';
import { chromium, expect, request } from '@playwright/test';
import { ensureTestDependencies, testEnv } from './test-environment.mjs';
import { notificationClient, notificationLogin } from './fixtures/notifications-acceptance.mjs';

const root = resolve('.'), out = resolve('.artifacts/account-avatar/browser');
mkdirSync(out, { recursive: true });
writeFileSync(join(out, 'server.log'), '');
const children = [], checks = [], errors = [];
const mark = text => { checks.push(text); console.log(text); };
async function freePort() { const s = createServer(); await new Promise(r => s.listen(0, '127.0.0.1', r)); const port = s.address().port; await new Promise(r => s.close(r)); return port; }
const uiPort = await freePort(), apiPort = await freePort();
const origin = `http://127.0.0.1:${uiPort}`, apiOrigin = `http://127.0.0.1:${apiPort}`;
function start(cmd, args, cwd, env = {}) { const c = spawn(cmd, args, { cwd, detached: true, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] }); children.push(c); for (const s of [c.stdout, c.stderr]) s.on('data', d => appendFileSync(join(out, 'server.log'), d)); return c; }
async function waitURL(url) { for (let i = 0; i < 120; i++) { try { if ((await fetch(url, { signal: AbortSignal.timeout(1000) })).ok) return; } catch { /* Starting. */ } if (children.some(c => c.exitCode !== null)) throw Error('Avatar lab exited; inspect server.log'); await new Promise(r => setTimeout(r, 250)); } throw Error('Avatar lab startup timeout'); }
async function stop(c) { if (c.exitCode !== null) return; const done = new Promise(r => c.once('exit', r)); try { process.kill(-c.pid, 'SIGTERM'); } catch { return; } await Promise.race([done, new Promise(r => { const t = setTimeout(r, 5000); t.unref(); })]); }
async function signIn(page, username = notificationLogin.username) { await page.goto(origin + '/login'); await page.getByLabel('用户名', { exact: true }).fill(username); await page.getByLabel('密码', { exact: true }).fill(notificationLogin.password); await page.getByRole('button', { name: '登录', exact: true }).click(); await page.waitForURL(u => u.pathname !== '/login'); }
let browser, api;
try {
  ensureTestDependencies();
  execFileSync('docker', ['compose', '-f', 'deploy/monitoring-contract/compose.yaml', 'up', '-d', '--wait', 's3'], { stdio: 'inherit' });
  if (process.env.INFRAPRO_AVATAR_SKIP_BUILD !== '1') execFileSync('pnpm', ['--filter', '@infrapro/console', 'build'], { stdio: 'inherit' });
  const binary = join(out, 'api-test');
  execFileSync('go', ['test', '-c', '-o', binary, './services/api/internal/transport/http'], { stdio: 'inherit' });
  start(binary, ['-test.run', '^TestE2EServer$', '-test.v', '-test.timeout', '0'], join(root, 'services/api/internal/transport/http'), { ...testEnv, INFRAPRO_E2E_SERVER: '1', INFRAPRO_AVATAR_E2E: '1', INFRAPRO_HTTP_ADDR: `127.0.0.1:${apiPort}`, INFRAPRO_E2E_CONSOLE_URL: origin, INFRAPRO_E2E_ORIGINS: origin });
  start(process.execPath, [join(root, 'apps/console/node_modules/vite/bin/vite.js'), 'preview'], join(root, 'apps/console'), { API_PROXY_TARGET: apiOrigin, CONSOLE_PORT: String(uiPort), VITE_API_BASE_URL: '/api/v1' });
  await waitURL(apiOrigin + '/readyz'); await waitURL(origin);
  api = await notificationClient(origin);
  const owner = (await api.call('/auth/me')).account;
  const tenant = (await api.call('/platform/tenants', { name: '头像验收租户', code: 'avatar-browser', administrator_account_id: owner.id, client_request_id: randomUUID(), reason: '头像合成验收' })).tenant_id;
  const tm = (await api.call(`/tenants/${tenant}/members`)).items[0].id;
  const project = (await api.call(`/tenants/${tenant}/projects`, { name: '头像验收项目', code: 'avatar-browser', purpose: '头像合成验收', owner_tenant_membership_id: tm, administrator_tenant_membership_id: tm, client_request_id: randomUUID(), reason: '头像合成验收' })).project_id;
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
  const page = await context.newPage(); page.on('pageerror', e => errors.push(e.message));
  await signIn(page); await page.goto(origin + '/profile');
  const menu = page.getByRole('button', { name: '账号菜单', exact: true });
  await expect(menu.locator('[data-slot="avatar"]')).toHaveCount(1);
  await expect(menu.locator('img')).toHaveCount(0);
  await menu.focus(); await page.keyboard.press('Enter'); await expect(page.getByRole('menu')).toBeVisible();
  await expect(page.getByRole('menu').locator('[data-slot="avatar"]')).toHaveCount(1);
  await page.keyboard.press('ArrowDown'); await page.keyboard.press('Escape'); await expect(menu).toBeFocused();
  mark('Default avatar and shadcn Avatar/DropdownMenu keyboard navigation');
  const input = page.getByLabel('选择头像图片', { exact: true });
  await input.setInputFiles({ name: 'invalid.png', mimeType: 'image/png', buffer: Buffer.from('invalid') });
  await expect(page.getByText('图片无法读取，请选择有效的 JPG 或 PNG 图片。')).toBeVisible();
  await input.setInputFiles({ name: 'large.png', mimeType: 'image/png', buffer: Buffer.alloc(5 * 1024 * 1024 + 1) });
  await expect(page.getByText('请选择 JPG 或 PNG 图片，大小不超过 5 MB。')).toBeVisible();
  const fixture = async (width, height, type = 'image/png') => Buffer.from(await page.evaluate(({ width, height, type }) => { const c = document.createElement('canvas'); c.width = width; c.height = height; const g = c.getContext('2d'); g.fillStyle = '#0f766e'; g.fillRect(0, 0, width, height); g.fillStyle = '#fbbf24'; g.fillRect(width / 3, height / 4, width / 3, height / 2); g.fillStyle = '#2563eb'; g.fillRect(width / 2, 0, width / 3, height / 3); return c.toDataURL(type).split(',')[1]; }, { width, height, type }), 'base64');
  const landscape = await fixture(960, 540), portrait = await fixture(480, 800, 'image/jpeg');
  const choose = async (buffer = landscape, mimeType = 'image/png') => { await input.setInputFiles({ name: mimeType === 'image/png' ? 'landscape.png' : 'portrait.jpg', mimeType, buffer }); await expect(page.getByRole('dialog', { name: '调整头像' })).toBeVisible(); await expect.poll(() => page.locator('canvas').evaluate(c => c.getContext('2d').getImageData(128, 128, 1, 1).data[3])).toBe(255); };
  await choose();
  const crop = page.getByRole('dialog', { name: '调整头像' });
  await crop.getByRole('button', { name: '取消', exact: true }).click(); await expect(page.getByRole('button', { name: '更换头像', exact: true })).toBeFocused(); await expect(menu.locator('img')).toHaveCount(0);
  await choose();
  await crop.getByLabel('缩放', { exact: true }).focus(); await page.keyboard.press('ArrowRight');
  await crop.getByRole('img').focus(); await page.keyboard.press('ArrowRight'); await page.keyboard.press('ArrowDown');
  const beforeDrag = await crop.getByLabel('水平位置', { exact: true }).inputValue();
  const bounds = await crop.locator('canvas').boundingBox();
  await page.mouse.move(bounds.x + 100, bounds.y + 100); await page.mouse.down(); await page.mouse.move(bounds.x + 125, bounds.y + 100); await page.mouse.up();
  expect(await crop.getByLabel('水平位置', { exact: true }).inputValue()).not.toBe(beforeDrag);
  await page.screenshot({ path: join(out, 'crop-desktop.png') });
  const pixelHash = async (mode) => page.evaluate(async mode => { let canvas; if (mode === 'preview') canvas = document.querySelector('canvas'); else { const r = await fetch('/api/v1/account/avatar'); const img = await createImageBitmap(await r.blob()); canvas = document.createElement('canvas'); canvas.width = canvas.height = 256; canvas.getContext('2d').drawImage(img, 0, 0); img.close(); } const bytes = canvas.getContext('2d').getImageData(0, 0, 256, 256).data; return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes))).join(','); }, mode);
  const expected = await pixelHash('preview');
  await crop.getByRole('button', { name: '保存头像' }).click(); await expect(crop).toHaveCount(0);
  await expect(menu.locator('img')).toBeVisible(); expect(await pixelHash('stored')).toBe(expected);
  await page.reload(); await expect(menu.locator('img')).toBeVisible();
  await page.screenshot({ path: join(out, 'profile-desktop.png'), fullPage: false });
  await menu.click(); await page.screenshot({ path: join(out, 'avatar-dropdown.png') }); await page.keyboard.press('Escape');
  mark('PNG crop/keyboard adjustment matches stored pixels; cancel/invalid/oversize preserve state; reload persists');
  // A second session changes the avatar while the first has a crop draft.
  await choose(portrait, 'image/jpeg');
  const upload = await api.client.put(api.api + '/account/avatar?expected_revision=1', { headers: { ...api.headers, 'Content-Type': 'application/octet-stream' }, data: portrait });
  expect(upload.status()).toBe(200);
  await crop.getByRole('button', { name: '保存头像' }).click(); await expect(crop.getByText('头像已在其他页面更新，请刷新后重新选择。')).toBeVisible();
  await crop.getByRole('button', { name: '取消', exact: true }).click(); await page.reload(); await expect(menu.locator('img')).toBeVisible();
  expect((await api.call('/auth/me')).account.avatar_revision).toBe('2');
  mark('Concurrent session conflict keeps the new JPEG avatar');
  await page.route('**/api/v1/account/avatar?expected_revision=*', async route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '合成存储故障' }) }));
  await choose(); await crop.getByRole('button', { name: '保存头像' }).click(); await expect(crop.getByText('合成存储故障')).toBeVisible(); await crop.getByRole('button', { name: '取消', exact: true }).click();
  await page.unroute('**/api/v1/account/avatar?expected_revision=*'); expect((await api.call('/auth/me')).account.avatar_revision).toBe('2');
  mark('Failed save leaves the current avatar intact');
  for (const path of ['/access/accounts', `/t/${tenant}/members`, `/t/${tenant}/p/${project}/members`]) {
    await page.goto(origin + path); await expect(page.getByRole('main').locator('[data-slot="avatar-image"]').first()).toBeVisible();
    await page.getByRole('main').getByRole('button', { name: owner.display_name, exact: true }).click();
    await expect(page.getByRole('dialog').locator('[data-slot="avatar-image"]').first()).toBeVisible();
  }
  mark('Platform account, tenant member and project member lists/details show the same avatar');
  await page.goto(origin + '/profile'); await page.setViewportSize({ width: 375, height: 812 });
  await choose(portrait, 'image/jpeg');
  await page.screenshot({ path: join(out, 'crop-mobile-light.png'), fullPage: false });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await page.screenshot({ path: join(out, 'crop-mobile-dark.png'), fullPage: false });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await crop.getByRole('button', { name: '保存头像' }).click(); await expect(crop).toHaveCount(0); await expect(menu.locator('img')).toBeVisible();
  await page.setViewportSize({ width: 812, height: 375 }); await choose();
  await crop.getByRole('button', { name: '取消', exact: true }).click();
  await page.setViewportSize({ width: 1440, height: 1000 });
  const exif = Buffer.from('45786966000049492a0008000000010012010300010000000600000000000000', 'hex');
  const exifHeader = Buffer.alloc(4); exifHeader.writeUInt16BE(0xffe1); exifHeader.writeUInt16BE(exif.length + 2, 2);
  const oriented = Buffer.concat([portrait.subarray(0, 2), exifHeader, exif, portrait.subarray(2)]);
  await choose(oriented, 'image/jpeg'); await crop.getByRole('button', { name: '取消', exact: true }).click();
  mark('Portrait/EXIF JPEG, dragging, 375px light/dark and landscape dialogs remain usable');
  const invitation = await api.call('/invitations', { note: '头像账号切换验收' });
  const token = new URLSearchParams(new URL(invitation.url).hash.slice(1)).get('token');
  const guest = await request.newContext();
  const csrf = await (await guest.get(api.api + '/auth/csrf')).json();
  const accepted = await guest.post(api.api + '/public/invitations/accept', { headers: { Origin: origin, 'X-CSRF-Token': csrf.token }, data: { token, username: 'avatar_other', display_name: '另一账号', password: notificationLogin.password } });
  expect(accepted.status()).toBe(204); await guest.dispose();
  await menu.click(); await page.getByRole('menuitem', { name: '退出登录' }).click(); await page.waitForURL(u => u.pathname === '/login');
  await signIn(page, 'avatar_other'); await page.goto(origin + '/profile'); await expect(menu.locator('img')).toHaveCount(0); await expect(menu).toContainText('另一');
  await menu.click(); await page.getByRole('menuitem', { name: '退出登录' }).click(); await page.waitForURL(u => u.pathname === '/login');
  await signIn(page); await page.goto(origin + '/profile'); await expect(menu.locator('img')).toBeVisible();
  mark('Switching accounts clears the previous user image and restores the correct owner image');
  await page.route('**/api/v1/account/avatar?version=*', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: '合成图片读取故障' }) }));
  await page.reload(); await expect(menu.locator('img')).toHaveCount(0); await expect(menu.locator('[data-slot="avatar-fallback"]')).toBeVisible();
  await page.unroute('**/api/v1/account/avatar?version=*'); await page.reload(); await expect(menu.locator('img')).toBeVisible();
  await page.getByRole('button', { name: '移除头像', exact: true }).click();
  await page.getByRole('button', { name: '确认移除', exact: true }).click(); await expect(menu.locator('img')).toHaveCount(0);
  await expect.poll(async () => (await api.call('/auth/me')).account.has_avatar).toBe(false);
  await expect(page.getByRole('dialog', { name: '移除头像', exact: true })).toHaveCount(0);
  for (const path of [`/t/${tenant}/members`, `/t/${tenant}/p/${project}/members`]) { await page.goto(origin + path); await expect(page.getByRole('main').locator('[data-slot="avatar-image"]')).toHaveCount(0); }
  await menu.click(); await page.getByRole('menuitem', { name: '退出登录' }).click(); await page.waitForURL(u => u.pathname === '/login');
  await signIn(page); await page.goto(origin + '/profile'); await expect(menu.locator('img')).toHaveCount(0);
  await page.getByLabel('显示名称', { exact: true }).fill('头像验收账号'); await page.getByRole('button', { name: '保存资料', exact: true }).click(); await expect(menu).toContainText('头像');
  mark('Read failures fall back; removal propagates; logout/login and display-name editing work');
  expect(errors).toEqual([]);
  writeFileSync(join(out, 'result.json'), JSON.stringify({ status: 'passed', checks, pageErrors: errors, origin, synthetic: true }, null, 2));
} finally {
  await browser?.close(); await api?.client.dispose();
  for (const c of children.reverse()) await stop(c);
}

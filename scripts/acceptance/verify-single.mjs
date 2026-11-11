import assert from 'node:assert/strict';
import { chromium, expect, request } from '@playwright/test';
import { mkdirSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { directory, environment, origin, readPrivate, savePrivate } from './environment.mjs';

environment();
const account = readPrivate('single-account.local.json');
const f = readPrivate('manifest.json');
const api = `${origin}/api/v1`;
const results = [];
const ctx = await request.newContext();
const record = (name, value) => { assert.ok(value, name); results.push({ name, passed: true }); };
const csrf = await ctx.get(`${api}/auth/csrf`);
assert.equal(csrf.status(), 200);
const headers = { Origin: origin, 'X-CSRF-Token': (await csrf.json()).token };
let browser;
async function read(path, status = 200) {
  const response = await ctx.get(api + path);
  assert.equal(response.status(), status, path);
  results.push({ name: `API:${path}`, status, passed: true });
  return response.json();
}
try {
  const login = await ctx.post(`${api}/auth/login`, { headers, data: { username: account.username, password: account.password } });
  record('single account signs in', login.status() === 200);
  for (const path of ['/access/accounts', '/access/roles', '/access/permissions', '/access/audit-events', '/invitations', '/account/profile', '/account/sessions', '/account/security-events', '/platform/tenants']) await read(path);
  for (const kind of ['type-definitions', 'units', 'metrics', 'monitoring-templates']) await read(`/platform/engineering/${kind}`);
  for (const [key, t] of Object.entries(f.tenants)) {
    const tenant = await read(`/platform/tenants/${t.id}`);
    record(`tenant ${key} assigned to single account`, tenant.administrator.id === account.id);
    if (tenant.status === 'disabled') {
      await read(`/account/tenants/${t.id}/availability`);
      await read(`/tenants/${t.id}/profile`, 403);
      continue;
    }
    for (const path of ['profile', 'members', 'member-invitations', 'roles', 'audit-events']) await read(`/tenants/${t.id}/${path}`);
  }
  for (const p of Object.values(f.projects)) {
    const base = `/tenants/${f.tenants.A.id}/projects/${p.id}`;
    const profile = await read(`${base}/profile`);
    record(`project ${p.id} assigned to single account`, profile.administrator.account_id === account.id);
    for (const path of ['members', 'roles', 'permissions', 'audit']) await read(`${base}/${path}`);
  }
  for (const [tenant, definition] of [['A', 'privateA'], ['B', 'privateB']]) {
    await read(`/tenants/${f.tenants[tenant].id}/engineering/monitoring-templates/${f.definitions[definition].definition_id}`);
    await read(`/tenants/${f.tenants[tenant].id}/engineering/resources/facilities`);
  }
  const base = `/tenants/${f.tenants.A.id}/engineering/resources/facilities/${f.resources.bridge.id}`;
  await read(base);
  await read(`${base}/components`);
  await read(`${base}/points/${f.resources.point.id}`);
  await read(`${base}/points/${f.resources.point.id}/channels`);
  await read(`${base}/changes`);
  const accountList = readPrivate('accounts.local.json');
  const guide = readFileSync(resolve(directory, '账号与验收入口.local.md'), 'utf8');
  record('guide contains only the single walkthrough password', guide.includes(account.password) && Object.values(accountList).filter((a) => a.username !== account.username).every((a) => !guide.includes(a.password)));

  if (!process.argv.includes('--api-only')) {
    browser = await chromium.launch();
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(`${origin}/login`);
    await page.getByRole('textbox', { name: '用户名', exact: true }).fill(account.username);
    await page.getByLabel('密码', { exact: true }).fill(account.password);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
    async function switchWorkspace(name, expectedPath) {
      await page.getByRole('button', { name: /^切换工作空间：/ }).click();
      await page.getByRole('menuitem', { name: new RegExp(name) }).click();
      await expect(page).toHaveURL(origin + expectedPath);
      record(`workspace switch:${name}`, true);
    }
    await switchWorkspace('平台工作空间', '/platform/tenants');
    await switchWorkspace(f.tenants.A.name, `/t/${f.tenants.A.id}/profile`);
    await switchWorkspace(f.projects.bridge.name, `/t/${f.tenants.A.id}/p/${f.projects.bridge.id}/profile`);
    await switchWorkspace(f.tenants.B.name, `/t/${f.tenants.B.id}/profile`);
    const A = f.tenants.A.id;
    const routes = [
      ['/access/accounts', '账号管理'], ['/access/roles', '角色管理'], ['/invitations', '账号开户邀请'], ['/access/audit', '平台审计'],
      [f.urls.definitions, '验收·梁式桥梁'], ['/platform/engineering/metrics', '验收·结构位移'], ['/platform/engineering/units', '毫米'], [f.urls.publicTemplate, '验收·桥梁位移与巡查模板'],
      [`/t/${A}/members`, 'accept_reader'], [`/t/${A}/roles`, '验收·工程资源维护'], [`/t/${A}/invitations`, 'accept_invitee'], [`/t/${A}/projects`, f.projects.bridge.name],
      [f.urls.privateTemplate, '验收·华东桥梁私有监测模板'], [f.urls.privateTemplateB, '验收·西部私有巡查模板'],
      [f.urls.otherResources, '验收·西部租户隔离边坡'], [f.urls.archive, '恢复项目'],
      [`/t/${A}/p/${f.projects.bridge.id}/members`, 'accept_reader'], [`/t/${A}/p/${f.projects.bridge.id}/roles`, '验收·项目资料维护'],
      [f.urls.tenantC, '恢复租户'], ['/profile', '个人资料'], ['/security', '账号安全'], [f.urls.bridge, '第一跨跨中位移测点'],
    ];
    for (const [path, expectedText] of routes) {
      await page.goto(path.startsWith('http') ? path : origin + path);
      await expect(page.getByRole('main')).toContainText(expectedText, { timeout: 15_000 });
      record(`UI:${path}`, true);
    }
    await expect(page.getByRole('button', { name: '添加测点', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '添加通道', exact: true })).toBeVisible();
    await expect(page.getByText('读取构件...', { exact: true })).toHaveCount(0);
    mkdirSync(resolve(directory, 'screenshots'), { recursive: true, mode: 0o700 });
    await page.screenshot({ path: resolve(directory, 'screenshots/single-account-resource.png'), fullPage: true });
    await page.getByRole('button', { name: /^切换工作空间：/ }).click();
    await expect(page.getByRole('menuitem', { name: /平台工作空间/ })).toBeVisible();
    await expect(page.getByRole('menuitem', { name: new RegExp(f.projects.practice.name) })).toBeVisible();
    await page.screenshot({ path: resolve(directory, 'screenshots/single-account-workspaces.png'), fullPage: true });
    record('one browser login reaches all workspaces without runtime errors', errors.length === 0);
    const logoutToken = (await (await context.request.get(`${api}/auth/csrf`)).json()).token;
    await context.request.post(`${api}/auth/logout`, { headers: { Origin: origin, 'X-CSRF-Token': logoutToken } });
    await context.close();
  }
  savePrivate(process.argv.includes('--api-only') ? 'verification-single-api.json' : 'verification-single.json', { checked_at: new Date().toISOString(), username: account.username, passed: true, count: results.length, results });
  console.log(`Single-account acceptance verified: ${results.length} checks passed.`);
} finally {
  if (browser) await browser.close();
  await ctx.post(`${api}/auth/logout`, { headers });
  await ctx.dispose();
}

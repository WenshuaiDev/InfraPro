import assert from 'node:assert/strict';
import { chromium, expect, request } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { directory, environment, origin, readPrivate, savePrivate } from './environment.mjs';

environment();
const accounts = readPrivate('accounts.local.json');
const f = readPrivate('manifest.json');
const api = `${origin}/api/v1`;
const results = [];
const sessions = {};
const check = (name, value) => { assert.ok(value, name); results.push({ name, passed: true }); };
async function session(key) {
  if (sessions[key]) return sessions[key];
  const ctx = await request.newContext();
  const csrf = await ctx.get(`${api}/auth/csrf`);
  assert.equal(csrf.status(), 200);
  const headers = { Origin: origin, 'X-CSRF-Token': (await csrf.json()).token };
  const response = await ctx.post(`${api}/auth/login`, { headers, data: { username: accounts[key].username, password: accounts[key].password } });
  assert.equal(response.status(), key === 'disabled' ? 401 : 200, `login ${key}`);
  sessions[key] = { ctx, headers };
  results.push({ name: `login:${key}`, passed: true, status: response.status() });
  return sessions[key];
}
async function read(key, path, status = 200, method = 'get', data) {
  const s = await session(key);
  const response = await s.ctx.fetch(api + path, { method: method.toUpperCase(), headers: s.headers, ...(data === undefined ? {} : { data }) });
  assert.equal(response.status(), status, `${key} ${path}`);
  results.push({ name: `${key}:${method}:${path}`, status, passed: true });
  return response.status() === 204 ? null : response.json();
}
let browser;
try {
  for (const key of Object.keys(accounts)) await session(key);
  const A = f.tenants.A.id;
  const B = f.tenants.B.id;
  const resource = `/tenants/${A}/engineering/resources/facilities/${f.resources.bridge.id}`;
  const project = `/tenants/${A}/projects/${f.projects.bridge.id}`;
  const privateTemplate = `/tenants/${A}/engineering/monitoring-templates/${f.definitions.privateA.definition_id}`;
  for (const [key, status] of [['admin', 200], ['engineer', 200], ['reader', 200], ['member', 403], ['project', 403], ['noauth', 403], ['other', 404], ['platform', 404]]) await read(key, resource, status);
  await read('reader', `/tenants/${A}/engineering/catalog/type-definitions`, 403);
  for (const [key, status] of [['admin', 200], ['template', 200], ['reader', 403], ['platform', 404], ['other', 404]]) await read(key, privateTemplate, status);
  for (const [key, status] of [['project', 200], ['project_editor', 200], ['reader', 200], ['admin', 404], ['platform', 404]]) await read(key, `${project}/profile`, status);
  await read('engineer', `/tenants/${B}/profile`);
  await read('engineer', `/tenants/${B}/engineering/resources/facilities`, 403);
  await read('suspended', `/tenants/${A}/profile`, 404);
  await read('removed', `/tenants/${A}/profile`, 404);
  await read('noauth', `/tenants/${A}/profile`, 403);
  await read('admin', `/tenants/${f.tenants.C.id}/profile`, 403);
  const availability = await read('admin', `/account/tenants/${f.tenants.C.id}/availability`);
  check('disabled tenant has visible availability', JSON.stringify(availability).includes('disabled'));
  const bridge = await read('engineer', resource);
  check('existing facility stays on V1', bridge.type_version_id === f.definitions.bridge.version_id);
  check('bridge boolean and multi-enum data', bridge.values.in_service === true && bridge.values.methods.length === 2);
  const empty = await read('engineer', `/tenants/${A}/engineering/resources/facilities/${f.resources.empty.id}`);
  check('required zero and false remain real values', empty.values.length === 0 && empty.values.in_service === false);
  const zero = await read('reader', `${resource}/points/${f.resources.zeroPoint.id}`);
  check('zero coordinates remain real values', zero.position.geographic.longitude === 0 && zero.position.geographic.latitude === 0 && zero.position.elevation.value_m === 0);
  const point = await read('reader', `${resource}/points/${f.resources.emptyPoint.id}`);
  check('empty point has no own coordinates or references', !point.position?.geographic && !point.position?.projected && !point.position?.local && !point.position?.elevation && !point.position?.linear_references?.length && !point.position?.sections?.length);
  const channels = await read('reader', `${resource}/points/${f.resources.point.id}/channels`);
  check('same metric two channels and enum channel', channels.items.length === 3 && channels.items.filter((c) => c.metric_version_id === f.definitions.displacement.version_id).length === 2);
  const history = await read('reader', `${resource}/changes`);
  check('resource name change history exists', JSON.stringify(history).includes('第一跨跨中位移测点'));
  const retired = await read('engineer', `/tenants/${A}/engineering/resources/facilities/${f.resources.retired.id}`);
  check('retired facility read-only', retired.status === 'retired' && retired.actions.edit === false);
  const retiredChild = await read('engineer', `/tenants/${A}/engineering/resources/facilities/${f.resources.retired.id}/points/${f.resources.ancestorRetiredPoint.id}`);
  check('ancestor retirement does not rewrite child status', retiredChild.status === 'normal' && retiredChild.actions.edit === false);
  await read('engineer', `/tenants/${A}/engineering/resources/facilities/${f.resources.deleted.id}`, 404);
  await read('engineer', `/tenants/${A}/engineering/resources/facilities/${f.resources.deleted.id}/changes`);
  const definition = await read('platform', `/platform/engineering/type-definitions/${f.definitions.bridge.definition_id}`);
  check('two immutable type versions', definition.versions.length === 2);
  const disabled = await read('platform', `/platform/engineering/type-definitions/${f.definitions.disabledType.definition_id}`);
  check('disabled type sample', disabled.versions[0].enabled === false);
  const archived = await read('project', `/tenants/${A}/projects/${f.projects.archived.id}/profile`);
  check('archived project sample', archived.status === 'archived');
  const invites = await read('admin', `/tenants/${A}/member-invitations?status=pending`);
  check('user invitation remains unconsumed', invites.items.some((i) => i.target.username === accounts.invitee.username));
  await read('editor', `/platform/engineering/type-definitions/${f.definitions.draft.definition_id}/publish`, 403, 'post', { request_id: crypto.randomUUID(), reason: '只读验证编制员禁止发布', revision: f.definitions.draft.revision });
  await read('template', `/tenants/${A}/engineering/monitoring-templates/${f.definitions.privateDraft.definition_id}/publish`, 403, 'post', { request_id: crypto.randomUUID(), reason: '只读验证编制员禁止发布', revision: f.definitions.privateDraft.revision });
  await read('audit', '/access/audit-events');
  await read('personal', '/account/profile');
  await read('personal', '/account/sessions');
  await read('personal', '/account/security-events');
  console.log(`API fixture and authorization checks passed (${results.length} checks).`);

  if (!process.argv.includes('--api-only')) {
    mkdirSync(resolve(directory, 'screenshots'), { recursive: true, mode: 0o700 });
    browser = await chromium.launch();
    async function visit(key, routes, width = 1440) {
      const context = await browser.newContext({ viewport: { width, height: width === 375 ? 812 : 1000 }, reducedMotion: 'reduce' });
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', (error) => errors.push(error.message));
      try {
        await page.goto(`${origin}/login`);
        await page.getByRole('textbox', { name: '用户名', exact: true }).fill(accounts[key].username);
        await page.getByLabel('密码', { exact: true }).fill(accounts[key].password);
        await page.getByRole('button', { name: '登录', exact: true }).click();
        await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
        for (const [path, text] of routes) {
          await page.goto(path.startsWith('http') ? path : origin + path);
          await expect(page.getByRole('main')).toBeVisible();
          await expect(page.getByRole('main')).toContainText(text, { timeout: 15_000 });
          await expect(page.getByText('页面出现了问题', { exact: false })).toHaveCount(0);
          check(`UI:${key}:${width}:${path}`, true);
        }
        if (['engineer', 'reader'].includes(key)) {
          await expect(page.getByRole('heading', { name: '测点：第一跨跨中位移测点', exact: true })).toBeVisible();
          await expect(page.getByRole('button', { name: '添加测点', exact: true })).toHaveCount(key === 'reader' ? 0 : 1);
          for (const theme of ['light', 'dark']) {
            await page.evaluate((t) => document.documentElement.classList.toggle('dark', t === 'dark'), theme);
            check(`UI:${key}:${width}:${theme}:no-horizontal-page-overflow`, await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
            await page.screenshot({ path: resolve(directory, `screenshots/${key}-${width}-${theme}.png`), fullPage: true });
          }
        } else await page.screenshot({ path: resolve(directory, `screenshots/${key}-${width}.png`), fullPage: true });
        check(`UI:${key}:${width}:no-runtime-errors`, errors.length === 0);
      } finally {
        const csrf = await context.request.get(`${api}/auth/csrf`);
        if (csrf.ok()) await context.request.post(`${api}/auth/logout`, { headers: { Origin: origin, 'X-CSRF-Token': (await csrf.json()).token } });
        await context.close();
      }
    }
    await visit('platform', [
      ['/access/accounts', 'accept_admin'], ['/access/roles', '验收·平台审计只读'], ['/platform/tenants', f.tenants.A.name],
      ['/platform/engineering/type-definitions', '验收·梁式桥梁'], ['/platform/engineering/metrics', '验收·结构位移'],
      ['/platform/engineering/units', '毫米'], [f.urls.publicTemplate, '验收·桥梁位移与巡查模板'], ['/access/audit', '平台审计'],
    ]);
    await visit('admin', [[f.urls.tenantA, f.tenants.A.name], [`/t/${A}/members`, 'accept_reader'], [`/t/${A}/invitations`, 'accept_invitee'], [`/t/${A}/roles`, '验收·工程资源维护'], [`/t/${A}/projects`, f.projects.bridge.name], [f.urls.privateTemplate, '验收·华东桥梁私有监测模板']]);
    await visit('project', [[f.urls.project, f.projects.bridge.name], [`/t/${A}/p/${f.projects.bridge.id}/members`, 'accept_reader'], [`/t/${A}/p/${f.projects.bridge.id}/roles`, '验收·项目资料维护'], [f.urls.archive, '恢复项目']]);
    await visit('engineer', [[f.urls.bridge, '第一跨跨中位移测点']]);
    await visit('engineer', [[f.urls.bridge, '第一跨跨中位移测点']], 375);
    await visit('reader', [[f.urls.bridge, '第一跨跨中位移测点']]);
    await visit('editor', [[`${f.urls.definitions}?definition=${f.definitions.draft.definition_id}`, '验收·涵洞（待发布草稿）']]);
    await visit('other', [[f.urls.otherResources, '验收·西部租户隔离边坡']]);
    await visit('noauth', [[`/t/${A}/profile`, '当前未获业务授权']]);
    await visit('personal', [['/profile', '个人资料'], ['/security', '账号安全']]);
  }
  savePrivate(process.argv.includes('--api-only') ? 'verification-api.json' : 'verification.json', { checked_at: new Date().toISOString(), count: results.length, passed: true, results });
  console.log(`Acceptance verification passed: ${results.length} checks.`);
} finally {
  if (browser) await browser.close();
  for (const [key, s] of Object.entries(sessions)) {
    if (key !== 'disabled') await s.ctx.post(`${api}/auth/logout`, { headers: s.headers });
    await s.ctx.dispose();
  }
}

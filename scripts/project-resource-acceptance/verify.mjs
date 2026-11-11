// Read-only smoke checks for the prepared manual acceptance data.
import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { directory, origin, readPrivate, savePrivate } from './environment.mjs';

const accounts = readPrivate('accounts.local.json');
const fixture = readPrivate('fixture.local.json');
const ids = fixture.data;
const api = origin + '/api/v1';
const browser = await chromium.launch();
const contexts = [];
const evidence = [];
mkdirSync(resolve(directory, 'screenshots'), { recursive: true });
async function session(key) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
  contexts.push(context);
  const csrf = await (await context.request.get(api + '/auth/csrf')).json();
  const response = await context.request.post(api + '/auth/login', { headers: { Origin: origin, 'X-CSRF-Token': csrf.token }, data: { username: accounts[key].username, password: accounts[key].password } });
  assert.equal(response.status(), 200, key + ' login');
  return context;
}
async function get(context, path, status = 200) {
  const response = await context.request.get(api + path);
  assert.ok((Array.isArray(status) ? status : [status]).includes(response.status()), `${path}: expected ${status}, received ${response.status()}`);
  return status === 200 ? response.json() : undefined;
}
try {
  const sessions = {};
  for (const key of ['tenant', 'maintainer', 'reader', 'platform', 'project_admin', 'other']) sessions[key] = await session(key);
  const tenantPath = `/tenants/${ids.tenant}/engineering/resources/facilities`;
  const projectPath = (id) => `/tenants/${ids.tenant}/projects/${id}/engineering/resources/facilities`;
  for (const [key, project] of [['maintainer', ids.A], ['reader', ids.B]]) {
    const data = await get(sessions[key], projectPath(project));
    assert.deepEqual(data.items.map((v) => v.id), [ids.F109]);
    await get(sessions[key], tenantPath, 403);
    const channels = await get(sessions[key], `${projectPath(project)}/${ids.F109}/points/${ids.P1}/channels`);
    assert.equal(channels.items.length, key === 'maintainer' ? 2 : 1);
    const point = await get(sessions[key], `${projectPath(project)}/${ids.F109}/points/${ids.P1}`);
    assert.equal(point.actions.edit, key === 'maintainer');
    evidence.push(key + ': project scope, same IDs, exact channel range, no tenant fallback, correct actions');
  }
  const usage = await get(sessions.tenant, `${tenantPath}/${ids.F109}/project-usage`);
  assert.equal(usage.items.length, 2);
  const governance = `/tenants/${ids.tenant}/projects/${ids.A}/resource-access/facilities/${ids.F109}`;
  for (const [owner, count] of [['root', 0], [fixture.actions.beam.id, 2]]) {
    const candidates = await get(sessions.tenant, governance + '?candidate_kind=channel&candidate_owner=' + owner);
    assert.equal(candidates.candidates.items.length, count, 'channel candidates follow component owner');
  }
  await get(sessions.platform, tenantPath, 404);
  await get(sessions.platform, projectPath(ids.A), [403, 404]);
  await get(sessions.other, tenantPath, 404);
  await get(sessions.project_admin, tenantPath, 403);
  evidence.push('Tenant usage contains A/B; platform, other tenant and project administrator cannot read the tenant resource directory');
  for (const [key, url, title, mobile] of [
    ['tenant', `/t/${ids.tenant}/resource-access/${ids.A}/facilities/${ids.F109}`, '共享桥梁 F109 · 项目资源授权', false],
    ['maintainer', `/t/${ids.tenant}/p/${ids.A}/resources/facilities/${ids.F109}?point=${ids.P1}`, '测点：长期观测测点 P1', false],
    ['reader', `/t/${ids.tenant}/p/${ids.B}/resources/facilities/${ids.F109}?point=${ids.P1}`, '测点：长期观测测点 P1', true],
  ]) {
    const page = await sessions[key].newPage();
    if (mobile) await page.setViewportSize({ width: 375, height: 812 });
    await page.goto(origin + url);
    await page.getByRole('heading', { name: title, exact: true }).waitFor();
    if (key === 'tenant') {
      await page.getByRole('button', { name: '展开 第一跨', exact: true }).click();
      await page.getByRole('button', { name: '第一梁 BEAM1', exact: true }).click();
      await page.getByLabel('对象种类', { exact: true }).selectOption('channel');
      await page.getByRole('heading', { name: '选择对象', exact: true }).locator('..').getByText('通道 · 长期位移 C1', { exact: true }).waitFor();
    }
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, key + ' overflow');
    await page.screenshot({ path: resolve(directory, 'screenshots', key + '.png'), fullPage: true });
    evidence.push(key + ': real page rendered, no horizontal overflow');
  }
  savePrivate('verification.local.json', { checked_at: new Date().toISOString(), evidence, user_acceptance: 'pending' });
  console.log('Verified six independent identities, exact resource scopes and desktop/mobile pages. Manual acceptance remains pending.');
} finally {
  for (const context of contexts) await context.close();
  await browser.close();
}

import { readFileSync } from 'node:fs';
import { randomUUID, createHash } from 'node:crypto';
import { chromium, expect } from '@playwright/test';

export async function prepareMFALabDisplay({ administrator, tenantAdmin, session }) {
  const p1 = administrator.call, ta = tenantAdmin.call, tenant = session.tenants[0], project = session.project;
  const meta = revision => ({ expected_revision: revision, request_id: randomUUID(), reason: 'MFA 人员大屏验收' });
  const type = await p1('/platform/engineering/type-definitions', { request_id: randomUUID(), reason: 'MFA 合成设施', code: 'mfa_lab_type', content: { metamodel_version: '1', name: 'MFA 验收设施类型', description: '合成资料', release_notes: '', fields: [], components: [], relations: [] } });
  const published = await p1('/platform/engineering/type-definitions/' + type.definition_id + '/publish', { request_id: randomUUID(), reason: 'MFA 合成设施', revision: type.revision });
  const facility = await ta(`/tenants/${tenant}/engineering/resources/facilities`, { request_id: randomUUID(), code: 'MFA-LAB', name: 'MFA 验收设施', type_version_id: published.version_id, values: {} });
  const access = `/tenants/${tenant}/projects/${project}/resource-access/facilities/${facility.id}`, policy = await ta(access);
  const change = { request_id: randomUUID(), reason: 'MFA 人员大屏验收', action: 'create_link', expected_access_revision: policy.policy.revision, add: [{ kind: 'facility', capabilities: ['read'] }] };
  const preview = await ta(access + '/preview', change); await ta(access + '/changes', { ...change, preview_hash: preview.preview_hash });
  const base = `/tenants/${tenant}/projects/${project}/displays`;
  await ta(base + '/settings', { ...meta('0'), enabled: true }, 'PUT');
  const created = await ta(base + '/instances', { ...meta('0'), name: 'MFA 人员会话验收' }), path = base + '/instances/' + created.id;
  let revision = created.revision;
  async function command(suffix, data, method = 'POST') {
    const result = await ta(path + suffix, { ...meta(revision), ...data }, method);
    if (result.revision) revision = result.revision;
    return result;
  }
  const zip = readFileSync('.artifacts/display/example.zip'), sha = createHash('sha256').update(zip).digest('hex');
  const upload = await command('/uploads', { size: zip.length, sha256: sha });
  await tenantAdmin.upload(path + '/uploads/' + upload.upload_id + '/parts/0', zip, sha);
  await command('/uploads/' + upload.upload_id + '/complete', {});
  let detail;
  for (let attempt = 0; attempt < 120; attempt++) {
    detail = await ta(path);
    if (detail.packages.some(p => p.state === 'ready')) break;
    if (attempt === 119) throw Error('MFA lab display package did not become ready');
    await new Promise(resolve => setTimeout(resolve, 250));
  }
  revision = detail.instance.revision;
  const config = await command('/configs', { package_id: detail.packages.find(p => p.state === 'ready').id, parameters: { title: 'MFA 人员会话验收' }, bindings: { site: { kind: 'resources', ids: [facility.id] } }, mode: 'follow' });
  await command('/grant', { grant: { capabilities: ['resources.read'], scope: { resources: [facility.id] }, valid_until: new Date(Date.now() + 12 * 3600_000).toISOString() } }, 'PUT');
  const prepared = await ta(path + '/prepare', { ...meta(revision), config_id: config.config_id });
  await command('/publish', { config_id: config.config_id, preparation: prepared.hash });
  session.display_url = session.runtime_origin + '/display/' + created.id;
  session.display_management_url = session.origin + `/t/${tenant}/p/${project}/displays?instance=${created.id}`;
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ storageState: await tenantAdmin.http.storageState() }), page = await context.newPage();
    await page.goto(session.display_url);
    await page.getByRole('button', { name: '继续', exact: true }).click();
    await expect(page.locator('#status')).toHaveText('已连接 · 应用已就绪', { timeout: 30_000 });
    session.person_display_session = await page.evaluate(() => sessionStorage.getItem('display_session'));
    return browser;
  } catch (error) { await browser.close(); throw error; }
}

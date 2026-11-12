import { type Page } from '@playwright/test';
import { test, expect, consoleURL } from './fixtures';

const tid = '11111111-1111-4111-8111-111111111111';
const did = '22222222-2222-4222-8222-222222222222';
const uid = '33333333-3333-4333-8333-333333333333';
const devicePath = `/tenants/${tid}/devices/${did}`;

// Control outages and delayed replies while exercising the real routed forms,
// portals, identity provider and command hook. All writes stay in this mock.
async function recoveryFixture(page: Page, platform = false) {
  const state = { identityUnavailable: false, tenantUnavailable: false, previewRequired: true, delayPreview: false };
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  const previews: Record<string, unknown>[] = [];
  let releasePreview = () => {};
  const device = {
    id: did, tenant_id: tid, name: '回归设备', code: 'REGRESSION', status: 'enabled', description: '', serial_number: '',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', revision: '1', output_revision: '1', installation_revision: '0',
    product_id: did, product_version_id: did, product: { name: '测试产品', model: 'TEST', number: 1, manufacturer_name: '测试厂家', source: 'public' },
    actions: { edit: true, state: true, delete: true, install: true, configure: false, history: false, use_source: false, reason: '' },
    current_installation: null, deleted_at: null, visibility: 'full',
  };
  await page.route('**/api/v1/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    const unavailable = (path === '/auth/me' && state.identityUnavailable) || (path === `/tenants/${tid}/context` && state.tenantUnavailable);
    if (unavailable) return route.fulfill({ status: 503, json: { code: 'unavailable', message: '模拟临时故障' } });
    let data: unknown = { items: [], next_cursor: '' };
    if (path === '/auth/csrf') data = { token: 'regression-token' };
    else if (path === '/public/account-help') data = { support_contact: '测试', console_url: consoleURL, password_min: 12, password_max: 256, username_pattern: '.*' };
    else if (path === '/auth/me') data = { account: { id: uid, username: 'regression', display_name: '测试账号', status: 'active', created_at: '2026-01-01T00:00:00Z' }, permissions: platform ? ['device.definition.read', 'device.manufacturer.manage'] : [], authorization_revision: '1' };
    else if (path === '/auth/activity') return route.fulfill({ status: 204 });
    else if (path === '/me/workspaces') data = { items: [], authorization_revision: '1', summary: { mode: 'single', tenant_count: platform ? 0 : 1, project_count: 0, single: platform ? { kind: 'platform', id: 'platform', name: '平台工作空间', default_path: '/platform/device-products' } : { kind: 'tenant', id: tid, name: '测试租户', default_path: `/t/${tid}/devices` } } };
    else if (path === `/tenants/${tid}/context`) data = { tenant: { id: tid, name: '测试租户', code: 'TEST', status: 'active' }, membership_id: uid, roles: [], permissions: ['tenant.device.read', 'tenant.device.manage', 'tenant.device.state', 'tenant.device.install'], permission_sources: {}, authorization_revision: '1', authorization: {}, can_enter: true };
    else if (path.endsWith('/mutation-preview')) {
      previews.push(request.postDataJSON());
      if (state.delayPreview) await new Promise<void>(resolve => { releasePreview = resolve; });
      data = { required: state.previewRequired, can_submit: true, shared: false, preview_hash: 'regression-preview', blockers: [], changes: {}, visible_impacts: [], next_cursor: '' };
    } else if (request.method() !== 'GET') {
      writes.push({ path, body: request.postDataJSON() });
      data = { id: did, revision: '2', installation_revision: '1' };
    } else if (path === devicePath) data = device;
    await route.fulfill({ json: data });
  });
  await page.goto(`${consoleURL}${platform ? '/platform/device-products' : `/t/${tid}/devices/${did}`}`);
  return { state, writes, previews, release: () => releasePreview() };
}

async function checkConnection(page: Page) {
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
}

for (const platform of [false, true]) {
  test(`identity outage preserves and disables ${platform ? 'platform catalog' : 'tenant device'} portal draft`, async ({ page }) => {
    const f = await recoveryFixture(page, platform);
    await page.getByRole('button', { name: platform ? '新建厂家' : '编辑档案', exact: true }).click();
    const field = page.getByLabel(platform ? '厂家名称 *' : '设备名称 *', { exact: true });
    await field.fill('故障期间保留的输入');
    if (platform) await page.getByLabel('厂家编号 *', { exact: true }).fill('REGRESSION');
    const save = page.getByRole('button', { name: platform ? '创建厂家' : '保存资料', exact: true });
    f.state.identityUnavailable = true;
    await checkConnection(page);
    await expect(page.getByRole('button', { name: '重新连接账号服务', exact: true })).toBeVisible();
    await expect(field).toBeDisabled();
    await expect(save).toBeDisabled();
    await page.getByRole('dialog').locator('form').dispatchEvent('submit');
    expect(f.writes).toHaveLength(0);
    expect(f.previews).toHaveLength(0);
    f.state.identityUnavailable = false;
    await checkConnection(page);
    await expect(field).toBeEnabled();
    await expect(field).toHaveValue('故障期间保留的输入');
    f.state.previewRequired = false;
    await save.click();
    await expect.poll(() => f.writes.length).toBe(1);
  });
}

test('tenant outage blocks an already previewed confirmation and preserves its reason', async ({ page }) => {
  const f = await recoveryFixture(page);
  await page.getByRole('button', { name: '停用设备', exact: true }).click();
  await page.getByLabel('操作原因 *', { exact: true }).fill('保留的停用原因');
  await page.getByRole('button', { name: '预览变更', exact: true }).click();
  const confirm = page.getByRole('button', { name: '确认提交', exact: true });
  await expect(confirm).toBeEnabled();
  f.state.tenantUnavailable = true;
  await checkConnection(page);
  await expect(page.getByText('服务暂时不可用，输入已保留', { exact: true })).toBeVisible();
  await expect(confirm).toBeDisabled();
  expect(f.writes).toHaveLength(0);
  f.state.tenantUnavailable = false;
  await checkConnection(page);
  await expect(confirm).toBeEnabled();
  await expect(page.getByLabel('操作原因 *', { exact: true })).toHaveValue('保留的停用原因');
  await confirm.click();
  await expect.poll(() => f.writes.length).toBe(1);
});

test('late preview cannot auto-submit after tenant availability is lost', async ({ page }) => {
  const f = await recoveryFixture(page);
  f.state.delayPreview = true;
  f.state.previewRequired = false;
  await page.getByRole('button', { name: '编辑档案', exact: true }).click();
  await page.getByLabel('设备名称 *', { exact: true }).fill('迟到预览保留的输入');
  await page.getByRole('button', { name: '保存资料', exact: true }).click();
  await expect.poll(() => f.previews.length).toBe(1);
  f.state.tenantUnavailable = true;
  await checkConnection(page);
  await expect(page.getByText('服务暂时不可用，输入已保留', { exact: true })).toBeVisible();
  f.release();
  const confirm = page.getByRole('button', { name: '确认提交', exact: true });
  await expect(confirm).toBeVisible();
  await expect(confirm).toBeDisabled();
  expect(f.writes).toHaveLength(0);
  f.state.tenantUnavailable = false;
  await checkConnection(page);
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect.poll(() => f.writes.length).toBe(1);
  expect(f.writes[0]!.body.name).toBe('迟到预览保留的输入');
});

test('typing an installation offset preserves local time and sends one RFC3339 suffix', async ({ page }) => {
  const f = await recoveryFixture(page);
  await page.getByRole('tab', { name: '安装履历', exact: true }).click();
  await page.getByRole('button', { name: '记录安装或补录历史', exact: true }).click();
  await page.getByLabel('位置说明 *', { exact: true }).fill('独立位置');
  const time = page.getByLabel('实际安装时间', { exact: true });
  const offset = page.getByLabel('实际安装时间时区偏移', { exact: true });
  await time.fill('2026-01-02T10:00:00.123456');
  await expect(time).toHaveValue('2026-01-02T10:00:00.123456');
  await offset.fill('');
  await offset.pressSequentially('-05:00');
  await expect(time).toHaveValue('2026-01-02T10:00:00.123456');
  await page.getByRole('button', { name: '预览安装变更', exact: true }).click();
  await expect.poll(() => f.previews.length).toBe(1);
  expect(f.previews[0]!.installed_at).toBe('2026-01-02T10:00:00.123456-05:00');
  await page.getByRole('button', { name: '返回修改', exact: true }).click();
  await offset.fill('+0');
  await page.getByRole('button', { name: '预览安装变更', exact: true }).click();
  expect(f.previews).toHaveLength(1);
  await expect(time).toHaveValue('2026-01-02T10:00:00.123456');
  await offset.fill('Z');
  await page.getByRole('button', { name: '预览安装变更', exact: true }).click();
  await expect.poll(() => f.previews.length).toBe(2);
  expect(f.previews[1]!.installed_at).toBe('2026-01-02T10:00:00.123456Z');
  await page.getByRole('button', { name: '返回修改', exact: true }).click();
  await offset.fill('+0');
  await time.fill('');
  await page.getByRole('button', { name: '预览安装变更', exact: true }).click();
  await expect.poll(() => f.previews.length).toBe(3);
  expect(f.previews[2]).not.toHaveProperty('installed_at');
});

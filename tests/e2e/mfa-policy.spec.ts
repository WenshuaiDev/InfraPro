import { test, expect, createAccount, login, consoleURL, csrf, password } from './fixtures';
import { Permissions as P } from '../../packages/ts/api-client/src/permissions.generated';
import { syntheticTOTP } from '../helpers/totp';
import { randomUUID } from 'node:crypto';
import type { APIRequestContext, Page } from '@playwright/test';

test.use({ trace: 'off', screenshot: 'off', video: 'off' });
test.afterEach(async ({ page }) => { await page.goto('about:blank'); });
const origin = new URL(consoleURL).origin, api = origin + '/api/v1';
type Login = { username: string; password: string };
async function apiClient(request: APIRequestContext) {
  const headers = { Origin: origin, 'X-CSRF-Token': await csrf(request) };
  return async (path: string, data?: unknown, method = data === undefined ? 'GET' : 'POST') => {
    const r = await request.fetch(api + path, { method, headers: { ...headers, 'X-Command-ID': randomUUID() }, data });
    expect([200, 201, 204].includes(r.status())).toBe(true);
    return r.status() === 204 ? null : r.json();
  };
}
async function passwordStep(page: Page, account: Login) {
  await page.goto(consoleURL + '/login');
  await page.getByLabel('用户名', { exact: true }).fill(account.username);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
}
async function enrollment(page: Page) {
  await page.getByRole('button', { name: '显示绑定二维码' }).click();
  await expect(page.locator('code')).toBeVisible();
  const secret = (await page.locator('code').textContent())!;
  await page.getByLabel('新验证器动态码').fill(syntheticTOTP(secret, -1));
  await page.getByRole('button', { name: '确认绑定', exact: true }).click();
  await expect(page.getByRole('list', { name: '一次性恢复码' })).toBeVisible();
  await page.getByRole('button', { name: '我已保存，继续' }).click();
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
}

test('platform policy, read-only permission and administrator recovery use the real MFA state machine', async ({ page, request, browser }, testInfo) => {
  test.setTimeout(120_000);
  const p1 = await createAccount(request, false), p2 = await createAccount(request, false), p3 = await createAccount(request, false), user = await createAccount(request, false);
  const call = await apiClient(request);
  expect((await call('/auth/login', { username: 'e2e_owner', password })).status).toBe('authenticated');
  for (const [account, permissions] of [[p1, [P.PlatformSecurityPolicyRead, P.PlatformSecurityPolicyUpdate]], [p2, [P.PlatformSecurityPolicyRead, P.PlatformSecurityPolicyUpdate, P.PlatformAccountRead, P.PlatformAccountMfaReset]], [p3, [P.PlatformSecurityPolicyRead]]] as const) {
    const target = (await call('/access/accounts?q=' + account.username)).items[0];
    const role = await call('/access/roles', { name: 'MFA_' + account.username, description: '隔离验收角色', reason: 'MFA 验收' });
    await call('/access/roles/' + role.id + '/permissions', { expected_version: role.version, permission_codes: permissions, reason: 'MFA 验收' }, 'PUT');
    await call('/access/accounts/' + target.id + '/roles', { expected_version: target.management_version, role_ids: [...target.roles.map((r: { id: string }) => r.id), role.id], reason: 'MFA 验收' }, 'PUT');
  }
  const p2Context = await browser.newContext(), guestContext = await browser.newContext(), readContext = await browser.newContext();
  const second = await p2Context.newPage(), guest = await guestContext.newPage(), read = await readContext.newPage();
  try {
    const operator = await apiClient(p2Context.request); await operator('/auth/login', p2);
    const setup = await operator('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
    await operator('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
    await second.goto(consoleURL + '/platform/security-policy'); await expect(second.getByRole('heading', { name: '平台安全策略' })).toBeVisible();
    await login(guest, user); await login(read, p3); await read.goto(consoleURL + '/platform/security-policy');
    await expect(read.getByRole('switch')).toBeDisabled(); await expect(read.getByRole('button', { name: '保存策略', exact: true })).toHaveCount(0);
    await login(page, p1); await page.goto(consoleURL + '/platform/security-policy');
    await page.getByRole('switch').focus(); await page.keyboard.press('Space'); await page.getByRole('button', { name: '保存策略', exact: true }).click();
    await expect(page.getByRole('dialog').getByLabel('动态码', { exact: true })).toHaveCount(0);
    await page.getByLabel('变更原因', { exact: true }).fill('合成全平台强制验收'); await page.getByRole('button', { name: '确认保存', exact: true }).click();
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    expect((await guestContext.request.get(api + '/auth/me')).status()).toBe(401);
    await guest.reload(); await expect(guest.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await passwordStep(page, p1); await enrollment(page);
    // A fully authenticated MFA administrator keeps the original session and
    // uses only the existing password rule when switching the global policy.
    await second.reload(); await second.getByRole('switch').uncheck(); await second.getByRole('button', { name: '保存策略', exact: true }).click();
    await second.getByLabel('变更原因', { exact: true }).fill('合成解除强制验收'); await second.getByRole('button', { name: '确认保存', exact: true }).click();
    await expect(second.getByText('安全策略已生效。', { exact: true })).toBeVisible();
    expect((await guestContext.request.get(api + '/auth/me')).status()).toBe(401);
    // Optional mode still requires the administrator-issued grant after reset.
    const guestCall = await apiClient(guestContext.request); await guestCall('/auth/login', user);
    const uSetup = await guestCall('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
    await guestCall('/account/mfa/enrollment/confirm', { enrollment_id: uSetup.enrollment_id, new_totp_code: syntheticTOTP(uSetup.secret, -1) });
    let statusUnavailable = true;
    await second.route('**/access/accounts/*/mfa', async route => {
      if (statusUnavailable) await route.fulfill({ status: 503, json: { code: 'unavailable', message: '认证状态暂时不可用。' } });
      else await route.continue();
    });
    await second.goto(consoleURL + '/access/accounts'); await second.getByLabel('搜索用户名或显示名称').fill(user.username);
    await second.getByRole('button', { name: '浏览器验收', exact: true }).click();
    const mfaStatus = second.getByRole('region', { name: '账号 MFA 状态', exact: true });
    await expect(mfaStatus.getByText('暂时无法核实认证状态', { exact: true })).toBeVisible();
    await expect(mfaStatus.getByText('未启用', { exact: true })).toHaveCount(0);
    await expect(mfaStatus.getByRole('button', { name: '重置该账号 MFA' })).toHaveCount(0);
    statusUnavailable = false; await second.getByRole('button', { name: '重新读取认证状态' }).click();
    await second.getByRole('button', { name: '重置该账号 MFA' }).click();
    await second.getByLabel('重置原因', { exact: true }).fill('已通过合成身份核验'); await second.getByRole('button', { name: '确认重置', exact: true }).click();
    await expect(second.getByLabel('专用 MFA 恢复链接')).toBeVisible(); const recoveryURL = await second.getByLabel('专用 MFA 恢复链接').inputValue();
    await second.getByRole('button', { name: '关闭结果' }).click(); await expect(second.getByLabel('专用 MFA 恢复链接')).toHaveCount(0);
    await guest.goto(recoveryURL); expect(new URL(guest.url()).hash).toBe('');
    await guest.getByLabel('用户名', { exact: true }).fill(user.username); await guest.getByLabel('密码', { exact: true }).fill(user.password); await guest.getByRole('button', { name: '登录', exact: true }).click();
    await expect(guest.getByLabel('MFA 恢复凭证')).toBeVisible(); await guest.getByRole('button', { name: '认领恢复凭证' }).click(); await enrollment(guest);
    expect((await guestContext.request.get(api + '/auth/me')).status()).toBe(200);
    await second.goto(consoleURL + '/platform/security-policy'); await second.emulateMedia({ colorScheme: 'dark' });
    await expect(second.getByRole('heading', { name: '平台安全策略', exact: true })).toBeVisible();
    await expect(second.getByRole('switch')).toBeEnabled();
    expect(await second.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await second.screenshot({ path: testInfo.outputPath('mfa-policy-safe.png'), fullPage: true });
  } finally {
    // Leave the shared synthetic platform optional even if a later UI assertion
    // fails. The MFA-proven operator is independent of the session being tested.
    const operator = await apiClient(p2Context.request);
    const policy = await operator('/platform/security-policy');
    if (policy.require_mfa) await operator('/platform/security-policy', { require_mfa: false, expected_version: policy.revision, reason: '隔离用例清理' }, 'PUT');
    await Promise.all([p2Context.close(), guestContext.close(), readContext.close()]);
  }
});

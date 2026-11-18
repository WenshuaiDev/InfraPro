import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { expect } from '@playwright/test';
import { syntheticTOTP } from '../tests/helpers/totp.ts';

// Called with real personnel and machine runtime streams already connected.
export async function checkMFADisplay({ browser, screen, wall, page, client, call, owner, api, consoleOrigin }) {
  const origin = new URL(consoleOrigin).origin, operator = await browser.newContext();
  const username = 'mfa_display_' + randomUUID().replaceAll('-', '').slice(0, 12), password = randomUUID() + ' synthetic';
  const invitation = await call('/invitations', { note: 'MFA 人员持续连接与机器隔离' });
  const csrf = await (await operator.request.get(api + '/auth/csrf')).json();
  const request = async (path, data, method = data === undefined ? 'GET' : 'POST') => {
    const response = await operator.request.fetch(api + path, { method, headers: { Origin: origin, 'X-CSRF-Token': csrf.token, 'X-Command-ID': randomUUID() }, data });
    assert.ok([200, 201, 204].includes(response.status()), 'MFA stream verification ' + method + ' ' + path + ' HTTP ' + response.status());
    return response.status() === 204 ? undefined : response.json();
  };
  try {
    await request('/public/invitations/accept', { token: new URLSearchParams(new URL(invitation.url).hash.slice(1)).get('token'), username, display_name: 'MFA 连接验收管理员', password });
    const target = (await call('/access/accounts?q=' + username)).items.find(a => a.username === username);
    await call('/access/accounts/' + target.id + '/roles', { expected_version: target.management_version, role_ids: owner.roles.map(r => r.id), reason: 'MFA 连接验收管理员' }, 'PUT');
    assert.equal((await request('/auth/login', { username, password })).status, 'authenticated');
    const setup = await request('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
    await request('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
    const policy = await request('/platform/security-policy');
    const started = Date.now();
    await request('/platform/security-policy', { require_mfa: true, expected_version: policy.revision, reason: '持续连接实时撤权验收' }, 'PUT');
    await expect(screen.locator('#status')).toContainText('运行已停止', { timeout: 30_000 });
    await expect(screen.locator('iframe')).toHaveCount(0);
    assert.ok(Date.now() - started < 30_000, 'personnel stream must stop within 30 seconds');
    await expect(wall.locator('#status')).toHaveText('已连接 · 应用已就绪');
    const wallStatus = await wall.evaluate(async () => {
      return (await fetch('/runtime/terminal/start', { method: 'POST', headers: { 'X-Display-Protocol': '1', 'Content-Type': 'application/json' }, body: '{}' })).status;
    });
    assert.equal(wallStatus, 200);
    assert.equal((await client.get(api + '/auth/me')).status(), 401);
    const required = await request('/platform/security-policy');
    await request('/platform/security-policy', { require_mfa: false, expected_version: required.revision, reason: '持续连接验收结束' }, 'PUT');
    assert.equal((await client.get(api + '/auth/me')).status(), 401);
    await call('/auth/logout', {});
    assert.equal((await call('/auth/login', { username: 'e2e_owner', password: 'Real browser 河流 secret 592' })).status, 'authenticated');
    await page.goto(consoleOrigin + '/login');
    await page.getByLabel('用户名', { exact: true }).fill('e2e_owner'); await page.getByLabel('密码', { exact: true }).fill('Real browser 河流 secret 592'); await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
    await screen.reload();
    await screen.getByRole('button', { name: '继续', exact: true }).click();
    await expect(screen.locator('#status')).toHaveText('已连接 · 应用已就绪', { timeout: 30_000 });
  } finally {
    await operator.close();
  }
}

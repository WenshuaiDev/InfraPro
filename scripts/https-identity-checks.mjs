import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import https from 'node:https';
import { expect } from '@playwright/test';
import { syntheticTOTP } from '../tests/helpers/totp.ts';

export async function checkHTTPSIdentity({ context, browser, page, client, call, consoleOrigin, api, headers, out }) {
  const consolePort = Number(process.env.INFRAPRO_HTTPS_CONSOLE_PORT ?? 28443), runtimeOrigin = `https://127.0.0.1:${process.env.INFRAPRO_HTTPS_RUNTIME_PORT ?? 28444}`, assetOrigin = `https://127.0.0.1:${process.env.INFRAPRO_HTTPS_ASSET_PORT ?? 28445}`;
  const checks = [], origin = new URL(consoleOrigin).origin;
  for (const probe of ['/healthz', '/readyz']) assert.equal((await client.get(origin + probe)).status(), 200);
  await page.goto(consoleOrigin + '/profile');
  await expect(page.getByRole('button', { name: '保存资料', exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('button', { name: '保存资料', exact: true })).toBeVisible();
  const cookies = await context.cookies();
  for (const name of ['infrapro_session', 'infrapro_csrf', 'infrapro_display', 'infrapro_terminal']) {
    const cookie = cookies.find(c => c.name === name);
    assert.ok(cookie?.secure && cookie.httpOnly, `secure HttpOnly ${name}`);
    assert.equal(cookie.domain, '127.0.0.1');
  }
  assert.equal(cookies.find(c => c.name === 'infrapro_session').sameSite, 'Lax');
  assert.ok(!(await page.evaluate(() => document.cookie)).includes('infrapro_session'));
  checks.push('trusted HTTPS deep route, login persistence, Secure/HttpOnly/SameSite cookies');

  await call('/account/profile', { display_name: 'HTTPS 自检账号' }, 'PATCH');
  assert.equal((await call('/account/profile')).account.display_name, 'HTTPS 自检账号');
  for (const bad of ['', 'null', `http://127.0.0.1:${consolePort}`, runtimeOrigin, 'https://attacker.example']) {
    const r = await client.patch(api + '/account/profile', { headers: { ...headers, Origin: bad, 'X-Forwarded-Proto': 'https', 'X-Forwarded-For': '198.51.100.99' }, data: { display_name: 'must not change' } });
    assert.equal(r.status(), 403); assert.equal((await r.json()).code, 'origin_rejected');
  }
  for (const token of ['', 'forged']) {
    const r = await client.patch(api + '/account/profile', { headers: { ...headers, 'X-CSRF-Token': token }, data: { display_name: 'must not change' } });
    assert.equal(r.status(), 403); assert.equal((await r.json()).code, 'csrf_invalid');
  }
  assert.equal((await call('/account/profile')).account.display_name, 'HTTPS 自检账号');
  const hostStatus = await new Promise((resolve, reject) => {
    const req = https.get({ hostname: '127.0.0.1', port: consolePort, servername: '', path: '/api/v1/auth/csrf', headers: { Host: 'attacker.example' } }, response => { response.resume(); resolve(response.statusCode); });
    req.on('error', reject);
  });
  assert.equal(hostStatus, 421);
  checks.push('write succeeds, wrong protocol/port/origin/Host and missing/forged CSRF rejected');

  const link = await call('/invitations', { note: 'HTTPS isolated acceptance' });
  assert.ok(link.url.startsWith(consoleOrigin + '/invite#token='));
  const guestContext = await browser.newContext(), guest = await guestContext.newPage();
  try {
    await guest.goto(link.url);
    await expect(guest.getByRole('heading', { name: '创建你的账号' })).toBeVisible();
    assert.equal(new URL(guest.url()).hash, '');
    await guest.getByLabel('用户名', { exact: true }).fill('https_guest');
    await guest.getByLabel('新密码', { exact: true }).fill('HTTPS synthetic 密码 592!');
    await guest.getByLabel('确认新密码', { exact: true }).fill('HTTPS synthetic 密码 592!');
    await guest.getByRole('button', { name: '创建账号', exact: true }).click();
    await expect(guest.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    const fixture = JSON.parse(readFileSync(join(out, 'account-links.json')));
    assert.ok(fixture.reset_desktop.startsWith(consoleOrigin + '/reset-password#token='));
    await guest.goto(fixture.reset_desktop);
    await expect(guest.getByRole('heading', { name: '设置新密码' })).toBeVisible();
    await guest.getByLabel('新密码', { exact: true }).fill('HTTPS reset 密码 592!');
    await guest.getByLabel('确认新密码', { exact: true }).fill('HTTPS reset 密码 592!');
    await guest.getByRole('button', { name: '重置密码', exact: true }).click();
    await expect(guest.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
    await guest.getByLabel('用户名', { exact: true }).fill('e2e_reset_desktop');
    await guest.getByLabel('密码', { exact: true }).fill('HTTPS reset 密码 592!');
    await guest.getByRole('button', { name: '登录', exact: true }).click();
    await expect(guest.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
    // Exercise the personnel challenge across the real TLS-terminating proxy.
    // Keep factor/code bodies solely in memory, outside captures and reports.
    const csrf = await (await guestContext.request.get(api + '/auth/csrf')).json();
    const mfaHeaders = { Origin: origin, 'X-CSRF-Token': csrf.token };
    const post = (path, data) => guestContext.request.post(api + path, { headers: mfaHeaders, data });
    const begin = await post('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
    assert.equal(begin.status(), 200);
    const setup = await begin.json();
    const confirmed = await post('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
    assert.equal(confirmed.status(), 200);
    const codes = (await confirmed.json()).recovery_codes;
    assert.equal(codes.length, 10);
    assert.equal((await post('/auth/logout')).status(), 204);
    const pending = await post('/auth/login', { username: 'e2e_reset_desktop', password: 'HTTPS reset 密码 592!' });
    assert.equal(pending.status(), 202);
    const challenge = await pending.json();
    assert.equal(challenge.status, 'mfa_required');
    assert.ok(!challenge.account);
    const challengeCookie = (await guestContext.cookies()).find(c => c.name === 'infrapro_auth_challenge');
    assert.ok(challengeCookie?.secure && challengeCookie.httpOnly && challengeCookie.sameSite === 'Lax');
    assert.equal(challengeCookie.path, '/api/v1/auth');
    assert.equal(challengeCookie.domain, '127.0.0.1');
    assert.ok(!pending.headers()['set-cookie'].toLowerCase().includes('domain='));
    assert.ok(!(await guestContext.cookies()).some(c => c.name === 'infrapro_session'));
    assert.equal((await guestContext.request.get(api + '/auth/me')).status(), 401);
    assert.equal((await guestContext.request.get(api + '/account/profile')).status(), 401);
    const verified = await post('/auth/mfa/verify', { challenge_id: challenge.challenge_id, method: 'recovery_code', code: codes[0] });
    assert.equal(verified.status(), 200);
    assert.equal((await verified.json()).status, 'authenticated');
    assert.ok(!(await guestContext.cookies()).some(c => c.name === 'infrapro_auth_challenge'));
    assert.equal((await guestContext.request.get(api + '/auth/me')).status(), 200);
    assert.equal((await post('/auth/logout')).status(), 204);
  } finally { await guestContext.close(); }
  checks.push('HTTPS invitation/acceptance and password reset/login through browser');
  checks.push('MFA enrollment, 202 challenge isolation, recovery-code login and logout through trusted HTTPS proxy');

  const asset = await client.get(assetOrigin + '/assets/invalid/file', { headers: { Cookie: 'infrapro_session=synthetic; infrapro_csrf=synthetic', Authorization: 'Bearer synthetic' } });
  assert.equal(asset.status(), 401); // A missing asset permit cannot authenticate with personnel cookies.
  assert.equal(asset.headers()['set-cookie'], undefined);
  const internal = await client.get(runtimeOrigin + '/internal/metrics');
  assert.equal(internal.status(), 404);
  checks.push('asset proxy strips supplied credentials; internal runtime metrics blocked');

  const checkTLS = `const https=require('node:https');const fs=require('node:fs');const req=https.get({host:'127.0.0.1',port:${consolePort},path:'/console/login',...JSON.parse(process.argv[1]),...(process.argv[2]?{ca:fs.readFileSync(process.argv[2])}:{})},()=>process.exit(2));req.on('error',e=>{if(!['UNABLE_TO_VERIFY_LEAF_SIGNATURE','UNABLE_TO_GET_ISSUER_CERT_LOCALLY','SELF_SIGNED_CERT_IN_CHAIN','ERR_TLS_CERT_ALTNAME_INVALID'].includes(e.code))process.exit(3);});`;
  const noCA = { ...process.env }; delete noCA.NODE_EXTRA_CA_CERTS;
  execFileSync(process.execPath, ['-e', checkTLS, '{}'], { env: noCA, stdio: 'pipe' });
  execFileSync(process.execPath, ['-e', checkTLS, '{"servername":"wrong.example"}', process.env.NODE_EXTRA_CA_CERTS], { stdio: 'pipe' });
  checks.push('untrusted CA and certificate name mismatch fail TLS validation');

  await page.screenshot({ path: join(out, 'https-profile.png'), fullPage: true });
  await page.getByRole('button', { name: '账号菜单', exact: true }).click();
  await page.getByRole('menuitem', { name: '退出登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  assert.equal((await context.request.get(api + '/auth/me')).status(), 401);
  assert.ok(!(await context.cookies(origin)).some(c => c.name === 'infrapro_session'));
  checks.push('browser logout clears secure session and rejects authenticated read');
  writeFileSync(join(out, 'https-identity.json'), JSON.stringify({ passed: true, checks }, null, 2) + '\n');
}

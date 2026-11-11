import { logout, navigateTo } from '../helpers/navigation';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { test, expect } from '@playwright/test';
const origin = `http://${process.env.INFRAPRO_HTTP_TEST_HOST}:18088`;
const base = `${origin}/console`;
const password = 'Real browser 河流 secret 592';
const compose = (...args: string[]) => execFileSync('docker', ['compose', '-f', 'deploy/compose/http-test.yaml', ...args], { stdio: 'pipe', encoding: 'utf8' });

test('ordinary HTTP deployment: CLI, deep routes, credentials, cache and dependency recovery', async ({ page, context, browser }, testInfo) => {
  const browserErrors: string[] = [];
  page.on('pageerror', (error) => browserErrors.push(error.message));
  await page.addInitScript(() => {
    const violations: string[] = [];
    Object.assign(window, { policyViolations: violations });
    document.addEventListener('securitypolicyviolation', (event) => violations.push(`${event.effectiveDirective}: ${event.blockedURI}`));
  });
  await page.goto(`${base}/profile`); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  expect(await page.evaluate(() => window.isSecureContext)).toBe(false);
  expect(await page.evaluate(() => Boolean(navigator.clipboard))).toBe(false);
  await page.getByLabel('用户名', { exact: true }).fill('e2e_owner'); await page.getByLabel('密码', { exact: true }).fill(password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('button', { name: '保存资料' })).toBeVisible(); await page.reload(); await expect(page.getByRole('button', { name: '保存资料' })).toBeVisible();
  const session = (await context.cookies()).find((item) => item.name === 'infrapro_session'); expect(session).toMatchObject({ secure: false, httpOnly: true, sameSite: 'Lax', path: '/' });
  await page.getByRole('button', { name: '切换主题', exact: true }).click(); await page.getByRole('menuitemradio', { name: '深色' }).click(); await expect(page.locator('html')).toHaveClass('dark');
  expect(await page.evaluate(() => Reflect.get(window, 'policyViolations'))).toEqual([]);
  expect(browserErrors).toEqual([]);
  const legacy = await context.newPage(); await legacy.goto(`${origin}/screen/`); await expect(legacy).toHaveURL(`${base}/`); await expect(legacy.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
  await navigateTo(page, '账号开户邀请'); await page.reload(); await page.getByRole('button', { name: '生成邀请链接', exact: true }).click();
  const link = await page.getByLabel('可手动复制的完整链接').inputValue(); expect(link.startsWith(`${base}/invite#token=`)).toBe(true);
  await page.getByRole('button', { name: '复制链接', exact: true }).click(); await expect(page.getByText('自动复制不可用，请选中上方完整链接手动复制。')).toBeVisible();
  const recipientContext = await browser.newContext(); const recipient = await recipientContext.newPage(); await recipient.goto(link);
  await expect(recipient.getByRole('heading', { name: '创建你的账号' })).toBeVisible(); expect(new URL(recipient.url()).hash).toBe('');
  await recipient.getByLabel('用户名', { exact: true }).fill('http_guest'); await recipient.getByLabel('新密码', { exact: true }).fill(password); await recipient.getByLabel('确认新密码', { exact: true }).fill(password); await recipient.getByRole('button', { name: '创建账号', exact: true }).click(); await expect(recipient.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  const reset = JSON.parse(execFileSync('python3', ['tests/http/cli.py', 'reset', 'http_guest'], { encoding: 'utf8' })) as { url: string };
  await recipient.goto(reset.url); await expect(recipient.getByRole('heading', { name: '设置新密码' })).toBeVisible();
  await recipient.getByLabel('新密码', { exact: true }).fill('HTTP new 密码 1234'); await recipient.getByLabel('确认新密码', { exact: true }).fill('HTTP new 密码 1234'); await recipient.getByRole('button', { name: '重置密码', exact: true }).click(); await expect(recipient.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await recipient.getByLabel('用户名', { exact: true }).fill('http_guest'); await recipient.getByLabel('密码', { exact: true }).fill('HTTP new 密码 1234'); await recipient.getByRole('button', { name: '登录', exact: true }).click(); await expect(recipient.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
  // Tenant creation must work on plaintext HTTP, where crypto.randomUUID is absent.
  await navigateTo(page, '租户管理'); await page.getByRole('button', { name: '创建租户', exact: true }).click();
  await page.getByLabel('租户名称', { exact: true }).fill('HTTP 验收租户'); await page.getByLabel('租户编号', { exact: true }).fill('http-tenant'); await page.getByLabel('搜索接任账号').fill('http_guest');
  await page.getByRole('dialog').getByRole('button', { name: 'http_guest（http_guest）', exact: true }).click(); await page.getByRole('dialog').getByLabel('变更原因').fill('普通 HTTP 创建'); await page.getByRole('dialog').getByRole('button', { name: '确认提交', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'HTTP 验收租户', exact: true })).toBeVisible(); const tenantID = new URL(page.url()).pathname.split('/').at(-1)!;
  await recipient.goto(`${base}/t/${tenantID}/profile`); await expect(recipient.getByRole('heading', { name: '租户资料', exact: true })).toBeVisible(); await recipient.reload(); await expect(recipient.getByRole('main').getByText('HTTP 验收租户', { exact: true })).toBeVisible();
  expect(() => compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'disable', '--username', 'http_guest', '--reason', '未交接应拒绝')).toThrow();
  expect((await recipientContext.request.get(`${origin}/api/v1/auth/me`)).status()).toBe(200);
  const getStatus = () => page.evaluate(async () => (await fetch('/api/v1/auth/me', { cache: 'no-store' })).status);
  compose('restart', 'api'); await expect.poll(getStatus, { timeout: 20_000 }).toBe(200);
  // Deleting only this test environment's session cache triggers DB refill without changing the cookie.
  compose('exec', '-T', 'redis', 'sh', '-c', 'redis-cli -a test-only-local-password --scan --pattern "infrapro:http-test:identity:v1:session:*" | xargs -r redis-cli -a test-only-local-password DEL');
  expect(await getStatus()).toBe(200);
  compose('stop', 'redis'); expect(await getStatus()).toBe(503); expect((await context.request.get(`${origin}/api/v1/platform/tenants`)).status()).toBe(503);
  compose('start', 'redis'); await expect.poll(getStatus, { timeout: 15_000 }).toBe(200);
  compose('stop', 'postgres'); expect(await getStatus()).toBe(503); expect((await context.request.get(`${origin}/api/v1/platform/tenants`)).status()).toBe(503);
  compose('start', 'postgres'); await expect.poll(getStatus, { timeout: 15_000 }).toBe(200);
  const missing = await page.evaluate(async () => { const r = await fetch('/api/v1/missing'); return { status: r.status, type: r.headers.get('content-type'), body: await r.json() }; });
  expect(missing.status).toBe(404); expect(missing.type).toContain('application/json'); expect(missing.body.code).toBe('not_found');
  await page.screenshot({ path: testInfo.outputPath('ordinary-http.png'), fullPage: true, mask: [page.getByLabel('可手动复制的完整链接')] });
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'disable', '--username', 'http_guest', '--emergency', '--reason', 'HTTP 应急停用');
  expect((await context.request.get(`${origin}/readyz`)).status()).toBe(200); await page.reload(); await expect(page.getByText('管理员不可用，待处理', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '交接管理员', exact: true }).click(); await page.getByLabel('搜索接任账号').fill('e2e_owner'); await page.getByRole('dialog').getByRole('button', { name: 'HTTP 验收（e2e_owner）', exact: true }).click();
  await page.getByRole('dialog').getByLabel('变更原因').fill('HTTP 应急后交接'); await page.getByRole('dialog').getByRole('button', { name: '确认提交', exact: true }).click(); await expect(page.getByRole('dialog')).toBeHidden();
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'disable', '--username', 'http_guest', '--reason', 'HTTP account disable acceptance');
  await recipient.reload(); await expect(recipient.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'access', 'recover-admin', '--username', 'http_guest', '--reason', 'HTTP recovery acceptance');
  const disabled = JSON.parse(compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'find', '--username', 'http_guest')) as { status: string };
  expect(disabled.status).toBe('disabled');
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'account', 'enable', '--username', 'http_guest', '--reason', 'credential recovery complete');
  expect((await recipientContext.request.get(`${origin}/api/v1/auth/me`)).status()).toBe(401);
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'access', 'sync');
  compose('run', '--rm', '--no-deps', '-T', 'ops', 'access', 'check');
  await recipient.getByLabel('用户名', { exact: true }).fill('http_guest'); await recipient.getByLabel('密码', { exact: true }).fill('HTTP new 密码 1234'); await recipient.getByRole('button', { name: '登录', exact: true }).click();
  await expect(recipient.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
  const recovered = await (await recipientContext.request.get(`${origin}/api/v1/auth/me`)).json(); expect(recovered.permissions).toHaveLength(40); expect((await recipientContext.request.get(`${origin}/api/v1/tenants/${tenantID}/profile`)).status()).toBe(404);
  await logout(page); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await legacy.reload(); await expect(legacy.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await recipientContext.close();
  const logs = compose('logs', '--no-color', 'api', 'web');
  expect(logs).not.toContain(password); expect(logs).not.toContain(link.split('#token=')[1]); expect(logs).not.toContain(session?.value);
});

test('retired screen URLs redirect to fixed console destinations and serve no second app', async ({ request }) => {
  for (const [path, target] of [['/screen', '/console/'], ['/screen/', '/console/'], ['/screen/login', '/console/login'], ['/screen/assets/old.js', '/console/'], ['/screen/anything?returnTo=https://attacker.invalid', '/console/']]) {
    const response = await request.get(`${origin}${path}`, { maxRedirects: 0 });
    expect(response.status()).toBe(302);
    const location = response.headers().location;
    if (!location) throw new Error('The retired route did not provide a redirect destination');
    expect(new URL(location, origin).href).toBe(`${origin}${target}`);
  }
  const help = await request.get(`${origin}/api/v1/public/account-help`);
  expect(await help.json()).not.toHaveProperty('screen_url');
  const html = await request.get(`${base}/login`);
  expect(html.headers()['content-security-policy']).toContain("script-src 'self'");
  expect(html.headers()['content-security-policy']).not.toContain("script-src 'self' 'unsafe-inline'");
  expect(html.headers()['content-security-policy']).not.toContain('unsafe-eval');
  expect(await html.text()).not.toContain('ui-preview');
});

test('upstream failures return JSON without logging credential URLs', async ({ request }) => {
  const marker = `log-regression-${randomBytes(16).toString('hex')}`;
  expect((await request.get(`${origin}/readyz`)).status()).toBe(200);
  try {
    compose('stop', 'api');
    const response = await request.get(`${origin}/api/v1/auth/me?token=${marker}`);
    expect(response.status()).toBe(503);
    expect(response.headers()['content-type']).toContain('application/json');
    expect(await response.json()).toMatchObject({ code: 'unavailable' });
    const logs = compose('logs', '--no-color', 'web');
    expect(logs).not.toContain(marker);
    expect(logs).not.toContain('/api/v1/auth/me?token=');
  } finally {
    compose('start', 'api');
    await expect.poll(async () => (await request.get(`${origin}/readyz`)).status(), { timeout: 20_000 }).toBe(200);
  }
});

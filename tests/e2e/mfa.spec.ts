import { test, expect, login, consoleURL, createAccount, csrf } from './fixtures';
import { logout } from '../helpers/navigation';
import { syntheticTOTP } from '../helpers/totp';

// Disable captures for the entire secret-bearing journey, including failures.
// The only explicit screenshot is taken after all secret panels are removed.
test.use({ trace: 'off', screenshot: 'off', video: 'off' });
test.afterEach(async ({ page }) => { await page.goto('about:blank'); });

test('MFA enrollment, login, recovery, replacement and password-only management', async ({ page, account }, testInfo) => {
  test.setTimeout(90_000);
  await login(page, account); await page.goto(`${consoleURL}/security`);
  await page.getByRole('button', { name: '启用多因素认证', exact: true }).click();
  const dialog = page.getByRole('dialog'); await dialog.getByRole('button', { name: '开始设置', exact: true }).click();
  await expect(dialog.locator('code')).toBeVisible(); const secret = (await dialog.locator('code').textContent())!;
  await dialog.getByLabel('新验证器动态码').fill('abcdef'); await dialog.getByRole('button', { name: '确认新验证器', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('验证码无效');
  await dialog.getByLabel('新验证器动态码').fill(syntheticTOTP(secret, -1)); await dialog.getByRole('button', { name: '确认新验证器', exact: true }).click();
  const recovery = page.getByRole('list', { name: '一次性恢复码' }); await expect(recovery).toBeVisible();
  const oldCodes = await recovery.locator('li').allTextContents(); expect(oldCodes.length).toBe(10);
  // Verify an explicit copy without placing test secrets on the OS clipboard.
  await page.evaluate(() => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: async (text: string) => {
      const values = Array.from(document.querySelectorAll('ul[aria-label="一次性恢复码"] li')).map(node => node.textContent!);
      document.body.dataset.testCopied = String(values.length === 10 && values.every(value => text.includes(value)));
    } } });
  });
  expect(await page.locator('body').getAttribute('data-test-copied')).toBeNull();
  await page.getByRole('button', { name: '复制恢复码', exact: true }).click();
  await expect(page.locator('body')).toHaveAttribute('data-test-copied', 'true');
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: '下载恢复码', exact: true }).click();
  const download = await downloadPromise, stream = await download.createReadStream(), chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(Buffer.from(chunk));
  expect(oldCodes.every(code => Buffer.concat(chunks).toString('utf8').includes(code))).toBe(true);
  await download.delete(); chunks.forEach(chunk => chunk.fill(0));
  await page.getByRole('button', { name: '我已保存，继续', exact: true }).click(); await expect(recovery).toHaveCount(0);
  await expect(page.getByText('本人已启用', { exact: true })).toBeVisible();
  await logout(page);
  await page.getByLabel('用户名', { exact: true }).fill(account.username); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: '验证登录' })).toBeVisible(); await page.reload(); await expect(page.getByRole('heading', { name: '验证登录' })).toBeVisible();
  expect((await page.request.get(`${new URL(consoleURL).origin}/api/v1/auth/me`)).status()).toBe(401);
  await page.getByLabel('动态码', { exact: true }).fill(syntheticTOTP(secret)); await page.getByRole('button', { name: '验证并登录', exact: true }).click();
  await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible(); await page.goto(`${consoleURL}/security`);
  await page.getByRole('button', { name: '重建恢复码', exact: true }).click(); await expect(dialog.getByLabel('动态码', { exact: true })).toHaveCount(0); await expect(dialog.getByLabel('新验证器动态码')).toHaveCount(0);
  await dialog.getByRole('button', { name: '确认操作', exact: true }).click(); await expect(recovery).toBeVisible(); const codes = await recovery.locator('li').allTextContents(); expect(codes.every((code) => !oldCodes.includes(code))).toBe(true);
  await page.getByRole('button', { name: '我已保存，继续' }).click();
  await page.getByRole('button', { name: '更换验证器', exact: true }).click(); await dialog.getByRole('button', { name: '开始设置' }).click(); await expect(dialog.locator('code')).toBeVisible();
  await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(page.getByText('本人已启用', { exact: true })).toBeVisible();
  await logout(page); await page.getByLabel('用户名', { exact: true }).fill(account.username); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.getByRole('button', { name: '使用恢复码', exact: true }).click(); await page.getByLabel('恢复码', { exact: true }).fill(oldCodes[0]!); await page.getByRole('button', { name: '验证并登录' }).click(); await expect(page.getByRole('alert')).toContainText('验证码无效');
  await page.getByLabel('恢复码', { exact: true }).fill(codes[0]!.toLowerCase()); await page.getByRole('button', { name: '验证并登录' }).click(); await expect(page.getByRole('button', { name: '账号菜单' })).toBeVisible();
  await page.goto(`${consoleURL}/security`); await page.getByRole('button', { name: '更换验证器', exact: true }).click(); await dialog.getByRole('button', { name: '开始设置' }).click();
  await expect(dialog.locator('code')).toBeVisible(); const nextSecret = (await dialog.locator('code').textContent())!; expect(nextSecret !== secret).toBe(true);
  await dialog.getByLabel('新验证器动态码').fill(syntheticTOTP(nextSecret, -1)); await dialog.getByRole('button', { name: '确认新验证器', exact: true }).click(); await expect(recovery).toBeVisible(); await page.getByRole('button', { name: '我已保存，继续' }).click();
  await page.getByRole('button', { name: '关闭 MFA', exact: true }).click(); await expect(dialog.getByLabel('动态码', { exact: true })).toHaveCount(0); await dialog.getByRole('button', { name: '确认操作', exact: true }).click(); await expect(page.getByText('本人未启用', { exact: true })).toBeVisible();
  await page.emulateMedia({ colorScheme: 'dark' }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('mfa-security-safe.png'), fullPage: true });
  await logout(page); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await login(page, account);
});

test('two tabs reject mismatched challenges and discard a late response after switching accounts', async ({ page, context, request, browser, account }) => {
  test.setTimeout(90_000);
  const next = await createAccount(request), origin = new URL(consoleURL).origin, api = origin + '/api/v1';
  const isolated = await browser.newContext();
  async function enable(http: typeof request, user: typeof account) {
    const headers = { Origin: origin, 'X-CSRF-Token': await csrf(http) };
    const post = (path: string, data?: unknown) => http.post(api + path, { headers, data });
    expect((await post('/auth/login', user)).status()).toBe(200);
    const start = await post('/account/mfa/enrollment/start', { mode: 'enable', expected_factor_version: '1' });
    expect(start.status()).toBe(200); const setup = await start.json();
    const confirmation = await post('/account/mfa/enrollment/confirm', { enrollment_id: setup.enrollment_id, new_totp_code: syntheticTOTP(setup.secret, -1) });
    expect(confirmation.status()).toBe(200); const result = await confirmation.json();
    expect((await post('/auth/logout')).status()).toBe(204);
    return result.recovery_codes[0] as string;
  }
  const firstCode = await enable(context.request, account), nextCode = await enable(isolated.request, next);
  await isolated.close();
  await page.goto(consoleURL + '/login');
  await page.getByLabel('用户名', { exact: true }).fill(account.username); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: '验证登录' })).toBeVisible();
  const firstChallenge = await (await context.request.get(api + '/auth/mfa/challenge')).json();
  let release!: () => void, reached!: () => void;
  const held = new Promise<void>(resolve => { release = resolve; }), started = new Promise<void>(resolve => { reached = resolve; });
  await page.route('**/auth/mfa/verify', async route => { reached(); await held; const response = await route.fetch(); await route.fulfill({ response }); });
  await page.getByRole('button', { name: '使用恢复码' }).click(); await page.getByLabel('恢复码', { exact: true }).fill(firstCode); await page.getByRole('button', { name: '验证并登录' }).click(); await started;
  const other = await context.newPage();
  try {
    await other.goto(consoleURL + '/login'); await other.getByRole('button', { name: '退出认证 / 切换账号' }).click();
    await other.getByLabel('用户名', { exact: true }).fill(next.username); await other.getByLabel('密码', { exact: true }).fill(next.password); await other.getByRole('button', { name: '登录', exact: true }).click();
    await expect(other.getByRole('heading', { name: '验证登录' })).toBeVisible();
    const mismatch = await context.request.post(api + '/auth/mfa/verify', { headers: { Origin: origin, 'X-CSRF-Token': await csrf(context.request) }, data: { challenge_id: firstChallenge.challenge_id, method: 'recovery_code', code: firstCode } });
    expect(mismatch.status()).toBe(401); expect((await mismatch.json()).code).toBe('mfa_challenge_invalid');
    await other.getByRole('button', { name: '使用恢复码' }).click(); await other.getByLabel('恢复码', { exact: true }).fill(nextCode); await other.getByRole('button', { name: '验证并登录' }).click(); await expect(other.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
    await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
    const late = page.waitForResponse(response => response.url().endsWith('/auth/mfa/verify')); release(); expect((await late).status()).toBe(401);
    await expect(page.getByRole('heading', { name: '验证登录' })).toHaveCount(0);
    expect((await (await page.request.get(api + '/auth/me')).json()).account.username).toBe(next.username);
    await expect(page.getByRole('list', { name: '一次性恢复码' })).toHaveCount(0);
  } finally { release(); await other.close(); }
});

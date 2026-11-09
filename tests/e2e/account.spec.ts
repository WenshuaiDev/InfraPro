import { logout, navigateTo, expectIdentity } from '../helpers/navigation';
import { randomBytes } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { test, expect, login, consoleURL, password } from './fixtures';

test('protected routes, safe return, profile, cookie and shared tab logout', async ({ page, account, context }, testInfo) => {
  await page.goto(`${consoleURL}/profile`);
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.getByLabel('用户名', { exact: true }).fill(account.username);
  await page.getByLabel('密码', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('button', { name: '保存资料' })).toBeVisible();
  await page.getByLabel('显示名称', { exact: true }).fill('中文姓名 🙂');
  await page.getByRole('button', { name: '保存资料' }).click();
  await expect(page.getByText('个人资料已保存。')).toBeVisible();
  await page.reload(); await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('中文姓名 🙂');
  const cookies = await context.cookies(); const session = cookies.find((item) => item.name === 'infrapro_session');
  expect(session).toMatchObject({ httpOnly: true, secure: false, sameSite: 'Lax', path: '/' });
  const other = await context.newPage(); await other.goto(consoleURL);
  await expect(other.getByRole('heading', { name: '账号首页' })).toBeVisible();
  await expect(other.getByRole('heading', { name: '当前账号' })).toBeVisible();
  await expect(other.getByRole('main').getByText(account.username, { exact: true })).toBeVisible();
  await expectIdentity(other, account.username, '中文姓名 🙂');
  expect(await other.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await other.screenshot({ path: testInfo.outputPath('second-tab-account-home.png'), fullPage: true });
  await page.screenshot({ path: testInfo.outputPath('profile.png'), fullPage: true });
  await logout(page); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await other.reload(); await expect(other.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await page.goto(`${consoleURL}/login?returnTo=https://attacker.invalid/`);
  await page.getByLabel('用户名', { exact: true }).fill(account.username); await page.getByLabel('密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByRole('heading', { name: '账号首页' })).toBeVisible(); expect(new URL(page.url()).origin).toBe(new URL(consoleURL).origin);
  await expect(page.getByRole('heading', { name: '当前账号' })).toBeVisible();
  await expect(page.getByRole('main').getByText(account.username, { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('console-account-home.png'), fullPage: true });
});

test('invitation creation, manual copy, recipient validation, one-use and member-only capability', async ({ page, account, browser }, testInfo) => {
  await login(page, account); await navigateTo(page, '邀请管理');
  await page.evaluate(() => Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true }));
  await page.getByLabel('用途备注').fill('受邀同事'); await page.getByRole('button', { name: '生成邀请链接', exact: true }).click();
  const output = page.getByLabel('可手动复制的完整链接'); await expect(output).toBeVisible(); const link = await output.inputValue();
  await page.getByRole('button', { name: '复制链接', exact: true }).click(); await expect(page.getByText('自动复制不可用，请选中上方完整链接手动复制。')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('invitations.png'), fullPage: true, mask: [output] });
  const recipientContext = await browser.newContext(); const recipient = await recipientContext.newPage(); await recipient.goto(link);
  await expect(recipient.getByRole('heading', { name: '创建你的账号' })).toBeVisible(); expect(new URL(recipient.url()).hash).toBe('');
  await recipient.getByLabel('用户名', { exact: true }).fill(account.username);
  await recipient.getByLabel('新密码', { exact: true }).fill(password); await recipient.getByLabel('确认新密码', { exact: true }).fill(password);
  await recipient.getByRole('button', { name: '创建账号', exact: true }).click(); await expect(recipient.getByRole('alert').filter({ hasText: '用户名已被占用' }).first()).toBeVisible();
  const username = `guest_${randomBytes(5).toString('hex')}`; await recipient.getByLabel('用户名', { exact: true }).fill(username);
  await recipient.getByLabel('新密码', { exact: true }).fill('ab中🙂xy7'); await recipient.getByLabel('确认新密码', { exact: true }).fill('ab中🙂xy7');
  await recipient.getByRole('button', { name: '创建账号', exact: true }).click(); await expect(recipient.getByRole('alert').filter({ hasText: '8–128' }).first()).toBeVisible();
  const unicodePassword = 'ab中🙂xy78'; await recipient.getByLabel('新密码', { exact: true }).fill(unicodePassword); await recipient.getByLabel('确认新密码', { exact: true }).fill(unicodePassword);
  await recipient.getByRole('button', { name: '创建账号', exact: true }).click(); await expect(recipient.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await login(recipient, { username, password: unicodePassword });
  await recipient.goto(`${consoleURL}/invitations`); await expect(recipient.getByRole('heading', { name: '403 · 无权限' })).toBeVisible();
  expect((await recipientContext.request.get(`${new URL(consoleURL).origin}/api/v1/invitations`)).status()).toBe(403);
  await logout(recipient); await expect(recipient.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await recipient.goto(link); await expect(recipient.getByRole('heading', { name: '暂时无法使用此链接' })).toBeVisible();
  await page.getByRole('button', { name: '刷新列表', exact: true }).click(); await expect(page.getByText('已使用', { exact: true })).toBeVisible();
  await recipientContext.close();
});

test('session revocation and changing password sign out other devices', async ({ page, account, browser }, testInfo) => {
  await login(page, account); const otherContext = await browser.newContext(); const other = await otherContext.newPage(); await login(other, account);
  await navigateTo(page, '账号安全'); await expect(page.getByText('其他会话', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '退出其他设备', exact: true }).click(); await expect(page.getByText('其他会话', { exact: true })).toHaveCount(0);
  await other.reload(); await expect(other.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await login(other, account);
  await page.getByLabel('当前密码', { exact: true }).fill(account.password); const next = 'Updated 河流 long passphrase';
  await page.getByLabel('新密码', { exact: true }).fill(next); await page.getByLabel('确认新密码', { exact: true }).fill(next);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath('security.png'), fullPage: true });
  await page.getByRole('button', { name: '修改密码并退出', exact: true }).click(); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await other.reload(); await expect(other.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await login(page, { username: account.username, password: next }); await navigateTo(page, '账号安全');
  await page.getByRole('button', { name: '退出全部设备', exact: true }).click(); await page.getByRole('alertdialog').getByRole('button', { name: '确认退出全部设备', exact: true }).click(); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await otherContext.close();
});

test('forgot instructions, reset completion, refresh loses only the in-memory credential', async ({ page }, testInfo) => {
  await page.goto(`${consoleURL}/forgot-password`); await expect(page.getByText('测试维护人员：内线 8000')).toBeVisible();
  const fixture = JSON.parse(readFileSync('build/e2e/fixture.json', 'utf8')) as Record<string, string>;
  const link = fixture[`reset_${testInfo.project.name}`]; if (!link) throw new Error('Missing reset fixture');
  await page.goto(link); await expect(page.getByRole('heading', { name: '设置新密码' })).toBeVisible(); expect(new URL(page.url()).hash).toBe('');
  await page.reload(); await expect(page.getByRole('heading', { name: '请重新打开原链接' })).toBeVisible();
  await page.goto(link); await expect(page.getByRole('heading', { name: '设置新密码' })).toBeVisible();
  const next = 'Reset 河流 changed 12345'; await page.getByLabel('新密码', { exact: true }).fill(next); await page.getByLabel('确认新密码', { exact: true }).fill(next);
  await page.getByRole('button', { name: '重置密码', exact: true }).click(); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible();
  await login(page, { username: `e2e_reset_${testInfo.project.name}`, password: next });
  await logout(page); await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible(); await page.goto(link); await expect(page.getByRole('heading', { name: '暂时无法使用此链接' })).toBeVisible();
});

test('account service outage is not mistaken for logout and retry recovers', async ({ page, account }) => {
  await login(page, account);
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ status: 503, json: { code: 'unavailable', message: '暂时不可用', request_id: 'test' } }));
  await page.reload(); await expect(page.getByRole('heading', { name: '服务暂时不可用' })).toBeVisible(); await expect(page.getByLabel('密码', { exact: true })).toHaveCount(0);
  await page.unroute('**/api/v1/auth/me'); await page.getByRole('button', { name: '重新连接账号服务' }).click(); await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
});

test('password confirmation and CSRF failure never silently resubmit a write', async ({ page, account }) => {
  await login(page, account); await navigateTo(page, '邀请管理');
  let creates = 0;
  await page.route('**/api/v1/invitations', async (route) => {
    if (route.request().method() !== 'POST') { await route.continue(); return; }
    creates++;
    if (creates === 1) await route.fulfill({ status: 403, json: { code: 'reauthentication_required', message: '请先确认当前密码。', request_id: 'test' } });
    else await route.continue();
  });
  await page.getByRole('button', { name: '生成邀请链接', exact: true }).click(); await expect(page.getByRole('heading', { name: '确认当前密码' })).toBeVisible();
  await page.getByLabel('当前密码', { exact: true }).fill(account.password); await page.getByRole('button', { name: '确认密码', exact: true }).click();
  await expect(page.getByText('密码已确认，请再次提交生成请求。')).toBeVisible(); expect(creates).toBe(1);
  await page.getByRole('button', { name: '重新提交生成' }).click();
  await expect(page.getByLabel('可手动复制的完整链接')).toBeVisible(); expect(creates).toBe(2);
  const firstLink = await page.getByLabel('可手动复制的完整链接').inputValue();
  await page.getByRole('button', { name: '重新生成', exact: true }).click(); await expect(page.getByLabel('可手动复制的完整链接')).not.toHaveValue(firstLink);
  await page.getByRole('button', { name: '撤销', exact: true }).click(); await expect(page.getByLabel('可手动复制的完整链接')).toHaveCount(0); await expect(page.getByText('已撤销', { exact: true })).toHaveCount(2);
  await navigateTo(page, '个人资料'); await page.getByLabel('显示名称', { exact: true }).fill('保留未保存内容');
  let saves = 0;
  await page.route('**/api/v1/account/profile', async (route) => { if (route.request().method() !== 'PATCH') { await route.continue(); return; } saves++; await route.fulfill({ status: 403, json: { code: 'csrf_invalid', message: '页面验证已过期，请重新提交。', request_id: 'test' } }); });
  await page.getByRole('button', { name: '保存资料' }).click(); await expect(page.getByRole('alert')).toContainText('重新提交'); expect(saves).toBe(1);
  await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('保留未保存内容');
  await page.unroute('**/api/v1/account/profile');
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ status: 503, json: { code: 'unavailable', message: '故障', request_id: 'test' } }));
  await page.evaluate(() => window.dispatchEvent(new Event('focus'))); await expect(page.getByRole('heading', { name: '服务暂时不可用' })).toBeVisible();
  await page.unroute('**/api/v1/auth/me'); await page.getByRole('button', { name: '重新连接账号服务' }).click();
  await expect(page.getByLabel('显示名称', { exact: true })).toHaveValue('保留未保存内容'); await page.getByRole('button', { name: '保存资料' }).click(); await expect(page.getByText('个人资料已保存。')).toBeVisible();
});

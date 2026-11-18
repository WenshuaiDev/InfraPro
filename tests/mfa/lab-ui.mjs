import { readFileSync, writeFileSync } from 'node:fs';
import { chromium, expect } from '@playwright/test';

export async function verifyMFALabUI(session, directory) {
  const users = JSON.parse(readFileSync(session.credentials_file, 'utf8'));
  const browser = await chromium.launch({ headless: true });
  try {
    for (const label of ['P1', 'U6']) {
      const context = await browser.newContext({ viewport: label === 'U6' ? { width: 375, height: 812 } : { width: 1440, height: 1000 } });
      try {
        const page = await context.newPage();
        await page.goto(session.origin + '/login');
        await page.getByLabel('用户名', { exact: true }).fill(users[label].username);
        await page.getByLabel('密码', { exact: true }).fill(users[label].password);
        await page.getByRole('button', { name: '登录', exact: true }).click();
        await expect(page.getByRole('button', { name: '账号菜单', exact: true })).toBeVisible();
        await page.goto(session.origin + (label === 'P1' ? '/platform/security-policy' : '/security'));
        if (label === 'P1') {
          await expect(page.getByRole('heading', { name: '平台安全策略', exact: true })).toBeVisible();
          await expect(page.getByText(/修改人：系统维护/)).toBeVisible();
          await expect(page.getByRole('switch')).not.toBeChecked();
          await expect(page.getByRole('heading', { name: '影响预览', exact: true })).toBeVisible();
          const target = session.api_a + '/api/v1/access/accounts/' + users.U5.id;
          const account = await context.request.get(target), mfa = await context.request.get(target + '/mfa');
          if (account.status() !== 200 || mfa.status() !== 200 || (await account.json()).status !== 'disabled' || (await mfa.json()).state !== 'enabled') throw Error('disabled MFA fixture incomplete');
        } else {
          await expect(page.getByRole('button', { name: '修改密码', exact: true })).toHaveCount(0);
          await expect(page.getByRole('button', { name: '管理会话', exact: true })).toHaveCount(0);
          await page.getByRole('button', { name: '管理验证器', exact: true }).click();
          await expect(page.getByRole('dialog').getByRole('button', { name: '启用多因素认证', exact: true })).toBeVisible();
          await page.keyboard.press('Escape');
        }
        await page.screenshot({ path: directory + '/manual-' + label.toLowerCase() + '-safe.png', fullPage: true });
      } finally { await context.close(); }
    }
    writeFileSync(directory + '/ui-check.json', JSON.stringify({ passed: true, synthetic: true, checks: ['P1 optional policy and updater identity visible', 'disabled U5 retains its factor for controlled reset', 'roleless U6 can manage MFA without extra personal permissions'], secretsCaptured: false }, null, 2) + '\n');
  } catch {
    throw Error('Manual acceptance UI check failed; no secret-bearing context captured.');
  } finally { await browser.close(); }
}

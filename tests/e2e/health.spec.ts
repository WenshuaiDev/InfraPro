import { test, expect } from '@playwright/test';

for (const app of [
  { name: 'console', port: 15173, title: '工程运行总览' },
  { name: 'screen', port: 15174, title: '运行状态展示' },
]) {
  const url = `http://127.0.0.1:${app.port}`;

  test(`${app.name}: real API, keyboard refresh and responsive layout`, async ({ page }, testInfo) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()); });
    await page.goto(url);
    await expect(page.getByRole('heading', { name: app.title })).toBeVisible();
    await expect(page.getByRole('heading', { name: '服务连接正常' })).toBeVisible();
    await expect(page.getByText('infrapro-api', { exact: true })).toBeVisible();

    const responsePromise = page.waitForResponse((response) => response.url().endsWith('/api/v1/health') && response.status() === 200);
    await page.getByRole('button', { name: '刷新状态' }).focus();
    await page.keyboard.press('Enter');
    const response = await responsePromise;
    const body = await response.json();
    expect(body).toMatchObject({ status: 'ok', service: 'infrapro-api', version: '0.1.0' });
    await expect(page.locator('time')).toHaveAttribute('datetime', body.time);
    await expect(page.getByRole('button', { name: '刷新状态' })).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`${app.name}.png`), fullPage: true });
    expect(errors).toEqual([]);
  });

  test(`${app.name}: service failure and retry recovery`, async ({ page }) => {
    await page.route('**/api/v1/health', (route) => route.fulfill({ status: 503, json: { error: 'unavailable' } }));
    await page.goto(url);
    await expect(page.getByRole('heading', { name: '服务连接失败' })).toBeVisible();
    await expect(page.getByText('服务暂时不可用（HTTP 503），请稍后重试。')).toBeVisible();
    await expect(page.locator('time')).toHaveCount(0);
    await page.unroute('**/api/v1/health');
    await page.getByRole('button', { name: '重新连接' }).click();
    await expect(page.getByRole('heading', { name: '服务连接正常' })).toBeVisible();
    await expect(page.locator('time')).toHaveCount(1);
  });

  test(`${app.name}: malformed successful response is not healthy`, async ({ page }) => {
    await page.route('**/api/v1/health', (route) => route.fulfill({ json: { status: 'ok' } }));
    await page.goto(url);
    await expect(page.getByRole('heading', { name: '服务连接失败' })).toBeVisible();
    await expect(page.getByText('服务响应不符合健康检查契约，请检查 API 版本。')).toBeVisible();
  });
}

test('API responds through both Vite proxies with uncached JSON', async ({ request }) => {
  for (const port of [18080, 15173, 15174]) {
    const response = await request.get(`http://127.0.0.1:${port}/api/v1/health`);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/json');
    expect(response.headers()['cache-control']).toBe('no-store');
    expect(response.headers()['x-request-id']).toBeTruthy();
    const body = await response.json();
    expect(Object.keys(body).sort()).toEqual(['service', 'status', 'time', 'version']);
    expect(Math.abs(Date.now() - Date.parse(body.time))).toBeLessThan(5000);
  }
});

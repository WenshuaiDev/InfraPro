import { defineConfig } from '@playwright/test';

const apiURL=process.env.INFRAPRO_E2E_API_URL||'http://127.0.0.1:18080';
const consoleURL=process.env.INFRAPRO_E2E_CONSOLE_URL||'http://127.0.0.1:15173';
const env = {
  INFRAPRO_CONFIG: '',
  INFRAPRO_HTTP_ADDR: new URL(apiURL).host,
  INFRAPRO_LOG_LEVEL: 'warn',
  API_PROXY_TARGET: apiURL,
  CONSOLE_PORT: new URL(consoleURL).port,
  INFRAPRO_E2E_CONSOLE_URL: consoleURL,
  INFRAPRO_E2E_ORIGINS: new URL(consoleURL).origin,
  VITE_API_BASE_URL: '/api/v1',
};
const serverOptions = {
  timeout: 120_000,
  reuseExistingServer: false,
  gracefulShutdown: { signal: 'SIGTERM' as const, timeout: 10_000 },
  env,
};
const appCommand = process.env.INFRAPRO_E2E_PREVIEW ? 'preview' : 'dev';

export default defineConfig({
  testDir: './tests/e2e',
  outputDir: './test-results/e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: 2,
  reporter: [['list'], ['html', { outputFolder: 'playwright-report/e2e', open: 'never' }]],
  use: { trace: 'retain-on-failure', screenshot: 'only-on-failure', reducedMotion: 'reduce' },
  projects: [
    { name: 'desktop', use: { browserName: 'chromium', viewport: { width: 1440, height: 1000 } } },
    { name: 'tablet', testMatch: '**/presentation.spec.ts', use: { browserName: 'chromium', viewport: { width: 1024, height: 768 } } },
    { name: 'mobile', use: { browserName: 'chromium', viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true } },
  ],
  webServer: [
    { ...serverOptions, command: 'node scripts/e2e-server.mjs', url: `${apiURL}/readyz` },
    { ...serverOptions, command: `pnpm --filter @infrapro/console ${appCommand}`, url: `${consoleURL}/api/v1/public/account-help` },
  ],
});

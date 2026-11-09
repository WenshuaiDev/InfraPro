import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/ui', fullyParallel: true, workers: 2,
  outputDir: './test-results/ui',
  use: { baseURL: 'http://127.0.0.1:15175', reducedMotion: 'reduce', screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  reporter: [['list'], ['html', { outputFolder: 'playwright-report/ui', open: 'never' }]],
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 1000 } } },
    { name: 'tablet', use: { viewport: { width: 1024, height: 768 } } },
    { name: 'mobile', use: { viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true } },
  ],
  webServer: { command: 'pnpm --filter @infrapro/console dev', env: { CONSOLE_PORT: '15175' }, url: 'http://127.0.0.1:15175/ui-preview.html', reuseExistingServer: false, gracefulShutdown: { signal: 'SIGTERM', timeout: 10_000 } },
});

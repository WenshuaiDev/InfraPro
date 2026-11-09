import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests/http', testMatch: '**/*.spec.ts', workers: 1, timeout: 120_000,
  reporter: [['list']], use: { browserName: 'chromium', viewport: { width: 1440, height: 1000 },
    launchOptions: { args: ['--no-proxy-server'] },
    screenshot: 'only-on-failure', trace: 'retain-on-failure',
  }, outputDir: 'test-results/http',
});

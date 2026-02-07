import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:5174',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] }
    }
  ],
  webServer: process.env.CI
    ? undefined
    : [
        {
          command: 'cd .. && ./dogwatch',
          url: 'http://localhost:9999',
          reuseExistingServer: true,
          timeout: 30000
        },
        {
          command: 'cd ../ui-v2 && npm run dev -- --host 0.0.0.0 --port 5174',
          url: 'http://localhost:5174',
          reuseExistingServer: true,
          timeout: 60000
        }
      ]
});

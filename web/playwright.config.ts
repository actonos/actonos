import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  webServer: process.env.ACTONOS_E2E_URL
    ? undefined
    : {
        command: 'npm run dev -- --host 127.0.0.1',
        url: 'http://127.0.0.1:5173',
        reuseExistingServer: true,
      },
  use: {
    baseURL: process.env.ACTONOS_E2E_URL || 'http://127.0.0.1:5173',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'mobile', use: { viewport: { width: 390, height: 844 } } },
    { name: 'tablet', use: { viewport: { width: 768, height: 1024 } } },
    { name: 'desktop', use: { viewport: { width: 1440, height: 900 } } },
  ],
});

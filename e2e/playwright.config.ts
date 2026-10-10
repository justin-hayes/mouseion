import { defineConfig, devices } from '@playwright/test';

// WebKit runs the native-controls journey and the Analysis evidence situations;
// the remaining specs run in Chromium only.
const webkitSpecs = ['**/webkit-native.spec.ts', '**/analysis-evidence.spec.ts'];

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  // CI runners vary in size (2 vCPUs for private repositories, 4 for public);
  // each worker drives a browser and its own fixture server, so match cores.
  workers: process.env.CI ? '100%' : 4,
  timeout: 15_000,
  expect: { timeout: 5_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['html', { outputFolder: 'playwright-report', open: 'never' }], ['line']] : 'list',
  use: {
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    headless: true,
  },
  projects: [
    { name: 'desktop-light', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 }, colorScheme: 'light' } },
    { name: 'desktop-dark', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 }, colorScheme: 'dark' } },
    { name: 'compact-light', use: { ...devices['Desktop Chrome'], viewport: { width: 375, height: 667 }, colorScheme: 'light' } },
    { name: 'compact-dark', use: { ...devices['Desktop Chrome'], viewport: { width: 375, height: 667 }, colorScheme: 'dark' } },
    { name: 'webkit-desktop-light', testMatch: webkitSpecs, use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'light' } },
    { name: 'webkit-desktop-dark', testMatch: webkitSpecs, use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'dark' } },
    { name: 'webkit-compact-light', testMatch: webkitSpecs, use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'light' } },
    { name: 'webkit-compact-dark', testMatch: webkitSpecs, use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'dark' } },
  ],
});

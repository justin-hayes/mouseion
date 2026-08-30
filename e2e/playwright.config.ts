import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  workers: process.env.CI ? 2 : 1,
  timeout: 15_000,
  expect: { timeout: 5_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['html', { outputFolder: 'playwright-report', open: 'never' }], ['line']] : 'list',
  use: {
    baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    headless: true,
  },
  projects: [
    { name: 'desktop-light', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 }, colorScheme: 'light' } },
    { name: 'desktop-dark', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 }, colorScheme: 'dark' } },
    { name: 'compact-light', use: { ...devices['Desktop Chrome'], viewport: { width: 375, height: 667 }, colorScheme: 'light' } },
    { name: 'compact-dark', use: { ...devices['Desktop Chrome'], viewport: { width: 375, height: 667 }, colorScheme: 'dark' } },
  ],
  webServer: {
    command: process.env.MOUSEION_FIXTURE_BIN ?? 'go run ./cmd/fixtureserver',
    url: process.env.MOUSEION_FIXTURE_URL ? `${process.env.MOUSEION_FIXTURE_URL}/healthz` : 'http://127.0.0.1:8099/healthz',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
    env: { MOUSEION_FIXTURE_ADDR: '127.0.0.1:8099' },
  },
});

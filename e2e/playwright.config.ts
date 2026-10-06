import { defineConfig, devices } from '@playwright/test';

const fixtureAddr = process.env.MOUSEION_FIXTURE_ADDR ?? '127.0.0.1:8099';
const fixtureURL = process.env.MOUSEION_FIXTURE_URL ?? `http://${fixtureAddr}`;

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  // The fixture server has one shared mutable store for the whole suite.
  // Parallel workers can otherwise observe another test's active language or
  // stateful workflow midway through an assertion.
  workers: 1,
  timeout: 15_000,
  expect: { timeout: 5_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['html', { outputFolder: 'playwright-report', open: 'never' }], ['line']] : 'list',
  use: {
    baseURL: fixtureURL,
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
    { name: 'webkit-desktop-light', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'light' } },
    { name: 'webkit-desktop-dark', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'dark' } },
    { name: 'webkit-compact-light', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'light' } },
    { name: 'webkit-compact-dark', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'dark' } },
  ],
  webServer: {
    command: process.env.MOUSEION_FIXTURE_BIN ?? 'go run ../cmd/fixtureserver',
    url: `${fixtureURL}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
    env: { MOUSEION_FIXTURE_ADDR: fixtureAddr },
  },
});

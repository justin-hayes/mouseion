import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 4,
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
    { name: 'webkit-desktop-light', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'light' } },
    { name: 'webkit-desktop-dark', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 1280, height: 800 }, colorScheme: 'dark' } },
    { name: 'webkit-compact-light', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'light' } },
    { name: 'webkit-compact-dark', testMatch: '**/webkit-native.spec.ts', use: { ...devices['Desktop Safari'], browserName: 'webkit', viewport: { width: 375, height: 667 }, colorScheme: 'dark' } },
  ],
});

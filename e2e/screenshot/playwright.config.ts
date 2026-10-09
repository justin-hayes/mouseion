// Separate Playwright configuration for the My Books documentation screenshot.
// It is not part of `make browser-smoke` or CI: `e2e/playwright.config.ts` only
// collects specs under `e2e/tests`, and this file only collects the screenshot
// spec. Run it through `make screenshot-my-books`, which sets the environment.
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: 'my-books.spec.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  // Catalog sync, covers, and analysis arrive through River jobs, so the workflow
  // waits on them. Each wait is bounded in my-books.spec.ts; this budget covers
  // their sum (about 46 minutes worst case) plus the UI steps between them.
  timeout: 75 * 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list']],
  outputDir: process.env.MOUSEION_SCREENSHOT_RESULTS ?? '../../.tmp/screenshot/test-results',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: process.env.MOUSEION_SCREENSHOT_BASE_URL,
    headless: true,
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    colorScheme: 'light',
    trace: 'off',
    screenshot: 'off',
    video: 'off',
  },
});

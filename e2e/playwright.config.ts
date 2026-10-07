import { execFileSync } from 'node:child_process';
import { defineConfig, devices } from '@playwright/test';

function availableLoopbackPort() {
  const script = [
    'const net = require("node:net");',
    'const server = net.createServer();',
    'server.listen(0, "127.0.0.1", () => {',
    '  const address = server.address();',
    '  if (!address || typeof address === "string") process.exitCode = 1;',
    '  else process.stdout.write(String(address.port));',
    '  server.close();',
    '});',
  ].join('\n');
  const port = Number(execFileSync(process.execPath, ['-e', script], { encoding: 'utf8' }).trim());
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`Could not select a loopback port for the fixture server: ${port}`);
  }
  return port;
}

const useExternalFixture = process.env.MOUSEION_REUSE_FIXTURE === '1';
const configuredURL = process.env.MOUSEION_FIXTURE_URL;
const configuredAddr = process.env.MOUSEION_FIXTURE_ADDR;
const fixtureAddr = configuredAddr
  ?? (configuredURL ? new URL(configuredURL).host : `127.0.0.1:${availableLoopbackPort()}`);
const fixtureURL = configuredURL ?? `http://${fixtureAddr}`;

// Keep helper-created browser contexts on the same isolated fixture URL.
process.env.MOUSEION_FIXTURE_ADDR = fixtureAddr;
process.env.MOUSEION_FIXTURE_URL = fixtureURL;

if (useExternalFixture && !configuredURL && !configuredAddr) {
  throw new Error('Set MOUSEION_FIXTURE_URL or MOUSEION_FIXTURE_ADDR when MOUSEION_REUSE_FIXTURE=1.');
}

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
  webServer: useExternalFixture ? undefined : {
    command: process.env.MOUSEION_FIXTURE_BIN ?? 'go run ../cmd/fixtureserver',
    url: `${fixtureURL}/healthz`,
    // Never accept a server from another checkout as this run's fixture.
    reuseExistingServer: false,
    timeout: 30_000,
    env: { MOUSEION_FIXTURE_ADDR: fixtureAddr },
  },
});

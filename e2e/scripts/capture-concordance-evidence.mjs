import { spawn } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from '@playwright/test';

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(scriptDirectory, '../..');
const baseURL = process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8100';
const address = new URL(baseURL);
const output = resolve(repositoryRoot, 'doc/evidence/concordance-review');
let server;
let browser;
try {
  async function isReady() {
    try {
      const response = await fetch(new URL('/healthz', baseURL));
      return response.ok;
    } catch {
      return false;
    }
  }

  let ready = await isReady();
  if (!ready) {
    server = spawn('go', ['run', '../cmd/fixtureserver'], {
      cwd: resolve(repositoryRoot, 'e2e'),
      detached: true,
      env: { ...process.env, MOUSEION_FIXTURE_ADDR: `${address.hostname}:${address.port}` },
      stdio: 'inherit',
    });
    for (let attempt = 0; attempt < 60; attempt += 1) {
      if (await isReady()) {
        ready = true;
        break;
      }
      await delay(500);
    }
  }
  if (!ready) throw new Error(`Fixture server did not become ready at ${baseURL}`);

  await mkdir(output, { recursive: true });
  browser = await chromium.launch({ headless: true });

  async function concordancePage({ colorScheme = 'light', width = 1280, javaScriptEnabled = true, missingBundle = false } = {}) {
    const page = await browser.newPage({
      baseURL,
      colorScheme,
      viewport: { width, height: width < 500 ? 812 : 800 },
      javaScriptEnabled,
    });
    if (missingBundle) {
      await page.route('**/static/concordance.js', route => route.fulfill({ status: 404, body: 'missing bundle' }));
    }
    await page.goto('/login');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).click();
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
    return page;
  }

  const desktop = await concordancePage();
  await desktop.locator('mouseion-concordance .concordance-results').waitFor();
  await desktop.screenshot({ path: resolve(output, 'desktop-light-collapsed.png'), fullPage: true });
  await desktop.locator('mouseion-concordance .concordance-result').first().locator('summary').click();
  await desktop.screenshot({ path: resolve(output, 'desktop-light-expanded.png'), fullPage: true });
  await desktop.close();

  const dark = await concordancePage({ colorScheme: 'dark' });
  await dark.locator('mouseion-concordance .concordance-results').waitFor();
  await dark.screenshot({ path: resolve(output, 'desktop-dark-collapsed.png'), fullPage: true });
  await dark.close();

  const compact = await concordancePage({ width: 375 });
  await compact.locator('mouseion-concordance .concordance-results').waitFor();
  await compact.screenshot({ path: resolve(output, 'compact-light-collapsed.png'), fullPage: true });
  await compact.locator('mouseion-concordance .concordance-result').first().locator('summary').click();
  await compact.screenshot({ path: resolve(output, 'compact-light-expanded.png'), fullPage: true });
  await compact.close();

  const noScript = await concordancePage({ width: 320, javaScriptEnabled: false });
  await noScript.locator('#concordance-native-results').waitFor();
  await noScript.screenshot({ path: resolve(output, 'compact-no-javascript.png'), fullPage: true });
  await noScript.close();

  const failedBundle = await concordancePage({ missingBundle: true });
  await failedBundle.locator('#concordance-native-results').waitFor();
  await failedBundle.screenshot({ path: resolve(output, 'desktop-failed-bundle.png'), fullPage: true });
  await failedBundle.close();

  console.log(`Concordance evidence written to ${output}`);
} finally {
  await browser?.close();
  if (server?.pid) {
    try {
      process.kill(-server.pid, 'SIGTERM');
    } catch {
      server.kill('SIGTERM');
    }
  }
}

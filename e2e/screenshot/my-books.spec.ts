// Learner workflow for the My Books documentation screenshot. It creates the
// first account, adds the static catalog, waits for catalog sync and covers by
// observing the interface with bounded timeouts, asserts the scenario books,
// and writes the 1280 px light-scheme capture. Every input comes from the
// environment set by run.mjs.
import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';

interface ManifestBook {
  gutenberg: number;
  title: string;
  author: string;
}

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required; run this spec through make screenshot-my-books`);
  return value;
}

const username = requiredEnv('MOUSEION_SCREENSHOT_USERNAME');
const password = requiredEnv('MOUSEION_SCREENSHOT_PASSWORD');
const catalogName = requiredEnv('MOUSEION_SCREENSHOT_CATALOG_NAME');
const catalogURL = requiredEnv('MOUSEION_SCREENSHOT_CATALOG_URL');
const outputPath = requiredEnv('MOUSEION_SCREENSHOT_OUTPUT');
const manifest = JSON.parse(readFileSync(requiredEnv('MOUSEION_SCREENSHOT_MANIFEST'), 'utf8')) as {
  books: ManifestBook[];
};

const SYNC_TIMEOUT_MS = 5 * 60_000;
const COVER_TIMEOUT_MS = 6 * 60_000;
const POLL_INTERVAL_MS = 5_000;
const MAX_SCREENSHOT_HEIGHT = 1600;

async function pollUntil(check: () => Promise<boolean>, timeoutMs: number, description: string): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, POLL_INTERVAL_MS));
  }
  throw new Error(`Timed out after ${timeoutMs / 1000}s waiting for ${description}`);
}

async function createAccount(page: Page): Promise<void> {
  await page.goto('/login');
  const onboarding = await page.getByRole('heading', { name: 'Create your account' }).isVisible();
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: onboarding ? 'Create account' : 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/, { timeout: 30_000 });
}

function connectionRecord(page: Page) {
  return page.locator('article.catalog-record', {
    has: page.getByRole('heading', { name: catalogName, exact: true }),
  });
}

async function addCatalog(page: Page): Promise<void> {
  await page.goto('/catalogs');
  const form = page.locator('#catalog-connection-form form');
  await form.getByLabel('Name', { exact: true }).fill(catalogName);
  await form.getByLabel('Catalog URL', { exact: true }).fill(catalogURL);
  await form.getByRole('button', { name: 'Add catalog' }).click();
  await expect(connectionRecord(page)).toBeVisible({ timeout: 30_000 });
}

async function syncCatalog(page: Page): Promise<void> {
  await page.goto('/catalogs');
  await connectionRecord(page).getByRole('button', { name: 'Sync now' }).click();
  await pollUntil(
    async () => {
      await page.goto('/catalogs');
      const text = await connectionRecord(page).innerText();
      if (text.includes('Sync failed')) {
        throw new Error(`Catalog sync failed. Connection status: ${text.replace(/\s+/g, ' ').trim()}`);
      }
      return text.includes('Last synced');
    },
    SYNC_TIMEOUT_MS,
    'catalog sync to finish',
  );
}

async function bookStatus(page: Page, book: ManifestBook): Promise<'missing' | 'no-cover' | 'ready'> {
  const item = page.locator('li.library-book', {
    has: page.getByRole('heading', { name: book.title, exact: true }),
  });
  if ((await item.count()) === 0) return 'missing';
  const cover = item.locator('img.book-cover-media__image');
  if ((await cover.count()) === 0) return 'no-cover';
  const loaded = await cover.first().evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0);
  return loaded ? 'ready' : 'no-cover';
}

async function waitForScenarioBooks(page: Page): Promise<void> {
  const statuses = new Map<number, 'missing' | 'no-cover' | 'ready'>();
  await pollUntil(
    async () => {
      await page.goto('/library');
      for (const book of manifest.books) {
        statuses.set(book.gutenberg, await bookStatus(page, book));
      }
      return [...statuses.values()].every(status => status === 'ready');
    },
    COVER_TIMEOUT_MS,
    'every scenario book with its catalog cover in My Books',
  ).catch(() => undefined);

  const unresolved = manifest.books
    .filter(book => statuses.get(book.gutenberg) !== 'ready')
    .map(book => `${book.title} (${statuses.get(book.gutenberg) ?? 'not checked'})`);
  expect(unresolved, 'scenario books missing from My Books or without a loaded catalog cover').toEqual([]);
}

async function settleAndCapture(page: Page): Promise<void> {
  await page.goto('/library');
  await page.emulateMedia({ colorScheme: 'light' });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForFunction(() => document.querySelectorAll('.htmx-request').length === 0, undefined, { timeout: 15_000 });
  // Covers below the fold are lazy-loaded, so scroll the whole list once to make
  // every image request, then return to the top before measuring.
  await page.evaluate(async () => {
    for (let y = 0; y < document.body.scrollHeight; y += 400) {
      window.scrollTo(0, y);
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    window.scrollTo(0, 0);
  });
  await page.waitForFunction(
    () => Array.from(document.images).every(image => image.complete && image.naturalWidth > 0),
    undefined,
    { timeout: 30_000 },
  );

  const height = await page.evaluate(() => document.documentElement.scrollHeight);
  await page.screenshot({
    path: outputPath,
    fullPage: true,
    clip: { x: 0, y: 0, width: 1280, height: Math.min(height, MAX_SCREENSHOT_HEIGHT) },
    type: 'png',
  });
}

test('My Books shows the scenario books with their catalog covers', async ({ page }) => {
  await createAccount(page);
  await addCatalog(page);
  await syncCatalog(page);
  await waitForScenarioBooks(page);
  await settleAndCapture(page);
});

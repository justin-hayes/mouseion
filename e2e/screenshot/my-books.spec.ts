// Learner workflow for the documentation screenshots. It creates the first
// account, adds the static catalog, waits for catalog sync and covers, moves the
// scenario Books to To Read and waits for each analysis, reads one Book to
// completion, starts the target Book, and then captures the Reading view and
// My Books. Every input comes from the environment set by run.mjs, and every
// wait is bounded and reports the Book or step that stalled.
import { expect, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';

type Journey = 'inbox' | 'to-read' | 'read' | 'current';

interface ManifestBook {
  gutenberg: number;
  title: string;
  author: string;
  journey: Journey;
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
const myBooksOutputPath = requiredEnv('MOUSEION_SCREENSHOT_OUTPUT');
const readingOutputPath = requiredEnv('MOUSEION_SCREENSHOT_READING_OUTPUT');
const manifest = JSON.parse(readFileSync(requiredEnv('MOUSEION_SCREENSHOT_MANIFEST'), 'utf8')) as {
  books: ManifestBook[];
};

const SYNC_TIMEOUT_MS = 5 * 60_000;
const COVER_TIMEOUT_MS = 6 * 60_000;
// Stanza analysis of full-length EPUBs runs on CPU and the Books are analyzed
// in parallel, so this bounds the whole To Read analysis set.
const ANALYSIS_TIMEOUT_MS = 30 * 60_000;
const COUNTS_TIMEOUT_MS = 5 * 60_000;
const MOVE_TIMEOUT_MS = 60_000;
const POLL_INTERVAL_MS = 5_000;
const MAX_SCREENSHOT_HEIGHT = 1600;

const WORKFLOW_LABEL: Record<Journey, string> = {
  inbox: 'Inbox',
  'to-read': 'To Read',
  read: 'Read',
  current: 'Currently reading',
};

async function pollUntil(check: () => Promise<boolean>, timeoutMs: number, description: string): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, POLL_INTERVAL_MS));
  }
  throw new Error(`Timed out after ${timeoutMs / 1000}s waiting for ${description}`);
}

function booksWithJourney(journey: Journey): ManifestBook[] {
  return manifest.books.filter(book => book.journey === journey);
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

function myBookItem(page: Page, book: ManifestBook) {
  return page.locator('li.library-book', {
    has: page.getByRole('heading', { name: book.title, exact: true }),
  });
}

function chooserItem(page: Page, book: ManifestBook) {
  return page.locator('li.reading-chooser-book', {
    has: page.getByRole('heading', { name: book.title, exact: true }),
  });
}

async function bookStatus(page: Page, book: ManifestBook): Promise<'missing' | 'no-cover' | 'ready'> {
  const item = myBookItem(page, book);
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

// The My Books membership badge names the workflow bucket (Inbox, To Read,
// Currently reading, Read). Reading the page fresh keeps the check current.
async function workflowLabel(page: Page, book: ManifestBook): Promise<string> {
  await page.goto('/library');
  const item = myBookItem(page, book);
  if ((await item.count()) === 0) return 'missing';
  return (await item.locator('.library-book__membership').innerText()).replace(/\s+/g, ' ').trim();
}

async function moveToRead(page: Page, book: ManifestBook): Promise<void> {
  await page.goto('/library');
  await myBookItem(page, book).getByRole('button', { name: 'Move to To Read' }).click();
  await pollUntil(
    async () => (await workflowLabel(page, book)) === WORKFLOW_LABEL['to-read'],
    MOVE_TIMEOUT_MS,
    `${book.title} to move to To Read`,
  );
}

// A To Read Book is ready for Reading once its candidate shows current known
// vocabulary coverage. Until then it stays in the chooser's in-progress or
// not-assessed group, and the group text is reported if it never gets there.
async function analysisState(page: Page, book: ManifestBook): Promise<string> {
  const item = chooserItem(page, book);
  if ((await item.count()) === 0) return 'not listed in Reading';
  const text = (await item.innerText()).replace(/\s+/g, ' ').trim();
  if (text.includes('Known vocabulary coverage')) return 'ready';
  return `not ready: ${text.slice(0, 160)}`;
}

async function waitForAnalyses(page: Page, books: ManifestBook[]): Promise<void> {
  const states = new Map<number, string>();
  await pollUntil(
    async () => {
      await page.goto('/reading');
      for (const book of books) {
        states.set(book.gutenberg, await analysisState(page, book));
      }
      return [...states.values()].every(state => state === 'ready');
    },
    ANALYSIS_TIMEOUT_MS,
    'the analysis of every To Read scenario book to complete',
  ).catch(() => undefined);

  const stalled = books
    .filter(book => states.get(book.gutenberg) !== 'ready')
    .map(book => `${book.title} (${states.get(book.gutenberg) ?? 'not checked'})`);
  expect(stalled, 'To Read scenario books whose analysis did not complete').toEqual([]);
}

async function startReading(page: Page, book: ManifestBook): Promise<void> {
  await page.goto('/reading');
  const start = chooserItem(page, book).locator('details').filter({ hasText: 'Start reading' });
  await expect(start, `${book.title} has a Start reading control`).toHaveCount(1, { timeout: 30_000 });
  await start.locator('summary').click();
  await start.getByRole('button', { name: 'Confirm start reading' }).click();
  await expect(page, `starting ${book.title} as the Current reading`).toHaveURL(/\/reading\?message=/, { timeout: 30_000 });
}

async function finishReading(page: Page, book: ManifestBook): Promise<void> {
  await page.goto('/reading');
  const goal = page.locator('#primary-goal-section');
  await expect(goal.getByRole('heading', { level: 1 }), `${book.title} is the Current reading`).toContainText(book.title, {
    timeout: 30_000,
  });
  const finish = goal.locator('details').filter({ hasText: 'Mark reading finished' }).first();
  await finish.locator('summary').click();
  await finish.getByRole('button', { name: 'Mark reading finished' }).click();
  await expect(page.getByRole('heading', { name: /Reading finished/i }), `finishing ${book.title}`).toBeVisible({
    timeout: 30_000,
  });
}

// Browse hides Known and Reserved words by default, so when the default view
// has no rows the learner reveals already-accounted-for words before the table
// is asserted. Counts that report themselves as updating are waited on first.
async function assertBookVocabulary(page: Page, book: ManifestBook): Promise<void> {
  await pollUntil(
    async () => {
      await page.goto('/reading');
      const status = page.locator('#vocabulary-counts-status');
      if ((await status.count()) === 0) return true;
      const text = await status.innerText();
      if (text.includes('unavailable')) {
        throw new Error(`Vocabulary counts for ${book.title} are unavailable: ${text.replace(/\s+/g, ' ').trim()}`);
      }
      return !text.includes('Updating');
    },
    COUNTS_TIMEOUT_MS,
    `vocabulary counts for ${book.title} to finish updating`,
  );

  const table = page.getByRole('table', { name: 'Current effective vocabulary' });
  if ((await table.locator('tbody tr').count()) === 0) {
    await page.locator('#vocabulary-include-all').check();
    await page.locator('form.vocabulary-filter').getByRole('button', { name: 'Search' }).click();
  }
  await expect(table.locator('tbody tr').first(), `a non-empty Book vocabulary table for ${book.title}`).toBeVisible({
    timeout: COUNTS_TIMEOUT_MS,
  });
}

async function settleAndCapture(page: Page, path: string, outputPath: string): Promise<void> {
  await page.goto(path);
  await page.emulateMedia({ colorScheme: 'light' });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForFunction(() => document.querySelectorAll('.htmx-request').length === 0, undefined, { timeout: 15_000 });
  // Covers below the fold are lazy-loaded, so scroll the whole page once to make
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

test('the scenario Books are analyzed, one is read to completion, and My Books and Reading are captured', async ({ page }) => {
  const [readBook] = booksWithJourney('read');
  const [currentBook] = booksWithJourney('current');
  expect(booksWithJourney('read'), 'manifest has exactly one read Book').toHaveLength(1);
  expect(booksWithJourney('current'), 'manifest has exactly one current Book').toHaveLength(1);
  expect(booksWithJourney('inbox').length, 'manifest keeps at least one Book in Inbox').toBeGreaterThan(0);

  await createAccount(page);
  await addCatalog(page);
  await syncCatalog(page);
  await waitForScenarioBooks(page);

  const toRead = manifest.books.filter(book => book.journey !== 'inbox');
  for (const book of toRead) {
    await moveToRead(page, book);
  }
  await waitForAnalyses(page, toRead);

  await startReading(page, readBook);
  await finishReading(page, readBook);
  await startReading(page, currentBook);
  await assertBookVocabulary(page, currentBook);
  await expect(page.locator('#primary-goal-section').getByRole('heading', { level: 1 })).toContainText(currentBook.title);
  await settleAndCapture(page, '/reading', readingOutputPath);

  for (const book of manifest.books) {
    expect(await workflowLabel(page, book), `${book.title} workflow bucket in My Books`).toBe(WORKFLOW_LABEL[book.journey]);
  }
  await settleAndCapture(page, '/library', myBooksOutputPath);
});

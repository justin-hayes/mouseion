import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

async function submitKnownVocabularyImport(page: Page) {
  const importForm = page.locator('form[hx-post*="/vocabulary/import"]');
  await importForm.locator('input[type="file"]').setInputFiles({
    name: 'known-de.txt', mimeType: 'text/plain',
    buffer: Buffer.from('Haus\nÜberraschung\n'),
  });
  await importForm.getByRole('button', { name: /import known vocabulary/i }).click();
  await expect(page.locator('#vocabulary-results')).toContainText(/queued/i);
}

test.describe('authenticated learner smoke', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('login and library expose representative content', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /my books|welcome/i }).first()).toBeVisible();
    await page.getByRole('link', { name: /my books/i }).first().click();
    await expect(page).toHaveURL(/\/library/);
    await expect(page.getByRole('heading', { name: 'My Books', exact: true })).toBeVisible();
    await expect(page.locator('section#library-results')).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-book"]', { hasText: 'Der lange Weg nach Hause' })).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-failed"]', { hasText: 'Fehlgeschlagene Analyse' })).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-empty"]', { hasText: 'Empty chapter' })).toBeVisible();
    await expect(page.getByText('Analysis failed — action required')).toBeVisible();
    await expect(page.locator('a[href="/jobs/43"]', { hasText: 'Review failed analysis' })).toBeVisible();
  });

  test('acquired books resolve from their canonical Book ID', async ({ page }) => {
    await page.goto('/books/fixture-book');
    await expect(page).toHaveURL('/books/fixture-book');
    await expect(page.getByRole('heading', { name: 'Der lange Weg nach Hause', exact: true })).toBeVisible();
    await expect(page.getByText('Acquire EPUB content')).toHaveCount(0);
  });

  test('metadata-only books show catalogue-driven actions', async ({ page }) => {
    await page.goto('/books/fixture-metadata-only');
    await page.getByRole('button', { name: 'Refresh metadata' }).click();
    await expect(page.getByRole('status')).toContainText('Metadata is already up to date.');

    await page.goto('/library');
    await expect(page.getByText('Add a book')).toHaveCount(0);
    const entry = page.locator('article.library-book').filter({ has: page.getByRole('heading', { name: 'Metadata-only migration book', exact: true }) });
    await expect(entry).toContainText('Not acquired');
    await expect(entry.locator('form[action$="/analyze"]')).toContainText('Start analysis');
    await expect(entry.getByText('Review scope')).toHaveCount(0);
    await expect(entry.getByText('Prepare deck')).toHaveCount(0);
    await expect(entry).toContainText('language not chosen');

  });

  test('exact analysis result redirects to the book page', async ({ page }) => {
    await page.goto('/books/fixture-book/analyses/fixture-run');
    await expect(page).toHaveURL(/\/books\/fixture-book/);
    await expect(page.getByRole('heading', { name: /Der lange Weg nach Hause/i })).toBeVisible();
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText(/Fixture German deck/i).first()).toBeVisible();
  });

  test('ready decks show truthful Primary Goal and Journey membership actions', async ({ page }) => {
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText('Primary Goal.', { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View Primary Goal in Reading Journey' })).toHaveAttribute('href', '/journey#journey-book-fixture-book');
    await expect(page.getByText(/campaign operations/i)).toHaveCount(0);

    await page.goto('/deck-preparations/fixture-journey-preparation/status');
    await expect(page.getByText('In Reading Journey.', { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View this Journey entry' })).toHaveAttribute('href', '/journey#journey-book-fixture-empty');
    await expect(page.getByRole('button', { name: 'Add to Reading Journey' })).toHaveCount(0);

    await page.goto('/deck-preparations/fixture-outside-journey-preparation/status');
    const addToJourney = page.getByRole('button', { name: 'Add to Reading Journey' });
    if (await addToJourney.count() > 0) {
      // First project run: the book starts outside the Journey.
      await expect(page.getByText('Not in Reading Journey.', { exact: false })).toBeVisible();
      await addToJourney.click();
    }
    // Idempotent end state for every project run over the shared fixture server:
    // the book is (or just became) a Journey member, linked to its exact entry.
    await expect(page.getByText('In Reading Journey.', { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View this Journey entry' })).toHaveAttribute('href', '/journey#journey-book-fixture-failed');
  });

  test('acquisition, Reading Journey, Vocabulary, and operational jobs are reachable', async ({ page }) => {
    await page.goto('/connections');
    await expect(page.getByText('Fixture catalog')).toBeVisible();
    await page.goto('/library');
    await expect(page.getByRole('heading', { name: 'My Books', exact: true })).toBeVisible();
    await expect(page.getByText(/Donaudampfschifffahrtsgesellschaftskapitänsmütze/).first()).toBeVisible();
    await page.goto('/books/fixture-metadata-only');
    await expect(page.getByRole('button', { name: 'Start analysis' })).toBeVisible();
    await expect(page.getByText('Acquire EPUB content')).toHaveCount(0);
    await page.goto('/campaigns?message=legacy-bookmark');
    await expect(page).toHaveURL(/\/journey\?message=legacy-bookmark/);
    await expect(page.getByRole('heading', { name: /reading journey/i })).toBeVisible();
    await page.goto('/journey');
    await expect(page.getByRole('heading', { name: /reading journey/i })).toBeVisible();
    await expect(page.locator('#primary-goal-heading')).toHaveText('Primary Goal');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('Provisional Journey');
    await expect(page.locator('#campaign-operations-heading')).toHaveText(/Campaign history & operations/);
    await expect(page.getByText(/Der lange Weg nach Hause/).first()).toBeVisible();
    await expect(page.getByText('Provisional — your order').first()).toBeVisible();
    await expect(page.getByRole('heading', { name: /compare your order with a vocabulary-efficient alternative/i })).toBeVisible();
    await page.getByText('Show vocabulary-efficient alternative (optional comparison)').click();
    // Earlier smoke cases may add fixture books to the Journey. Keep this
    // assertion structural so the comparison remains stable as that state
    // grows.
    await expect(page.getByText(/\d+ comparable, \d+ incomparable/)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Your order (canonical)', exact: true })).toBeVisible();
    await expect(page.getByText('Route match: familiar German').first()).toBeVisible();
    await expect(page.getByText('Route evidence pending').first()).toBeVisible();
    await expect(page.locator('#campaign-fixture-completed-campaign')).toBeVisible();
    await page.goto('/vocabulary');
    await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
    const languagePicker = page.locator('form.vocabulary-language-picker select[name="language"]');
    await expect(languagePicker.locator('option[value="de"]')).toContainText('German');
    await expect(languagePicker.locator('option[value="it"]')).toContainText('Italian');
    await languagePicker.selectOption('de');
    await page.getByRole('button', { name: 'View known vocabulary' }).click();
    await expect(page).toHaveURL(/\/vocabulary\?language=de/);
    await submitKnownVocabularyImport(page);
    await page.goto('/library');
    const primaryNavigation = page.locator('nav.site-header__nav');
    await expect(primaryNavigation.getByRole('link', { name: 'Settings', exact: true })).toHaveCount(0);
    await page.goto('/settings');
    await expect(page).toHaveURL(/\/library/);
    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' })).toBeVisible();
    await expect(page.getByRole('link', { name: '#1 · result' })).toBeVisible();
  });

  test('asserts initial HTML before HTMX enhancement and observes status', async ({ page }) => {
    // The import form and its results region are server-rendered only once a
    // study language is selected on the Vocabulary page.
    await page.goto('/vocabulary?language=de');
    const importForm = page.locator('form[hx-post*="/vocabulary/import"]');
    // Initial (pre-enhancement) HTML already carries the form and swap target.
    await expect(importForm).toHaveCount(1);
    await expect(page.locator('#vocabulary-results')).toHaveCount(1);
    // Attach a small multilingual UTF-8 lemma file, then submit via HTMX.
    await submitKnownVocabularyImport(page);
    await expect(page).toHaveURL(/\/vocabulary/);
    // The HTMX submission replaces the region with a durable status.
    await expect(page.locator('#vocabulary-results')).toContainText(/queued/i);
  });
});

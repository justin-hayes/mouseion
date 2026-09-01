import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('authenticated learner smoke', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('login and library expose representative content', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /my books|welcome/i }).first()).toBeVisible();
    await page.getByRole('link', { name: /my books/i }).first().click();
    await expect(page).toHaveURL(/\/library/);
    await expect(page.getByRole('heading', { name: 'My Books', exact: true })).toBeVisible();
    await expect(page.locator('section[aria-labelledby="acquired-books-heading"]')).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-book"]', { hasText: 'Der lange Weg nach Hause' })).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-failed"]', { hasText: 'Fehlgeschlagene Analyse' })).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-empty"]', { hasText: 'Empty chapter' })).toBeVisible();
    await expect(page.getByText('Analysis failed — action required')).toBeVisible();
    await expect(page.locator('a[href="/jobs/43"]', { hasText: 'Review failed analysis' })).toBeVisible();
  });

  test('metadata-only books can be added, show only supported actions, and removed', async ({ page }) => {
    await page.goto('/library');
    await page.locator('#book-form > summary').click();
    await expect(page.locator('#book-form')).toHaveAttribute('open', '');
    await page.getByRole('textbox', { name: 'Title' }).fill('Metadata-only browser book');
    await page.getByRole('radio', { name: /not chosen/i }).check();
    await page.getByRole('button', { name: 'Add to My Books' }).click();
    await expect(page).toHaveURL(/\/library\?message=/);

    const entry = page.locator('article.library-book').filter({ hasText: 'Metadata-only browser book' });
    await expect(entry).toContainText('Not acquired');
    await expect(entry.getByRole('button', { name: 'Acquire this book' })).toHaveAttribute('href', /book_id=/);
    await expect(entry.getByText('Review scope')).toHaveCount(0);
    await expect(entry.getByText('Start analysis')).toHaveCount(0);
    await expect(entry.getByText('Prepare deck')).toHaveCount(0);
    await expect(entry).toContainText('language not chosen');

    await entry.getByText('Remove from My Books').click();
    await entry.getByRole('button', { name: 'Confirm removal' }).click();
    await expect(page).toHaveURL(/\/library\?message=/);
    await expect(page.locator('article.library-book').filter({ hasText: 'Metadata-only browser book' })).toHaveCount(0);
  });

  test('exact analysis result and deck status are reachable', async ({ page }) => {
    await page.goto('/books/fixture-book/analyses/fixture-run');
    await expect(page.getByRole('heading', { name: /analysis result/i })).toBeVisible();
    // The book title appears in the breadcrumb, heading, and result body; assert
    // it is present without tripping Playwright strict mode.
    await expect(page.getByText('Der lange Weg nach Hause').first()).toBeVisible();
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

  test('acquisition, Reading Journey, Settings, and operational jobs are reachable', async ({ page }) => {
    await page.goto('/connections');
    await expect(page.getByText('Fixture catalog')).toBeVisible();
    await page.goto('/catalog?connection=fixture-connection');
    await expect(page.getByRole('heading', { name: 'Fixture catalog' })).toBeVisible();
    // The catalog page offers a language browse form: a visible select with
    // German and Italian options (options themselves are collapsed/hidden).
    const languageSelect = page.locator('form[action="/opds/language"] select[name="language"]');
    await expect(languageSelect).toBeVisible();
    await expect(languageSelect.locator('option[value="de"]')).toContainText('German');
    await expect(languageSelect.locator('option[value="it"]')).toContainText('Italian');
    // Browse a language via the server-rendered path and assert feed entries.
    await page.goto('/opds/browse?connection=fixture-connection&language=de');
    await expect(page.getByText('Ein deutsches Buch').first()).toBeVisible();
    await expect(page.getByText('Un libro italiano').first()).toBeVisible();
    await expect(page.getByRole('button', { name: /add to my books/i }).first()).toBeVisible();
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
    await expect(page.locator('#campaign-fixture-completed-campaign')).toBeVisible();
    await page.goto('/settings');
    await expect(page.getByRole('heading', { name: /German/ })).toBeVisible();
    await expect(page.getByRole('heading', { name: /Italian/ })).toBeVisible();
    await page.goto('/jobs');
    await expect(page.getByText(/Analysis job/i).first()).toBeVisible();
  });

  test('asserts initial HTML before HTMX enhancement and observes status', async ({ page }) => {
    // The import form and its results region are server-rendered only once a
    // study language is selected on the settings page.
    await page.goto('/settings?language=de');
    const importForm = page.locator('form[hx-post*="/known-vocab/import"]');
    // Initial (pre-enhancement) HTML already carries the form and swap target.
    await expect(importForm).toHaveCount(1);
    await expect(page.locator('#known-vocabulary-results')).toHaveCount(1);
    // Attach a small multilingual UTF-8 lemma file, then submit via HTMX.
    await importForm.locator('input[type="file"]').setInputFiles({
      name: 'known-de.txt', mimeType: 'text/plain',
      buffer: Buffer.from('Haus\nÜberraschung\n'),
    });
    await importForm.getByRole('button', { name: /import known vocabulary/i }).click();
    await expect(page).toHaveURL(/\/settings/);
    // The HTMX submission replaces the region with a durable status.
    await expect(page.locator('#known-vocabulary-results')).toContainText(/queued/i);
  });
});

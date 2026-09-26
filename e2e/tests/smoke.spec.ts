import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

async function setStudyLanguage(page: Page, language: string) {
  await page.goto('/library');
  const switcher = page.getByLabel('Study language');
  if (await switcher.inputValue() !== language) {
    await switcher.selectOption(language);
    await expect(page).toHaveURL(/\/library$/);
  }
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
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await expect(page.locator('section#library-results')).toBeVisible();
    await expect(page.locator('.library-grid a.library-book__identity-link[href="/reading#journey-book-fixture-book"]', { hasText: 'Der lange Weg nach Hause' })).toBeVisible();
    await expect(page.locator('.library-grid a[href="/books/fixture-failed"]', { hasText: 'Fehlgeschlagene Analyse' })).toHaveCount(0);
    await expect(page.locator('.library-grid a[href="/books/fixture-empty"]', { hasText: 'Empty chapter' })).toHaveCount(0);
    await expect(page.getByText('Analysis failed — action required')).toHaveCount(0);
    await expect(page.locator('a[href="/jobs/43"]', { hasText: 'Review failed analysis' })).toHaveCount(0);
  });

  test('legacy Journey entry redirects to the canonical Reading destination', async ({ page }) => {
    await page.goto('/journey?expected_revision=obsolete');
    await expect(page).toHaveURL('/reading');
    await expect(page.getByRole('link', { name: 'Reading', exact: true })).toHaveAttribute('aria-current', 'page');
  });

  test('active study language persists and marks new and no-book languages', async ({ page }) => {
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    await expect(switcher).toHaveValue('de');
    const initialNewLanguageOption = switcher.locator('option').filter({ hasText: '(new)' });
    await expect(initialNewLanguageOption).toHaveCount(1);
    const initialNewLanguage = await initialNewLanguageOption.getAttribute('value');
    expect(initialNewLanguage).toBeTruthy();
    expect(initialNewLanguage).not.toBe('de');
    await expect(switcher.locator('option[value="fr"]')).not.toBeDisabled();
    await expect(switcher.locator('option[value="fr"]')).toContainText('(no books)');

    await page.goto('/catalogs');
    await page.locator('#connection-fixture-connection').getByRole('button', { name: 'Sync now' }).click();
    await expect(page).toHaveURL(/\/catalogs\?message=/);
    await page.goto('/library');
    const arrivedNewLanguageOption = page.getByLabel('Study language').locator('option').filter({ hasText: '(new)' });
    await expect(arrivedNewLanguageOption).toHaveCount(1);
    const arrivedNewLanguage = await arrivedNewLanguageOption.getAttribute('value');
    expect(arrivedNewLanguage).toBeTruthy();
    expect(arrivedNewLanguage).not.toBe(initialNewLanguage);
    await expect(page.getByLabel('Study language').locator(`option[value="${initialNewLanguage}"]`)).not.toContainText('(new)');
    await expect(page.getByLabel('Study language')).toHaveValue('de');

    await switcher.selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading');
    await expect(page.getByLabel('Study language')).toHaveValue('it');
    await expect(page.locator('main h1')).toContainText(/Reading Journey in Italian|Choose your next book in Italian/);
    await expect(page.getByText('different study language', { exact: false })).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.getByRole('heading', { name: 'Choose your next book in fr', exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'No To Read books yet', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Browse My Books', exact: true })).toBeVisible();
    await page.getByLabel('Study language').selectOption('de');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.locator('main h1')).toContainText(/Reading Journey in German|Choose your next book in German/);

    await page.goto('/journey/fixture-book');
    await expect(page).toHaveURL('/reading#journey-book-fixture-book');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
  });

  test('Journey bookmarks resolve from their canonical Book ID', async ({ page }) => {
    await setStudyLanguage(page, 'de');
    await page.goto('/journey/fixture-book');
    await expect(page).toHaveURL('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book')).toBeVisible();
    await expect(page.getByText('Acquire EPUB content')).toHaveCount(0);
  });

  test('completed Journey members open their Reading Journey anchor', async ({ page }) => {
    await setStudyLanguage(page, 'de');
    await page.goto('/journey/fixture-route-match');

    await expect(page).toHaveURL('/reading#journey-book-fixture-route-match');
    await expect(page.locator('#journey-book-fixture-route-match')).toBeVisible();
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Deck preparation', exact: true })).toHaveCount(0);
  });

  test('Journey books open the focused deck preparation task', async ({ page }) => {
    await setStudyLanguage(page, 'de');
    await page.goto('/reading');
    const card = page.locator('#journey-book-fixture-route-match');
    const deckLink = card.getByRole('button', { name: /Prepare deck|View deck preparation/ }).first();
    await expect(deckLink).toHaveAttribute('href', '/reading/books/fixture-route-match/deck/preparations/new');
    await deckLink.click();

    await expect(page).toHaveURL('/reading/books/fixture-route-match/deck/preparations/new');
    await expect(page.getByRole('heading', { name: 'Deck preparation task', exact: true })).toBeVisible();
    await expect(page.getByText('fixture-route-match-run', { exact: true })).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /sending selected vocabulary/i })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Back to this Book in Reading Journey', exact: true })).toHaveAttribute('href', '/reading#journey-book-fixture-route-match');

    const form = page.locator('form[action="/reading/books/fixture-route-match/deck/preparations"]');
    await form.evaluate((element) => (element as HTMLFormElement).submit());
    await expect(page).toHaveURL(/\/deck-preparations\/fixture-submitted-fixture-route-match-run\/status$/);
    await expect(page.locator('#deck-preparation-status').getByText('Deck preparation queued', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Return to book', exact: true })).toHaveAttribute('href', '/reading#journey-book-fixture-route-match');
  });

  test('Reading Journey bookmarks retain the active study language', async ({ page }) => {
    await setStudyLanguage(page, 'de');
    await page.goto('/journey/fixture-route-match');

    await expect(page).toHaveURL('/reading#journey-book-fixture-route-match');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page.locator('#journey-book-fixture-route-match')).toBeVisible();
  });

  test('cross-language Journey bookmarks offer an explicit language handoff', async ({ page }) => {
    await setStudyLanguage(page, 'de');
    await page.goto('/library');
    await page.getByLabel('Study language').selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    await expect(page.getByRole('link', { name: 'Back to this Book in Reading Journey', exact: true })).toHaveAttribute(
      'href',
      '/reading?language_handoff_book=fixture-route-match&language_handoff_language=de',
    );
    await page.goto('/journey/fixture-route-match');

    await expect(page).toHaveURL(/\/reading\?language_handoff_book=fixture-route-match&language_handoff_language=de/);
    await expect(page.getByRole('heading', { name: 'This Book is in German', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Switch to German and open this Book', exact: true })).toBeVisible();
    await expect(page.locator('#journey-book-fixture-route-match')).toHaveCount(0);

    await page.getByRole('button', { name: 'Switch to German and open this Book', exact: true }).click();
    await expect(page).toHaveURL('/reading#journey-book-fixture-route-match');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page.locator('#journey-book-fixture-route-match')).toBeVisible();
  });

  test('unavailable and non-member Journey bookmarks remain unavailable', async ({ page }) => {
    const unavailable = await page.goto('/journey/fixture-empty');
    expect(unavailable?.status()).toBe(404);
    const nonMember = await page.goto('/journey/fixture-metadata-only');
    expect(nonMember?.status()).toBe(404);
    const unknown = await page.goto('/journey/not-owned-book');
    expect(unknown?.status()).toBe(404);
  });

  test('metadata-only book detail URLs are retired', async ({ page }) => {
    const response = await page.goto('/books/fixture-metadata-only');
    expect(response?.status()).toBe(404);

    await page.goto('/library');
    await expect(page.getByText('Add a book')).toHaveCount(0);
    await expect(page.locator('article.library-book').filter({ has: page.getByRole('heading', { name: 'Metadata-only migration book', exact: true }) })).toHaveCount(0);
  });

  test('exact analysis result redirects to the Reading Journey anchor', async ({ page }) => {
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') {
      await switcher.selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
    }
    await page.goto('/books/fixture-book/analyses/fixture-run');
    await expect(page).toHaveURL('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book')).toBeVisible();
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText(/Fixture German deck/i).first()).toBeVisible();
  });

  test('ready decks show truthful Primary Goal and Journey membership actions', async ({ page }) => {
    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText('Primary Goal.', { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View current book in Reading' })).toHaveAttribute('href', '/reading#journey-book-fixture-book');
    await expect(page.getByText(/campaign operations/i)).toHaveCount(0);

    await page.getByLabel('Study language').selectOption('it');
    await expect(page.getByLabel('Study language')).toHaveValue('it');
    await expect(page).toHaveURL(/\/deck-preparations\/fixture-preparation\/status$/);
    await expect(page.getByRole('link', { name: 'Return to book', exact: true })).toHaveAttribute(
      'href',
      '/reading?language_handoff_book=fixture-book&language_handoff_language=de',
    );
    await expect(page.getByRole('button', { name: 'Add to Reading Journey' })).toHaveCount(0);
    await page.goto('/deck-preparations/fixture-journey-preparation/status');
    await expect(page.getByText('In Reading Journey.', { exact: false })).toBeVisible();
    await expect(page.locator('a[href="/reading#journey-book-fixture-empty"]')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add to Reading Journey' })).toHaveCount(0);

    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page).toHaveURL(/\/deck-preparations\/fixture-journey-preparation\/status$/);
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
    await expect(page.locator('a[href="/reading#journey-book-fixture-failed"]')).toBeVisible();
  });

  test('acquisition, Reading Journey, Vocabulary, and operational jobs are reachable', async ({ page }) => {
    await page.goto('/catalogs');
    await expect(page.getByText('Fixture catalog')).toBeVisible();
    await page.goto('/library');
    const libraryLanguage = page.getByLabel('Study language');
    if (await libraryLanguage.inputValue() !== 'de') await libraryLanguage.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    const retiredBookResponse = await page.goto('/books/fixture-metadata-only');
    expect(retiredBookResponse?.status()).toBe(404);
    const retiredCampaignResponse = await page.goto('/campaigns?message=legacy-bookmark');
    expect(retiredCampaignResponse?.status()).toBe(404);
    await page.goto('/reading');
    await expect(page.getByRole('heading', { name: /reading journey/i })).toBeVisible();
    await expect(page.locator('#primary-goal-heading')).toHaveText('Primary Goal');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('Your order');
    await expect(page.locator('#campaign-operations-heading')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Start learning' })).toHaveCount(0);
    await expect(page.getByText(/Der lange Weg nach Hause/).first()).toBeVisible();
    await expect(page.getByText('Your order', { exact: true }).first()).toBeVisible();
    await expect(page.getByText('How coverage is shown', { exact: true })).toBeVisible();
    await expect(page.getByText(/vocabulary-efficient alternative/i)).toHaveCount(0);
    await expect(page.getByText(/advisory order/i)).toHaveCount(0);
    await expect(page.getByText('Route match: familiar German').first()).toBeVisible();
    await expect(page.getByText('Route evidence pending').first()).toBeVisible();
    await page.goto('/journey/fixture-book');
    await expect(page).toHaveURL('/reading#journey-book-fixture-book');
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await page.goto('/vocabulary');
    await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
    await expect(page.locator('form.vocabulary-language-picker')).toHaveCount(0);
    await expect(page.getByText(/Viewing German/)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);
    await expect(page.locator('.table-region')).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL('/vocabulary');
    await expect(page.getByText(/Viewing fr/)).toBeVisible();
    await expect(page.getByText('bonjour')).toHaveCount(0);
    await expect(page.getByRole('button', { name: /import known vocabulary/i })).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByText(/Viewing German/)).toBeVisible();
    await submitKnownVocabularyImport(page);
    await page.goto('/library');
    const primaryNavigation = page.locator('nav.site-header__nav');
    await expect(primaryNavigation.getByRole('link', { name: 'Settings', exact: true })).toHaveCount(0);
    await page.goto('/settings');
    await expect(page).toHaveURL(/\/library/);
    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' })).toBeVisible();
    await expect(page.locator('a[href="/reading#journey-book-fixture-book"]').first()).toBeVisible();
  });

  test('asserts initial HTML before HTMX enhancement and observes status', async ({ page }) => {
    // The import form and its results region are server-rendered only once a
    // study language is selected on the Vocabulary page.
    await page.goto('/vocabulary');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
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

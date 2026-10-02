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
    if (arrivedNewLanguage !== initialNewLanguage) {
      await expect(page.getByLabel('Study language').locator(`option[value="${initialNewLanguage}"]`)).not.toContainText('(new)');
    }
    await expect(page.getByLabel('Study language')).toHaveValue('de');

    await switcher.selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading');
    await expect(page.getByLabel('Study language')).toHaveValue('it');
    await expect(page.locator('main h1')).toContainText(/Reading in Italian|Choose your next book in Italian/);
    await expect(page.getByText('different study language', { exact: false })).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.getByRole('heading', { name: 'Choose your next book in fr', exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'No To Read books yet', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Browse My Books', exact: true })).toBeVisible();
    await page.getByLabel('Study language').selectOption('de');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.locator('main h1')).toContainText(/Reading in German|Choose your next book in German/);

  });

  test('retired Journey endpoints are unavailable', async ({ page }) => {
    const route = await page.goto('/journey/fixture-book');
    expect(route?.status()).toBe(404);
  });

  test('Concordance lookup is server-rendered and submits without JavaScript', async ({ browser }) => {
    const noScript = await browser.newPage({
      baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099',
      javaScriptEnabled: false,
      // A 320 CSS-pixel viewport approximates 400% zoom on a 1280px desktop.
      viewport: { width: 320, height: 812 },
    });
    try {
      await signIn(noScript);
      await noScript.goto('/vocabulary/concordance');
      await expect(noScript.getByRole('heading', { name: 'Vocabulary · Concordance', exact: true })).toBeVisible();
      await expect(noScript.getByLabel('Lookup evidence')).toHaveValue('surface');
      for (const disclosure of ['Books (applied: all current Books)', 'Grammar (applied: no grammar filter)']) {
        const summary = noScript.getByText(disclosure, { exact: true });
        await summary.focus();
        await noScript.keyboard.press('Enter');
        await expect(summary.locator('..')).toHaveAttribute('open', '');
        await expect(summary).toBeFocused();
      }
      await noScript.getByLabel('Exact term').fill('Haus');
      await noScript.getByRole('button', { name: 'Find', exact: true }).click();
      await expect(noScript).toHaveURL(/mode=surface.*term=Haus/);
      await expect(noScript.getByRole('heading', { name: 'Current results' })).toBeVisible();
      const rows = noScript.locator('details.concordance-row');
      await expect(rows).toHaveCount(3);
      await expect(rows.nth(0)).toContainText('Corrected evidence');
      await expect(rows.nth(0)).toContainText('corrected for this occurrence');
      await expect(rows.nth(1)).toContainText('Excluded evidence');
      await expect(rows.nth(1)).toContainText('excluded from effective vocabulary');
      await expect(rows.nth(2)).toContainText('Unchanged evidence');
      await rows.nth(2).locator('summary').focus();
      await noScript.keyboard.press('Enter');
      await expect(rows.nth(2)).toHaveAttribute('open', '');
      await expect(rows.nth(2).getByText('Das Haus sieht gut aus.', { exact: true })).toBeVisible();
      await rows.nth(0).locator('..').getByRole('link', { name: 'Study this sentence and its syntax' }).click();
      await expect(noScript.getByRole('heading', { name: 'Study this sentence and its syntax' })).toBeVisible();
      await expect(noScript.locator('.sentence-study-text')).toContainText('Das Haus sieht gut aus.');
      await expect(noScript.locator('.sentence-study-tokens')).toContainText('Corrected for this occurrence');
      await noScript.getByRole('link', { name: 'Return to Concordance results' }).click();
      await expect(noScript).toHaveURL(/\/vocabulary\/concordance\?.*#occurrence-fixture-concordance-book-0-1$/);
      await expect(noScript.getByRole('heading', { name: 'Current results' })).toBeVisible();
      await expect.poll(() => noScript.evaluate(() => document.activeElement?.id)).toBe('occurrence-fixture-concordance-book-0-1');
      const overflow = await noScript.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(overflow).toBe(false);
    } finally {
      await noScript.close();
    }
  });

  test('Concordance keeps its applied results labeled while an enhanced lookup is pending', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
    await expect(page.getByRole('heading', { name: 'Current results' })).toBeVisible();
    await page.evaluate(() => {
      const nativeFetch = window.fetch.bind(window);
      window.fetch = (...args) => new Promise((resolve, reject) => {
        window.setTimeout(() => nativeFetch(...args).then(resolve, reject), 1300);
      });
    });
    await page.getByLabel('Exact term').fill('Haus2');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await expect(page.locator('#concordance-request-status')).toContainText('previous results remain under their previously applied query and scope');
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
    await expect(page.locator('#concordance-results')).not.toContainText('lookup for “Haus2”');
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”', { timeout: 5000 });
    await expect(page).toHaveURL(/term=Haus2/);
    await page.goBack();
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
    await expect(page.locator('#concordance-results')).not.toContainText('lookup for “Haus2”');
    await page.goForward();
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”');
    await page.evaluate(() => {
      const nativeFetch = window.fetch.bind(window);
      window.fetch = (input, init) => {
        const requestURL = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (requestURL.includes('term=Changed')) return Promise.resolve(new Response('', { status: 409 }));
        return nativeFetch(input, init);
      };
    });
    await page.getByLabel('Exact term').fill('Changed');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await expect(page.locator('#concordance-request-status')).toContainText('The analyzed evidence changed');
    const restart = page.locator('#concordance-request-status').getByRole('link', { name: 'Restart from results' });
    await expect(restart).toHaveAttribute('href', /page=1/);
    await expect(restart).not.toHaveAttribute('href', /rev=/);
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”');
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

  test('ready decks show truthful Current reading and Journey membership actions', async ({ page }) => {
    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText('Current Book.', { exact: false })).toBeVisible();
    await expect(page.getByRole('link', { name: 'View current book in Reading' })).toHaveAttribute('href', '/reading#journey-book-fixture-book');
    await expect(page.getByText(/campaign operations/i)).toHaveCount(0);
    const historicalDeckDownload = await page.request.get('/deck-preparations/fixture-preparation/download');
    expect(historicalDeckDownload.status()).toBe(200);
    expect(historicalDeckDownload.headers()['content-disposition']).toContain('Fixture German deck.apkg');

    await page.getByLabel('Study language').selectOption('it');
    await expect(page.getByLabel('Study language')).toHaveValue('it');
    await expect(page).toHaveURL(/\/deck-preparations\/fixture-preparation\/status$/);
    await expect(page.getByRole('link', { name: 'Return to book', exact: true })).toHaveAttribute(
      'href',
      '/reading?language=de&language_handoff_book=fixture-book&language_handoff_language=de',
    );
    await expect(page.getByRole('button', { name: 'Move to To Read' })).toHaveCount(0);
    await page.goto('/deck-preparations/fixture-journey-preparation/status');
    await expect(page.getByText('To Read.', { exact: false })).toBeVisible();
    await expect(page.locator('a[href="/reading#journey-book-fixture-empty"]')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Move to To Read' })).toHaveCount(0);

    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByLabel('Study language')).toHaveValue('de');
    await expect(page).toHaveURL(/\/deck-preparations\/fixture-journey-preparation\/status$/);
    await page.goto('/deck-preparations/fixture-outside-journey-preparation/status');
    const addToJourney = page.getByRole('button', { name: 'Move to To Read' });
    if (await addToJourney.count() > 0) {
      // First project run: the book starts outside the Journey.
      await expect(page.getByText('Not in To Read.', { exact: false })).toBeVisible();
      await addToJourney.click();
    }
    // Idempotent end state for every project run over the shared fixture server:
    // the book is (or just became) To Read and links to its Reading card.
    await expect(page.getByText('To Read.', { exact: false })).toBeVisible();
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
    await expect(page.getByRole('heading', { name: /reading in german/i })).toBeVisible();
    await expect(page.locator('#primary-goal-heading')).toHaveText('Current reading');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('To Read books');
    await expect(page.locator('#campaign-operations-heading')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Start learning' })).toHaveCount(0);
    await expect(page.getByText(/Der lange Weg nach Hause/).first()).toBeVisible();
    await expect(page.getByText('To Read books', { exact: true }).first()).toBeVisible();
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    await expect(page.getByText(/vocabulary-efficient alternative/i)).toHaveCount(0);
    await expect(page.getByText(/advisory order/i)).toHaveCount(0);
    await expect(page.getByText('Route match: familiar German').first()).toBeVisible();
    await expect(page.getByText('Route evidence pending').first()).toBeVisible();
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await page.goto('/vocabulary');
    await expect(page.getByRole('heading', { name: 'Vocabulary · Browse', exact: true })).toBeVisible();
    await expect(page.locator('form.vocabulary-language-picker')).toHaveCount(0);
    await expect(page.getByText(/active study language.*de/)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);
    await expect(page.locator('.table-region')).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL('/vocabulary');
    await expect(page.getByText(/active study language.*fr/)).toBeVisible();
    await expect(page.getByText('bonjour')).toHaveCount(0);
    await expect(page.getByRole('button', { name: /import known vocabulary/i })).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('de');
    await expect(page.getByText(/active study language.*de/)).toBeVisible();
    await page.goto('/vocabulary/import');
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
    await page.goto('/vocabulary/import');
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

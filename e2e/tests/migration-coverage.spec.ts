import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

async function expectPostFormsCarryCSRF(page: Page) {
  const forms = page.locator('form[method="post"]');
  expect(await forms.count()).toBeGreaterThan(0);
  for (let i = 0; i < await forms.count(); i += 1) {
    await expect(forms.nth(i).locator('input[name="csrf_token"]')).toHaveCount(1);
  }
}

test.describe('migration and epistemic regression coverage', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('shows migrated My Books states and server-rendered controls', async ({ page }) => {
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await expect(page.locator('.library-grid').getByText('Metadata-only migration book')).toHaveCount(0);
    await expect(page.getByText('Analysis result ready')).toHaveCount(0);
    await expect(page.getByText('language not chosen')).toHaveCount(0);
    await expectPostFormsCarryCSRF(page);
  });

  test('loads compiled foundations only on owning routes and keeps styles isolated', async ({ page }) => {
    for (const path of ['/library', '/reading', '/catalogs', '/vocabulary', '/vocabulary/import', '/vocabulary/concordance', '/reading/books/fixture-lemma-flag-book/lemma-review?form=Weg', '/jobs']) {
      const response = await page.goto(path);
      expect(response?.ok(), `${path} should render`).toBeTruthy();
      await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
      await expect(page.locator('link[rel="stylesheet"][href="/static/my-books.css"]')).toHaveCount(path === '/library' ? 1 : 0);
      const isVocabulary = path.startsWith('/vocabulary');
      const isLemmaReview = path.startsWith('/reading/books/');
      await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount((path !== '/library' && !isVocabulary) || isLemmaReview ? 1 : 0);
      await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(isVocabulary || isLemmaReview ? 1 : 0);
      await expect(page.locator('link[rel="stylesheet"][href*="pico"]')).toHaveCount(0);
      await expect(page.locator('[class~="outline"], [class~="secondary"], [class~="container"], [class~="grid"]')).toHaveCount(0);
    }

    const stylesheet = await page.request.get('/static/app.css');
    expect(stylesheet.ok()).toBeTruthy();
    const css = await stylesheet.text();
    expect(css).not.toContain('--pico-');
    expect(css).toContain('prefers-reduced-motion');
  });

  test('shows current reading and unordered To Read choices without forecasts', async ({ page }) => {
    await page.goto('/reading');
    await expect(page.locator('#primary-goal-heading')).toHaveText('Current reading');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('To Read books');
    await expect(page.getByRole('heading', { name: 'Campaign history & operations' })).toHaveCount(0);
    await expect(page.getByRole('list', { name: 'To Read books' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    await expect(page.getByText(/vocabulary-efficient alternative/i)).toHaveCount(0);
    await expect(page.getByText(/advisory order/i)).toHaveCount(0);
    await expect(page.locator('#provisional-journey-status')).toHaveAttribute('aria-live', 'polite');
    await expectPostFormsCarryCSRF(page);
    const retiredJourneyRoute = await page.goto('/journey/fixture-book');
    expect(retiredJourneyRoute?.status()).toBe(404);
  });

  test('states the completion consequence and preserves provenance labels', async ({ page }) => {
    await page.goto('/reading');
    const goal = page.locator('#primary-goal-section');
    const disclosure = goal.locator('details').filter({ hasText: 'Mark reading finished' }).first();
    await expect(disclosure).toBeVisible();
    await disclosure.locator('summary').focus();
    await expect(disclosure.locator('summary')).toBeFocused();
    await disclosure.locator('summary').press('Enter');
    await expect(goal).toContainText('Record the reading achievement');
    await expect(goal).toContainText(/accept \d+ currently eligible frozen Reserved identities into Known vocabulary/);
    await expect(goal.locator('form[action="/reading/finish"] input[name="csrf_token"]')).toHaveCount(1);
    await expect(goal.locator('form[action="/reading/finish"] input[name="expected_current_book_id"]')).toHaveCount(1);
    await expect(goal.locator('form[action="/reading/finish"] input[name="expected_current_snapshot_id"]')).toHaveCount(1);

    await page.goto('/vocabulary/import');
    await expect(page.getByRole('heading', { name: 'Vocabulary · Import known vocabulary', exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);
    await expect(page.locator('.table-region')).toHaveCount(0);
    await expect(page.locator('form.vocabulary-language-picker')).toHaveCount(0);
    await expect(page.getByText(/Viewing German/)).toBeVisible();
    await expectPostFormsCarryCSRF(page);
  });
});

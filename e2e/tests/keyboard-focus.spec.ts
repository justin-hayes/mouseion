import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page, disableEnhancement = false) {
  if (disableEnhancement) {
    await page.route('**/static/vendor/htmx-*.js', (route) => route.abort());
  }
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

test.describe.configure({ mode: 'serial' });

test.describe('keyboard, focus, and asynchronous-state acceptance', () => {
  test('skip link is first focusable, moves focus to main, and navigation is keyboard reachable', async ({ page }) => {
    await signIn(page);
    await page.keyboard.press('Tab');
    await expect(page.locator('a.skip-link')).toBeFocused();
    await expect(page.locator('a.skip-link')).toHaveAttribute('href', '#main-content');
    await expect(page.locator('a.skip-link')).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page.locator('main#main-content')).toBeFocused();
    await expect(page.locator('main#main-content')).toHaveAttribute('tabindex', '-1');

    const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
    await expect(navigation).toBeVisible();
    const destinations = ['My Books', 'Reading', 'Vocabulary'];
    for (const name of destinations) {
      const link = navigation.getByRole('link', { name });
      await link.focus();
      await expect(link).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(page).toHaveURL(new RegExp({
        'My Books': '\\/library', Reading: '\\/reading', Vocabulary: '\\/vocabulary',
      }[name]));
      await page.goBack();
      await expect(navigation).toBeVisible();
    }
  });

  test('representative tab order and focus indicators remain visible', async ({ page }) => {
    await signIn(page);
    await page.goto('/vocabulary');
    await page.keyboard.press('Tab');
    await expect(page.locator('a.skip-link')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Mouseion' })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'My Books' })).toBeFocused();
    const focusStyle = await page.evaluate(() => {
      const element = document.activeElement;
      if (!element) return { outline: 'none', width: '0px', boxShadow: 'none' };
      const style = getComputedStyle(element);
      return { outline: style.outlineStyle, width: style.outlineWidth, boxShadow: style.boxShadow };
    });
    expect(focusStyle.outline !== 'none' || focusStyle.width !== '0px' || focusStyle.boxShadow !== 'none').toBeTruthy();
  });

  test('My Books exposes keyboard-reachable identity links and failed-analysis recovery', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    const title = page.locator('.library-grid a.library-book__identity-link[href="/reading#journey-book-fixture-book"]', { hasText: 'Der lange Weg nach Hause' });
    await title.focus();
    await expect(title).toBeFocused();
    const item = title.locator('xpath=ancestor::li');
    const itemFocusStops = item.locator('a, button, summary');
    await expect(itemFocusStops.first()).toHaveClass(/library-book__identity-link/);
    await expect(itemFocusStops.nth(1)).toHaveText(/View in Reading/);
    await expect(itemFocusStops.nth(2)).toHaveText('More actions');
    await expect(page.locator('.library-grid').getByText('Review failed analysis')).toHaveCount(0);
  });

  test('Reading Journey keeps goal-first keyboard order and announces feedback', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading?message=Journey%20updated');
    const goalHeading = page.locator('#primary-goal-heading');
    const provisionalHeading = page.locator('#provisional-journey-heading');
    await expect(goalHeading).toBeVisible();
    await expect(page.getByRole('status')).toContainText('Journey updated');
    expect(await goalHeading.evaluate((node) => node.compareDocumentPosition(document.querySelector('#provisional-journey-heading')!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy();
    const goalLink = page.locator('.journey-book--goal a').first();
    await goalLink.focus();
    await expect(goalLink).toBeFocused();
  });

  test('unassessed books have no detail page or standalone analysis action', async ({ page }) => {
    await signIn(page);
    const response = await page.goto('/books/fixture-empty');
    expect(response?.status()).toBe(404);
    await page.goto('/reading');
    await expect(page.getByRole('button', { name: 'Start analysis' })).toHaveCount(0);
  });

  test('book page leads to preparation and terminal polling stops', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    await expect(page.getByRole('heading', { name: 'Deck preparation task' })).toBeVisible();
    await expect(page.locator('input[name="external_translation_consent"]')).toHaveCount(0);
    await expect(page.getByText(/uses the configured translation provider/i)).toBeVisible();
    const preparation = page.locator('[data-deck-preparation]');
    await expect(preparation).toHaveAttribute('role', 'status');
    await expect(preparation).toHaveAttribute('aria-live', 'polite');
    await expect(preparation).toHaveAttribute('aria-atomic', 'true');

    let polls = 0;
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/status', (route) => {
      polls += 1;
      if (route.request().headers()['accept'] === 'text/html') {
        return route.fulfill({ contentType: 'text/html', body: '<section id="deck-preparation-status" data-deck-preparation><h3>Deck ready</h3><a download href="/download">Download deck</a><div><p>Current Book. This deck is preparation for the Book you are reading now.</p></div></section>' });
      }
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({
        state: 'ready', progress: 100, ready: true, deck_name: 'Fixture German deck', filename: 'fixture.apkg',
        download_url: '/deck-preparations/fixture-preparation/download', completeness: { total_cards: 3 },
      }) });
    });
    const prepare = page.getByRole('button', { name: 'Prepare deck' });
    await prepare.focus();
    await prepare.press('Enter');
    await expect(preparation).toContainText('Deck ready');
    await expect(preparation.getByRole('button', { name: /cancel/i })).toHaveCount(0);
    await expect(preparation.locator('[aria-busy="true"]')).toHaveCount(0);
    // Terminal ready state performs exactly one JSON poll plus the server-rendered
    // ready fragment fetch; polling must not continue afterwards.
    await expect.poll(() => polls).toBe(2);
    await page.waitForTimeout(1600);
    expect(polls).toBe(2);
  });

  test('provider outage leaves an actionable failure without blocking Reading', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    await expect(page.locator('input[name="external_translation_consent"]')).toHaveCount(0);
    await page.route('**/reading/books/fixture-route-match/deck/preparations', async (route) => {
      expect(route.request().postData() ?? '').not.toContain('external_translation_consent');
      await route.fulfill({ status: 303, headers: { location: '/deck-preparations/fixture-provider-outage/status' } });
    });
    await page.route('**/deck-preparations/fixture-provider-outage/status', (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        id: 'fixture-provider-outage', state: 'failed', progress: 100, ready: false,
        error: 'Contextual translation is required for every new deck. Configure the translation provider, then retry; no local-only deck was published.',
      }),
    }));

    const status = page.locator('[data-deck-preparation]');
    await page.getByRole('button', { name: 'Prepare deck' }).click();
    await expect(status).toContainText('Deck preparation failed');
    await expect(status).toContainText('no local-only deck was published');
    await expect(status.getByRole('button', { name: 'Retry preparation' })).toBeVisible();
    await expect(status.getByRole('link', { name: /download/i })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Back to this Book in Reading' })).toBeVisible();
  });

  test('preparation cancel and retry are keyboard-operable and terminal state removes polling controls', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    let state = 'queued';
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/status', (route) => {
      if (route.request().headers()['accept'] === 'text/html') {
        return route.fulfill({ contentType: 'text/html', body: '<section id="deck-preparation-status" data-deck-preparation><h3>Deck ready</h3><a download href="/download">Download deck</a><div><p>Current Book. This deck is preparation for the Book you are reading now.</p></div></section>' });
      }
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ state, progress: state === 'queued' ? 1 : 100, ready: state === 'ready', deck_name: 'Fixture German deck', download_url: '/download' }) });
    });
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/cancel', (route) => { state = 'cancelled'; return route.fulfill({ contentType: 'application/json', body: '{}' }); });
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/retry', (route) => { state = 'ready'; return route.fulfill({ contentType: 'application/json', body: '{}' }); });
    const status = page.locator('[data-deck-preparation]');
    await page.getByRole('button', { name: 'Prepare deck' }).press('Enter');
    const cancel = status.getByRole('button', { name: 'Cancel preparation' });
    await expect(cancel).toBeVisible();
    await cancel.press('Enter');
    await expect(status).toContainText('Deck preparation cancelled');
    const retry = status.getByRole('button', { name: 'Retry preparation' });
    await retry.press('Enter');
    await expect(status).toContainText('Deck ready');
    await expect(status.getByRole('button', { name: /cancel|retry/i })).toHaveCount(0);
  });

  test('My Books and table region are keyboard-scrollable', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await page.goto('/jobs');
    const region = page.getByRole('region', { name: 'Analysis history' });
    await expect(region).toHaveAttribute('tabindex', '0');
    await expect(region).toHaveAttribute('aria-label', 'Analysis history');
    await region.focus();
    await expect(region).toBeFocused();
  });

  test('known-vocabulary import works with enhancement disabled and enabled', async ({ page }) => {
    for (const disabled of [true, false]) {
      await signIn(page, disabled);
      await page.goto('/vocabulary/import');
      const switcher = page.getByLabel('Study language');
      if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
      const form = page.locator('form[hx-post*="/vocabulary/import"]');
      await form.locator('input[type="file"]').setInputFiles({ name: 'known.txt', mimeType: 'text/plain', buffer: Buffer.from('Haus\nÜberraschung\n') });
      await form.getByRole('button', { name: /import known vocabulary/i }).press('Enter');
      if (disabled) {
        await expect(page).toHaveURL(/\/vocabulary\/imports\/7\/status/);
        await expect(page.getByRole('status')).toContainText(/complete|queued/i);
      } else {
        await expect(page).toHaveURL(/\/vocabulary/);
        await expect(page.locator('#vocabulary-results')).toContainText(/queued|complete/i);
      }
      if (disabled) await page.unroute('**/static/vendor/htmx-*.js');
      await page.context().clearCookies();
    }
  });

  test('failed analysis exposes keyboard-operable recovery and intentional error focus pattern', async ({ page }) => {
    await signIn(page);
    await page.goto('/jobs/43');
    const error = page.getByRole('alert').filter({ hasText: 'Analysis needs attention' });
    await expect(error).toBeVisible();
    const retry = page.getByRole('button', { name: 'Retry analysis' });
    await expect(retry).toBeVisible();
    await retry.focus();
    await expect(retry).toBeFocused();
    await retry.press('Enter');
    await expect(page).toHaveURL(/\/jobs\/43\?message=/);
  });
});

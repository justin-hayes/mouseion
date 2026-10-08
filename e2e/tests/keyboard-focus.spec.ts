import { expect, Page, test } from '../support/test';

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

async function switchCurrentReadingTo(page: Page, title: string) {
  await page.goto('/reading/switch');
  const candidate = page.locator('li.reading-chooser-book').filter({ hasText: title });
  await candidate.locator('details').filter({ hasText: 'Switch to this book' }).locator('summary').click();
  await candidate.getByRole('button', { name: 'Confirm switch to this book' }).click();
  await expect(page).toHaveURL(/\/reading\?message=/);
}

async function openGermanReading(page: Page) {
  await page.goto('/library');
  const language = page.getByLabel('Study language');
  if (await language.inputValue() !== 'de') await language.selectOption('de');
  await page.goto('/reading');
}

async function switchToRouteMatch(page: Page) {
  await openGermanReading(page);
  const current = page.locator('.journey-book--goal');
  const currentID = await current.count() ? await current.getAttribute('id') : null;
  if (!currentID) {
    const candidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Route match: familiar German' });
    const start = candidate.locator('details').filter({ hasText: 'Start reading' });
    await start.locator('summary').click();
    await start.getByRole('button', { name: 'Confirm start reading' }).click();
    await expect(page).toHaveURL(/\/reading\?message=/);
    return;
  }
  if (currentID === 'journey-book-fixture-route-match') {
    await switchCurrentReadingTo(page, 'Der lange Weg nach Hause');
  }
  await switchCurrentReadingTo(page, 'Route match: familiar German');
}

async function restoreFixtureBookCurrent(page: Page) {
  await openGermanReading(page);
  const current = page.locator('.journey-book--goal');
  const currentID = await current.count() ? await current.getAttribute('id') : null;
  if (currentID === 'journey-book-fixture-book') return;
  if (currentID) {
    await switchCurrentReadingTo(page, 'Der lange Weg nach Hause');
    return;
  }
  const candidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Der lange Weg nach Hause' });
  const start = candidate.locator('details').filter({ hasText: 'Start reading' });
  await start.locator('summary').click();
  await start.getByRole('button', { name: 'Confirm start reading' }).click();
  await expect(page).toHaveURL(/\/reading\?message=/);
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
    for (const name of ['My Books', 'Reading', 'Vocabulary', 'Catalogs']) {
      await page.keyboard.press('Tab');
      await expect(page.getByRole('link', { name, exact: true })).toBeFocused();
    }
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Study language')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('.site-header__account summary')).toBeFocused();
    const focusStyle = await page.evaluate(() => {
      const element = document.activeElement;
      if (!element) return { outline: 'none', width: '0px', boxShadow: 'none' };
      const style = getComputedStyle(element);
      return { outline: style.outlineStyle, width: style.outlineWidth, boxShadow: style.boxShadow };
    });
    expect(focusStyle.outline !== 'none' || focusStyle.width !== '0px' || focusStyle.boxShadow !== 'none').toBeTruthy();

    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/library');
    await page.keyboard.press('Tab');
    await expect(page.locator('.skip-link')).toBeFocused();
    await page.keyboard.press('Tab');
    for (const name of ['Mouseion', 'My Books', 'Reading', 'Vocabulary', 'Catalogs']) {
      await expect(page.getByRole('link', { name, exact: true })).toBeFocused();
      if (name !== 'Catalogs') await page.keyboard.press('Tab');
    }
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Study language')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('.site-header__account summary')).toBeFocused();
  });

  test('My Books exposes keyboard-reachable identity links and failed-analysis recovery', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    const title = page.locator('.library-books a.library-book__identity-link[href="/reading#journey-book-fixture-book"]', { hasText: 'Der lange Weg nach Hause' });
    await title.focus();
    await expect(title).toBeFocused();
    const item = title.locator('xpath=ancestor::li');
    const itemFocusStops = item.locator('a, button, summary');
    await expect(itemFocusStops.first()).toHaveClass(/library-book__identity-link/);
    await expect(itemFocusStops.nth(1)).toHaveText(/View in Reading/);
    await expect(itemFocusStops.nth(2)).toHaveText('More actions');
    await expect(page.locator('.library-books').getByText('Review failed analysis')).toHaveCount(0);
  });

  test('Reading keeps goal-first keyboard order and announces feedback', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading?message=Journey%20updated');
    const goalHeading = page.locator('.journey-book__title');
    await expect(goalHeading).toBeVisible();
    await expect(page.getByRole('status')).toContainText('Journey updated');
    // The current Book precedes the Vocabulary Browse; no Other To Read section is rendered.
    await expect(page.locator('#provisional-journey-heading')).toHaveCount(0);
    expect(await goalHeading.evaluate((node) => node.compareDocumentPosition(document.querySelector('#vocabulary-workflow')!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy();
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

  test('focused Book preparation opens only while its snapshot is Current reading', async ({ page }) => {
    await signIn(page);
    await restoreFixtureBookCurrent(page);
    const beforeReading = await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    expect(beforeReading?.status()).toBe(404);

    await switchToRouteMatch(page);
    const whileReading = await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    expect(whileReading?.status()).toBe(200);
    await expect(page.getByRole('heading', { name: 'Deck preparation task' })).toBeVisible();
    await restoreFixtureBookCurrent(page);
  });

  test('book page leads to preparation and terminal polling stops', async ({ page }) => {
    await signIn(page);
    await switchToRouteMatch(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    await expect(page.getByRole('heading', { name: 'Deck preparation task' })).toBeVisible();
    await expect(page.locator('input[name="external_translation_consent"]')).toHaveCount(0);
    await expect(page.getByText(/this exact reading snapshot remains unchanged/i)).toBeVisible();
    const prepareForm = page.locator('form.focused-deck-form');
    await expect(prepareForm.getByRole('button', { name: 'Prepare deck' })).toBeVisible();
    expect(await prepareForm.evaluate(node => Number.parseFloat(getComputedStyle(node).paddingInlineStart))).toBeGreaterThanOrEqual(16);
    expect(await prepareForm.evaluate(node => Number.parseFloat(getComputedStyle(node).rowGap))).toBeGreaterThan(0);
    const preparation = page.locator('[data-deck-preparation]');
    await expect(preparation).toHaveAttribute('role', 'status');
    await expect(preparation).toHaveAttribute('aria-live', 'polite');
    await expect(preparation).toHaveAttribute('aria-atomic', 'true');

    let polls = 0;
    await page.route('**/deck-preparations/*/status', (route) => {
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
    await restoreFixtureBookCurrent(page);
  });

  test('provider outage leaves an actionable failure without blocking Reading', async ({ page }) => {
    await signIn(page);
    await switchToRouteMatch(page);
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
    await restoreFixtureBookCurrent(page);
  });

  test('preparation cancel and retry are keyboard-operable and terminal state removes polling controls', async ({ page }) => {
    await signIn(page);
    await switchToRouteMatch(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    let state = 'queued';
    let releaseCancel!: () => void;
    const cancelGate = new Promise<void>((resolve) => { releaseCancel = resolve; });
    await page.route('**/deck-preparations/*/status', (route) => {
      if (route.request().headers()['accept'] === 'text/html') {
        return route.fulfill({ contentType: 'text/html', body: '<section id="deck-preparation-status" data-deck-preparation><h3>Deck ready</h3><a download href="/download">Download deck</a><div><p>Current Book. This deck is preparation for the Book you are reading now.</p></div></section>' });
      }
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ state, progress: state === 'queued' ? 1 : 100, ready: state === 'ready', deck_name: 'Fixture German deck', download_url: '/download' }) });
    });
    await page.route('**/deck-preparations/*/cancel', async (route) => {
      await cancelGate;
      state = 'cancelled';
      await route.fulfill({ contentType: 'application/json', body: '{}' });
    });
    await page.route('**/deck-preparations/*/retry', (route) => { state = 'ready'; return route.fulfill({ contentType: 'application/json', body: '{}' }); });
    const status = page.locator('[data-deck-preparation]');
    await page.getByRole('button', { name: 'Prepare deck' }).press('Enter');
    const cancel = status.getByRole('button', { name: 'Cancel preparation' });
    await expect(cancel).toBeVisible();
    await cancel.press('Enter');
    const cancelPending = status.getByRole('button', { name: 'Canceling deck preparation…' });
    await expect(cancelPending).toBeDisabled();
    await expect(status.getByRole('status')).toHaveText('Canceling deck preparation…');
    releaseCancel();
    await expect(status).toContainText('Deck preparation cancelled');
    const retry = status.getByRole('button', { name: 'Retry preparation' });
    await retry.press('Enter');
    await expect(status).toContainText('Deck ready');
    await expect(status.getByRole('button', { name: /cancel|retry/i })).toHaveCount(0);
    await restoreFixtureBookCurrent(page);
  });

  test('My Books and table region are keyboard-scrollable', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
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
      await expect(form.locator('input[type="file"]')).toHaveAttribute('id', 'known-vocabulary-file');
      await expect(form.getByRole('button', { name: /import known vocabulary/i })).toHaveClass(/\bbtn\b/);
      await form.locator('input[type="file"]').setInputFiles({ name: 'known.txt', mimeType: 'text/plain', buffer: Buffer.from('Haus\nÜberraschung\nbad\tline\n') });
      await form.getByRole('button', { name: /import known vocabulary/i }).press('Enter');
      if (disabled) {
        await expect(page).toHaveURL(/\/vocabulary\/imports\/8\/status/);
        await expect(page.locator('body')).toHaveClass('vocabulary-shell');
        await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
        await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
        await expect(page.getByRole('status')).toContainText(/complete|queued/i);
      } else {
        await expect(page).toHaveURL(/\/vocabulary/);
        await expect(page.locator('#vocabulary-results')).toContainText(/queued|complete/i);
      }
      await expect(page.getByRole('region', { name: 'Rejected vocabulary rows' })).toContainText('expected exactly one lemma');
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
    await expect(retry).toHaveClass(/\bbutton\b/);
    await expect(retry.locator('xpath=ancestor::form')).toHaveAttribute('method', 'post');
    await expect(retry.locator('xpath=ancestor::form')).toHaveAttribute('action', '/jobs/43/retry');
    await expect(retry).toBeVisible();
    await retry.focus();
    await expect(retry).toBeFocused();
    await retry.press('Enter');
    await expect(page).toHaveURL(/\/jobs\/43\?message=/);
  });
});

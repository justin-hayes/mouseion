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

  test('reordering a Journey recalculates downstream forecast coverage', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture journey runs once per browser suite.');
    await signIn(page, true);
    await page.goto('/reading');
    const downstream = page.locator('#journey-book-fixture-route-match');
    await expect(downstream.getByRole('region', { name: 'Reading coverage forecast' })).toBeVisible();
    await page.locator('#journey-book-fixture-route-differs').getByRole('button', { name: /Move .* earlier/ }).press('Enter');
    await expect(page).toHaveURL(/\/reading\?message=/);
    await expect(page.getByText(/Coverage forecast recalculated for the saved order/)).toBeVisible();
    const after = await page.locator('#journey-book-fixture-route-match').getByRole('region', { name: 'Reading coverage forecast' }).innerText();
    expect(after).toContain('On arrival coverage');
    await expect(page.locator('#journey-book-fixture-route-match')).toContainText('On arrival');
  });

  test('enhanced reorder announces recalculation, blocks duplicate activation, and restores focus', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture journey runs once per browser suite.');
    await signIn(page);
    await page.goto('/reading');
    const book = page.locator('#journey-book-fixture-route-differs');
    const move = book.getByRole('button', { name: /Move .* earlier/ });
    let requests = 0;
    await page.route('**/reading/entries/fixture-route-differs/move-earlier', async (route) => {
      requests += 1;
      await new Promise((resolve) => setTimeout(resolve, 500));
      await route.continue();
    });

    await move.click();
    await expect(page.locator('#provisional-journey-content')).toHaveAttribute('aria-busy', 'true');
    await expect(page.locator('#provisional-journey-status')).toHaveText('Updating Reading order...');
    await expect(move).toBeDisabled();
    const duplicatePrevented = await move.locator('xpath=ancestor::form').evaluate((form) => {
      const event = new Event('submit', { bubbles: true, cancelable: true });
      return !form.dispatchEvent(event) && event.defaultPrevented;
    });
    expect(duplicatePrevented).toBeTruthy();
    await expect.poll(() => requests).toBe(1);
    await expect(page.locator('#provisional-journey-status')).toContainText(/Moved .* in To Read books/);
    await expect(page.locator('#provisional-journey-content')).not.toHaveAttribute('aria-busy', 'true');
    await expect(page.locator('#journey-book-fixture-route-differs')).toBeFocused();

    await page.route('**/reading/entries/fixture-route-differs/move-later', async (route) => {
      const response = await route.fetch();
      const body = await response.text();
      await route.fulfill({ response, body: body.replace('Coverage forecast recalculated for the saved order.', 'Coverage forecast unavailable; the saved order remains in place. Retry Reading Journey.') });
    });
    await page.locator('#journey-book-fixture-route-differs').getByRole('button', { name: /Move .* later/ }).click();
    await expect(page.locator('#provisional-journey-status')).toContainText(/saved order remains in place/);
    await expect(page.locator('#provisional-journey-status')).toContainText(/unavailable/);
    await expect(page.getByRole('alert')).toHaveCount(0);
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
    const preparation = page.locator('[data-deck-preparation]');
    await expect(preparation).toHaveAttribute('role', 'status');
    await expect(preparation).toHaveAttribute('aria-live', 'polite');
    await expect(preparation).toHaveAttribute('aria-atomic', 'true');

    let polls = 0;
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/status', (route) => {
      polls += 1;
      if (route.request().headers()['hx-request'] === 'true') {
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

  test('preparation cancel and retry are keyboard-operable and terminal state removes polling controls', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    let state = 'queued';
    await page.route('**/deck-preparations/fixture-submitted-fixture-route-match-run/status', (route) => {
      if (route.request().headers()['hx-request'] === 'true') {
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

  test('Journey reorder controls work without JavaScript and retain focus with HTMX', async ({ page, browser }) => {
    test.skip(true, 'The former Journey screen is no longer a primary destination; retired form endpoints are covered by server tests.');
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture journey runs once per browser suite.');
    await signIn(page, true);
    const journeyAddSideEffects: string[] = [];
    page.on('request', request => {
      const pathname = new URL(request.url()).pathname;
      if (pathname.endsWith('/add')) journeyAddSideEffects.push(pathname);
    });
    await page.getByLabel('Study language').selectOption('it');
    await expect(page.getByLabel('Study language')).toHaveValue('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading');
    await expect(page.locator('.journey-book--goal .journey-book__controls')).toHaveCount(0);
    const edge = page.locator('#journey-book-fixture-edge-content');
    await edge.getByRole('button', { name: /Move .* earlier/ }).press('Enter');
    await expect(page).toHaveURL(/\/reading\?message=/);
    const reordered = page.locator('#provisional-journey-list .journey-list > li > article');
    await expect(reordered.first()).toHaveAttribute('id', 'journey-book-fixture-edge-content');

    await page.unroute('**/static/vendor/htmx-*.js');
    await page.goto('/reading');
    const empty = page.locator('#journey-book-fixture-empty');
    await empty.getByRole('button', { name: /Move .* earlier/ }).press('Enter');
    await expect(page.locator('#provisional-journey-status')).toContainText(/Moved .* position .* in To Read books/);
    await expect(page.locator('#journey-book-fixture-empty')).toBeFocused();
    await expect(reordered.first()).toHaveAttribute('id', 'journey-book-fixture-empty');

    await page.setViewportSize({ width: 375, height: 667 });
    await page.reload();
    expect(journeyAddSideEffects).toEqual([]);
    for (const id of ['fixture-empty', 'fixture-edge-content']) {
      const book = page.locator(`#journey-book-${id}`);
      await expect(book.locator('.journey-book__controls')).toBeVisible();
      expect(await book.locator('.journey-book__controls').evaluate((node) => node.parentElement?.lastElementChild === node)).toBeTruthy();
    }

    const noJavaScriptContext = await browser.newContext({ baseURL: new URL(page.url()).origin, javaScriptEnabled: false });
    try {
      const noJavaScriptPage = await noJavaScriptContext.newPage();
      let nativeMoveRequest = false;
      noJavaScriptPage.on('request', (request) => {
        if (request.url().includes('/reading/entries/') && request.url().includes('/move-')) {
          nativeMoveRequest = request.headers()['hx-request'] !== 'true';
        }
      });
      await signIn(noJavaScriptPage);
      await noJavaScriptPage.goto('/journey');
      await noJavaScriptPage.locator('form[data-journey-reorder] button:not([disabled])').first().press('Enter');
      await expect(noJavaScriptPage).toHaveURL(/\/reading\?message=/);
      await expect.poll(() => nativeMoveRequest).toBeTruthy();
      await expect(noJavaScriptPage.getByRole('heading', { name: /Reading/ }).first()).toBeVisible();
    } finally {
      await noJavaScriptContext.close();
    }
  });

  test('known-vocabulary import works with enhancement disabled and enabled', async ({ page }) => {
    for (const disabled of [true, false]) {
      await signIn(page, disabled);
      await page.goto('/vocabulary');
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

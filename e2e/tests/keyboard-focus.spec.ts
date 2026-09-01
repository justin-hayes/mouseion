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
    const destinations = ['My Books', 'Reading Journey', 'Add books', 'Settings'];
    for (const name of destinations) {
      const link = navigation.getByRole('link', { name });
      await link.focus();
      await expect(link).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(page).toHaveURL(new RegExp({
        'My Books': '\\/library', 'Reading Journey': '\\/journey', 'Add books': '\\/connections', Settings: '\\/settings',
      }[name]));
      await page.goBack();
      await expect(navigation).toBeVisible();
    }
  });

  test('representative tab order and focus indicators remain visible', async ({ page }) => {
    await signIn(page);
    await page.goto('/settings?language=de');
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
    await expect(page.getByRole('heading', { name: 'My Books', exact: true })).toBeVisible();
    const title = page.locator('a[href="/books/fixture-book"]', { hasText: 'Der lange Weg nach Hause' });
    await title.focus();
    await expect(title).toBeFocused();
    await expect(page.locator('a[href="/jobs/43"]', { hasText: 'Review failed analysis' })).toBeVisible();
  });

  test('Reading Journey keeps goal-first keyboard order and announces feedback', async ({ page }) => {
    await signIn(page);
    await page.goto('/journey?message=Journey%20updated');
    const goalHeading = page.locator('#primary-goal-heading');
    const provisionalHeading = page.locator('#provisional-journey-heading');
    const operationsHeading = page.locator('#campaign-operations-heading');
    await expect(goalHeading).toBeVisible();
    await expect(page.getByRole('status')).toContainText('Journey updated');
    expect(await goalHeading.evaluate((node) => node.compareDocumentPosition(document.querySelector('#provisional-journey-heading')!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy();
    expect(await provisionalHeading.evaluate((node) => node.compareDocumentPosition(document.querySelector('#campaign-operations-heading')!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy();
    const goalLink = page.locator('.journey-book--goal a').first();
    await goalLink.focus();
    await expect(goalLink).toBeFocused();
  });

  test('scope confirmation is keyboard-only, returns to the book, and does not start analysis', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book/scope');
    await expect(page.getByRole('heading', { name: 'Review analysis scope' })).toBeVisible();
    const confirm = page.getByRole('button', { name: 'Confirm scope' });
    await confirm.focus();
    await expect(confirm).toBeFocused();
    await confirm.press('Enter');
    await expect(page).toHaveURL(/\/books\/fixture-book\?message=/);
    await expect(page.getByRole('heading', { name: 'Der lange Weg nach Hause' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'View analysis result' }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Start analysis' })).toHaveCount(0);
    await expect(page.getByText('Analysis scope saved')).toBeVisible();
  });

  test('exact result leads to preparation and terminal polling stops', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book/analyses/fixture-run');
    await expect(page.getByRole('heading', { name: 'Analysis result' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Prepare deck' })).toBeVisible();
    const preparation = page.locator('[data-deck-preparation]');
    await expect(preparation).toHaveAttribute('role', 'status');
    await expect(preparation).toHaveAttribute('aria-live', 'polite');
    await expect(preparation).toHaveAttribute('aria-atomic', 'true');

    let polls = 0;
    await page.route('**/deck-preparations/fixture-preparation/status', (route) => {
      polls += 1;
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
    await expect.poll(() => polls).toBe(1);
  });

  test('preparation cancel and retry are keyboard-operable and terminal state removes polling controls', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book/analyses/fixture-run');
    let state = 'queued';
    await page.route('**/deck-preparations/fixture-preparation/status', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ state, progress: state === 'queued' ? 1 : 100, ready: state === 'ready', deck_name: 'Fixture German deck', download_url: '/download' }) }));
    await page.route('**/deck-preparations/fixture-preparation/cancel', (route) => { state = 'cancelled'; return route.fulfill({ contentType: 'application/json', body: '{}' }); });
    await page.route('**/deck-preparations/fixture-preparation/retry', (route) => { state = 'ready'; return route.fulfill({ contentType: 'application/json', body: '{}' }); });
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

  test('catalog acquisition updates in place without stealing focus and table region is keyboard-scrollable', async ({ page }) => {
    await signIn(page);
    await page.goto('/opds/browse?connection=fixture-connection&language=de');
    const entry = page.getByRole('heading', { name: 'Ein deutsches Buch' }).locator('..');
    const add = entry.getByRole('button', { name: 'Add to My Books' });
    await add.focus();
    await add.press('Enter');
    await expect(entry).toContainText('Added to My Books');
    await expect(page.getByRole('button', { name: 'Add to My Books' })).toHaveCount(0);

    await page.goto('/jobs');
    const region = page.getByRole('region', { name: 'Analysis history' });
    await expect(region).toHaveAttribute('tabindex', '0');
    await expect(region).toHaveAttribute('aria-label', 'Analysis history');
    await region.focus();
    await expect(region).toBeFocused();
  });

  test('Journey reorder controls work without JavaScript and retain focus with HTMX', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture journey runs once per browser suite.');
    await signIn(page, true);
    await page.goto('/journey');
    await expect(page.locator('.journey-book--goal .journey-book__controls')).toHaveCount(0);
    const edge = page.locator('#journey-book-fixture-edge-content');
    await edge.getByRole('button', { name: /Move .* earlier/ }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    const reordered = page.locator('#provisional-journey-list .journey-list > article');
    await expect(reordered.first()).toHaveAttribute('id', 'journey-book-fixture-edge-content');

    await page.unroute('**/static/vendor/htmx-*.js');
    await page.goto('/journey');
    const empty = page.locator('#journey-book-fixture-empty');
    await empty.getByRole('button', { name: /Move .* earlier/ }).press('Enter');
    await expect(page.locator('#provisional-journey-status')).toContainText(/Moved .* provisional position/);
    await expect(page.locator('#journey-book-fixture-empty')).toBeFocused();
    await expect(reordered.first()).toHaveAttribute('id', 'journey-book-fixture-empty');

    await page.setViewportSize({ width: 375, height: 667 });
    await page.reload();
    for (const id of ['fixture-empty', 'fixture-edge-content']) {
      const book = page.locator(`#journey-book-${id}`);
      await expect(book.locator('.journey-book__controls')).toBeVisible();
      expect(await book.locator('.journey-book__controls').evaluate((node) => node.parentElement?.lastElementChild === node)).toBeTruthy();
    }
  });

  test('learning completion and abandonment confirmations support cancel/confirm focus return', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture journey runs once per browser suite.');
    await signIn(page, true);
    await page.goto('/journey');
    const active = page.locator('#campaign-fixture-campaign');
    const finishBook = active.getByRole('button', { name: /Mark book finished/ });
    await finishBook.press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    const completionDisclosure = page.locator('#campaign-fixture-campaign details').filter({ hasText: /Complete campaign/ }).first();
    await expect(completionDisclosure).toBeVisible();
    await completionDisclosure.locator('summary').press('Enter');
    await expect(completionDisclosure).toHaveAttribute('open', '');
    await completionDisclosure.locator('summary').press('Enter');
    await expect(completionDisclosure.locator('summary')).toBeFocused();
    await completionDisclosure.locator('summary').press('Enter');
    await expect(completionDisclosure).toHaveAttribute('open', '');
    await completionDisclosure.getByRole('button', { name: /Complete campaign/ }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.getByText('Campaign complete')).toBeVisible();

    const abandonment = page.locator('#campaign-fixture-queued-campaign');
    const abandonSummary = abandonment.locator('summary', { hasText: 'Abandon campaign' });
    await abandonSummary.press('Enter');
    await expect(abandonSummary).toBeFocused();
    const abandonDisclosure = abandonment.locator('details').filter({ hasText: 'Abandon campaign' }).first();
    await abandonSummary.press('Enter');
    await expect(abandonSummary).toBeFocused();
    await abandonSummary.press('Enter');
    await expect(abandonDisclosure).toHaveAttribute('open', '');
    await abandonDisclosure.getByRole('button', { name: 'Confirm abandonment' }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.getByText('Campaign abandoned')).toBeVisible();
  });

  test('known-vocabulary import works with enhancement disabled and enabled', async ({ page }) => {
    for (const disabled of [true, false]) {
      await signIn(page, disabled);
      await page.goto('/settings?language=de');
      const form = page.locator('form[hx-post*="/known-vocab/import"]');
      await form.locator('input[type="file"]').setInputFiles({ name: 'known.txt', mimeType: 'text/plain', buffer: Buffer.from('Haus\nÜberraschung\n') });
      await form.getByRole('button', { name: /import known vocabulary/i }).press('Enter');
      if (disabled) {
        await expect(page).toHaveURL(/\/known-vocab\/imports\/7\/status/);
        await expect(page.getByRole('status')).toContainText(/complete|queued/i);
      } else {
        await expect(page).toHaveURL(/\/settings/);
        await expect(page.locator('#known-vocabulary-results')).toContainText(/queued|complete/i);
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

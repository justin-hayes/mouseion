import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('My Books collection browsing', () => {
  test('uses Mouseion-owned styles without loading Pico', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');

    await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico"]')).toHaveCount(0);
    await expect(page.locator('body')).toHaveClass(/my-books-shell/);
    const covers = page.locator('.library-grid .book-cover-media img');
    await expect(covers.first()).toBeVisible();
    expect(await covers.evaluateAll(images => images.every(image => image.getAttribute('alt') === ''))).toBe(true);
    expect(await page.locator('.library-grid .book-cover-media__placeholder').evaluateAll(nodes => nodes.every(node => node.getAttribute('aria-hidden') === 'true'))).toBe(true);
    const search = page.getByLabel('Search My Books');
    const button = page.getByRole('button', { name: 'Search' });
    await expect(search).toBeVisible();
    expect(await search.evaluate(node => getComputedStyle(node).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)');
    expect((await button.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await search.focus();
    expect(await search.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('solid');
    const inboxFilter = page.getByRole('navigation', { name: 'My Books filters' }).getByRole('link', { name: /Inbox/ });
    expect((await inboxFilter.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    await page.goto('/reading');
    await expect(page.locator('link[rel="stylesheet"][href*="pico"]')).toHaveCount(0);
    await expect(page.locator('body')).toHaveClass(/reading-shell/);
  });

  test('browses and searches only the active study language', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');

    await expect(page.getByLabel('Search My Books')).toBeVisible();
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await expect(page.getByRole('navigation', { name: 'Languages' })).toHaveCount(0);
    await expect(page.getByText('All languages')).toHaveCount(0);
    await expect(page.locator('ul.library-grid')).toHaveCount(1);
    await expect(page.locator('ul[role="grid"]')).toHaveCount(0);
    await expect(page.locator('.library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('.library-grid').getByText('Empty chapter')).toHaveCount(0);
    await expect(page.locator('.library-grid')).not.toContainText('language:');
    await expect(page.locator('.library-grid .library-book__membership').first()).toBeVisible();
    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(/q=Der(%20|\+)lange/);
    await expect(page.locator('#library-books-heading')).toBeFocused();
    await page.goto('/library');
    const firstBook = page.locator('.library-grid .library-book').first();
    await firstBook.getByText('More actions', { exact: true }).click();
    await expect(firstBook.getByText('Remove from My Books', { exact: true })).toHaveCount(0);

    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.locator('#library-results .library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('#library-results a[href^="/reading#"]').first()).toBeVisible();

    await page.getByLabel('Search My Books').fill('no-local-book-matches-this-term');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.getByText(/No books in your local collection.*match/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Clear search' })).toBeVisible();

    await page.goto('/library');
    await switcher.selectOption('it');
    await expect(page).toHaveURL('/library');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in Italian');
    await expect(page.locator('.library-grid').getByText('Empty chapter')).toBeVisible();
    await expect(page.locator('.library-grid').getByText('Der lange Weg nach Hause')).toHaveCount(0);
    await switcher.selectOption('de');
  });

  test('keeps the server-rendered grid usable without JavaScript', async ({ browser }) => {
    const context = await browser.newContext({ baseURL: test.info().project.use.baseURL, javaScriptEnabled: false });
    try {
      const page = await context.newPage();
      await signIn(page);
      await page.goto('/library?q=Der%20lange');
      await expect(page.locator('ul.library-grid')).toBeVisible();
      await expect(page.locator('.library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
      await page.goto('/library?q=Fehlgeschlagene');
      const book = page.locator('.library-grid .library-book').filter({ hasText: 'Fehlgeschlagene Analyse' });
      await expect(book).toBeVisible();
      await book.getByText('More actions', { exact: true }).click();
      await expect(book.getByText('Remove from My Books', { exact: true })).toHaveCount(0);
      await book.getByText('Set aside', { exact: true }).click();
      await book.getByRole('button', { name: 'Confirm set aside' }).click();
      await expect(page).toHaveURL(/disposition=set_aside/);
      const setAsideBook = page.locator('.library-grid .library-book').filter({ hasText: 'Fehlgeschlagene Analyse' });
      await expect(setAsideBook).toBeVisible();
      await setAsideBook.getByRole('button', { name: 'Move to To Read' }).click();
      await expect(page).toHaveURL(/disposition=to_read/);
      await expect(page.locator('.library-grid').getByText('Fehlgeschlagene Analyse')).toBeVisible();
    } finally {
      await context.close();
    }
  });

  test('restores pushed My Books history from a full server response', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(/q=Der(%20|\+)lange/);

    const historyResponse = page.waitForResponse(response => {
      const url = new URL(response.url());
      return url.pathname === '/library' && !url.searchParams.has('q');
    });
    await page.goBack();
    const response = await historyResponse;
    expect(await response.text()).toContain('<!doctype html>');
    await expect(page.locator('#library-results .library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
  });

  test('navigates to a complete corrected page after enhanced browse redirects', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');

    for (const [requestPath, correctedPath] of [
      ['/library?language=de&q=Der', '/library?q=Der'],
      ['/library?page=999', '/library'],
    ]) {
      await page.goto('/library');
      await page.evaluate(({ requestPath }) => {
        document.querySelector('[data-test-browse-correction]')?.remove();
        const link = document.createElement('a');
        link.href = requestPath;
        link.textContent = 'Test corrected browse navigation';
        link.setAttribute('data-test-browse-correction', '');
        link.setAttribute('data-my-books-enhanced', '');
        link.setAttribute('hx-get', requestPath);
        link.setAttribute('hx-target', '#library-results');
        link.setAttribute('hx-swap', 'outerHTML');
        document.querySelector('#library-results')?.append(link);
      }, { requestPath });

      const navigation = page.waitForNavigation();
      await page.locator('[data-test-browse-correction]').click();
      const destination = await navigation;
      expect(destination?.status()).toBe(200);
      expect(await destination?.text()).toContain('<!doctype html>');
      await expect(page).toHaveURL(correctedPath);
      await expect(page.locator('#library-results')).toBeVisible();
      await expect(page.locator('#library-results .library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
      if (correctedPath === '/library?q=Der') {
        await page.goBack();
        await expect(page).toHaveURL('/library');
        await page.goForward();
        await expect(page).toHaveURL(correctedPath);
        await expect(page.locator('#library-results .library-grid').getByText('Der lange Weg nach Hause')).toBeVisible();
      }
    }
  });

  test('shows actionable errors from a failed My Books handler in the enhanced results region', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    await page.getByLabel('Search My Books').fill('fixture-handler-error');
    const failure = page.waitForResponse(response => new URL(response.url()).searchParams.get('q') === 'fixture-handler-error');
    await page.getByRole('button', { name: 'Search' }).click();
    const response = await failure;
    expect(response.status()).toBe(500);
    expect(await response.text()).toContain('My Books could not be loaded');
    await expect(page.locator('#library-results [role="alert"]')).toContainText('My Books could not be loaded');
    await expect(page.locator('#library-results a', { hasText: 'Try again' })).toHaveAttribute('href', '/library?q=fixture-handler-error');
  });

  test('keeps the current results when an enhancement request fails on the network', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const currentResults = page.locator('#library-results');
    await expect(currentResults.getByText('Der lange Weg nach Hause')).toBeVisible();
    await page.route('**/library*', route => route.abort());
    await page.getByLabel('Search My Books').fill('network-error-check');
    const failedRequest = page.waitForEvent('requestfailed', request => new URL(request.url()).searchParams.get('q') === 'network-error-check');
    await page.getByRole('button', { name: 'Search' }).click();
    await failedRequest;
    await expect(currentResults.getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('#library-recovery [role="alert"]')).toContainText('connection failed');
    await expect(page.locator('#library-recovery a', { hasText: 'Retry My Books request' })).toHaveAttribute('href', /network-error-check/);
  });

  test('reviews needs-language books without actions', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture sync runs once per browser suite.');
    await signIn(page);
    await page.goto('/library');
    const strip = page.locator('section.library-needs-language');
    await expect(strip).toContainText(/book[s]? .*outside the active language collections/);
    await strip.getByRole('link', { name: 'Review books awaiting a language' }).click();
    await expect(page).toHaveURL(/\/library\?needs-language/);
    const needsRow = page.locator('.library-diagnostic-list li').filter({ hasText: 'Browser sync metadata book' });
    if (await needsRow.count()) {
      await expect(needsRow).toBeVisible();
      await expect(needsRow.locator('a')).toHaveCount(0);
      await expect(needsRow.locator('form')).toHaveCount(0);

      await page.goto('/catalogs');
      const sync = page.locator('#connection-fixture-browser-sync-connection').getByRole('button', { name: 'Sync now' });
      if (await sync.count()) {
        await sync.click();
        await expect(page).toHaveURL(/\/catalogs\?/);
      }
      await page.goto('/library?needs-language');
      await expect(page.locator('.library-diagnostic-list').getByText('Browser sync metadata book')).toHaveCount(0);
      await page.goto('/library');
      const syncedBook = page.locator('.library-grid .library-book').filter({ hasText: 'Browser sync metadata book' });
      await expect(syncedBook).toBeVisible();
      await expect(syncedBook.locator('.metadata').filter({ hasText: 'Workflow' })).toContainText('Inbox');
    }
  });
});

import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('My Books collection browsing', () => {
  test('keeps failed-analysis recovery beside its neutral evidence note at desktop and compact widths', async ({ page }) => {
    await signIn(page);
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      for (const viewport of [{ width: 1280, height: 800 }, { width: 375, height: 667 }]) {
        await page.setViewportSize(viewport);
        await page.goto('/library');
        const failedRow = page.locator('#book-row-fixture-failed');
        await expect(failedRow.locator('.library-book__evidence')).toHaveText('Analysis failed.');
        const retryAnalysis = failedRow.getByRole('button', { name: 'Retry analysis' });
        await expect(retryAnalysis).toBeVisible();
        await expect(retryAnalysis.locator('xpath=..')).toHaveAttribute('action', '/reading/books/fixture-failed/reanalyze');
        await expect(failedRow.locator('.library-book__evidence')).not.toContainText(/0%|coverage|ready/i);
        const analyzedRow = page.locator('#book-row-fixture-book');
        await expect(analyzedRow.locator('.library-book__evidence')).toContainText('97.4% of running words Known');
        await expect(analyzedRow.locator('.library-book__evidence')).toContainText('16 more words to reach 99%');
        await expect(analyzedRow.locator('.library-book__evidence')).toContainText('Deck ready: 412 cards');
        await expect(analyzedRow.locator('.library-book__membership')).toContainText('Currently reading');
        await expect(page.locator('#book-row-fixture-failed .library-book__evidence')).not.toContainText('Deck');
        await expect(page.locator('#book-row-fixture-running .library-book__evidence')).toHaveText('Analysis running.');
        const unavailableRow = page.locator('#book-row-fixture-route-unavailable');
        await expect(unavailableRow.locator('.library-book__evidence')).toHaveText('Content unavailable.');
        await expect(unavailableRow.getByRole('button', { name: 'Retry acquisition' })).toBeVisible();
        await expect(page.locator('#book-row-fixture-not-analyzed .library-book__evidence')).toHaveText('Not analysed yet.');
        await expect(page.locator('#book-row-fixture-read-history .library-book__evidence')).toHaveText('Finished 3 Jan 2026.');
      }
    }
  });

  test('wraps the recovery action without compact horizontal overflow at 200% text', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/library');
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    const recovery = page.locator('.library-book__recovery');
    await expect(recovery.first()).toBeVisible();
    const dimensions = await page.evaluate(() => {
      const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('.library-book__recovery'));
      return {
        viewport: document.documentElement.clientWidth,
        document: document.documentElement.scrollWidth,
        recoveries: buttons.map(button => ({
          right: button.getBoundingClientRect().right,
          scrollWidth: button.scrollWidth,
          clientWidth: button.clientWidth,
        })),
      };
    });
    expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
    for (const button of dimensions.recoveries) {
      expect(button.right).toBeLessThanOrEqual(dimensions.viewport);
      expect(button.scrollWidth).toBeLessThanOrEqual(button.clientWidth);
    }
  });

  test('uses one visible native disclosure cue and text-backed status shapes', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');

    const account = page.locator('details').filter({ has: page.getByText('Account', { exact: true }) });
    const summary = account.locator('summary');
    await expect(summary).toBeVisible();
    expect((await summary.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await page.emulateMedia({ reducedMotion: 'reduce' });
    const closedCue = await summary.evaluate(element => getComputedStyle(element, '::before').transform);
    await summary.focus();
    await page.keyboard.press('Space');
    await expect(account).toHaveAttribute('open', '');
    const openCue = await summary.evaluate(element => getComputedStyle(element, '::before').transform);
    expect(openCue).not.toBe(closedCue);

    await page.goto('/library');
    const badge = page.locator('.library-book__membership .status-badge').first();
    await expect(badge).toBeVisible();
    await expect(badge).toContainText(/Currently reading|To Read|Read|Inbox/);
    const shape = badge.locator('[aria-hidden="true"]');
    await expect(shape).toBeVisible();
    await expect(shape).toHaveAttribute('aria-hidden', 'true');
    const shapeStyle = await shape.evaluate(element => ({
      className: element.className,
      background: getComputedStyle(element).backgroundColor,
      border: getComputedStyle(element).borderColor,
    }));
    if (shapeStyle.className.includes('shape--ring')) {
      expect(shapeStyle.border).not.toBe('rgba(0, 0, 0, 0)');
    } else {
      expect(shapeStyle.background).not.toBe('rgba(0, 0, 0, 0)');
    }
    expect((await badge.innerText()).trim().length).toBeGreaterThan(0);
  });

  test('uses Mouseion-owned styles without loading Pico', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');

    await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico"]')).toHaveCount(0);
    await expect(page.locator('body')).toHaveClass(/my-books-shell/);
    const covers = page.locator('.library-books .book-cover-media img');
    await expect(covers.first()).toBeVisible();
    expect(await covers.evaluateAll(images => images.every(image => image.getAttribute('alt') === ''))).toBe(true);
    const placeholders = page.locator('.library-books .book-cover-media__placeholder');
    expect(await placeholders.count()).toBeGreaterThan(0);
    expect(await placeholders.evaluateAll(nodes => nodes.every(node => node.getAttribute('aria-hidden') === 'true'))).toBe(true);
    expect(await placeholders.locator('.book-cover-media__initial').evaluateAll(nodes => nodes.every(node => getComputedStyle(node).display !== 'none'))).toBe(true);
    expect(await placeholders.locator('.book-cover-media__label').evaluateAll(nodes => nodes.every(node => getComputedStyle(node).display === 'none'))).toBe(true);
    const search = page.getByLabel('Search My Books');
    const button = page.getByRole('button', { name: 'Search' });
    await expect(search).toBeVisible();
    expect(await search.evaluate(node => getComputedStyle(node).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)');
    expect((await button.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    const searchLayout = await page.locator('.library-search__controls').evaluate(node => {
      const input = node.querySelector('input')!.getBoundingClientRect();
      const submit = node.querySelector('button')!.getBoundingClientRect();
      const controls = node.getBoundingClientRect();
      return { inputWidth: input.width, inputRight: input.right, inputBottom: input.bottom, inputTop: input.top, submitLeft: submit.left, submitTop: submit.top, controlsWidth: controls.width };
    });
    // The field has a bounded measure (28rem) with the outline Search action beside it.
    expect(searchLayout.inputWidth).toBeGreaterThan(Math.min(searchLayout.controlsWidth * 0.55, 400));
    if (Math.abs(searchLayout.submitTop - searchLayout.inputTop) < 1) {
      expect(searchLayout.submitLeft - searchLayout.inputRight).toBeGreaterThanOrEqual(0);
      expect(searchLayout.submitLeft - searchLayout.inputRight).toBeLessThanOrEqual(12);
    } else {
      expect(searchLayout.submitTop).toBeGreaterThanOrEqual(searchLayout.inputBottom);
    }
    await search.focus();
    expect(await search.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('solid');
    const inboxFilter = page.getByRole('navigation', { name: 'My Books filters' }).getByRole('link', { name: /Inbox/ });
    expect((await inboxFilter.boundingBox())?.height).toBeGreaterThanOrEqual(44);

    await page.goto('/reading');
    await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico"]')).toHaveCount(0);
    await expect(page.locator('body')).toHaveClass(/reading-shell/);
    await expect(page.getByRole('heading', { level: 1, name: 'Der lange Weg nach Hause', exact: true })).toBeVisible();
    await expect(page.locator('.journey-book__since')).toContainText('Reading since');
    await expect(page.locator('#vocabulary-prefix')).toHaveClass(/\binput\b/);
    await expect(page.locator('.reading-supporting')).toHaveCount(1);
    await page.goto('/vocabulary/concordance');
    const concordanceSelects = page.locator('#concordance-workflow select');
    await expect(concordanceSelects).toHaveCount(0);
  });

  test('shows the first Book identity in the initial desktop and compact viewport', async ({ page }) => {
    await signIn(page);
    for (const viewport of [{ width: 1280, height: 800 }, { width: 375, height: 667 }]) {
      await page.setViewportSize(viewport);
      await page.goto('/library');
      const firstBook = page.locator('.library-books .library-book').first();
      await expect(firstBook).toBeVisible();
      const visibleIdentity = await firstBook.evaluate((item, height) => {
        const title = item.querySelector('.bibliographic-title')!.getBoundingClientRect();
        const cover = item.querySelector('.book-cover-media')!.getBoundingClientRect();
        const intersects = (rect: DOMRect) => rect.top < height && rect.bottom > 0;
        return { title: intersects(title), cover: intersects(cover) };
      }, viewport.height);
      expect(visibleIdentity.title, `Book title should intersect ${viewport.width}x${viewport.height}`).toBe(true);
      expect(visibleIdentity.cover, `Book cover/placeholder should intersect ${viewport.width}x${viewport.height}`).toBe(true);
      const notice = page.locator('aside.library-needs-language');
      if (await notice.count()) {
        await expect(notice.getByRole('link', { name: 'Review' })).toBeVisible();
        expect(await notice.evaluate((node, height) => {
          const rect = node.getBoundingClientRect();
          return rect.top < height && rect.bottom > 0;
        }, viewport.height)).toBe(true);
        await notice.getByText('Why?', { exact: true }).click();
        await expect(notice.getByRole('link', { name: 'Catalogs' })).toBeVisible();
      }
    }
  });

  test('aligns identity and actions in the text column with status in the margin', async ({ page }) => {
    await signIn(page);
    for (const viewport of [{ width: 1280, height: 800 }, { width: 900, height: 900 }, { width: 375, height: 667 }]) {
      await page.setViewportSize(viewport);
      await page.goto('/library');
      const row = page.locator('.library-books .library-book').filter({ has: page.locator('.library-book__author') }).first();
      const title = row.locator('.bibliographic-title');
      const author = row.locator('.library-book__author');
      const cover = row.locator('.book-cover-media');
      const notes = row.locator('.library-book__notes');
      const actions = row.locator('.library-book__actions');
      const geometry = await Promise.all([title, author, cover, notes, actions].map(locator => locator.boundingBox()));
      const [titleBox, authorBox, coverBox, notesBox, actionsBox] = geometry;
      expect(titleBox && authorBox && coverBox && notesBox && actionsBox).toBeTruthy();
      expect(Math.abs(titleBox!.x - authorBox!.x)).toBeLessThanOrEqual(1);
      expect(coverBox!.x).toBeLessThan(titleBox!.x);
      expect(Math.abs(titleBox!.x - actionsBox!.x)).toBeLessThanOrEqual(1);
      expect(await notes.evaluate(element => getComputedStyle(element).borderLeftWidth)).not.toBe('0px');
      if (viewport.width <= 640) {
        expect(notesBox!.y).toBeGreaterThanOrEqual(authorBox!.y + authorBox!.height);
        expect(actionsBox!.y).toBeGreaterThanOrEqual(notesBox!.y + notesBox!.height);
      } else {
        expect(notesBox!.x).toBeGreaterThan(titleBox!.x);
        expect(actionsBox!.y).toBeGreaterThanOrEqual(titleBox!.y + titleBox!.height);
      }
    }
    await page.emulateMedia({ forcedColors: 'active' });
    const marginEdge = page.locator('.library-book__notes').first();
    await expect(marginEdge).toBeVisible();
    expect(await marginEdge.evaluate(element => getComputedStyle(element).borderLeftColor)).not.toBe('rgba(0, 0, 0, 0)');
  });

  test('wraps long multilingual Book identity at compact width and 200% text size', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/library');
    const book = page.locator('.library-books .library-book').filter({ has: page.locator('.library-book__author') }).first();
    const title = book.locator('.bibliographic-title');
    const author = book.locator('.library-book__author');
    const longGreekTitle = 'Una storia straordinariamente lunga: Donaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamtengesellschaft; Μια εξαιρετικά μακριά ελληνική βιβλιογραφική περιγραφή';
    const longGermanAuthor = 'Autorin mit einem außergewöhnlich langen deutschen Familiennamen';
    await title.evaluate((element, text) => { element.textContent = text; }, longGreekTitle);
    await author.evaluate((element, text) => { element.textContent = text; }, longGermanAuthor);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });

    const dimensions = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
    await expect(title).toHaveText(longGreekTitle);
    await expect(author).toHaveText(longGermanAuthor);
    expect(await title.evaluate(element => element.scrollWidth)).toBeLessThanOrEqual(await title.evaluate(element => element.clientWidth));
    expect(await author.evaluate(element => element.scrollWidth)).toBeLessThanOrEqual(await author.evaluate(element => element.clientWidth));
    await expect(book.getByText('More actions', { exact: true })).toBeVisible();
  });

  test('browses and searches only the active study language', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');

    await expect(page.getByLabel('Search My Books')).toBeVisible();
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    await expect(page.locator('.library-language-context')).toHaveText('German collection');
    await expect(page.getByRole('navigation', { name: 'Languages' })).toHaveCount(0);
    await expect(page.getByText('All languages')).toHaveCount(0);
    await expect(page.locator('ul.library-books')).toHaveCount(1);
    await expect(page.locator('ul[role="grid"]')).toHaveCount(0);
    await expect(page.locator('.library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('.library-books').getByText('Empty chapter')).toHaveCount(0);
    await expect(page.locator('.library-books')).not.toContainText('language:');
    await expect(page.locator('.library-books .library-book__membership').first()).toBeVisible();
    const readingLink = page.locator('.library-books a.library-book__journey-action').first();
    await expect(readingLink).toHaveText('View in Reading');
    await expect(readingLink).toHaveAccessibleName('View in Reading');
    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(/q=Der(%20|\+)lange/);
    await expect(page.locator('#library-results')).toBeFocused();
    await page.goto('/library');
    const firstBook = page.locator('.library-books .library-book').first();
    await firstBook.getByText('More actions', { exact: true }).click();
    await expect(firstBook.getByText('Remove from My Books', { exact: true })).toHaveCount(0);

    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.locator('#library-results .library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('#library-results a[href^="/reading#"]').first()).toBeVisible();

    await page.getByLabel('Search My Books').fill('no-local-book-matches-this-term');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.getByText(/No books in your local collection.*match/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Clear search' })).toBeVisible();

    await page.goto('/library');
    await switcher.selectOption('it');
    await expect(page).toHaveURL('/library');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    await expect(page.locator('.library-language-context')).toHaveText('Italian collection');
    await expect(page.locator('.library-books').getByText('Empty chapter')).toBeVisible();
    await expect(page.locator('.library-books').getByText('Der lange Weg nach Hause')).toHaveCount(0);
    await switcher.selectOption('de');
  });

  test('My Books search keeps its native GET fallback and submits enhanced updates', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const form = page.getByRole('search');
    await expect(form).toHaveAttribute('method', 'get');
    await expect(form).toHaveAttribute('action', /\/library/);
    await expect(form.locator('input[name="q"]')).toHaveAttribute('type', 'search');
    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(/q=Der(%20|\+)lange/);
    await expect(page.locator('#library-results .library-books')).toContainText('Der lange Weg nach Hause');
  });

  test('keeps the server-rendered list usable without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    try {
      const page = await context.newPage();
      await signIn(page);
      await page.goto('/library?q=Der%20lange');
      await expect(page.locator('ul.library-books')).toBeVisible();
      await expect(page.locator('.library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
      await page.goto('/library?q=Fehlgeschlagene');
      const book = page.locator('.library-books .library-book').filter({ hasText: 'Fehlgeschlagene Analyse' });
      await expect(book).toBeVisible();
      await book.getByText('More actions', { exact: true }).click();
      await expect(book.getByText('Remove from My Books', { exact: true })).toHaveCount(0);
      await expect(book.getByText('Set aside', { exact: true })).toHaveCount(0);
      await expect(book.getByRole('button', { name: 'Confirm set aside' })).toHaveCount(0);
      await expect(page.getByRole('link', { name: /Set Aside/ })).toHaveCount(0);
      await expect(page.locator('.library-books').getByText('Fehlgeschlagene Analyse')).toBeVisible();
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
    await expect(page.locator('#library-results .library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
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
      await expect(page.locator('#library-results .library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
      if (correctedPath === '/library?q=Der') {
        await page.goBack();
        await expect(page).toHaveURL('/library');
        await page.goForward();
        await expect(page).toHaveURL(correctedPath);
        await expect(page.locator('#library-results .library-books').getByText('Der lange Weg nach Hause')).toBeVisible();
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
    await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
    const alert = page.locator('#library-results [role="alert"]');
    await expect(alert).toContainText('My Books could not be loaded');
    expect(await alert.evaluate(node => getComputedStyle(node).color)).not.toBe(await alert.evaluate(node => getComputedStyle(node.parentElement!).color));
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
    await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
    const alert = page.locator('#library-recovery [role="alert"]');
    await expect(alert).toContainText('connection failed');
    expect(await alert.evaluate(node => getComputedStyle(node).color)).not.toBe(await alert.evaluate(node => getComputedStyle(node.parentElement!).color));
    await expect(page.locator('#library-recovery a', { hasText: 'Retry My Books request' })).toHaveAttribute('href', /network-error-check/);
  });

  test('reviews needs-language books with only the visibility action', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const strip = page.locator('aside.library-needs-language');
    await expect(strip).toContainText(/book[s]? awaiting a language/);
    await strip.getByRole('link', { name: 'Review' }).click();
    await expect(page).toHaveURL(/\/library\?needs-language/);
    const needsRow = page.locator('.library-diagnostic-list li').filter({ hasText: 'Browser sync metadata book' });
    if (await needsRow.count()) {
      await expect(needsRow).toBeVisible();
      await expect(needsRow.locator('a')).toHaveCount(0);
      await expect(needsRow.locator('form')).toHaveCount(1);
      await expect(needsRow.locator('form')).toHaveAttribute('action', /\/hide$/);

      await page.goto('/catalogs');
      const sync = page.locator('#connection-fixture-browser-sync-connection').getByRole('button', { name: 'Sync now' });
      if (await sync.count()) {
        await sync.click();
        await expect(page).toHaveURL(/\/catalogs\?/);
      }
      await page.goto('/library?needs-language');
      await expect(page.locator('.library-diagnostic-list').getByText('Browser sync metadata book')).toHaveCount(0);
      await page.goto('/library');
      const syncedBook = page.locator('.library-books .library-book').filter({ hasText: 'Browser sync metadata book' });
      await expect(syncedBook).toBeVisible();
      await expect(syncedBook.locator('.library-book__membership .status-badge')).toHaveText('Inbox');
    }
  });
});

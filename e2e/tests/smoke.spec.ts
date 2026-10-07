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

async function expectConcordanceScanSpace(page: Page, selector: string) {
  const scan = await page.locator(selector).evaluateAll(rows => {
    const boxes = rows.map(row => row.getBoundingClientRect());
    const firstTop = boxes[0]?.top ?? Number.POSITIVE_INFINITY;
    return {
      space: window.innerHeight - firstTop,
      visibleRows: boxes.filter(box => box.top < window.innerHeight && box.bottom > 0).length,
    };
  });
  expect(scan.space).toBeGreaterThanOrEqual(180);
  expect(scan.visibleRows).toBeGreaterThanOrEqual(4);
}

test.describe('authenticated learner smoke', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('expired enhanced action signs in and returns without exposing the raw failure', async ({ page }) => {
    await page.goto('/library');
    await page.getByLabel('Search My Books').fill('retry after sign in');

    await page.context().clearCookies();
    const unauthorized = page.waitForResponse(response =>
      response.url().includes('/library') && response.status() === 401,
    );
    await page.getByRole('button', { name: 'Search', exact: true }).click();
    await unauthorized;

    await expect(page).toHaveURL(/\/login\?/);
    await expect.poll(() => new URL(page.url()).searchParams.get('next'))
      .toBe('/library?q=retry+after+sign+in');
    await expect(page.getByRole('heading', { name: /sign in|log in/i })).toBeVisible();
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).click();

    await expect(page).toHaveURL(/\/library\?q=retry\+after\+sign\+in$/);
    await expect(page.getByText('authentication required')).toHaveCount(0);
  });

  test('expired enhanced My Books mutation explains that it was not completed', async ({ page }) => {
    await page.goto('/library');
    const csrf = await page.locator('input[name="csrf_token"]').first().inputValue();
    await page.evaluate(token => {
      const form = document.createElement('form');
      form.method = 'post';
      form.action = '/library/books/fixture-book/refresh';
      form.setAttribute('hx-post', form.action);
      form.setAttribute('hx-target', '#library-results');
      form.innerHTML = '<input type="hidden" name="csrf_token"><button type="submit">Refresh metadata</button>';
      form.querySelector('input[name="csrf_token"]')!.value = token;
      document.getElementById('main-content')!.prepend(form);
      window.htmx.process(form);
    }, csrf);

    await page.context().clearCookies();
    let refreshRequests = 0;
    page.on('request', request => {
      if (request.url().includes('/library/books/fixture-book/refresh')) refreshRequests++;
    });
    const unauthorized = page.waitForResponse(response =>
      response.url().includes('/library/books/fixture-book/refresh') && response.status() === 401,
    );
    await page.getByRole('button', { name: 'Refresh metadata' }).click();
    await unauthorized;

    await expect(page).toHaveURL(/\/login\?/);
    await expect(page.getByRole('alert'))
      .toHaveText('Your session expired. The action was not completed. Sign in and retry it.');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).click();

    await expect(page).toHaveURL(/\/library$/);
    expect(refreshRequests).toBe(1);
  });

  test('login and library expose representative content', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /my books|welcome/i }).first()).toBeVisible();
    await page.getByRole('link', { name: /my books/i }).first().click();
    await expect(page).toHaveURL(/\/library/);
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    await expect(page.locator('section#library-results')).toBeVisible();
    await expect(page.locator('.library-books a.library-book__identity-link[href="/reading#journey-book-fixture-book"]', { hasText: 'Der lange Weg nach Hause' })).toBeVisible();
    await expect(page.locator('.library-books a[href="/books/fixture-failed"]', { hasText: 'Fehlgeschlagene Analyse' })).toHaveCount(0);
    await expect(page.locator('.library-books a[href="/books/fixture-empty"]', { hasText: 'Empty chapter' })).toHaveCount(0);
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
    await expect(page.locator('main h1')).toHaveCount(1);
    await expect(page.getByText('different study language', { exact: false })).toHaveCount(0);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.getByRole('heading', { name: 'Choose a To Read book in fr', exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'No To Read books yet', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Browse My Books', exact: true })).toBeVisible();
    await page.getByLabel('Study language').selectOption('de');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.locator('main h1')).toHaveCount(1);

  });

  test('retired Journey endpoints are unavailable', async ({ page }) => {
    const route = await page.goto('/journey/fixture-book');
    expect(route?.status()).toBe(404);
  });

  test('Reading confirmations and chooser actions retain native semantics without JavaScript', async ({ browser }) => {
    const noScript = await browser.newPage({
      baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099',
      javaScriptEnabled: false,
      colorScheme: test.info().project.name.endsWith('-dark') ? 'dark' : 'light',
      reducedMotion: 'reduce',
      viewport: { width: 320, height: 812 },
    });
    try {
      await signIn(noScript);
      await noScript.goto('/reading');
      const current = noScript.locator('#primary-goal-section');
      const switchLink = current.locator('a[href="/reading/switch"]');
      await expect(switchLink).toBeVisible();
      await expect(switchLink).toHaveAttribute('class', /button--outline/);
      await expect(switchLink).not.toHaveAttribute('role', 'button');

      const finish = current.locator('details.confirmation').filter({ hasText: 'Mark reading finished' });
      const finishSummary = finish.locator('summary');
      await finishSummary.focus();
      await noScript.keyboard.press('Enter');
      await expect(finish).toHaveAttribute('open', '');
      await expect(finish.getByRole('button', { name: 'Mark reading finished' })).toBeVisible();
      await expect(finishSummary).toBeFocused();
      await noScript.keyboard.press('Space');
      await expect(finish).not.toHaveAttribute('open', '');

      const compactOverflow = await noScript.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(compactOverflow).toBe(false);
      await noScript.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
      const enlargedTextOverflow = await noScript.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(enlargedTextOverflow).toBe(false);

      await switchLink.click();
      const candidateConfirmation = noScript.locator('details.confirmation').filter({ hasText: 'Switch to this book' }).first();
      const candidateSummary = candidateConfirmation.locator('summary');
      await candidateSummary.focus();
      await noScript.keyboard.press('Space');
      await expect(candidateConfirmation).toHaveAttribute('open', '');
      await expect(candidateConfirmation.getByRole('button', { name: 'Confirm switch to this book' })).toBeVisible();
    } finally {
      await noScript.close();
    }
  });

  test('Concordance lookup is server-rendered and submits without JavaScript', async ({ browser }) => {
    const noScript = await browser.newPage({
      baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099',
      javaScriptEnabled: false,
      colorScheme: test.info().project.name.endsWith('-dark') ? 'dark' : 'light',
      // 320×200 CSS pixels approximates 400% zoom on a 1280×800 desktop.
      viewport: { width: 320, height: 812 },
    });
    try {
      await signIn(noScript);
      await noScript.goto('/vocabulary/concordance');
      await expect(noScript.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
      expect(await noScript.locator('#concordance-workflow select').evaluateAll(nodes => nodes.every(node => getComputedStyle(node).appearance === 'auto'))).toBe(true);
      await expect(noScript.getByLabel('Lookup evidence')).toHaveValue('surface');
      await expect(noScript.getByLabel('Part of speech')).toBeHidden();
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
      await expect(rows).toHaveCount(25);
      const studyLinks = noScript.getByRole('link', { name: 'Study this sentence and its syntax' });
      await expect(studyLinks).toHaveCount(25);
      await expect(studyLinks.nth(0)).toBeVisible();
      await expect(studyLinks.nth(0)).toHaveAttribute('title', 'Study this sentence and its syntax');
      await expect(studyLinks.nth(0)).toHaveText('↗');
      await expect(rows.nth(0)).not.toHaveAttribute('open', '');
      await expect(studyLinks.nth(0).locator('xpath=ancestor::details')).toHaveCount(0);
      await rows.nth(0).locator('summary').focus();
      await noScript.keyboard.press('Tab');
      await expect(studyLinks.nth(0)).toBeFocused();
      await noScript.evaluate(() => {
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
        window.scrollTo(0, 0);
      });
      await noScript.keyboard.press('ArrowDown');
      await expect.poll(() => noScript.evaluate(() => window.scrollY)).toBeGreaterThan(0);
      await rows.nth(2).locator('summary').focus();
      await noScript.keyboard.press('Enter');
      await expect(rows.nth(2)).toHaveAttribute('open', '');
      await expect(rows.nth(2).locator('.concordance-context p')).toHaveText('Das Haus sieht gut aus.');
      await rows.nth(3).locator('summary').focus();
      await noScript.keyboard.press('Space');
      await expect(rows.nth(3)).toHaveAttribute('open', '');
      await expect(rows.nth(2)).not.toHaveAttribute('open', '');
      await noScript.keyboard.press('Escape');
      await expect(rows.nth(3)).toHaveAttribute('open', '');
      const expandedContext = rows.nth(2).locator('.concordance-context');
      await expect(expandedContext.locator('.concordance-observed-target')).toHaveText('Haus');
      await expect(expandedContext).not.toContainText('Unchanged evidence');
      await expect(expandedContext).not.toContainText('Analyzer lemma');
      await expect(expandedContext).not.toContainText('Dependency:');
      await studyLinks.nth(0).click();
      await expect(noScript.getByRole('heading', { name: 'Study this sentence and its syntax' })).toBeVisible();
      await expect(noScript.locator('.sentence-study-text')).toContainText('Das Haus sieht gut aus.');
      await expect(noScript.locator('.sentence-study-tokens')).toContainText('Corrected for this occurrence');
      await noScript.getByRole('link', { name: 'Return to Concordance results' }).click();
      await expect(noScript).toHaveURL(/\/vocabulary\/concordance\?.*#occurrence-fixture-book-0-1$/);
      await expect(noScript.getByRole('heading', { name: 'Current results' })).toBeVisible();
      await expect.poll(() => noScript.evaluate(() => document.activeElement?.id)).toBe('occurrence-fixture-book-0-1');
      const compactOverflow = await noScript.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(compactOverflow).toBe(false);
      await noScript.setViewportSize({ width: 320, height: 200 });
      const highZoomOverflow = await noScript.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
      expect(highZoomOverflow).toBe(false);
      const highZoomResult = noScript.locator('#concordance-native-results .concordance-result').first();
      await highZoomResult.locator('summary').click();
      await expect(highZoomResult.locator('.concordance-context')).toContainText('Das Haus sieht gut aus.');
      const highZoomContent = await highZoomResult.evaluate(row => {
        const rowBox = row.getBoundingClientRect();
        const book = row.querySelector('.concordance-book-label')!.getBoundingClientRect();
        const source = row.querySelector('.concordance-context')!.getBoundingClientRect();
        const study = row.querySelector('.concordance-study-link')!.getBoundingClientRect();
        return { rowLeft: rowBox.left, rowRight: rowBox.right, bookLeft: book.left, bookRight: book.right, sourceLeft: source.left, sourceRight: source.right, studyLeft: study.left, studyRight: study.right };
      });
      expect(highZoomContent.rowLeft).toBeGreaterThanOrEqual(0);
      expect(highZoomContent.rowRight).toBeLessThanOrEqual(320);
      expect(highZoomContent.bookLeft).toBeGreaterThanOrEqual(0);
      expect(highZoomContent.bookRight).toBeLessThanOrEqual(320);
      expect(highZoomContent.sourceLeft).toBeGreaterThanOrEqual(0);
      expect(highZoomContent.sourceRight).toBeLessThanOrEqual(320);
      expect(highZoomContent.studyLeft).toBeGreaterThanOrEqual(0);
      expect(highZoomContent.studyRight).toBeLessThanOrEqual(320);
      await noScript.setViewportSize({ width: 1280, height: 800 });
      await noScript.goto('/vocabulary/concordance?mode=surface&term=Haus');
      await expectConcordanceScanSpace(noScript, '#concordance-native-results .concordance-result');
      await noScript.getByRole('navigation', { name: 'Concordance pages' }).getByRole('link', { name: 'Next' }).click();
      await expect(noScript).toHaveURL(/page=2/);
      await expect(noScript.locator('details.concordance-row')).toHaveCount(1);
      const secondPageID = await noScript.locator('details.concordance-row').getAttribute('id');
      await noScript.getByRole('link', { name: 'Study this sentence and its syntax' }).click();
      await noScript.getByRole('link', { name: 'Return to Concordance results' }).click();
      await expect(noScript).toHaveURL(new RegExp(`page=2.*#${secondPageID}$`));
      await expect.poll(() => noScript.evaluate(() => document.activeElement?.id)).toBe(secondPageID);
    } finally {
      await noScript.close();
    }
  });

  test('Concordance enhancement keeps one server-rendered list and native disclosures', async ({ page }) => {
    const concordanceRequests: string[] = [];
    page.on('request', request => {
      const url = new URL(request.url());
      if (url.pathname === '/vocabulary/concordance' && url.searchParams.get('term') === 'Haus') concordanceRequests.push(request.url());
    });
    await page.goto('/vocabulary/concordance');
    await expect(page.locator('#concordance-results')).toHaveCount(0);
    await page.getByLabel('Exact term').fill('Haus');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    const results = page.locator('#concordance-results');
    const rows = page.locator('#concordance-native-results');
    await expect(rows).toBeVisible();
    await expect(rows.locator('.concordance-result')).toHaveCount(25);
    await expect(rows).toHaveAttribute('role', 'list');
    await expect(page.locator('#concordance-summary')).toBeFocused();
    await expect(page.locator('.concordance-results')).toHaveCount(1);
    await expect(page.locator('[data-concordance-data], mouseion-concordance')).toHaveCount(0);
    await page.setViewportSize({ width: 1280, height: 800 });
    await expectConcordanceScanSpace(page, '#concordance-native-results .concordance-result');
    const occurrenceIDs = await rows.locator('details.concordance-row').evaluateAll(rows => rows.map(row => row.id));
    expect(new Set(occurrenceIDs).size).toBe(occurrenceIDs.length);
    expect(occurrenceIDs).toEqual(Array.from({ length: 25 }, (_, index) => `occurrence-fixture-book-${index}-1`));
    await expect(page.locator('#occurrence-fixture-book-0-1')).toBeVisible();
    const firstRow = rows.locator('.concordance-result').first();
    await expect(firstRow.locator('.concordance-book-label')).toContainText('Der lange Weg nach Hause');
    await expect(firstRow.locator('.concordance-study-link')).toBeVisible();
    const accentUsesMouseionToken = await firstRow.locator('.concordance-surface').evaluate(element => {
      const probe = document.createElement('span');
      probe.style.color = 'var(--mouseion-color-accent)';
      element.append(probe);
      const tokenColor = getComputedStyle(probe).color;
      probe.remove();
      return getComputedStyle(element).color === tokenColor;
    });
    expect(accentUsesMouseionToken).toBe(true);
    await page.emulateMedia({ reducedMotion: 'reduce' });
    const bookFilter = page.locator('.concordance-scopes details').first();
    const bookFilterSummary = bookFilter.locator('summary');
    const closedBookCue = await bookFilterSummary.evaluate(element => getComputedStyle(element, '::before').transform);
    await bookFilterSummary.click();
    await expect(bookFilter).toHaveAttribute('open', '');
    expect(await bookFilterSummary.evaluate(element => getComputedStyle(element, '::before').transform)).not.toBe(closedBookCue);
    await bookFilterSummary.click();
    await expect(firstRow.locator('details')).not.toHaveAttribute('open', '');
    const closedOccurrenceCue = await firstRow.locator('summary').evaluate(element => getComputedStyle(element, '::before').transform);
    await firstRow.locator('summary').click();
    expect(await firstRow.locator('summary').evaluate(element => getComputedStyle(element, '::before').transform)).not.toBe(closedOccurrenceCue);
    await expect(firstRow.locator('.concordance-context')).toContainText('Das Haus sieht gut aus.');
    await expect(firstRow.locator('.concordance-observed-target')).toHaveText('Haus');
    await expect(firstRow.locator('.concordance-study-link')).toBeVisible();
    const secondRow = rows.locator('.concordance-result').nth(1);
    await secondRow.locator('summary').click();
    await expect(secondRow.locator('details')).toHaveAttribute('open', '');
    await expect(firstRow.locator('details')).not.toHaveAttribute('open', '');
    expect(concordanceRequests).toHaveLength(1);
    await page.locator('#concordance-workflow').getByRole('navigation', { name: 'Concordance pages' }).getByRole('link', { name: 'Next' }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(1);
    await page.goBack();
    await expect(page).toHaveURL(/term=Haus/);
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(25);
    await page.goForward();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(1);
    await page.goBack();
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(25);
    const rowKeysDefaultPrevented = await page.evaluate(() => {
      const summary = document.querySelector('#concordance-native-results details summary')!;
      return ['ArrowDown', 'ArrowUp', 'Escape'].map(key => {
        const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
        summary.dispatchEvent(event);
        return event.defaultPrevented;
      });
    });
    expect(rowKeysDefaultPrevented).toEqual([false, false, false]);
    await page.setViewportSize({ width: 320, height: 812 });
    const horizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
    expect(horizontalOverflow).toBe(false);
    await expect(firstRow.locator('.concordance-book-label')).toBeVisible();
    await expect(firstRow.locator('.concordance-study-link')).toBeVisible();
    await expect(secondRow.locator('.concordance-context')).toContainText('Das Haus sieht gut aus.');
    await firstRow.locator('.concordance-study-link').click();
    await expect(page.getByRole('heading', { name: 'Study this sentence and its syntax' })).toBeVisible();
    await page.getByRole('link', { name: 'Return to Concordance results' }).click();
    const returnedOccurrence = page.locator('#occurrence-fixture-book-0-1');
    await expect(returnedOccurrence).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe('occurrence-fixture-book-0-1');
    await expect(results).toContainText('lookup for “Haus”');
  });

  test('Concordance paging preserves the query controls in the DOM', async ({ page }) => {
    await page.goto('/vocabulary/concordance');
    await page.getByLabel('Exact term').fill('Haus');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(25);
    await page.getByLabel('Exact term').evaluate(element => element.setAttribute('data-stable-control', 'yes'));
    await page.getByRole('navigation', { name: 'Concordance pages' }).getByRole('link', { name: 'Next' }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(1);
    await expect(page.getByLabel('Exact term')).toHaveAttribute('data-stable-control', 'yes');
  });

  test('Concordance network failure keeps old results and offers the attempted query', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj');
    const oldResults = page.locator('#concordance-results');
    await expect(oldResults).toContainText('lookup for “Haus”');
    await page.route(/\/vocabulary\/concordance.*term=Netzwerk/, route => route.abort());
    await page.getByLabel('Exact term').fill('Netzwerk');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    const recovery = page.locator('#concordance-recovery');
    await expect(recovery).toContainText('connection failed or the browser-side request timed out');
    const retry = recovery.getByRole('link', { name: 'Retry Concordance lookup' });
    await expect(retry).toHaveAttribute('href', /term=Netzwerk/);
    await expect(retry).toHaveAttribute('href', /book=fixture-book/);
    await expect(retry).toHaveAttribute('href', /grammar=own/);
    await expect(retry).toHaveAttribute('href', /relation=obj/);
    await expect(oldResults).toContainText('lookup for “Haus”');
    await expect(page).toHaveURL(/term=Haus/);
    await page.unroute(/\/vocabulary\/concordance.*term=Netzwerk/);
  });

  test('Concordance browser-side timeout keeps old results and exposes recovery', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
    const oldResults = page.locator('#concordance-results');
    let releaseRequest!: () => void;
    const requestGate = new Promise<void>(resolve => { releaseRequest = resolve; });
    let requestStarted!: () => void;
    const started = new Promise<void>(resolve => { requestStarted = resolve; });
    await page.route(/\/vocabulary\/concordance.*term=Slow/, async route => {
      requestStarted();
      await requestGate;
      try { await route.continue(); } catch { /* HTMX times out and aborts this request. */ }
    });
    try {
      await page.getByLabel('Exact term').fill('Slow');
      await page.getByRole('button', { name: 'Find', exact: true }).click();
      await started;
      await expect(page.locator('#concordance-recovery')).toContainText('browser-side request timed out', { timeout: 11_000 });
      await expect(oldResults).toContainText('lookup for “Haus”');
      await expect(page).toHaveURL(/term=Haus/);
    } finally {
      releaseRequest();
      await page.unroute(/\/vocabulary\/concordance.*term=Slow/);
    }
  });

  test('Concordance applies only the newest overlapping lookup', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
    let releaseSlow!: () => void;
    const slowResponse = new Promise<void>(resolve => { releaseSlow = resolve; });
    let slowRequestStarted!: () => void;
    const slowRequest = new Promise<void>(resolve => { slowRequestStarted = resolve; });
    await page.route(/\/vocabulary\/concordance.*term=Langsam/, async route => {
      slowRequestStarted();
      await slowResponse;
      try { await route.continue(); } catch { /* HTMX may have aborted the superseded request. */ }
    });
    await page.getByLabel('Exact term').fill('Langsam');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await slowRequest;
    await page.getByLabel('Exact term').fill('Haus2');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”');
    releaseSlow();
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”');
    await expect(page.locator('#concordance-results')).not.toContainText('lookup for “Langsam”');
    await expect(page).toHaveURL(/term=Haus2/);
  });

  test('Concordance Book and grammar drafts stay separate until explicitly applied', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
    const currentResults = page.locator('#concordance-results');
    await expect(currentResults).toContainText('all current Books');
    await expect(currentResults).toContainText('no grammar filter');

    const books = page.locator('details').filter({ hasText: /^Books \(applied:/ });
    await books.locator('summary').click();
    const firstBook = books.locator('input[name="book"]').first();
    await expect(firstBook).toBeVisible();
    const bookTitle = (await firstBook.locator('xpath=..').innerText()).trim();
    await firstBook.check();
    await expect(currentResults).toContainText('all current Books');
    await books.getByRole('button', { name: 'Apply Books' }).click();
    await expect(page).toHaveURL(/book=/);
    await expect(page.locator('.concordance-results')).toHaveCount(1);
    await expect(currentResults).toContainText(bookTitle);
    await expect(currentResults).toContainText('no grammar filter');

    const grammar = page.locator('details').filter({ hasText: /^Grammar \(applied:/ });
    await grammar.locator('summary').click();
    await grammar.getByLabel('Grammar direction').selectOption('own');
    await grammar.getByLabel('Dependency relation').fill('obj');
    await expect(currentResults).toContainText('no grammar filter');
    await grammar.getByRole('button', { name: 'Apply grammar' }).click();
    await expect(page).toHaveURL(/grammar=own.*relation=obj/);
    await expect(currentResults).toContainText('own relation: obj');
  });

test('Concordance disclosures, study return, and paging work across the 25-result boundary', async ({ page }) => {
    const query = 'mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj';
    await page.goto(`/vocabulary/concordance?${query}&page=1`);
    const results = page.locator('#concordance-native-results');
    await expect(results.locator('.concordance-result')).toHaveCount(25);

    const disclosures = results.locator('details.concordance-row');
    const summaries = disclosures.locator('summary');
    await summaries.nth(4).focus();
    await page.keyboard.press('Enter');
    await expect(disclosures.nth(4)).toHaveAttribute('open', '');
    await summaries.nth(5).focus();
    await page.keyboard.press('Space');
    await expect(disclosures.nth(5)).toHaveAttribute('open', '');
    await expect(disclosures.nth(4)).not.toHaveAttribute('open', '');

    const firstStudyLink = results.locator('.concordance-study-link').first();
    await summaries.first().focus();
    await page.keyboard.press('Tab');
    await expect(firstStudyLink).toBeFocused();
    await page.keyboard.press('Enter');
    await page.getByRole('link', { name: 'Return to Concordance results' }).click();
    await expect(page).toHaveURL(/page=1.*#occurrence-fixture-book-0-1$/);
    let returnedURL = new URL(page.url());
    expect(returnedURL.searchParams.getAll('book')).toEqual(['fixture-book']);
    expect(returnedURL.searchParams.get('grammar')).toBe('own');
    expect(returnedURL.searchParams.get('relation')).toBe('obj');
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe('occurrence-fixture-book-0-1');
    expect(returnedURL.searchParams.get('focus')).toBe('occurrence-fixture-book-0-1');

    const next = page.getByRole('navigation', { name: 'Concordance pages' }).getByRole('link', { name: 'Next' });
    await expect(next).toHaveAttribute('href', /page=2/);
    await next.click();
    await expect(page).toHaveURL(/page=2/);
    await expect(results.locator('.concordance-result')).toHaveCount(1);
    const pageTwoID = await disclosures.first().getAttribute('id');
    expect(pageTwoID).toBe('occurrence-fixture-book-25-1');
    await results.locator('.concordance-result').first().locator('.concordance-study-link').click();
    await page.getByRole('link', { name: 'Return to Concordance results' }).click();
    await expect(page).toHaveURL(new RegExp(`page=2.*#${pageTwoID}$`));
    returnedURL = new URL(page.url());
    expect(returnedURL.searchParams.getAll('book')).toEqual(['fixture-book']);
    expect(returnedURL.searchParams.get('grammar')).toBe('own');
    expect(returnedURL.searchParams.get('relation')).toBe('obj');
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe(pageTwoID);
    expect(returnedURL.searchParams.get('focus')).toBe(pageTwoID);
    await expect(page.locator('#concordance-summary').locator('..')).toContainText('page 2');
    await expect(results.locator('.concordance-book-title')).toHaveText('Der lange Weg nach Hause');

    await results.locator('.concordance-result').first().locator('.concordance-study-link').click();
    const returnLink = page.getByRole('link', { name: 'Return to Concordance results' });
    await returnLink.evaluate(link => {
      const returnURL = new URL((link as HTMLAnchorElement).href);
      returnURL.hash = 'occurrence-no-longer-present';
      returnURL.searchParams.set('focus', 'occurrence-no-longer-present');
      (link as HTMLAnchorElement).href = returnURL.href;
    });
    await returnLink.click();
    await expect(results).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.activeElement?.id)).toBe('concordance-summary');
  });

  test('Concordance return focus is server-rendered with JavaScript disabled', async ({ browser }) => {
    const context = await browser.newContext({ javaScriptEnabled: false });
    const noScriptPage = await context.newPage();
    try {
      await signIn(noScriptPage);
      await noScriptPage.goto('/vocabulary/concordance?mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj&page=2&focus=occurrence-fixture-book-25-1#occurrence-fixture-book-25-1');
      const target = noScriptPage.locator('#occurrence-fixture-book-25-1');
      await expect(target).toBeFocused();
      await target.locator('..').locator('.concordance-study-link').click();
      await noScriptPage.getByRole('link', { name: 'Return to Concordance results' }).click();
      await expect(noScriptPage).toHaveURL(/page=2.*#occurrence-fixture-book-25-1$/);
      expect(new URL(noScriptPage.url()).searchParams.get('focus')).toBe('occurrence-fixture-book-25-1');
      await expect(target).toBeFocused();

      await noScriptPage.goto('/vocabulary/concordance?mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj&page=2&focus=occurrence-missing#occurrence-missing');
      await expect(noScriptPage.locator('#concordance-summary')).toBeFocused();
    } finally {
      await context.close();
    }
  });

  test('Concordance keeps its applied results labeled while an enhanced lookup is pending', async ({ page }) => {
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj');
    await expect(page.getByRole('heading', { name: 'Current results' })).toBeVisible();
    await page.evaluate(() => {
      const nativeFetch = window.fetch.bind(window);
      window.fetch = (...args) => new Promise((resolve, reject) => {
        window.setTimeout(() => nativeFetch(...args).then(resolve, reject), 1300);
      });
    });
    await page.getByLabel('Exact term').fill('Haus2');
    await page.getByRole('button', { name: 'Find', exact: true }).click();
    await expect(page.locator('#concordance-pending')).toBeVisible();
    await expect(page.locator('#concordance-pending')).toContainText('previous results remain under their previously applied query and scope');
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
    await expect(page.locator('#concordance-results')).not.toContainText('lookup for “Haus2”');
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus2”', { timeout: 5000 });
    await expect(page).toHaveURL(/term=Haus2/);
    await page.goBack();
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
    await expect(page.locator('#concordance-results')).not.toContainText('lookup for “Haus2”');
    await page.goto('/vocabulary/concordance?mode=surface&term=Haus&book=fixture-book&grammar=own&relation=obj');
    const nextPage = page.getByRole('navigation', { name: 'Concordance pages' }).getByRole('link', { name: 'Next' });
    await nextPage.evaluate(link => {
      const nextURL = new URL((link as HTMLAnchorElement).href);
      nextURL.searchParams.set('rev', 'fixture-stale-revision');
      (link as HTMLAnchorElement).href = nextURL.href;
      link.setAttribute('hx-get', nextURL.href);
    });
    const conflictResponse = page.waitForResponse(response => response.url().includes('fixture-stale-revision'));
    await nextPage.click();
    expect((await conflictResponse).status()).toBe(409);
    await expect(page.locator('#concordance-recovery')).toContainText('Current evidence changed');
    const restart = page.locator('#concordance-recovery').getByRole('link', { name: 'Restart from results' });
    await expect(restart).toHaveAttribute('href', /page=1/);
    await expect(restart).not.toHaveAttribute('href', /rev=/);
    await expect(restart).toHaveAttribute('href', /book=fixture-book/);
    await expect(restart).toHaveAttribute('href', /grammar=own/);
    await expect(restart).toHaveAttribute('href', /relation=obj/);
    await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
    await expect(page).toHaveURL(/term=Haus/);

    for (const [term, statusCode, message] of [['fixture-server-error', 500, 'Concordance lookup was not applied'], ['fixture-server-timeout', 504, 'Concordance server timed out']] as const) {
      await page.getByLabel('Exact term').fill(term);
      const failureResponse = page.waitForResponse(response => response.url().includes(`term=${term}`));
      await page.getByRole('button', { name: 'Find', exact: true }).click();
      expect((await failureResponse).status()).toBe(statusCode);
      await expect(page.locator('#concordance-recovery')).toContainText(message);
      const retryForm = page.locator('#concordance-recovery form');
      await expect(retryForm.getByRole('button', { name: 'Retry Concordance lookup from page 1' })).toBeVisible();
      await expect(retryForm.locator('input[name="term"]')).toHaveValue(term);
      await expect(retryForm.locator('input[name="book"]')).toHaveValue('fixture-book');
      await expect(retryForm.locator('input[name="book"]')).toBeChecked();
      await expect(retryForm.locator('input[name="grammar"]')).toHaveValue('own');
      await expect(retryForm.locator('input[name="relation"]')).toHaveValue('obj');
      await expect(page.locator('#concordance-results')).toContainText('lookup for “Haus”');
      await expect(page.locator('#concordance-results')).not.toContainText(`lookup for “${term}”`);
      await expect(page).toHaveURL(/term=Haus/);
    }
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
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    const retiredBookResponse = await page.goto('/books/fixture-metadata-only');
    expect(retiredBookResponse?.status()).toBe(404);
    const retiredCampaignResponse = await page.goto('/campaigns?message=legacy-bookmark');
    expect(retiredCampaignResponse?.status()).toBe(404);
    await page.goto('/reading');
    await expect(page.locator('.journey-book__title')).toBeVisible();
    await expect(page.locator('#primary-goal-section')).toHaveAttribute('aria-label', 'Current reading');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('Other To Read books');
    await expect(page.locator('#campaign-operations-heading')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Start learning' })).toHaveCount(0);
    await expect(page.getByText(/Der lange Weg nach Hause/).first()).toBeVisible();
    await expect(page.getByText('Other To Read books', { exact: true }).first()).toBeVisible();
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    await expect(page.getByText(/vocabulary-efficient alternative/i)).toHaveCount(0);
    await expect(page.getByText(/advisory order/i)).toHaveCount(0);
    await expect(page.getByText('Route match: familiar German').first()).toBeVisible();
    await expect(page.getByText('Route evidence pending').first()).toBeVisible();
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await page.goto('/vocabulary');
    await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
    await expect(page.locator('form.vocabulary-language-picker')).toHaveCount(0);
    await expect(page.getByText(/active study language.*de/)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Current effective vocabulary' })).toBeVisible();
    await page.getByRole('checkbox', { name: 'Show already accounted-for words' }).check();
    await page.getByRole('button', { name: 'Search', exact: true }).click();
    await expect(page.getByRole('table', { name: 'Current effective vocabulary' }).getByRole('row')).toHaveCount(3);
    await page.getByLabel('Study language').selectOption('fr');
    await expect(page).toHaveURL(/\/vocabulary(?:\?|$)/);
    await page.goto('/vocabulary');
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

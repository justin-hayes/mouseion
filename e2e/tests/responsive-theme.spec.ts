import { expect, Locator, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
  const switcher = page.getByLabel('Study language');
  if (await switcher.inputValue() !== 'de') {
    await switcher.selectOption('de');
    await expect(page).toHaveURL(/\/library$/);
  }
}

const representativePages: Array<[string, RegExp]> = [
  ['/library', /My Books/],
  ['/reading#journey-book-fixture-book', /Der lange Weg nach Hause/],
  ['/jobs/42', /Analysis job #1/],
  ['/books/fixture-book/analyses/fixture-run', /Der lange Weg nach Hause/],
  ['/deck-preparations/fixture-preparation/status', /Deck preparation/],
  ['/jobs', /Jobs/],
  ['/catalogs', /Catalogs/],
  ['/reading', /Reading/],
  ['/vocabulary', /Vocabulary/],
];

async function expectNoPageOverflow(page: Page) {
  const sizes = await page.evaluate(() => ({
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
    viewport: window.innerWidth,
    offenders: Array.from(document.querySelectorAll('body *')).map((node) => {
      const box = node.getBoundingClientRect();
      return { tag: node.tagName, className: (node as HTMLElement).className, right: Math.round(box.right), text: node.textContent?.trim().slice(0, 40) };
    }).filter((node) => node.right > window.innerWidth + 1).slice(0, 8),
  }));
  expect(sizes.document, JSON.stringify(sizes)).toBeLessThanOrEqual(sizes.viewport + 1);
  expect(sizes.body, JSON.stringify(sizes)).toBeLessThanOrEqual(sizes.viewport + 1);
  const regions = await page.locator('.table-region').evaluateAll((nodes) => nodes.map((node) => ({
    overflow: getComputedStyle(node).overflowX,
    right: node.getBoundingClientRect().right,
  })));
  for (const region of regions) {
    expect(region.overflow).toBe('auto');
    expect(region.right).toBeLessThanOrEqual(sizes.viewport + 1);
  }
}

async function expect44pxTouchTargets(controls: Locator) {
  const sizes = await controls.evaluateAll((nodes) => nodes.map((node) => {
    const box = node.getBoundingClientRect();
    return { width: box.width, height: box.height, label: node.textContent?.trim() };
  }));
  for (const size of sizes) {
    expect(size.width, size.label).toBeGreaterThanOrEqual(44);
    expect(size.height, size.label).toBeGreaterThanOrEqual(44);
  }
}

async function textContrast(element: Locator) {
  return element.evaluate((node) => {
    const channels = (color: string) => color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1).map(Number) ?? [];
    const luminance = (color: string) => channels(color).map((channel) => {
      const value = channel / 255;
      return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    }).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
    const style = getComputedStyle(node);
    const foreground = luminance(style.color);
    const background = luminance(style.backgroundColor);
    return (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
  });
}

test.describe('responsive and theme regression coverage', () => {
  test('Jobs uses owned styling for history, recovery, and asynchronous status', async ({ page }) => {
    await signIn(page);

    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/jobs');
      await expect(page.locator('body')).toHaveClass('jobs-shell');
      await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount(1);
      await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
      const analysisHistory = page.getByRole('region', { name: 'Analysis history' });
      await expect(analysisHistory.getByRole('table')).toBeVisible();
      await expect(analysisHistory).toContainText('Completed');
      await expect(analysisHistory).toContainText('Failed');
      await expect(page.getByRole('region', { name: 'Catalog sync history' })).toContainText('Running');
      await expectNoPageOverflow(page);
      await expect(analysisHistory).toHaveCSS('overflow-x', 'auto');

      await page.goto('/jobs/43');
      await expect(page.locator('body')).toHaveClass('jobs-shell');
      const retry = page.getByRole('button', { name: 'Retry analysis' });
      await expect(retry).toBeVisible();
      await retry.focus();
      await expect(retry).toBeFocused();
      expect(await retry.evaluate((node) => getComputedStyle(node).outlineStyle)).toBe('solid');
      const retryBox = await retry.boundingBox();
      expect(retryBox?.width).toBeGreaterThanOrEqual(44);
      expect(retryBox?.height).toBeGreaterThanOrEqual(44);
      expect(await textContrast(retry)).toBeGreaterThanOrEqual(4.5);
      await page.locator('html').evaluate((node) => node.setAttribute('data-theme', 'dark'));
      expect(await textContrast(retry)).toBeGreaterThanOrEqual(4.5);
      await page.emulateMedia({ reducedMotion: 'reduce' });
      const motion = await page.evaluate(() => ({
        requested: matchMedia('(prefers-reduced-motion: reduce)').matches,
        animations: Array.from(document.querySelectorAll('main h1, main h2, button, a[role="button"]')).map((node) => getComputedStyle(node).animationName),
      }));
      expect(motion.requested).toBe(true);
      expect(motion.animations.every((name) => name === 'none')).toBe(true);
      await expectNoPageOverflow(page);
    }
  });

  test('Catalogs owns its styling and preserves legible, recoverable controls', async ({ page, browser }) => {
    await signIn(page);

    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/catalogs');
      await expect(page.locator('body')).toHaveClass('catalogs-shell');
      await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount(1);
      await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
      await expect(page.getByRole('heading', { name: 'Catalogs' })).toBeVisible();
      const addConnectionForm = page.locator('#catalog-connection-form');
      await expect(addConnectionForm.getByLabel('Name', { exact: true })).toBeVisible();
      await expect(addConnectionForm.getByLabel('Catalog URL', { exact: true })).toBeVisible();
      await expect(addConnectionForm.getByLabel('Username (optional)')).toBeVisible();
      await expect(addConnectionForm.getByLabel('Password (optional, encrypted at rest)')).toBeVisible();
      await expect(page.getByRole('status').first()).toContainText('Last synced');
      await expect(page.getByText('Authentication failed for this connection.')).toBeVisible();
      const deletionConfirmations = page.locator('.confirmation');
      await expect(deletionConfirmations).toHaveCount(5);
      expect(await deletionConfirmations.evaluateAll((nodes) => nodes.every((node) => !(node as HTMLDetailsElement).open))).toBe(true);
      await expect(deletionConfirmations.first().locator('summary')).toHaveText('Delete catalog connection');
      await deletionConfirmations.first().locator('summary').click();
      await expect(deletionConfirmations.first()).toContainText('Books already in My Books and their artifacts remain available.');
      await expect(deletionConfirmations.first().getByRole('button', { name: 'Confirm deletion' })).toBeVisible();
      await deletionConfirmations.first().locator('summary').click();

      // Model an unusually long upstream status to ensure it wraps instead of
      // widening the page or pushing controls beyond the viewport.
      await page.locator('#connection-status-fixture-failed-connection').evaluate((node) => {
        const message = document.createElement('p');
        message.textContent = 'Status detail '.repeat(40);
        node.append(message);
      });
      await expectNoPageOverflow(page);

      const controls = page.locator('#main-content :is(button, input:not([type="hidden"]), summary, a[role="button"]):visible');
      await expect44pxTouchTargets(controls);
      await addConnectionForm.getByLabel('Name', { exact: true }).focus();
      expect(await addConnectionForm.getByLabel('Name', { exact: true }).evaluate((node) => getComputedStyle(node).outlineStyle)).toBe('solid');
      expect(await textContrast(page.getByRole('button', { name: 'Add catalog' }))).toBeGreaterThanOrEqual(4.5);
      await page.locator('html').evaluate((node) => node.setAttribute('data-theme', 'dark'));
      expect(await textContrast(page.getByRole('button', { name: 'Add catalog' }))).toBeGreaterThanOrEqual(4.5);
      await expectNoPageOverflow(page);
    }

    const noScriptContext = await browser.newContext({
      baseURL: test.info().project.use.baseURL,
      javaScriptEnabled: false,
    });
    const noScriptPage = await noScriptContext.newPage();
    try {
      await noScriptPage.goto('/login');
      await noScriptPage.getByLabel('Username').fill('fixture-learner');
      await noScriptPage.getByLabel('Password').fill('fixture-password');
      await noScriptPage.getByRole('button', { name: 'Sign in' }).click();
      await expect(noScriptPage).toHaveURL(/\/library/);
      await noScriptPage.goto('/catalogs');
      await expect(noScriptPage.getByLabel('Study language')).toBeVisible();
      await expect(noScriptPage.getByRole('button', { name: 'Switch language' })).toBeVisible();
      await noScriptPage.getByLabel('Study language').selectOption('it');
      await noScriptPage.getByRole('button', { name: 'Switch language' }).click();
      await expect(noScriptPage).toHaveURL(/\/catalogs$/);
      await expect(noScriptPage.getByLabel('Study language')).toHaveValue('it');
      await expect(noScriptPage.getByRole('button', { name: 'Log out' })).toBeVisible();
    } finally {
      await noScriptContext.close();
    }
  });

  test('Reading uses Mouseion-owned styling for the current Book and unordered chooser', async ({ page }) => {
    await signIn(page);
    await page.getByLabel('Study language').selectOption('de');
    await expect(page).toHaveURL(/\/library$/);

    await page.goto('/reading');
    await expect(page.locator('#primary-goal-section')).toBeVisible();
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    const currentBook = page.locator('#primary-goal-section article.journey-book');
    await expect(currentBook).toBeVisible();
    expect(await currentBook.evaluate((node) => getComputedStyle(node).display)).toBe(
      test.info().project.name.startsWith('compact') ? 'flex' : 'grid',
    );
    await expect(currentBook).toContainText('Current reading');
    await expect(currentBook).toContainText('Reserved vocabulary');
    await expect(currentBook.getByRole('link', { name: 'Open focused deck task' })).toHaveAttribute(
      'href',
      /\/reading\/books\/[^/]+\/deck\/preparations\/new$/,
    );

    const controls = page.locator('#primary-goal-section summary:visible, #primary-goal-section button:visible, #primary-goal-section a[role="button"]:visible');
    const controlBoxes = await controls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, text: node.textContent?.trim() };
    }));
    for (const box of controlBoxes) {
      expect(box.width, box.text).toBeGreaterThanOrEqual(44);
      expect(box.height, box.text).toBeGreaterThanOrEqual(44);
    }

    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    await page.goto('/reading/switch');
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.locator('ul.reading-chooser-list').first()).toBeVisible();
    await expect(page.locator('ol.reading-chooser-list')).toHaveCount(0);
    await expect(page.locator('.reading-chooser-book').filter({ hasText: 'Current coverage:' }).first()).toContainText('Current coverage:');
    await expectNoPageOverflow(page);
    const chooserSummaries = await page.locator('.reading-chooser-book details > summary:visible').evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height };
    }));
    for (const summary of chooserSummaries) {
      expect(summary.width).toBeGreaterThanOrEqual(44);
      expect(summary.height).toBeGreaterThanOrEqual(44);
    }

    await page.goto('/library');
    await page.getByLabel('Study language').selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading');
    await expect(page.getByRole('heading', { name: /Donaudampfschifffahrtsgesellschaftskapitänsmütze/ })).toBeVisible();
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
  });

  test('Vocabulary Browse and selection own their responsive styling', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/vocabulary');
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount(0);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toBeVisible();
    await expect(page.getByRole('table', { name: 'Current effective vocabulary' })).toBeVisible();
    await expectNoPageOverflow(page);
    const firstResult = page.getByRole('row').nth(1);
    const firstResultSize = await firstResult.boundingBox();
    expect(firstResultSize).not.toBeNull();
    expect(firstResultSize!.height).toBeGreaterThanOrEqual(44);
    expect(await textContrast(page.locator('.vocabulary-filter button'))).toBeGreaterThanOrEqual(4.5);
    expect(await textContrast(page.locator('#vocabulary-prefix'))).toBeGreaterThanOrEqual(4.5);

    const addFirstResult = firstResult.getByRole('button', { name: 'Select', exact: true });
    if (await addFirstResult.count()) await addFirstResult.click();

    const browseControls = page.locator('.vocabulary-filter input[type="search"], .vocabulary-filter label:has(input[type="checkbox"]), .vocabulary-filter button, #vocabulary-browse-results td button, #vocabulary-browse-results tbody th a, nav[aria-label="Vocabulary views"] a');
    const browseBoxes = await browseControls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, text: node.textContent?.trim() };
    }));
    for (const box of browseBoxes) {
      expect(box.width, box.text).toBeGreaterThanOrEqual(44);
      expect(box.height, box.text).toBeGreaterThanOrEqual(44);
    }

    await page.goto('/vocabulary?q=paging');
    const browsePageLinks = page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link');
    const pageLinkBoxes = await browsePageLinks.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, text: node.textContent?.trim() };
    }));
    for (const box of pageLinkBoxes) {
      expect(box.width, box.text).toBeGreaterThanOrEqual(44);
      expect(box.height, box.text).toBeGreaterThanOrEqual(44);
    }

    const tableRegion = page.locator('.table-region');
    await expect(tableRegion).toBeVisible();
    expect(await tableRegion.evaluate((node) => getComputedStyle(node).overflowX)).toBe('auto');
    await page.locator('#vocabulary-prefix').focus();
    expect(await page.locator('#vocabulary-prefix').evaluate((node) => getComputedStyle(node).outlineStyle)).toBe('solid');

    await page.locator('html').evaluate((node) => node.setAttribute('data-theme', 'dark'));
    expect(await textContrast(page.locator('.vocabulary-filter button'))).toBeGreaterThanOrEqual(4.5);
    expect(await textContrast(page.locator('#vocabulary-prefix'))).toBeGreaterThanOrEqual(4.5);
    await expectNoPageOverflow(page);

    await page.goto('/vocabulary/selection');
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount(0);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Review Browse selection' })).toBeVisible();
    const selectionControls = page.locator('.vocabulary-selection-filter, .vocabulary-selection-page-link, .vocabulary-deck-link, .vocabulary-selection-list button, form[action="/vocabulary/decks"] :is(input:not([type="hidden"]), button), a[role="button"]:visible');
    const selectionBoxes = await selectionControls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, text: node.textContent?.trim() };
    }));
    for (const box of selectionBoxes) {
      expect(box.width, box.text).toBeGreaterThanOrEqual(44);
      expect(box.height, box.text).toBeGreaterThanOrEqual(44);
    }
    await expect(page.getByLabel('Deck name')).toBeVisible();
    expect(await textContrast(page.getByLabel('Deck name'))).toBeGreaterThanOrEqual(4.5);
    expect(await textContrast(page.getByRole('button', { name: 'Create Custom deck' }))).toBeGreaterThanOrEqual(4.5);
    await expectNoPageOverflow(page);

    await page.goto('/vocabulary/selection/clear-confirm');
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Confirm clear selection' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Cancel and keep selection' })).toHaveCSS('min-height', '44px');

    await page.goto('/vocabulary/concordance');
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
  });

  test('Concordance and occurrence review use owned styles at compact, desktop, dark, and 200% text size', async ({ page }) => {
    await signIn(page);
    for (const width of [375, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto('/vocabulary/concordance?mode=surface&term=Haus');
      await expect(page.locator('body')).toHaveClass('vocabulary-shell');
      await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
      await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
      const row = page.locator('#concordance-native-results .concordance-result').first();
      await row.locator('summary').focus();
      await expect(row.locator('summary')).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(row.locator('details')).toHaveAttribute('open', '');
      const source = row.locator('.concordance-context p');
      await source.evaluate((node) => {
        node.textContent = 'Das Haus, das seit vielen Jahren am ruhigen Rand des kleinen Dorfes steht, sieht trotz des langen Winters überraschend gut aus. '.repeat(2);
      });
      await expect(source).toBeVisible();
      await expectNoPageOverflow(page);
      expect(await textContrast(page.locator('.concordance-query input'))).toBeGreaterThanOrEqual(4.5);
      await page.locator('html').evaluate((node) => node.setAttribute('data-theme', 'dark'));
      expect(await textContrast(page.locator('.concordance-query input'))).toBeGreaterThanOrEqual(4.5);
      await page.locator('html').evaluate((node) => node.removeAttribute('data-theme'));
      await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
      await expectNoPageOverflow(page);
    }

    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/reading/books/fixture-lemma-flag-book/lemma-review?form=Weg');
    await expect(page.locator('body')).toHaveClass('reading-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    const sentence = page.locator('.lemma-review .reading-text').first();
    await expect(sentence).toBeVisible();
    await sentence.evaluate((node) => {
      node.textContent = 'Der Weg führt durch die historische Altstadt, vorbei an schmalen Häusern und einem alten Brunnen, bis hin zum Marktplatz, auf dem sich die Bewohner am frühen Morgen treffen.';
    });
    const exactForm = page.getByLabel('Exact observed form');
    await exactForm.focus();
    expect(await exactForm.evaluate((node) => getComputedStyle(node).outlineStyle)).toBe('solid');
    await expectNoPageOverflow(page);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    await page.locator('html').evaluate((node) => node.setAttribute('data-theme', 'dark'));
    expect(await textContrast(exactForm)).toBeGreaterThanOrEqual(4.5);
    await expectNoPageOverflow(page);
  });

  test('shared shell keeps native navigation, language context, focus, and touch targets', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('body > a.skip-link[href="#main-content"]')).toHaveCount(1);
    await expect(page.getByRole('banner')).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Primary navigation' })).toBeVisible();
    await expect(page.getByRole('main')).toHaveAttribute('id', 'main-content');

    await page.keyboard.press('Tab');
    await expect(page.locator('.skip-link')).toBeFocused();
    const skipTop = await page.locator('.skip-link').evaluate((node) => node.getBoundingClientRect().top);
    expect(skipTop).toBeGreaterThanOrEqual(0);

    await signIn(page);
    await page.goto('/library');
    const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
    await page.keyboard.press('Tab');
    await expect(page.locator('.skip-link')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(navigation.getByRole('link', { name: 'Mouseion' })).toBeFocused();
    const navigationFocus = await navigation.getByRole('link', { name: 'Mouseion' }).evaluate((node) => {
      const style = getComputedStyle(node);
      return { outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth };
    });
    expect(navigationFocus.outlineStyle).toBe('solid');
    expect(parseFloat(navigationFocus.outlineWidth)).toBeGreaterThanOrEqual(3);

    await expect(navigation.getByRole('link', { name: 'My Books' })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByLabel('Study language')).toBeVisible();
    await expect(navigation.getByRole('link', { name: 'Reading' })).toBeVisible();
    const controls = navigation.locator('a, select, button:visible');
    const boxes = await controls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { text: node.textContent?.trim(), width: box.width, height: box.height, left: box.left, right: box.right };
    }));
    for (const box of boxes) {
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.left, box.text).toBeGreaterThanOrEqual(-1);
      expect(box.right, box.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }

    const colors = await page.locator('.site-header').evaluate((node) => {
      const style = getComputedStyle(node);
      const probe = document.createElement('span');
      probe.style.backgroundColor = 'var(--mouseion-color-surface-quiet)';
      document.body.append(probe);
      const tokenBackground = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return { background: style.backgroundColor, tokenBackground };
    });
    expect(colors.background).not.toBe('rgba(0, 0, 0, 0)');
    expect(colors.background).toBe(colors.tokenBackground);

    await page.goto('/deck-preparations/fixture-preparation/status');
    const secondaryAction = page.locator('.action-group .button--outline').first();
    await expect(secondaryAction).toBeVisible();
    await secondaryAction.hover();
    expect(await textContrast(secondaryAction)).toBeGreaterThanOrEqual(4.5);

    await page.goto('/library');
    const activeNavigation = page.getByRole('navigation', { name: 'Primary navigation' });
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    await expect(activeNavigation.getByRole('link', { name: 'Vocabulary' })).toBeVisible();
    await expect(page.getByLabel('Study language')).toBeVisible();
  });

  test('sign-in controls are styled, focused, and reachable at 200 percent text size', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('link[rel="stylesheet"][href="/static/login.css"]')).toHaveCount(1);
    const shell = await page.locator('body > header.site-header').evaluate((node) => {
      const style = getComputedStyle(node);
      return style.maxWidth;
    });
    expect(shell).toBe('none');

    const username = page.getByLabel('Username');
    const initial = await username.evaluate((node) => {
      const style = getComputedStyle(node);
      const box = node.getBoundingClientRect();
      return { background: style.backgroundColor, border: style.borderTopWidth, height: box.height };
    });
    expect(initial.background).not.toBe('rgba(0, 0, 0, 0)');
    expect(parseFloat(initial.border)).toBeGreaterThan(0);
    expect(initial.height).toBeGreaterThanOrEqual(48);

    expect(await textContrast(username)).toBeGreaterThanOrEqual(4.5);
    expect(await textContrast(page.locator('.login-screen button'))).toBeGreaterThanOrEqual(4.5);

    await username.focus();
    const focus = await username.evaluate((node) => getComputedStyle(node).outlineStyle);
    expect(focus).toBe('solid');

    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    for (const control of [username, page.getByLabel('Password'), page.getByRole('button', { name: 'Sign in' })]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(-1);
      expect(box!.x + box!.width).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
  });

  test('Reading respects the reduced-motion preference', async ({ page }) => {
    await signIn(page);
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto('/reading');
    const motion = await page.evaluate(() => ({
      requested: matchMedia('(prefers-reduced-motion: reduce)').matches,
      animations: Array.from(document.querySelectorAll('main h1, main h2, button, a[role="button"]')).map((node) => getComputedStyle(node).animationName),
    }));
    expect(motion.requested).toBe(true);
    expect(motion.animations.every((name) => name === 'none')).toBe(true);
  });

  test('authenticated shell and primary workflows remain readable without compact page overflow', async ({ page }) => {
    await signIn(page);
    for (const [url, heading] of representativePages) {
      await page.goto(url);
      await expect(page.getByRole('heading', { name: heading }).first()).toBeVisible();
      if (test.info().project.name.startsWith('compact')) await expectNoPageOverflow(page);
    }
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') {
      await switcher.selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
    }
    await expect(page.locator('.library-grid .library-book__identity-link[href="/reading#journey-book-fixture-book"]')).toBeVisible();
    await expect(page.locator('.library-grid a[href="/books/fixture-failed"]')).toHaveCount(0);
    await expect(page.locator('.library-grid a[href="/books/fixture-edge-content"]')).toHaveCount(0);
    await expect(page.locator('.library-grid a[href="/books/fixture-empty"]')).toHaveCount(0);
    expect(await page.locator('.library-book').filter({ has: page.locator('.library-book__identity-link[href^="/reading#"]') }).count()).toBeGreaterThan(0);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    const libraryControls = page.locator('.library-grid button:visible, .library-grid a.library-book__journey-action:visible, .library-grid > .library-book > details > summary:visible');
    const libraryControlBoxes = await libraryControls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
    }));
    for (const control of libraryControlBoxes) {
      expect(control.width, control.text).toBeGreaterThanOrEqual(44);
      expect(control.height, control.text).toBeGreaterThanOrEqual(44);
      expect(control.left, control.text).toBeGreaterThanOrEqual(-1);
      expect(control.right, control.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    if (test.info().project.name.startsWith('compact')) {
      // The Italian Journey retains the long-title content for narrow-layout
      // coverage without relying on the retired unassessed Book page.
      await page.goto('/library');
      await page.getByLabel('Study language').selectOption('it');
      await expect(page).toHaveURL(/\/library$/);
      await page.goto('/reading');
      await expect(page.getByRole('heading', { name: /Donaudampfschifffahrtsgesellschaftskapitänsmütze/ })).toBeVisible();
      await expectNoPageOverflow(page);
    }
  });

  test('dense analysis, deck provenance, errors, and import surfaces expose realistic content', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book')).toBeVisible();
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' }).locator('tbody tr')).toHaveCount(18);
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.getByRole('heading', { name: "This Book's vocabulary study" })).toHaveCount(0);
    await page.goto('/jobs/43');
    await expect(page.getByRole('alert')).toContainText(/Retry the analysis when you are ready/);
    await expect(page.getByRole('button', { name: 'Retry analysis' })).toBeVisible();
    await page.goto('/vocabulary/import');
    await expect(page.getByRole('heading', { name: 'Vocabulary · Import known vocabulary', exact: true })).toBeVisible();
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href="/static/vocabulary.css"]')).toHaveCount(1);
    await expect(page.locator('link[rel="stylesheet"][href="/static/catalog-ops.css"]')).toHaveCount(0);
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByLabel('UTF-8 lemma file')).toBeVisible();
    const importControls = page.locator('nav[aria-label="Vocabulary views"] a, form[action="/vocabulary/import"] input[type="file"], form[action="/vocabulary/import"] button');
    await expect44pxTouchTargets(importControls);
    await expectNoPageOverflow(page);
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);

    const importForm = page.locator('form[action="/vocabulary/import"]');
    await importForm.locator('input[type="file"]').setInputFiles({ name: 'empty.txt', mimeType: 'text/plain', buffer: Buffer.alloc(0) });
    await importForm.getByRole('button', { name: 'Import known vocabulary' }).click();
    await expect(page.getByRole('alert')).toContainText('Choose a non-empty UTF-8 text file to import.');
    await expect(page.getByLabel('UTF-8 lemma file')).toBeVisible();

    await page.getByLabel('UTF-8 lemma file').setInputFiles({ name: 'known.txt', mimeType: 'text/plain', buffer: Buffer.from('selten\n') });
    await page.getByRole('button', { name: 'Import known vocabulary' }).click();
    await expect(page.locator('#vocabulary-results').getByRole('status')).toContainText(/continuing in the background|import is complete/i);

    await page.getByLabel('UTF-8 lemma file').setInputFiles({ name: 'mixed.txt', mimeType: 'text/plain', buffer: Buffer.from('Haus\nbad\tline\n') });
    await page.getByRole('button', { name: 'Import known vocabulary' }).click();
    const rejectedRows = page.locator('#vocabulary-results').getByRole('region', { name: 'Rejected vocabulary rows' });
    await expect(rejectedRows).toContainText('bad\tline');
    await expect(rejectedRows).toContainText('expected exactly one lemma');
    await expectNoPageOverflow(page);
  });

  test('saved Custom deck editing and preparation use owned responsive styling', async ({ page }) => {
    await signIn(page);
    await page.goto('/vocabulary/selection/clear-confirm');
    const csrf = await page.locator('input[name="csrf_token"]').first().inputValue();
    const cleared = await page.request.post('/vocabulary/selection/clear', { form: { csrf_token: csrf } });
    expect(cleared.ok()).toBeTruthy();
    const selected = await page.request.post('/vocabulary/selection/add', {
      form: { csrf_token: csrf, lemma: 'gehen', upos: 'VERB' },
    });
    expect(selected.ok()).toBeTruthy();
    const selectedWithEvidence = await page.request.post('/vocabulary/selection/add', {
      form: { csrf_token: csrf, lemma: 'fixture-prepare-ready', upos: 'VERB' },
    });
    expect(selectedWithEvidence.ok()).toBeTruthy();

    await page.goto('/vocabulary/selection');
    const name = `Responsive review ${test.info().project.name}`;
    await page.getByLabel('Deck name').fill(name);
    await page.getByRole('button', { name: 'Create Custom deck' }).click();
    await expect(page.getByRole('heading', { name: `Custom deck · ${name}` })).toBeVisible();
    const deckURL = new URL(page.url()).pathname;
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: '2 selected identities' })).toContainText('1 missing current evidence');
    const prepareForm = page.locator('form.custom-deck-prepare');
    await expect(prepareForm).toHaveAttribute('method', 'post');
    await expect(prepareForm).toHaveAttribute('action', /\/vocabulary\/decks\/[^/]+\/preparations$/);
    await expect(prepareForm).not.toHaveAttribute('hx-post', /.+/);
    await expect(prepareForm.getByRole('button', { name: 'Prepare deck' })).toBeVisible();
    await expect(page.getByRole('complementary', { name: 'Anki import and overlap warning' })).toBeVisible();

    const controls = page.locator('nav[aria-label="Vocabulary views"] a, .custom-deck-form :is(input:not([type="hidden"]), select, button), .custom-deck-prepare button, .custom-deck-identity-list :is(a, button)');
    await expect44pxTouchTargets(controls);
    await expectNoPageOverflow(page);

    await prepareForm.getByRole('button', { name: 'Prepare deck' }).click();
    await expect(page).toHaveURL(/\/vocabulary\/deck-preparations\/fixture-custom-deck-preparation-/);
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByRole('status')).toContainText('queued');
    await expect44pxTouchTargets(page.locator('a.vocabulary-action-link, button:visible'));
    await expectNoPageOverflow(page);

    await page.goto(deckURL + '/delete-confirm');
    await expect(page.locator('body')).toHaveClass('vocabulary-shell');
    await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
    await expect(page.getByText('Mouseion cannot revoke APKG files already downloaded', { exact: false })).toBeVisible();
    await expect44pxTouchTargets(page.getByRole('button', { name: 'Confirm delete Custom deck' }));
    await expectNoPageOverflow(page);
    await page.getByRole('button', { name: 'Confirm delete Custom deck' }).click();
    await expect(page).toHaveURL(/\/vocabulary\/selection$/);
  });

  test('action order and compact touch targets preserve reachability', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');
    const current = page.locator('#primary-goal-section');
    if (await current.locator('.journey-book--goal').count()) {
      await expect(current.getByRole('button', { name: 'Switch current reading' })).toBeVisible();
      const finishSummary = current.locator('details').filter({ hasText: 'Mark reading finished' }).locator('summary');
      await expect(finishSummary).toBeVisible();
      const primaryActions = await current.locator('.goal-card__actions > a[role="button"], .goal-card__actions > .confirmation > summary').evaluateAll((nodes) => nodes.map((node) => {
        const box = node.getBoundingClientRect();
        return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
      }));
      expect(primaryActions.length).toBeGreaterThan(0);
      for (const action of primaryActions) {
        expect(action.width, action.text).toBeGreaterThanOrEqual(44);
        expect(action.height, action.text).toBeGreaterThanOrEqual(44);
        expect(action.left, action.text).toBeGreaterThanOrEqual(-1);
        expect(action.right, action.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
      }
    } else {
      const eligibleCandidate = page.locator('li.reading-chooser-book').filter({ has: page.getByRole('button', { name: 'Confirm start reading' }) }).first();
      await expect(eligibleCandidate).toBeVisible();
      await eligibleCandidate.locator('details').filter({ hasText: 'Start reading' }).locator('summary').click();
      const startAction = eligibleCandidate.getByRole('button', { name: 'Confirm start reading' });
      await expect(startAction).toBeVisible();
      const box = await startAction.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(-1);
      expect(box!.x + box!.width).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    const order = await page.locator('.action-group').evaluateAll((groups) => groups.map((group) => {
      const controls = Array.from(group.querySelectorAll('button, a[role="button"]'));
      return controls.map((control) => control.classList.contains('secondary'));
    }));
    for (const controls of order) if (controls.length) expect(controls[0]).toBe(false);
    const controls = await page.locator('button:visible, a[role="button"]:visible').evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
    }));
    for (const control of controls) {
      expect(control.width, control.text).toBeGreaterThanOrEqual(44);
      expect(control.height, control.text).toBeGreaterThanOrEqual(44);
      expect(control.left, control.text).toBeGreaterThanOrEqual(-1);
      expect(control.right, control.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book a[href$="/deck/preparations/new"]')).toBeVisible();
  });

  test('standard Journey rows preserve a readable book column beside controls', async ({ page }) => {
    test.skip(!test.info().project.name.startsWith('desktop'), 'This contract applies to the standard desktop layout.');
    await signIn(page);
    await page.goto('/reading');
    const rows = await page.locator('.journey-book:not(.journey-book--goal)').evaluateAll((nodes) => nodes.map((node) => {
      const book = node.querySelector<HTMLElement>('.journey-book__identity')?.getBoundingClientRect();
      const controls = node.querySelector<HTMLElement>('.journey-book__controls')?.getBoundingClientRect();
      return { bookWidth: book?.width ?? 0, controlsWidth: controls?.width ?? 0, rowWidth: node.getBoundingClientRect().width };
    }));
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(row.bookWidth).toBeGreaterThan(200);
      expect(row.controlsWidth).toBeLessThan(row.rowWidth * 0.6);
    }
  });

  test('Journey thumbnails stay aligned, quiet, and outside the keyboard order', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');

    const cards = page.locator('.journey-book');
    const thumbnails = cards.locator('.journey-book__cover');
    await expect(thumbnails).toHaveCount(await cards.count());
    await expect(page.locator('.journey-book--goal .journey-book__cover img')).toHaveAttribute('alt', '');
    await expect(page.locator('.journey-book--goal .journey-book__cover img')).toHaveAttribute('src', '/books/fixture-book/cover');
    await expect(page.locator('.journey-book__cover .book-cover-media__placeholder[aria-hidden="true"]').first()).toBeVisible();
    await expect(page.locator('.journey-book__cover a, .journey-book__cover button, .journey-book__cover summary')).toHaveCount(0);

    const layout = await thumbnails.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      const card = node.parentElement?.getBoundingClientRect();
      return { width: box.width, height: box.height, right: box.right, cardRight: card?.right ?? 0 };
    }));
    for (const item of layout) {
      expect(item.width).toBeGreaterThan(0);
      expect(item.height).toBeGreaterThan(0);
      expect(item.right).toBeLessThanOrEqual(item.cardRight + 1);
    }
    await expectNoPageOverflow(page);
  });

  test('Journey titles and actions remain reachable at 200 percent text size', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expect(page.getByRole('heading', { name: 'To Read books', exact: true })).toBeVisible();
    await expect(page.locator('.journey-book__controls').first()).toBeVisible();
    await expectNoPageOverflow(page);
  });

  test('semantic status text, readable measures, and live theme tokens meet contrast targets', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book/analyses/fixture-run');
    const semantics = await page.locator('.status-badge').evaluateAll((nodes) => nodes.map((node) => ({
      text: node.textContent?.trim(), color: getComputedStyle(node).color, border: getComputedStyle(node).borderTopColor,
    })));
    expect(semantics.length).toBeGreaterThan(0);
    for (const item of semantics) {
      expect(item.text).toBeTruthy();
      expect(item.color).not.toBe('rgba(0, 0, 0, 0)');
      expect(item.border).toBe(item.color);
    }
    const measure = await page.evaluate(() => {
      const root = getComputedStyle(document.documentElement);
      const reading = document.querySelector<HTMLElement>('.reading-text, .reading-width');
      return { token: root.getPropertyValue('--mouseion-width-reading').trim(), rootFontSize: parseFloat(root.fontSize), width: reading?.getBoundingClientRect().width ?? null };
    });
    expect(measure.token).toBe('42rem');
    if (measure.width !== null) expect(measure.width).toBeLessThanOrEqual(42 * measure.rootFontSize + 1);

    const contrast = await page.evaluate(() => {
      const parse = (value: string) => value.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1).map(Number) ?? [];
      const luminance = (value: string) => {
        const rgb = parse(value).map((channel) => { const s = channel / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4; });
        return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
      };
      const ratio = (foreground: string, background: string) => { const a = luminance(foreground); const b = luminance(background); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05); };
      const probe = (foreground: string, background: string) => { const node = document.createElement('span'); node.style.color = 'var(' + foreground + ')'; node.style.backgroundColor = 'var(' + background + ')'; node.textContent = 'probe'; document.body.append(node); const style = getComputedStyle(node); const value = ratio(style.color, style.backgroundColor); node.remove(); return value; };
      return Object.fromEntries(['--mouseion-color-text', '--mouseion-color-text-muted', '--mouseion-color-info', '--mouseion-color-success', '--mouseion-color-warning', '--mouseion-color-danger', '--mouseion-color-focus'].map((token) => [token, probe(token, '--mouseion-color-surface')]));
    });
    for (const [token, value] of Object.entries(contrast)) expect(value, token).toBeGreaterThanOrEqual(token === '--mouseion-color-focus' ? 3 : 4.5);
  });
});

test.describe('sign-in server-rendered recovery without JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  test('invalid credentials return an accessible styled error and remain retryable', async ({ page }) => {
    await page.goto('/login');
    const username = page.getByLabel('Username');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL('/login');
    expect(await username.evaluate((node) => (node as HTMLInputElement).validity.valueMissing)).toBe(true);

    await username.fill('fixture-learner');
    await page.getByLabel('Password').fill('incorrect-password');
    await page.getByRole('button', { name: 'Sign in' }).click();

    await expect(page).toHaveURL(/\/login\?error=/);
    await expect(page.getByRole('alert')).toHaveText('Invalid credentials');
    await expect(page.locator('link[href="/static/login.css"]')).toHaveCount(1);
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
  });
});

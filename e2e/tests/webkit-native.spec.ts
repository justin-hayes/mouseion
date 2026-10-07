import { expect, Page, test } from '@playwright/test';
import { renderedTextContrast } from '../support/contrast';

async function signIn(page: Page, username = 'fixture-learner', password = 'fixture-password') {
  await page.goto('/login');
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByLabel('Password').press('Enter');
  await expect(page).toHaveURL(/\/library$/);
}

async function chooseStudyLanguage(page: Page, language: string, noJavaScript = false) {
  const switcher = page.getByLabel('Study language');
  await switcher.selectOption(language);
  if (noJavaScript) await page.getByRole('button', { name: 'Switch language' }).click();
}

async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
}

async function expectMainWithinViewport(page: Page) {
  const bounds = await page.locator('main#main-content').evaluate(element => {
    const rect = element.getBoundingClientRect();
    return { left: rect.left, right: rect.right, width: rect.width, viewport: window.innerWidth };
  });
  expect(bounds.width).toBeGreaterThan(0);
  expect(bounds.left).toBeGreaterThanOrEqual(0);
  expect(bounds.right).toBeLessThanOrEqual(bounds.viewport);
}

async function lookupScopedConcordance(page: Page) {
  await page.goto('/vocabulary/concordance');
  await page.getByLabel('Exact term').fill('Haus');
  await page.getByRole('button', { name: 'Find', exact: true }).click();
  const booksScope = page.getByText('Books (applied: all current Books)', { exact: true });
  await booksScope.click();
  const bookForm = page.locator('.concordance-scopes form').first();
  await bookForm.getByLabel('Der lange Weg nach Hause').check();
  await bookForm.getByRole('button', { name: 'Apply Books' }).click();
}

async function expectConcordanceListSemantics(page: Page) {
  const list = page.locator('#concordance-native-results');
  const snapshot = await list.ariaSnapshot();
  expect(snapshot).toContain('- list:');
  expect(snapshot).toContain('- listitem');
  expect(snapshot).toContain('Der lange Weg nach Hause');
  expect(snapshot).toContain('Haus');
}

test.describe('native WebKit smoke journey', () => {
  test('sign-in renders associated native controls and recovers from invalid credentials', async ({ page }) => {
    await page.goto('/login');
    const username = page.getByLabel('Username');
    const password = page.getByLabel('Password');
    await expect(username).toHaveAttribute('name', 'username');
    await expect(password).toHaveAttribute('name', 'password');
    await expect(password).toHaveAttribute('type', 'password');

    await username.fill('fixture-learner');
    await password.fill('incorrect-password');
    await password.press('Enter');
    await expect(page).toHaveURL(/\/login\?/);
    await expect(page.getByRole('alert')).toBeVisible();
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByLabel('Password').press('Enter');
    await expect(page).toHaveURL(/\/library$/);
    await expect(page.getByRole('heading', { name: /My Books|Welcome/i }).first()).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });

  test('authenticated shell navigation, language return, and Catalogs controls are usable', async ({ page }) => {
    await signIn(page);
    const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
    const destinations = [
      { name: 'My Books', href: '/library', active: '/library' },
      { name: 'Reading', href: '/reading', active: '/reading' },
      { name: 'Vocabulary', href: '/vocabulary', active: '/vocabulary' },
      { name: 'Catalogs', href: '/catalogs', active: '/catalogs' },
    ];
    for (const destination of destinations) {
      const link = navigation.getByRole('link', { name: destination.name, exact: true });
      await expect(link).toHaveAttribute('href', destination.href);
      await link.click();
      await expect(page).toHaveURL(new RegExp(`${destination.active}(?:$|[?#])`));
      await expect(navigation.getByRole('link', { name: destination.name, exact: true })).toHaveAttribute('aria-current', 'page');
    }

    await page.goto('/reading');
    await chooseStudyLanguage(page, 'it');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.locator('main h1')).toContainText(/Italian/);
    await expect(page.getByLabel('Study language')).toHaveValue('it');

    await page.goto('/catalogs');
    const addForm = page.locator('#catalog-connection-form form');
    await expect(addForm.getByLabel('Name', { exact: true })).toBeVisible();
    await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveAttribute('type', 'url');
    await expect(addForm.getByLabel('Username (optional)', { exact: true })).toBeVisible();
    await expect(addForm.getByLabel('Password (optional, encrypted at rest)', { exact: true })).toHaveAttribute('type', 'password');
    await expect(addForm).toHaveAttribute('method', 'post');
    await expect(addForm).toHaveAttribute('action', '/connections');
    await addForm.getByLabel('Name', { exact: true }).fill('WebKit smoke catalog');
    await addForm.getByLabel('Catalog URL', { exact: true }).fill('https://catalog.example.invalid/opds');
    await addForm.getByLabel('Username (optional)', { exact: true }).fill('fixture-user');
    await addForm.getByLabel('Password (optional, encrypted at rest)', { exact: true }).fill('fixture-secret');
    await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveValue('https://catalog.example.invalid/opds');
    const addCatalog = addForm.getByRole('button', { name: 'Add catalog', exact: true });
    await expect(addCatalog).toBeEnabled();
    const targetSize = await addCatalog.evaluate(element => {
      const rect = element.getBoundingClientRect();
      return { width: rect.width, height: rect.height };
    });
    expect(targetSize.width).toBeGreaterThanOrEqual(44);
    expect(targetSize.height).toBeGreaterThanOrEqual(44);
    expect(await addCatalog.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
    const themeTokens = ['--mouseion-color-text', '--mouseion-color-text-muted'];
    await page.evaluate(tokens => {
      for (const token of tokens) {
        const probe = document.createElement('span');
        probe.dataset.contrastToken = token;
        probe.style.color = `var(${token})`;
        probe.style.backgroundColor = 'var(--mouseion-color-surface)';
        probe.textContent = token;
        document.body.append(probe);
      }
    }, themeTokens);
    for (const token of themeTokens) {
      expect(await page.locator(`[data-contrast-token="${token}"]`).evaluate(renderedTextContrast), token).toBeGreaterThanOrEqual(4.5);
    }
    await page.locator('[data-contrast-token]').evaluateAll(nodes => nodes.forEach(node => node.remove()));

    const confirmation = page.locator('#connection-fixture-connection details').last();
    const summary = confirmation.locator('summary');
    await expect(confirmation).not.toHaveAttribute('open', '');
    await summary.focus();
    await page.keyboard.press('Enter');
    await expect(confirmation).toHaveAttribute('open', '');
    await page.keyboard.press('Space');
    await expect(confirmation).not.toHaveAttribute('open', '');
    await expectNoHorizontalOverflow(page);
    await expectMainWithinViewport(page);

    await page.goto('/library');
    await chooseStudyLanguage(page, 'de');
  });

  test('shared navigation and study-language control stay visually consistent across destinations', async ({ page }) => {
    await signIn(page);
    const measurements = [] as Array<{ destination: string; labelDirection: string; selectFont: string; selectHeight: number; selectRadius: string; currentBackground: string; currentDecoration: string }>;
    for (const destination of [
      { name: 'My Books', path: '/library' },
      { name: 'Reading', path: '/reading' },
      { name: 'Vocabulary', path: '/vocabulary' },
      { name: 'Catalogs', path: '/catalogs' },
    ]) {
      await page.goto(destination.path);
      const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
      const current = navigation.getByRole('link', { name: destination.name, exact: true });
      await expect(current).toHaveAttribute('aria-current', 'page');
      await expect(current).not.toHaveClass(/\bbtn\b/);
      const style = await page.getByLabel('Study language').evaluate(select => {
        const label = select.closest('label')!;
        const labelStyle = getComputedStyle(label);
        const selectStyle = getComputedStyle(select);
        const rect = select.getBoundingClientRect();
        return {
          labelDirection: labelStyle.flexDirection,
          selectFont: `${selectStyle.fontFamily}|${selectStyle.fontSize}|${selectStyle.lineHeight}`,
          selectHeight: rect.height,
          selectRadius: selectStyle.borderRadius,
        };
      });
      const currentStyle = await current.evaluate(link => {
        const style = getComputedStyle(link);
        return { background: style.backgroundColor, decoration: style.textDecorationLine };
      });
      measurements.push({ destination: destination.name, ...style, currentBackground: currentStyle.background, currentDecoration: currentStyle.decoration });
    }

    expect(new Set(measurements.map(item => item.labelDirection)).size).toBe(1);
    expect(new Set(measurements.map(item => item.selectFont)).size).toBe(1);
    expect(new Set(measurements.map(item => item.selectHeight)).size).toBe(1);
    expect(new Set(measurements.map(item => item.selectRadius)).size).toBe(1);
    expect(measurements.every(item => item.labelDirection === 'row')).toBe(true);
    expect(measurements.every(item => item.selectHeight >= 44)).toBe(true);
    expect(measurements.every(item => item.currentDecoration.includes('underline'))).toBe(true);
    expect(new Set(measurements.map(item => item.currentBackground)).size, JSON.stringify(measurements)).toBe(1);
    expect(measurements.every(item => item.currentBackground !== 'rgb(36, 87, 178)')).toBe(true);
    await expectNoHorizontalOverflow(page);
  });

  test('skip link and keyboard focus remain available at compact and desktop widths', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');
    await page.evaluate(() => document.activeElement instanceof HTMLElement && document.activeElement.blur());
    await page.keyboard.press('Tab');
    const skipLink = page.getByRole('link', { name: 'Skip to main content' });
    await expect(skipLink).toBeFocused();
    await expect(skipLink).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page.locator('main#main-content')).toBeFocused();

    const focusStyle = await page.evaluate(() => {
      const element = document.activeElement;
      if (!(element instanceof HTMLElement)) return false;
      const style = getComputedStyle(element);
      return style.outlineStyle !== 'none' || style.outlineWidth !== '0px' || style.boxShadow !== 'none';
    });
    expect(focusStyle).toBe(true);
    await expectNoHorizontalOverflow(page);
  });

  test('Concordance uses one meaningful results list with native disclosure and study navigation', async ({ page }) => {
    await signIn(page);
    await lookupScopedConcordance(page);

    const results = page.locator('#concordance-results');
    await expect(results).toContainText('Applied exact observed surface lookup for “Haus” · Der lange Weg nach Hause · no grammar filter');
    await expect(results).toContainText('Results 1–25 · page 1');
    await expect(page.locator('#concordance-native-results li')).toHaveCount(25);
    await expectConcordanceListSemantics(page);
    await expect(page.locator('.concordance-results')).toHaveCount(1);
    await expect(page.locator('#concordance-results [data-concordance-data], #concordance-results mouseion-concordance')).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
    await expectMainWithinViewport(page);

    const firstRow = page.locator('#concordance-native-results details').first();
    const secondRow = page.locator('#concordance-native-results details').nth(1);
    const firstSummary = firstRow.locator('summary');
    await expect(firstSummary).toContainText('Der lange Weg nach Hause');
    await expect(firstSummary).toContainText('Haus');

    // Pointer activation opens native context; keyboard focus remains visible.
    await firstSummary.click();
    await expect(firstRow).toHaveAttribute('open', '');
    await expect(firstRow.locator('.concordance-context')).toContainText('Das Haus sieht gut aus.');
    await expect(firstRow.locator('.concordance-observed-target')).toHaveText('Haus');
    await firstSummary.focus();
    const focusVisible = await firstSummary.evaluate(element => {
      const style = getComputedStyle(element);
      return style.outlineStyle !== 'none' || style.outlineWidth !== '0px' || style.boxShadow !== 'none';
    });
    expect(focusVisible).toBe(true);
    await page.keyboard.press('Enter');
    await expect(firstRow).not.toHaveAttribute('open', '');
    await page.keyboard.press('Space');
    await expect(firstRow).toHaveAttribute('open', '');

    // Native details[name] grouping is not uniform across WebKit versions.
    // Both native outcomes are valid; the newly activated row must open.
    await secondRow.locator('summary').focus();
    await page.keyboard.press('Enter');
    await expect(secondRow).toHaveAttribute('open', '');
    const firstRemainsOpen = await firstRow.evaluate(element => element.hasAttribute('open'));
    const secondRemainsOpen = await secondRow.evaluate(element => element.hasAttribute('open'));
    expect([[false, true], [true, true]]).toContainEqual([firstRemainsOpen, secondRemainsOpen]);

    const studyLink = firstRow.locator('xpath=following-sibling::a');
    await firstSummary.focus();
    await page.keyboard.press('Tab');
    await expect(studyLink).toBeFocused();
    await expect(studyLink).toHaveAccessibleName('Study this sentence and its syntax');
    await expect(studyLink).toBeVisible();
    await studyLink.click();
    await expect(page.getByRole('heading', { name: 'Study this sentence and its syntax' })).toBeVisible();
    await expect(page.locator('.sentence-study-text')).toContainText('Das Haus sieht gut aus.');
    await expect(page.getByText('Identified target: Haus', { exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'Return to Concordance results' }).click();
    await expect(page).toHaveURL(/\/vocabulary\/concordance\?.*book=fixture-book.*#occurrence-fixture-book-0-1$/);
    await expect(page.locator('#concordance-results')).toContainText('Results 1–25 · page 1');
    await expect(page.locator('#occurrence-fixture-book-0-1')).toBeFocused();
  });

  test('Concordance lookup, context, and study navigation work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      javaScriptEnabled: false,
      viewport: test.info().project.name.includes('compact') ? { width: 375, height: 667 } : { width: 1280, height: 800 },
      colorScheme: test.info().project.name.endsWith('-dark') ? 'dark' : 'light',
    });
    const page = await context.newPage();
    try {
      await signIn(page);
      await lookupScopedConcordance(page);
      const results = page.locator('#concordance-results');
      await expect(results).toContainText('Applied exact observed surface lookup for “Haus” · Der lange Weg nach Hause · no grammar filter');
      await expect(results).toContainText('Results 1–25 · page 1');
      await expect(page.locator('#concordance-native-results li')).toHaveCount(25);
      await expectConcordanceListSemantics(page);
      await expect(page.locator('.concordance-results')).toHaveCount(1);
      await expectNoHorizontalOverflow(page);
      await expectMainWithinViewport(page);

      const firstRow = page.locator('#concordance-native-results details').first();
      await firstRow.locator('summary').click();
      await expect(firstRow).toHaveAttribute('open', '');
      await expect(firstRow.locator('.concordance-context')).toContainText('Das Haus sieht gut aus.');
      await expect(firstRow.locator('.concordance-observed-target')).toHaveText('Haus');
      const studyLink = firstRow.locator('xpath=following-sibling::a');
      await expect(studyLink).toHaveAccessibleName('Study this sentence and its syntax');
      await studyLink.click();
      await expect(page.getByRole('heading', { name: 'Study this sentence and its syntax' })).toBeVisible();
      await expect(page.locator('.sentence-study-text')).toContainText('Das Haus sieht gut aus.');
      await expect(page.getByText('Identified target: Haus', { exact: true })).toBeVisible();
      await page.getByRole('link', { name: 'Return to Concordance results' }).click();
      await expect(page).toHaveURL(/\/vocabulary\/concordance\?.*book=fixture-book.*#occurrence-fixture-book-0-1$/);
      await expect(page.locator('#concordance-results')).toContainText('Results 1–25 · page 1');
      await expect(page.locator('#occurrence-fixture-book-0-1')).toBeFocused();
    } finally {
      await context.close();
    }
  });

  test('server-rendered sign-in and authenticated native forms work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      javaScriptEnabled: false,
      viewport: test.info().project.name.includes('compact') ? { width: 375, height: 667 } : { width: 1280, height: 800 },
      colorScheme: test.info().project.name.endsWith('-dark') ? 'dark' : 'light',
    });
    const page = await context.newPage();
    try {
      await signIn(page);
      await expect(page).toHaveURL(/\/library$/);
      await expect(page.locator('nav.site-header__nav')).toBeVisible();
      const vocabularyLink = page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link', { name: 'Vocabulary', exact: true });
      await expect(vocabularyLink).toHaveAttribute('href', '/vocabulary');
      await vocabularyLink.click();
      await expect(page).toHaveURL(/\/vocabulary$/);
      await page.goto('/reading');
      await chooseStudyLanguage(page, 'it', true);
      await expect(page).toHaveURL(/\/reading$/);
      await expect(page.locator('main h1')).toContainText(/Italian/);

      await page.goto('/catalogs');
      const confirmation = page.locator('#connection-fixture-connection details').last();
      const summary = confirmation.locator('summary');
      await expect(confirmation).not.toHaveAttribute('open', '');
      await summary.focus();
      await page.keyboard.press('Enter');
      await expect(confirmation).toHaveAttribute('open', '');
      await expect(confirmation.getByRole('button', { name: 'Confirm deletion' })).toBeVisible();
      await page.keyboard.press('Space');
      await expect(confirmation).not.toHaveAttribute('open', '');
      await expectNoHorizontalOverflow(page);

      await page.goto('/library');
      await chooseStudyLanguage(page, 'de', true);
      await expect(page.getByLabel('Study language')).toHaveValue('de');
    } finally {
      await context.close();
    }
  });
});

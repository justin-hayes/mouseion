import { expect, Page, test } from '../support/test';
import { renderedTextContrast } from '../support/contrast';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
  await page.getByLabel('Study language').selectOption('de');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page).toHaveURL(/\/library$/);
}

function isCompact(projectName: string) {
  return projectName.startsWith('compact');
}

test('Reading shows the current Book with Vocabulary Browse in the primary area and no Other To Read section', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading');
  const goal = page.locator('#primary-goal-section');
  const browse = page.locator('#vocabulary-workflow');
  await expect(goal.getByRole('heading', { level: 1, name: 'Der lange Weg nach Hause' })).toBeVisible();
  await expect(browse.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
  await expect(page.locator('main #vocabulary-workflow')).toHaveCount(1);
  await expect(page.locator('details #vocabulary-workflow')).toHaveCount(0);
  expect(await goal.evaluate(node => Boolean(node.compareDocumentPosition(document.querySelector('#vocabulary-workflow')!) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
  await expect(page.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toBeVisible();
  await expect(page.getByRole('table', { name: 'Current effective vocabulary' })).toBeVisible();

  await expect(page.locator('#provisional-journey-heading')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Other To Read books', exact: true })).toHaveCount(0);
  await expect(page.locator('.journey-list')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Start reading' })).toHaveCount(0);
});

test('Reading chooser lists the other To Read books and returns to the current Book', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading');
  await page.getByRole('link', { name: 'Switch current reading', exact: true }).click();
  await expect(page).toHaveURL(/\/reading\/switch$/);
  await expect(page.getByRole('heading', { level: 1, name: 'Switch current reading' })).toBeVisible();
  const candidates = page.locator('li.reading-chooser-book');
  await expect(candidates.filter({ hasText: 'Route match: familiar German' })).toBeVisible();
  await expect(candidates.filter({ hasText: 'Der lange Weg nach Hause' })).toHaveCount(0);

  await page.getByRole('link', { name: 'Return to current reading', exact: true }).click();
  await expect(page).toHaveURL(/\/reading$/);
  await expect(page.locator('.journey-book--goal')).toBeVisible();
});

test('Reading keeps analysis details in a disclosure that is open on desktop and collapsed on compact', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading');
  const disclosure = page.locator('details.reading-supporting');
  const summary = disclosure.locator(':scope > summary');
  const reserved = page.getByRole('heading', { name: 'Reserved vocabulary', exact: true });

  // Deck status and its actions are primary and stay visible at every width.
  await expect(page.locator('.goal-card__deck')).toContainText('Deck status');
  await expect(page.getByRole('link', { name: 'Download deck' })).toBeVisible();

  if (isCompact(test.info().project.name)) {
    await expect(summary).toBeVisible();
    expect(await disclosure.evaluate(node => (node as HTMLDetailsElement).open)).toBe(false);
    await expect(reserved).toBeHidden();
    await summary.click();
    await expect(reserved).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Analysis', exact: true })).toBeVisible();
  } else {
    await expect(summary).toBeHidden();
    await expect(reserved).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Analysis', exact: true })).toBeVisible();
  }
});

test('Browse controls keep readable contrast and no page overflow in this colour scheme', async ({ page }) => {
  const dark = test.info().project.name.endsWith('dark');
  await signIn(page);
  await page.goto('/reading?q=paging');
  expect(await page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(dark);

  const prefix = page.locator('#vocabulary-prefix');
  const search = page.locator('#vocabulary-workflow').getByRole('button', { name: 'Search', exact: true });
  await expect(prefix).toBeVisible();
  expect(await prefix.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  expect(await search.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);

  if (isCompact(test.info().project.name)) {
    for (const control of [search, page.locator('#vocabulary-include-all').locator('xpath=ancestor::label')]) {
      const box = await control.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
  }
});

test('Reading keyboard order reaches the Browse search field after the current Book', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading');
  const goalLink = page.locator('.journey-book--goal a').first();
  await goalLink.focus();
  await expect(goalLink).toBeFocused();

  const prefix = page.locator('#vocabulary-prefix');
  let reached = false;
  for (let tab = 0; tab < 60 && !reached; tab++) {
    await page.keyboard.press('Tab');
    reached = await prefix.evaluate(node => node === document.activeElement);
  }
  expect(reached, 'search field reachable by Tab').toBe(true);
  await page.keyboard.press('Tab');
  await expect(page.locator('#vocabulary-include-all')).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.locator('#vocabulary-workflow').getByRole('button', { name: 'Search', exact: true })).toBeFocused();
});

test('Reading Browse works without JavaScript for prefix, paging, and page recovery', async ({ page, browser, baseURL }) => {
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await signIn(noScriptPage);
    await noScriptPage.goto('/reading?q=paging');
    const browsePages = noScriptPage.getByRole('navigation', { name: 'Browse pages' });
    await expect(noScriptPage.getByText('Page 1 of 2')).toBeVisible();
    await browsePages.getByRole('link', { name: 'Next' }).click();
    await expect(noScriptPage).toHaveURL(/page=2/);
    await expect(noScriptPage.getByText('Page 2 of 2')).toBeVisible();
    await expect(browsePages.getByRole('link', { name: 'Previous' })).toBeVisible();

    const search = noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' });
    await search.fill('paging');
    await noScriptPage.locator('#vocabulary-workflow').getByRole('button', { name: 'Search', exact: true }).click();
    await expect(noScriptPage.locator('.vocabulary-applied-query')).toContainText('Applied prefix: paging');
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveValue('paging');

    const malformed = await noScriptPage.goto('/reading?q=paging&page=abc');
    expect(malformed?.status()).toBe(400);
    const recovery = noScriptPage.locator('#vocabulary-recovery');
    await expect(recovery).toContainText('That page is not available');
    await expect(noScriptPage.locator('#vocabulary-browse-results')).toHaveCount(0);
    await recovery.getByText('First page', { exact: true }).click();
    await expect(noScriptPage).toHaveURL(/page=1/);
    await expect(noScriptPage.getByText('Page 1 of 2')).toBeVisible();

    const outOfRange = await noScriptPage.goto('/reading?q=paging&page=999');
    expect(outOfRange?.status()).toBe(404);
    await expect(noScriptPage.locator('#vocabulary-recovery')).toContainText('That page is not available');
    await expect(noScriptPage.locator('#vocabulary-recovery').getByText('First page', { exact: true })).toBeVisible();
    await expect(noScriptPage.locator('#vocabulary-browse-results')).toHaveCount(0);
  } finally {
    await noScriptContext.close();
  }
});

test('/vocabulary is the Concordance and /known-vocab is not found without redirect', async ({ page }) => {
  await signIn(page);
  const vocabulary = await page.goto('/vocabulary');
  expect(vocabulary?.status()).toBe(200);
  expect(new URL(page.url()).pathname).toBe('/vocabulary');
  const header = page.locator('.vocabulary-page-header');
  await expect(header.getByRole('heading', { level: 1, name: 'Vocabulary', exact: true })).toBeVisible();
  const views = header.getByRole('navigation', { name: 'Vocabulary views' });
  await expect(views.getByRole('link')).toHaveCount(2);
  await expect(views.getByRole('link', { name: 'Concordance', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(views.getByRole('link', { name: 'Import Known words', exact: true })).toHaveAttribute('href', '/vocabulary/import');
  await expect(page.getByRole('link', { name: 'Browse', exact: true })).toHaveCount(0);

  const retired = await page.goto('/known-vocab');
  expect(retired?.status()).toBe(404);
  expect(retired?.request().redirectedFrom()).toBeNull();
  expect(new URL(page.url()).pathname).toBe('/known-vocab');
  await expect(page.locator('.vocabulary-page-header')).toHaveCount(0);
});

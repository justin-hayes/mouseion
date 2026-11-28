import { expect, Page, test } from '../support/test';
import { renderedTextContrast } from '../support/contrast';

const detail = '/books/fixture-book';

type Scenario = '' | 'no-analysis' | 'no-vocabulary' | 'counts-updating' | 'counts-unavailable' | 'accounted';

async function setScenario(page: Page, baseURL: string | undefined, scenario: Scenario) {
  const response = await page.request.post(new URL('/fixture/vocabulary-browse-scenario', baseURL).href, { form: { scenario } });
  expect(response.status(), `set Browse scenario ${scenario || 'default'}`).toBe(204);
}

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

async function expectNoPageOverflow(page: Page, label: string) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), label).toBe(true);
}

test.afterEach(async ({ page, baseURL }) => {
  await setScenario(page, baseURL, '');
});

test('Book detail hosts the analysis actions and lists the Book words by Study state with Concordance links', async ({ page }) => {
  const dark = test.info().project.name.endsWith('dark');
  await signIn(page);
  await page.goto('/de/library');
  await page.locator('.library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByRole('link', { name: 'Der lange Weg nach Hause' }).click();
  await expect(page).toHaveURL(/\/books\/fixture-book$/);
  expect(await page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(dark);

  const summary = page.locator('#book-detail-summary');
  await expect(summary.getByRole('heading', { level: 2, name: 'Der lange Weg nach Hause' })).toBeVisible();
  await expect(summary.locator('[data-book-language]')).toContainText('de');
  await expect(page.getByRole('link', { name: 'Back to My Books' })).toBeVisible();
  await expect(page.locator('#book-detail-analysis').getByRole('heading', { name: 'Analysis', exact: true })).toBeVisible();

  const words = page.locator('#vocabulary-workflow');
  await expect(words.getByRole('heading', { name: 'Words', exact: true })).toBeVisible();
  const table = words.getByRole('table', { name: 'Words by Study state' });
  await expect(table).toBeVisible();
  await expect(table.getByRole('columnheader', { name: 'Study state' })).toBeVisible();
  await expect(table.locator('[data-study-state]')).toHaveText(['Not yet introduced', 'Mature', 'In study']);
  await expect(words).not.toContainText(/\b(Known|Reserved|Unknown)\b/);
  // The list edits nothing: its only form is the GET prefix search.
  await expect(words.locator('form')).toHaveCount(1);
  await expect(words.locator('form[method="post"]')).toHaveCount(0);
  await expect(table.getByRole('button')).toHaveCount(0);

  await table.getByRole('link', { name: 'gehen' }).click();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\?/);
  await expect(page.getByRole('link', { name: 'Back to book vocabulary' })).toHaveAttribute('href', /\/books\/fixture-book\?.*row=/);
});

test('Retired working-desk addresses move to the Book detail page or My Books', async ({ page }) => {
  await signIn(page);
  await page.goto('/de/reading?reading=fixture-book&q=gehen&all=1&snapshot=stale-snapshot&page=1');
  await expect(page).toHaveURL(/\/books\/fixture-book\?q=gehen$/);
  await expect(page.locator('#vocabulary-workflow')).toBeVisible();
  for (const address of ['/de/reading?page=1', '/de/reading?q=ha&all=1&row=NOUN%3Ahaus', '/de/reading']) {
    await page.goto(address);
    await expect(page, address).toHaveURL(/\/de\/library$/);
    await expect(page.locator('#vocabulary-workflow'), address).toHaveCount(0);
  }
  const missing = await page.goto('/books/no-such-book');
  expect(missing?.status()).toBe(404);
});

test('Back to book vocabulary returns to the same Book detail page and row', async ({ page }) => {
  await signIn(page);
  await page.goto(`${detail}?q=ha`);
  await page.locator('a.vocabulary-lemma-link').last().click();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\?/);
  const back = page.getByRole('link', { name: 'Back to book vocabulary' });
  await expect(back).toBeVisible();

  // An independent search keeps the way back.
  await page.locator('#concordance-term').fill('Haus');
  await page.locator('#concordance-term').press('Enter');
  await expect(page.getByRole('link', { name: 'Back to book vocabulary' })).toBeVisible();

  await page.getByRole('link', { name: 'Back to book vocabulary' }).click();
  await expect(page).toHaveURL(/\/books\/fixture-book\?/);
  await expect(page.locator('#vocabulary-workflow')).toBeVisible();
  await expect(page.locator('tr[id^="browse-row-"]').last()).toBeFocused();

  // Native history still works.
  await page.goBack();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\?/);
});

test('Book detail keeps its analysis details and word list within the viewport in every state', async ({ page, baseURL }) => {
  test.setTimeout(90_000);
  await signIn(page);
  for (const scenario of ['', 'no-analysis', 'counts-updating', 'counts-unavailable'] as Scenario[]) {
    await setScenario(page, baseURL, scenario);
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 700 });
      await page.goto(detail);
      const label = `${scenario || 'default'} at ${width}px`;
      await expect(page.locator('#vocabulary-workflow'), label).toBeVisible();
      await expectNoPageOverflow(page, label);
      const title = page.locator('.journey-book__title');
      await expect(title, label).toHaveText('Der lange Weg nach Hause');
      expect(await title.evaluate(element => element.scrollWidth <= element.clientWidth), label).toBe(true);
    }
  }
});

test('Book detail shows counts updating with Refresh status and withholds the words', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'counts-updating');
  await signIn(page);
  await page.goto(detail);
  const words = page.locator('#vocabulary-workflow');
  const status = words.locator('#vocabulary-counts-status');
  await expect(status).toHaveAttribute('role', 'status');
  await expect(status).toContainText('Updating vocabulary counts.');
  await expect(words.getByRole('table', { name: 'Words by Study state' })).toHaveCount(0);
  await expect(words).not.toContainText('No eligible vocabulary in this analysis');
  expect(await status.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  await words.getByRole('link', { name: 'Refresh status', exact: true }).click();
  await expect(page.locator('#book-detail-analysis')).toBeVisible();

  await setScenario(page, baseURL, 'counts-unavailable');
  await page.goto(detail);
  await expect(words.locator('#vocabulary-counts-status')).toHaveAttribute('role', 'alert');
  await expect(words.locator('#vocabulary-counts-status')).toContainText('Vocabulary counts unavailable.');

  await setScenario(page, baseURL, 'no-analysis');
  await page.goto(detail);
  await expect(words).toContainText('no current completed analysis');
  await expect(words.getByRole('table')).toHaveCount(0);
});

test('Word list controls keep readable contrast and no page overflow in this colour scheme', async ({ page }) => {
  await signIn(page);
  await page.goto(`${detail}?q=paging`);
  const prefix = page.locator('#vocabulary-prefix');
  const search = page.locator('#vocabulary-workflow').getByRole('button', { name: 'Search', exact: true });
  await expect(prefix).toBeVisible();
  expect(await prefix.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  expect(await search.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  await expectNoPageOverflow(page, 'word list');
  if (isCompact(test.info().project.name)) {
    expect((await search.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  }
});

test('Word list works without JavaScript for prefix, paging and page recovery', async ({ browser, baseURL }) => {
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await signIn(noScriptPage);
    await noScriptPage.goto(`${detail}?q=paging`);
    const pages = noScriptPage.getByRole('navigation', { name: 'Word pages' });
    await expect(noScriptPage.getByText('Page 1 of 2')).toBeVisible();
    await pages.getByRole('link', { name: 'Next' }).click();
    await expect(noScriptPage).toHaveURL(/page=2/);
    await expect(noScriptPage.getByText('Page 2 of 2')).toBeVisible();
    await expect(pages.getByRole('link', { name: 'Previous' })).toBeVisible();

    const search = noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' });
    await expect(search).toHaveClass(/\binput\b/);
    await search.fill('haus');
    await search.press('Enter');
    await expect(noScriptPage).toHaveURL(/\/books\/fixture-book\?q=haus/);
    await expect(noScriptPage.locator('.vocabulary-applied-query')).toContainText('Applied prefix: haus');
    await expect(noScriptPage.locator('#vocabulary-results-heading')).toBeFocused();
    await expect(noScriptPage.getByRole('row').filter({ hasText: 'haus' })).toContainText('Mature');
    await expectNoPageOverflow(noScriptPage, 'no-script word list');

    await noScriptPage.goto(`${detail}?q=zznotfound`);
    await expect(noScriptPage.getByText('No words in this Book match this canonical-lemma prefix.')).toBeVisible();
    await noScriptPage.getByRole('link', { name: 'Clear prefix' }).click();
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveValue('');

    const malformed = await noScriptPage.goto(`${detail}?q=paging&page=abc`);
    expect(malformed?.status()).toBe(400);
    const recovery = noScriptPage.locator('#vocabulary-recovery');
    await expect(recovery).toContainText('That page is not available');
    await expect(noScriptPage.locator('#vocabulary-browse-results')).toHaveCount(0);
    await recovery.getByText('First page', { exact: true }).click();
    await expect(noScriptPage.getByText('Page 1 of 2')).toBeVisible();

    const outOfRange = await noScriptPage.goto(`${detail}?q=paging&page=999`);
    expect(outOfRange?.status()).toBe(404);
    await expect(noScriptPage.locator('#vocabulary-recovery')).toContainText('That page is not available');
  } finally {
    await noScriptContext.close();
  }
});

test('Word list keeps the page and controls while exploring a word on page two', async ({ page }) => {
  await signIn(page);
  await page.goto(`${detail}?q=paging`);
  await page.locator('#vocabulary-workflow form').evaluate(element => element.setAttribute('data-stable-search', 'yes'));
  await page.getByRole('navigation', { name: 'Word pages' }).getByRole('link', { name: 'Next' }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.locator('#vocabulary-workflow form')).toHaveAttribute('data-stable-search', 'yes');
  const row = page.getByRole('row').filter({ hasText: 'paging25' });
  await expect(row.getByRole('link', { name: 'paging25' })).toBeVisible();
  await expect(row.getByRole('button')).toHaveCount(0);
});

test('Word list pagination lands on the new results rather than above them', async ({ page }) => {
  await signIn(page);
  await page.goto(`${detail}?q=paging`);
  for (const [direction, number] of [['Next', 2], ['Previous', 1]] as const) {
    await page.getByRole('navigation', { name: 'Word pages' }).getByRole('link', { name: direction }).click();
    await expect(page.getByText(`Page ${number} of 2`)).toBeVisible();
    const heading = page.locator('#vocabulary-results-heading');
    await expect(heading).toBeFocused();
    const results = await heading.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return { top: bounds.top, bottom: bounds.bottom, viewportHeight: window.innerHeight };
    });
    expect(results.top).toBeGreaterThanOrEqual(-1); // Subpixel rounding at narrow widths.
    expect(results.bottom).toBeLessThan(results.viewportHeight);
    expect(results.top).toBeLessThan(160);
  }
});

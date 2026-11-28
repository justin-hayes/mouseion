import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
  const language = page.getByLabel('Study language');
  if (await language.inputValue() !== 'de') {
    await language.selectOption('de');
    await expect(page).toHaveURL(/\/de\/library$/);
  }
}

async function expectNoPageOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
}

// The fixture store is file-scoped, so the journey stays ordered: it reads the
// pinned Current reading first, then replaces it with Read next, then ends it.
test.describe.configure({ mode: 'serial' });

test.describe('My Books pins the Current reading', () => {
  test('the Current reading is pinned above To Read with coverage and words left to Target coverage', async ({ page }) => {
    await signIn(page);
    const pinned = page.locator('#primary-goal-section');
    await expect(pinned).toBeVisible();
    await expect(pinned).toHaveAttribute('aria-label', 'Current reading');
    await expect(pinned.getByRole('heading', { level: 2, name: 'Der lange Weg nach Hause' })).toBeVisible();

    const coverage = pinned.locator('.journey-book__coverage');
    await expect(coverage).toContainText(/Mature coverage:\s*37\.0%/);
    await expect(coverage).toContainText('45,678 of 123,456 word occurrences');
    await expect(coverage).toContainText(/With words in study:\s*42\.1%/);
    await expect(coverage).toContainText('About 540 more words to reach Target coverage (98%).');

    // Pinned above the list, with Finish and End acting on this Book.
    expect(await pinned.evaluate((node) => Boolean(node.compareDocumentPosition(document.querySelector('#library-results')!) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
    await expect(pinned.locator('form[action="/reading/finish"] input[name="language"]')).toHaveValue('de');
    await expect(pinned.locator('form[action="/reading/end"] input[name="language"]')).toHaveValue('de');
    await expect(pinned.getByRole('link', { name: 'Switch current reading' })).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
    await expectNoPageOverflow(page);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
  });

  test('Reading is not in the shell navigation and its addresses return to My Books in the same language', async ({ page }) => {
    await signIn(page);
    const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
    await expect(navigation.getByRole('link', { name: 'Reading' })).toHaveCount(0);
    await expect(navigation.locator('.site-header__navigation a')).toHaveText(['My Books', 'Study', 'Vocabulary', 'Catalog']);

    for (const [path, expected] of [
      ['/de/reading', /\/de\/library$/],
      ['/de/reading/switch', /\/de\/library$/],
      ['/it/reading', /\/it\/library$/],
      ['/it/reading/switch', /\/it\/library$/],
      ['/de/reading?message=Saved', /\/de\/library\?message=Saved$/],
    ] as const) {
      await page.goto(path);
      await expect(page, path).toHaveURL(expected);
    }
    await page.goto('/de/library');
  });

  test('Read next on a To Read row replaces the Current reading and offers no confirmation', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library?disposition=to_read');
    const candidate = page.locator('.library-books .library-book').filter({ hasText: 'Route match: familiar German' });
    const readNext = candidate.getByRole('button', { name: 'Read next' });
    await expect(readNext).toBeVisible();
    expect((await readNext.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expect(candidate.locator('details.confirmation form[action$="/start"]')).toHaveCount(0);

    // The pinned Book has no Read next of its own.
    const current = page.locator('.library-books .library-book').filter({ hasText: 'Der lange Weg nach Hause' });
    await expect(current.getByRole('button', { name: 'Read next' })).toHaveCount(0);

    await readNext.click();
    await expect(page).toHaveURL(/\/de\/library\?message=/);
    await expect(page.getByRole('status').first()).toContainText('is now your current reading');
    await expect(page.locator('#primary-goal-section').getByRole('heading', { level: 2, name: 'Route match: familiar German' })).toBeVisible();
    await expect(page.locator('#primary-goal-section')).not.toContainText('Der lange Weg nach Hause');
    await expectNoPageOverflow(page);
  });

  test('Finish and End act on the pinned Book with native confirmations that work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    const page = await context.newPage();
    await signIn(page);
    const pinned = page.locator('#primary-goal-section');
    await expect(pinned).toContainText('Route match: familiar German');

    const finish = pinned.locator('details.confirmation').filter({ hasText: 'Mark reading finished' });
    await finish.locator('summary').click();
    await expect(finish.getByRole('button', { name: 'Mark reading finished' })).toBeVisible();
    await expect(pinned).toContainText('This changes no vocabulary');
    await expect(pinned.locator('input[name="expected_current_book_id"]')).toHaveCount(0);
    await expect(pinned.locator('input[name="expected_current_snapshot_id"]')).toHaveCount(0);

    const end = pinned.locator('details.confirmation').filter({ hasText: 'End current reading' });
    await end.locator('summary').click();
    await end.getByRole('button', { name: 'Confirm end current reading' }).click();
    await expect(page).toHaveURL(/\/de\/library\?message=/);
    await expect(page.getByRole('status').first()).toContainText('Reading ended');
    await expect(page.locator('#primary-goal-section')).toHaveCount(0);
    await context.close();
  });

  test('Finish records the reading and returns to My Books', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library?disposition=to_read');
    await page.locator('.library-books .library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByRole('button', { name: 'Read next' }).click();
    await expect(page.locator('#primary-goal-section')).toContainText('Der lange Weg nach Hause');

    const finish = page.locator('#primary-goal-section details.confirmation').filter({ hasText: 'Mark reading finished' });
    await finish.locator('summary').click();
    await finish.getByRole('button', { name: 'Mark reading finished' }).click();
    await expect(page.getByRole('heading', { name: /Reading finished/i })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Choose a To Read book in My Books' })).toHaveAttribute('href', '/de/library');
    await page.getByRole('link', { name: 'Choose a To Read book in My Books' }).click();
    await expect(page).toHaveURL(/\/de\/library$/);
    await expect(page.locator('#primary-goal-section')).toHaveCount(0);
  });
});

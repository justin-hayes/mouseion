import { expect, Page, test } from '../support/test';
import { readNext } from '../support/reading';

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

async function switchCurrentReadingTo(page: Page, title: string) {
  await readNext(page, title);
}

async function openGermanReading(page: Page) {
  await page.goto('/de/library');
  const language = page.getByLabel('Study language');
  if (await language.inputValue() !== 'de') await language.selectOption('de');
  await page.goto('/de/library');
}

async function switchToRouteMatch(page: Page) {
  await openGermanReading(page);
  const current = page.locator('.journey-book--goal');
  const currentID = await current.count() ? await current.getAttribute('id') : null;
  if (!currentID) {
    await readNext(page, 'Route match: familiar German');
    return;
  }
  if (currentID === 'journey-book-fixture-route-match') {
    await switchCurrentReadingTo(page, 'Der lange Weg nach Hause');
  }
  await switchCurrentReadingTo(page, 'Route match: familiar German');
}

async function restoreFixtureBookCurrent(page: Page) {
  await openGermanReading(page);
  const current = page.locator('.journey-book--goal');
  const currentID = await current.count() ? await current.getAttribute('id') : null;
  if (currentID === 'journey-book-fixture-book') return;
  if (currentID) {
    await switchCurrentReadingTo(page, 'Der lange Weg nach Hause');
    return;
  }
  await readNext(page, 'Der lange Weg nach Hause');
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
    const destinations = ['My Books', 'Corpus'];
    for (const name of destinations) {
      const link = navigation.getByRole('link', { name });
      await link.focus();
      await expect(link).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(page).toHaveURL(new RegExp({
        'My Books': '\\/library', Corpus: '\\/corpus',
      }[name]));
      await page.goBack();
      await expect(navigation).toBeVisible();
    }
  });

  test('representative tab order and focus indicators remain visible', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/corpus');
    await page.keyboard.press('Tab');
    await expect(page.locator('a.skip-link')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.getByRole('link', { name: 'Mouseion' })).toBeFocused();
    for (const name of ['My Books', 'Corpus', 'Catalog']) {
      await page.keyboard.press('Tab');
      await expect(page.getByRole('link', { name, exact: true })).toBeFocused();
    }
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Study language')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('.site-header__account summary')).toBeFocused();
    const focusStyle = await page.evaluate(() => {
      const element = document.activeElement;
      if (!element) return { outline: 'none', width: '0px', boxShadow: 'none' };
      const style = getComputedStyle(element);
      return { outline: style.outlineStyle, width: style.outlineWidth, boxShadow: style.boxShadow };
    });
    expect(focusStyle.outline !== 'none' || focusStyle.width !== '0px' || focusStyle.boxShadow !== 'none').toBeTruthy();

    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/de/library');
    await page.keyboard.press('Tab');
    await expect(page.locator('.skip-link')).toBeFocused();
    await page.keyboard.press('Tab');
    for (const name of ['Mouseion', 'My Books', 'Corpus', 'Catalog']) {
      await expect(page.getByRole('link', { name, exact: true })).toBeFocused();
      if (name !== 'Catalog') await page.keyboard.press('Tab');
    }
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Study language')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('.site-header__account summary')).toBeFocused();
  });

  test('My Books exposes keyboard-reachable identity links and failed-analysis recovery', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    const item = page.locator('.library-books .library-book').filter({ hasText: 'Der lange Weg nach Hause' });
    const itemFocusStops = item.locator('a, button:not([disabled]), summary');
    await expect(itemFocusStops.first()).toHaveText('Der lange Weg nach Hause');
    await expect(itemFocusStops.first()).toHaveAttribute('href', '/books/fixture-book');
    await itemFocusStops.first().focus();
    await expect(itemFocusStops.first()).toBeFocused();
    await expect(page.locator('.library-books').getByText('Review failed analysis')).toHaveCount(0);
  });

  test('My Books keeps the pinned Current reading first and announces feedback', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library?message=Journey%20updated');
    const goalHeading = page.locator('.journey-book__title');
    await expect(goalHeading).toBeVisible();
    await expect(page.getByRole('status').first()).toContainText('Journey updated');
    // The pinned Current reading precedes the To Read list.
    expect(await goalHeading.evaluate((node) => node.compareDocumentPosition(document.querySelector('#library-results')!) & Node.DOCUMENT_POSITION_FOLLOWING)).toBeTruthy();
    const goalLink = page.locator('.journey-book--goal a').first();
    await goalLink.focus();
    await expect(goalLink).toBeFocused();
  });

  test('unassessed books offer no standalone analysis action on their detail page', async ({ page }) => {
    await signIn(page);
    const response = await page.goto('/books/fixture-empty');
    expect(response?.status()).toBe(200);
    await expect(page.getByRole('button', { name: 'Start analysis' })).toHaveCount(0);
    expect((await page.goto('/books/no-such-book'))?.status()).toBe(404);
  });

  test('the focused Book deck task is gone whether or not the Book is Current reading', async ({ page }) => {
    await signIn(page);
    await restoreFixtureBookCurrent(page);
    const beforeReading = await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    expect(beforeReading?.status()).toBe(404);

    await switchToRouteMatch(page);
    const whileReading = await page.goto('/reading/books/fixture-route-match/deck/preparations/new');
    expect(whileReading?.status()).toBe(404);
    await restoreFixtureBookCurrent(page);
  });

  test('My Books and table region are keyboard-scrollable', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books');
    await page.goto('/jobs');
    const region = page.getByRole('region', { name: 'Book preparations' });
    await expect(region).toHaveAttribute('tabindex', '0');
    await expect(region).toHaveAttribute('aria-label', 'Book preparations');
    await region.focus();
    await expect(region).toBeFocused();
  });

  test('failed analysis exposes keyboard-operable recovery and intentional error focus pattern', async ({ page }) => {
    await signIn(page);
    await page.goto('/jobs/43');
    const error = page.getByRole('alert').filter({ hasText: 'Analysis needs attention' });
    await expect(error).toBeVisible();
    const retry = page.getByRole('button', { name: 'Retry analysis' });
    await expect(retry).toHaveClass(/\bbutton\b/);
    await expect(retry.locator('xpath=ancestor::form')).toHaveAttribute('method', 'post');
    await expect(retry.locator('xpath=ancestor::form')).toHaveAttribute('action', '/jobs/43/retry');
    await expect(retry).toBeVisible();
    await retry.focus();
    await expect(retry).toBeFocused();
    await retry.press('Enter');
    await expect(page).toHaveURL(/\/jobs\/43\?message=/);
  });
});

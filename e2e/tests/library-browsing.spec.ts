import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('My Books collection browsing', () => {
  test('browses and searches only the active study language', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');

    await expect(page.getByLabel('Search My Books')).toBeVisible();
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await expect(page.getByRole('navigation', { name: 'Languages' })).toHaveCount(0);
    await expect(page.getByText('All languages')).toHaveCount(0);
    await expect(page.locator('.library-list').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('.library-list').getByText('Empty chapter')).toHaveCount(0);
    await expect(page.locator('.library-list')).not.toContainText('language:');

    await page.getByLabel('Search My Books').fill('Der lange');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.locator('#library-results .library-list').getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.locator('#library-results a[href^="/journey/"]').first()).toBeVisible();

    await page.getByLabel('Search My Books').fill('no-local-book-matches-this-term');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.getByText(/No books in your local collection.*match/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Clear search' })).toBeVisible();

    await page.goto('/library');
    await switcher.selectOption('it');
    await expect(page).toHaveURL('/library');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in Italian');
    await expect(page.locator('.library-list').getByText('Empty chapter')).toBeVisible();
    await expect(page.locator('.library-list').getByText('Der lange Weg nach Hause')).toHaveCount(0);
    await switcher.selectOption('de');
  });

  test('reviews needs-language books without actions', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture sync runs once per browser suite.');
    await signIn(page);
    await page.goto('/library');
    const strip = page.locator('section.library-needs-language');
    await expect(strip).toContainText(/book[s]? .*outside the active language collections/);
    await strip.getByRole('link', { name: 'Review books awaiting a language' }).click();
    await expect(page).toHaveURL(/\/library\?needs-language/);
    const needsRow = page.locator('.library-list article.library-book').filter({ hasText: 'Browser sync metadata book' });
    await expect(needsRow).toBeVisible();
    await expect(needsRow.locator('a')).toHaveCount(0);
    await expect(needsRow.locator('form')).toHaveCount(0);

    await page.goto('/catalogs');
    await page.locator('#connection-fixture-browser-sync-connection').getByRole('button', { name: 'Sync now' }).click();
    await expect(page).toHaveURL(/\/catalogs\?/);
    await page.goto('/library?needs-language');
    await expect(page.locator('.library-list').getByText('Browser sync metadata book')).toHaveCount(0);
    await page.goto('/library');
    await expect(page.locator('.library-list').getByText('Browser sync metadata book')).toBeVisible();
  });
});

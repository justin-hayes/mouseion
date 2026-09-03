import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('My Books collection browsing', () => {
  test('searches, filters by language, and opens acquired and metadata-only books', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');

    await expect(page.getByLabel('Search My Books')).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Languages' })).toBeVisible();
    await expect(page.getByRole('link', { name: /^de /i })).toBeVisible();
    await expect(page.getByRole('link', { name: /Unknown language/i })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Metadata-only migration book' })).toHaveAttribute('href', /\/books\/fixture-metadata-only$/);

    await page.getByLabel('Search My Books').fill('Donaudampf');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.getByText(/Donaudampfschifffahrtsgesellschaftskapitänsmütze/)).toBeVisible();
    await expect(page.locator('#library-results a[href^="/books/"]').first()).toBeVisible();

    await page.getByLabel('Search My Books').fill('no-local-book-matches-this-term');
    await page.getByRole('button', { name: 'Search' }).click();
    await expect(page.getByText(/No books in your local collection match/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Clear search' })).toBeVisible();

    await page.goto('/library');
    await page.getByRole('link', { name: /^de /i }).click();
    // The language view panel also names the book in its per-book spread, so
    // scope the assertion to the bibliographic collection list.
    await expect(page.locator('.library-list').getByText('Der lange Weg nach Hause')).toBeVisible();
  });
});

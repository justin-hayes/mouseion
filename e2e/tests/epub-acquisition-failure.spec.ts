import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('EPUB acquisition failure evidence', () => {
  test('My Books shows the recorded reason and Retry on a failed acquisition', async ({ page }) => {
    await signIn(page);

    // The fixture seeds the row Hidden so it does not change the visible
    // collection. Unhide it, then confirm the reason and Retry on the ordinary
    // My Books collection rather than only under Show hidden books.
    await page.goto('/library?disposition=to_read&show-hidden');
    const hiddenRow = page.locator('#book-row-fixture-acquisition-failed');
    await expect(hiddenRow).toHaveCount(1);
    await hiddenRow.getByRole('button', { name: 'Unhide' }).click();

    await page.goto('/library?disposition=to_read');
    const row = page.locator('#book-row-fixture-acquisition-failed');
    await expect(row).toHaveCount(1);
    await expect(row.locator('.library-book__membership')).toContainText('To Read');
    await expect(row.locator('.library-book__evidence')).toHaveText('The EPUB could not be downloaded.');

    const retry = row.getByRole('button', { name: 'Retry', exact: true });
    await expect(retry).toBeVisible();
    await expect(retry.locator('xpath=..')).toHaveAttribute('action', '/reading/books/fixture-acquisition-failed/reanalyze');

    // Retry is reading intent's Retry route, not a separate acquisition action.
    await expect(row.locator('form[action="/reading/books/fixture-acquisition-failed/reanalyze"] input[name="csrf_token"]')).toHaveCount(1);
  });
});

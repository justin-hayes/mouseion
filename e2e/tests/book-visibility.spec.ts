import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('My Books Hide and Unhide', () => {
  test('hides with an ordinary form, recovers through Show hidden books, and unhides', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const row = page.locator('#book-row-fixture-not-analyzed');
    await row.getByText('More actions').click();
    await row.getByRole('button', { name: 'Hide', exact: true }).click();

    // Default scope omits the Book and offers recovery.
    await expect(page.locator('#book-row-fixture-not-analyzed')).toHaveCount(0);
    const toggle = page.locator('#library-show-hidden');
    await expect(toggle).toContainText('Show hidden books');
    await toggle.click();
    await expect(page).toHaveURL(/show-hidden/);

    const hiddenRow = page.locator('#book-row-fixture-not-analyzed');
    await expect(hiddenRow.locator('.library-book__membership')).toContainText('Hidden');
    await hiddenRow.getByRole('button', { name: 'Unhide' }).click();

    await page.goto('/library');
    await expect(page.locator('#book-row-fixture-not-analyzed')).toHaveCount(1);
    await expect(page.locator('#library-show-hidden')).toHaveText('Show hidden books');
  });
});

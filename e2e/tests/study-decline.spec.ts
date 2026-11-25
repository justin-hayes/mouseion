import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Study new-word batch', () => {
  test('declining the batch takes one tap and introduces no word', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    await expect(page.locator('#study-offer')).toBeVisible();

    await page.getByRole('button', { name: 'Not today' }).click();

    await expect(page.locator('#study-idle')).toHaveText('Nothing to study right now.');
    await expect(page.getByRole('button', { name: 'Take these words' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Show answer' })).toHaveCount(0);
    // The words stay ready for another Study day.
    await expect(page.locator('#study-ready')).toHaveText('3 new words are ready.');
  });
});

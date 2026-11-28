import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
}

test.describe('Study new-word batch', () => {
  test('declining the batch takes one tap, introduces no word and can be undone from Fluency', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    await expect(page.locator('#study-offer')).toBeVisible();

    await page.getByRole('button', { name: 'Not today' }).click();

    // Declining opens Fluency, which has nothing mature to show yet.
    await expect(page.locator('#study-nothing-mature')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Not today' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Show answer' })).toHaveCount(0);
    await expect(page.locator('#study-ready')).toHaveText('3 new words are ready.');

    // The learner can still change their mind from inside Fluency.
    await page.getByRole('button', { name: 'Take these words' }).click();
    await expect(page.getByRole('button', { name: 'Show answer' })).toBeVisible();
  });
});

import { readNext } from '../support/reading';
import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
}

// A learner who starts reading a To Read Book with a published analysis, and
// whose newer run then fails, keeps that analysis as the Current reading. The
// Current reading cannot be refreshed, so Reading presents the re-analysis
// failure without a Retry analysis action.
test.describe('Current reading over a failed re-analysis', () => {
  test('Reading shows the re-analysis failure without Retry analysis', async ({ page }) => {
    await signIn(page);

    await readNext(page, 'Neue Analyse fehlgeschlagen');

    await page.goto('/de/reading');
    await expect(page.locator('#primary-goal-section').getByRole('heading', { name: 'Neue Analyse fehlgeschlagen' })).toBeVisible();
    // The hero names the failure and where recovery works; it offers no Retry,
    // which the server refuses for the Current reading.
    const status = page.locator('#primary-goal-section .current-hero__status');
    await expect(status).toContainText('Re-analysis failed.');
    await expect(status).toContainText('The current analysis stays in effect.');
    await expect(status).toContainText('End this reading to retry it from To Read.');
    await expect(page.locator('#primary-goal-section details.reading-supporting')).toHaveCount(0);
    await expect(page.locator('#primary-goal-section').getByRole('button', { name: /^Retry / })).toHaveCount(0);
  });
});

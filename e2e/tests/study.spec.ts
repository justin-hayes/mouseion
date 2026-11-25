import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Study', () => {
  test('opens for a German learner, reports its new words and fits the screen', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');

    await expect(page.getByRole('heading', { level: 1, name: 'Study' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 2, name: 'New words' })).toBeVisible();
    // Opening the page asks for the next words to be prepared. The fixture
    // server prepares nothing, so they are reported as being prepared.
    await expect(page.locator('#study-ready')).toHaveText(/\d+ new words? (is|are) ready\./);
    await expect(page.locator('#study-preparing')).toHaveText(/\d+ more words? (is|are) being prepared\./);

    // No page-level horizontal scroll, in the compact viewport or any other.
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('is reached from the language-prefixed address only', async ({ page }) => {
    await signIn(page);
    await page.goto('/study');
    await expect(page).toHaveURL(/\/[a-z]{2,3}\/study$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Study' })).toBeVisible();
  });
});

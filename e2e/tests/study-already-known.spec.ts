import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Study Already known', () => {
  test('answers an introduction Already known, which is offered on introductions only', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    await page.getByRole('button', { name: 'Take these words' }).click();

    const noOverflow = async () => {
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    };

    // The answer is offered with the others once the answer side is shown.
    await expect(page.getByRole('button', { name: 'Already known' })).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();
    const alreadyKnown = page.getByRole('button', { name: 'Already known' });
    await expect(alreadyKnown).toBeVisible();
    await expect(page.getByRole('button', { name: 'Got it' })).toBeVisible();
    await noOverflow();
    await alreadyKnown.click();

    // The word is answered, so the next introduction is the next card.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await expect(page.locator('#study-gloss')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();

    // Again is a grade: it keeps the word in learning. Its repeat is a later
    // review, which is not offered Already known (covered by the Go tests, as
    // the repeat is not due for a minute).
    await page.getByRole('button', { name: 'Again' }).click();

    // The third word is another introduction, so it offers the answer too.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Show answer' }).click();
    await expect(page.getByRole('button', { name: 'Already known' })).toBeVisible();
    await page.getByRole('button', { name: 'Got it' }).click();

    // Only the repeated word is left, and it is not due for a minute.
    await expect(page.locator('#study-repeating')).toHaveText('1 word comes back later today.');
    await expect(page.getByRole('button', { name: 'Already known' })).toHaveCount(0);
    await noOverflow();
  });
});

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
    // server prepares them at once.
    await expect(page.locator('#study-ready')).toHaveText('3 new words are ready.');
    await expect(page.locator('#study-offer')).toHaveText('3 new words are ready to learn.');

    // No page-level horizontal scroll, in the compact viewport or any other.
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('takes the batch and grades each card with Show answer, Again and Got it', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    await page.getByRole('button', { name: 'Take these words' }).click();

    const noOverflow = async () => {
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    };

    // First word: the sentence shows the target word marked, the answer is
    // hidden until Show answer, and Got it moves on.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await expect(page.locator('.study-card__target')).toBeVisible();
    await expect(page.locator('#study-gloss')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Got it' })).toHaveCount(0);
    await noOverflow();
    await page.getByRole('button', { name: 'Show answer' }).click();
    await expect(page.locator('#study-gloss')).toBeVisible();
    await expect(page.locator('#study-translation')).toBeVisible();
    await expect(page.locator('#study-headword')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Again' })).toBeVisible();
    await noOverflow();
    await page.getByRole('button', { name: 'Got it' }).click();

    // Second word: Again keeps it in learning to come back later today.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await expect(page.locator('#study-gloss')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();
    await page.getByRole('button', { name: 'Again' }).click();

    // Third word: Got it.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Show answer' }).click();
    await page.getByRole('button', { name: 'Got it' }).click();

    // The repeated word is not due for a minute, so Today waits for it.
    await expect(page.locator('#study-repeating')).toHaveText('1 word comes back later today.');
    await expect(page.getByRole('button', { name: 'Show answer' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Take these words' })).toHaveCount(0);
    await noOverflow();
  });

  test('is reached from the language-prefixed address only', async ({ page }) => {
    await signIn(page);
    await page.goto('/study');
    await expect(page).toHaveURL(/\/[a-z]{2,3}\/study$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Study' })).toBeVisible();
  });
});

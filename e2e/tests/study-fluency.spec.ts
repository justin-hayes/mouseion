import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server for this spec has one mature word with two Fluency
// contexts, plus the usual three new words on offer.
test.describe('Study Fluency', () => {
  test('opens after the offered batch is answered and shows a card without grading', async ({ page }) => {
    const noOverflow = async () => {
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    };

    await signIn(page);
    await page.goto('/de/study');

    // Nothing is due, but the day's batch is on offer: Fluency waits for it to
    // be taken or declined.
    await expect(page.locator('#study-finished')).toBeVisible();
    await expect(page.locator('#study-offer')).toBeVisible();
    await expect(page.getByRole('heading', { level: 2, name: 'Fluency' })).toHaveCount(0);

    await page.getByRole('button', { name: 'Not today' }).click();

    // A Fluency card: the sentence with the word marked, and the answer hidden.
    await expect(page.getByRole('heading', { level: 2, name: 'Fluency' })).toBeVisible();
    await expect(page.locator('.study-card--fluency .study-card__target')).toBeVisible();
    await expect(page.locator('#fluency-gloss')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Recognised', exact: true })).toHaveCount(0);
    await noOverflow();

    await page.getByRole('button', { name: 'Show answer' }).click();
    await expect(page.locator('#fluency-gloss')).toHaveText('to go');
    await expect(page.locator('#fluency-translation')).toBeVisible();
    await expect(page.locator('#fluency-headword')).toBeVisible();
    // Ungraded: two answers about recognition, and no Again or Got it.
    await expect(page.getByRole('button', { name: 'Recognised', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: "Didn't recognise it" })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Got it' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Again' })).toHaveCount(0);
    await noOverflow();

    const first = await page.locator('.study-card--fluency .study-card__sentence').innerText();
    await page.getByRole('button', { name: "Didn't recognise it" }).click();

    // The next card is another context of the word.
    await expect(page.getByRole('heading', { level: 2, name: 'Fluency' })).toBeVisible();
    await expect(page.locator('#fluency-gloss')).toHaveCount(0);
    const second = await page.locator('.study-card--fluency .study-card__sentence').innerText();
    expect(second).not.toEqual(first);
    await page.getByRole('button', { name: 'Show answer' }).click();
    await page.getByRole('button', { name: 'Recognised', exact: true }).click();

    // Round robin: the first context comes round again.
    await expect(page.locator('.study-card--fluency .study-card__sentence')).toHaveText(first);
    await noOverflow();
  });
});

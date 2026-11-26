import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server for this spec has the usual three new words on offer.
test.describe('Study reports', () => {
  test('reports a wrong word after Show answer, goes on to the next card and lists the report', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    await page.getByRole('button', { name: 'Take these words' }).click();

    // Report waits for Show answer.
    await expect(page.locator('#study-report')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();
    const reported = await page.locator('#study-headword').innerText();

    // Both kinds are shown; only Wrong word can be chosen yet.
    await page.locator('#study-report summary').click();
    await expect(page.getByRole('radio', { name: /Wrong word/ })).toBeChecked();
    await expect(page.getByRole('radio', { name: /Unsuitable context/ })).toBeDisabled();
    await page.getByLabel('Note (optional)').fill('The highlighted word is another word here.');
    await page.getByRole('button', { name: 'Send report' }).click();

    // The next card appears, and the reported word is held.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await expect(page.locator('#study-held')).toContainText('1 word is held');
    await page.getByRole('button', { name: 'Show answer' }).click();
    expect(await page.locator('#study-headword').innerText()).not.toBe(reported);

    // Reported words lists it with its sentence and note.
    await page.getByRole('link', { name: 'Reported words' }).click();
    await expect(page).toHaveURL(/\/de\/study\/reports$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Reported words' })).toBeVisible();
    const entry = page.locator('#study-reports li');
    await expect(entry).toHaveCount(1);
    await expect(entry.getByRole('heading', { level: 2 })).toHaveText(reported);
    await expect(entry.locator('.study-card__target')).toBeVisible();
    await expect(entry).toContainText('The highlighted word is another word here.');
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);

    // Dismiss ends the report: the entry leaves the list and the word is no
    // longer held.
    await entry.getByRole('button', { name: 'Dismiss' }).click();
    await expect(page).toHaveURL(/\/de\/study\/reports$/);
    await expect(page.locator('#study-reports')).toHaveCount(0);
    await expect(page.locator('#study-reports-empty')).toBeVisible();
    await page.goto('/de/study');
    await expect(page.locator('#study-held')).toHaveCount(0);
  });
});

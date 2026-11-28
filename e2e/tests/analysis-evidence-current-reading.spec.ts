import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

// A learner who starts reading a To Read Book with a published analysis, and
// whose newer run then fails, keeps that analysis as the Current reading. The
// Current reading cannot be refreshed, so Reading presents the re-analysis
// failure without a Retry analysis action.
test.describe('Current reading over a failed re-analysis', () => {
  test('Reading shows the re-analysis failure without Retry analysis', async ({ page }) => {
    await signIn(page);

    await page.goto('/de/reading/switch');
    const candidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Neue Analyse fehlgeschlagen' });
    await expect(candidate).toHaveCount(1);
    // Without a cover image the chooser shows a Title plate, hidden from assistive technology.
    const plate = candidate.locator('.book-cover-media__placeholder');
    await expect(plate).toHaveAttribute('aria-hidden', 'true');
    await expect(plate.locator('.title-plate__title')).toBeVisible();
    await candidate.getByRole('button', { name: 'Read next' }).click();

    await page.goto('/de/reading');
    await expect(page.getByRole('heading', { name: 'Neue Analyse fehlgeschlagen' })).toBeVisible();
    // The supporting details are a disclosure whose summary is hidden on desktop,
    // where only Chromium keeps the closed content shown. Open it directly so the
    // assertion checks the evidence wording rather than the disclosure control.
    await page.locator('details.reading-supporting').evaluate((details: HTMLDetailsElement) => { details.open = true; });
    await expect(page.locator('strong', { hasText: 'Re-analysis failed.' })).toBeVisible();
    await expect(page.getByText('The current analysis stays in effect.')).toBeVisible();
    await expect(page.getByRole('button', { name: /^Retry / })).toHaveCount(0);
  });
});

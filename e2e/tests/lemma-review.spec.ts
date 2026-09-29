import { expect, test } from '@playwright/test';

test('exact-form occurrence review is usable without JavaScript', async ({ browser }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop-light', 'The fixture server is shared across browser projects.');
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.goto('/login');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
    await expect(page).toHaveURL(/\/library/);
    const language = page.getByLabel('Study language');
    if (await language.inputValue() !== 'de') await language.selectOption('de');
    await page.goto('/reading/books/fixture-route-match/lemma-review');
    await page.getByLabel('Exact observed form').fill('Weg');
    await page.getByRole('button', { name: 'Find occurrences' }).click();
    await expect(page.getByText('Der Weg führt zum Haus.').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Save correction for this occurrence' })).toHaveCount(2);
  } finally {
    await context.close();
  }
});

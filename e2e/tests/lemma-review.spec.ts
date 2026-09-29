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
    const findOccurrences = page.getByRole('button', { name: 'Find occurrences' });
    await findOccurrences.press('Enter');
    await expect(page.getByText('Der Weg führt zum Haus.').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Save correction for this occurrence' })).toHaveCount(2);
    const exclusionDisclosure = page.locator('details summary').first();
    await findOccurrences.focus();
    for (let tab = 0; tab < 4; tab++) await page.keyboard.press('Tab');
    await expect(exclusionDisclosure).toBeFocused();
    await page.keyboard.press('Enter');
    const excludeButton = page.getByRole('button', { name: 'Exclude this occurrence', exact: true }).first();
    await page.keyboard.press('Tab');
    await expect(excludeButton).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.getByText('excluded for this occurrence.')).toBeVisible();
  } finally {
    await context.close();
  }
});

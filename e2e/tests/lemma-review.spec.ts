import { expect, test } from '@playwright/test';

test('exact-form occurrence review is usable without JavaScript', async ({ browser }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop-light', 'The fixture server is shared across browser projects.');
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/login');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
    await expect(page).toHaveURL(/\/library/);
    const language = page.getByLabel('Study language');
    if (await language.inputValue() !== 'de') await language.selectOption('de');
    await page.goto('/reading/books/fixture-book/lemma-review?form=Weg');
    await expect(page.getByRole('heading', { name: "Stop before changing this Book's vocabulary" })).toBeVisible();
    await expect(page.getByText(/stop this reading without marking it finished/i)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Open Reading to stop this Book' })).toHaveAttribute('href', '/reading');
    await expect(page.getByRole('heading', { name: 'A ready deck is a historical artifact' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Download the existing deck' })).toBeVisible();

    await page.goto('/reading/books/fixture-route-match/lemma-review');
    await page.getByLabel('Exact observed form').fill('Weg');
    const findOccurrences = page.getByRole('button', { name: 'Find occurrences' });
    await findOccurrences.click();
    await expect(page.getByText('Der Weg führt zum Haus.').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Preview correction' })).toHaveCount(2);
    await page.getByRole('checkbox').first().check();
    await page.getByLabel('Corrected canonical lemma').first().fill('Pfad');
    await page.getByRole('button', { name: 'Preview correction' }).first().focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Review the proposed change' })).toBeVisible();
    await expect(page.getByText('This is a preview only. Nothing changes until you confirm.')).toBeVisible();
    await page.getByRole('button', { name: 'Apply correction to selected occurrences' }).click();
    await expect(page.getByText('Effective lemma: pfad (corrected)')).toHaveCount(2);

    const exclusionDisclosure = page.locator('details summary').first();
    await exclusionDisclosure.focus();
    await page.keyboard.press('Enter');
    const excludeButton = page.getByRole('button', { name: 'Preview exclusion' }).first();
    await excludeButton.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Review the proposed change' })).toBeVisible();
    await expect(page.getByText('This is a preview only. Nothing changes until you confirm.')).toBeVisible();
    await page.getByRole('button', { name: 'Exclude selected occurrences from vocabulary' }).click();
    await expect(page.getByText('excluded for this occurrence.')).toBeVisible();
  } finally {
    await context.close();
  }
});

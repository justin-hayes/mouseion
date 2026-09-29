import { expect, test } from '@playwright/test';

test('a corrected occurrence remains reviewable when starting its Book', async ({ page }, testInfo) => {
  test.skip(!process.env.MOUSEION_CORRECTED_START_SMOKE, 'Runs separately so its fixture mutations cannot affect the shared browser suite.');
  test.skip(testInfo.project.name !== 'desktop-light', 'The isolated corrected-start smoke runs once.');

  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
  if (await page.getByLabel('Study language').inputValue() !== 'de') {
    await page.getByLabel('Study language').selectOption('de');
  }

  await page.goto('/reading');
  await page.locator('summary').filter({ hasText: 'Mark reading finished' }).click();
  await page.getByRole('button', { name: 'Mark reading finished' }).click();

  await page.goto('/reading/books/fixture-route-match/lemma-review?form=Weg');
  const occurrence = page.locator('main article').filter({ hasText: 'Observed form: Weg' }).first();
  await occurrence.getByLabel('Corrected canonical lemma').fill('pfad');
  await occurrence.getByRole('button', { name: 'Preview correction' }).click();
  await expect(page.getByRole('heading', { name: 'Review the proposed change' })).toBeVisible();
  await page.getByRole('button', { name: 'Apply correction to selected occurrences' }).click();
  await expect(page.getByText('Effective lemma: pfad (corrected)', { exact: true })).toBeVisible();

  await page.goto('/reading');
  const start = page.locator('form[action="/reading/books/fixture-route-match/start"]');
  await start.locator('xpath=../..').locator('summary').click();
  await start.getByRole('button', { name: 'Confirm start reading' }).click();
  await expect(page.getByRole('heading', { name: 'Current reading' })).toBeVisible();
  await expect(page.getByText('Route match: familiar German is now your current reading.', { exact: true })).toBeVisible();
});

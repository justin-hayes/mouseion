import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
  await page.getByLabel('Study language').selectOption('de');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page).toHaveURL(/\/library$/);
}

test('Browse to Concordance to Study and back restores the row with native navigation', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading?all=1');
  await page.locator('a.vocabulary-lemma-link').last().click();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\?/);
  const back = page.getByRole('link', { name: 'Back to book vocabulary' });
  await expect(back).toBeVisible();

  // An independent search keeps the way back.
  await page.locator('#concordance-term').fill('Haus');
  await page.locator('#concordance-term').press('Enter');
  await expect(page.getByRole('link', { name: 'Back to book vocabulary' })).toBeVisible();

  await page.getByRole('link', { name: 'Back to book vocabulary' }).click();
  await expect(page).toHaveURL(/\/reading\?/);
  await expect(page.locator('#vocabulary-workflow')).toBeVisible();
  await expect(page.locator('tr[id^="browse-row-"]').last()).toBeFocused();

  // Native history still works.
  await page.goBack();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\?/);
});

test('A copied Back link for an expired reading is explained, not restored', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading?language=de&reading=fixture-book&snapshot=stale-snapshot&page=2&q=ha');
  await expect(page.getByRole('heading', { name: 'That book vocabulary was not restored' })).toBeFocused();
  await expect(page.locator('#vocabulary-workflow')).toHaveCount(0);
});

import { expect, Page, test } from '../support/test';

// A Book no other spec reads, so the position entered here (the fixture store
// is shared across the server run and Undo is a later feature) leaks nowhere.
const detail = '/books/fixture-route-tie-a';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
  await page.getByLabel('Study language').selectOption('de');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page).toHaveURL(/\/reading$/);
}

test('the learner enters a percentage on Book detail and sees it as the Reading position', async ({ page }) => {
  await signIn(page);
  await page.goto(detail);

  const section = page.locator('#reading-position');
  await expect(section.getByRole('heading', { level: 2, name: 'Reading position' })).toBeVisible();
  await expect(section).toContainText('No position yet');

  const field = section.getByLabel('Percentage read');
  await expect(field).toHaveAttribute('inputmode', 'decimal');
  await field.fill('101');
  await section.getByRole('button', { name: 'Record Reading position' }).click();
  await expect(page.getByRole('alert')).toContainText('must be from 0 to 100');
  await expect(page.locator('#reading-position').getByLabel('Percentage read')).toHaveValue('101');
  await expect(page.locator('#reading-position')).toContainText('No position yet');

  await page.locator('#reading-position').getByLabel('Percentage read').fill('35.7');
  await page.locator('#reading-position').getByRole('button', { name: 'Record Reading position' }).click();
  await expect(page).toHaveURL(/\/books\/fixture-route-tie-a\?message=/);
  const recorded = page.locator('#reading-position');
  await expect(recorded).toContainText('35%');
  await expect(recorded).toContainText('entered by you');
  await expect(recorded.locator('time')).toHaveAttribute('datetime', /^\d{4}-\d{2}-\d{2}$/);
  await expect(recorded).not.toContainText('No position yet');

  // The form fits every layout, compact included.
  await expect(recorded.getByRole('button', { name: 'Record Reading position' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

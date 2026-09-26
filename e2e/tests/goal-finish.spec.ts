import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

// Keep the cross-project browser assertion read-only because the fixture server
// is shared across projects. One desktop-light journey below exercises the
// complete finish/history/reread loop; isolated Go tests cover idempotency.
test('Current reading exposes an accessible reading-finish action', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading');

  const goal = page.locator('#primary-goal-section');
  const finishDisclosure = goal.locator('details').filter({ hasText: 'Mark reading finished' }).first();
  if (await finishDisclosure.count() === 0) return;

  const finishForm = finishDisclosure.locator('form[action="/goal/finish"]');
  await finishDisclosure.locator('summary').click();
  await expect(finishForm.getByRole('button', { name: 'Mark reading finished' })).toBeVisible();
  await expect(goal).toContainText('Record the reading achievement');
  await expect(finishForm.locator('input[name="csrf_token"]')).toHaveCount(1);
  await expect(finishForm.locator('input[name="expected_goal_book_id"]')).toHaveCount(1);
});

test('finishing current reading records Read history and supports reading again', async ({ page }) => {
  test.skip(test.info().project.name !== 'desktop-light', 'The fixture server is shared across browser projects.');
  await signIn(page);
  await page.getByLabel('Study language').selectOption('it');
  await expect(page).toHaveURL(/\/library$/);
  await page.goto('/reading');

  const currentReading = page.locator('#primary-goal-section');
  if (await currentReading.count() === 0) {
    const title = 'Italian route baseline';
    const candidate = page.locator('article').filter({ has: page.getByRole('heading', { name: title, exact: true }) });
    await expect(page.getByRole('region', { name: 'Below 95%' })).toContainText(title);
    await candidate.locator('details').filter({ hasText: 'Start reading' }).locator('summary').click();
    await candidate.getByRole('button', { name: 'Confirm start reading' }).click();
    await expect(page).toHaveURL(/\/reading\?message=/);
  }
  const title = (await currentReading.getByRole('heading', { level: 3 }).first().textContent())?.trim() ?? '';
  expect(title).toBeTruthy();
  const finishDisclosure = currentReading.locator('details').filter({ hasText: 'Mark reading finished' }).first();
  await finishDisclosure.locator('summary').click();
  await expect(finishDisclosure).toContainText('Record the reading achievement');
  await finishDisclosure.getByRole('button', { name: 'Mark reading finished' }).click();

  await expect(page.getByRole('heading', { name: /Reading finished/i })).toBeVisible();
  await expect(page.locator('#primary-goal-finish-heading')).toBeFocused();
  await expect(page.getByRole('link', { name: 'Choose what to read next' })).toHaveAttribute('href', '/reading');
  await page.goto('/library?history=read');
  const history = page.locator('.library-grid .library-book').filter({ hasText: title });
  await expect(history).toBeVisible();
  await expect(history).toContainText('Read');
  await history.getByText('More actions', { exact: true }).click();
  await expect(history.getByRole('button', { name: 'Read again' })).toBeVisible();
  await history.getByRole('button', { name: 'Read again' }).click();

  await expect(page).toHaveURL(/\/library\?disposition=to_read/);
  const reread = page.locator('.library-grid .library-book').filter({ hasText: title });
  await expect(reread).toContainText('To Read');
  await expect(reread.getByRole('link', { name: 'View in Reading' })).toBeVisible();
});

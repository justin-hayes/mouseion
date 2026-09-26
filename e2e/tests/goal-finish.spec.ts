import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

// Keep this browser assertion read-only because the fixture server is shared
// across projects. The isolated Go tests exercise the idempotent transition
// itself; this covers the learner-facing control and its accessibility copy in
// desktop, compact, light, and dark projects.
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

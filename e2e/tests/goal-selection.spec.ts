import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server is shared by every project/worker, and existing specs
// assert the pristine fixture Goal (fixture-book still holds an active
// learning campaign). This spec therefore only READS state and never mutates
// the Goal; the change/clear/reading-only transitions are covered by Go unit
// and integration tests against isolated databases.
test.describe('Primary Goal selection', () => {
  test('Journey and My Books expose truthful Goal controls and residual state', async ({ page }) => {
    await signIn(page);
    await page.goto('/journey');

    const goal = page.locator('#primary-goal-section');
    await expect(goal).toContainText('Der lange Weg nach Hause');
    await expect(goal).toContainText('Current commitment');
    await expect(goal).toContainText('Vocabulary work remains');
    await expect(goal).toContainText('active campaign and its reserved vocabulary unchanged');
    await expect(goal.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);

    // Clear stays behind an explicit confirmation that explains the residual
    // reservation is retained; opening it must not submit anything.
    const clear = goal.locator('details').filter({ hasText: 'Clear Primary Goal' });
    await expect(clear).toHaveCount(1);
    await clear.locator('summary').press('Enter');
    await expect(clear).toHaveAttribute('open', '');
    await expect(clear).toContainText('active campaign and its reserved vocabulary stay unchanged');
    await expect(clear.getByRole('button', { name: 'Clear Primary Goal' })).toBeVisible();

    const provisional = page.locator('#provisional-journey-list .journey-list > article');
    await expect(provisional).toHaveCount(2);
    await expect(provisional.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(2);

    await page.goto('/library');
    await expect(page.locator('article.library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByText('Current Primary Goal')).toBeVisible();
    await expect(page.getByRole('link', { name: 'View Primary Goal in Reading Journey' })).toHaveAttribute('href', '/journey#journey-book-fixture-book');
    await expect(page.locator('article.library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Choose as Primary Goal' })).toBeVisible();
  });
});
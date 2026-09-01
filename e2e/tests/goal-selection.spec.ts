import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page, disableEnhancement = false) {
  if (disableEnhancement) {
    await page.route('**/static/vendor/htmx-*.js', (route) => route.abort());
  }
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Primary Goal selection', () => {
  test('Journey and My Books expose truthful Goal controls and residual state', async ({ page }) => {
    await signIn(page);
    await page.goto('/journey');

    const goal = page.locator('#primary-goal-section');
    await expect(goal).toContainText('Der lange Weg nach Hause');
    await expect(goal).toContainText('Current commitment');
    await expect(goal).toContainText('Vocabulary work remains');
    await expect(goal).toContainText('active campaign and its reserved vocabulary unchanged');
    await expect(goal.locator('details').filter({ hasText: 'Clear Primary Goal' })).toHaveCount(1);
    await expect(goal.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);

    const provisional = page.locator('#provisional-journey-list .journey-list > article');
    await expect(provisional).toHaveCount(2);
    await expect(provisional.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(2);

    await page.goto('/library');
    await expect(page.locator('article.library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByText('Current Primary Goal')).toBeVisible();
    await expect(page.getByRole('link', { name: 'View Primary Goal in Reading Journey' })).toHaveAttribute('href', '/journey#journey-book-fixture-book');
    await expect(page.locator('article.library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Choose as Primary Goal' })).toBeVisible();
  });

  test('can move to a reading-only Goal, clear it, and restore the fixture Goal', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture Goal runs once per browser suite.');
    await signIn(page, true);
    await page.goto('/journey');

    const currentGoal = page.locator('#primary-goal-section');
    const clear = currentGoal.locator('details').filter({ hasText: 'Clear Primary Goal' });
    await clear.locator('summary').press('Enter');
    await expect(clear).toHaveAttribute('open', '');
    await clear.getByRole('button', { name: 'Clear Primary Goal' }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.locator('#primary-goal-section')).toContainText('No Primary Goal yet');

    await page.locator('#journey-book-fixture-empty').getByRole('button', { name: 'Choose as Primary Goal' }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    const readingOnlyGoal = page.locator('#primary-goal-section');
    await expect(readingOnlyGoal.locator('#journey-book-fixture-empty')).toBeVisible();
    await expect(readingOnlyGoal).toContainText('Reading-only Goal');
    await expect(readingOnlyGoal).toContainText('No analysis or deck exists yet');
    await expect(page.locator('#campaign-fixture-campaign')).toContainText('Active');

    await readingOnlyGoal.getByRole('button', { name: 'Clear Primary Goal' }).press('Enter');
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.locator('#primary-goal-section')).toContainText('No Primary Goal yet');

    await page.goto('/library');
    const fixtureBook = page.locator('article.library-book').filter({ hasText: 'Der lange Weg nach Hause' });
    await fixtureBook.getByRole('button', { name: 'Choose as Primary Goal' }).press('Enter');
    await expect(page).toHaveURL(/\/library\?message=/);
    await expect(page.locator('article.library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByText('Current Primary Goal')).toBeVisible();
  });
});

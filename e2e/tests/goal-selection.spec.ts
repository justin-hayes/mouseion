import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server is shared by every project/worker, and other specs mutate
// it (e.g. keyboard-focus completes the active learning campaign). This spec
// therefore only asserts Goal facts that are stable across those mutations and
// never changes the Goal: the goal book identity, the Clear affordance (present
// directly or behind the residual-work confirmation), the provisional books'
// "Choose as Primary Goal" controls, and the My Books goal badge/link. The
// change/clear/reading-only/residual transitions are covered by Go unit and
// integration tests against isolated databases.
test.describe('Primary Goal selection', () => {
  test('Journey and My Books expose truthful Goal controls', async ({ page }) => {
    await signIn(page);
    await page.goto('/journey');

    const goal = page.locator('#primary-goal-section');
    await expect(goal).toContainText('Der lange Weg nach Hause');
    await expect(goal).toContainText('Current commitment');
    // The goal card always offers a Clear form (directly, or via the
    // residual-work confirmation when an active campaign still reserves
    // vocabulary). It never offers to re-choose itself.
    await expect(goal.locator('form[action="/goal/clear"]')).toHaveCount(1);
    await expect(goal.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);

    const provisional = page.locator('#provisional-journey-list .journey-list > article');
    // Membership can grow across the shared fixture suite (e.g. a deck-flow test
    // adds a book), so assert structurally instead of by exact count.
    await expect(provisional.first()).toBeVisible();
    await expect(provisional.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(await provisional.count());

    await page.goto('/library');
    await expect(page.locator('article.library-book').filter({ hasText: 'Der lange Weg nach Hause' }).getByText('Current Primary Goal')).toBeVisible();
    await expect(page.getByRole('link', { name: 'View Primary Goal in Reading Journey' })).toHaveAttribute('href', '/journey#journey-book-fixture-book');
    await expect(page.locator('article.library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Choose as Primary Goal' })).toBeVisible();
  });
});
import { readNext } from '../support/reading';
import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/reading/);
}

// The fixture store is file-scoped, so the states run in order: each Read next
// replaces the Current reading the previous test left, and the last one ends it.
test.describe.configure({ mode: 'serial' });

test.describe('the Current reading hero', () => {
  test('follows the new Book after Read next', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');
    await expect(page.getByRole('region', { name: 'Der lange Weg nach Hause' })).toBeVisible();
    await readNext(page, 'Route match: familiar German');
    await expect(page.getByRole('region', { name: 'Route match: familiar German' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Der lange Weg nach Hause' })).toHaveCount(0);
    await expect(page.locator('.current-hero')).toHaveCount(1);
  });

  test('keeps its shape with the title plate when the cover is unavailable', async ({ page }) => {
    await signIn(page);
    await readNext(page, 'Route tie A');
    await page.goto('/de/reading');

    const hero = page.getByRole('region', { name: 'Route tie A' });
    await expect(hero).toBeVisible();
    await expect(hero.locator('.book-cover-media__placeholder')).toBeVisible();
    await expect(hero.locator('.book-cover-media__image')).toHaveCount(0);
    await expect(hero.getByText('Currently reading', { exact: true })).toBeVisible();
    await expect(hero.getByRole('link', { name: 'Open Book' })).toBeVisible();
    await expect(hero.locator('.current-hero__status')).toHaveCount(0);
  });

  test('shows a Classical Work once, with its Work title and original title', async ({ page }) => {
    await signIn(page);
    await page.goto('/grc/reading?disposition=to_read');
    const row = page.locator('.library-books .library-book').filter({ hasText: 'Iliad' }).first();
    await row.getByRole('button', { name: 'Read next' }).click();
    await expect(page).toHaveURL(/\/grc\/reading\?message=/);

    const hero = page.getByRole('region', { name: 'Iliad' });
    await expect(page.locator('.current-hero')).toHaveCount(1);
    await expect(hero.getByRole('heading', { level: 2, name: 'Iliad' })).toBeVisible();
    await expect(hero.locator('.current-hero__original')).toHaveText('Ἰλιάς');
    await expect(hero.locator('.current-hero__author')).toHaveText('Homer');
  });

  test('has no hero without a Current reading', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');
    const end = page.locator('#primary-goal-section details.confirmation').filter({ hasText: 'End current reading' });
    await end.locator('summary').click();
    await end.getByRole('button', { name: 'Confirm end current reading' }).click();
    await expect(page.getByRole('status').first()).toContainText('Reading ended');
    await expect(page.locator('.current-hero')).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Current reading' })).toHaveCount(0);
  });
});

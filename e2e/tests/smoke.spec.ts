import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/$/);
}

test.describe('authenticated learner smoke', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('login and library expose representative content', async ({ page }) => {
    await expect(page.getByRole('heading', { name: /library|welcome/i }).first()).toBeVisible();
    await page.getByRole('link', { name: /library/i }).first().click();
    await expect(page).toHaveURL(/\/library/);
    await expect(page.getByText('Der lange Weg nach Hause')).toBeVisible();
    await expect(page.getByText('Fehlgeschlagene Analyse')).toBeVisible();
    await expect(page.getByText('Empty chapter')).toBeVisible();
  });

  test('exact analysis result and deck status are reachable', async ({ page }) => {
    await page.goto('/books/fixture-book/analyses/fixture-run');
    await expect(page.getByRole('heading', { name: /analysis result/i })).toBeVisible();
    await expect(page.getByText('Der lange Weg nach Hause')).toBeVisible();
    await page.goto('/deck-preparations/fixture-preparation/status');
    await expect(page.getByText(/Fixture German deck/i)).toBeVisible();
  });

  test('acquisition, Learning, Settings, and operational jobs are reachable', async ({ page }) => {
    await page.goto('/connections');
    await expect(page.getByText('Fixture catalog')).toBeVisible();
    await page.goto('/catalog?connection=fixture-connection');
    await expect(page.getByText('Fixture catalog')).toBeVisible();
    await expect(page.getByText('Ein deutsches Buch')).toBeVisible();
    await expect(page.getByRole('button', { name: /add to library/i })).toBeVisible();
    await page.goto('/campaigns');
    await expect(page.getByRole('heading', { name: /learning/i })).toBeVisible();
    await expect(page.getByText(/Der lange Weg nach Hause/)).toBeVisible();
    await page.goto('/settings');
    await expect(page.getByText('German')).toBeVisible();
    await expect(page.getByText('Italian')).toBeVisible();
    await page.goto('/jobs');
    await expect(page.getByText(/Analysis job/i).first()).toBeVisible();
  });

  test('asserts initial HTML before HTMX enhancement and observes status', async ({ page }) => {
    await page.goto('/settings');
    const importForm = page.locator('form[hx-post="/known-vocab/import"]');
    await expect(importForm).toHaveCount(1);
    await expect(page.locator('#known-vocabulary-results')).toContainText(/import|known vocabulary/i);
    await page.locator('input[type="file"]').setInputFiles({ name: 'fixture.txt', mimeType: 'text/plain', buffer: Buffer.from('Haus\nÜberraschung\n') });
    await importForm.getByRole('button', { name: /import known vocabulary/i }).click();
    await expect(page.locator('#known-vocabulary-results')).toContainText(/queued|known vocabulary import/i);
  });
});

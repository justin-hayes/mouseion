import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
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
    await expect(page.getByRole('heading', { name: 'Fixture catalog' })).toBeVisible();
    // The catalog page offers the language browse form (German/Italian).
    await expect(page.getByText('German')).toBeVisible();
    await expect(page.getByText('Italian')).toBeVisible();
    // Browse a language via the server-rendered path and assert feed entries.
    await page.goto('/opds/browse?connection=fixture-connection&language=de');
    await expect(page.getByText('Ein deutsches Buch')).toBeVisible();
    await expect(page.getByText('Un libro italiano')).toBeVisible();
    await expect(page.getByRole('button', { name: /add to library/i }).first()).toBeVisible();
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
    // The import form and its results region are server-rendered only once a
    // study language is selected on the settings page.
    await page.goto('/settings?language=de');
    const importForm = page.locator('form[hx-post*="/known-vocab/import"]');
    // Initial (pre-enhancement) HTML already carries the form and swap target.
    await expect(importForm).toHaveCount(1);
    await expect(page.locator('#known-vocabulary-results')).toHaveCount(1);
    // Attach a small multilingual UTF-8 lemma file, then submit via HTMX.
    await importForm.locator('input[type="file"]').setInputFiles({
      name: 'known-de.txt', mimeType: 'text/plain',
      buffer: Buffer.from('Haus\nÜberraschung\n'),
    });
    await importForm.getByRole('button', { name: /import known vocabulary/i }).click();
    await expect(page).toHaveURL(/\/settings/);
    // The HTMX submission replaces the region with a durable status.
    await expect(page.locator('#known-vocabulary-results')).toContainText(/queued/i);
  });
});

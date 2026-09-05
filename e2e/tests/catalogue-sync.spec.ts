import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('catalogue sync status', () => {
  test('shows each connection state and its server-rendered recovery path', async ({ page }) => {
    await signIn(page);
    await page.goto('/connections');

    const states = [
      { id: 'fixture-connection', label: 'Last synced', detail: /3 books added or updated/ },
      { id: 'fixture-failed-connection', label: 'Sync failed', detail: /Authentication failed for this connection/ },
      { id: 'fixture-syncing-connection', label: 'Syncing', detail: /Existing Books remain available/ },
      { id: 'fixture-never-synced-connection', label: 'Never synced', detail: /ready non-English catalogue languages/ },
    ];

    for (const state of states) {
      const article = page.locator(`article#connection-${state.id}`);
      await expect(article).toBeVisible();
      await expect(article).toContainText(state.label);
      await expect(article).toContainText(state.detail);
      await expect(article.getByRole('button', { name: 'Sync now' })).toHaveCount(1);
    }
    await expect(page.locator('#connection-fixture-never-synced-connection')).not.toContainText(/ready study-language metadata/);

    await expect(page.locator('#connection-fixture-syncing-connection button[disabled]')).toHaveCount(1);
    await expect(page.locator('#connection-fixture-syncing-connection').getByText('Sync already in progress.')).toBeVisible();
    await expect(page.getByRole('link', { name: 'View job status' }).first()).toHaveAttribute('href', '/jobs');
  });
});

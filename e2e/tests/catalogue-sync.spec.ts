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
    await page.goto('/catalogs');

    const states = [
      { id: 'fixture-connection', label: 'Last synced', detail: /3 books added or updated/ },
      { id: 'fixture-failed-connection', label: 'Sync failed', detail: /Authentication failed for this connection/ },
      { id: 'fixture-syncing-connection', label: 'Syncing', detail: /Existing Books remain available/ },
      { id: 'fixture-never-synced-connection', label: 'Never synced', detail: /ready non-English catalog languages/ },
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

  test('exposes Catalogs as the current shell destination and retires the old route', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');
    await expect(page).toHaveTitle('Catalogs · Mouseion');
    await expect(page.getByRole('heading', { name: 'Catalogs', exact: true })).toBeVisible();
    await expect(page.locator('nav.site-header__nav a[aria-current="page"]')).toHaveText('Catalogs');

    const redirect = await page.request.get('/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=drop', { maxRedirects: 0 });
    expect(redirect.status()).toBe(301);
    expect(redirect.headers().location).toBe('/catalogs?book_id=book%2F1&error=Try+again&message=Catalog+added');
  });
});

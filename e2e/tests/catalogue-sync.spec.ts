import { expect, Page, test } from '@playwright/test';
import { renderedTextContrast } from '../support/contrast';

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

  test('shared connection and sync controls preserve enabled, disabled, and field states', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');

    for (const theme of ['light', 'dark']) {
      await page.locator('html').evaluate((node, value) => node.setAttribute('data-theme', value), theme);
      const enabled = page.locator('#connection-fixture-connection').getByRole('button', { name: 'Sync now' });
      const disabled = page.locator('#connection-fixture-syncing-connection').getByRole('button', { name: 'Sync now' });
      await expect(enabled).toHaveClass(/\bbutton\b/);
      await expect(disabled).toHaveClass(/\bbutton\b/);
      await expect(enabled).toBeEnabled();
      await expect(disabled).toBeDisabled();
      await expect(page.locator('#connection-fixture-syncing-connection')).toContainText('Sync already in progress.');
      const opacity = await Promise.all([enabled, disabled].map(control => control.evaluate(node => Number(getComputedStyle(node).opacity))));
      expect(opacity[0]).toBeGreaterThan(opacity[1]);
      expect(await enabled.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);

      const addForm = page.locator('#catalog-connection-form form');
      const name = addForm.getByLabel('Name', { exact: true });
      const url = addForm.getByLabel('Catalog URL', { exact: true });
      await expect(name).toHaveClass(/\binput\b/);
      await expect(url).toHaveClass(/\binput\b/);
      await expect(name).toHaveAttribute('required', '');
      expect(await name.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);
      expect(await url.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);
      await name.focus();
      expect(await name.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('solid');
      await addForm.getByRole('button', { name: 'Add catalog' }).click();
      expect(await name.evaluate(node => (node as HTMLInputElement).validity.valid)).toBe(false);
      expect(await name.evaluate(node => node.matches(':user-invalid'))).toBe(true);
      expect(await name.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);

      const editForm = page.locator('#edit-connection-fixture-connection form.catalog-connection-form');
      await expect(editForm).toHaveAttribute('method', 'post');
      await expect(editForm).toHaveAttribute('action', '/connections/fixture-connection');
      await expect(editForm.getByLabel('Name', { exact: true })).toHaveClass(/\binput\b/);
    }
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

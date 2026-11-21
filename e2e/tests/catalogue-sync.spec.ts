import { expect, Page, test } from '../support/test';
import { renderedTextContrast } from '../support/contrast';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('catalog sources', () => {
  test('shows each connection state and its recovery path on the Catalog', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');

    const states = [
      { id: 'fixture-connection', label: 'Last synced', detail: /3 books added or updated/ },
      { id: 'fixture-failed-connection', label: 'Sync failed', detail: /Authentication failed for this connection/ },
      { id: 'fixture-syncing-connection', label: 'Syncing', detail: /Existing Books remain available/ },
      { id: 'fixture-never-synced-connection', label: 'Never synced', detail: /ready non-English catalog languages/ },
    ];

    for (const state of states) {
      const line = page.locator(`li#source-status-${state.id}`);
      await expect(line).toBeVisible();
      await expect(line).toContainText(state.label);
      await expect(line).toContainText(state.detail);
      await expect(line.getByRole('button', { name: 'Sync now' })).toHaveCount(1);
    }
    await expect(page.locator('#source-status-fixture-never-synced-connection')).not.toContainText(/ready study-language metadata/);

    await expect(page.locator('#source-status-fixture-syncing-connection button[disabled]')).toHaveCount(1);
    await expect(page.locator('#source-status-fixture-syncing-connection').getByText('Sync already in progress.')).toBeVisible();
    await expect(page.locator('#source-status-fixture-failed-connection').getByRole('link', { name: 'Manage sources' })).toHaveAttribute('href', '/catalogs/sources#connection-fixture-failed-connection');
    await expect(page.locator('.page-header__actions').getByRole('button', { name: 'Manage sources', exact: true })).toHaveAttribute('href', '/catalogs/sources');
    await expect(page.getByRole('link', { name: 'View job status' }).first()).toHaveAttribute('href', '/jobs');

    // The Catalog keeps the built-in Classical source and does not manage connections.
    await expect(page.getByRole('heading', { name: 'Classical catalog', exact: true })).toBeVisible();
    await expect(page.locator('#catalog-connection-form')).toHaveCount(0);
  });

  test('Manage sources preserves enabled, disabled, and field states', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs/sources');

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
      await expect(editForm).toHaveAttribute('action', '/catalogs/sources/fixture-connection');
      await expect(editForm.getByLabel('Name', { exact: true })).toHaveClass(/\binput\b/);
    }
  });

  test('exposes Catalog as the current shell destination and retires the old route', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');
    await expect(page).toHaveTitle('Catalog · Mouseion');
    await expect(page.getByRole('heading', { name: 'Catalog', exact: true })).toBeVisible();
    await expect(page.locator('nav.site-header__nav a[aria-current="page"]')).toHaveText('Catalog');

    const redirect = await page.request.get('/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=drop', { maxRedirects: 0 });
    expect(redirect.status()).toBe(301);
    expect(redirect.headers().location).toBe('/catalogs?book_id=book%2F1&error=Try+again&message=Catalog+added');

    // GET /catalogs still carries its supported query parameters.
    const direct = await page.request.get('/catalogs?message=Catalog+added&error=Try+again&book_id=book%2F1', { maxRedirects: 0 });
    expect(direct.status()).toBe(200);
  });

  test('Manage sources is reachable without JavaScript and keeps native controls', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    const page = await context.newPage();
    try {
      await signIn(page);
      await page.goto('/catalogs');
      const manageSources = page.locator('.page-header__actions').getByRole('button', { name: 'Manage sources', exact: true });
      await expect(manageSources).toBeVisible();
      await manageSources.click();
      await expect(page).toHaveURL(/\/catalogs\/sources$/);
      await expect(page.getByRole('heading', { name: 'Manage sources', exact: true })).toBeVisible();

      const addForm = page.locator('#catalog-connection-form form');
      await expect(addForm).toHaveAttribute('method', 'post');
      await expect(addForm).toHaveAttribute('action', '/catalogs/sources');
      await expect(addForm.getByLabel('Name', { exact: true })).toHaveAttribute('required', '');
      await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveAttribute('required', '');
      await expect(addForm.evaluate(form => (form as HTMLFormElement).reportValidity())).resolves.toBe(false);

      const deletion = page.locator('#connection-fixture-connection details.confirmation');
      await expect(deletion.locator('summary')).toHaveText('Delete catalog connection');
      await deletion.locator('summary').click();
      await expect(deletion.getByRole('button', { name: 'Confirm deletion' })).toBeVisible();
    } finally {
      await context.close();
    }
  });

  test('Catalog source controls follow keyboard order at narrow widths', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/catalogs');

    const manage = page.locator('.page-header__actions').getByRole('button', { name: 'Manage sources', exact: true });
    await manage.focus();
    await page.keyboard.press('Tab');
    await expect(page.locator('#source-status-fixture-connection').getByRole('button', { name: 'Sync now' })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('#source-status-fixture-failed-connection').getByRole('link', { name: 'Manage sources' })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('#source-status-fixture-failed-connection').getByRole('button', { name: 'Sync now' })).toBeFocused();

    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  });
});

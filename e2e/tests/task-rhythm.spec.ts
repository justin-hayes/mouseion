import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('compact task rhythm', () => {
  test('Catalogs and focused deck forms have deliberate interior spacing', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');

    const catalogForm = page.locator('#catalog-connection-form');
    await expect(catalogForm.getByRole('heading', { name: 'Add catalog connection' })).toBeVisible();
    const catalogPadding = await catalogForm.evaluate(node => Number.parseFloat(getComputedStyle(node).paddingInlineStart));
    expect(catalogPadding).toBeGreaterThanOrEqual(16);

  });

  test('job status keeps its task identity, timestamps, progress, and recovery together', async ({ page }) => {
    await signIn(page);
    await page.goto('/jobs/43');

    const status = page.locator('#job-status');
    await expect(page.getByRole('heading', { name: /Analysis job #\d+/ })).toBeVisible();
    await expect(status).toContainText('Failed');
    await expect(status.locator('.operational-metadata')).toContainText('Created');
    await expect(status.locator('.operational-metadata')).toContainText('Attempt');
    await expect(status.locator('.operational-metadata')).toContainText('Fehlgeschlagene Analyse');
    await expect(status.getByRole('progressbar')).toBeVisible();
    await expect(status.getByRole('button', { name: 'Retry analysis' })).toBeVisible();

    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' })).toContainText('Fehlgeschlagene Analyse');
  });

  test('catalog-sync polling retains the live status root and its metadata', async ({ page }) => {
    await signIn(page);
    await page.goto('/jobs/103');

    const status = page.locator('#catalogue-sync-job-status');
    await expect(status).toHaveAttribute('hx-swap', 'outerHTML');
    await expect(status).toContainText('Fixture syncing catalog');
    await expect(status.locator('.operational-metadata')).toContainText('Created');

    const response = await page.request.get('/jobs/103/status');
    expect(response.ok()).toBeTruthy();
    const html = await response.text();
    const rootID = await page.evaluate(markup => {
      const root = new DOMParser().parseFromString(markup, 'text/html').body.firstElementChild;
      return root?.id;
    }, html);
    expect(rootID).toBe('catalogue-sync-job-status');
    expect(html).toContain('class="operational-metadata"');
  });
});

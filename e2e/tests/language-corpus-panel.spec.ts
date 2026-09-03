import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test('selected My Books language shows evidence and zoom links', async ({ page }) => {
  await signIn(page);
  await page.goto('/library');
  await page.getByRole('link', { name: /^de /i }).click();

  const panel = page.locator('#language-view-panel');
  await expect(panel).toBeVisible();
  await expect(panel.getByRole('heading', { level: 2 })).toHaveText(/Coverage across|Language view/i);
  await expect(panel.getByRole('heading', { level: 2 })).not.toContainText(/Corpus/i);
  await expect(panel.locator('.language-view-aggregates > section')).toHaveCount(4);
  await expect(panel.getByRole('heading', { name: 'Analyzed books', exact: true })).toBeVisible();
  await expect(panel.getByText(/\d+\.\d+%.*\d+ of \d+ tokens/)).toBeVisible();
  await expect(panel.getByRole('heading', { name: 'Highest-impact unknown vocabulary', exact: true })).toBeVisible();
  await expect(panel.getByRole('heading', { name: 'Per-book spread', exact: true })).toBeVisible();
  await expect(panel.getByText(/Excluded: analysis failed or incomplete/)).toBeVisible();
  await expect(panel.getByText(/Excluded: no current acquired source/)).toBeVisible();
  await expect(panel.getByRole('button')).toHaveCount(0);

  const zoomLinks = panel.locator('.language-view-book-list a');
  expect(await zoomLinks.count()).toBeGreaterThan(0);
  for (const link of await zoomLinks.all()) {
    await expect(link).toHaveAttribute('href', /^\/books\//);
  }
});

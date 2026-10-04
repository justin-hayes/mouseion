import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

async function clearSelection(page: Page) {
  await page.goto('/vocabulary/selection/clear-confirm');
  await page.getByRole('button', { name: 'Confirm clear selection' }).click();
}

test('Current-reading Browse keeps its prefix form usable without JavaScript', async ({ page, browser }) => {
  const baseURL = process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099';
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await signIn(noScriptPage);
    await noScriptPage.goto('/vocabulary');
    await expect(noScriptPage.getByRole('heading', { name: 'Vocabulary · Browse' })).toBeVisible();
    const browseForm = noScriptPage.locator('form[action="/vocabulary"]');
    await expect(browseForm.locator('input[name="reading"]')).toHaveValue('fixture-book');
    const includeAll = noScriptPage.getByRole('checkbox', { name: 'Show already accounted-for words' });
    await expect(includeAll).not.toBeChecked();
    await includeAll.check();
    const search = noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' });
    await search.focus();
    await expect(search).toBeFocused();
    await search.fill('haus');
    await search.press('Enter');
    await expect(noScriptPage).toHaveURL(/\/vocabulary\?.*all=1.*q=haus|\/vocabulary\?.*q=haus.*all=1/);
    await expect(noScriptPage.locator('#vocabulary-results-heading')).toBeFocused();
    await expect(browseForm.locator('input[name="book"], input[name="pos"], select[name="known"], select[name="reserved"], select[name="sort"]')).toHaveCount(0);
    const hausRow = noScriptPage.getByRole('row').filter({ hasText: 'haus' });
    await expect(hausRow).toContainText('Known');
    await expect(hausRow).toContainText('In a Book deck');
    const noOverflow = await noScriptPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    expect(noOverflow).toBe(true);
  } finally {
    await noScriptContext.close();
  }

  await signIn(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto('/vocabulary');
  await expect(page.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('Browse keeps the current page and controls while selecting a word on page two', async ({ page }) => {
  await signIn(page);
  await clearSelection(page);
  await page.goto('/vocabulary?q=paging');
  await page.locator('form[action="/vocabulary"]').evaluate(element => element.setAttribute('data-stable-search', 'yes'));
  await page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link', { name: 'Next' }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.locator('form[action="/vocabulary"]')).toHaveAttribute('data-stable-search', 'yes');
  await page.locator('#vocabulary-workflow').evaluate(element => element.setAttribute('data-stable-controls', 'yes'));
  const row = page.getByRole('row').filter({ hasText: 'paging25' });
  await row.getByRole('button', { name: 'Select', exact: true }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.locator('#vocabulary-workflow')).toHaveAttribute('data-stable-controls', 'yes');
  await expect(page.getByRole('link', { name: 'Browse selection (1)' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Remove from selection' })).toBeFocused();
  await row.getByRole('button', { name: 'Remove from selection' }).click();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Browse selection (0)' })).toBeVisible();
  await expect(row.getByRole('button', { name: 'Select', exact: true })).toBeFocused();
});

test('Browse selection returns to the same page without JavaScript', async ({ browser }) => {
  const noScriptContext = await browser.newContext({ baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099', javaScriptEnabled: false });
  const page = await noScriptContext.newPage();
  try {
    await signIn(page);
    await clearSelection(page);
    await page.goto('/vocabulary?q=paging&page=2&reading=fixture-book');
    await page.getByRole('row').filter({ hasText: 'paging25' }).getByRole('button', { name: 'Select', exact: true }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByText('Page 2 of 2')).toBeVisible();
    await expect(page.getByRole('row').filter({ hasText: 'paging25' }).getByRole('button', { name: 'Remove from selection' })).toBeVisible();
  } finally {
    await noScriptContext.close();
  }
});

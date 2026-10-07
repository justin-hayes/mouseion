import { expect, Page, test } from '@playwright/test';

function fixtureBaseURL() {
  return process.env.MOUSEION_FIXTURE_URL ?? `http://${process.env.MOUSEION_FIXTURE_ADDR ?? '127.0.0.1:8099'}`;
}

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
  await page.getByLabel('Study language').selectOption('de');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page).toHaveURL(/\/library$/);
}

test('Current-reading Browse keeps its prefix form usable without JavaScript', async ({ page, browser }) => {
  const baseURL = fixtureBaseURL();
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await signIn(noScriptPage);
    await noScriptPage.goto('/vocabulary');
    await expect(noScriptPage.getByRole('heading', { name: 'Vocabulary · Browse' })).toBeVisible();
    await expect(noScriptPage.locator('body')).not.toContainText('Browse selection');
    await expect(noScriptPage.locator('body')).not.toContainText('Custom deck');
    await expect(noScriptPage.getByRole('button', { name: 'Select', exact: true })).toHaveCount(0);
    await expect(noScriptPage.getByRole('button', { name: 'Remove', exact: true })).toHaveCount(0);
    const browseForm = noScriptPage.locator('form[action="/vocabulary"]');
    await expect(browseForm.locator('input[name="reading"]')).toHaveValue('fixture-book');
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveClass(/\binput\b/);
    const includeAll = noScriptPage.getByRole('checkbox', { name: 'Show already accounted-for words' });
    await expect(includeAll).not.toBeChecked();
    const checkboxSize = await includeAll.evaluate(element => {
      const rect = element.getBoundingClientRect();
      return { width: rect.width, height: rect.height, labelHeight: element.closest('label')!.getBoundingClientRect().height };
    });
    expect(checkboxSize.width).toBeLessThan(44);
    expect(checkboxSize.height).toBeLessThan(44);
    expect(checkboxSize.labelHeight).toBeGreaterThanOrEqual(44);
    await expect(browseForm.getByRole('button', { name: 'Search' })).toHaveClass(/\bbtn\b/);
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

test('Browse keeps the current page and controls while exploring a word on page two', async ({ page }) => {
  await signIn(page);
  await page.goto('/vocabulary?q=paging');
  await page.locator('form[action="/vocabulary"]').evaluate(element => element.setAttribute('data-stable-search', 'yes'));
  await page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link', { name: 'Next' }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.locator('form[action="/vocabulary"]')).toHaveAttribute('data-stable-search', 'yes');
  await page.locator('#vocabulary-workflow').evaluate(element => element.setAttribute('data-stable-controls', 'yes'));
  const row = page.getByRole('row').filter({ hasText: 'paging25' });
  await expect(row.getByRole('link', { name: 'paging25' })).toBeVisible();
  await expect(row.getByRole('button')).toHaveCount(0);
  await expect(page.locator('#vocabulary-workflow')).toHaveAttribute('data-stable-controls', 'yes');
  await expect(page.locator('body')).not.toContainText('Browse selection');
  await expect(page.locator('body')).not.toContainText('Custom deck');
});

test('Browse pagination lands on the new results rather than above them', async ({ page }) => {
  await signIn(page);
  await page.goto('/vocabulary?q=paging');
  for (const [direction, number] of [['Next', 2], ['Previous', 1]] as const) {
    await page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link', { name: direction }).click();
    await expect(page.getByText(`Page ${number} of 2`)).toBeVisible();
    const heading = page.locator('#vocabulary-results-heading');
    await expect(heading).toBeFocused();
    const results = await heading.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return { top: bounds.top, bottom: bounds.bottom, viewportHeight: window.innerHeight };
    });
    expect(results.top).toBeGreaterThanOrEqual(-1); // Subpixel rounding at narrow widths.
    expect(results.bottom).toBeLessThan(results.viewportHeight);
    expect(results.top).toBeLessThan(160);
  }
});

import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
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
    const search = noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' });
    await search.focus();
    await expect(search).toBeFocused();
    await search.fill('haus');
    await search.press('Enter');
    await expect(noScriptPage).toHaveURL(/\/vocabulary\?q=haus/);
    await expect(browseForm.locator('input[name="book"], input[name="pos"], select[name="known"], select[name="reserved"], select[name="sort"]')).toHaveCount(0);
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

import { expect, Page, test } from '../support/test';

// The fixture server for this file seeds twelve German reviews that are due.

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Study tab due chip', () => {
  test('shows the due count on every page with the destination and count as its name', async ({ page }) => {
    await signIn(page);
    for (const path of ['/de/library', '/de/study', '/jobs', '/catalogs']) {
      await page.goto(path);
      const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
      const study = navigation.getByRole('link', { name: 'Study, 12 reviews due', exact: true });
      await expect(study).toBeVisible();
      await expect(study.locator('.site-nav__count')).toHaveText('12');
      await expect(study).toContainText('Study');
    }
  });

  test('fits the header without wrapping the destinations', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/library');
    const links = page.locator('.site-header__navigation a');
    await expect(links).toHaveCount(4);
    const boxes = await links.evaluateAll(items => items.map(item => {
      const box = item.getBoundingClientRect();
      return { top: box.top, bottom: box.bottom, right: box.right };
    }));
    for (const box of boxes) expect(Math.abs(box.top - boxes[0].top)).toBeLessThan(2);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    const chip = await page.locator('.site-nav__count').evaluate(item => {
      const box = item.getBoundingClientRect();
      return { height: box.height, width: box.width };
    });
    expect(chip.height).toBeLessThan(32);
    expect(chip.width).toBeGreaterThan(0);
  });
});

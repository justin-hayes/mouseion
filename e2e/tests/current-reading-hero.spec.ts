import { readNext } from '../support/reading';
import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/reading/);
}

async function expectNoOverflow(page: Page) {
  const sizes = await page.evaluate(() => ({ document: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  expect(sizes.document).toBeLessThanOrEqual(sizes.viewport + 1);
}

// The fixture store is file-scoped, so the states run in order: each Read next
// replaces the Current reading the previous test left, and the last one ends it.
test.describe.configure({ mode: 'serial' });

test.describe('the Current reading hero', () => {
  for (const colorScheme of ['light', 'dark'] as const) {
    test(`shows both figures and the two-segment bar compact and wide, ${colorScheme}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme });
      await signIn(page);
      await page.goto('/de/reading');
      const hero = page.getByRole('region', { name: 'Der lange Weg nach Hause' });
      const mature = hero.locator('[data-current-reading-mature]');
      const inStudy = hero.locator('[data-current-reading-in-study]');
      const bar = hero.getByRole('img', { name: 'Mature 37.0%; with words in study 42.1%' });

      for (const viewport of [{ width: 390, height: 844 }, { width: 1280, height: 800 }]) {
        await page.setViewportSize(viewport);
        await expect(mature).toHaveText('37.0%');
        await expect(inStudy).toHaveText('42.1%');
        await expect(hero.getByText('45,678 of 123,456 word occurrences are Mature.')).toBeVisible();
        await expect(bar).toBeVisible();
        // The bar stacks Mature, then only the share beyond it.
        const widths = await bar.locator('span').evaluateAll((nodes) => nodes.map((node) => node.getBoundingClientRect().width));
        const total = await bar.evaluate((node) => node.getBoundingClientRect().width);
        expect(widths[0] / total).toBeCloseTo(0.37, 2);
        expect(widths[1] / total).toBeCloseTo(0.051, 2);
        const colours = await bar.locator('span').evaluateAll((nodes) => nodes.map((node) => getComputedStyle(node).backgroundColor));
        expect(colours[0]).not.toBe(colours[1]);
        await expectNoOverflow(page);
      }
    });
  }

  test('keeps the cover, title, both figures and every action on the first screen at 390px', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 390, height: 844 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto('/de/reading');
      const hero = page.getByRole('region', { name: 'Der lange Weg nach Hause' });
      const parts = {
        cover: hero.locator('.current-hero__cover'),
        title: hero.getByRole('heading', { level: 2 }),
        mature: hero.locator('[data-current-reading-mature]'),
        inStudy: hero.locator('[data-current-reading-in-study]'),
        open: hero.getByRole('link', { name: 'Open Book' }),
        finish: hero.getByText('Mark reading finished', { exact: true }).first(),
        end: hero.getByText('End current reading', { exact: true }).first(),
      };
      for (const [name, locator] of Object.entries(parts)) {
        await expect(locator, `${name} (${colorScheme})`).toBeVisible();
        const box = (await locator.boundingBox())!;
        expect(box.y, `${name} starts on the first screen (${colorScheme})`).toBeGreaterThanOrEqual(0);
        expect(box.y + box.height, `${name} ends on the first screen (${colorScheme})`).toBeLessThanOrEqual(844);
      }
    }
  });

  test('follows the new Book after Read next', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');
    await expect(page.getByRole('region', { name: 'Der lange Weg nach Hause' })).toBeVisible();
    await readNext(page, 'Route match: familiar German');
    await expect(page.getByRole('region', { name: 'Route match: familiar German' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Der lange Weg nach Hause' })).toHaveCount(0);
    await expect(page.locator('.current-hero')).toHaveCount(1);
  });

  test('keeps its shape with the title plate when the cover is unavailable', async ({ page }) => {
    await signIn(page);
    await readNext(page, 'Route tie A');
    await page.goto('/de/reading');

    const hero = page.getByRole('region', { name: 'Route tie A' });
    await expect(hero).toBeVisible();
    await expect(hero.locator('.book-cover-media__placeholder')).toBeVisible();
    await expect(hero.locator('.book-cover-media__image')).toHaveCount(0);
    await expect(hero.getByText('Currently reading', { exact: true })).toBeVisible();
    await expect(hero.getByRole('link', { name: 'Open Book' })).toBeVisible();
    await expect(hero.locator('.current-hero__status')).toHaveCount(0);
  });

  test('reads Updating, not zero, while the counts are updating', async ({ page }) => {
    await signIn(page);
    await readNext(page, 'Route tie B');
    await page.goto('/de/reading');
    const hero = page.getByRole('region', { name: 'Route tie B' });
    await expect(hero.locator('.current-hero__figure')).toHaveText(['Updating', 'Updating']);
    await expect(hero.getByText('Coverage returns when the counts are ready.')).toBeVisible();
    await expect(hero.locator('.coverage-bar')).toHaveCount(0);
    await expect(hero).not.toContainText('0.0%');
    await expect(hero.locator('.current-hero__status')).toHaveCount(0);
  });

  test('reads Unavailable, not zero, when the Book has no statistics', async ({ page }) => {
    await signIn(page);
    await readNext(page, 'Route differs: new German');
    await page.goto('/de/reading');
    const hero = page.getByRole('region', { name: 'Route differs: new German' });
    await expect(hero.locator('.current-hero__figure')).toHaveText(['Unavailable', 'Unavailable']);
    await expect(hero.getByText('Coverage returns when the counts are ready.')).toBeVisible();
    await expect(hero.locator('.coverage-bar')).toHaveCount(0);
    await expect(hero).not.toContainText('0.0%');
  });

  test('shows a Classical Work once, with its Work title and original title', async ({ page }) => {
    await signIn(page);
    await page.goto('/grc/reading?disposition=to_read');
    const row = page.locator('.library-books .library-book').filter({ hasText: 'Iliad' }).first();
    await row.getByRole('button', { name: 'Read next' }).click();
    await expect(page).toHaveURL(/\/grc\/reading\?message=/);

    const hero = page.getByRole('region', { name: 'Iliad' });
    await expect(page.locator('.current-hero')).toHaveCount(1);
    await expect(hero.getByRole('heading', { level: 2, name: 'Iliad' })).toBeVisible();
    await expect(hero.locator('.current-hero__original')).toHaveText('Ἰλιάς');
    await expect(hero.locator('.current-hero__author')).toHaveText('Homer');
  });

  test('has no hero without a Current reading', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');
    const end = page.locator('#primary-goal-section details.confirmation').filter({ hasText: 'End current reading' });
    await end.locator('summary').click();
    await end.getByRole('button', { name: 'Confirm end current reading' }).click();
    await expect(page.getByRole('status').first()).toContainText('Reading ended');
    await expect(page.locator('.current-hero')).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Current reading' })).toHaveCount(0);
  });
});

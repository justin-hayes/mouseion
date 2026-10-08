import { expect, test } from '../support/test';

test('switch chooser keeps Book actions with identity and coverage in the margin', async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
  await page.getByLabel('Study language').selectOption('de');
  await expect(page).toHaveURL(/\/library$/);
  await page.goto('/reading/switch');
  const candidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Route match: familiar German' });
  await expect(candidate.getByRole('heading', { name: 'Route match: familiar German' })).toBeVisible();
  const identity = candidate.locator('.reading-chooser-entry__identity');
  await expect(identity).toContainText('By');
  await expect(identity.locator('summary').filter({ hasText: 'Switch to this book' })).toBeVisible();
  await expect(identity.getByRole('link', { name: 'Review a word in this Book' })).toHaveAttribute('href', /\/lemma-review$/);
  const evidence = candidate.locator('.reading-chooser-entry__evidence');
  await expect(evidence).toContainText(/Known vocabulary coverage:\s*\d+\.\d%/);
  await expect(evidence).toContainText('45,678 of 123,456 word occurrences');
  await expect(evidence).toContainText('word occurrences');
  await expect(evidence).toContainText(/distinct words marked Known|At least 99%/);
  await expect(evidence.getByRole('button', { name: /switch/i })).toHaveCount(0);
  const retry = page.getByRole('button', { name: 'Retry acquisition or analysis' }).first();
  await expect(retry).toBeVisible();
  await expect(retry.locator('xpath=..')).toHaveAttribute('action', /\/reading\/books\/[^/]+\/reanalyze$/);
  expect(await retry.evaluate((button) => getComputedStyle(button).backgroundColor)).not.toBe(
    await candidate.locator('.confirmation button').first().evaluate((button) => getComputedStyle(button).backgroundColor),
  );
  await expect(page.getByRole('heading', { name: 'Not assessed' })).toBeVisible();
  await expect(page.locator('ul.reading-chooser-list').first()).toHaveAttribute('role', 'list');
  const noOverflow = () => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);
  expect(await noOverflow()).toBe(true);
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  expect(await noOverflow()).toBe(true);
  await expect(identity.locator('summary').filter({ hasText: 'Switch to this book' })).toBeVisible();
  await expect(retry).toBeVisible();
});

import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

async function expectPostFormsCarryCSRF(page: Page) {
  const forms = page.locator('form[method="post"]');
  expect(await forms.count()).toBeGreaterThan(0);
  for (let i = 0; i < await forms.count(); i += 1) {
    await expect(forms.nth(i).locator('input[name="csrf_token"]')).toHaveCount(1);
  }
}

test.describe('migration and epistemic regression coverage', () => {
  test.beforeEach(async ({ page }) => signIn(page));

  test('shows migrated My Books states and server-rendered controls', async ({ page }) => {
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') await switcher.selectOption('de');
    await expect(page.locator('#library-page-title')).toHaveText('My Books in German');
    await expect(page.locator('.library-list').getByText('Metadata-only migration book')).toHaveCount(0);
    await expect(page.getByText('Analysis result ready').first()).toBeVisible();
    await expect(page.getByText('language not chosen')).toHaveCount(0);
    await expectPostFormsCarryCSRF(page);
  });

  test('shows current versus conditional Journey evidence without changing learner order', async ({ page }) => {
    await page.goto('/journey');
    await expect(page.locator('#primary-goal-heading')).toHaveText('Primary Goal');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('Provisional Journey');
    await expect(page.getByRole('heading', { name: 'Campaign history & operations' })).toHaveCount(0);
    await expect(page.getByRole('heading', { name: /compare your order with a vocabulary-efficient alternative/i })).toBeVisible();
    // The advisory comparison is inside a collapsed disclosure; open it before
    // asserting its current and conditional projected evidence.
    await page.getByText('Show vocabulary-efficient alternative (optional comparison)').click();
    await expect(page.getByText(/current known-token coverage/i).first()).toBeVisible();
    await expect(page.getByText(/separate conditional projected variant/i)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Your order (canonical)', exact: true })).toBeVisible();
    const tableRegion = page.locator('.table-region[aria-label="Current advisory order table"]');
    await expect(tableRegion).toBeVisible();
    await tableRegion.focus();
    await expect(tableRegion).toBeFocused();
    await expect(page.locator('#provisional-journey-status')).toHaveAttribute('aria-live', 'polite');
    await expectPostFormsCarryCSRF(page);
    await page.goto('/journey/fixture-book');
    await expect(page.getByRole('heading', { name: "This Book's vocabulary study" })).toBeVisible();
  });

  test('separates reading achievement from graduation and preserves provenance labels', async ({ page }) => {
    await page.goto('/journey');
    const goal = page.locator('#primary-goal-section');
    const disclosure = goal.locator('details').filter({ hasText: 'Mark reading finished' }).first();
    await expect(disclosure).toBeVisible();
    await disclosure.locator('summary').focus();
    await expect(disclosure.locator('summary')).toBeFocused();
    await disclosure.locator('summary').press('Enter');
    await expect(goal).toContainText('Record the reading achievement');
    await expect(goal).toContainText('Vocabulary is changed only when the associated prepared deck has been reviewed');
    await expect(goal.locator('form[action="/goal/finish"] input[name="csrf_token"]')).toHaveCount(1);
    await expect(goal.locator('form[action="/goal/finish"] input[name="expected_goal_book_id"]')).toHaveCount(1);

    await page.goto('/vocabulary');
    await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Explicitly recorded' }).first()).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Graduated from completed campaign' }).first()).toBeVisible();
    await expect(page.locator('form.vocabulary-language-picker')).toHaveCount(0);
    await expect(page.getByText(/Viewing German/)).toBeVisible();
    await expectPostFormsCarryCSRF(page);
  });
});

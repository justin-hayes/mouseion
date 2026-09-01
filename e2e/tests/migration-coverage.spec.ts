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
    await expect(page.getByRole('heading', { name: 'My Books', exact: true })).toBeVisible();
    await expect(page.getByText('Metadata-only migration book')).toBeVisible();
    await expect(page.locator('article.library-book').filter({ hasText: 'Metadata-only migration book' })).toContainText('Not acquired');
    await expect(page.getByText('Analysis result ready').first()).toBeVisible();
    await expect(page.getByText('language not chosen').first()).toBeVisible();
    await expectPostFormsCarryCSRF(page);
  });

  test('shows current versus conditional Journey evidence without changing learner order', async ({ page }) => {
    await page.goto('/journey');
    await expect(page.locator('#primary-goal-heading')).toHaveText('Primary Goal');
    await expect(page.locator('#provisional-journey-heading')).toHaveText('Provisional Journey');
    await expect(page.locator('#campaign-operations-heading')).toContainText('Campaign history & operations');
    await expect(page.getByRole('heading', { name: /compare your order with a vocabulary-efficient alternative/i })).toBeVisible();
    // The advisory comparison is inside a collapsed disclosure; open it before
    // asserting its current and conditional projected evidence.
    await page.getByText('Show vocabulary-efficient alternative (optional comparison)').click();
    await expect(page.getByText(/current known-token coverage/i).first()).toBeVisible();
    await expect(page.getByText(/separate conditional projected variant/i)).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Your order (canonical)', exact: true })).toBeVisible();
    await expect(page.locator('#campaign-fixture-completed-campaign')).toBeVisible();
    await expect(page.locator('#campaign-fixture-abandoned-campaign')).toBeVisible();

    const tableRegion = page.locator('.table-region[aria-label="Current advisory order table"]');
    await expect(tableRegion).toBeVisible();
    await tableRegion.focus();
    await expect(tableRegion).toBeFocused();
    await expect(page.locator('#provisional-journey-status')).toHaveAttribute('aria-live', 'polite');
    await expectPostFormsCarryCSRF(page);
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

    await page.goto('/settings?language=de');
    await expect(page.locator('#known-vocabulary-heading')).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Explicitly recorded' }).first()).toBeVisible();
    await expect(page.getByRole('cell', { name: 'Graduated from completed campaign' }).first()).toBeVisible();
    const languagePicker = page.locator('form.settings-language-picker select[name="language"]');
    await expect(languagePicker).toBeVisible();
    // Options inside a collapsed select are not "visible"; assert their content,
    // matching the catalog browse pattern in smoke.spec.ts.
    await expect(languagePicker.locator('option[value="de"]')).toContainText('German');
    await expect(languagePicker.locator('option[value="it"]')).toContainText('Italian');
    await expectPostFormsCarryCSRF(page);
  });
});

import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

// Keep the cross-project browser assertion read-only because the fixture server
// is shared across projects. One desktop-light journey below exercises the
// complete finish/history/reread loop; isolated Go tests cover idempotency.
test('Current reading exposes a native reading-finish confirmation without JavaScript', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  await signIn(page);
  await page.goto('/reading');
  await expect(page.locator('body')).toHaveClass('reading-shell');
  await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);

  const goal = page.locator('#primary-goal-section');
  const finishDisclosure = goal.locator('details').filter({ hasText: 'Mark reading finished' }).first();
  await expect(finishDisclosure).toHaveCount(1);

  const finishForm = finishDisclosure.locator('form[action="/reading/finish"]');
  await finishDisclosure.locator('summary').click();
  await expect(finishForm.getByRole('button', { name: 'Mark reading finished' })).toBeVisible();
  await expect(finishForm.getByRole('button', { name: 'Mark reading finished' })).toHaveClass(/\bbutton\b/);
  await expect(goal).toContainText('Record the reading achievement');
  await expect(finishForm.locator('input[name="csrf_token"]')).toHaveCount(1);
  await expect(finishForm.locator('input[name="expected_current_book_id"]')).toHaveCount(1);
  await expect(finishForm.locator('input[name="expected_current_snapshot_id"]')).toHaveCount(1);

  for (const [label, button] of [
    ['Stop reading for now', 'Confirm stop for now'],
    ['Set aside this Book', 'Confirm set aside'],
  ]) {
    const disclosure = goal.locator('details').filter({ hasText: label }).first();
    await disclosure.locator('summary').click();
    await expect(disclosure.getByRole('button', { name: button })).toBeVisible();
  }

  await page.goto('/reading/switch');
  const switchCandidate = page.locator('.reading-chooser-book').filter({ hasText: 'Switch to this book' }).first();
  const switchDisclosure = switchCandidate.locator('details').filter({ hasText: 'Switch to this book' }).first();
  await switchDisclosure.locator('summary').click();
  await expect(switchDisclosure.getByRole('button', { name: /Confirm switch to this book/ })).toBeVisible();
  await context.close();
});

test('finishing current reading records Read history and supports reading again', async ({ page }) => {
  test.skip(test.info().project.name !== 'desktop-light', 'The fixture server is shared across browser projects.');
  await signIn(page);
  await page.getByLabel('Study language').selectOption('it');
  await expect(page).toHaveURL(/\/library$/);
  await page.goto('/reading');
  await expect(page.locator('body')).toHaveClass('reading-shell');
  await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);

  const currentReading = page.locator('#primary-goal-section');
  if (await currentReading.count() === 0) {
    const title = 'Italian route baseline';
    const candidate = page.locator('article').filter({ has: page.getByRole('heading', { name: title, exact: true }) });
    await expect(page.getByRole('region', { name: 'Below 95%' })).toContainText(title);
    await candidate.locator('details').filter({ hasText: 'Start reading' }).locator('summary').click();
    await candidate.getByRole('button', { name: 'Confirm start reading' }).click();
    await expect(page).toHaveURL(/\/reading\?message=/);
  }
  const title = (await currentReading.getByRole('heading', { level: 1 }).locator('a').textContent())?.trim() ?? '';
  expect(title).toBeTruthy();
  const finishDisclosure = currentReading.locator('details').filter({ hasText: 'Mark reading finished' }).first();
  await finishDisclosure.locator('summary').click();
  await expect(finishDisclosure).toContainText('Record the reading achievement');
  await finishDisclosure.getByRole('button', { name: 'Mark reading finished' }).click();

  await expect(page.getByRole('heading', { name: /Reading finished/i })).toBeVisible();
  // HTMX swaps only the receipt fragment; the host page keeps the route-owned
  // stylesheets that style both Mouseion content and the compiled controls.
  await expect(page.locator('body')).toHaveClass('reading-shell');
  await expect(page.locator('link[rel="stylesheet"][href="/static/app.css"]')).toHaveCount(1);
  await expect(page.locator('link[rel="stylesheet"][href*="pico-"]')).toHaveCount(0);
  await expect(page.locator('#primary-goal-section')).toHaveClass(/journey-finish-outcome/);
  await expect(page.locator('#primary-goal-section')).toContainText('Vocabulary transition');
  await expect(page.locator('#primary-goal-finish-heading')).toBeFocused();
  await expect(page.getByRole('link', { name: 'Choose a To Read book' })).toHaveAttribute('href', '/reading');
  await page.goto('/library?history=read');
  const history = page.locator('.library-books .library-book').filter({ hasText: title });
  await expect(history).toBeVisible();
  await expect(history).toContainText('Read');
  await history.getByText('More actions', { exact: true }).click();
  await expect(history.getByRole('button', { name: 'Read again' })).toBeVisible();
  await history.getByRole('button', { name: 'Read again' }).click();

  await expect(page).toHaveURL(/\/library\?disposition=to_read/);
  const reread = page.locator('.library-books .library-book').filter({ hasText: title });
  await expect(reread).toContainText('To Read');
  await expect(reread.getByRole('link', { name: 'View in Reading' })).toBeVisible();

  await reread.getByText('More actions', { exact: true }).click();
  await reread.getByText('Set aside', { exact: true }).click();
  await reread.getByRole('button', { name: 'Confirm set aside' }).click();
  await expect(page).toHaveURL(/\/library\?history=read&message=/);
  const readAgainLater = page.locator('.library-books .library-book').filter({ hasText: title });
  await expect(readAgainLater).toContainText('Disposition');
  await expect(readAgainLater).toContainText('Read');
});

import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
}

async function showStudyLanguage(page: Page, language: string) {
  const switcher = page.getByLabel('Study language');
  if (await switcher.inputValue() !== language) await switcher.selectOption(language);
}

test.describe('Remove from Reading', () => {
  // This spec has its own fixture server, so the Add/Remove state it creates
  // never leaks into another spec file.
  test('removes an OPDS Book and restores it from the Catalog', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs?language=de');

    const entry = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Ein kurzer Sommer' });
    await entry.getByRole('button', { name: 'Add to Reading', exact: true }).click();
    await expect(entry.getByRole('link', { name: 'In Reading', exact: true })).toBeVisible();

    await page.goto('/de/reading');
    const row = page.locator('#book-row-fixture-catalogue-restore');
    await expect(row).toBeVisible();
    await row.getByText('More actions', { exact: true }).click();
    await row.getByRole('button', { name: 'Remove from Reading', exact: true }).click();
    await expect(page).toHaveURL(/\/reading\?.*message=/);
    await expect(page.getByText(/Removed from Reading\. Its analysis and history are unchanged/)).toBeVisible();
    await expect(row).toHaveCount(0);

    // The Catalog offers Add again, and Add restores the same Book.
    await page.goto('/catalogs?language=de');
    const addedAgain = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Ein kurzer Sommer' });
    await expect(addedAgain.getByRole('button', { name: 'Add to Reading', exact: true })).toBeVisible();
    await addedAgain.getByRole('button', { name: 'Add to Reading', exact: true }).click();
    await expect(addedAgain.getByRole('link', { name: 'In Reading', exact: true })).toBeVisible();
    await page.goto('/de/reading');
    await expect(page.locator('#book-row-fixture-catalogue-restore')).toBeVisible();
  });

  test('refuses Remove on the current reading with a reason', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');

    const current = page.locator('#book-row-fixture-book');
    await expect(current.locator('.library-book__membership')).toContainText('Currently reading');
    await current.getByText('More actions', { exact: true }).click();
    await expect(current.getByRole('button', { name: 'Remove from Reading', exact: true })).toBeDisabled();
    await expect(current.getByText('Your current reading cannot be removed. Finish or end the reading first.')).toBeVisible();
  });

  test('removes a withdrawn Book without offering to restore it', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/reading');
    await showStudyLanguage(page, 'grc');
    const first = page.locator('#book-row-fixture-classical-fixture-iliad-1-fixture-learner');
    const second = page.locator('#book-row-fixture-classical-fixture-iliad-2-fixture-learner');
    await expect(first).toBeVisible();
    await expect(second).toBeVisible();
    await expect(page.getByRole('button', { name: 'Remove the work', exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Move the work to To Read', exact: true })).toHaveCount(0);

    await first.getByText('More actions', { exact: true }).click();
    await first.getByRole('button', { name: 'Remove from Reading', exact: true }).click();
    await expect(page).toHaveURL(/\/reading\?.*message=/);
    await expect(page.getByText(/This Reading unit is no longer in the Catalog, so it can't be added back\./)).toBeVisible();
    await expect(first).toHaveCount(0);
    await expect(second).toBeVisible();

    // Owning a leftover Book does not block adding the whole Work.
    await page.goto('/catalogs/classical/works/fixture-iliad');
    const contents = page.getByRole('region', { name: /Contents of Iliad/ }).getByRole('table');
    await expect(contents.getByRole('row')).toHaveCount(4);
    await expect(contents.getByRole('button')).toHaveCount(0);
    const add = page.getByRole('button', { name: 'Add to Reading', exact: true });
    await expect(add).toHaveCount(1);
    await expect(page.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(1);
    await add.click();
    await expect(page).toHaveURL(/\/catalogs\/classical\/works\/fixture-iliad\?/);
    await page.goto('/de/reading');
    await showStudyLanguage(page, 'grc');
    await expect(first).toHaveCount(0);
    await expect(second).toBeVisible();
    await expect(page.locator('#book-row-fixture-classical-fixture-iliad-fixture-learner')).toBeVisible();
  });
});

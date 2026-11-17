import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('Classical catalog fixture journey', () => {
  test('adds a whole work and reaches My Books and Reading', async ({ page }) => {
    await signIn(page);

    await page.goto('/catalogs');
    await expect(page.getByRole('heading', { name: 'Classical catalog', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Browse the Classical catalog', exact: true }).click();
    await expect(page).toHaveURL(/\/catalogs\/classical$/);

    await expect(page.getByRole('link', { name: 'Homer', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Xenophon', exact: true })).toBeVisible();

    // The fixture release carries one Work split into constituent books and
    // one whole-work unit under another author.
    await page.getByRole('link', { name: 'Xenophon', exact: true }).click();
    const wholeWorkRecord = page.getByRole('article', { name: 'Hellenica' });
    await expect(wholeWorkRecord).toContainText('passage');
    await expect(wholeWorkRecord.getByRole('button', { name: 'Add the work' })).toBeVisible();
    await page.goto('/catalogs/classical');

    await page.getByRole('link', { name: 'Homer', exact: true }).click();
    await expect(page).toHaveURL(/\/catalogs\/classical\/authors\/Homer$/);
    const iliadRecord = page.getByRole('article', { name: 'Iliad' });
    await expect(iliadRecord).toContainText('3 books');

    await iliadRecord.getByRole('link', { name: 'Iliad', exact: true }).click();
    await expect(page).toHaveURL(/\/catalogs\/classical\/works\/fixture-iliad$/);
    await expect(page.getByRole('heading', { name: 'Iliad', level: 1 })).toBeVisible();
    for (const book of ['Book 1', 'Book 2', 'Book 3']) {
      await expect(page.getByRole('cell', { name: book, exact: true })).toBeVisible();
    }

    await page.getByRole('button', { name: 'Add the whole work', exact: true }).click();
    await expect(page.getByText(/Added 3 Books to My Books/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'In My Books', exact: true }).first()).toBeVisible();
    await page.getByRole('link', { name: 'In My Books', exact: true }).first().click();

    await expect(page).toHaveURL(/\/library\?disposition=inbox/);
    const workRow = page.locator('#work-row-fixture-iliad');
    await expect(workRow).toBeVisible();
    await expect(workRow.getByRole('heading', { name: 'Iliad', exact: true })).toBeVisible();
    await expect(workRow.getByRole('list', { name: 'Books of Iliad' }).locator('.library-work__cell-number'))
      .toHaveText(['1', '2', '3']);

    await workRow.getByRole('button', { name: 'Move the work to To Read', exact: true }).click();
    await expect(page).toHaveURL(/\/library\?disposition=inbox&message=/);

    await page.goto('/reading');
    await expect(page.getByRole('heading', { name: 'Choose a To Read book in Ancient Greek', exact: true })).toBeVisible();
    const workEntry = page.locator('.reading-chooser-work');
    await expect(workEntry).toHaveCount(1);
    await expect(workEntry.getByRole('heading', { name: 'Iliad', exact: true })).toBeVisible();
    await expect(workEntry.locator('.reading-chooser-bands__row')).toHaveCount(3);
    await expect(workEntry.locator('.reading-chooser-bands dt'))
      .toHaveText(['99% or above', '97% to below 99%', 'Below 95%']);
    await expect(workEntry.locator('.reading-chooser-band-book')).toHaveText(['1', '2', '3']);

    const bandCell = workEntry.locator('.reading-chooser-band-book').first();
    const entryAnchor = await bandCell.getAttribute('href');
    expect(entryAnchor).toMatch(/^#reading-chooser-book-/);
    await bandCell.click();
    await expect(page).toHaveURL(url => url.hash === entryAnchor);

    const candidate = page.locator(entryAnchor!);
    await expect(candidate).toBeVisible();
    await expect(candidate.getByRole('heading', { name: 'Iliad, Book 1', exact: true })).toBeVisible();
    await expect(candidate.getByText('Start reading', { exact: true })).toBeVisible();
  });
});

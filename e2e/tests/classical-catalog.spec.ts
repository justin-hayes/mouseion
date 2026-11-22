import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test('adds a single book and a whole work from the Classical catalog index', async ({ page }) => {
  await signIn(page);

  await page.goto('/catalogs');
  await page.getByRole('link', { name: 'Classical catalog', exact: true }).click();
  await expect(page).toHaveURL(/\/catalogs\/classical$/);

  await expect(page.getByRole('heading', { name: 'Homer', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Xenophon', exact: true })).toBeVisible();
  await expect(page.getByText(/Adding a Book puts it in To Read and downloads and prepares the text/)).toBeVisible();

  // An old author URL lands on that author's section of the index.
  await page.goto('/catalogs/classical/authors/Homer');
  await expect(page).toHaveURL(/\/catalogs\/classical#author-Homer$/);
  await expect(page.getByRole('heading', { name: 'Homer', exact: true })).toBeVisible();

  const iliad = page.locator('#work-fixture-iliad');
  await expect(iliad).toContainText('μῆνιν ἄειδε θεὰ Πηληϊάδεω Ἀχιλῆος');
  await expect(iliad).toContainText('600 words in 3 books');
  await expect(iliad).toContainText('Fixture reviewed edition');
  await expect(iliad.getByRole('button', { name: 'Add and prepare the whole work', exact: true })).toBeVisible();

  // Add only Book 1 from the native "Choose books" disclosure.
  await iliad.getByText('Choose books', { exact: true }).click();
  const bookOne = iliad.getByRole('row', { name: /Book 1/ });
  await expect(bookOne).toContainText('1.1.1–1.10.19');
  await bookOne.getByRole('button', { name: 'Add and prepare', exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\/classical\?/);
  await expect(page.getByText(/Added 1 Book to My Books/)).toBeVisible();
  await expect(page.getByText(/The text is already prepared/)).toBeVisible();
  await expect(iliad).toContainText('1 of 3 books in My Books');
  await expect(iliad.getByRole('button', { name: 'Add and prepare the remaining books', exact: true })).toBeVisible();

  await iliad.getByText('Choose books', { exact: true }).click();
  await expect(iliad.getByRole('row', { name: /Book 1/ }).getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
  await expect(iliad.getByRole('row', { name: /Book 2/ }).getByRole('button', { name: 'Add and prepare', exact: true })).toBeVisible();

  // Add the whole-work unit from the margin note.
  const hellenica = page.locator('#work-fixture-hellenica');
  await expect(hellenica).toContainText('500 words, read whole');
  await hellenica.getByRole('button', { name: 'Add and prepare the work', exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\/classical\?/);
  await expect(page.getByText(/Added 1 Book to My Books/)).toBeVisible();
  await expect(page.getByText(/The text is already prepared/)).toBeVisible();
  await expect(hellenica.getByText('In My Books', { exact: true })).toBeVisible();
  await expect(hellenica.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(0);

  // The same index fits a phone without sideways scrolling, and its controls
  // stay keyboard-operable and large enough to tap.
  await page.setViewportSize({ width: 375, height: 667 });
  await expect(page.getByRole('heading', { name: 'Classical catalog', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

  const openingBox = await iliad.locator('.classical-opening').boundingBox();
  expect(openingBox?.width ?? 0).toBeLessThanOrEqual(375);

  const addBox = await iliad.locator('.classical-work-entry__margin button').first().boundingBox();
  expect(addBox?.height ?? 0).toBeGreaterThanOrEqual(44);

  const summary = iliad.getByText('Choose books', { exact: true });
  await summary.focus();
  await page.keyboard.press('Enter');
  await expect(iliad.locator('.classical-books__table')).toBeVisible();

  const tableBox = await iliad.locator('.classical-books__table').boundingBox();
  expect(tableBox?.width ?? 0).toBeLessThanOrEqual(375);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});

test("I've read this records one classical Reading unit as Read and never a whole Work", async ({ page }) => {
  await signIn(page);
  await page.goto('/catalogs/classical');

  const iliad = page.locator('#work-fixture-iliad');
  await expect(iliad.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(0);
  await iliad.getByText('Choose books', { exact: true }).click();

  const bookThree = iliad.getByRole('row', { name: /Book 3/ });
  await expect(bookThree.getByRole('button', { name: "I've read this", exact: true })).toBeVisible();
  await bookThree.getByRole('button', { name: "I've read this", exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\/classical\?/);
  await expect(page.getByText(/Recorded “Iliad” as previously read/)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Ancient Greek in My Books', exact: true })).toBeVisible();

  await iliad.getByText('Choose books', { exact: true }).click();
  await expect(iliad.getByRole('row', { name: /Book 3/ }).getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
  await expect(iliad.locator('.classical-work-entry__margin').getByRole('button', { name: "I've read this", exact: true })).toHaveCount(0);

  await page.getByRole('link', { name: 'Open Ancient Greek in My Books', exact: true }).click();
  await expect(page).toHaveURL(/\/library/);
  await expect(page.getByRole('heading', { name: /Iliad, Book 3/ })).toBeVisible();
  await expect(page.getByText('Read before Mouseion')).toBeVisible();
});

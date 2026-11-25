import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test('the Classical catalog is an ordinary card section; Add leaves the Catalog and the Work page keeps its contents', async ({ page }) => {
  await signIn(page);

  // There is no separate Classical catalog button; Ancient Greek is a Language
  // filter choice.
  await page.goto('/catalogs');
  await expect(page.getByRole('link', { name: 'Classical catalog', exact: true })).toHaveCount(0);
  await page.getByLabel('Language', { exact: true }).selectOption('grc');
  await page.getByRole('button', { name: 'Search' }).click();
  await expect(page).toHaveURL(/\/catalogs\?.*language=grc/);

  const section = page.locator('#catalog-source-classical');
  await expect(section.getByRole('heading', { name: 'Classical catalog', exact: true })).toBeVisible();
  await expect(section.locator('.catalog-entry', { hasText: 'Homer' }).first()).toBeVisible();
  await expect(section.locator('.catalog-entry', { hasText: 'Xenophon' }).first()).toBeVisible();
  await expect(section.locator('.catalog-source__footer')).toContainText('Greek text source:');

  // The retired index and author URLs land on the Catalog filtered to Greek.
  for (const retired of ['/catalogs/classical', '/catalogs/classical/authors/Homer']) {
    await page.goto(retired);
    await expect(page).toHaveURL(/\/catalogs\?language=grc$/);
    await expect(page.locator('#catalog-source-classical')).toBeVisible();
  }

  // The Work page has one Add and one I've read this per Edition, the Opening,
  // the length strip and a read-only contents list.
  await page.goto('/catalogs/classical/works/fixture-odyssey');
  await expect(page.getByRole('button', { name: 'Add', exact: true })).toHaveCount(1);
  await expect(page.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(1);
  await expect(page.getByText('Choose books')).toHaveCount(0);
  await expect(page.locator('.classical-opening')).toContainText('ἄνδρα μοι ἔννεπε, μοῦσα, πολύτροπον');
  await expect(page.locator('.classical-length-caption')).toContainText('600 words in 3 books');
  await expect(page.locator('.classical-length')).toHaveCount(1);
  const contents = page.getByRole('region', { name: 'Contents of Odyssey, Fixture reviewed edition' });
  await expect(contents.getByRole('row')).toHaveCount(4);
  await expect(contents.getByRole('row', { name: /Book 2/ })).toContainText('2.1.1–2.10.19');
  await expect(contents.getByRole('row', { name: /Book 2/ })).toContainText('200');
  await expect(contents.getByRole('button')).toHaveCount(0);
  await expect(contents.getByRole('link')).toHaveCount(0);
  await page.getByRole('navigation', { name: 'Breadcrumb' }).getByRole('link').click();
  await expect(page).toHaveURL(/\/catalogs\?language=grc$/);

  // A Work is one Book: its card has one Add, and Add confirms in place.
  const odyssey = page.locator('#catalog-source-classical .catalog-entry', { hasText: 'Odyssey' });
  await expect(odyssey).toContainText('Ὀδύσσεια');
  await expect(odyssey).toContainText('Fixture reviewed edition');
  await expect(odyssey).toContainText('Adding prepares the text for reading.');
  await expect(odyssey.getByRole('button', { name: 'Add', exact: true })).toHaveCount(1);
  await odyssey.getByRole('button', { name: 'Add', exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\?language=grc&/);
  await expect(page.locator('#catalog-add-confirmation')).toContainText('Added 1 Book to My Books');
  await expect(page.locator('#catalog-add-confirmation')).toContainText('The text is already prepared');
  await expect(page.locator('#catalog-add-confirmation').getByRole('link', { name: 'View in My Books' })).toBeVisible();
  await expect(odyssey.getByText('In My Books', { exact: true })).toBeVisible();
  await expect(odyssey.getByRole('button', { name: 'Add', exact: true })).toHaveCount(0);

  // An owned Work page keeps its contents list and offers no Add.
  await page.goto('/catalogs/classical/works/fixture-odyssey');
  await expect(page.getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Contents of Odyssey, Fixture reviewed edition' }).getByRole('row')).toHaveCount(4);

  // The Classical section fits a phone without sideways scrolling.
  await page.goto('/catalogs?language=grc');
  await page.setViewportSize({ width: 375, height: 667 });
  await expect(page.locator('#catalog-source-classical')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});

test("I've read this works from a Classical card and confirms like an OPDS card", async ({ page }) => {
  await signIn(page);
  await page.goto('/catalogs?language=grc');
  const iliad = page.locator('#catalog-source-classical .catalog-entry', { hasText: 'Iliad' });
  await iliad.getByRole('button', { name: "I've read this", exact: true }).click();
  await expect(page.locator('#catalog-read-confirmation')).toContainText(/Recorded “Iliad” as previously read/);
  await expect(page.locator('#catalog-read-confirmation').getByRole('link', { name: 'View in My Books' })).toBeVisible();
});

test("I've read this records the whole Work as Read from its Work page", async ({ page }) => {
  await signIn(page);
  await page.goto('/catalogs/classical/works/fixture-hellenica');
  await expect(page.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(1);
  await page.getByRole('button', { name: "I've read this", exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\/classical\/works\/fixture-hellenica\?/);
  await expect(page.getByText(/Recorded “Hellenica” as previously read/)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Open Ancient Greek in My Books', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'In My Books', exact: true }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Add', exact: true })).toHaveCount(0);

  await page.getByRole('link', { name: 'Open Ancient Greek in My Books', exact: true }).click();
  await expect(page).toHaveURL(/\/library/);
  await expect(page.getByRole('heading', { name: /Hellenica/ })).toBeVisible();
  await expect(page.locator('#book-row-fixture-classical-fixture-hellenica-fixture-learner').getByText('Read before Mouseion')).toBeVisible();
});

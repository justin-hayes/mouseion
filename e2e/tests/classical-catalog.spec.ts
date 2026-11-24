import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test('adds a whole work with constituent books as one Book and shows its contents on the Work page', async ({ page }) => {
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

  // The Work page has one Add and one I've read this per Edition and a
  // read-only contents list.
  await page.goto('/catalogs/classical/works/fixture-odyssey');
  await expect(page.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(1);
  await expect(page.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(1);
  await expect(page.getByText('Choose books')).toHaveCount(0);
  const contents = page.getByRole('region', { name: 'Contents of Odyssey' });
  await expect(contents.getByRole('row')).toHaveCount(4);
  await expect(contents.getByRole('row', { name: /Book 2/ })).toContainText('2.1.1–2.10.19');
  await expect(contents.getByRole('row', { name: /Book 2/ })).toContainText('200');
  await expect(contents.getByRole('button')).toHaveCount(0);
  await expect(contents.getByRole('link')).toHaveCount(0);

  // A Work is one Book: the index has one Add and no per-book controls.
  await page.goto('/catalogs/classical');
  const odyssey = page.locator('#work-fixture-odyssey');
  await expect(odyssey).toContainText('ἄνδρα μοι ἔννεπε, μοῦσα, πολύτροπον');
  await expect(odyssey).toContainText('600 words in 3 books');
  await expect(odyssey).toContainText('Fixture reviewed edition');
  await expect(odyssey.getByText('Choose books')).toHaveCount(0);
  await expect(odyssey.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(1);
  await odyssey.getByRole('button', { name: 'Add and prepare the work', exact: true }).click();

  await expect(page).toHaveURL(/\/catalogs\/classical\?/);
  await expect(page.getByText(/Added 1 Book to My Books/)).toBeVisible();
  await expect(page.getByText(/The text is already prepared/)).toBeVisible();
  await expect(odyssey.getByText('In My Books', { exact: true })).toBeVisible();
  await expect(odyssey.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(0);

  // An owned Work page keeps its contents list and offers no Add.
  await page.goto('/catalogs/classical/works/fixture-odyssey');
  await expect(page.getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Contents of Odyssey' }).getByRole('row')).toHaveCount(4);

  // The same index fits a phone without sideways scrolling.
  await page.goto('/catalogs/classical');
  await page.setViewportSize({ width: 375, height: 667 });
  await expect(page.getByRole('heading', { name: 'Classical catalog', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  const openingBox = await odyssey.locator('.classical-opening').boundingBox();
  expect(openingBox?.width ?? 0).toBeLessThanOrEqual(375);
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
  await expect(page.getByRole('button', { name: 'Add and prepare the work', exact: true })).toHaveCount(0);

  await page.getByRole('link', { name: 'Open Ancient Greek in My Books', exact: true }).click();
  await expect(page).toHaveURL(/\/library/);
  await expect(page.getByRole('heading', { name: /Hellenica/ })).toBeVisible();
  await expect(page.getByText('Read before Mouseion')).toBeVisible();
});

import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
  await page.getByLabel('Study language').selectOption('de');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page).toHaveURL(/\/library$/);
}

// The acceptance viewports are 375px and 1200px in both themes; the project
// supplies the theme and the compact or desktop baseline.
async function acceptanceViewport(page: Page) {
  const viewport = page.viewportSize()!;
  const width = viewport.width <= 600 ? 375 : 1200;
  await page.setViewportSize({ width, height: 800 });
}

async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
}

// The fixture's Greek native_agdt sentence is reached from a fixture
// Concordance row; its words are λέγει (root), ὅτι (the identified target),
// ἄνδρας (headed by an unprinted node) and καλούς (headed by a comma).
async function openSyntaxSentence(page: Page) {
  await page.goto('/de/corpus/concordance?term=fixture-syntax');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await row.locator('summary').click();
  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page).toHaveURL(/\/corpus\/concordance\/sentence/);
}

async function expectWordRelations(page: Page, open: () => Promise<void>) {
  const relations = page.locator('#sentence-study-word-relations');
  await expect(relations).not.toHaveAttribute('open', '');
  await open();
  await expect(relations).toHaveAttribute('open', '');
  await expect(page.getByText('Syntax unavailable')).toHaveCount(0);

  const rows = relations.locator('tbody tr');
  await expect(rows).toHaveCount(4);
  await expect(rows.nth(0)).toHaveText(/λέγει\s*main verb\s*— \(top of the sentence\)/);
  await expect(rows.nth(1)).toHaveText(/ὅτι \(identified target\)\s*subordinating conjunction\s*λέγει \(word 1\)/);
  await expect(rows.nth(1).locator('strong')).toBeVisible();
  await expect(rows.nth(2)).toHaveText(/ἄνδρας\s*subject\s*‹an unprinted word›, which is the object of ὅτι \(word 2\)/);
  await expect(rows.nth(3)).toHaveText(/καλούς\s*describes a noun\s*ἄνδρας \(word 3\)/);
  await expect(relations.getByRole('cell', { name: ',', exact: true })).toHaveCount(0);
  await expect(relations.locator('#sentence-study-unprinted-note')).toHaveCount(1);
  await expectNoHorizontalOverflow(page);
}

test('Word relations opens from the keyboard and lists each word with its head', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);

  await expectWordRelations(page, async () => {
    const summary = page.locator('#sentence-study-word-relations > summary');
    await summary.focus();
    await expect(summary).toBeFocused();
    await page.keyboard.press('Enter');
  });
});

test('Word relations works without JavaScript', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await acceptanceViewport(page);
    await signIn(page);
    await openSyntaxSentence(page);
    await expectWordRelations(page, () => page.locator('#sentence-study-word-relations > summary').click());
  } finally {
    await context.close();
  }
});

test('Sentence study without native syntax shows no Word relations', async ({ page }) => {
  await signIn(page);
  await page.goto('/de/corpus/concordance?term=Haus');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await row.locator('summary').click();
  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page.locator('#sentence-study-word-relations')).toHaveCount(0);
});

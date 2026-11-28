import { expect, Locator, Page, test } from '../support/test';
import { acceptanceViewport, expectNoHorizontalOverflow, underlined, weight } from '../support/construction-probes';

// This spec has its own fixture server. The Greek fixture Book comes from adding
// the fixture classical Work, which also makes Ancient Greek a study language.
async function signInToGreek(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
  await page.goto('/catalogs/classical/works/fixture-iliad');
  const add = page.getByRole('button', { name: 'Add to Reading', exact: true });
  if (await add.isVisible().catch(() => false)) await add.click();
  await page.goto('/de/reading');
  await page.getByLabel('Study language').selectOption('grc');
  const noScriptSwitch = page.getByRole('button', { name: 'Switch language' });
  if (await noScriptSwitch.isVisible().catch(() => false)) await noScriptSwitch.click();
  await expect(page.getByLabel('Study language')).toHaveValue('grc');
}

// Activating with the keyboard (focus, then Enter) rather than the pointer.
async function activate(page: Page, locator: Locator) {
  await locator.focus();
  await expect(locator).toBeFocused();
  await page.keyboard.press('Enter');
}

async function openSyntaxSentence(page: Page) {
  await page.goto('/de/corpus/concordance?term=fixture-syntax');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await row.locator('summary').click();
  await activate(page, row.getByRole('link', { name: 'Study this sentence' }));
  await expect(page).toHaveURL(/\/corpus\/concordance\/sentence/);
}

// Parallels → the tiered page → expand a row → Study → the focused entry.
async function walkParallels(page: Page) {
  await openSyntaxSentence(page);
  const entries = page.locator('#sentence-study-constructions ol > li');
  await expect(entries).toHaveCount(3);
  await expect(entries.nth(1).getByRole('link', { name: 'Parallels for Genitive absolute', exact: true })).toBeVisible();
  await expect(entries.nth(0).getByRole('link', { name: 'Parallels for Indirect statement, infinitive', exact: true })).toBeVisible();
  await expect(page.locator('.sentence-study-construction--focused')).toHaveCount(0);

  await test.step('Parallels opens the three tiers in order', async () => {
    await activate(page, entries.nth(1).getByRole('link', { name: 'Parallels for Genitive absolute', exact: true }));
    await expect(page).toHaveURL(/as=construction/);
    await expect(page.getByRole('heading', { level: 2, name: 'Parallels of Genitive absolute' })).toBeVisible();
    await expect(page.locator('h3.concordance-tier')).toHaveText(['This Book', 'This author', 'Other authors']);
    await expect(page.locator('#concordance-native-results .concordance-result')).toHaveCount(6);
    await expect(page.locator('#concordance-parallels-widen')).toHaveCount(0);
    await expect(page.locator('#concordance-term')).toHaveValue('');
    await expectNoHorizontalOverflow(page);
  });

  await test.step('the Books that could not be searched are named once, in one quiet line', async () => {
    const line = page.locator('#concordance-unsearched');
    await expect(line).toHaveCount(1);
    await expect(line).toHaveText('Not searched for constructions: Synthetic Apology — Analysis update available, Synthetic Crito');
    await expect(line.locator('em')).toHaveText(['Synthetic Apology', 'Synthetic Crito']);
    await expectNoHorizontalOverflow(page);
  });

  await test.step('Only with narrows to the anchor lemma, and All clears the filter', async () => {
    const results = page.locator('#concordance-native-results .concordance-result');
    const only = page.getByRole('link', { name: 'Only with ἄνδρας: parallels signalled by this lemma', exact: true });
    await expect(only).toHaveText('Only with ἄνδρας');
    await activate(page, only);
    await expect(page).toHaveURL(/[?&]lemma=/);
    await expect(page.locator('#concordance-parallels-lemma')).toHaveText('Only with ἄνδρας');
    await expect(page.locator('h3.concordance-tier')).toHaveText(['This Book', 'This author', 'Other authors']);
    await expect(results).toHaveCount(3);
    await expect(page.locator('#concordance-unsearched')).toHaveCount(1);
    await expectNoHorizontalOverflow(page);
    const clear = page.getByRole('link', { name: 'All Genitive absolute: clear the lemma filter', exact: true });
    await expect(clear).toHaveText('All Genitive absolute');
    await activate(page, clear);
    await expect(page).not.toHaveURL(/[?&]lemma=/);
    await expect(results).toHaveCount(6);
    await expect(page.getByRole('link', { name: 'Only with ἄνδρας: parallels signalled by this lemma', exact: true })).toBeVisible();
  });

  await test.step('a row is a KWIC line centred on the anchor, and expands to the extract', async () => {
    const row = page.locator('#concordance-native-results .concordance-result').first();
    await expect(row.locator('.concordance-surface')).toHaveText('ἄνδρας');
    await row.locator('summary').click();
    const construction = row.locator('.concordance-construction');
    await expect(construction).toBeVisible();
    await expect(construction.locator('.sentence-study-construction-name')).toHaveText('Genitive absolute');
    const extract = construction.locator('.sentence-study-construction-extract');
    await expect(extract).toHaveAttribute('aria-hidden', 'true');
    await expect(extract).toHaveText('λέγει ὅτι ἄνδρας, καλούς.');
    expect(await underlined(extract.locator('.construction-anchor'))).toBe(true);
    expect(await weight(extract.locator('.construction-extent').first())).toBeGreaterThanOrEqual(600);
    expect(await weight(extract.locator('.construction-muted').first())).toBe(400);
    await expect(construction.locator('.sr-only')).toHaveText('Words: λέγει … ἄνδρας καλούς');
    await expect(construction.getByRole('link', { name: /Smyth 1920/ })).toHaveAttribute('rel', 'noopener noreferrer');
    await expectNoHorizontalOverflow(page);
  });

  await test.step('the origin sentence is one link away', async () => {
    const back = page.locator('#concordance-parallels-origin');
    await expect(back).toHaveText('Back to the sentence you came from');
    await expect(back).toHaveAttribute('href', /construction=genitive_absolute.*#construction-genitive_absolute-2$/);
  });

  await test.step('Study opens Sentence study with the construction focused, in unchanged order', async () => {
    const row = page.locator('#concordance-native-results .concordance-result').nth(2);
    await activate(page, row.getByRole('link', { name: 'Study this sentence' }));
    await expect(page).toHaveURL(/construction=genitive_absolute/);
    await expect(page).toHaveURL(/#construction-genitive_absolute-2$/);
    const entries = page.locator('#sentence-study-constructions ol > li');
    await expect(entries).toHaveCount(3);
    await expect(entries.nth(0).locator('.sentence-study-construction-name')).toHaveText('Indirect statement (infinitive)');
    await expect(entries.nth(1).locator('.sentence-study-construction-name')).toHaveText('Genitive absolute · from Concordance');
    await expect(entries.nth(2).locator('.sentence-study-construction-name')).toHaveText('Purpose clause');
    await expect(page.locator('.sentence-study-construction--focused')).toHaveCount(1);
    await expect(entries.nth(1)).toHaveClass(/sentence-study-construction--focused/);
    const edge = await entries.nth(1).evaluate((element) => Number.parseFloat(getComputedStyle(element).borderInlineStartWidth));
    expect(edge).toBeGreaterThanOrEqual(3);
    const others = await entries.nth(0).evaluate((element) => Number.parseFloat(getComputedStyle(element).borderInlineStartWidth));
    expect(others).toBe(0);
    await expect(entries.nth(1)).toBeInViewport();
    await expectNoHorizontalOverflow(page);
  });

  await test.step('the way back returns to the same tiered page', async () => {
    await activate(page, page.getByRole('link', { name: 'Return to Concordance results' }));
    await expect(page).toHaveURL(/as=construction/);
    await expect(page.locator('h3.concordance-tier')).toHaveText(['This Book', 'This author', 'Other authors']);
  });
}

test('Parallels lists the tiers, expands a row and returns to the focused entry', async ({ page }) => {
  await signInToGreek(page);
  await acceptanceViewport(page);
  await walkParallels(page);
});

test('Parallels works without JavaScript', async ({ browser, baseURL }, testInfo) => {
  // Keep the project's width and theme: a bare context would fall back to the
  // desktop and light defaults.
  const { viewport, colorScheme } = testInfo.project.use;
  const context = await browser.newContext({ baseURL, javaScriptEnabled: false, viewport, colorScheme });
  const page = await context.newPage();
  try {
    await acceptanceViewport(page);
    await signInToGreek(page);
    await walkParallels(page);
  } finally {
    await context.close();
  }
});

test('a labelled subtype opens filtered and All widens to the whole construction', async ({ page }) => {
  await signInToGreek(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);
  await activate(page, page.getByRole('link', { name: 'Parallels for Indirect statement, infinitive', exact: true }));
  await expect(page).toHaveURL(/subtype=infinitive/);
  await expect(page.getByRole('heading', { level: 2, name: 'Parallels of Indirect statement (infinitive)' })).toBeVisible();
  const widen = page.getByRole('link', { name: 'All Indirect statement', exact: true });
  await expect(widen).toBeVisible();
  await activate(page, widen);
  await expect(page).not.toHaveURL(/subtype=/);
  await expect(page.getByRole('heading', { level: 2, name: 'Parallels of Indirect statement' })).toBeVisible();
  await expect(page.locator('h3.concordance-tier')).toHaveText(['This Book', 'This author', 'Other authors']);
  await expectNoHorizontalOverflow(page);
});

test('a parallels link without its origin gets the safe restart notice', async ({ page }) => {
  await signInToGreek(page);
  await page.goto('/grc/corpus/concordance?as=construction&term=genitive_absolute');
  await expect(page.getByRole('alert')).toContainText('This Concordance link is not valid.');
  await expect(page.locator('h3.concordance-tier')).toHaveCount(0);
});

test('a lemma with no parallels is a corpus result and still names the unsearched Books', async ({ page }) => {
  await signInToGreek(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);
  await activate(page, page.getByRole('link', { name: 'Parallels for Genitive absolute', exact: true }));
  await expect(page).toHaveURL(/as=construction/);
  const href = page.url();
  await page.goto(`${href}&lemma=${encodeURIComponent('οὐδέν')}`);
  await expect(page.getByText('No other analyzed occurrences of Genitive absolute', { exact: false })).toBeVisible();
  await expect(page.locator('#concordance-unsearched')).toContainText('Not searched for constructions:');
  await expectNoHorizontalOverflow(page);
});

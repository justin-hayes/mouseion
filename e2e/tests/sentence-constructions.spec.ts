import { expect, Page, test } from '../support/test';
import { acceptanceViewport, expectNoHorizontalOverflow, underlined, weight } from '../support/construction-probes';

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

// The fixture's Greek native_agdt sentence "λέγει ὅτι ἄνδρας, καλούς." carries
// three synthetic occurrences: an indirect statement over the whole sentence,
// a discontinuous genitive absolute (λέγει … ἄνδρας, καλούς) and a purpose
// clause nested in the first, anchored on the identified target ὅτι.
async function openSyntaxSentence(page: Page) {
  await page.goto('/vocabulary/concordance?term=fixture-syntax');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await expect(row.locator('.concordance-location')).toHaveText('1.1.1\u20132');
  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\/sentence/);
  await expect(page.locator('.sentence-study-location')).toHaveText('1.1.1\u20132');
}

async function expectConstructions(page: Page) {
  const region = page.locator('#sentence-study-constructions');
  await expect(region.getByRole('heading', { level: 3, name: 'Constructions' })).toBeVisible();
  const entries = region.locator('ol > li');
  await expect(entries).toHaveCount(3);

  await test.step('entries are ordered by first extent word, outer first, with their names', async () => {
    await expect(entries.nth(0).locator('.sentence-study-construction-name')).toHaveText('Indirect statement (infinitive)');
    await expect(entries.nth(1).locator('.sentence-study-construction-name')).toHaveText('Genitive absolute');
    await expect(entries.nth(2).locator('.sentence-study-construction-name')).toHaveText('Purpose clause');
  });

  await test.step('nesting and the identified target are stated in words', async () => {
    await expect(entries.nth(0).locator('.sentence-study-construction-nesting')).toHaveCount(0);
    await expect(entries.nth(1)).toContainText('Inside construction 1.');
    await expect(entries.nth(2)).toContainText('Inside construction 1.');
    await expect(entries.nth(0)).toContainText('Signalled by λέγει.');
    await expect(entries.nth(0)).toContainText('Contains the identified target.');
    await expect(entries.nth(1)).toContainText('Signalled by ἄνδρας.');
    await expect(entries.nth(1)).not.toContainText('identified target');
    await expect(entries.nth(2)).toContainText('Signalled by ὅτι. The identified target signals this construction.');
  });

  await test.step('the discontinuous extent is visible without colour: weight and underline', async () => {
    const extract = entries.nth(1).locator('.sentence-study-construction-extract');
    await expect(extract).toHaveAttribute('aria-hidden', 'true');
    await expect(extract).toHaveText('λέγει ὅτι ἄνδρας, καλούς.');
    const extent = extract.locator('.construction-extent').first();
    const muted = extract.locator('.construction-muted').first();
    const anchor = extract.locator('.construction-anchor');
    await expect(extent).toHaveText('λέγει');
    await expect(muted).toHaveText('ὅτι');
    await expect(anchor).toHaveText('ἄνδρας');
    expect(await weight(extent)).toBeGreaterThanOrEqual(600);
    expect(await weight(muted)).toBe(400);
    expect(await underlined(anchor)).toBe(true);
    expect(await underlined(muted)).toBe(false);
    // The comma between ἄνδρας and καλούς is a punctuation-only gap, so it is extent.
    const trailing = extract.locator('.construction-extent').last();
    await expect(trailing).toHaveText(', καλούς');
    expect(await weight(trailing)).toBeGreaterThanOrEqual(600);
  });

  await test.step('the nested extract mutes what lies outside it', async () => {
    const extract = entries.nth(2).locator('.sentence-study-construction-extract');
    await expect(extract.locator('.construction-muted').first()).toHaveText('λέγει');
    await expect(extract.locator('.construction-anchor')).toHaveText('ὅτι');
    expect(await underlined(extract.locator('.construction-anchor'))).toBe(true);
  });

  await test.step('a visually hidden Words line stands in for the extract', async () => {
    await expect(entries.nth(0).locator('.sr-only')).toHaveText('Words: λέγει ὅτι ἄνδρας καλούς');
    await expect(entries.nth(1).locator('.sr-only')).toHaveText('Words: λέγει … ἄνδρας καλούς');
    await expect(entries.nth(2).locator('.sr-only')).toHaveText('Words: ὅτι ἄνδρας καλούς');
  });

  await test.step('the citation links to the grammar in a new tab without an opener', async () => {
    const link = entries.nth(2).getByRole('link', { name: /Smyth 1920/ });
    await expect(link).toHaveAttribute('href', /perseus\.tufts\.edu.*smythp=2193$/);
    await expect(link).toHaveAttribute('target', '_blank');
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
  });

  await test.step('the region sits between the sentence and the morphology line', async () => {
    const order = await page.evaluate(() => {
      const index = (selector: string) => {
        const element = document.querySelector(selector);
        return element ? Array.from(document.querySelectorAll('*')).indexOf(element) : -1;
      };
      return [index('.sentence-study-text'), index('#sentence-study-constructions'), index('.sentence-study-line'), index('#sentence-study-word-relations')];
    });
    expect(order[0]).toBeGreaterThan(-1);
    expect(order[1]).toBeGreaterThan(order[0]);
    expect(order[2]).toBeGreaterThan(order[1]);
    expect(order[3]).toBeGreaterThan(order[2]);
  });

  await test.step('the complete text alternative names each word\'s constructions', async () => {
    const alternative = page.locator('#sentence-study-text-alternative');
    await alternative.locator('summary').click();
    await expect(alternative).toContainText('Constructions: 1 (Indirect statement), 3 (Purpose clause). Signals: 3 (Purpose clause).');
  });

  await expectNoHorizontalOverflow(page);
}

test('Sentence study names the constructions with nested and discontinuous extracts', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);
  await expectConstructions(page);
});

test('Sentence study constructions work without JavaScript', async ({ browser, baseURL }, testInfo) => {
  // Keep the project's width and theme: a bare context would fall back to the
  // desktop and light defaults.
  const { viewport, colorScheme } = testInfo.project.use;
  const context = await browser.newContext({ baseURL, javaScriptEnabled: false, viewport, colorScheme });
  const page = await context.newPage();
  try {
    await acceptanceViewport(page);
    await signIn(page);
    await openSyntaxSentence(page);
    await expectConstructions(page);
  } finally {
    await context.close();
  }
});

// Enlarged text (200%) must not push the page wider than the viewport.
test('Sentence study constructions survive enlarged text', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  await expect(page.locator('#sentence-study-constructions ol > li')).toHaveCount(3);
  await expectNoHorizontalOverflow(page);
});

// A long Greek sentence is reprinted in every extract; it wraps rather than
// widening the page, at normal and enlarged text.
test('Sentence study constructions wrap a long Greek sentence', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);
  await openSyntaxSentence(page);
  const url = new URL(page.url());
  url.searchParams.set('unit', 'fixture-syntax-long-unit');
  url.searchParams.set('sentence', '0');
  url.searchParams.set('target', '5');
  await page.goto(url.toString());
  const entries = page.locator('#sentence-study-constructions ol > li');
  await expect(entries).toHaveCount(3);
  await expect(entries.nth(0).locator('.sentence-study-construction-extract')).toContainText('ἀνθρωπότητος');
  await expectNoHorizontalOverflow(page);
  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  await expectNoHorizontalOverflow(page);
  for (const extract of await entries.locator('.sentence-study-construction-extract').all()) {
    const box = await extract.boundingBox();
    expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  }
});

test('Sentence study without native syntax shows no Constructions', async ({ page }) => {
  await signIn(page);
  await page.goto('/vocabulary/concordance?term=Haus');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await row.locator('summary').click();
  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page.locator('#sentence-study-constructions')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Constructions' })).toHaveCount(0);
});

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

function sentenceStudyURL(unit: string, target: string, surface: string) {
  const values = new URLSearchParams({
    book: 'fixture-kwic-poetry',
    run: 'fixture-run',
    corpus: 'fixture-corpus',
    unit,
    sentence: '0',
    target,
    target_surface: surface,
  });
  return `/vocabulary/concordance/sentence?${values.toString()}`;
}

test('Concordance and sentence study keep source lines distinct from soft wraps', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);
  await page.goto('/vocabulary/concordance?term=fixture-poetry');

  const row = page.locator('#concordance-native-results .concordance-result').first();
  const summary = row.locator('summary');
  await expect(row.locator('.concordance-surface')).toHaveText('Geist');
  await expect(summary.locator('.concordance-line-break')).toHaveCount(3);
  await expect(summary.locator('.concordance-line-break').first()).toHaveText(' / ');

  await summary.focus();
  await expect(summary).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(row.locator('details')).toHaveAttribute('open', '');
  await expect(row.locator('.concordance-attribution')).toHaveText('Mephistopheles');
  await expect(row.locator('.concordance-context-line')).toHaveCount(4);
  await expect(row.locator('.concordance-context-line').first()).toHaveText('Der Geist der Medicin ist leicht zu fassen;');
  await expect(row.locator('.concordance-observed-target')).toHaveText('Geist');

  const lineStyle = await row.locator('.concordance-context-line').first().evaluate(element => {
    const style = getComputedStyle(element);
    return {
      display: style.display,
      padding: Number.parseFloat(style.paddingInlineStart),
      indent: Number.parseFloat(style.textIndent),
    };
  });
  expect(lineStyle.display).toBe('block');
  expect(lineStyle.padding).toBeGreaterThan(0);
  expect(lineStyle.indent).toBeLessThan(0);
  await expectNoHorizontalOverflow(page);

  await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
  await expectNoHorizontalOverflow(page);

  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page).toHaveURL(/\/vocabulary\/concordance\/sentence/);
  await expect(page.locator('.sentence-study-attribution')).toHaveText('Mephistopheles');
  await expect(page.locator('.sentence-study-text .sentence-study-source-line')).toHaveCount(4);
  await expect(page.locator('.sentence-study-line .sentence-study-line-row')).toHaveCount(4);
  await expect(page.locator('.sentence-study-line-row').first().locator('strong.sentence-study-word')).toHaveText('Geist');

  const alternative = page.locator('#sentence-study-text-alternative');
  await expect(alternative).not.toHaveAttribute('open', '');
  await alternative.locator('summary').click();
  await expect(alternative.locator('.sentence-study-line-end')).toHaveCount(3);
  await expect(alternative.locator('.sentence-study-line-end').first()).toContainText('end of source line');
  await expectNoHorizontalOverflow(page);
});

test('Sentence study names a direction sentence and keeps prose inline', async ({ page }) => {
  await signIn(page);
  await acceptanceViewport(page);

  await page.goto(sentenceStudyURL('fixture-stage-unit', '1', 'schlägt'));
  await expect(page.locator('.sentence-study-attribution')).toHaveText('Stage direction');
  await expect(page.locator('.sentence-study-text')).toContainText('Er schlägt das Buch auf und erblickt das Zeichen des Makrokosmus.');
  await expect(page.locator('.sentence-study-source-line')).toHaveCount(0);
  await expect(page.locator('.sentence-study-line-row')).toHaveCount(0);
  await expectNoHorizontalOverflow(page);

  await page.goto('/vocabulary/concordance?term=Haus');
  const row = page.locator('#concordance-native-results .concordance-result').first();
  await expect(row.locator('.concordance-line-break')).toHaveCount(0);
  await expect(row.locator('.concordance-context-line')).toHaveCount(0);
  await row.locator('summary').click();
  await expect(row.locator('.concordance-context p')).toHaveText('Das Haus sieht gut aus.');

  await row.getByRole('link', { name: 'Study this sentence' }).click();
  await expect(page.locator('.sentence-study-attribution')).toHaveCount(0);
  await expect(page.locator('.sentence-study-source-line')).toHaveCount(0);
  await expect(page.locator('.sentence-study-line-row')).toHaveCount(0);
  await expect(page.locator('.reading-text.sentence-study-text')).toHaveText('Das Haus sieht gut aus.');
});

test('Source-line disclosures and sentence study work without JavaScript', async ({ page, browser, baseURL }) => {
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await acceptanceViewport(noScriptPage);
    await signIn(noScriptPage);
    await noScriptPage.goto('/vocabulary/concordance?term=fixture-poetry');
    const row = noScriptPage.locator('#concordance-native-results .concordance-result').first();
    await row.locator('summary').click();
    await expect(row.locator('.concordance-context-line')).toHaveCount(4);
    await expectNoHorizontalOverflow(noScriptPage);

    await row.getByRole('link', { name: 'Study this sentence' }).click();
    await expect(noScriptPage.locator('.sentence-study-source-line')).toHaveCount(4);
    await expect(noScriptPage.locator('.sentence-study-line-row')).toHaveCount(4);
    const alternative = noScriptPage.locator('#sentence-study-text-alternative');
    await alternative.locator('summary').click();
    await expect(alternative.locator('.sentence-study-line-end')).toHaveCount(3);
    await expectNoHorizontalOverflow(noScriptPage);
  } finally {
    await noScriptContext.close();
  }
});

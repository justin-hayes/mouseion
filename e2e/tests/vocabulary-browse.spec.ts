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

test('Current-reading Browse keeps its prefix form usable without JavaScript', async ({ page, browser, baseURL }) => {
  const noScriptContext = await browser.newContext({ baseURL, javaScriptEnabled: false });
  const noScriptPage = await noScriptContext.newPage();
  try {
    await signIn(noScriptPage);
    await noScriptPage.goto('/reading');
    await expect(noScriptPage.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
    await expect(noScriptPage.locator('body')).not.toContainText('Browse selection');
    await expect(noScriptPage.locator('body')).not.toContainText('Custom deck');
    await expect(noScriptPage.getByRole('button', { name: 'Select', exact: true })).toHaveCount(0);
    await expect(noScriptPage.getByRole('button', { name: 'Remove', exact: true })).toHaveCount(0);
    const browseForm = noScriptPage.locator('form[action="/reading"]');
    await expect(browseForm.locator('input[name="reading"]')).toHaveValue('fixture-book');
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveClass(/\binput\b/);
    const includeAll = noScriptPage.getByRole('checkbox', { name: 'Show already accounted-for words' });
    await expect(includeAll).not.toBeChecked();
    const checkboxSize = await includeAll.evaluate(element => {
      const rect = element.getBoundingClientRect();
      return { width: rect.width, height: rect.height, labelHeight: element.closest('label')!.getBoundingClientRect().height };
    });
    expect(checkboxSize.width).toBeLessThan(44);
    expect(checkboxSize.height).toBeLessThan(44);
    expect(checkboxSize.labelHeight).toBeGreaterThanOrEqual(44);
    await expect(browseForm.getByRole('button', { name: 'Search' })).toHaveClass(/\bbtn\b/);
    await includeAll.check();
    const search = noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' });
    await search.focus();
    await expect(search).toBeFocused();
    await search.fill('haus');
    await search.press('Enter');
    await expect(noScriptPage).toHaveURL(/\/reading\?.*all=1.*q=haus|\/reading\?.*q=haus.*all=1/);
    await expect(noScriptPage.locator('.vocabulary-applied-query')).toContainText('Applied prefix: haus');
    await expect(noScriptPage.locator('#vocabulary-results-heading')).toBeFocused();
    await expect(browseForm.locator('input[name="book"], input[name="pos"], select[name="known"], select[name="reserved"], select[name="sort"]')).toHaveCount(0);
    const hausRow = noScriptPage.getByRole('row').filter({ hasText: 'haus' });
    await expect(hausRow).toContainText('Known');
    await expect(hausRow).toContainText('In a Book deck');
    const noOverflow = await noScriptPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
    expect(noOverflow).toBe(true);

    await noScriptPage.goto('/reading?q=zznotfound');
    await expect(noScriptPage.getByText('No visible identities match this canonical-lemma prefix. Already-accounted-for words may be hidden; show them to include those matches.')).toBeVisible();
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveValue('zznotfound');
    await noScriptPage.getByRole('link', { name: 'Clear prefix' }).click();
    await expect(noScriptPage.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveValue('');
  } finally {
    await noScriptContext.close();
  }

  await signIn(page);
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto('/reading');
  await expect(page.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('Vocabulary heading and peer navigation keep the compact hierarchy', async ({ page }) => {
  await signIn(page);
  await page.setViewportSize({ width: 1280, height: 800 });
  // Browse lives in Reading; /vocabulary is the Concordance and its peer views.
  await page.goto('/vocabulary');
  const header = page.locator('.vocabulary-page-header');
  const heading = header.getByRole('heading', { name: 'Vocabulary', exact: true });
  const navigation = header.getByRole('navigation', { name: 'Vocabulary views' });
  await expect(heading).toBeVisible();
  await expect(navigation.getByRole('link')).toHaveCount(2);
  await expect(navigation.getByRole('link', { name: 'Concordance', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(navigation.getByRole('link', { name: 'Import Known words' })).not.toHaveAttribute('aria-current', 'page');
  const desktopLayout = await header.evaluate(element => {
    const title = element.querySelector('h1')!.getBoundingClientRect();
    const tabs = element.querySelector('nav')!.getBoundingClientRect();
    return { titleTop: title.top, tabsTop: tabs.top, titleSize: getComputedStyle(element.querySelector('h1')!).fontSize };
  });
  expect(desktopLayout.titleSize).toBe('25px');
  expect(Math.abs(desktopLayout.tabsTop - desktopLayout.titleTop)).toBeLessThan(10);

  await page.setViewportSize({ width: 375, height: 667 });
  const compactLayout = await header.evaluate(element => {
    const title = element.querySelector('h1')!.getBoundingClientRect();
    const tabs = element.querySelector('nav')!.getBoundingClientRect();
    return { titleTop: title.top, tabsTop: tabs.top };
  });
  expect(compactLayout.tabsTop).toBeGreaterThan(compactLayout.titleTop);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('Concordance query and applied results summary fit the first desktop and compact viewport', async ({ page }) => {
  await signIn(page);
  for (const viewport of [{ width: 1280, height: 800 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport);
    await page.goto('/vocabulary/concordance?term=Haus');
    await expect(page.locator('#concordance-term')).toHaveValue('Haus');
    await expect(page.locator('#concordance-summary')).toBeVisible();
    const initiallyVisible = await page.evaluate(() => {
      const selectors = [
        '#concordance-term',
        '.concordance-query-term-controls > button',
        '#concordance-summary',
        '#concordance-results > .concordance-results-summary',
        '#concordance-native-results li:first-child .concordance-row > summary',
      ];
      return Object.fromEntries(selectors.map(selector => {
        const bounds = document.querySelector(selector)!.getBoundingClientRect();
        return [selector, bounds.bottom > 0 && bounds.top < window.innerHeight];
      }));
    });
    expect(initiallyVisible, `query and result summary intersect ${viewport.width}x${viewport.height}`).toEqual({
      '#concordance-term': true,
      '.concordance-query-term-controls > button': true,
      '#concordance-summary': true,
      '#concordance-results > .concordance-results-summary': true,
      '#concordance-native-results li:first-child .concordance-row > summary': true,
    });
    const bookLabel = page.locator('.concordance-source').first();
    await expect(bookLabel).toContainText('occurrences on this page');
    await expect(bookLabel.locator('.concordance-source-title')).toBeVisible();
    const rowPresentation = await page.locator('.concordance-row summary').first().evaluate(summary => {
      const before = summary.querySelector('.concordance-before')!;
      const target = summary.querySelector('.concordance-surface')!;
      const after = summary.querySelector('.concordance-after')!;
      const bounds = (element: Element) => element.getBoundingClientRect();
      return {
        beforeRight: bounds(before).right,
        targetLeft: bounds(target).left,
        targetRight: bounds(target).right,
        afterLeft: bounds(after).left,
        beforeDisplay: getComputedStyle(before).display,
        sourceFont: getComputedStyle(summary).fontFamily,
      };
    });
    expect(rowPresentation.sourceFont).toContain('Literata');
    if (viewport.width > 600) {
      expect(rowPresentation.beforeRight).toBeLessThanOrEqual(rowPresentation.targetLeft);
      expect(rowPresentation.afterLeft).toBeGreaterThanOrEqual(rowPresentation.targetRight);
    } else {
      expect(rowPresentation.beforeDisplay).toBe('block');
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  }
});

test('Concordance groups and KWIC rows wrap long German, Italian, and Greek text', async ({ page }) => {
  await signIn(page);
  const examples = [
    {
      title: 'Der außergewöhnlich lange Titel einer umfassenden deutschen Ausgabe',
      target: 'Donaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamtengesellschaft',
      before: '… zwischen außergewöhnlich langen zusammengesetzten deutschen Wörtern',
      after: ' und weiteren sorgfältig ausgewählten Beispielen aus dem Text.',
      passage: 'Am Anfang stand ein langer deutscher Satz, der sich mit vielen zusätzlichen Wörtern und erklärenden Gedanken fortsetzt.',
    },
    {
      title: 'Una storia straordinariamente lunga: edizione italiana annotata',
      target: 'precipitevolissimevolmente',
      before: '… attraverso un contesto italiano particolarmente articolato',
      after: ' e una frase che continua con ulteriori dettagli significativi.',
      passage: 'All’inizio c’era una frase italiana molto lunga, con accenti, parole composte e un contesto che continua con chiarezza.',
    },
    {
      title: 'Μια εξαιρετικά μακριά ελληνική βιβλιογραφική περιγραφή',
      target: 'ηλεκτροεγκεφαλογραφήματος',
      before: '… μέσα σε μια εξαιρετικά εκτενή ελληνική πρόταση',
      after: ' και με πρόσθετες λέξεις που συνεχίζουν το νόημα.',
      passage: 'Στην αρχή υπήρχε μια μεγάλη ελληνική πρόταση με τόνους, διαλυτικά και αρκετές επιπλέον λέξεις για το πλήρες νόημα.',
    },
  ];

  for (const viewport of [{ width: 1280, height: 800 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(viewport);
    for (const example of examples) {
      await page.goto('/vocabulary/concordance?term=Haus');
      await page.locator('.concordance-row').first().evaluate((row, content) => {
        row.closest('.concordance-result')!.querySelector('.concordance-source-title')!.textContent = content.title;
        row.querySelector('.concordance-before')!.textContent = content.before;
        row.querySelector('.concordance-surface')!.textContent = content.target;
        row.querySelector('.concordance-after')!.textContent = content.after;
        row.querySelector('.concordance-context p')!.textContent = content.passage;
      }, example);
      const firstRow = page.locator('.concordance-row').first();
      await expect(page.locator('.concordance-source-title').first()).toHaveText(example.title);
      await firstRow.locator('summary').click();
      await expect(firstRow.locator('.concordance-context p')).toHaveText(example.passage);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `${viewport.width}px ${example.title}`).toBe(true);
    }
  }
});

test('Browse keeps the current page and controls while exploring a word on page two', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading?q=paging');
  await page.locator('form[action="/reading"]').evaluate(element => element.setAttribute('data-stable-search', 'yes'));
  await page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link', { name: 'Next' }).click();
  await expect(page).toHaveURL(/page=2/);
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(page.locator('form[action="/reading"]')).toHaveAttribute('data-stable-search', 'yes');
  await page.locator('#vocabulary-workflow').evaluate(element => element.setAttribute('data-stable-controls', 'yes'));
  const row = page.getByRole('row').filter({ hasText: 'paging25' });
  await expect(row.getByRole('link', { name: 'paging25' })).toBeVisible();
  await expect(row.getByRole('button')).toHaveCount(0);
  await expect(page.locator('#vocabulary-workflow')).toHaveAttribute('data-stable-controls', 'yes');
  await expect(page.locator('body')).not.toContainText('Browse selection');
  await expect(page.locator('body')).not.toContainText('Custom deck');
});

test('Browse pagination lands on the new results rather than above them', async ({ page }) => {
  await signIn(page);
  await page.goto('/reading?q=paging');
  for (const [direction, number] of [['Next', 2], ['Previous', 1]] as const) {
    await page.getByRole('navigation', { name: 'Browse pages' }).getByRole('link', { name: direction }).click();
    await expect(page.getByText(`Page ${number} of 2`)).toBeVisible();
    const heading = page.locator('#vocabulary-results-heading');
    await expect(heading).toBeFocused();
    const results = await heading.evaluate(element => {
      const bounds = element.getBoundingClientRect();
      return { top: bounds.top, bottom: bounds.bottom, viewportHeight: window.innerHeight };
    });
    expect(results.top).toBeGreaterThanOrEqual(-1); // Subpixel rounding at narrow widths.
    expect(results.bottom).toBeLessThan(results.viewportHeight);
    expect(results.top).toBeLessThan(160);
  }
});

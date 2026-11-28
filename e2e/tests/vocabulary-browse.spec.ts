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

test('Vocabulary heading keeps the compact hierarchy without peer navigation', async ({ page }) => {
  await signIn(page);
  await page.setViewportSize({ width: 1280, height: 800 });
  // /vocabulary is the single Concordance view; a Book's words live on its detail page.
  await page.goto('/de/vocabulary');
  const header = page.locator('.vocabulary-page-header');
  const heading = header.getByRole('heading', { name: 'Vocabulary', exact: true });
  await expect(heading).toBeVisible();
  await expect(header.getByRole('navigation')).toHaveCount(0);
  const desktopLayout = await header.evaluate(element => {
    const title = element.querySelector('h1')!.getBoundingClientRect();
    return { titleSize: getComputedStyle(element.querySelector('h1')!).fontSize };
  });
  expect(desktopLayout.titleSize).toBe('25px');

  await page.setViewportSize({ width: 375, height: 667 });
  await expect(heading).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test('Concordance query and applied results summary fit the first desktop and compact viewport', async ({ page }) => {
  // The project's viewport is either the desktop or the compact layout under test.
  await signIn(page);
  const viewport = page.viewportSize()!;
  await page.goto('/de/vocabulary/concordance?term=Haus');
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

  // Runs at the project's viewport; desktop and compact are separate projects.
  const viewport = page.viewportSize()!;
  for (const example of examples) {
    await page.goto('/de/vocabulary/concordance?term=Haus');
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
});

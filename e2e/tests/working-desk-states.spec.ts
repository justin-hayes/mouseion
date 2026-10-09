import { expect, Locator, Page, test } from '../support/test';
import { renderedTextContrast } from '../support/contrast';

type Scenario = '' | 'no-analysis' | 'no-vocabulary' | 'counts-updating' | 'counts-unavailable' | 'accounted';

async function setScenario(page: Page, baseURL: string | undefined, scenario: Scenario) {
  const response = await page.request.post(new URL('/fixture/vocabulary-browse-scenario', baseURL).href, { form: { scenario } });
  expect(response.status(), `set Browse scenario ${scenario || 'default'}`).toBe(204);
}

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

// The Working desk's degraded states must leave the Reading lifecycle reachable:
// switching, finishing, ending, and the retained deck download.
async function expectLifecycleReachable(page: Page) {
  const goal = page.locator('.journey-book--goal');
  await expect(goal.getByRole('link', { name: 'Switch current reading', exact: true })).toBeVisible();
  await expect(goal.locator('details').filter({ hasText: 'Mark reading finished' }).locator('summary')).toBeVisible();
  const end = goal.locator('details').filter({ hasText: 'End current reading' }).locator('summary');
  await end.click();
  await expect(goal.getByRole('button', { name: 'Confirm end current reading' })).toBeVisible();
  await expect(goal.getByRole('link', { name: 'Download deck' })).toBeVisible();
}

async function expectInsideViewport(locator: Locator, width: number) {
  const box = await locator.boundingBox();
  expect(box, 'element has a rendered box').not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(width);
}

test.afterEach(async ({ page, baseURL }) => {
  await setScenario(page, baseURL, '');
});

test('Working desk shows counts updating with Refresh status and withholds vocabulary', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'counts-updating');
  await signIn(page);
  await page.goto('/reading');
  const workflow = page.locator('#vocabulary-workflow');
  const status = workflow.locator('#vocabulary-counts-status');
  await expect(status).toHaveAttribute('role', 'status');
  await expect(status).toContainText('Updating vocabulary counts.');
  await expect(status).toContainText('Vocabulary is withheld until every currently analyzed Book in this study language has exact, ready counts.');
  await expect(workflow.getByRole('table', { name: 'Current effective vocabulary' })).toHaveCount(0);
  await expect(workflow).not.toContainText('No eligible vocabulary in this analysis');
  expect(await status.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);

  const refresh = workflow.getByRole('link', { name: 'Refresh status', exact: true });
  await expect(refresh).toBeVisible();
  await refresh.click();
  await expect(page).toHaveURL(/\/reading\?/);
  await expect(page.locator('#vocabulary-workflow #vocabulary-counts-status')).toContainText('Updating vocabulary counts.');
  await expectLifecycleReachable(page);
});

test('Working desk explains unavailable counts without leaking rows and keeps Refresh status', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'counts-unavailable');
  await signIn(page);
  await page.goto('/reading');
  const workflow = page.locator('#vocabulary-workflow');
  const status = workflow.locator('#vocabulary-counts-status');
  await expect(status).toHaveAttribute('role', 'alert');
  await expect(status).toContainText('Vocabulary counts unavailable.');
  await expect(status).toContainText('Refresh status only re-checks; it does not requeue failed work or rerun analysis.');
  await expect(status).toContainText('Ask your server operator to restart Mouseion');
  expect(await status.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
  await expect(workflow.getByRole('table', { name: 'Current effective vocabulary' })).toHaveCount(0);
  await expect(workflow.getByRole('row', { name: /gehen/ })).toHaveCount(0);
  await expect(workflow).not.toContainText('Updating vocabulary counts.');
  await expect(workflow.getByRole('link', { name: 'Refresh status', exact: true })).toBeVisible();
  await expectLifecycleReachable(page);
});

test('Working desk points an analysis-less Book to Switch and never shows a zero result', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'no-analysis');
  await signIn(page);
  await page.goto('/reading');
  const workflow = page.locator('#vocabulary-workflow');
  await expect(workflow.getByText('This Book has no current completed analysis, so Browse cannot show current vocabulary evidence.')).toBeVisible();
  await expect(workflow.getByRole('link', { name: 'Switch current reading', exact: true })).toHaveAttribute('href', '/reading/switch');
  await expect(workflow.getByRole('table', { name: 'Current effective vocabulary' })).toHaveCount(0);
  await expect(workflow).not.toContainText('No eligible vocabulary in this analysis');
  await expect(workflow).not.toContainText('Updating vocabulary counts.');
  await expectLifecycleReachable(page);
});

test('Working desk reports an analysis with no eligible vocabulary as an empty result', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'no-vocabulary');
  await signIn(page);
  await page.goto('/reading');
  const workflow = page.locator('#vocabulary-workflow');
  await expect(workflow.getByText('No eligible vocabulary in this analysis.', { exact: true })).toBeVisible();
  await expect(workflow.getByRole('table', { name: 'Current effective vocabulary' })).toHaveCount(0);
  await expect(workflow.getByRole('link', { name: 'Clear prefix' })).toHaveCount(0);
  await expect(workflow).not.toContainText('already Known, Reserved, or included in a prepared Book deck');
  await expectLifecycleReachable(page);
});

test('Working desk separates everything-accounted-for from annotated rows and recovers with the reveal and Clear prefix', async ({ page, baseURL }) => {
  await setScenario(page, baseURL, 'accounted');
  await signIn(page);
  await page.goto('/reading');
  const workflow = page.locator('#vocabulary-workflow');
  await expect(workflow.getByText('All eligible identities in this Book are already Known, Reserved, or included in a prepared Book deck.')).toBeVisible();
  await expect(workflow).toContainText('This does not mean they are mastered, and it is not a failed search.');
  await expect(workflow.getByRole('table', { name: 'Current effective vocabulary' })).toHaveCount(0);
  await expect(workflow).not.toContainText('No eligible vocabulary in this analysis');
  expect(await workflow.locator('p').filter({ hasText: 'already Known, Reserved' }).evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);

  const reveal = workflow.getByRole('checkbox', { name: 'Show already accounted-for words' });
  await reveal.check();
  await workflow.getByRole('button', { name: 'Search', exact: true }).click();
  await expect(page.locator('#vocabulary-results-heading')).toBeFocused();
  const table = workflow.getByRole('table', { name: 'Current effective vocabulary' });
  await expect(table).toBeVisible();
  await expect(table.getByRole('row').filter({ hasText: 'haus' })).toContainText('Known');
  await expect(table.getByRole('row').filter({ hasText: 'haus' })).toContainText('In a Book deck');
  await expect(table.getByRole('row').filter({ hasText: 'lesen' })).toContainText('Known · Reserved');
  await expect(table.getByRole('row').filter({ hasText: 'lesen' })).not.toContainText('In a Book deck');
  await expectLifecycleReachable(page);

  await page.goto('/reading?q=zznone');
  await expect(workflow.getByText('No visible identities match this canonical-lemma prefix. Already-accounted-for words may be hidden; show them to include those matches.')).toBeVisible();
  await expect(workflow).not.toContainText('All eligible identities in this Book are already Known');
  await workflow.getByRole('link', { name: 'Clear prefix', exact: true }).click();
  await expect(page.getByRole('searchbox', { name: 'Canonical lemma prefix' })).toHaveValue('');
  await expect(workflow.getByText('All eligible identities in this Book are already Known, Reserved, or included in a prepared Book deck.')).toBeVisible();
});

test('Working desk states fit 390 and 320 px without page overflow, with a complete title and collapsed notes', async ({ page, baseURL }) => {
  test.setTimeout(90_000);
  await signIn(page);
  const cases: Array<[Scenario, string]> = [
    ['', '/reading'],
    ['no-analysis', '/reading'],
    ['no-vocabulary', '/reading'],
    ['counts-updating', '/reading'],
    ['counts-unavailable', '/reading'],
    ['accounted', '/reading'],
    ['accounted', '/reading?all=1'],
  ];
  for (const [scenario, path] of cases) {
    await setScenario(page, baseURL, scenario);
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 700 });
      await page.goto(path);
      const label = `${scenario || 'default'} ${path} at ${width}px`;
      await expect(page.locator('#vocabulary-workflow'), label).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), label).toBe(true);

      const title = page.locator('.journey-book__title');
      await expect(title, label).toHaveText('Der lange Weg nach Hause');
      expect(await title.evaluate(element => element.scrollWidth <= element.clientWidth), label).toBe(true);
      await expectInsideViewport(title, width);

      await expect(page.locator('.goal-card__deck'), label).toContainText('Deck status');
      await expectInsideViewport(page.getByRole('link', { name: 'Download deck' }), width);

      const disclosure = page.locator('details.reading-supporting');
      await expect(disclosure.locator(':scope > summary'), label).toBeVisible();
      expect(await disclosure.evaluate(node => (node as HTMLDetailsElement).open), label).toBe(false);
      await expect(page.getByRole('heading', { name: 'Reserved vocabulary', exact: true }), label).toBeHidden();
    }
  }
});

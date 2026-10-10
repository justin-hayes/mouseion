import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

// Each fixture Book states one Analysis evidence situation as typed signals.
// The expectations are the wording and recovery that My Books and the Reading
// chooser show for that situation, in the group the chooser places it.
const situations = [
  {
    name: 'stale analysis',
    bookId: 'fixture-stale',
    title: 'Veraltete Analyse',
    evidence: 'Analysis out of date.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-stale/reanalyze' },
    chooserGroup: 'Not assessed',
    chooserDescription: 'The analysis no longer matches the current book content. Retry analysis to refresh its evidence.',
    chooserRetry: true,
  },
  {
    name: 'cancelled analysis',
    bookId: 'fixture-cancelled',
    title: 'Abgebrochene Analyse',
    evidence: 'Analysis cancelled.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-cancelled/reanalyze' },
    chooserGroup: 'Not assessed',
    chooserDescription: 'The last analysis did not complete. Retry analysis to refresh its evidence.',
    chooserRetry: true,
  },
  {
    name: 'publication pending',
    bookId: 'fixture-publication-pending',
    title: 'Veröffentlichung ausstehend',
    evidence: 'Analysis in progress.',
    recovery: undefined,
    chooserGroup: 'Analysis in progress',
    chooserDescription: 'Analysis finished and its result is being published.',
    chooserRetry: false,
  },
  {
    name: 're-analysis running over a published analysis',
    bookId: 'fixture-reanalysis-running',
    title: 'Neue Analyse läuft',
    evidence: 'Re-analysis running.',
    recovery: undefined,
    chooserGroup: 'Below 95%',
    chooserDescription: undefined,
    chooserRetry: false,
  },
  {
    name: 're-analysis failed over a published analysis',
    bookId: 'fixture-reanalysis-failed',
    title: 'Neue Analyse fehlgeschlagen',
    evidence: 'Re-analysis failed.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-reanalysis-failed/reanalyze' },
    chooserGroup: 'Below 95%',
    chooserDescription: undefined,
    chooserRetry: false,
  },
];

test.describe('Analysis evidence situations', () => {
  for (const situation of situations) {
    test(`My Books and the Reading chooser present ${situation.name}`, async ({ page }) => {
      await signIn(page);

      await page.goto('/library?disposition=to_read');
      const row = page.locator(`#book-row-${situation.bookId}`);
      await expect(row).toHaveCount(1);
      await expect(row.locator('.library-book__evidence')).toHaveText(situation.evidence);
      if (situation.recovery) {
        const recovery = row.getByRole('button', { name: situation.recovery.label, exact: true });
        await expect(recovery).toBeVisible();
        await expect(recovery.locator('xpath=..')).toHaveAttribute('action', situation.recovery.action);
      } else {
        await expect(row.getByRole('button', { name: /^Retry / })).toHaveCount(0);
      }

      await page.goto('/library');
      await page.getByLabel('Study language').selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
      await page.goto('/reading/switch');
      const group = page.getByRole('region', { name: situation.chooserGroup, exact: true });
      const candidate = group.locator('li.reading-chooser-book').filter({ hasText: situation.title });
      await expect(candidate).toHaveCount(1);
      await expect(candidate.getByRole('heading', { name: situation.title })).toBeVisible();
      if (situation.chooserDescription) {
        await expect(candidate.locator('.reading-chooser-entry__evidence')).toContainText(situation.chooserDescription);
      }
      const retry = candidate.getByRole('button', { name: 'Retry acquisition or analysis' });
      await expect(retry).toHaveCount(situation.chooserRetry ? 1 : 0);
      if (situation.chooserGroup === 'Below 95%') {
        // Coverage groups read the published analysis and offer switching to it;
        // a newer run's recovery stays in My Books, not in the chooser.
        await expect(candidate.locator('.reading-chooser-entry__evidence')).toContainText('45,678 of 123,456 word occurrences');
        // A current reading exists in the fixture, so the chooser offers the switch.
        await expect(candidate.locator('summary').filter({ hasText: 'Switch to this book' })).toBeVisible();
        await expect(candidate.getByRole('link', { name: 'Review a word in this Book' })).toBeVisible();
      }
      if (situation.chooserGroup === 'Analysis in progress') {
        await expect(candidate.getByRole('link', { name: 'Review To Read books' })).toBeVisible();
      }
    });
  }
});

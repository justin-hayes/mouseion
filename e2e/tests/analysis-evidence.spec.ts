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
// chooser show for that situation, with the band the candidate's margin names.
const situations = [
  {
    name: 'stale analysis',
    bookId: 'fixture-stale',
    title: 'Veraltete Analyse',
    evidence: 'Analysis out of date.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-stale/reanalyze' },
    chooserBand: 'Not assessed',
    chooserDescription: 'The analysis no longer matches the current book content. Retry analysis to refresh its evidence.',
    chooserRetry: true,
  },
  {
    name: 'cancelled analysis',
    bookId: 'fixture-cancelled',
    title: 'Abgebrochene Analyse',
    evidence: 'Analysis cancelled.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-cancelled/reanalyze' },
    chooserBand: 'Not assessed',
    chooserDescription: 'The last analysis did not complete. Retry analysis to refresh its evidence.',
    chooserRetry: true,
  },
  {
    name: 'publication pending',
    bookId: 'fixture-publication-pending',
    title: 'Veröffentlichung ausstehend',
    evidence: 'Analysis in progress.',
    recovery: undefined,
    chooserBand: 'Analysis in progress',
    chooserDescription: 'Analysis finished and its result is being published.',
    chooserRetry: false,
  },
  {
    name: 're-analysis running over a published analysis',
    bookId: 'fixture-reanalysis-running',
    title: 'Neue Analyse läuft',
    evidence: 'Re-analysis running. Coverage updating.',
    recovery: undefined,
    chooserBand: 'Below 95%',
    chooserDescription: undefined,
    chooserRetry: false,
  },
  {
    name: 're-analysis failed over a published analysis',
    bookId: 'fixture-reanalysis-failed',
    title: 'Neue Analyse fehlgeschlagen',
    evidence: 'Re-analysis failed. Coverage updating.',
    recovery: { label: 'Retry analysis', action: '/reading/books/fixture-reanalysis-failed/reanalyze' },
    chooserBand: 'Below 95%',
    chooserDescription: undefined,
    chooserRetry: false,
  },
];

test.describe('Analysis evidence situations', () => {
  for (const situation of situations) {
    test(`My Books presents ${situation.name}`, async ({ page }) => {
      await signIn(page);

      await page.goto('/de/library?disposition=to_read');
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

      // Books with a published analysis offer Read next from their row; a newer
      // run's recovery stays on the row, never on the pinned Current reading.
      await expect(row.getByRole('button', { name: 'Read next' })).toHaveCount(situation.chooserBand === 'Below 95%' ? 1 : 0);
    });
  }

  // Runs last: submitting the retry starts a new run for this non-current Book.
  test('submitting the retry for a non-current re-analysis failure succeeds', async ({ page }) => {
    await signIn(page);

    await page.goto('/de/library?disposition=to_read');
    const row = page.locator('#book-row-fixture-reanalysis-failed');
    await row.getByRole('button', { name: 'Retry analysis', exact: true }).click();
    await expect(page).toHaveURL(/\/library\?message=Analysis/);
  });
});

import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server is shared by every project/worker, and other specs mutate
// it (e.g. keyboard-focus completes the active learning campaign). This spec
// therefore only asserts Goal facts that are stable across those mutations and
// never changes the Goal: the goal book identity, the Clear affordance (present
// directly or behind the residual-work confirmation), eligible To Read
// controls, and the My Books disposition/link. The change/clear/reading-only/
// residual transitions are covered by Go unit and integration tests against
// isolated databases.
test.describe('Current reading selection', () => {
    test('current-reading confirmations carry stale-write guards and distinct consequences', async ({ page }) => {
      await signIn(page);
      await page.getByLabel('Study language').selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
      await page.goto('/reading');

      const current = page.locator('#primary-goal-section');
      const stop = current.locator('details').filter({ hasText: 'Stop reading for now' });
      const setAside = current.locator('details').filter({ hasText: 'Set aside this Book' });
      await expect(stop).toContainText('The Book remains To Read');
      await expect(setAside).toContainText('moves the Book to Set Aside');
      for (const [confirmation, button] of [[stop, 'Confirm stop for now'], [setAside, 'Confirm set aside']] as const) {
        await confirmation.locator('summary').focus();
        await expect(confirmation.locator('summary')).toBeFocused();
        await confirmation.locator('summary').press('Enter');
        const form = confirmation.locator('form');
        await expect(form.locator('input[name="expected_current_book_id"]')).toHaveValue('fixture-book');
        await expect(form.locator('input[name="expected_current_snapshot_id"]')).not.toHaveValue('');
        await expect(form.getByRole('button', { name: button })).toBeVisible();
        await confirmation.locator('summary').press('Enter');
      }

      await expect(current.locator('a[href="/reading/switch"]')).toBeVisible();
    });

    test('Reading and My Books expose truthful current-reading controls', async ({ page }) => {
      await signIn(page);
      const switcher = page.getByLabel('Study language');
      if (await switcher.inputValue() !== 'de') {
        await switcher.selectOption('de');
        await expect(page).toHaveURL(/\/library$/);
      }
      await page.goto('/reading');

    const goal = page.locator('#primary-goal-section');
      await expect(goal).toContainText('Der lange Weg nach Hause');
      await expect(goal).toContainText('By Mara Weiss');
      expect(await goal.locator('.journey-book__identity').evaluate((identity) => {
        const title = identity.querySelector('h3');
        const author = identity.querySelector('.journey-book__author');
        return title !== null && author !== null && Boolean(title.compareDocumentPosition(author) & Node.DOCUMENT_POSITION_FOLLOWING);
      })).toBe(true);
      await expect(goal).toContainText('Current commitment');
      await expect(goal).toContainText('Reading state');
      await expect(goal).toContainText('Not yet marked finished');
    // The goal card always offers a Clear form (directly, or via the
    // residual-work confirmation when an active campaign still reserves
    // vocabulary). It never offers to re-choose itself.
     await expect(goal.locator('form[action="/goal/clear"]')).toHaveCount(1);
     await expect(goal.getByRole('button', { name: 'Start reading' })).toHaveCount(0);
      await expect(goal).toContainText('Deck preparation');
      await expect(goal).toContainText('Deck ready');
      await expect(goal.getByRole('button', { name: 'Download deck' })).toHaveAttribute('download', '');
      await expect(goal).toContainText('Reserved vocabulary: 2 frozen identities.');
     await expect(goal.locator('input[name="external_translation_consent"]')).toHaveCount(0);

    const provisional = page.locator('#provisional-journey-list .journey-list > li');
    // Membership can grow across the shared fixture suite (e.g. a deck-flow test
    // adds a book), so assert structurally instead of by exact count.
    await expect(provisional.first()).toBeVisible();
     await expect(provisional.filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
     await expect(provisional.filter({ hasText: 'Donaudampfschifffahrtsgesellschaftskapitänsmütze' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route evidence pending' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route match: familiar German' })).toContainText('By Anja Roth');
      await expect(provisional.filter({ hasText: 'Route evidence pending' }).locator('.journey-book__author')).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route match: familiar German' }).getByRole('button', { name: 'Start reading' })).toBeVisible();
      await expect(provisional.filter({ hasText: 'Route differs: new German' }).getByRole('button', { name: 'Start reading' })).toBeVisible();

      const eligible = provisional.filter({ hasText: 'Route match: familiar German' });
      const moreActions = eligible.locator('details.more-actions');
      await expect(moreActions).toBeVisible();
      await expect(moreActions).not.toHaveAttribute('open', '');
      await expect(moreActions.getByRole('button', { name: 'Confirm removal' })).toBeHidden();
      await moreActions.locator(':scope > summary').click();
      await moreActions.locator('.confirmation > summary').click();
      await expect(moreActions.getByRole('button', { name: 'Confirm removal' })).toBeVisible();

      const ineligible = provisional.filter({ hasText: 'Route evidence pending' });
      await expect(ineligible).toContainText('cannot be started');
      await expect(ineligible.getByRole('button', { name: 'Start reading' })).toHaveCount(0);
      await expect(ineligible.getByRole('button', { name: 'Retry acquisition' })).toBeVisible();

      await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);

      await expect(page.getByRole('button', { name: 'Add books from My Books' })).toHaveAttribute('href', '/library');

      const retryAcquisition = ineligible.getByRole('button', { name: 'Retry acquisition' });
      const retryForm = retryAcquisition.locator('xpath=ancestor::form');
      const retryResponse = await page.request.post(new URL(await retryForm.getAttribute('action') ?? '', page.url()).toString(), {
        maxRedirects: 0,
        form: { csrf_token: await retryForm.locator('input[name="csrf_token"]').inputValue() },
      });
      expect(retryResponse.status()).toBe(303);

      await page.goto('/library');
      const goalBook = page.locator('.library-grid .library-book').filter({ hasText: 'Der lange Weg nach Hause' });
      await expect(goalBook.locator('.library-book__membership')).toHaveText(/To Read/);
      await expect(goalBook.getByRole('link', { name: 'View in Reading' })).toHaveAttribute('href', '/reading#journey-book-fixture-book');
      await expect(goalBook.getByText('Current reading')).toHaveCount(0);
      await expect(page.locator('.library-grid .library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
  });

  test('clearing current reading returns to the language-specific chooser', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture Goal runs once per browser suite.');
    await signIn(page);
    await page.getByLabel('Study language').selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/reading');
    const currentReading = page.locator('#primary-goal-section');
    if (await currentReading.count()) {
      const goalMoreActions = currentReading.locator('details.more-actions');
      await goalMoreActions.locator(':scope > summary').click();
      await goalMoreActions.locator('.confirmation > summary').first().click();
      await currentReading.locator('form[action="/goal/clear"] button').click();
      await expect(page).toHaveURL(/\/reading\?message=/);
      await expect(currentReading).toHaveCount(0);
    }
    await expect(page.getByRole('heading', { name: 'Choose your next book in Italian', exact: true })).toBeVisible();
    await expect(page.getByLabel('Study language')).toHaveValue('it');
  });
});

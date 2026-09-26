import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

// The fixture server is shared by every project/worker, and other specs mutate
// it (e.g. keyboard-focus completes a current reading). This spec asserts facts
// stable across those mutations and never changes the current reading: its
// identity, canonical controls, switch confirmation, and My Books disposition.
// Reading lifecycle mutations are covered by Go tests against isolated stores.
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

      const staleStopForm = stop.locator('form');
      const staleResponse = await page.request.post(new URL('/reading/stop', page.url()).toString(), {
        maxRedirects: 0,
        form: {
          csrf_token: await staleStopForm.locator('input[name="csrf_token"]').inputValue(),
          expected_current_book_id: 'stale-book-id',
          expected_current_snapshot_id: await staleStopForm.locator('input[name="expected_current_snapshot_id"]').inputValue(),
        },
      });
      expect(staleResponse.status()).toBe(303);
      const staleLocation = staleResponse.headers().location;
      expect(staleLocation).toContain('error=');
      await page.goto(staleLocation);
      await expect(page.getByRole('alert')).toContainText('No changes were made');
      await page.goto('/reading');
      await expect(page.locator('#primary-goal-section')).toContainText('Der lange Weg nach Hause');

      await expect(current.locator('a[href="/reading/switch"]')).toBeVisible();
      await page.goto('/reading/switch');
      const switchCandidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Route match: familiar German' });
      const switchDisclosure = switchCandidate.locator('details').filter({ hasText: 'Switch to this book' });
      await switchDisclosure.locator('summary').click();
      const switchForm = switchDisclosure.locator('form');
      await expect(switchForm.locator('input[name="expected_current_book_id"]')).toHaveValue('fixture-book');
      await expect(switchForm.locator('input[name="expected_current_snapshot_id"]')).not.toHaveValue('');
      await expect(switchForm.getByRole('button', { name: 'Confirm switch to this book' })).toBeVisible();
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
    // Lifecycle controls are canonical Reading mutations; retired Goal forms
    // are not present on the page.
     await expect(goal.locator('form[action="/goal/clear"], form[action="/goal/finish"]')).toHaveCount(0);
     await expect(goal.locator('form[action="/reading/finish"] input[name="expected_current_snapshot_id"]')).toHaveCount(1);
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
       await expect(provisional.filter({ hasText: 'Route match: familiar German' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
       await expect(provisional.filter({ hasText: 'Route differs: new German' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);

       const eligible = provisional.filter({ hasText: 'Route match: familiar German' });
       const moreActions = eligible.locator('details.more-actions');
       await expect(moreActions).toBeVisible();
       await expect(moreActions).not.toHaveAttribute('open', '');
       await expect(provisional.getByRole('button', { name: 'Confirm removal' })).toHaveCount(0);
       await moreActions.locator(':scope > summary').click();
       await expect(moreActions.getByRole('link', { name: 'My Books' })).toHaveAttribute('href', '/library');
       const currentSetAside = goal.locator('details').filter({ hasText: 'Set aside this Book' });
       await currentSetAside.locator('summary').click();
       await expect(currentSetAside.getByRole('button', { name: 'Confirm set aside' })).toBeVisible();

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

   test('stop and set-aside mutations return to truthful Reading state', async ({ page }) => {
     test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture workflow runs once per browser suite.');
     await signIn(page);
     await page.getByLabel('Study language').selectOption('it');
     await expect(page).toHaveURL(/\/library$/);
     await page.goto('/reading');

     const currentCard = page.locator('.journey-book--goal');
     if (await currentCard.count() === 0) {
       const initialCandidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Italian route baseline' });
       const initialStart = initialCandidate.locator('details').filter({ hasText: 'Start reading' });
       await initialStart.locator('summary').click();
       await initialStart.getByRole('button', { name: 'Confirm start reading' }).click();
       await expect(page).toHaveURL(/\/reading\?message=/);
     }
     await expect(currentCard).toBeVisible();
     const title = (await currentCard.getByRole('heading', { level: 3 }).textContent())?.trim() ?? '';
     expect(title).toBeTruthy();

     const stop = currentCard.locator('details').filter({ hasText: 'Stop reading for now' });
     await stop.locator('summary').click();
     await stop.getByRole('button', { name: 'Confirm stop for now' }).click();
     await expect(page).toHaveURL(/\/reading\?message=/);
     await expect(page.locator('.journey-book--goal')).toHaveCount(0);

     const startBook = async (bookTitle: string) => {
       await page.goto('/reading');
       const candidate = page.locator('li.reading-chooser-book').filter({ has: page.getByRole('heading', { name: bookTitle, exact: true }) });
       await expect(candidate).toBeVisible();
       const start = candidate.locator('details').filter({ hasText: 'Start reading' });
       await start.locator('summary').click();
       await start.getByRole('button', { name: 'Confirm start reading' }).click();
       await expect(page).toHaveURL(/\/reading\?message=/);
       await expect(page.locator('.journey-book--goal')).toContainText(bookTitle);
     };

     await startBook(title);
     const setAside = page.locator('.journey-book--goal details').filter({ hasText: 'Set aside this Book' });
     await setAside.locator('summary').click();
     await setAside.getByRole('button', { name: 'Confirm set aside' }).click();
     await expect(page).toHaveURL(/\/reading\?message=/);
     await expect(page.locator('.journey-book--goal')).toHaveCount(0);

     await page.goto('/library');
     const setAsideBook = page.locator('.library-grid .library-book').filter({ hasText: title });
     await expect(setAsideBook.locator('.metadata').filter({ hasText: 'Workflow' })).toContainText('Set Aside');
     await setAsideBook.getByRole('button', { name: 'Move to To Read' }).click();
     await startBook(title);
   });

});

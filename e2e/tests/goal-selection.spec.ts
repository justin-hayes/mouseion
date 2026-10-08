import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/library/);
}

// Compact Reading collapses the analysis, reservation, and preparation details.
// Open them before asserting on their contents; desktop shows them already.
async function openSupportingDetails(page: Page) {
  const summary = page.locator('details.reading-supporting > summary');
  if (await summary.isVisible()) await summary.click();
}

// Stateful flows remain ordered within this file and get a fresh fixture store
// for each project/worker assignment.
test.describe('Current reading selection', () => {
  test('current reading opens its focused prepared-deck task', async ({ page }) => {
    await signIn(page);
    const language = page.getByLabel('Study language');
    if (await language.inputValue() !== 'de') {
      await language.selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
    }
    await page.goto('/reading');

    const currentCard = page.locator('.journey-book--goal');
    if (await currentCard.count() === 0) {
      const candidate = page.locator('li.reading-chooser-book').filter({ hasText: 'Der lange Weg nach Hause' });
      const start = candidate.locator('details').filter({ hasText: 'Start reading' });
      await start.locator('summary').click();
      await start.getByRole('button', { name: 'Confirm start reading' }).click();
      await expect(page).toHaveURL(/\/reading\?message=/);
    }
    await expect(currentCard).toBeVisible();
    await openSupportingDetails(page);
    await currentCard.getByRole('link', { name: 'Open focused deck task' }).click();
    await expect(page).toHaveURL(/\/reading\/books\/[^/]+\/deck\/preparations\/new$/);
    await expect(page.getByRole('heading', { name: 'Deck preparation task' })).toBeVisible();
    await expect(page.getByText('Frozen reading snapshot', { exact: true })).toBeVisible();
    await expect(page.locator('input[name="external_translation_consent"]')).toHaveCount(0);
  });

  test('current-reading confirmations carry stale-write guards and distinct consequences', async ({ page }) => {
      await signIn(page);
      await page.getByLabel('Study language').selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
      await page.goto('/reading');

      const current = page.locator('#primary-goal-section');
      const stop = current.locator('details').filter({ hasText: 'End current reading' });
      await expect(stop).toContainText('The Book stays in To Read');
      await expect(current.locator('details').filter({ hasText: 'Set aside this Book' })).toHaveCount(0);
      for (const [confirmation, button] of [[stop, 'Confirm end current reading']] as const) {
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
      const staleResponse = await page.request.post(new URL('/reading/end', page.url()).toString(), {
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
      await page.setViewportSize({ width: 1280, height: 800 });
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
      await expect(goal.getByRole('heading', { name: 'Der lange Weg nach Hause', level: 1 })).toBeVisible();
      const titleLink = goal.getByRole('heading', { level: 1 }).getByRole('link', { name: 'Der lange Weg nach Hause' });
      await expect(titleLink).toHaveCSS('text-decoration-line', 'none');
      await titleLink.hover();
      await expect(titleLink).toHaveCSS('text-decoration-line', 'underline');
      await page.mouse.move(0, 0);
      await page.evaluate(() => document.activeElement instanceof HTMLElement && document.activeElement.blur());
      let titleReceivedKeyboardFocus = false;
      for (let tab = 0; tab < 30; tab++) {
        await page.keyboard.press('Tab');
        if (await titleLink.evaluate((link) => link === document.activeElement)) {
          titleReceivedKeyboardFocus = true;
          break;
        }
      }
      expect(titleReceivedKeyboardFocus).toBe(true);
      await expect(titleLink).toHaveCSS('text-decoration-line', 'underline');
      await expect(goal.getByRole('heading', { name: 'Reserved vocabulary' })).toBeVisible();
      await expect(goal.getByRole('heading', { name: 'Analysis' })).toBeVisible();
      await expect(goal.getByRole('heading', { name: 'Book deck' })).toBeVisible();
      await expect(goal.getByRole('link', { name: 'Review flagged dictionary forms' })).toHaveAttribute('href', /\/reading\/books\/[^/]+\/lemma-review$/);
      await expect(goal.locator('.journey-book__evidence')).not.toContainText(/immutable artifact|frozen identities|remain independent facts/i);
      const titlePageItems = await goal.locator('.journey-book__cover, .journey-book__title, .journey-book__author, .journey-book__lifecycle').evaluateAll((nodes) => nodes.map((node) => {
        const rect = node.getBoundingClientRect();
        return { bottom: rect.bottom, width: rect.width };
      }));
      for (const item of titlePageItems) {
        expect(item.bottom).toBeLessThanOrEqual(800);
      }
      expect(titlePageItems[0].width).toBeGreaterThanOrEqual(160);
      expect(await goal.locator('.journey-book__identity').evaluate((identity) => {
        const title = identity.querySelector('h1');
        const author = identity.querySelector('.journey-book__author');
        return title !== null && author !== null && Boolean(title.compareDocumentPosition(author) & Node.DOCUMENT_POSITION_FOLLOWING);
      })).toBe(true);
      await expect(goal.locator('.journey-book__lifecycle')).toBeVisible();
    // Lifecycle controls are canonical Reading mutations; retired Goal forms
    // are not present on the page.
     await expect(goal.locator('form[action="/goal/clear"], form[action="/goal/finish"]')).toHaveCount(0);
     await expect(goal.locator('form[action="/reading/finish"] input[name="expected_current_snapshot_id"]')).toHaveCount(1);
     await expect(goal.getByRole('button', { name: 'Start reading' })).toHaveCount(0);
      await expect(goal).toContainText('Book deck');
      await expect(goal).toContainText('Deck ready');
      await expect(goal.getByRole('link', { name: 'Download deck' })).toHaveAttribute('download', '');
      await expect(goal.locator('.reading-note').filter({ hasText: '2 lemmas' })).toContainText('2 lemmas');
     await expect(goal.locator('input[name="external_translation_consent"]')).toHaveCount(0);

    // While a Book is current, Reading has no Other To Read section; the
    // alternatives and their start or switch actions live in the chooser.
    await expect(page.locator('#provisional-journey-heading')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Other To Read books', exact: true })).toHaveCount(0);
    await expect(goal.getByRole('button', { name: 'Start reading' })).toHaveCount(0);
    await expect(page.getByRole('link', { name: 'Add books from My Books' })).toHaveAttribute('href', '/library');
    await expect(page.getByRole('region', { name: 'Reading coverage forecast' })).toHaveCount(0);
    const currentEnd = goal.locator('details').filter({ hasText: 'End current reading' });
    await currentEnd.locator('summary').click();
    await expect(currentEnd.getByRole('button', { name: 'Confirm end current reading' })).toBeVisible();

    await page.goto('/reading/switch');
    const chooser = page.locator('li.reading-chooser-book');
    // Membership can grow within the stateful sequence in this file, so assert
    // structurally instead of by exact count.
    await expect(chooser.first()).toBeVisible();
    await expect(chooser.locator('details').filter({ hasText: 'Start reading' })).toHaveCount(0);
    await expect(chooser.filter({ hasText: 'Empty chapter' }).locator('details').filter({ hasText: 'Switch to this book' })).toHaveCount(0);
    const eligible = chooser.filter({ hasText: 'Route match: familiar German' });
    await expect(eligible).toContainText('By Anja Roth');
    await expect(eligible.locator('details').filter({ hasText: 'Switch to this book' })).toHaveCount(1);

    const ineligible = chooser.filter({ hasText: 'Route evidence pending' });
    await expect(ineligible).toContainText('Route evidence pending');
    await expect(ineligible.locator('details').filter({ hasText: 'Switch to this book' })).toHaveCount(0);
    await expect(ineligible.getByRole('button', { name: 'Retry acquisition' })).toBeVisible();

      const retryAcquisition = ineligible.getByRole('button', { name: 'Retry acquisition' });
      const retryForm = retryAcquisition.locator('xpath=ancestor::form');
      const retryResponse = await page.request.post(new URL(await retryForm.getAttribute('action') ?? '', page.url()).toString(), {
        maxRedirects: 0,
        form: { csrf_token: await retryForm.locator('input[name="csrf_token"]').inputValue() },
      });
      expect(retryResponse.status()).toBe(303);

      await page.goto('/library');
      const goalBook = page.locator('.library-books .library-book').filter({ hasText: 'Der lange Weg nach Hause' });
      await expect(goalBook.locator('.library-book__membership')).toHaveText('Currently reading');
      await expect(goalBook.getByRole('link', { name: 'View in Reading' })).toHaveAttribute('href', '/reading#journey-book-fixture-book');
      await expect(goalBook.getByText('To Read', { exact: true })).toHaveCount(0);
      await page.goto('/library?disposition=to_read');
      await expect(page.getByRole('link', { name: 'To Read (9)' })).toBeVisible();
      await expect(page.locator('.library-books .library-book').filter({ hasText: 'Der lange Weg nach Hause' }).locator('.library-book__membership')).toHaveText('Currently reading');
      await expect(page.locator('.library-books .library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Start reading' })).toHaveCount(0);
   });

   test('end mutations return to truthful Reading state', async ({ page }) => {
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
      const title = (await currentCard.getByRole('heading', { level: 1 }).locator('a').textContent())?.trim() ?? '';
      expect(title).toBeTruthy();

      await openSupportingDetails(page);
      await expect(currentCard.getByRole('link', { name: 'Open focused deck task' }))
        .toHaveAttribute('href', /\/reading\/books\/[^/]+\/deck\/preparations\/new$/);

      const stop = currentCard.locator('details').filter({ hasText: 'End current reading' });
      await stop.locator('summary').click();
      await stop.getByRole('button', { name: 'Confirm end current reading' }).click();
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
     const endAgain = page.locator('.journey-book--goal details').filter({ hasText: 'End current reading' });
     await endAgain.locator('summary').click();
     await endAgain.getByRole('button', { name: 'Confirm end current reading' }).click();
     await expect(page).toHaveURL(/\/reading\?message=/);
     await expect(page.locator('.journey-book--goal')).toHaveCount(0);

     await page.goto('/library');
     const endedBook = page.locator('.library-books .library-book').filter({ hasText: title });
     await expect(endedBook.locator('.library-book__membership .status-badge')).toHaveText('To Read');
     await startBook(title);
   });

});

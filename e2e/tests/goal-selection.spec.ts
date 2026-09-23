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
// directly or behind the residual-work confirmation), eligible provisional
// controls, and the My Books Journey membership/link. The change/clear/reading-only/
// residual transitions are covered by Go unit and integration tests against
// isolated databases.
test.describe('Primary Goal selection', () => {
    test('Journey and My Books expose truthful Goal controls', async ({ page }) => {
      await signIn(page);
      const switcher = page.getByLabel('Study language');
      if (await switcher.inputValue() !== 'de') {
        await switcher.selectOption('de');
        await expect(page).toHaveURL(/\/library$/);
      }
      await page.goto('/journey');

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
     await expect(goal.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
     await expect(goal).toContainText('Goal deck preparation');
      await expect(goal).toContainText('Deck ready');
      await expect(goal.getByRole('button', { name: 'Download Goal deck' })).toHaveAttribute('download', '');
      await expect(goal).toContainText('Reserved vocabulary: 2 frozen identities.');
     await expect(goal.locator('input[name="external_translation_consent"]')).toHaveCount(0);

    const provisional = page.locator('#provisional-journey-list .journey-list > li');
    // Membership can grow across the shared fixture suite (e.g. a deck-flow test
    // adds a book), so assert structurally instead of by exact count.
    await expect(provisional.first()).toBeVisible();
     await expect(provisional.filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
     await expect(provisional.filter({ hasText: 'Donaudampfschifffahrtsgesellschaftskapitänsmütze' }).getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route evidence pending' }).getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route match: familiar German' })).toContainText('By Anja Roth');
      await expect(provisional.filter({ hasText: 'Route evidence pending' }).locator('.journey-book__author')).toHaveCount(0);
      await expect(provisional.filter({ hasText: 'Route match: familiar German' }).getByRole('button', { name: 'Choose as Primary Goal' })).toBeVisible();
      await expect(provisional.filter({ hasText: 'Route differs: new German' }).getByRole('button', { name: 'Choose as Primary Goal' })).toBeVisible();

      const eligible = provisional.filter({ hasText: 'Route match: familiar German' });
      const moreActions = eligible.locator('details.more-actions');
      await expect(moreActions).toBeVisible();
      await expect(moreActions).not.toHaveAttribute('open', '');
      await expect(moreActions.getByRole('button', { name: 'Confirm removal' })).toBeHidden();
      await moreActions.locator(':scope > summary').click();
      await moreActions.locator('.confirmation > summary').click();
      await expect(moreActions.getByRole('button', { name: 'Confirm removal' })).toBeVisible();

      const ineligible = provisional.filter({ hasText: 'Route evidence pending' });
      await expect(ineligible).toContainText('cannot become a Primary Goal');
      await expect(ineligible.getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
      await expect(ineligible.getByRole('button', { name: 'Retry acquisition' })).toBeVisible();

      const goalForecast = goal.getByRole('region', { name: 'Journey coverage forecast' });
      await expect(goalForecast.locator('.journey-forecast__stage')).toHaveCount(2);
      await expect(goalForecast).toContainText('Current coverage');
      await expect(goalForecast).toContainText('After Primary Goal');
      await expect(goalForecast).toContainText('After completion of this Primary Goal');
      await expect(goalForecast).not.toContainText('On arrival');

      const unchanged = provisional.filter({ hasText: 'Route match: familiar German' }).getByRole('region', { name: 'Journey coverage forecast' });
      await expect(unchanged).toContainText('90.0%');
      await expect(unchanged).toContainText('No change');
      await expect(unchanged.locator('details summary')).toHaveText('Evidence and calculation');

      const changed = provisional.filter({ hasText: 'Route differs: new German' }).getByRole('region', { name: 'Journey coverage forecast' });
      if (test.info().project.name === 'desktop-light') {
        await expect(changed).toContainText('25.0%');
        await expect(changed).toContainText('+5.0 percentage points');
      }

      const lowerBound = provisional.filter({ hasText: 'Route tie A' }).getByRole('region', { name: 'Journey coverage forecast' });
      await expect(lowerBound).toContainText('Lower bound');
      const unavailable = provisional.filter({ hasText: 'Route evidence pending' }).getByRole('region', { name: 'Journey coverage forecast' });
      await expect(unavailable).toContainText('Unavailable');
      await expect(unavailable).toContainText('completed analysis evidence is unavailable');

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
      await expect(goalBook.locator('.library-book__membership')).toHaveText(/In Reading Journey/);
      await expect(goalBook.getByRole('link', { name: 'View in Reading Journey' })).toHaveAttribute('href', '/journey#journey-book-fixture-book');
      await expect(goalBook.getByText('Current Primary Goal')).toHaveCount(0);
      await expect(page.locator('.library-grid .library-book').filter({ hasText: 'Empty chapter' }).getByRole('button', { name: 'Choose as Primary Goal' })).toHaveCount(0);
  });

  test('promotes and clears the active language Goal without touching another language', async ({ page }) => {
    test.skip(test.info().project.name !== 'desktop-light', 'This stateful fixture Goal runs once per browser suite.');
    await signIn(page);
    await page.getByLabel('Study language').selectOption('it');
    await expect(page).toHaveURL(/\/library$/);
    await page.goto('/journey');
    await expect(page.locator('#journey-book-fixture-italian-goal')).toBeVisible();

    const goalMoreActions = page.locator('#primary-goal-section details.more-actions');
    await goalMoreActions.locator(':scope > summary').click();
    await goalMoreActions.locator('.confirmation > summary').first().click();
    await page.locator('#primary-goal-section form[action="/goal/clear"] button').click();
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.locator('#primary-goal-section')).toContainText('No Primary Goal yet');
    await expect(page.locator('#provisional-journey-content .journey-forecast').first()).toContainText('No active Primary Goal');

    const noGoalUnchanged = page.locator('#provisional-journey-list .journey-list > li').filter({ hasText: 'Italian route baseline' }).getByRole('region', { name: 'Journey coverage forecast' });
    await expect(noGoalUnchanged).toContainText('No change');
    await expect(noGoalUnchanged).not.toContainText('Change from After Primary Goal coverage');

    const noGoalChanged = page.locator('#provisional-journey-list .journey-list > li').filter({ hasText: 'Una meta italiana' }).getByRole('region', { name: 'Journey coverage forecast' });
    await expect(noGoalChanged).toContainText('Change from Current coverage: +5.0 percentage points');
    await expect(noGoalChanged).not.toContainText('Change from After Primary Goal coverage');

    await page.locator('#journey-book-fixture-italian-goal').getByRole('button', { name: 'Choose as Primary Goal' }).click();
    await expect(page).toHaveURL(/\/journey\?message=/);
    await expect(page.locator('#primary-goal-section')).toContainText('Una meta italiana');

    await page.getByLabel('Study language').selectOption('de');
    await expect(page).toHaveURL(/\/journey(?:\?|$)/);
    await expect(page.locator('#primary-goal-section')).toContainText('Der lange Weg nach Hause');
    await expect(page.locator('#primary-goal-section')).not.toContainText('Una meta italiana');
  });
});

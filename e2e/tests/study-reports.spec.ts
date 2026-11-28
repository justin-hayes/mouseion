import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/reading/);
}

// The fixture server for this spec has the usual three new words on offer and
// is shared by the tests of this file, which may run in any order. Every test
// sets up its own state with openRevealedCard: it takes the batch when it is
// still on offer and reveals the answer of the card that is due. The reports of
// one test never use up what another needs: a test reports at most one word, and
// only the Wrong-word test leaves a word held.
async function openRevealedCard(page: Page) {
  await page.goto('/de/study');
  const take = page.getByRole('button', { name: 'Take these words' });
  if (await take.count()) await take.click();
  await page.getByRole('button', { name: 'Show answer' }).click();
}

test.describe('Study reports', () => {
  test('reports a wrong word after Show answer, goes on to the next card and lists the report', async ({ page }) => {
    await signIn(page);
    await page.goto('/de/study');
    const take = page.getByRole('button', { name: 'Take these words' });
    if (await take.count()) await take.click();

    // Report waits for Show answer.
    await expect(page.locator('#study-report')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();
    const reported = await page.locator('#study-headword').innerText();

    // Both kinds are shown, and Wrong word is the default.
    await page.locator('#study-report summary').click();
    await expect(page.getByRole('radio', { name: /Wrong word/ })).toBeChecked();
    await expect(page.getByRole('radio', { name: /Unsuitable context/ })).toBeEnabled();
    await page.getByLabel('Note (optional)').fill('The highlighted word is another word here.');
    await page.getByRole('button', { name: 'Send report' }).click();

    // The next card appears, and the reported word is held.
    await expect(page.getByRole('heading', { level: 2, name: 'New word', exact: true })).toBeVisible();
    await expect(page.locator('#study-held')).toContainText('1 word is held');
    await page.getByRole('button', { name: 'Show answer' }).click();
    expect(await page.locator('#study-headword').innerText()).not.toBe(reported);

    // Reported words lists it with its sentence and note.
    await page.getByRole('link', { name: 'Reported words' }).click();
    await expect(page).toHaveURL(/\/de\/study\/reports$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Reported words' })).toBeVisible();
    const entry = page.locator('#study-reports li');
    await expect(entry).toHaveCount(1);
    await expect(entry.getByRole('heading', { level: 2 })).toHaveText(reported);
    await expect(entry.locator('.study-card__target')).toBeVisible();
    await expect(entry).toContainText('The highlighted word is another word here.');
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);

    // The entry opens the remediation screen on the reported occurrence, not a
    // Book page.
    await entry.getByRole('link', { name: 'Repair this word' }).click();
    await expect(page).toHaveURL(/\/study\/reports\/[^/]+\/remediation$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Repair a reported word' })).toBeVisible();
    await expect(page.locator('.study-remediation')).toContainText('The highlighted word is another word here.');
    await expect(page.locator('.study-remediation .reading-text')).toHaveCount(1);
    // The Current reading's Book is repaired like any other; the preview and
    // confirmation are covered over HTTP against Postgres.
    await expect(page.getByRole('button', { name: 'Preview correction' })).toBeVisible();
    await page.goto('/de/study/reports');

    // Dismiss ends the report: the entry leaves the list and the word is no
    // longer held.
    await entry.getByRole('button', { name: 'Dismiss' }).click();
    await expect(page).toHaveURL(/\/de\/study\/reports$/);
    await expect(page.locator('#study-reports')).toHaveCount(0);
    await expect(page.locator('#study-reports-empty')).toBeVisible();
    await page.goto('/de/study');
    await expect(page.locator('#study-held')).toHaveCount(0);
  });

  test('reports an unsuitable context, sees the reported card once more and goes on with Next', async ({ page }) => {
    await signIn(page);
    await openRevealedCard(page);
    const reported = await page.locator('#study-headword').innerText();
    const sentence = await page.locator('.study-card__sentence').innerText();

    await page.locator('#study-report summary').click();
    await page.getByRole('radio', { name: /Unsuitable context/ }).check();
    await page.getByRole('button', { name: 'Send report' }).click();

    // The reported card stays on screen, with its answer side and a Next control.
    await expect(page.locator('#study-reported')).toContainText('Reported as an unsuitable context');
    await expect(page.locator('.study-card__sentence')).toHaveText(sentence);
    await expect(page.locator('#study-headword')).toHaveText(reported);

    // Next goes on to another word; the reported one is not held.
    const reportedUrl = page.url();
    await page.locator('#study-reported-next').click();
    await expect(page.locator('#study-reported')).toHaveCount(0);
    await page.getByRole('button', { name: 'Show answer' }).click();
    expect(await page.locator('#study-headword').innerText()).not.toBe(reported);

    // Moving on ended the offer: back on the reported card, Undo is gone.
    await page.goto(reportedUrl);
    await expect(page.locator('#study-reported-undo')).toHaveCount(0);
  });

  test('undoes an unsuitable-context report while its card is shown', async ({ page }) => {
    await signIn(page);
    await openRevealedCard(page);
    const reported = await page.locator('#study-headword').innerText();

    await page.locator('#study-report summary').click();
    await page.getByRole('radio', { name: /Unsuitable context/ }).check();
    await page.getByRole('button', { name: 'Send report' }).click();
    await expect(page.locator('#study-reported')).toBeVisible();

    await page.locator('#study-reported-undo').click();

    // The report has ended and the reported word's card is back.
    await expect(page.locator('#study-reported')).toHaveCount(0);
    await expect(page.locator('#study-headword')).toHaveText(reported);
  });

  test('a card with Report open fits a phone viewport and keeps its controls reachable', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await signIn(page);
    await openRevealedCard(page);
    await page.locator('#study-report summary').click();

    const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(await overflow()).toBeLessThanOrEqual(0);

    // Every control of the open Report form is inside the viewport's width and
    // can be scrolled to and used.
    for (const control of [
      page.getByRole('radio', { name: /Wrong word/ }),
      page.getByRole('radio', { name: /Unsuitable context/ }),
      page.getByLabel('Note (optional)'),
      page.getByRole('button', { name: 'Send report' }),
    ]) {
      await control.scrollIntoViewIfNeeded();
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(375);
    }

    // The reported card's Undo and Next stay reachable too.
    await page.getByRole('radio', { name: /Unsuitable context/ }).check();
    await page.getByRole('button', { name: 'Send report' }).click();
    await expect(page.locator('#study-reported')).toBeVisible();
    expect(await overflow()).toBeLessThanOrEqual(0);
    for (const control of [page.locator('#study-reported-undo'), page.locator('#study-reported-next')]) {
      await control.scrollIntoViewIfNeeded();
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(375);
    }
    await page.locator('#study-reported-undo').click();
    await expect(page.locator('#study-reported')).toHaveCount(0);
  });
});

import { expect, test } from '@playwright/test';

test('Custom deck review and editing work without JavaScript at 400% zoom', async ({ browser }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop-light', 'The fixture server shares mutable state across browser projects.');
  const page = await browser.newPage({
    baseURL: process.env.MOUSEION_FIXTURE_URL ?? 'http://127.0.0.1:8099',
    javaScriptEnabled: false,
    viewport: { width: 320, height: 812 },
  });
  try {
    await page.goto('/login');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: 'Sign in' }).press('Enter');
    await expect(page).toHaveURL(/\/library$/);
    const language = page.getByLabel('Study language');
    if (await language.inputValue() !== 'de') {
      await language.selectOption('de');
      await page.getByRole('button', { name: 'Switch language' }).click();
      await expect(page.getByLabel('Study language')).toHaveValue('de');
    }

    // The fixture has no analyzed Browse rows. Submit a Browse selection form
    // through the authenticated HTTP route, then exercise its browser UI.
    await page.goto('/vocabulary/selection/clear-confirm');
    const csrf = await page.locator('input[name="csrf_token"]').first().inputValue();
    const longLemma = 'donaudampfschifffahrtsgesellschaftskapitaensmuetze';
    for (const lemma of ['haus', longLemma]) {
      const added = await page.request.post('/vocabulary/selection/add', {
        form: { csrf_token: csrf, lemma, upos: 'NOUN' },
      });
      expect(added.ok()).toBeTruthy();
    }

    await page.goto('/vocabulary/selection');
    await expect(page.getByRole('heading', { name: 'Review Browse selection' })).toBeVisible();
    await expect(page.getByText('2 selected identities')).toBeVisible();
    await expect(page.getByText('2 currently lack evidence')).toBeVisible();
    await page.getByRole('link', { name: 'Show only identities missing current evidence' }).focus();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(/\/vocabulary\/selection\?missing=true$/);
    await expect(page.getByText(longLemma)).toBeVisible();
    await page.getByRole('link', { name: 'Show all selected identities' }).click();
    const longName = 'German cross-Book practice — ' + 'long-name-'.repeat(7);
    await page.getByLabel('Deck name').fill(longName);
    await page.getByRole('button', { name: 'Create Custom deck' }).press('Enter');
    await expect(page).toHaveURL(/\/vocabulary\/decks\/[^/?]+$/);
    const deckURL = new URL(page.url()).pathname;
    await expect(page.getByRole('heading', { name: `Custom deck · ${longName}` })).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: '2 selected identities' })).toContainText('2 missing current evidence');
    await expect(page.getByRole('button', { name: 'Prepare deck' })).toHaveCount(0);
    await expect(page.getByText('Preparation is unavailable because no selected identity has current eligible evidence.')).toBeVisible();
    await expect(page.getByRole('complementary', { name: 'Anki import and overlap warning' })).toContainText('Custom decks can overlap');
    expect(await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)).toBe(false);

    await page.getByLabel('Rename deck').fill('Renamed practice');
    await page.getByRole('button', { name: 'Save name' }).press('Enter');
    await expect(page.getByRole('heading', { name: 'Custom deck · Renamed practice' })).toBeVisible();
    await page.getByLabel('Add an identity').fill('unseen');
    await page.getByRole('button', { name: 'Add identity' }).press('Enter');
    await expect(page.getByRole('status').filter({ hasText: '3 selected identities' })).toContainText('3 missing current evidence');
    await page.getByRole('link', { name: 'Show only identities missing current evidence' }).click();
    await expect(page).toHaveURL(/\?missing=true/);
    await page.locator('ul li').filter({ hasText: 'unseen' }).getByRole('button', { name: 'Remove' }).click();
    await expect(page.getByRole('status').filter({ hasText: '2 selected identities' })).toContainText('2 missing current evidence');
    await expect(page.getByText('unseen', { exact: true })).toHaveCount(0);

    await page.goto(deckURL);
    await page.getByRole('button', { name: 'Delete Custom deck' }).click();
    await expect(page.getByRole('heading', { name: 'Delete Custom deck?' })).toBeVisible();
    await expect(page.getByText('Mouseion cannot revoke APKG files already downloaded', { exact: false })).toBeVisible();
    await page.getByRole('link', { name: 'Cancel and keep deck' }).click();
    await expect(page.getByRole('heading', { name: 'Custom deck · Renamed practice' })).toBeVisible();
    await page.getByRole('button', { name: 'Delete Custom deck' }).click();
    await page.getByRole('button', { name: 'Confirm delete Custom deck' }).press('Enter');
    await expect(page).toHaveURL(/\/vocabulary\/selection$/);
    await expect(page.getByText('No saved Custom decks yet.')).toBeVisible();

    const selectedAgain = await page.request.post('/vocabulary/selection/add', {
      form: { csrf_token: csrf, lemma: 'clear-me', upos: 'NOUN' },
    });
    expect(selectedAgain.ok()).toBeTruthy();
    await page.goto('/vocabulary/selection');
    await page.getByRole('button', { name: 'Clear selection' }).click();
    await expect(page.getByRole('heading', { name: 'Clear Browse selection?' })).toBeVisible();
    await page.getByRole('link', { name: 'Cancel and keep selection' }).click();
    await expect(page.getByText('1 selected identities')).toBeVisible();
    await page.getByRole('button', { name: 'Clear selection' }).click();
    await page.getByRole('button', { name: 'Confirm clear selection' }).press('Enter');
    await expect(page.getByText('0 selected identities')).toBeVisible();
  } finally {
    await page.close();
  }
});

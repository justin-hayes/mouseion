import { expect, test } from '@playwright/test';

test('exact-form occurrence review is usable without JavaScript', async ({ browser }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop-light', 'The fixture server is shared across browser projects.');
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();
  try {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/login');
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
    await expect(page).toHaveURL(/\/library/);
    const language = page.getByLabel('Study language');
    if (await language.inputValue() !== 'de') {
      await language.selectOption('de');
      await page.getByRole('button', { name: 'Switch language' }).click();
      await expect(page.getByLabel('Study language')).toHaveValue('de');
    }
    await page.goto('/reading/books/fixture-book/lemma-review?form=Weg');
    await expect(page.getByRole('heading', { name: "Stop before changing this Book's vocabulary" })).toBeVisible();
    await expect(page.getByText(/stop this reading without marking it finished/i)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Open Reading to stop this Book' })).toHaveAttribute('href', '/reading');
    await expect(page.getByRole('heading', { name: 'A ready deck is a historical artifact' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Download the existing deck' })).toBeVisible();

    await page.goto('/reading/books/fixture-lemma-flag-book/lemma-review');
    await expect(page.getByText(/This is not a clean verdict; manual occurrence review remains available\./)).toBeVisible();
    await expect(page.getByText(/Needs learner review — not a verdict\./)).toBeVisible();
    await expect(page.getByText('Der Weg führt zum Haus.').first()).toBeVisible();
    await expect(page.getByText(/Local-index alternative: pfad/)).toBeVisible();
    await expect(page.getByRole('link', { name: 'Review exact form “Weg”' })).toHaveAttribute('href', /form=Weg/);
    await page.getByLabel('Exact observed form').fill('Weg');
    const findOccurrences = page.getByRole('button', { name: 'Find occurrences' });
    await findOccurrences.click();
    await expect(page.getByText('Der Weg führt zum Haus.').first()).toBeVisible();
    await page.goto('/reading/books/fixture-route-match/lemma-review?form=Weg');
    await expect(page.getByRole('button', { name: 'Preview correction' })).toHaveCount(2);
    await page.getByRole('button', { name: 'Ask for an optional LLM lemma suggestion' }).first().click();
    await expect(page.getByText(/LLM lemma suggestion — not applied: Pfad/)).toBeVisible();
    await expect(page.getByText(/Provider: fixture-lemma-model · Version: fixture-v1/)).toBeVisible();
    await expect(page.getByText(/Analyzer lemma: weg/)).toHaveCount(2);
    await expect(page.getByRole('button', { name: 'Preview correction' })).toHaveCount(2);

    await page.goto('/reading/books/fixture-lemma-flag-book/lemma-review?form=Weg');
    await page.getByRole('button', { name: 'Ask for an optional LLM lemma suggestion' }).first().click();
    await expect(page.getByText(/A lemma suggestion is unavailable right now\./)).toBeVisible();
    await expect(page.getByRole('button', { name: 'Preview correction' })).toHaveCount(2);
    await expect(page.getByRole('button', { name: 'Preview keeping analyzer lemma' })).toHaveCount(2);

    await page.goto('/reading/books/fixture-lemma-flag-book/lemma-review?form=Weg');
    await expect(page.getByLabel('Exact observed form')).toHaveClass(/\binput\b/);
    await expect(page.getByLabel('Corrected canonical lemma').first()).toHaveClass(/\binput\b/);
    const additionalOccurrence = page.locator('.lemma-review fieldset label').first();
    const additionalOccurrenceBounds = await additionalOccurrence.boundingBox();
    expect(additionalOccurrenceBounds?.height).toBeGreaterThanOrEqual(44);
    await page.getByRole('checkbox').first().check();
    await page.getByLabel('Corrected canonical lemma').first().fill('Pfad');
    await page.getByRole('button', { name: 'Preview correction' }).first().focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Review the proposed change' })).toBeVisible();
    await expect(page.getByText('This is a preview only. Nothing changes until you confirm.')).toBeVisible();
    await page.getByRole('button', { name: 'Apply correction to selected occurrences' }).click();
    await expect(page.getByText('Effective lemma: pfad (corrected)')).toHaveCount(2);
    await expect(page.getByText(/Previously reviewed: correct/)).toBeVisible();

    const exclusionDisclosure = page.locator('details summary').first();
    await exclusionDisclosure.focus();
    await page.keyboard.press('Enter');
    const excludeButton = page.getByRole('button', { name: 'Preview exclusion' }).first();
    await excludeButton.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Review the proposed change' })).toBeVisible();
    await expect(page.getByText('This is a preview only. Nothing changes until you confirm.')).toBeVisible();
    await page.getByRole('button', { name: 'Exclude selected occurrences from vocabulary' }).click();
    await expect(page.getByText('excluded for this occurrence.')).toBeVisible();
    await expect(page.getByText(/Previously reviewed: exclude/)).toBeVisible();
  } finally {
    await context.close();
  }
});

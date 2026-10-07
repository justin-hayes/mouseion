import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

test('Preflight is bounded by Mouseion document and shared-control contracts', async ({ page }) => {
  await signIn(page);
  await page.goto('/library');

  const foundation = await page.evaluate(() => {
    const root = document.querySelector('main')!;
    root.insertAdjacentHTML('beforeend', `
      <section id="preflight-contract">
        <h3>Foundation heading</h3>
        <p>Foundation paragraph <a id="preflight-inline-link" href="/library">Inline link</a></p>
        <ul><li>Native list item</li></ul>
        <fieldset><legend>Native group</legend><input aria-label="Contract field"></fieldset>
      </section>`);
    const style = (selector: string) => getComputedStyle(root.querySelector(selector)!);
    return {
      heading: { size: style('#preflight-contract h3').fontSize, weight: style('#preflight-contract h3').fontWeight, lineHeight: style('#preflight-contract h3').lineHeight },
      paragraphMargin: style('#preflight-contract p').marginBottom,
      inlineLinkDecoration: style('#preflight-inline-link').textDecorationLine,
      list: { marker: style('#preflight-contract ul').listStyleType, indent: style('#preflight-contract ul').paddingInlineStart },
      legendWeight: style('#preflight-contract legend').fontWeight,
      input: { minHeight: style('#preflight-contract input').minHeight, borderColor: style('#preflight-contract input').borderTopColor },
      headingMargins: style('#preflight-contract h3').marginBottom,
      bodyMargin: getComputedStyle(document.body).margin,
    };
  });

  expect(foundation.heading).toEqual({ size: '20px', weight: '650', lineHeight: '24px' });
  expect(foundation.paragraphMargin).toBe('12px');
  expect(foundation.inlineLinkDecoration).toContain('underline');
  expect(foundation.list.marker).toBe('disc');
  expect(foundation.list.indent).not.toBe('0px');
  expect(foundation.legendWeight).toBe('650');
  expect(foundation.input.minHeight).toBe('48px');
  expect(foundation.input.borderColor).toBe('rgb(113, 131, 141)');
  expect(foundation.headingMargins).toBe('12px');
  expect(foundation.bodyMargin).toBe('0px');

  const sharedInput = page.locator('#library-search-query');
  const inputStyle = await sharedInput.evaluate((input) => {
    const style = getComputedStyle(input);
    return { minHeight: style.minHeight, borderColor: style.borderTopColor, width: style.width };
  });
  expect(inputStyle.minHeight).toBe('48px');
  expect(inputStyle.borderColor).toBe('rgb(113, 131, 141)');
  expect(Number.parseFloat(inputStyle.width)).toBeGreaterThan(0);

  const books = page.locator('.library-grid');
  await expect(books).toHaveAttribute('role', 'list');
  await expect(books.getByRole('listitem').first()).toBeVisible();

  const sharedButton = page.getByRole('button', { name: 'Search', exact: true });
  const buttonHeight = await sharedButton.evaluate((button) => getComputedStyle(button).minHeight);
  expect(Number.parseFloat(buttonHeight)).toBeGreaterThanOrEqual(44);
});

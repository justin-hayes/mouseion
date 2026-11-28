import { expect, Page } from '@playwright/test';

// Read next replaces the Current reading from the book's row on My Books
// (ADR 0136 §4). Starting and switching are the same transition.
export async function readNext(page: Page, title: string, language = 'de') {
  await page.goto(`/${language}/library?disposition=to_read`);
  const row = page.locator('.library-books .library-book').filter({ hasText: title });
  await row.getByRole('button', { name: 'Read next' }).click();
  await expect(page).toHaveURL(/\/library\?message=/);
}

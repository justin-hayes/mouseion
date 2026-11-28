import { expect, Page, test } from '../support/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).press('Enter');
  await expect(page).toHaveURL(/\/reading/);
}

test.describe('Devices', () => {
  test('adds a Device and shows its credential once', async ({ page }) => {
    await signIn(page);
    await page.locator('.site-header__account > summary').click();
    await page.getByRole('link', { name: 'Devices' }).click();
    await expect(page).toHaveURL(/\/devices$/);
    await expect(page.getByText('No devices yet')).toBeVisible();

    await page.getByLabel('Name').fill('Palma');
    await page.getByRole('button', { name: 'Add device' }).click();

    await expect(page.getByText('This password will not be shown again')).toBeVisible();
    const username = (await page.locator('#device-username').textContent()) ?? '';
    const password = (await page.locator('#device-password').textContent()) ?? '';
    expect(username).toMatch(/^palma-[a-z0-9]{4}$/);
    expect(password).toMatch(/^([a-z0-9]{4}-){3}[a-z0-9]{4}$/);
    await expect(page.locator('#device-address')).toHaveText(/^http:\/\/.+\/kosync$/);
    await expect(page.getByText('Last seen: Never')).toBeVisible();

    await page.goto('/devices');
    await expect(page.getByText(username)).toBeVisible();
    await expect(page.getByText(password)).toHaveCount(0);
    await expect(page.getByText('This password will not be shown again')).toHaveCount(0);
  });
});

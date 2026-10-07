import { expect, test } from '../support/test';

test('retired Browse selection and Custom-deck routes are unavailable', async ({ page, baseURL }) => {
  const unauthenticated = await page.request.get(`${baseURL}/vocabulary/decks/retained-deck`);
  expect(unauthenticated.status()).toBe(404);

  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);

  await page.goto('/vocabulary');
  await expect(page.getByRole('heading', { name: 'Vocabulary', exact: true })).toBeVisible();
  for (const retired of ['Browse selection', 'Custom deck', 'Saved Custom decks']) {
    await expect(page.locator('body')).not.toContainText(retired);
  }
  await expect(page.getByRole('button', { name: 'Select', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Remove', exact: true })).toHaveCount(0);

  const retiredRoutes = [
    '/vocabulary/selection',
    '/vocabulary/selection/clear-confirm',
    '/vocabulary/decks',
    '/vocabulary/decks/retained-deck',
    '/vocabulary/decks/retained-deck/preparations',
    '/vocabulary/deck-preparations/retained-preparation',
    '/vocabulary/deck-preparations/retained-preparation/download',
  ];
  for (const route of retiredRoutes) {
    const response = await page.request.get(route);
    expect(response.status(), route).toBe(404);
  }
  for (const [route, form] of [
    ['/vocabulary/selection/add', { lemma: 'mutate', upos: 'NOUN' }],
    ['/vocabulary/selection/remove', { lemma: 'mutate', upos: 'NOUN' }],
    ['/vocabulary/selection/clear', {}],
    ['/vocabulary/decks', { name: 'new custom deck' }],
    ['/vocabulary/decks/retained-deck/rename', { name: 'renamed' }],
    ['/vocabulary/decks/retained-deck/preparations', {}],
    ['/vocabulary/deck-preparations/retained-preparation/cancel', {}],
    ['/vocabulary/decks/retained-deck/delete', {}],
  ] as const) {
    const response = await page.request.post(route, { form });
    expect(response.status(), route).toBe(404);
  }
});

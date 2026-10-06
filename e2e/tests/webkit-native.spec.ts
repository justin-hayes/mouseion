import { expect, Page, test } from '@playwright/test';
import { renderedTextContrast } from '../support/contrast';

async function signIn(page: Page, username = 'fixture-learner', password = 'fixture-password') {
  await page.goto('/login');
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByLabel('Password').press('Enter');
}

async function chooseStudyLanguage(page: Page, language: string, noJavaScript = false) {
  const switcher = page.getByLabel('Study language');
  await switcher.selectOption(language);
  if (noJavaScript) await page.getByRole('button', { name: 'Switch language' }).click();
}

async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
}

async function expectMainWithinViewport(page: Page) {
  const bounds = await page.locator('main#main-content').evaluate(element => {
    const rect = element.getBoundingClientRect();
    return { left: rect.left, right: rect.right, width: rect.width, viewport: window.innerWidth };
  });
  expect(bounds.width).toBeGreaterThan(0);
  expect(bounds.left).toBeGreaterThanOrEqual(0);
  expect(bounds.right).toBeLessThanOrEqual(bounds.viewport);
}

test.describe('native WebKit smoke journey', () => {
  test('sign-in renders associated native controls and recovers from invalid credentials', async ({ page }) => {
    await page.goto('/login');
    const username = page.getByLabel('Username');
    const password = page.getByLabel('Password');
    await expect(username).toHaveAttribute('name', 'username');
    await expect(password).toHaveAttribute('name', 'password');
    await expect(password).toHaveAttribute('type', 'password');

    await username.fill('fixture-learner');
    await password.fill('incorrect-password');
    await password.press('Enter');
    await expect(page).toHaveURL(/\/login\?/);
    await expect(page.getByRole('alert')).toBeVisible();
    await page.getByLabel('Username').fill('fixture-learner');
    await page.getByLabel('Password').fill('fixture-password');
    await page.getByLabel('Password').press('Enter');
    await expect(page).toHaveURL(/\/library$/);
    await expect(page.getByRole('heading', { name: /My Books|Welcome/i }).first()).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });

  test('authenticated shell navigation, language return, and Catalogs controls are usable', async ({ page }) => {
    await signIn(page);
    const navigation = page.getByRole('navigation', { name: 'Primary navigation' });
    const destinations = [
      { name: 'My Books', href: '/library', active: '/library' },
      { name: 'Reading', href: '/reading', active: '/reading' },
      { name: 'Vocabulary', href: '/vocabulary', active: '/vocabulary' },
      { name: 'Catalogs', href: '/catalogs', active: '/catalogs' },
    ];
    for (const destination of destinations) {
      const link = navigation.getByRole('link', { name: destination.name, exact: true });
      await expect(link).toHaveAttribute('href', destination.href);
      await link.click();
      await expect(page).toHaveURL(new RegExp(`${destination.active}(?:$|[?#])`));
      await expect(navigation.getByRole('link', { name: destination.name, exact: true })).toHaveAttribute('aria-current', 'page');
    }

    await page.goto('/reading');
    await chooseStudyLanguage(page, 'it');
    await expect(page).toHaveURL(/\/reading$/);
    await expect(page.locator('main h1')).toContainText(/Italian/);
    await expect(page.getByLabel('Study language')).toHaveValue('it');

    await page.goto('/catalogs');
    const addForm = page.locator('#catalog-connection-form form');
    await expect(addForm.getByLabel('Name', { exact: true })).toBeVisible();
    await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveAttribute('type', 'url');
    await expect(addForm.getByLabel('Username (optional)', { exact: true })).toBeVisible();
    await expect(addForm.getByLabel('Password (optional, encrypted at rest)', { exact: true })).toHaveAttribute('type', 'password');
    await expect(addForm).toHaveAttribute('method', 'post');
    await expect(addForm).toHaveAttribute('action', '/connections');
    await addForm.getByLabel('Name', { exact: true }).fill('WebKit smoke catalog');
    await addForm.getByLabel('Catalog URL', { exact: true }).fill('https://catalog.example.invalid/opds');
    await addForm.getByLabel('Username (optional)', { exact: true }).fill('fixture-user');
    await addForm.getByLabel('Password (optional, encrypted at rest)', { exact: true }).fill('fixture-secret');
    await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveValue('https://catalog.example.invalid/opds');
    const addCatalog = addForm.getByRole('button', { name: 'Add catalog', exact: true });
    await expect(addCatalog).toBeEnabled();
    const targetSize = await addCatalog.evaluate(element => {
      const rect = element.getBoundingClientRect();
      return { width: rect.width, height: rect.height };
    });
    expect(targetSize.width).toBeGreaterThanOrEqual(44);
    expect(targetSize.height).toBeGreaterThanOrEqual(44);
    expect(await addCatalog.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);
    const themeTokens = ['--mouseion-color-text', '--mouseion-color-text-muted'];
    await page.evaluate(tokens => {
      for (const token of tokens) {
        const probe = document.createElement('span');
        probe.dataset.contrastToken = token;
        probe.style.color = `var(${token})`;
        probe.style.backgroundColor = 'var(--mouseion-color-surface)';
        probe.textContent = token;
        document.body.append(probe);
      }
    }, themeTokens);
    for (const token of themeTokens) {
      expect(await page.locator(`[data-contrast-token="${token}"]`).evaluate(renderedTextContrast), token).toBeGreaterThanOrEqual(4.5);
    }
    await page.locator('[data-contrast-token]').evaluateAll(nodes => nodes.forEach(node => node.remove()));

    const confirmation = page.locator('#connection-fixture-connection details').last();
    const summary = confirmation.locator('summary');
    await expect(confirmation).not.toHaveAttribute('open', '');
    await summary.focus();
    await page.keyboard.press('Enter');
    await expect(confirmation).toHaveAttribute('open', '');
    await page.keyboard.press('Space');
    await expect(confirmation).not.toHaveAttribute('open', '');
    await expectNoHorizontalOverflow(page);
    await expectMainWithinViewport(page);

    await page.goto('/library');
    await chooseStudyLanguage(page, 'de');
  });

  test('skip link and keyboard focus remain available at compact and desktop widths', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');
    await page.evaluate(() => document.activeElement instanceof HTMLElement && document.activeElement.blur());
    await page.keyboard.press('Tab');
    const skipLink = page.getByRole('link', { name: 'Skip to main content' });
    await expect(skipLink).toBeFocused();
    await expect(skipLink).toBeVisible();
    await page.keyboard.press('Enter');
    await expect(page.locator('main#main-content')).toBeFocused();

    const focusStyle = await page.evaluate(() => {
      const element = document.activeElement;
      if (!(element instanceof HTMLElement)) return false;
      const style = getComputedStyle(element);
      return style.outlineStyle !== 'none' || style.outlineWidth !== '0px' || style.boxShadow !== 'none';
    });
    expect(focusStyle).toBe(true);
    await expectNoHorizontalOverflow(page);
  });

  test('server-rendered sign-in and authenticated native forms work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({
      baseURL,
      javaScriptEnabled: false,
      viewport: test.info().project.name.includes('compact') ? { width: 375, height: 667 } : { width: 1280, height: 800 },
      colorScheme: test.info().project.name.endsWith('-dark') ? 'dark' : 'light',
    });
    const page = await context.newPage();
    try {
      await signIn(page);
      await expect(page).toHaveURL(/\/library$/);
      await expect(page.locator('nav.site-header__nav')).toBeVisible();
      const vocabularyLink = page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('link', { name: 'Vocabulary', exact: true });
      await expect(vocabularyLink).toHaveAttribute('href', '/vocabulary');
      await vocabularyLink.click();
      await expect(page).toHaveURL(/\/vocabulary$/);
      await page.goto('/reading');
      await chooseStudyLanguage(page, 'it', true);
      await expect(page).toHaveURL(/\/reading$/);
      await expect(page.locator('main h1')).toContainText(/Italian/);

      await page.goto('/catalogs');
      const confirmation = page.locator('#connection-fixture-connection details').last();
      const summary = confirmation.locator('summary');
      await expect(confirmation).not.toHaveAttribute('open', '');
      await summary.focus();
      await page.keyboard.press('Enter');
      await expect(confirmation).toHaveAttribute('open', '');
      await expect(confirmation.getByRole('button', { name: 'Confirm deletion' })).toBeVisible();
      await page.keyboard.press('Space');
      await expect(confirmation).not.toHaveAttribute('open', '');
      await expectNoHorizontalOverflow(page);

      await page.goto('/library');
      await chooseStudyLanguage(page, 'de', true);
      await expect(page.getByLabel('Study language')).toHaveValue('de');
    } finally {
      await context.close();
    }
  });
});

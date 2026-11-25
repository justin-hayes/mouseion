import { expect, Page, test } from '../support/test';
import { renderedTextContrast } from '../support/contrast';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: /sign in|log in/i }).click();
  await expect(page).toHaveURL(/\/library/);
}

test.describe('catalog sources and browsing', () => {
  test('shows each connection state, its recovery path, and its OPDS section on the Catalog', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');

    const states = [
      { id: 'fixture-connection', label: 'Last synced', detail: /3 Books updated from their Catalog entries/ },
      { id: 'fixture-failed-connection', label: 'Sync failed', detail: /Authentication failed for this connection/ },
      { id: 'fixture-syncing-connection', label: 'Syncing', detail: /Existing Books remain available/ },
      { id: 'fixture-never-synced-connection', label: 'Never synced', detail: /ready non-English catalog languages/ },
    ];

    for (const state of states) {
      const line = page.locator(`li#source-status-${state.id}`);
      await expect(line).toBeVisible();
      await expect(line).toContainText(state.label);
      await expect(line).toContainText(state.detail);
      await expect(line.getByRole('button', { name: 'Sync now' })).toHaveCount(1);
    }
    await expect(page.locator('#source-status-fixture-never-synced-connection')).not.toContainText(/ready study-language metadata/);

    await expect(page.locator('#source-status-fixture-syncing-connection button[disabled]')).toHaveCount(1);
    await expect(page.locator('#source-status-fixture-syncing-connection').getByText('Sync already in progress.')).toBeVisible();
    await expect(page.locator('#source-status-fixture-failed-connection').getByRole('link', { name: 'Manage sources' })).toHaveAttribute('href', '/catalogs/sources#connection-fixture-failed-connection');
    await expect(page.locator('.page-header__actions').getByRole('link', { name: 'Manage sources', exact: true })).toHaveAttribute('href', '/catalogs/sources');
    await expect(page.getByRole('link', { name: 'View job status' }).first()).toHaveAttribute('href', '/jobs');

    // The Catalog owns one search and one language filter over every source.
    const search = page.getByLabel('Search the Catalog');
    await expect(search).toBeVisible();
    await expect(page.getByLabel('Language', { exact: true })).toHaveValue('de');
    await expect(page.locator('#catalog-result-summary')).toHaveAttribute('role', 'status');
    await expect(page.locator('#catalog-result-summary')).toHaveAttribute('aria-live', 'polite');

    // The default study language selects the German OPDS section, paged in
    // title-then-author order with ownership only.
    const fixture = page.locator('#catalog-source-fixture-connection');
    await expect(fixture).toBeVisible();
    await expect(fixture).toContainText('Der lange Weg nach Hause');
    await expect(fixture).toContainText('Mara Weiss');
    await expect(fixture).toContainText('Page 1 of 2');
    await expect(fixture.getByRole('link', { name: 'Next' })).toHaveAttribute('href', /language=de.*page_fixture-connection=2/);
    const addEntry = fixture.locator('.catalog-entry', { hasText: 'Ein kurzer Sommer' });
    await expect(addEntry.getByRole('button', { name: 'Add', exact: true })).toBeVisible();
    await expect(addEntry).toContainText('Adding downloads and prepares the text.');
    await expect(page.locator('#catalog-source-fixture-browser-sync-connection').getByRole('link', { name: 'In My Books', exact: true }).first()).toBeVisible();

    // The Classical catalog is a section chosen with the Language filter, not a
    // separate page or button, and connection management stays off the Catalog.
    await expect(page.getByRole('link', { name: 'Classical catalog', exact: true })).toHaveCount(0);
    await expect(page.getByLabel('Language', { exact: true }).locator('option[value="grc"]')).toHaveCount(1);
    await expect(page.locator('#catalog-source-classical')).toHaveCount(0);
    await expect(page.locator('#catalog-connection-form')).toHaveCount(0);
  });

  test('Sync now maintains the Catalog index without adding Books', async ({ page }) => {
    await signIn(page);
    await page.goto('/library');
    const before = await page.locator('.library-books > li.library-book').count();
    expect(before).toBeGreaterThan(0);

    await page.goto('/catalogs');
    await page.locator('#source-status-fixture-connection').getByRole('button', { name: 'Sync now' }).click();
    await expect(page).toHaveURL(/\/catalogs\?/);

    await page.goto('/library');
    await expect(page.locator('.library-books > li.library-book')).toHaveCount(before);
    const retired = await page.goto('/library?needs-language');
    expect(retired?.status()).toBe(404);
  });

  test('Add on an OPDS entry restores a To Read Book and focuses its confirmation', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs?language=de');

    const entry = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Ein kurzer Sommer' });
    await expect(entry).toContainText('Adding downloads and prepares the text.');
    // A text without a cover shows a Title plate that assistive technology skips.
    const plate = entry.locator('.book-cover-media__placeholder');
    await expect(plate).toHaveAttribute('aria-hidden', 'true');
    await expect(plate.locator('.title-plate__title')).toHaveText('Ein kurzer Sommer');
    await entry.getByRole('button', { name: 'Add', exact: true }).click();

    await expect(page).toHaveURL(/\/catalogs\?.*message=/);
    const confirmation = page.locator('#catalog-add-confirmation');
    await expect(confirmation).toBeFocused();
    await expect(confirmation).toContainText('Restored “Ein kurzer Sommer” to My Books.');
    await expect(confirmation).toContainText('Downloading and analyzing the text.');

    // The same entry now shows ownership and no second Add.
    const owned = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Ein kurzer Sommer' });
    await expect(owned.getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
    await expect(owned.getByRole('button', { name: 'Add', exact: true })).toHaveCount(0);

    // The confirmation names and reaches the Book in My Books.
    const view = confirmation.getByRole('link', { name: 'View in My Books', exact: true });
    await expect(view).toBeVisible();
    await view.click();
    await expect(page).toHaveURL(/\/library/);
    await expect(page.getByRole('heading', { name: 'Ein kurzer Sommer', exact: true })).toBeVisible();
  });

  test("I've read this records an OPDS entry as Read without requesting the EPUB", async ({ page }) => {
    await signIn(page);
    const contacted: string[] = [];
    page.on('request', request => {
      const host = new URL(request.url()).host;
      if (host.endsWith('.invalid')) contacted.push(request.url());
    });
    await page.goto('/catalogs?language=de');

    const entry = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Der lange Weg nach Hause' });
    await expect(entry.getByRole('button', { name: "I've read this", exact: true })).toBeVisible();
    await entry.getByRole('button', { name: "I've read this", exact: true }).click();

    await expect(page).toHaveURL(/\/catalogs\?.*message=/);
    const confirmation = page.locator('#catalog-read-confirmation');
    await expect(confirmation).toBeFocused();
    await expect(confirmation).toContainText('Recorded “Der lange Weg nach Hause” as previously read.');

    const owned = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Der lange Weg nach Hause' });
    await expect(owned.getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
    await expect(owned.getByRole('button', { name: 'Add', exact: true })).toHaveCount(0);
    await expect(owned.getByRole('button', { name: "I've read this", exact: true })).toHaveCount(0);
    expect(contacted, `I've read this contacted ${contacted.join(', ')}`).toEqual([]);

    await confirmation.getByRole('link', { name: 'View in My Books', exact: true }).click();
    await expect(page).toHaveURL(/\/library/);
    const row = page.locator('li.library-book', { has: page.getByRole('heading', { name: 'Der lange Weg nach Hause', exact: true }) });
    await expect(row).toBeVisible();
    await expect(row.getByText('Read before Mouseion')).toBeVisible();
  });

  test('one search covers the Classical catalog and OPDS without contacting a server', async ({ page }) => {
    await signIn(page);
    const contacted: string[] = [];
    page.on('request', request => {
      const host = new URL(request.url()).host;
      if (host.endsWith('.invalid')) contacted.push(request.url());
    });

    await page.goto('/catalogs?language=&q=homer');
    await expect(page.locator('#catalog-source-classical')).toBeVisible();
    await expect(page.locator('#catalog-source-classical')).toContainText('Homer');
    await expect(page.locator('#catalog-source-classical')).toContainText('Iliad');
    await expect(page.locator('#catalog-source-classical')).toContainText('Odyssey');
    await expect(page.locator('#catalog-source-fixture-connection')).toContainText('Ödipus auf Kolonos');
    await expect(page.locator('#catalog-source-fixture-connection')).toContainText('Hómēr Nachfolger');
    await expect(page.locator('#catalog-result-summary')).toHaveText('3 results from 2 sources.');
    await expect(page.locator('#catalog-source-fixture-connection')).not.toContainText('Der lange Weg nach Hause');
    expect(contacted, `Catalog search contacted ${contacted.join(', ')}`).toEqual([]);

    // Empty states are distinct: no match and nothing in this language.
    await page.goto('/catalogs?language=&q=nothing-matches');
    await expect(page.getByRole('heading', { name: 'No matches' })).toBeVisible();
    await expect(page.locator('#catalog-results').getByRole('link', { name: 'Clear search' })).toBeVisible();
    await page.goto('/catalogs?language=fr');
    await expect(page.getByRole('heading', { name: 'Nothing in fr' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Show all languages' })).toBeVisible();
  });

  test('Catalog search, paging, and filter work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    const page = await context.newPage();
    try {
      await signIn(page);
      await page.goto('/catalogs');

      await page.getByLabel('Search the Catalog').fill('uber');
      await page.getByLabel('Language', { exact: true }).selectOption('');
      await page.getByRole('button', { name: 'Search' }).click();
      await expect(page).toHaveURL(/\/catalogs\?.*q=uber/);
      await expect(page).toHaveURL(/language=/);
      await expect(page.locator('#catalog-source-fixture-connection')).toContainText('Über die Berge');
      await expect(page.locator('#catalog-source-fixture-connection')).not.toContainText('Der lange Weg nach Hause');

      await page.goto('/catalogs?language=de');
      await page.locator('#catalog-source-fixture-connection').getByRole('link', { name: 'Next' }).click();
      await expect(page).toHaveURL(/page_fixture-connection=2/);
      await expect(page).toHaveURL(/language=de/);
      await expect(page.locator('#catalog-source-fixture-connection')).toContainText('Page 2 of 2');
      await expect(page.locator('#catalog-source-fixture-connection')).toContainText('Über die Berge');

      // Ownership links and Add forms are ordinary server-rendered controls.
      await page.goto('/catalogs?language=de');
      const owned = page.locator('#catalog-source-fixture-browser-sync-connection').getByRole('link', { name: 'In My Books', exact: true }).first();
      await expect(owned).toHaveAttribute('href', /\/catalogs\/books\/[^/]+\/in-my-books$/);

      const entry = page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Gedichte aus dem Norden' });
      await entry.getByRole('button', { name: 'Add', exact: true }).click();
      await expect(page).toHaveURL(/\/catalogs\?.*message=/);
      await expect(page.locator('#catalog-add-confirmation')).toContainText('Restored “Gedichte aus dem Norden” to My Books.');
      await expect(page.locator('#catalog-source-fixture-connection .catalog-entry', { hasText: 'Gedichte aus dem Norden' }).getByRole('link', { name: 'In My Books', exact: true })).toBeVisible();
    } finally {
      await context.close();
    }
  });


  test('Manage sources preserves enabled, disabled, and field states', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs/sources');

    for (const theme of ['light', 'dark']) {
      await page.locator('html').evaluate((node, value) => node.setAttribute('data-theme', value), theme);
      const enabled = page.locator('#connection-fixture-connection').getByRole('button', { name: 'Sync now' });
      const disabled = page.locator('#connection-fixture-syncing-connection').getByRole('button', { name: 'Sync now' });
      await expect(enabled).toHaveClass(/\bbutton\b/);
      await expect(disabled).toHaveClass(/\bbutton\b/);
      await expect(enabled).toBeEnabled();
      await expect(disabled).toBeDisabled();
      await expect(page.locator('#connection-fixture-syncing-connection')).toContainText('Sync already in progress.');
      const opacity = await Promise.all([enabled, disabled].map(control => control.evaluate(node => Number(getComputedStyle(node).opacity))));
      expect(opacity[0]).toBeGreaterThan(opacity[1]);
      expect(await enabled.evaluate(renderedTextContrast)).toBeGreaterThanOrEqual(4.5);

      const addForm = page.locator('#catalog-connection-form form');
      const name = addForm.getByLabel('Name', { exact: true });
      const url = addForm.getByLabel('Catalog URL', { exact: true });
      await expect(name).toHaveClass(/\binput\b/);
      await expect(url).toHaveClass(/\binput\b/);
      await expect(name).toHaveAttribute('required', '');
      expect(await name.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);
      expect(await url.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);
      await name.focus();
      expect(await name.evaluate(node => getComputedStyle(node).outlineStyle)).toBe('solid');
      await addForm.getByRole('button', { name: 'Add catalog' }).click();
      expect(await name.evaluate(node => (node as HTMLInputElement).validity.valid)).toBe(false);
      expect(await name.evaluate(node => node.matches(':user-invalid'))).toBe(true);
      expect(await name.evaluate(renderedTextContrast, true)).toBeGreaterThanOrEqual(3);

      const editForm = page.locator('#edit-connection-fixture-connection form.catalog-connection-form');
      await expect(editForm).toHaveAttribute('method', 'post');
      await expect(editForm).toHaveAttribute('action', '/catalogs/sources/fixture-connection');
      await expect(editForm.getByLabel('Name', { exact: true })).toHaveClass(/\binput\b/);
    }
  });

  test('exposes Catalog as the current shell destination and retires the old route', async ({ page }) => {
    await signIn(page);
    await page.goto('/catalogs');
    await expect(page).toHaveTitle('Catalog · Mouseion');
    await expect(page.getByRole('heading', { name: 'Catalog', exact: true })).toBeVisible();
    await expect(page.locator('nav.site-header__nav a[aria-current="page"]')).toHaveText('Catalog');

    const redirect = await page.request.get('/connections?book_id=book%2F1&message=Catalog+added&error=Try+again&ignored=drop', { maxRedirects: 0 });
    expect(redirect.status()).toBe(301);
    expect(redirect.headers().location).toBe('/catalogs?book_id=book%2F1&error=Try+again&message=Catalog+added');

    // GET /catalogs still carries its supported query parameters.
    const direct = await page.request.get('/catalogs?message=Catalog+added&error=Try+again&book_id=book%2F1', { maxRedirects: 0 });
    expect(direct.status()).toBe(200);
  });

  test('Manage sources is reachable without JavaScript and keeps native controls', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    const page = await context.newPage();
    try {
      await signIn(page);
      await page.goto('/catalogs');
      const manageSources = page.locator('.page-header__actions').getByRole('link', { name: 'Manage sources', exact: true });
      await expect(manageSources).toBeVisible();
      await manageSources.click();
      await expect(page).toHaveURL(/\/catalogs\/sources$/);
      await expect(page.getByRole('heading', { name: 'Manage sources', exact: true })).toBeVisible();

      const addForm = page.locator('#catalog-connection-form form');
      await expect(addForm).toHaveAttribute('method', 'post');
      await expect(addForm).toHaveAttribute('action', '/catalogs/sources');
      await expect(addForm.getByLabel('Name', { exact: true })).toHaveAttribute('required', '');
      await expect(addForm.getByLabel('Catalog URL', { exact: true })).toHaveAttribute('required', '');
      await expect(addForm.evaluate(form => (form as HTMLFormElement).reportValidity())).resolves.toBe(false);

      const deletion = page.locator('#connection-fixture-connection details.confirmation');
      await expect(deletion.locator('summary')).toHaveText('Delete catalog connection');
      await deletion.locator('summary').click();
      await expect(deletion.getByRole('button', { name: 'Confirm deletion' })).toBeVisible();
    } finally {
      await context.close();
    }
  });

  test('Catalog search, filter, source controls, and results follow keyboard order at narrow widths', async ({ page }) => {
    await signIn(page);
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/catalogs');

    const manage = page.locator('.page-header__actions').getByRole('link', { name: 'Manage sources', exact: true });
    await manage.focus();
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Search the Catalog')).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.getByLabel('Language', { exact: true })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.getByRole('button', { name: 'Search' })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(page.locator('#source-status-fixture-connection').getByRole('button', { name: 'Sync now' })).toBeFocused();

    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    const entryBox = await page.locator('#catalog-source-fixture-connection .catalog-entry').first().boundingBox();
    expect(entryBox?.width ?? 0).toBeLessThanOrEqual(375);
  });
});

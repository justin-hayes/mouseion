import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
}

const representativePages: Array<[string, RegExp]> = [
  ['/library', /My Books/],
  ['/books/fixture-book', /Der lange Weg nach Hause/],
  ['/books/fixture-book/scope', /Review analysis scope/],
  ['/jobs/42', /Analysis job #1/],
  ['/books/fixture-book/analyses/fixture-run', /Der lange Weg nach Hause/],
  ['/deck-preparations/fixture-preparation/status', /Deck preparation/],
  ['/jobs', /Jobs/],
  ['/connections', /Add books/],
  ['/catalog?connection=fixture-connection', /Fixture catalog/],
  ['/journey', /Reading Journey/],
  ['/settings?language=de', /Account settings/],
];

async function expectNoPageOverflow(page: Page) {
  const sizes = await page.evaluate(() => ({
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
    viewport: window.innerWidth,
  }));
  expect(sizes.document, JSON.stringify(sizes)).toBeLessThanOrEqual(sizes.viewport + 1);
  expect(sizes.body, JSON.stringify(sizes)).toBeLessThanOrEqual(sizes.viewport + 1);
  const regions = await page.locator('.table-region').evaluateAll((nodes) => nodes.map((node) => ({
    overflow: getComputedStyle(node).overflowX,
    right: node.getBoundingClientRect().right,
  })));
  for (const region of regions) {
    expect(region.overflow).toBe('auto');
    expect(region.right).toBeLessThanOrEqual(sizes.viewport + 1);
  }
}

test.describe('responsive and theme regression coverage', () => {
  test('authenticated shell and primary workflows remain readable without compact page overflow', async ({ page }) => {
    await signIn(page);
    for (const [url, heading] of representativePages) {
      await page.goto(url);
      await expect(page.getByRole('heading', { name: heading }).first()).toBeVisible();
      if (test.info().project.name.startsWith('compact')) await expectNoPageOverflow(page);
    }
    await page.goto('/library');
    await expect(page.locator('a[href="/books/fixture-edge-content"]')).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-empty"]')).toBeVisible();
    await expect(page.locator('a[href="/books/fixture-failed"]')).toBeVisible();
    await expect(page.locator('.library-book').filter({ has: page.locator('.bibliographic-title a') })).toHaveCount(9);
    if (test.info().project.name.startsWith('compact')) {
      // Filter to books with a bibliographic title so metadata-only fixture
      // entries (which have no title link) do not displace the last acquired
      // book in this overflow assertion.
      await expect(page.locator('.library-book').filter({ has: page.locator('.bibliographic-title a') }).last()).toContainText('Donaudampfschifffahrtsgesellschaftskapitänsmütze');
      await expect(page.locator('.library-book').filter({ has: page.locator('.bibliographic-title a') }).last()).toBeVisible();
    }
    await page.goto('/opds/browse?connection=fixture-connection&language=de');
    await expect(page.getByText(/Un libro italiano/)).toBeVisible();
    await expect(page.getByText(/Donaudampfschifffahrtsgesellschaftskapitänsmütze/)).toBeVisible();
  });

  test('dense analysis, campaign history, errors, and import surfaces expose realistic content', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book');
    await expect(page.locator('.stat-group__value').filter({ hasText: '37.0%' })).toBeVisible();
    await expect(page.getByText('Randlemma-18')).toBeVisible();
    await expect(page.locator('.top-unknown li')).toHaveCount(18);
    await expect(page.locator('.stat-group').last()).toBeVisible();
    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' }).locator('tbody tr')).toHaveCount(18);
    await page.goto('/journey');
    await expect(page.locator('#campaign-fixture-queued-campaign-6')).toBeVisible();
    await expect(page.locator('[aria-labelledby="campaign-operations-heading"] article').first()).toBeVisible();
    await page.goto('/jobs/43');
    await expect(page.getByRole('alert')).toContainText(/Reload the confirmed scope and retry/);
    await expect(page.getByRole('button', { name: 'Retry analysis' })).toBeVisible();
    await page.goto('/settings?language=it');
    await expect(page.getByText(/Known vocabulary/).first()).toBeVisible();
  });

  test('action order and compact touch targets preserve reachability', async ({ page }) => {
    await signIn(page);
    await page.goto('/journey');
    const order = await page.locator('.action-group').evaluateAll((groups) => groups.map((group) => {
      const controls = Array.from(group.querySelectorAll('button, a[role="button"]'));
      return controls.map((control) => control.classList.contains('secondary'));
    }));
    for (const controls of order) if (controls.length) expect(controls[0]).toBe(false);
    const controls = await page.locator('button, a[role="button"]').evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
    }));
    for (const control of controls) {
      expect(control.width, control.text).toBeGreaterThanOrEqual(44);
      expect(control.height, control.text).toBeGreaterThanOrEqual(44);
      expect(control.left, control.text).toBeGreaterThanOrEqual(-1);
      expect(control.right, control.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    await page.goto('/books/fixture-book');
    const resultActions = await page.locator('[aria-labelledby="deck-preparation-heading"] button, [aria-labelledby="deck-preparation-heading"] a[role="button"]').evaluateAll((nodes) => nodes.map((node) => node.textContent?.trim()));
    expect(resultActions[0]).toMatch(/Prepare deck|Download deck/);
  });

  test('semantic status text, readable measures, and live theme tokens meet contrast targets', async ({ page }) => {
    await signIn(page);
    await page.goto('/books/fixture-book/analyses/fixture-run');
    const semantics = await page.locator('.status-badge').evaluateAll((nodes) => nodes.map((node) => ({
      text: node.textContent?.trim(), color: getComputedStyle(node).color, border: getComputedStyle(node).borderTopColor,
    })));
    expect(semantics.length).toBeGreaterThan(0);
    for (const item of semantics) {
      expect(item.text).toBeTruthy();
      expect(item.color).not.toBe('rgba(0, 0, 0, 0)');
      expect(item.border).toBe(item.color);
    }
    const measure = await page.evaluate(() => {
      const root = getComputedStyle(document.documentElement);
      const reading = document.querySelector<HTMLElement>('.reading-text, .reading-width');
      return { token: root.getPropertyValue('--mouseion-width-reading').trim(), rootFontSize: parseFloat(root.fontSize), width: reading?.getBoundingClientRect().width ?? null };
    });
    expect(measure.token).toBe('42rem');
    if (measure.width !== null) expect(measure.width).toBeLessThanOrEqual(42 * measure.rootFontSize + 1);

    const contrast = await page.evaluate(() => {
      const parse = (value: string) => value.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1).map(Number) ?? [];
      const luminance = (value: string) => {
        const rgb = parse(value).map((channel) => { const s = channel / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4; });
        return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
      };
      const ratio = (foreground: string, background: string) => { const a = luminance(foreground); const b = luminance(background); return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05); };
      const probe = (foreground: string, background: string) => { const node = document.createElement('span'); node.style.color = 'var(' + foreground + ')'; node.style.backgroundColor = 'var(' + background + ')'; node.textContent = 'probe'; document.body.append(node); const style = getComputedStyle(node); const value = ratio(style.color, style.backgroundColor); node.remove(); return value; };
      return Object.fromEntries(['--mouseion-color-text', '--mouseion-color-text-muted', '--mouseion-color-info', '--mouseion-color-success', '--mouseion-color-warning', '--mouseion-color-danger', '--mouseion-color-focus'].map((token) => [token, probe(token, '--mouseion-color-surface')]));
    });
    for (const [token, value] of Object.entries(contrast)) expect(value, token).toBeGreaterThanOrEqual(token === '--mouseion-color-focus' ? 3 : 4.5);
  });
});

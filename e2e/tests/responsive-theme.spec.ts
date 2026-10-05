import { expect, Page, test } from '@playwright/test';

async function signIn(page: Page) {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
  const switcher = page.getByLabel('Study language');
  if (await switcher.inputValue() !== 'de') {
    await switcher.selectOption('de');
    await expect(page).toHaveURL(/\/library$/);
  }
}

const representativePages: Array<[string, RegExp]> = [
  ['/library', /My Books/],
  ['/reading#journey-book-fixture-book', /Der lange Weg nach Hause/],
  ['/jobs/42', /Analysis job #1/],
  ['/books/fixture-book/analyses/fixture-run', /Der lange Weg nach Hause/],
  ['/deck-preparations/fixture-preparation/status', /Deck preparation/],
  ['/jobs', /Jobs/],
  ['/catalogs', /Catalogs/],
  ['/reading', /Reading/],
  ['/vocabulary', /Vocabulary/],
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
  test('sign-in controls are styled, focused, and reachable at 200 percent text size', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('link[rel="stylesheet"][href="/static/login.css"]')).toHaveCount(1);
    const shell = await page.locator('body > header.site-header').evaluate((node) => {
      const style = getComputedStyle(node);
      return style.maxWidth;
    });
    expect(shell).toBe('none');

    const username = page.getByLabel('Username');
    const initial = await username.evaluate((node) => {
      const style = getComputedStyle(node);
      const box = node.getBoundingClientRect();
      return { background: style.backgroundColor, border: style.borderTopWidth, height: box.height };
    });
    expect(initial.background).not.toBe('rgba(0, 0, 0, 0)');
    expect(parseFloat(initial.border)).toBeGreaterThan(0);
    expect(initial.height).toBeGreaterThanOrEqual(48);

    const contrast = await page.evaluate(() => {
      const channels = (color: string) => color.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/)?.slice(1).map(Number) ?? [];
      const luminance = (color: string) => channels(color).map((channel) => {
        const value = channel / 255;
        return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
      }).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
      const ratio = (foreground: string, background: string) => {
        const a = luminance(foreground);
        const b = luminance(background);
        return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
      };
      const input = document.querySelector<HTMLInputElement>('.login-screen input[name="username"]')!;
      const button = document.querySelector<HTMLButtonElement>('.login-screen button')!;
      const inputStyle = getComputedStyle(input);
      const buttonStyle = getComputedStyle(button);
      return {
        field: ratio(inputStyle.color, inputStyle.backgroundColor),
        action: ratio(buttonStyle.color, buttonStyle.backgroundColor),
      };
    });
    expect(contrast.field).toBeGreaterThanOrEqual(4.5);
    expect(contrast.action).toBeGreaterThanOrEqual(4.5);

    await username.focus();
    const focus = await username.evaluate((node) => getComputedStyle(node).outlineStyle);
    expect(focus).toBe('solid');

    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    for (const control of [username, page.getByLabel('Password'), page.getByRole('button', { name: 'Sign in' })]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(-1);
      expect(box!.x + box!.width).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
  });

  test('Reading respects the reduced-motion preference', async ({ page }) => {
    await signIn(page);
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto('/reading');
    const motion = await page.evaluate(() => ({
      requested: matchMedia('(prefers-reduced-motion: reduce)').matches,
      animations: Array.from(document.querySelectorAll('main h1, main h2, button, a[role="button"]')).map((node) => getComputedStyle(node).animationName),
    }));
    expect(motion.requested).toBe(true);
    expect(motion.animations.every((name) => name === 'none')).toBe(true);
  });

  test('authenticated shell and primary workflows remain readable without compact page overflow', async ({ page }) => {
    await signIn(page);
    for (const [url, heading] of representativePages) {
      await page.goto(url);
      await expect(page.getByRole('heading', { name: heading }).first()).toBeVisible();
      if (test.info().project.name.startsWith('compact')) await expectNoPageOverflow(page);
    }
    await page.goto('/library');
    const switcher = page.getByLabel('Study language');
    if (await switcher.inputValue() !== 'de') {
      await switcher.selectOption('de');
      await expect(page).toHaveURL(/\/library$/);
    }
    await expect(page.locator('.library-grid .library-book__identity-link[href="/reading#journey-book-fixture-book"]')).toBeVisible();
    await expect(page.locator('.library-grid a[href="/books/fixture-failed"]')).toHaveCount(0);
    await expect(page.locator('.library-grid a[href="/books/fixture-edge-content"]')).toHaveCount(0);
    await expect(page.locator('.library-grid a[href="/books/fixture-empty"]')).toHaveCount(0);
    expect(await page.locator('.library-book').filter({ has: page.locator('.library-book__identity-link[href^="/reading#"]') }).count()).toBeGreaterThan(0);
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expectNoPageOverflow(page);
    const libraryControls = page.locator('.library-grid button:visible, .library-grid a.library-book__journey-action:visible, .library-grid > .library-book > details > summary:visible');
    const libraryControlBoxes = await libraryControls.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
    }));
    for (const control of libraryControlBoxes) {
      expect(control.width, control.text).toBeGreaterThanOrEqual(44);
      expect(control.height, control.text).toBeGreaterThanOrEqual(44);
      expect(control.left, control.text).toBeGreaterThanOrEqual(-1);
      expect(control.right, control.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    if (test.info().project.name.startsWith('compact')) {
      // The Italian Journey retains the long-title content for narrow-layout
      // coverage without relying on the retired unassessed Book page.
      await page.goto('/library');
      await page.getByLabel('Study language').selectOption('it');
      await expect(page).toHaveURL(/\/library$/);
      await page.goto('/reading');
      await expect(page.getByRole('heading', { name: /Donaudampfschifffahrtsgesellschaftskapitänsmütze/ })).toBeVisible();
      await expectNoPageOverflow(page);
    }
  });

  test('dense analysis, deck provenance, errors, and import surfaces expose realistic content', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book')).toBeVisible();
    await expect(page.getByText('Vocabulary investment', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Highest-impact unknown vocabulary', { exact: true })).toHaveCount(0);
    await page.goto('/jobs');
    await expect(page.getByRole('region', { name: 'Analysis history' }).locator('tbody tr')).toHaveCount(18);
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.getByRole('heading', { name: "This Book's vocabulary study" })).toHaveCount(0);
    await page.goto('/jobs/43');
    await expect(page.getByRole('alert')).toContainText(/Retry the analysis when you are ready/);
    await expect(page.getByRole('button', { name: 'Retry analysis' })).toBeVisible();
    await page.goto('/vocabulary/import');
    await expect(page.getByRole('heading', { name: 'Vocabulary · Import known vocabulary', exact: true })).toBeVisible();
    await expect(page.getByLabel('UTF-8 lemma file')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Known vocabulary', exact: true })).toHaveCount(0);
  });

  test('action order and compact touch targets preserve reachability', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');
    const current = page.locator('#primary-goal-section');
    if (await current.locator('.journey-book--goal').count()) {
      await expect(current.getByRole('button', { name: 'Switch current reading' })).toBeVisible();
      const finishSummary = current.locator('details').filter({ hasText: 'Mark reading finished' }).locator('summary');
      await expect(finishSummary).toBeVisible();
      const primaryActions = await current.locator('.goal-card__actions > a[role="button"], .goal-card__actions > .confirmation > summary').evaluateAll((nodes) => nodes.map((node) => {
        const box = node.getBoundingClientRect();
        return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
      }));
      expect(primaryActions.length).toBeGreaterThan(0);
      for (const action of primaryActions) {
        expect(action.width, action.text).toBeGreaterThanOrEqual(44);
        expect(action.height, action.text).toBeGreaterThanOrEqual(44);
        expect(action.left, action.text).toBeGreaterThanOrEqual(-1);
        expect(action.right, action.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
      }
    } else {
      const eligibleCandidate = page.locator('li.reading-chooser-book').filter({ has: page.getByRole('button', { name: 'Confirm start reading' }) }).first();
      await expect(eligibleCandidate).toBeVisible();
      await eligibleCandidate.locator('details').filter({ hasText: 'Start reading' }).locator('summary').click();
      const startAction = eligibleCandidate.getByRole('button', { name: 'Confirm start reading' });
      await expect(startAction).toBeVisible();
      const box = await startAction.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.width).toBeGreaterThanOrEqual(44);
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.x).toBeGreaterThanOrEqual(-1);
      expect(box!.x + box!.width).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    const order = await page.locator('.action-group').evaluateAll((groups) => groups.map((group) => {
      const controls = Array.from(group.querySelectorAll('button, a[role="button"]'));
      return controls.map((control) => control.classList.contains('secondary'));
    }));
    for (const controls of order) if (controls.length) expect(controls[0]).toBe(false);
    const controls = await page.locator('button:visible, a[role="button"]:visible').evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return { width: box.width, height: box.height, left: box.left, right: box.right, text: node.textContent?.trim() };
    }));
    for (const control of controls) {
      expect(control.width, control.text).toBeGreaterThanOrEqual(44);
      expect(control.height, control.text).toBeGreaterThanOrEqual(44);
      expect(control.left, control.text).toBeGreaterThanOrEqual(-1);
      expect(control.right, control.text).toBeLessThanOrEqual(await page.evaluate(() => window.innerWidth) + 1);
    }
    await page.goto('/reading#journey-book-fixture-book');
    await expect(page.locator('#journey-book-fixture-book a[href$="/deck/preparations/new"]')).toBeVisible();
  });

  test('standard Journey rows preserve a readable book column beside controls', async ({ page }) => {
    test.skip(!test.info().project.name.startsWith('desktop'), 'This contract applies to the standard desktop layout.');
    await signIn(page);
    await page.goto('/reading');
    const rows = await page.locator('.journey-book:not(.journey-book--goal)').evaluateAll((nodes) => nodes.map((node) => {
      const book = node.querySelector<HTMLElement>('.journey-book__identity')?.getBoundingClientRect();
      const controls = node.querySelector<HTMLElement>('.journey-book__controls')?.getBoundingClientRect();
      return { bookWidth: book?.width ?? 0, controlsWidth: controls?.width ?? 0, rowWidth: node.getBoundingClientRect().width };
    }));
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(row.bookWidth).toBeGreaterThan(200);
      expect(row.controlsWidth).toBeLessThan(row.rowWidth * 0.6);
    }
  });

  test('Journey thumbnails stay aligned, quiet, and outside the keyboard order', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');

    const cards = page.locator('.journey-book');
    const thumbnails = cards.locator('.journey-book__cover');
    await expect(thumbnails).toHaveCount(await cards.count());
    await expect(page.locator('.journey-book--goal .journey-book__cover img')).toHaveAttribute('alt', '');
    await expect(page.locator('.journey-book--goal .journey-book__cover img')).toHaveAttribute('src', '/books/fixture-book/cover');
    await expect(page.locator('.journey-book__cover .book-cover-media__placeholder[aria-hidden="true"]').first()).toBeVisible();
    await expect(page.locator('.journey-book__cover a, .journey-book__cover button, .journey-book__cover summary')).toHaveCount(0);

    const layout = await thumbnails.evaluateAll((nodes) => nodes.map((node) => {
      const box = node.getBoundingClientRect();
      const card = node.parentElement?.getBoundingClientRect();
      return { width: box.width, height: box.height, right: box.right, cardRight: card?.right ?? 0 };
    }));
    for (const item of layout) {
      expect(item.width).toBeGreaterThan(0);
      expect(item.height).toBeGreaterThan(0);
      expect(item.right).toBeLessThanOrEqual(item.cardRight + 1);
    }
    await expectNoPageOverflow(page);
  });

  test('Journey titles and actions remain reachable at 200 percent text size', async ({ page }) => {
    await signIn(page);
    await page.goto('/reading');
    await page.evaluate(() => { document.documentElement.style.fontSize = '200%'; });
    await expect(page.getByRole('heading', { name: 'To Read books', exact: true })).toBeVisible();
    await expect(page.locator('.journey-book__controls').first()).toBeVisible();
    await expectNoPageOverflow(page);
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

test.describe('sign-in server-rendered recovery without JavaScript', () => {
  test.use({ javaScriptEnabled: false });

  test('invalid credentials return an accessible styled error and remain retryable', async ({ page }) => {
    await page.goto('/login');
    const username = page.getByLabel('Username');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page).toHaveURL('/login');
    expect(await username.evaluate((node) => (node as HTMLInputElement).validity.valueMissing)).toBe(true);

    await username.fill('fixture-learner');
    await page.getByLabel('Password').fill('incorrect-password');
    await page.getByRole('button', { name: 'Sign in' }).click();

    await expect(page).toHaveURL(/\/login\?error=/);
    await expect(page.getByRole('alert')).toHaveText('Invalid credentials');
    await expect(page.locator('link[href="/static/login.css"]')).toHaveCount(1);
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
  });
});

import { expect, test } from '../support/test';

test('locally served fonts shape Latin and Greek text on the sign-in page', async ({ page }) => {
  const fontRequests: string[] = [];
  page.on('request', (request) => {
    if (/\.woff2(?:\?|$)/.test(request.url())) fontRequests.push(request.url());
  });

  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.locator('main').evaluate((main) => {
    main.insertAdjacentHTML('beforeend', '<p id="font-proof" class="reading-text">Für Donaudampfschifffahrtsgesellschaftskapitän; città; συνταγματολόγος — Αθήνα, ΐ and Ϊ.</p>');
  });

  const proof = await page.locator('#font-proof').evaluate(async (element) => {
    const sample = element.textContent!;
    const appFaces = await document.fonts.load('400 16px "Commissioner Variable"', sample);
    const readingFaces = await document.fonts.load('400 17px "Literata Variable"', sample);
    const text = getComputedStyle(element);
    return {
      appLoaded: appFaces.length > 0 && appFaces.every((face) => face.status === 'loaded'),
      readingLoaded: readingFaces.length > 0 && readingFaces.every((face) => face.status === 'loaded'),
      family: text.fontFamily,
      size: text.fontSize,
      width: element.getBoundingClientRect().width,
      scrollWidth: element.scrollWidth,
    };
  });

  expect(proof.appLoaded).toBe(true);
  expect(proof.readingLoaded).toBe(true);
  expect(proof.family).toContain('Literata Variable');
  expect(proof.size).toBe('17px');
  expect(proof.width).toBeGreaterThan(0);
  expect(proof.scrollWidth).toBeLessThanOrEqual(proof.width);
  expect(fontRequests.length).toBeGreaterThanOrEqual(3);
  expect(fontRequests.every((url) => url.startsWith(new URL('/static/vendor/fonts/', page.url()).origin))).toBe(true);
});

test('applies Literata to actual Book identity and passage text', async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);

  const bookTitle = page.locator('.bibliographic-title').first();
  await expect(bookTitle).toBeVisible();
  const bookFace = await bookTitle.evaluate(async (element) => {
    const style = getComputedStyle(element);
    const loadedFaces = await document.fonts.load(`600 ${style.fontSize} "Literata Variable"`, element.textContent!);
    return { family: style.fontFamily, loaded: loadedFaces.length > 0 && loadedFaces.every((face) => face.status === 'loaded') };
  });
  expect(bookFace.family).toContain('Literata Variable');
  expect(bookFace.loaded).toBe(true);

  await page.goto('/vocabulary/concordance?term=Haus');
  await page.locator('.concordance-row summary').first().click();
  const passage = page.locator('.reading-text').first();
  await expect(passage).toBeVisible();
  const passageFace = await passage.evaluate(async (element) => {
    const style = getComputedStyle(element);
    const loadedFaces = await document.fonts.load(`400 ${style.fontSize} "Literata Variable"`, element.textContent!);
    const bounds = element.getBoundingClientRect();
    return {
      family: style.fontFamily,
      loaded: loadedFaces.length > 0 && loadedFaces.every((face) => face.status === 'loaded'),
      fits: element.scrollWidth <= bounds.width,
      width: bounds.width,
    };
  });
  expect(passageFace.family).toContain('Literata Variable');
  expect(passageFace.loaded).toBe(true);
  expect(passageFace.width).toBeGreaterThan(0);
  expect(passageFace.fits).toBe(true);
});

test('loads self-hosted Literata italic only for italic Book labels', async ({ page }) => {
  const fontRequests: string[] = [];
  page.on('request', (request) => {
    if (/\.woff2(?:\?|$)/.test(request.url())) fontRequests.push(request.url());
  });
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  expect(fontRequests.every(url => url.startsWith(new URL('/static/vendor/fonts/', page.url()).origin))).toBe(true);
  expect(fontRequests.some(url => url.includes('-italic.woff2'))).toBe(false);
  expect(await page.locator('link[rel="preload"][href*="italic"]').count()).toBe(0);

  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.goto('/vocabulary/concordance?term=Haus');
  const label = page.locator('.concordance-book-title').first();
  await expect(label).toBeVisible();
  const face = await label.evaluate(async element => {
    const style = getComputedStyle(element);
    const loaded = await document.fonts.load(`italic ${style.fontWeight} ${style.fontSize} "Literata Variable"`, element.textContent!);
    return { style: style.fontStyle, loaded: loaded.length > 0 && loaded.every(font => font.status === 'loaded') };
  });
  expect(face.style).toBe('italic');
  expect(face.loaded).toBe(true);
  expect(fontRequests.some(url => url.includes('-italic.woff2'))).toBe(true);
  expect(fontRequests.every(url => url.startsWith(new URL('/static/vendor/fonts/', page.url()).origin))).toBe(true);
});

test('sign-in and Greek text remain visible and usable when local font requests fail', async ({ page }) => {
  await page.route('**/*.woff2', (route) => route.abort());
  await page.goto('/login');
  await page.locator('main').evaluate((main) => {
    main.insertAdjacentHTML('beforeend', '<p id="fallback-proof" class="reading-text">Für Donaudampfschifffahrtsgesellschaftskapitän; città; συνταγματολόγος — Αθήνα, ΐ and Ϊ.</p>');
  });

  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await expect(page.locator('#fallback-proof')).toBeVisible();
  const layout = await page.evaluate(() => ({
    pageWidth: document.documentElement.scrollWidth,
    viewportWidth: document.documentElement.clientWidth,
    probeWidth: document.querySelector('#fallback-proof')!.getBoundingClientRect().width,
  }));
  expect(layout.pageWidth).toBeLessThanOrEqual(layout.viewportWidth);
  expect(layout.probeWidth).toBeGreaterThan(0);

  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('incorrect-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();

  await page.getByLabel('Username').fill('fixture-learner');
  await page.getByLabel('Password').fill('fixture-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/\/library/);
  const bookTitle = page.locator('.bibliographic-title').first();
  await expect(bookTitle).toBeVisible();
  const bookFits = await bookTitle.evaluate((element) => ({
    family: getComputedStyle(element).fontFamily,
    contentWidth: element.scrollWidth,
    renderedWidth: element.getBoundingClientRect().width,
    pageWidth: document.documentElement.scrollWidth,
    viewportWidth: document.documentElement.clientWidth,
  }));
  expect(bookFits.family).toContain('Literata Variable');
  expect(bookFits.contentWidth).toBeLessThanOrEqual(Math.ceil(bookFits.renderedWidth));
  expect(bookFits.pageWidth).toBeLessThanOrEqual(bookFits.viewportWidth);

  await page.goto('/vocabulary/concordance?term=Haus');
  await page.locator('.concordance-row summary').first().click();
  const passage = page.locator('.reading-text').first();
  await expect(passage).toBeVisible();
  const passageFits = await passage.evaluate((element) => ({
    family: getComputedStyle(element).fontFamily,
    contentWidth: element.scrollWidth,
    renderedWidth: element.getBoundingClientRect().width,
    pageWidth: document.documentElement.scrollWidth,
    viewportWidth: document.documentElement.clientWidth,
  }));
  expect(passageFits.family).toContain('Literata Variable');
  expect(passageFits.contentWidth).toBeLessThanOrEqual(Math.ceil(passageFits.renderedWidth));
  expect(passageFits.pageWidth).toBeLessThanOrEqual(passageFits.viewportWidth);
});

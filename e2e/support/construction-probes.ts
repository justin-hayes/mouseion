import { expect, type Locator, type Page } from './test';

// The acceptance viewports are 375px and 1200px in both themes; the project
// supplies the theme and the compact or desktop baseline.
export async function acceptanceViewport(page: Page) {
  const viewport = page.viewportSize()!;
  const width = viewport.width <= 600 ? 375 : 1200;
  await page.setViewportSize({ width, height: 800 });
}

export async function expectNoHorizontalOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
}

export const weight = (locator: Locator) => locator.evaluate((element) => Number.parseInt(getComputedStyle(element).fontWeight, 10));
export const underlined = (locator: Locator) => locator.evaluate((element) => getComputedStyle(element).textDecorationLine.includes('underline'));

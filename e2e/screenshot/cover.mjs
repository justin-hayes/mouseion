// Renders one typographic cover (title and author) with the browser suite's
// pinned Playwright Chromium. Every scenario book uses the same template.
import playwright from '@playwright/test';
import { escapeXml } from './epub.mjs';

const COVER_WIDTH = 600;
const COVER_HEIGHT = 900;

export function coverHtml({ title, author }) {
  // Long titles step down in size so the title stays inside the frame.
  const titleSize = title.length > 36 ? 44 : title.length > 20 ? 52 : 60;
  return `<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<style>
  html, body { margin: 0; width: ${COVER_WIDTH}px; height: ${COVER_HEIGHT}px; overflow: hidden; }
  body {
    box-sizing: border-box;
    padding: 84px 62px 72px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: space-between;
    background: #f3ead6;
    color: #2a2118;
    font-family: "Iowan Old Style", Georgia, "DejaVu Serif", "Liberation Serif", serif;
    text-align: center;
    border: 22px solid #2a2118;
  }
  .rule { width: 120px; height: 2px; background: #8a3b22; }
  .rule.bottom { margin-top: 0; }
  h1 {
    margin: 0;
    font-weight: 400;
    font-size: ${titleSize}px;
    line-height: 1.12;
    letter-spacing: 0.01em;
    hyphens: auto;
    overflow-wrap: break-word;
  }
  .author {
    margin: 0;
    font-size: 26px;
    font-style: italic;
    letter-spacing: 0.04em;
    color: #5b4a3a;
  }
</style>
</head>
<body>
  <div class="rule"></div>
  <div>
    <h1 lang="de">${escapeXml(title)}</h1>
  </div>
  <div>
    <p class="author">${escapeXml(author)}</p>
    <div class="rule bottom"></div>
  </div>
</body>
</html>`;
}

export async function renderCovers(books) {
  const browser = await playwright.chromium.launch();
  try {
    const context = await browser.newContext({
      viewport: { width: COVER_WIDTH, height: COVER_HEIGHT },
      deviceScaleFactor: 1,
      colorScheme: 'light',
    });
    const page = await context.newPage();
    const covers = new Map();
    for (const book of books) {
      await page.setContent(coverHtml(book), { waitUntil: 'load' });
      await page.evaluate(() => document.fonts.ready);
      covers.set(book.gutenberg, await page.screenshot({ type: 'png', fullPage: false }));
    }
    await context.close();
    return covers;
  } finally {
    await browser.close();
  }
}

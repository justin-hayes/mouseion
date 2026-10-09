// Fetches the pinned Gutenberg sources, verifies them, and prepares the EPUBs,
// covers, and static OPDS catalog that the isolated stack serves. Everything it
// writes lives under the repository's ignored .tmp directory.
import { createHash } from 'node:crypto';
import { copyFile, mkdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { renderCovers } from './cover.mjs';
import { bodyParagraphs, buildEpub, sourceCredits, stripGutenberg } from './epub.mjs';
import { catalogFiles } from './opds.mjs';

export const USER_AGENT =
  'MouseionDocsScreenshot/1.0 (+https://github.com/justin-hayes/mouseion; documentation screenshot, one request at a time)';
const DOWNLOAD_TIMEOUT_MS = 120_000;
const POLITE_DELAY_MS = 1_000;

export class ScreenshotError extends Error {}

export function sourceUrl(book) {
  return `https://www.gutenberg.org/ebooks/${book.gutenberg}.txt.utf-8`;
}

export function sha256(buffer) {
  return createHash('sha256').update(buffer).digest('hex');
}

async function exists(path) {
  try {
    return (await stat(path)).isFile();
  } catch {
    return false;
  }
}

async function writeAtomic(path, data) {
  await mkdir(join(path, '..'), { recursive: true });
  const temporary = `${path}.partial`;
  await writeFile(temporary, data);
  await rename(temporary, path);
}

// Returns the verified source text for one book. A cached file is reused only
// while its checksum matches; a stale cache entry is replaced by one download.
// A downloaded file whose checksum differs is never written to the cache.
export async function fetchSource(book, sourcesDir, log) {
  const path = join(sourcesDir, `${book.gutenberg}.txt`);
  if (await exists(path)) {
    const cached = await readFile(path);
    if (sha256(cached) === book.sha256) {
      log(`  cached   #${book.gutenberg} ${book.title}`);
      return cached;
    }
    log(`  stale    #${book.gutenberg} cached copy does not match the pinned checksum; downloading again`);
    await rm(path, { force: true });
  }

  let response;
  try {
    response = await fetch(sourceUrl(book), {
      headers: { 'user-agent': USER_AGENT, accept: 'text/plain' },
      redirect: 'follow',
      signal: AbortSignal.timeout(DOWNLOAD_TIMEOUT_MS),
    });
  } catch (error) {
    throw new ScreenshotError(
      `Cannot download Project Gutenberg #${book.gutenberg} (${sourceUrl(book)}): ${error.message}. ` +
        'Check network access to www.gutenberg.org, or place the pinned file at ' +
        `${path} and rerun. No images were written.`,
    );
  }
  if (!response.ok) {
    throw new ScreenshotError(
      `Project Gutenberg #${book.gutenberg} returned HTTP ${response.status}. Retry later. No images were written.`,
    );
  }
  const data = Buffer.from(await response.arrayBuffer());
  const actual = sha256(data);
  if (actual !== book.sha256) {
    throw new ScreenshotError(
      `Checksum mismatch for Project Gutenberg #${book.gutenberg} (${book.title}): expected ${book.sha256}, got ${actual}. ` +
        'Project Gutenberg may have revised this edition. Review e2e/screenshot/manifest.json and the new text, then update the pinned checksum deliberately. No images were written.',
    );
  }
  await writeAtomic(path, data);
  log(`  download #${book.gutenberg} ${book.title}`);
  return data;
}

// The Gutenberg header and license must not survive into the body matter.
// The colophon is generated afterwards and may name the source.
function assertNoGutenbergBoilerplate(book, bodyText) {
  const pattern = /Project Gutenberg|\*\*\* ?(?:START|END) OF|gutenberg\.org/i;
  const match = pattern.exec(bodyText);
  if (match) {
    throw new ScreenshotError(
      `${book.title}: Gutenberg boilerplate ("${match[0]}") remained after stripping. Update the stripping rules before using this edition.`,
    );
  }
}

export async function prepareLibrary({ manifest, paths, log = console.log }) {
  const { books } = manifest;
  await mkdir(paths.sources, { recursive: true });

  log('Fetching and verifying Project Gutenberg sources (one request at a time)');
  const sources = new Map();
  for (const book of books) {
    const before = await exists(join(paths.sources, `${book.gutenberg}.txt`));
    sources.set(book.gutenberg, await fetchSource(book, paths.sources, log));
    if (!before) await new Promise(resolve => setTimeout(resolve, POLITE_DELAY_MS));
  }

  log('Preparing EPUBs and covers');
  const editions = books.map(book => {
    const raw = sources.get(book.gutenberg).toString('utf8');
    const work = bodyParagraphs(stripGutenberg(raw));
    if (work.length === 0) throw new ScreenshotError(`${book.title}: no body text remained after removing the Gutenberg header.`);
    assertNoGutenbergBoilerplate(book, work.join('\n'));
    return { book, work, credits: sourceCredits(raw) };
  });
  const covers = await renderCovers(books);

  await rm(paths.catalog, { recursive: true, force: true });
  await mkdir(join(paths.catalog, 'opds', 'books'), { recursive: true });
  await mkdir(join(paths.catalog, 'opds', 'covers'), { recursive: true });
  for (const { book, work, credits } of editions) {
    const cover = covers.get(book.gutenberg);
    const epub = buildEpub({ book, bodyParagraphs: work, credits, cover });
    await writeAtomic(join(paths.books, `${book.gutenberg}.epub`), epub);
    await writeAtomic(join(paths.covers, `${book.gutenberg}.png`), cover);
    await copyFile(join(paths.books, `${book.gutenberg}.epub`), join(paths.catalog, 'opds', 'books', `${book.gutenberg}.epub`));
    await copyFile(join(paths.covers, `${book.gutenberg}.png`), join(paths.catalog, 'opds', 'covers', `${book.gutenberg}.png`));
    log(`  epub     #${book.gutenberg} ${book.title} (${epub.length} bytes)`);
  }
  for (const feed of catalogFiles(books)) {
    await writeAtomic(join(paths.catalog, feed.path), feed.body);
  }
  return { books: editions.map(edition => edition.book) };
}

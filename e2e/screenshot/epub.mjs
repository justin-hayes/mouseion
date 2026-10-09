// Repackages a Project Gutenberg plain-text edition as a clean EPUB 3 file.
// Pure functions only: the caller supplies the source text and cover bytes.
import { zipEntries } from './zip.mjs';

// Fixed so that rebuilding from the same source produces the same bytes.
const MODIFIED = '2026-10-09T00:00:00Z';

// Ordinal words that open a chapter heading in German editions.
const ORDINALS =
  'Erste|Zweite|Dritte|Vierte|Fünfte|Sechste|Siebente|Achte|Neunte|Zehnte|Elfte|Zwölfte|Dreizehnte|Vierzehnte|Fünfzehnte|Sechzehnte|Siebzehnte|Achtzehnte|Neunzehnte|Zwanzigste|' +
  'Erstes|Zweites|Drittes|Viertes|Fünftes|Sechstes|Siebentes|Achtes|Neuntes|Zehntes|' +
  'Erster|Zweiter|Dritter|Vierter|Fünfter|Sechster|Siebenter|Achter|Neunter|Zehnter';
// Numerals stay case-sensitive so that ordinary lowercase words are not headings.
// Word headings are matched case-insensitively because editions print them in
// capitals (ERSTES KAPITEL) or in title case (Erstes Kapitel).
const NUMERAL_HEADING = /^(?:[IVXLCDM]+\.?|\d+\.?)$/;
const WORD_HEADING = new RegExp(
  `^(?:(?:${ORDINALS})(?:\\s+(?:Kapitel|Vigilie|Teil|Buch|Abschnitt|Stück|Brief))?|` +
    '(?:Kapitel|Teil|Buch|Abschnitt|Chapter)\\s+[IVXLCDM\\d]+\\.?)$',
  'i',
);
// Scene breaks such as "*   *   *" carry no text and are dropped.
const SCENE_BREAK = /^[*\s]+$/;
const MAX_HEADING_LENGTH = 60;

const START_MARKER = /^\*\*\* ?START OF (?:THE|THIS) PROJECT GUTENBERG EBOOK[^\n]*\*\*\*[ \t]*$/m;
const END_MARKER = /^\*\*\* ?END OF (?:THE|THIS) PROJECT GUTENBERG EBOOK[^\n]*\*\*\*[ \t]*$/m;
const TRANSCRIBER_NOTE = /\[\s*Anmerkungen zur Transkription:[\s\S]*?\]/g;

// Returns the header metadata that the colophon credits (release date and the
// credits block) without the license text that surrounds it.
export function sourceCredits(raw) {
  const text = normalize(raw);
  const header = text.slice(0, Math.max(0, startMarker(text).index));
  const release = /^Release date:\s*(.+)$/m.exec(header)?.[1].trim() ?? '';
  const credits = /^Credits:\s*([\s\S]*?)(?:\n\s*\n|$)/m.exec(header)?.[1].replace(/\s+/g, ' ').trim() ?? '';
  return { release, credits };
}

// Extracts the work between the Gutenberg START and END markers and removes the
// transcriber's notes that Project Gutenberg places inside the marked region.
export function stripGutenberg(raw) {
  const text = normalize(raw);
  const start = startMarker(text);
  const endMatch = END_MARKER.exec(text);
  if (!endMatch || endMatch.index <= start.index) {
    throw new Error('Project Gutenberg END marker not found after the START marker');
  }
  return text
    .slice(start.index + start[0].length, endMatch.index)
    .replace(TRANSCRIBER_NOTE, '');
}

function startMarker(text) {
  const match = START_MARKER.exec(text);
  if (!match) throw new Error('Project Gutenberg START marker not found');
  return match;
}

function normalize(raw) {
  return raw.replace(/^﻿/, '').replace(/\r\n?/g, '\n');
}

// Splits marked text into paragraphs. Gutenberg hard-wraps lines, so the lines
// of a paragraph are joined with single spaces.
export function paragraphs(body) {
  return body
    .split(/\n[ \t]*\n/)
    .map(block => block.split('\n').map(line => line.trim()).filter(Boolean).join(' '))
    .filter(paragraph => paragraph && !SCENE_BREAK.test(paragraph));
}

// Gutenberg places production credits and file pointers at the top of the
// marked region, before the work itself. They are removed only as a leading
// run, so the same words later in the text are never touched.
const PRODUCTION_NOTE =
  /Gutenberg|PG-DE|pgdp|Produced by|prepared by|Korrektur von Satzfehlern|gesperrter Text|https?:\/\/|Transkription|Transcriber/i;

export function dropLeadingProductionNotes(paras) {
  let index = 0;
  while (index < paras.length && PRODUCTION_NOTE.test(paras[index])) index++;
  return paras.slice(index);
}

// The paragraphs of the work itself, with production notes removed.
export function bodyParagraphs(bodyText) {
  return dropLeadingProductionNotes(paragraphs(bodyText));
}

export function isHeading(paragraph) {
  return (
    paragraph.length <= MAX_HEADING_LENGTH && (NUMERAL_HEADING.test(paragraph) || WORD_HEADING.test(paragraph))
  );
}

// Groups paragraphs into chapters at heading paragraphs. Text before the first
// heading becomes an untitled chapter so no source text is dropped.
export function splitChapters(paras) {
  const chapters = [];
  let current = { title: null, paragraphs: [] };
  for (const paragraph of paras) {
    if (isHeading(paragraph)) {
      if (current.title !== null || current.paragraphs.length) chapters.push(current);
      current = { title: paragraph, paragraphs: [] };
    } else {
      current.paragraphs.push(paragraph);
    }
  }
  if (current.title !== null || current.paragraphs.length) chapters.push(current);
  return chapters;
}

export function escapeXml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;');
}

const STYLES = `body { font-family: serif; line-height: 1.5; margin: 5% 8%; }
h1 { font-size: 2em; text-align: center; margin-top: 30%; }
h2 { font-size: 1.4em; margin: 2em 0 1em; text-align: center; }
p { margin: 0 0 0.8em; text-indent: 1.2em; text-align: justify; }
p.first, h2 + p { text-indent: 0; }
.author { text-align: center; font-size: 1.2em; font-style: italic; }
.colophon p { text-indent: 0; font-size: 0.9em; }
`;

function xhtmlPage({ title, bodyType, body, bodyClass }) {
  const classAttr = bodyClass ? ` class="${bodyClass}"` : '';
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="de" lang="de">
<head>
<meta charset="utf-8"/>
<title>${escapeXml(title)}</title>
<link rel="stylesheet" type="text/css" href="styles.css"/>
</head>
<body epub:type="${bodyType}"${classAttr}>
${body}
</body>
</html>
`;
}

// Builds a complete EPUB 3 package. `book` carries the manifest identity,
// `bodyParagraphs` is the output of bodyParagraphs(), `credits` of
// sourceCredits(), and `cover` is a PNG buffer.
export function buildEpub({ book, bodyParagraphs: work, credits, cover }) {
  const chapters = splitChapters(work);
  if (chapters.length === 0) throw new Error(`${book.title}: no body text after removing the Gutenberg header and license`);

  const title = escapeXml(book.title);
  const author = escapeXml(book.author);
  const sourceUrl = `https://www.gutenberg.org/ebooks/${book.gutenberg}`;

  const chapterFiles = chapters.map((chapter, index) => {
    const id = `chapter-${String(index + 1).padStart(2, '0')}`;
    const heading = chapter.title ? `<h2>${escapeXml(chapter.title)}</h2>\n` : '';
    const paragraphsXml = chapter.paragraphs.map(p => `<p>${escapeXml(p)}</p>`).join('\n');
    const body = `<section epub:type="chapter" id="${id}">\n${heading}${paragraphsXml}\n</section>`;
    return {
      id,
      href: `${id}.xhtml`,
      label: chapter.title ?? 'Textbeginn',
      xhtml: xhtmlPage({ title: chapter.title ?? book.title, bodyType: 'bodymatter', body }),
    };
  });

  const coverPage = xhtmlPage({
    title: book.title,
    bodyType: 'cover',
    body: '<section><img src="cover.png" alt="Umschlag: ' + title + ' von ' + author + '" style="width:100%"/></section>',
    bodyClass: 'cover',
  });

  const titlePage = xhtmlPage({
    title: book.title,
    bodyType: 'frontmatter',
    body: `<section class="titlepage" epub:type="titlepage">\n<h1>${title}</h1>\n<p class="author">${author}</p>\n</section>`,
  });

  const colophonLines = [
    `Quelle: ${book.title} von ${book.author}. Project Gutenberg eBook #${book.gutenberg}, Veröffentlichung ${credits.release || 'laut Quellenangabe'}.`,
    `Abrufbar unter ${sourceUrl}.`,
    credits.credits ? `Erstellt von: ${credits.credits}` : '',
    'Der Text ist gemeinfrei. Kopfzeile und Lizenzhinweise der Quelle wurden für diese Fassung entfernt.',
  ].filter(Boolean);
  const colophon = xhtmlPage({
    title: 'Kolophon',
    bodyType: 'backmatter',
    body: `<section class="colophon" epub:type="colophon">\n<h2>Kolophon</h2>\n${colophonLines.map(line => `<p>${escapeXml(line)}</p>`).join('\n')}\n</section>`,
  });

  const pages = [
    { id: 'cover', href: 'cover.xhtml', label: 'Umschlag', xhtml: coverPage, landmark: 'cover' },
    { id: 'title', href: 'title.xhtml', label: 'Titelseite', xhtml: titlePage, landmark: 'frontmatter' },
    ...chapterFiles.map((file, index) => ({ ...file, landmark: index === 0 ? 'bodymatter' : undefined })),
    { id: 'colophon', href: 'colophon.xhtml', label: 'Kolophon', xhtml: colophon, landmark: 'backmatter' },
  ];

  const nav = navDocument(book, pages);
  const opf = packageDocument(book, pages, sourceUrl);
  const container = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
<rootfiles>
<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
</rootfiles>
</container>
`;

  return zipEntries([
    { name: 'mimetype', data: Buffer.from('application/epub+zip', 'ascii') },
    { name: 'META-INF/container.xml', data: Buffer.from(container, 'utf8') },
    { name: 'OEBPS/content.opf', data: Buffer.from(opf, 'utf8') },
    { name: 'OEBPS/nav.xhtml', data: Buffer.from(nav, 'utf8') },
    { name: 'OEBPS/styles.css', data: Buffer.from(STYLES, 'utf8') },
    { name: 'OEBPS/cover.png', data: cover },
    ...pages.map(page => ({ name: `OEBPS/${page.href}`, data: Buffer.from(page.xhtml, 'utf8') })),
  ]);
}

function navDocument(book, pages) {
  const items = pages.map(page => `<li><a href="${page.href}">${escapeXml(page.label)}</a></li>`).join('\n');
  const landmarks = pages
    .filter(page => page.landmark)
    .map(page => `<li><a epub:type="${page.landmark}" href="${page.href}">${escapeXml(page.label)}</a></li>`)
    .join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="de" lang="de">
<head>
<meta charset="utf-8"/>
<title>${escapeXml(book.title)}</title>
</head>
<body>
<nav epub:type="toc" id="toc">
<h1>Inhalt</h1>
<ol>
${items}
</ol>
</nav>
<nav epub:type="landmarks" id="landmarks">
<h2>Struktur</h2>
<ol>
${landmarks}
</ol>
</nav>
</body>
</html>
`;
}

function packageDocument(book, pages, sourceUrl) {
  const id = `urn:mouseion:screenshot:gutenberg:${book.gutenberg}`;
  const manifest = [
    '<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>',
    '<item id="styles" href="styles.css" media-type="text/css"/>',
    '<item id="cover-image" href="cover.png" media-type="image/png" properties="cover-image"/>',
    ...pages.map(page => `<item id="${page.id}" href="${page.href}" media-type="application/xhtml+xml"/>`),
  ].join('\n');
  const spine = pages.map(page => `<itemref idref="${page.id}"/>`).join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id" xml:lang="de">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="book-id">${id}</dc:identifier>
<dc:title>${escapeXml(book.title)}</dc:title>
<dc:creator id="creator">${escapeXml(book.author)}</dc:creator>
<dc:language>de</dc:language>
<dc:source>${sourceUrl}</dc:source>
<meta property="dcterms:modified">${MODIFIED}</meta>
<meta name="cover" content="cover-image"/>
</metadata>
<manifest>
${manifest}
</manifest>
<spine>
${spine}
</spine>
</package>
`;
}

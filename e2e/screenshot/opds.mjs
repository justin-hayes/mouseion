// Static OPDS 1.2 feeds in the Calibre-Web shape that Mouseion's catalogue client
// walks: a root feed, a language navigation feed, and one numeric language feed
// of acquisition entries. Paths are relative to the catalog root.
import { escapeXml } from './epub.mjs';

export const CATALOG_ROOT_PATH = '/opds';
export const LANGUAGE_ID = 1;
export const LANGUAGE_NAME = 'German';

const NAVIGATION = 'application/atom+xml;profile=opds-catalog;kind=navigation';
const ACQUISITION = 'application/atom+xml;profile=opds-catalog;kind=acquisition';
const ACQUISITION_REL = 'http://opds-spec.org/acquisition';
const IMAGE_REL = 'http://opds-spec.org/image';
const THUMBNAIL_REL = 'http://opds-spec.org/image/thumbnail';
const UPDATED = '2026-10-09T00:00:00Z';

// Returns [{ path, body }] for every feed and every book file the catalog
// serves. Book and cover file names use the manifest's Gutenberg identifier.
export function catalogFiles(books) {
  const root = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<id>urn:mouseion:screenshot:catalog</id>
<title>Mouseion screenshot catalog</title>
<updated>${UPDATED}</updated>
<link rel="self" href="${CATALOG_ROOT_PATH}" type="${NAVIGATION}"/>
<link rel="start" href="${CATALOG_ROOT_PATH}" type="${NAVIGATION}"/>
<entry>
<id>urn:mouseion:screenshot:languages</id>
<title>Languages</title>
<updated>${UPDATED}</updated>
<content type="text">Browse by language</content>
<link rel="subsection" href="${CATALOG_ROOT_PATH}/language" type="${NAVIGATION}"/>
</entry>
</feed>
`;

  const languages = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<id>urn:mouseion:screenshot:languages:feed</id>
<title>Languages</title>
<updated>${UPDATED}</updated>
<link rel="self" href="${CATALOG_ROOT_PATH}/language" type="${NAVIGATION}"/>
<link rel="start" href="${CATALOG_ROOT_PATH}" type="${NAVIGATION}"/>
<entry>
<id>urn:mouseion:screenshot:language:${LANGUAGE_ID}</id>
<title>${LANGUAGE_NAME}</title>
<updated>${UPDATED}</updated>
<content type="text">${books.length} books</content>
<link rel="subsection" href="${CATALOG_ROOT_PATH}/language/${LANGUAGE_ID}" type="${ACQUISITION}"/>
</entry>
</feed>
`;

  const entries = books.map(book => {
    const epub = `${CATALOG_ROOT_PATH}/books/${book.gutenberg}.epub`;
    const cover = `${CATALOG_ROOT_PATH}/covers/${book.gutenberg}.png`;
    return `<entry>
<id>urn:mouseion:screenshot:book:${book.gutenberg}</id>
<title>${escapeXml(book.title)}</title>
<updated>${UPDATED}</updated>
<author><name>${escapeXml(book.author)}</name></author>
<link rel="${ACQUISITION_REL}" href="${epub}" type="application/epub+zip"/>
<link rel="${IMAGE_REL}" href="${cover}" type="image/png"/>
<link rel="${THUMBNAIL_REL}" href="${cover}" type="image/png"/>
</entry>`;
  });

  const acquisition = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<id>urn:mouseion:screenshot:language:${LANGUAGE_ID}:feed</id>
<title>${LANGUAGE_NAME}</title>
<updated>${UPDATED}</updated>
<link rel="self" href="${CATALOG_ROOT_PATH}/language/${LANGUAGE_ID}" type="${ACQUISITION}"/>
<link rel="start" href="${CATALOG_ROOT_PATH}" type="${NAVIGATION}"/>
<link rel="up" href="${CATALOG_ROOT_PATH}/language" type="${NAVIGATION}"/>
${entries.join('\n')}
</feed>
`;

  return [
    { path: 'opds.atom', body: root },
    { path: 'opds/language.atom', body: languages },
    { path: `opds/language/${LANGUAGE_ID}.atom`, body: acquisition },
  ];
}

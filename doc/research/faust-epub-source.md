# German *Faust, Part One* EPUB source for the MVP acceptance case

**Research date:** 2026-10-17

**Ticket:** [Research: choose a structurally useful German Faust I EPUB](https://github.com/justin-hayes/mouseion/issues/2145)
(parent map: [Poetry support in Mouseion: Faust I decision map](https://github.com/justin-hayes/mouseion/issues/2143))

**Scope:** Identify a pinned, publicly available German *Faust I* EPUB that can
serve as the MVP acceptance source through Mouseion's existing OPDS
catalog/acquisition route, and record what its packaged markup does and does
not carry. This is evidence for a human decision. It is **not** an accepted
source, not an accepted structural model, and not legal advice.

**Status:** Research findings on the throwaway branch `research/faust-epub-source`.
Not for merge; no PR.

## Summary

- **Recommended candidate (agent recommendation only):** Project Gutenberg
  eBook **#21000**, *Faust: Eine Tragödie [erster Teil]*, proofread by
  Distributed Proofreaders (PGDP) from scans of the Klassik Stiftung Weimar copy.
  Its EPUB 2 "older e-readers, no images" package is the smallest of its
  variants, and its XHTML is byte-identical to the "with images" EPUB 2 package.
- Its markup is more explicit than the other candidate: 922 speaker paragraphs
  (`<p class="speaker">`), 194 stage-direction paragraphs in dedicated classes,
  and verse lines separated by `<br/>`. Its gaps are real and must be handled
  by the eventual structural contract: no markup distinguishes verse from prose,
  stanza breaks are only sometimes paragraph boundaries, and verse lines are
  sometimes one `<p>` each.
- Gutenberg #2229, *Faust: Der Tragödie erster Teil*, is the weaker candidate:
  it has verse lines (`<br/>`) but **no** speaker or stage-direction markup
  (0 speaker and 0 stage blocks); speakers and directions exist only as text.
- ADR 0066's landmark-based main-text selection does **not** narrow either
  candidate: neither carries a `bodymatter` landmark, so analysis falls back to
  the whole snapshot, including the printed title page and the Gutenberg
  header/licence text.
- The OPDS feed exposes two entries per Gutenberg number (no-images and
  with-images) with different entry IDs. The repo's acquisition-link selection
  is deterministic per entry, but the catalog would list the same work twice.

## Candidates and provenance

All downloads were made on 2026-10-17 (UTC, around 09:49) from the Gutenberg
URLs shown. Digests are SHA-256 of the downloaded file. Gutenberg updates files
(#2229's landing page says "Most recently updated: June 11, 2026"), so any pin
must be re-verified against the digest at the time of pinning.

| Edition | Gutenberg # | Variant | OPDS entry ID | Acquisition link (as advertised) | Bytes | SHA-256 |
| --- | --- | --- | --- | --- | --- | --- |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 2, no images | `urn:gutenberg:21000:2` | `https://www.gutenberg.org/ebooks/21000.epub.noimages` → `…/cache/epub/21000/pg21000.epub` | 179103 | `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01` |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 2, with images | `urn:gutenberg:21000:3` | `https://www.gutenberg.org/ebooks/21000.epub.images` → `…/pg21000-images.epub` | 184579 | `846b286ea74770cbb3a321332e4d542ecd654f06b5de020530e9a6065d08a1bf` |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 3, with images | `urn:gutenberg:21000:3` | `https://www.gutenberg.org/ebooks/21000.epub3.images` | 183390 | `9bd848806b6aaaabf830edd607665be7d3ad883d33b1e7c71602ba6e36681809` |
| *Der Tragödie erster Teil* (Pullen) | 2229 | EPUB 2, no images | `urn:gutenberg:2229:2` | `https://www.gutenberg.org/ebooks/2229.epub.noimages` | 184272 | `5176b65907c1b3fe2236cb92b9c791775b60ee218000a03c2e3932dfbbe3ed5d` |
| *Der Tragödie erster Teil* (Pullen) | 2229 | EPUB 2, with images | `urn:gutenberg:2229:3` | `https://www.gutenberg.org/ebooks/2229.epub.images` | 191405 | `6e0ca94e1a76c247983f5e680c17f15763ace9dfade35714aa1b58e1788877ea` |

Sources:

- Gutenberg landing pages: <https://www.gutenberg.org/ebooks/21000> and <https://www.gutenberg.org/ebooks/2229>.
- Gutenberg OPDS entries: <https://www.gutenberg.org/ebooks/21000.opds> and <https://www.gutenberg.org/ebooks/2229.opds>. Each lists two `<entry>` elements (no-images and with-images), each with EPUB acquisition links, and the `length` attributes match the downloaded byte counts above.
- Gutenberg's file redirects were followed with `curl -L`. The byte counts match the OPDS `length` values.

Other candidates considered and not inspected at package level:

- **Projekt Gutenberg-DE**, <https://projekt-gutenberg.org/authors/johann-wolfgang-von-goethe/books/faust-eine-tragoedie/>. The page offers "Als EPUB herunterladen" (a button, with no stable direct file URL found), and cites a Reclam Verlag 1971 print edition. I did not find an explicit licence statement for the text on the fetched page, so its rights evidence is weaker than Gutenberg's and I did not download or inspect its EPUB.
- Gutenberg's own **#2229 EPUB 3 with images** (OPDS entry `urn:gutenberg:2229:3`, 192387 bytes advertised) was not downloaded or inspected.

## Rights evidence

Sources, with what each one shows:

1. **Package metadata.** Each EPUB's `content.opf` carries `<dc:rights>Public domain in the USA.</dc:rights>`, `<dc:language>de</dc:language>`, and the creator *Johann Wolfgang von Goethe* (file-as "Goethe, Johann Wolfgang von"). The `pg-header` boilerplate in the #21000 package points to the Project Gutenberg licence for redistribution terms. The #21000 landing page lists the author as "Goethe, Johann Wolfgang von, 1749-1832", which gives the death year used below.
2. **Gutenberg licence statement inside the package.** `OEBPS/8980964947130898904_21000-h-6.htm.html` (the footer file, EPUB 2 no-images package) contains `*** END OF THE PROJECT GUTENBERG EBOOK FAUST: EINE TRAGÖDIE [ERSTER TEIL] ***` followed by the Project Gutenberg licence text. The marker appears at character 646 of 18,820 characters of that file. This file is part of the spine and is not separable from the prose by the EPUB's own structure (see below).
3. **Gutenberg permissions policy.** <https://www.gutenberg.org/policy/permission.html> says that "the vast majority of Project Gutenberg eBooks are in the public domain in the US", that PG "claims no copyright for markup, formatting, spelling modernization, etc.", and that PG is "US only": "Not all items that are public domain in the US are public domain in other countries, and vice versa." It also says "No permission is needed for non-commercial use", and that Gutenberg does not grant permission for public-domain items.
4. **German term (jurisdiction check).** § 64 UrhG: "Das Urheberrecht erlischt siebzig Jahre nach dem Tode des Urhebers." (<https://www.gesetze-im-internet.de/urhg/__64.html>). § 69 UrhG: the periods "beginnen mit dem Ablauf des Kalenderjahres, in dem das für den Beginn der Frist maßgebende Ereignis eingetreten ist" (<https://www.gesetze-im-internet.de/urhg/__69.html>). Applied to a death in 1832, the calendar-year rule ends the term on 31 December 1902, so the author's work would be in the public domain in Germany from 1 January 1903 under these two provisions. This is my reading of the statute, not a legal opinion.

Caveats:

- The public-domain status of the *text* is well supported for the US and for the German author's term. The status of the **specific Gutenberg transcription**, its proofreading, and its markup is Gutenberg's claim (PG says it claims no copyright in markup and formatting; see item 3).
- Scans come from Klassik Stiftung Weimar / Herzogin Anna Amalia Bibliothek (per the #21000 metadata). I did not examine whether any separate scan, edition, or database right applies in Germany or elsewhere. Scholarly-edition protection (§ 70 UrhG) was not examined.
- The package points to the "Project Gutenberg License" for redistribution terms. I did not read that licence text in full in this session, so its terms are unverified here.
- Nothing here is legal advice. The owning human decision should confirm rights for the intended use.

## Package structure

### #21000, EPUB 2, no images (candidate)

- `mimetype`, `META-INF/container.xml` → `OEBPS/content.opf` (OPF version 2.0).
- Manifest: `pgepub.css`, `0.css`, a cover PNG, seven XHTML chunks (`…_21000-h-0.htm.html` through `…-h-6.htm.html`), `toc.ncx`, and a `wrap0000.html` cover page.
- Spine: cover page, `pg-header`, then six chunks (`item4`–`item9`). The `pg-header` chunk (`-h-0`) contains the Gutenberg header boilerplate, the printed title page, and the sections from Zueignung through Vor dem Thor (the NCX `pgepubid` anchors place them there).
- Navigation: `toc.ncx` only. No `nav.xhtml`, and no `epub:type` anywhere in the package (checked).
- `<guide>` contains a single `cover` reference. There are no landmarks.
- Scene headings are `<h2>` (Vorspiel, Prolog im Himmel, …) and `<h3>` (sub-scenes such as "Trüber Tag."). The NCX lists scenes with fragment links such as `…-h-0.htm.html#pgepubid00006`.

### #21000, EPUB 3, with images (variant)

- `OEBPS/content.opf` (OPF version 3.0), `OEBPS/toc.xhtml` marked `properties="nav"`, and a `toc.ncx`.
- The `landmarks` nav in `toc.xhtml` is titled "Page List" and contains only page-number links typed `epub:type="normal"`. The `toc` nav and the body of `toc.xhtml` are `frontmatter`.
- There is **no** `bodymatter` or `backmatter` landmark anywhere in the package (checked by grep over all files).
- The same markup counts as the EPUB 2 package (see census), but packaged in **four** XHTML chunks (`…-h-0.htm.xhtml` to `…-h-3.htm.xhtml`) rather than seven, so chunk boundaries differ from the EPUB 2 package.

### #2229, EPUB 2, no images (comparison)

- OPF 2.0, `toc.ncx`, 29 XHTML chapter files, each `<div class="chapter">` with a `<h2>` heading. No `nav`, no `epub:type`, no landmarks.
- The chapter files are the Gutenberg/Pullen transcription. The metadata credits Michael Pullen (2000 release; updated 2010 and 2026 according to the landing page).

### What the package structure means for ADR 0066

[ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md) identifies main text only from declared EPUB 3 `bodymatter` landmarks, and falls back to the whole snapshot when none is declared. Neither candidate declares `bodymatter`, so **analysis of either candidate analyzes the whole snapshot**. That includes the Gutenberg header and footer, the printed title page and transcriber notes in the front matter, and the Gutenberg licence text in the footer. This is a fact about the current contract, not a recommendation to change it. The owning human decision needs to decide whether that fallback is acceptable for this MVP.

## Markup census

Counts come from parsing every XHTML chunk in each package. "Blocks" are `<p>`, `<h2>`, `<h3>` and `<h1>` elements, with their text.

| Measure | #21000 EPUB 2 (no images) | #2229 EPUB 2 (no images) |
| --- | --- | --- |
| XHTML chunks | 7 | 29 |
| `<br/>` elements | 3678 | 4793 |
| Verse-like blocks (contain `<br/>`) | 706 | 1019 |
| Verse lines (non-empty, `<br/>`-delimited) | ~4376 | ~5809 |
| Blocks without `<br/>` (of which >150 chars) | 235 (9) | 30 (0) |
| Speaker paragraphs `<p class="speaker">` | **922** | **0** |
| Stage-direction paragraphs (`entrance`, `sdcenter`, `sdsetting`, `dirlong`, `dirlongind`) | **194** | **0** |
| Inline `<span class="speaker">` | yes (e.g. h-2 line 346) | no |
| Run-in stage directions and speaker names in plain text | not counted | **yes** (e.g. "Faust. Margarete vorübergehend.") |
| `<ins class="correction">` (transcriber corrections) | 25 | 0 |
| `<em class="gesperrt">` (spaced emphasis, styled `font-style: normal`) | 13 | 0 |
| `x-ebookmaker-pageno` spans (printed page markers) | 301 | 0 |

The EPUB 3 variant of #21000 shows the same counts for speaker (922), `<br/>` (3678) and stage directions (`sdcenter`, 128).

The EPUB 2 no-images and with-images packages of #21000 have **byte-identical XHTML** (zero changed lines across all seven chunks; only the cover image differs). The no-image variant is therefore the same text and markup without illustrations.

## Markup samples

Sample lines are quoted for structure only. Each is a short fragment of the packaged file, with the path relative to `OEBPS/`. Full text is not reproduced.

### Verse lines, `<br/>`, and incidental source newlines (#21000)

`8980964947130898904_21000-h-0.htm.html` lines 90–92:

```html
<p>Ihr beyden die ihr mir so oft,<br/>
In Noth und Trübsal, beygestanden,<br/>
Sagt was ihr wohl, in deutschen Landen,<br/>
```

- Verse lines are delimited by `<br/>`. The newline after each `<br/>` is a source-formatting newline, not a meaningful break. The same file has 3876 raw source newlines inside blocks; collapsing whitespace and splitting on `<br/>` gives the verse lines.
- The sentence "Ihr beyden … hofft?" runs across lines 90–93, so a sentence spans verse lines.
- A single verse line can contain two sentences: `…-h-0.htm.html` line 195: `Was fällt euch an? Entzückung oder Schmerzen?`

### Speaker and stage direction (#21000)

`…-h-0.htm.html` lines 89 and 148: `<p class="speaker">Director.</p>` and the speech that follows in the next `<p>`. Dedicated stage-direction classes:

```html
<p class="sdcenter">Der Himmel schließt, die Erzengel vertheilen sich,</p>
```

(`…-h-0.htm.html` line 445; the `sdcenter` class is used 128 times.) Mixed run-in case: `…-h-2.htm.html` line 346 has `<span class="speaker">Mephistopheles</span> mit seltsamen Geberden.` inside a stage-direction `<p>`.

### Stanza grouping (#21000 song, and the un-marked Vorspiel)

- The *Es war ein König in Thule* song (`…-h-3.htm.html` from line 140) is marked by **one `<p class="i12 indent">` per stanza**, so stanza boundaries are paragraph boundaries here.
- The Vorspiel's Director speech (`…-h-0.htm.html` lines 90–147) is **one `<p>`** with no class. It contains several groups of verse lines, and nothing in the markup shows where the groups end. Paragraph boundaries are therefore not a reliable stanza signal in general.

### Verse lines as paragraphs, and unmarked prose (#21000)

- `…-h-1.htm.html` line 108 is a single-line `<p>` with no `<br/>`: `<p>Siehst du den schwarzen Hund durch Saat und Stoppel streifen?</p>`. Some verse lines are one `<p>` each.
- In the "Trüber Tag." scene (heading at `…-h-5.htm.html` line 420), a speech attributed to Faust (`<p class="speaker">Faust.</p>` at line 424) is followed by an unbroken paragraph (`<p class="indent">Im Elend! Verzweifelnd! …`, line 425) of long sentences with no `<br/>` and no classes. Its length (9 such paragraphs over 150 characters in the file) and sentence structure suggest prose. **This is an inference from the text.** The markup does not say "prose", and I did not check it against a printed critical edition in this session.
- Nothing in the EPUB markup (classes, `epub:type`, `lang`) distinguishes verse from prose. Classification of verse vs. prose from the packaged file would be text-based judgement.

### Spaced emphasis and transcriber corrections (#21000)

- `…-h-0.htm.html` line 71: `Mein <ins class="correction" title="Transcriber's note: newer editions read 'Lied'">Leid</ins> ertönt der unbekannten Menge,`. The visible text is the older reading. The newer reading is only in the `title` attribute.
- `…-h-0.htm.html` line 412: `<ins class="correction" title="Transcriber's note: newer editions read 'Am'">An</ins> meisten lieb’ ich mir die vollen frischen Wangen.`
- `<em class="gesperrt">` (letter-spaced emphasis) is used for words such as "Wort" and "Sinn" in `…-h-1.htm.html` lines 206–219. It carries no semantic distinction beyond emphasis.

### Printed page markers (#21000)

- `<span class="x-ebookmaker-pageno" title="[10]" id="…"><a id="Page_10" title="[10]"></a></span>` appears 301 times, usually at the start of a line after a `<br/>`, and is not text. Three occurrences are preceded by text on the same line (approximate count), so the page-marker stripping rule must handle both positions.

### #2229 for comparison

- Speakers are plain uppercase text with no class, e.g. `5068675349464888044_2229-h-10.htm.html` lines 22 and 35: `FAUST.<br/>`. A stage direction is run-in plain text in its own `<p>`: line 18, `Faust. Margarete vorübergehend.`
- The "Es war ein König in Thule" song (`…-h-11.htm.html` line 172) is also verse with `<br/>`, and stanzas are plain `<p>` blocks with no class.
- Consequence: an analyzer cannot separate speaker labels or stage directions from verse in #2229 without a text-based rule.

## Catalog and acquisition route

- The catalog route is the repo's OPDS client. `internal/opds/client.go` (`FindEPUBs`, around lines 210–216) selects links whose `rel` is the OPDS acquisition relation and whose type is `application/epub+zip`. `internal/cataloguesync/sync.go` (`acquisitionHref`, around lines 647–656) takes the **first** non-blank such link of the matched entry, and entries are matched by `entry.ID`.
- Gutenberg's feed gives two entries per number with distinct IDs. For #21000, the no-images entry (`urn:gutenberg:21000:2`) advertises EPUB 2 (no images) first, so the catalog would acquire the EPUB 2 no-images package. The with-images entry (`urn:gutenberg:21000:3`) advertises **EPUB 3** first, so the catalog would acquire the EPUB 3 package for that entry.
- Consequence: a catalog listing of this work would show two Books (or two entries) with the same title, differing only by the images option. The choice between them is a catalog-entry decision, not something the EPUB contract resolves.
- Redirects: the advertised `…/ebooks/21000.epub.noimages` URL returns a 302 to `…/cache/epub/21000/pg21000.epub`. I did not verify that the repo's downloader follows redirects; this is a limit to check before pinning.
- Sync is metadata-first and content is acquired lazily on "Move to To Read" (see [catalog sync feature](../features/catalog-sync.md), "Lazy content acquisition"). The acquisition path therefore runs only at that point.
- The e2e and development docs already pin Gutenberg German editions for screenshots and tests (`doc/development.md` around lines 205–220; `e2e/README.md` around lines 195–215; `e2e/screenshot/manifest.json` with SHA-256 pins). That is a repository precedent for a pinned, checksum-verified Gutenberg source; I did not change or extend that manifest.

## Limits

- **Unrun probes.** I did not run the NLP service, the ingest pipeline, the EPUB extractor, or the catalog sync against these packages. The statements about "the repo would acquire X" are read from code, not from an execution. The EPUB extraction contract (`internal/epub/`) was not run against either package.
- **Markup census is heuristic.** Verse-line and block counts come from a Python `html.parser` pass, not an XML parse. Figures are approximate (the "~" marks). The counts of speaker, stage and `br` elements are exact.
- **Verse vs. prose is not established.** The prose cases above are text-based inferences; I did not verify them against a printed critical edition.
- **Stanza structure is not established for the whole text.** Only the Thule song (in #21000) and the Vorspiel speech were examined closely. Other songs and speeches were sampled, not exhaustively checked.
- **Not checked:** Gutenberg #2229 EPUB 3; Projekt Gutenberg-DE's EPUB; any scan or edition-level rights outside the US and Germany; whether the repo's downloader follows redirects; whether the EPUB 3 nav's "Page List" is used by any extractor path.
- **Gutenberg files change.** Pin by digest, and re-verify the digest when pinning (see the provenance table).
- **No full text is reproduced here.** Samples are short fragments, and sentences are quoted only where they illustrate structure.

## Recommendation (agent recommendation, not acceptance)

For the MVP acceptance source, the **practical candidate** is Gutenberg #21000 via the EPUB 2 no-images package, OPDS entry `urn:gutenberg:21000:2` (SHA-256 `93d5a2bd…01`, 179103 bytes). Reasons:

1. It carries the explicit speaker and stage-direction markup the map asks about (922 speaker paragraphs, 194 stage-direction paragraphs).
2. Verse lines are explicit `<br/>` breaks, and stanza boundaries are paragraph boundaries in the song examined.
3. Its no-images package has identical text markup to the with-images package, and its EPUB 3 package has the same markup counts, so the choice between EPUB 2 and EPUB 3 is a packaging choice, not a content choice.
4. Its PGDP provenance and transcriber-correction notes are explicit in the markup.

The owning human decision should weigh the following before accepting this:

- Verse vs. prose and stanza grouping are not marked and need a text-based or structural rule.
- Transcriber corrections (`<ins class="correction">`) show older readings in visible text.
- The front matter and Gutenberg licence text are in the spine and fall into the whole-snapshot fallback under ADR 0066.
- The OPDS listing duplicates the work across the no-images and with-images entries.

Gutenberg #2229 is the comparison candidate. Its verse breaks are equally explicit, but it has no speaker or stage-direction markup, so it would need more text-based inference for the speaker and stage-direction features that the map's fidelity target names.

This recommendation is a research input. It is not human acceptance of the source, of the structural model, or of the rights position.

## Asset links

- Gutenberg #21000 landing page: <https://www.gutenberg.org/ebooks/21000>
- Gutenberg #21000 OPDS: <https://www.gutenberg.org/ebooks/21000.opds>
- Gutenberg #21000 no-images EPUB 2 (redirect target): <https://www.gutenberg.org/cache/epub/21000/pg21000.epub>
- Gutenberg #21000 with-images EPUB 2 (redirect target): <https://www.gutenberg.org/cache/epub/21000/pg21000-images.epub>
- Gutenberg #21000 EPUB 3 with images: <https://www.gutenberg.org/ebooks/21000.epub3.images>
- Gutenberg #2229 landing page: <https://www.gutenberg.org/ebooks/2229>
- Gutenberg #2229 OPDS: <https://www.gutenberg.org/ebooks/2229.opds>
- Gutenberg permissions policy: <https://www.gutenberg.org/policy/permission.html>
- § 64 UrhG: <https://www.gesetze-im-internet.de/urhg/__64.html>
- § 69 UrhG: <https://www.gesetze-im-internet.de/urhg/__69.html>
- Projekt Gutenberg-DE (not inspected): <https://projekt-gutenberg.org/authors/johann-wolfgang-von-goethe/books/faust-eine-tragoedie/>

Downloaded packages and extracted files are kept outside the repository, under
`~/.cache/scratch/poetry-faust-source/`, and are not committed.

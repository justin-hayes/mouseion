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

- **Agent recommendation (not acceptance):** Project Gutenberg eBook **#21000**,
  *Faust: Eine Tragödie [erster Teil]*, transcribed by Distributed Proofreaders
  (PGDP) from Klassik Stiftung Weimar scans. Its transcriber's note names the
  printed basis (an 1808 first edition, per the transcriber's note; quoted under *Printed-edition statements*). Its EPUB 2 "no images" package
  is the smallest variant, and its XHTML is byte-identical to the "with images"
  EPUB 2 package.
- Its markup is explicit: 922 `<p class="speaker">` paragraphs, 194
  stage-direction paragraphs in dedicated classes, and verse lines separated by
  `<br/>`. Its gaps are real: nothing in the markup distinguishes verse from
  prose, stanza breaks are only sometimes paragraph boundaries, some verse lines
  are one `<p>` each, and 25 transcriber corrections (`ins.correction`) show the
  older reading in the visible text.
- Gutenberg #2229, *Faust: Der Tragödie erster Teil* (Michael Pullen), is the
  comparison candidate. It has verse `<br/>` breaks but **no** `class` on any
  speaker or stage-direction paragraph. Speaker labels (for example `FAUST.`)
  and stage directions exist only as text, so separating them needs text-based
  rules. It uses modernised spelling and has no printed-edition statement in
  the package.
- ADR 0066's landmark-based main-text selection narrows neither candidate:
  neither package declares a `bodymatter` or `backmatter` landmark (grep over
  all six packages), so analysis falls back to the whole snapshot, including the
  Gutenberg header and licence text.
- The OPDS feed gives each Gutenberg number two entries with different IDs
  (`urn:gutenberg:N:2` no images, `urn:gutenberg:N:3` with images). Whether
  Mouseion would create one or two Books depends on which entries a connected
  feed actually exposes; see the catalog section for what is and is not
  established.

## Candidates and provenance

All six packages were downloaded on 2026-10-17 (UTC; the metadata fetch log
records 14:08:07Z) with `curl -L` from the URLs shown, and digests are SHA-256 of
the downloaded file (`sha256sum`). The five digests from the first research pass
re-verified unchanged; the 2229 EPUB 3 package was added in this pass. Gutenberg
changes files over time (the #2229 package header says "Most recently updated:
June 11, 2026"), so re-verify a digest at the time of pinning.

To reproduce, for example:

```sh
curl -L -o 21000-noimg.epub https://www.gutenberg.org/ebooks/21000.epub.noimages
sha256sum 21000-noimg.epub   # expect 93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01
```

| Edition | Gutenberg # | Variant | OPDS entry ID | Acquisition link (as advertised) | Bytes | SHA-256 |
| --- | --- | --- | --- | --- | --- | --- |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 2, no images | `urn:gutenberg:21000:2` | `https://www.gutenberg.org/ebooks/21000.epub.noimages` → `…/cache/epub/21000/pg21000.epub` | 179103 | `93d5a2bd657dc0ffdb39dbff0ab0979cb1229be05e1efbf69ca08691b1430b01` |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 2, with images | `urn:gutenberg:21000:3` | `https://www.gutenberg.org/ebooks/21000.epub.images` → `…/pg21000-images.epub` | 184579 | `846b286ea74770cbb3a321332e4d542ecd654f06b5de020530e9a6065d08a1bf` |
| *Eine Tragödie [erster Teil]* (PGDP) | 21000 | EPUB 3, with images | `urn:gutenberg:21000:3` | `https://www.gutenberg.org/ebooks/21000.epub3.images` | 183390 | `9bd848806b6aaaabf830edd607665be7d3ad883d33b1e7c71602ba6e36681809` |
| *Der Tragödie erster Teil* (Pullen) | 2229 | EPUB 2, no images | `urn:gutenberg:2229:2` | `https://www.gutenberg.org/ebooks/2229.epub.noimages` | 184272 | `5176b65907c1b3fe2236cb92b9c791775b60ee218000a03c2e3932dfbbe3ed5d` |
| *Der Tragödie erster Teil* (Pullen) | 2229 | EPUB 2, with images | `urn:gutenberg:2229:3` | `https://www.gutenberg.org/ebooks/2229.epub.images` | 191405 | `6e0ca94e1a76c247983f5e680c17f15763ace9dfade35714aa1b58e1788877ea` |
| *Der Tragödie erster Teil* (Pullen) | 2229 | EPUB 3, with images | `urn:gutenberg:2229:3` | `https://www.gutenberg.org/ebooks/2229.epub3.images` | 192387 | `c9f43aa402c72e24e7fb9c0090b89206cd29b649eb93b561cf3e5d8eace055b4` |

Sources:

- Gutenberg landing pages: <https://www.gutenberg.org/ebooks/21000> and <https://www.gutenberg.org/ebooks/2229>.
- Gutenberg OPDS entries: <https://www.gutenberg.org/ebooks/21000.opds> and <https://www.gutenberg.org/ebooks/2229.opds>. Each lists two entries (`:2` no images, `:3` with images). The `length` attributes match the downloaded byte counts above for every EPUB row. For entry `:3` the EPUB 3 link is listed before the EPUB 2 with-images link.
- The `.noimages` URL for #21000 returns a 302 to `https://www.gutenberg.org/cache/epub/21000/pg21000.epub`; the table's byte count is for the final file.

Other candidates considered and not inspected at package level:

- **Projekt Gutenberg-DE**, <https://projekt-gutenberg.org/authors/johann-wolfgang-von-goethe/books/faust-eine-tragoedie/>. The page offers "Als EPUB herunterladen" (a button, with no stable direct file URL found). I read only the main page in this pass: I did not find an explicit licence statement for the text there, and I did not download or inspect its EPUB. The draft's earlier Reclam 1971 citation was not re-verified.

## Rights evidence

Sources, with what each one shows:

1. **Package metadata.** Each EPUB's `content.opf` carries `<dc:rights>Public domain in the USA.</dc:rights>` and `<dc:language>de</dc:language>`, with *Johann Wolfgang von Goethe* as creator (#2229 EPUB 3 checked directly; #21000 metadata checked in the EPUB 2 and EPUB 3 packages). The #2229 OPF gives `dc:date` 2000-06-01 and identifies the source as `gutenberg.org/files/2229/2229-h/2229-h.htm`. The `pg-header` text in each package points to the Project Gutenberg licence for redistribution terms.
2. **Gutenberg licence statement inside the package.** In the #21000 EPUB 2 no-images package, the last chunk `OEBPS/8980964947130898904_21000-h-6.htm.html` begins with the heading "THE FULL PROJECT GUTENBERG™ LICENSE", then the transcriber's notes (German and English), then `*** END OF THE PROJECT GUTENBERG EBOOK …` and the licence sections, starting with `Section 1.`. Offsets are in characters of the tag-stripped, whitespace-collapsed text (not raw file offsets): the END marker is at 646 of 18,820, `Section 1.` at 2452. The EPUB 3 package keeps the licence in `…-h-3.htm.xhtml` (`pg-footer`). The #21000 package also contains the standard 1.E.1 sentence ("This eBook is for the use of anyone anywhere in the United States …") near the start, in the first chunk (text offset about 74). The footer is part of the spine and is not separable from the prose by the EPUB's own structure (see below).
3. **Gutenberg permissions policy.** <https://www.gutenberg.org/policy/permission.html> says that "The vast majority of Project Gutenberg eBooks are in the public domain in the US", that PG "claims no copyright for markup, formatting, spelling modernization, etc.", and that PG is "US only": "Not all items that are public domain in the US are public domain in other countries, and vice versa." It advises operators outside the US to "get professional guidance".
4. **German term (jurisdiction check).** § 64 UrhG: "Das Urheberrecht erlischt siebzig Jahre nach dem Tode des Urhebers." (<https://www.gesetze-im-internet.de/urhg/__64.html>). § 69 UrhG: the periods "beginnen mit dem Ablauf des Kalenderjahres, in dem das für den Beginn der Frist maßgebende Ereignis eingetreten ist" (<https://www.gesetze-im-internet.de/urhg/__69.html>). Applied to a death in 1832, the calendar-year rule ends the term on 31 December 1902, so the author's work would be in the public domain in Germany from 1 January 1903 under these two provisions. This is my reading of the statute, not a legal opinion.

Caveats:

- The public-domain status of the *text* is well supported for the US and for the German author's term. The status of the **specific Gutenberg transcription**, its proofreading, and its markup is Gutenberg's claim (PG says it claims no copyright in markup and formatting; see item 3).
- Scans come from Klassik Stiftung Weimar / Herzogin Anna Amalia Bibliothek (per the #21000 metadata). I did not examine whether any separate scan, edition, or database right applies in Germany or elsewhere. Scholarly-edition protection (§ 70 UrhG) was not examined.
- The Project Gutenberg licence was read in full in this pass, §1.E included, from the web page (`gutenberg.org/policy/license.html`) and from the EPUB's embedded footer (see *Licence text* below). That is a factual reading, not a legal certification. Read it again before any use that copies, displays, or redistributes the text.
- Nothing here is legal advice. The owning human decision should confirm rights for the intended use.

## Licence text (§1.E and the embedded copy)

Read in full in this pass. This is a factual reading of the text, not a legal opinion.

- **Web:** <https://www.gutenberg.org/policy/license.html>, saved as `meta/license-web.html` (scratch, not committed). <https://www.gutenberg.org/license> returned identical bytes. The top block reads, in part: "by using or distributing this work (or any other work associated in any way with the phrase 'Project Gutenberg'), you agree to comply with all the terms of the Full Project Gutenberg-tm License". §1.A: "By reading or using any part of this Project Gutenberg-tm electronic work, you indicate that you have read, understand, agree to and accept all the terms of this license".
- **Embedded:** #21000 EPUB 2 no-images, `OEBPS/8980964947130898904_21000-h-6.htm.html`, from the heading "THE FULL PROJECT GUTENBERG™ LICENSE" to the end of the licence. The #2229 EPUB 2 footer (`…2229-h-28.htm.html`) has the same licence text; in a word-level diff, the only difference is #21000's transcriber-note block, which sits between that heading and the licence.

§1.E as written in the web text. It applies "Unless you have removed all references to Project Gutenberg". The embedded text has the same paragraph structure and the same substance, except for the differences listed below.

- **§1.E.1:** a fixed sentence ("This eBook is for the use of anyone anywhere in the United States …") must "appear prominently whenever any copy of a Project Gutenberg-tm work … is accessed, displayed, performed, viewed, copied or distributed".
- **§1.E.2:** for a work with "Project Gutenberg" associated with or appearing on it, redistribution or providing access requires compliance with §§1.E.1–1.E.7, or permission under §1.E.8 or §1.E.9.
- **§1.E.4:** "Do not unlink or detach or remove the full Project Gutenberg-tm License terms from this work, or any files containing a part of this work or any other work associated with Project Gutenberg-tm."
- **§1.E.5:** "Do not copy, display, perform, distribute or redistribute this electronic work, or any part of this electronic work, without prominently displaying the sentence set forth in paragraph 1.E.1 with active links or immediate access to the full terms".
- **§1.E.6:** a format other than "Plain Vanilla ASCII" or the official format requires, "at no additional cost, fee or expense to the user", "a copy, a means of exporting a copy, or a means of obtaining a copy upon request" of the work in the original form. "Any alternate format must include the full Project Gutenberg-tm License as specified in paragraph 1.E.1."
- **§1.E.7:** no fee for access, viewing, display, copying, or distribution unless §1.E.8 or §1.E.9 applies.
- **§1.E.8:** a reasonable fee is allowed on conditions: a 20% royalty of gross profits to the trademark owner (donated to the Foundation), a full refund on written notice within 30 days, a refund for defects reported within 90 days, and the other free-distribution terms.
- **§1.E.9:** different fee or distribution terms need permission in writing. **The web text names "both the Project Gutenberg Literary Archive Foundation and The Project Gutenberg Trademark LLC, the owner of the Project Gutenberg-tm trademark". The embedded text names "the Project Gutenberg Literary Archive Foundation, the manager of the Project Gutenberg™ trademark".** The two name different permission-givers. Which one applies is not resolved here.

Other differences between the two copies (word-level diff after normalising ™/-tm and quote marks):

- §1.C and §1.E.2: "unrestricted" and "restricted" (web) against "unprotected" and "protected" (embedded).
- The web copy has a conditional paragraph beginning "[*] This particular work is one of the few individual works restricted by copyright law … included in the Project Gutenberg collection with the permission of the copyright holder". The embedded copy does not contain it. Whether it bears on this work is for the owning decision.
- The contact block differs: the web copy names an executive director and gives "in Montclair, New Jersey"; the embedded copy gives a street address, Montclair NJ 07042, and a telephone number.
- §1.E.1: "you'll have to check" (web) against "you will have to check" (embedded).

So the embedded copy is not the same text as the web page fetched on 2026-10-17. Which revision governs a given copy is not established.

## Package structure

### #21000, EPUB 2, no images (candidate)

- `mimetype`, `META-INF/container.xml` → `OEBPS/content.opf` (OPF version 2.0).
- Manifest: `pgepub.css`, `0.css`, a cover PNG, seven XHTML chunks (`…_21000-h-0.htm.html` through `…-h-6.htm.html`), `toc.ncx`, and a `wrap0000.html` cover page.
- Spine: cover page, `pg-header`, then six chunks (`item4`–`item9`). The `pg-header` chunk (`-h-0`) contains the Gutenberg header boilerplate, the printed title page, and the sections from Zueignung through Vor dem Thor (the NCX `pgepubid` anchors place them there).
- Navigation: `toc.ncx` only. No `nav.xhtml`, and no `epub:type` anywhere in the package (checked).
- The OPF guide contains a single `cover` reference, and there are no landmarks. The spine has eight itemrefs: `coverpage-wrapper`, `pg-header`, then `item4`–`item9`.
- Scene headings are `<h2>` (Vorspiel, Prolog im Himmel, …) and `<h3>` (sub-scenes such as "Trüber Tag."). The NCX lists scenes with fragment links such as `…-h-0.htm.html#pgepubid00006`.

### #21000, EPUB 3, with images (variant)

- `OEBPS/content.opf` (OPF version 3.0), `OEBPS/toc.xhtml` marked `properties="nav"`, and a `toc.ncx`.
- `toc.xhtml` has a `toc` nav (`epub:type="toc"`) and a `landmarks` nav. The `landmarks` nav is titled "Page List" (`aria-label`) and holds about 300 page-number links typed `epub:type="normal"`. The `<body>` of `toc.xhtml` is `epub:type="frontmatter"`. The content chunks carry no `epub:type`.
- There is **no** `bodymatter` or `backmatter` token anywhere in the package (grep over all six extracted packages returned none).
- The package has **four** XHTML chunks (`…-h-0.htm.xhtml` to `…-h-3.htm.xhtml`) rather than seven, so chunk boundaries differ from the EPUB 2 package. The licence footer is in the last chunk (`pg-end-separator` in `-h-3.htm.xhtml`).

### #2229, EPUB 2, no images (comparison)

- OPF 2.0, `toc.ncx`, 29 XHTML chunks. Chunk `-h-0` is front matter (no `div.chapter`, three `<h2>`). The other 28 chunks, `-h-1` to `-h-28`, hold 29 `div.chapter` elements in total. `-h-4` has two `<h2>` headings and the rest have one. No `nav`, no `epub:type`, no landmarks.
- The metadata credits Michael Pullen as the transcriber (`Credits : Michael Pullen`, package header). The package header says "Release date: June 1, 2000" and "Most recently updated: June 11, 2026". I found no 2010 update statement in either the package or the landing page, so the earlier draft's 2010 date is removed.

### #2229, EPUB 3, with images (comparison)

- A 30-chunk XHTML set (`-h-0` to `-h-29`), with `toc.xhtml` alongside. Its paragraph and `<br/>` counts are identical to the EPUB 2 package (1056 `<p>`, 4793 `<br/>`). The `toc.xhtml` has a `toc` nav only. There is no `landmarks` nav, and the package has no `bodymatter` token.

### What the package structure means for ADR 0066

[ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md) identifies main text only from declared EPUB 3 `bodymatter` landmarks, and falls back to the whole snapshot when none is declared. Neither candidate declares `bodymatter`, so **analysis of either candidate analyzes the whole snapshot**. That includes the Gutenberg header and footer, the printed title page and transcriber notes in the front matter, and the Gutenberg licence text in the footer. This is a fact about the current contract, not a recommendation to change it. The owning human decision needs to decide whether that fallback is acceptable for this MVP.

## Markup census

Counts come from parsing each XHTML chunk with Python's `xml.etree` (well-formed
XML; every chunk parsed) in spine order. "Blocks" are `<p>` and `<h1>`–`<h3>`
elements. Run-in and class-based counts are exact; "segments" are heuristic
(see Limits).

| Measure | #21000 EPUB 2 (no images) | #2229 EPUB 2 (no images) |
| --- | --- | --- |
| XHTML chunks | 7 | 29 |
| `<p>` elements | 2192 | 1056 |
| `<br/>` elements | 3678 | 4793 |
| Blocks containing `<br/>` (`<p>` and headings) | 712 | 1019 |
| Non-empty `<br/>`-delimited segments in those blocks | 4389 | 5809 |
| `<p>` without `<br/>` | 1480 (speaker 922, stage 188, other 370) | 38 (all short, none over 150 characters) |
| Other `<p>` without `<br/>`, over 150 characters | 12 (includes the two footer transcriber notes) | 0 |
| Speaker paragraphs `<p class="speaker">` | **922** | **0** |
| Stage-direction paragraphs (`entrance`, `sdcenter`, `sdsetting`, `dirlong`, `dirlongind`) | **194** (6 with `<br/>`) | **0** |
| `span.speaker` inside a paragraph | yes (e.g. `-h-2` line 346) | no |
| `span.sd` (stage text inside a paragraph) | in 125 paragraphs (97 are speaker paragraphs) | no |
| Run-in speaker and stage text as plain text | not the main pattern | **yes**, e.g. "Faust. Margarete vorübergehend." (`-h-10` line 18) |
| `class` token `indent` (`text-indent: 1.5em`) | 69 | 0 (no classes on prose) |
| `<ins class="correction">` (transcriber corrections) | 25 (23 in text, 2 as examples in the footer note) | 0 |
| `<em class="gesperrt">` (letter-spaced emphasis) | 13 | 0 |
| `x-ebookmaker-pageno` spans (printed page markers) | 301 | 0 |
| Heading elements (`h1`–`h3`) | 32 | 34 |

The EPUB 3 variant of #21000 (4 chunks) has identical class-token counts to the
EPUB 2 package (speaker, stage, `sd`, `person`, `indent`, `ins`, `gesperrt`,
pageno), identical `<p>` (2192) and `<br/>` (3678) totals, and the same
heading and page-marker counts. Its word-level text is **not** identical to
the EPUB 2 text: the word multisets differ by about nine tokens, all in heading
text (EPUB 2 has the extra tokens "Studirzimmer.", "Auerbachs Keller in
Leipzig.", "Abend.", "Gretchens Stube."; EPUB 3 has an extra "Walpurgisnachtstraum").
The cause is not established.

The EPUB 2 no-images and with-images packages of #21000 have **byte-identical XHTML
in all seven chunks**, and identical `pgepub.css` and `toc.ncx`. The `content.opf`
differs in the conversion timestamp and an image-size comment, and the cover image
differs. The no-images variant is therefore the same text and markup without
illustrations.

## Packaged CSS

Paths are relative to `OEBPS/`. Each EPUB 2 chunk links `0.css` and `pgepub.css`. #21000 EPUB 3 links `0.css`, `pgepub.css`, and `1.css`. I read `pgepub.css` and `0.css` in full for #21000 EPUB 2 and #2229 EPUB 2, and `pgepub.css` and `0.css` in full for #21000 EPUB 3. EPUB 3 `1.css` was read in part (lines 1–60, 130–246) and by its selector list; its lines 61–129 were not read, and nothing below relies on them. #21000 and #2229 EPUB 2 `pgepub.css` are byte-identical. Line numbers below refer to #21000 EPUB 2 `0.css` unless marked otherwise. I did not diff EPUB 3 against EPUB 2 line by line.

Rules that match this source's classes (exact selectors and declarations, summarised):

| Selector (lines) | Declarations | Use in the content (census) |
| --- | --- | --- |
| `p` (1–5) | `margin-top`/`margin-bottom` 0.75em; `text-align: justify` | every `<p>` |
| `p.speaker` (6–12) | centred; `letter-spacing: 0.35ex`; `font-size: large`; margin-top 1em, margin-bottom 0.4em (EPUB 3 `1.css:6`) | 922 |
| `span.speaker` (13–16) | `letter-spacing: 0.35ex`; `font-size: large`; no alignment (EPUB 3 `1.css:13`) | inside paragraphs |
| `p.entrance` (17–22), `p.sdcenter` (23–28), `p.sdsetting` (29–35) | centred; `x-large`, `small`, and `small` with letter-spacing | 56, 128, 3 |
| `p.dirlong` (36–39), `p.dirlongind` (40–44) | `font-size: small`; `dirlongind` adds `text-indent: 1.5em` | 2, 5 |
| `.sd` (50–53) | `font-size: small`; `letter-spacing: 0` (EPUB 3 `1.css:50`) | 125 spans |
| `.person` (45–49) | `font-size: x-large`; letter-spacing; padding-left | 74 spans |
| `p.indent` (54–56) | `text-indent: 1.5em` (EPUB 3 `1.css:54`) | 69 |
| `.i4` … `.i34` (186–233) | `margin-left: Nex`; `.i12` is `margin-left: 12ex` (198–200; EPUB 3 `1.css:205`) | `p.i12` 21, `span.i12` 4 |
| `em.gesperrt` (134–138) | letter-spacing; padding-left; `font-style: normal` | 13 |
| `ins.correction` (182–185) | `text-decoration: none`; `border-bottom: thin dotted red` (EPUB 3 `1.css:189`) | 25 |
| `div.note` (126–133) | dashed 1px border; green background; `font-size: smaller` | transcriber notes, `-h-6` |
| `.pagenum` (173–181) | `font-size: x-small`; grey; right-aligned (EPUB 3 `1.css:180`) | none |

From `pgepub.css` (EPUB 2 and EPUB 3): `div, p, pre, h1`–`h6` are `display: block` with zero side margins (lines 9–13). EPUB 3 adds `section.pgheader` and `section.pgfooter` page-break rules (lines 14–19) and `body.x-ebookmaker.x-ebookmaker-3 .pgshow { visibility: visible; display: inline }` (lines 51–53). No content element carries `pgshow`.

Absent from every CSS file I read (checked by grep): a selector for `span.x-ebookmaker-pageno` (301 uses in #21000), any `br` rule, any `white-space` declaration, and any `display: none` or `visibility: hidden` rule.

What this CSS does and does not establish:

- **Class semantics (this source).** The names `speaker`, `sd`, `person`, the paragraph classes listed above, `indent`, `i<N>`, `gesperrt`, `correction`, and `x-ebookmaker-pageno` are in the XHTML. Grouping the five paragraph classes `entrance`, `sdcenter`, `sdsetting`, `dirlong`, and `dirlongind` as stage directions is my reading of their names. The CSS gives them centring and sizes only.
- **CSS rendering (not universal).** These rules are what the packaged stylesheet asks for. A reader may override them.
- **Line and group boundaries.** The CSS adds no line breaks. Each `<p>` is a block, so a paragraph boundary is a block boundary. `<br/>` is the only forced line break in the markup. Newlines in the source after a `<br/>` are XHTML whitespace, which collapses under the default `white-space: normal`; they do not create lines. Whether a reader stretches a justified line that ends in `<br/>` is reader-dependent, and I did not check a reader.
- **Stanza appearance.** In the *Thule* song, the indent comes from `.i12` (`margin-left: 12ex`) together with `p.indent` (`text-indent: 1.5em`). The vertical gap between stanzas comes from the 0.75em `p` margins, not from any stanza class.
- **Speaker alignment.** `p.speaker` is a centred block; `span.speaker` is inline with no alignment. The same speaker word looks different when it opens its own paragraph.
- **Transcriber corrections.** The CSS shows the older visible reading with a dotted red bottom border. The note says hovering shows the corrected version. No selector in the package implements hover or a tooltip, so any tooltip depends on the reader's use of the `title` attribute.
- **Page markers.** No rule hides `x-ebookmaker-pageno`, so the stylesheet leaves each marker as an empty span wrapping an empty anchor. Its visible effect depends on the reader.

## Markup samples

Sample lines are quoted for structure only. Each is a short fragment of the packaged file, with the path relative to `OEBPS/`. Full text is not reproduced.

### Printed-edition statements (#21000)

- Transcriber's note, German, `OEBPS/8980964947130898904_21000-h-6.htm.html` lines 12–13: "Dieses elektronische Buch wurde auf / Grundlage der 1808 erschienenen Erstausgabe erstellt." ("This electronic book was made on the basis of the 1808 first edition.") Lines 14–15 say corrections from later print editions are marked "rot unterstrichen" (red underline).
- Transcriber's note, English, same file, lines 20–21: "This ebook has been prepared from the first print / edition published in 1808."
- Printed title page, `…-h-0.htm.html` lines 46–48: `Tübingen.` / `in der J. G. Cotta’schen Buchhandlung.` / `1808.`

The notes say the base text is the 1808 first edition, and the title page prints 1808. I did not compare the transcription with a scan or a catalogue record of that edition in this pass, so "1808 first edition" is the transcriber's claim, supported by the printed title page, and not verified against the edition itself. The note's hover statement is discussed under *Packaged CSS*.

### Verse lines, `<br/>`, and incidental source newlines (#21000)

`8980964947130898904_21000-h-0.htm.html` lines 90–92:

```html
<p>Ihr beyden die ihr mir so oft,<br/>
In Noth und Trübsal, beygestanden,<br/>
Sagt was ihr wohl, in deutschen Landen,<br/>
```

- Verse lines are delimited by `<br/>`. The newline after each `<br/>` is source formatting, not a meaningful break; splitting the block on `<br/>` (and dropping empty segments) gives the verse lines.
- The sentence "Ihr beyden … hofft?" runs across lines 90–93, so a sentence spans verse lines.
- A single verse line can contain two sentences: `…-h-0.htm.html` line 195: `Was fällt euch an? Entzückung oder Schmerzen?`

### Speaker and stage direction (#21000)

`…-h-0.htm.html` lines 89 and 148: `<p class="speaker">Director.</p>` and the speech that follows in the next `<p>`. Dedicated stage-direction classes:

```html
<p class="sdcenter">Der Himmel schließt, die Erzengel vertheilen sich,</p>
```

(`…-h-0.htm.html` line 445; the `sdcenter` class is used 128 times.) Mixed run-in case: `…-h-2.htm.html` line 346 has `<span class="speaker">Mephistopheles</span> mit seltsamen Geberden.`, a speaker span followed by stage text inside one classless `<p>`. `span.sd` appears inside 97 speaker paragraphs, so speaker and stage text can share a paragraph.

### Stanza grouping (#21000 song, and the un-marked Vorspiel)

- The *Es war ein König in Thule* song (`…-h-3.htm.html`, first verse line at line 141) is six `<p class="i12 indent">` blocks, **one per stanza**, each with four `<br/>`-delimited lines. Stanza boundaries are paragraph boundaries here.
- The Director speech in the Vorspiel (`…-h-0.htm.html` lines 90–147) is **one `<p>`** with no class, made of `<br/>`-delimited lines. The markup gives no segment boundary inside it, so I classify it as one source-speech block. I do not assert hidden or missing stanzas; that would need textual evidence. Paragraph boundaries are therefore not a reliable stanza signal in general.

### Verse lines as paragraphs, and unmarked prose (#21000)

- `…-h-1.htm.html` line 108 is a single-line `<p>` with no `<br/>`: `<p>Siehst du den schwarzen Hund durch Saat und Stoppel streifen?</p>`. Some verse lines are one `<p>` each.
- In the "Trüber Tag" scene (`<h3>` at `…-h-5.htm.html` line 420), the Faust speech starts with `<p class="speaker">Faust.</p>` (line 424), and the following paragraph (`<p class="indent">Im Elend! Verzweifelnd! …`, line 425) has no `<br/>`. It carries the `indent` class, and its lines are separated by raw source newlines. The same passage in #2229 (`-h-26`) is a `FAUST.<br/>` block with raw newlines, so the two packages mark it differently. Whether this passage is prose or verse is **an inference from the text**; the markup does not say, and I did not check it against a printed critical edition.
- Nothing in the EPUB markup (classes, `epub:type`, `lang`) distinguishes verse from prose. Classification of verse vs. prose from the packaged file would be text-based judgement.

### Spaced emphasis and transcriber corrections (#21000)

- `…-h-0.htm.html` line 71: `Mein <ins class="correction" title="Transcriber's note: newer editions read 'Lied'">Leid</ins> ertönt der unbekannten Menge,`. The visible text is the older reading. The newer reading is only in the `title` attribute.
- `…-h-0.htm.html` line 412: `<ins class="correction" title="Transcriber's note: newer editions read 'Am'">An</ins> meisten lieb’ ich mir die vollen frischen Wangen.`
- `<em class="gesperrt">` (letter-spaced emphasis) is used for words such as "Wort" and "Sinn" in `…-h-1.htm.html` lines 206–214 (13 uses in the package). It carries no semantic distinction beyond emphasis.

### Printed page markers (#21000)

- `<span class="x-ebookmaker-pageno" title="[9]" id="…"><a id="Page_9" title="[9]"></a></span>` appears 301 times and is not text. A raw-string check found each one directly after a `<br/>` (150) or after a line or tag boundary (151, which may be a paragraph, heading, or inline-tag edge). None followed plain text on the same line, but the stripping rule should still treat the marker as a zero-width element at any position.

### #2229 for comparison

- Speakers are plain uppercase text with no class, e.g. `5068675349464888044_2229-h-10.htm.html` lines 22 and 35: `FAUST.<br/>`. A stage direction is run-in plain text in its own `<p>`: line 18, `Faust. Margarete vorübergehend.`
- The "Es war ein König in Thule" song (`…-h-11.htm.html` line 172) is also verse with `<br/>`, and stanzas are plain `<p>` blocks with no class.
- Consequence: an analyzer cannot separate speaker labels or stage directions from verse in #2229 without a text-based rule.

## Catalog and acquisition route

All statements in this section are **static reading** of the code at commit
`218d336859f66cefd3e56480004710eee4856077` (this branch's base; `6e02d180b1015536127839e0a45953684c0e5a83` has byte-identical `internal/`, checked with `git diff --quiet`). Links below use `218d3368…`. Nothing was executed: no Go test, sync, or
download of Mouseion code was run in this pass (the parent's route test attempt failed to start; see *Limits*).

- **Entry selection.** [`FindEPUBs`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L209) keeps links whose `rel` is the OPDS acquisition relation and whose type is `application/epub+zip`. [`resolveAcquisitionEntry`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L611) matches the alias to an entry by `entry.ID` (`strings.TrimSpace(entry.ID) != alias.Value`), and [`acquisitionHref`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L649) takes the first matching link. [`Service.acquire`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/service.go#L171) uses the first link from `FindEPUBs` ([line 175](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/service.go#L175)).
- **Feed route.** The sync lookup does not read a book's own feed. It reads the Calibre-Web language feed: [`resolveAcquisitionEntry`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L611-L647) calls `Languages` and `BrowseLanguage`, which call `ListLanguages` and `ListLanguage` ([`client.go#L93–L103`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L93-L103)) at `<catalog>/language` and `<catalog>/language/<positive numeric id>` ([`catalogEndpoint`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L581)). The per-book Gutenberg feeds fetched in this pass (`/ebooks/21000.opds`, `/ebooks/2229.opds`) list only the two entries and their acquisition links, with no language navigation. **So a Gutenberg per-number OPDS URL is not, by static reading, a usable sync connection.** The Gutenberg `urn:gutenberg:…` IDs are upstream identifiers, and a Calibre-Web library would assign its own IDs. The route that this reading supports is: obtain the pinned bytes, place them in a learner's Calibre/Calibre-Web library that the connection can browse, then sync and move the book to To Read. This is an inference from the code and was not tested.
- **Language prerequisites (static).** [`eligibleLanguages`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L900) skips any capability that is not `Ready` or is English. The supported-language table is written from [`analyzer.ReadySupportedLanguages`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/analyzer/capabilities.go#L50) at [`sync.go#L1026`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L1026). In [`resolveAcquisitionEntry`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L619), the book's language is looked up in `ListSupportedLanguages` ([`sync.go#L619`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L619)), and then in the Calibre-Web language feed through `opds.LanguageID` ([`sync.go#L628`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L628)); an empty ID returns `ErrNotFound` ([`sync.go#L630`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/cataloguesync/sync.go#L630)). So German needs a `Ready` capability from the running NLP service and a German entry in the connected library. Neither was checked against a running service.
- **Origin scope for a Calibre-Web connection (static).** A connection client is built with `Auth.Origin` set to the connection URL ([`service.go#L257`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/service.go#L257)). For an acquisition, [`Download`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L370) calls [`withOrigin`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L430-L431), which returns the client unchanged when `Origin` is already set, so it does not scope a new origin. [`get`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L503) then calls `validateTarget` before any request, and [`validateTarget`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L517-L527) returns `opds: URL is outside the catalog origin` for a target whose origin differs from `Origin`. So a `www.gutenberg.org` acquisition link on a Calibre-Web connection is refused before any request is sent. The pinned bytes would have to be served from the Calibre-Web library's own origin. This is a reading of the code, not an executed result.
- **Two entries, two Books?** [`ReconcileCatalogueEntry`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/persistence/books.go#L759) and [`resolveCatalogueEntryBook`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/persistence/books.go#L791) key on owner, connection, and source identifier. If two entries with different IDs (for example `urn:gutenberg:21000:2` and `:3`) both reached reconciliation on one connection, the static reading gives two Book records. Whether a connected feed would expose both entries is not established, so this is a conditional, not a finding that the catalog duplicates the work.
- **Redirects.** [`httpClient`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L531) installs a redirect check that calls [`validateTarget`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L540) on each hop. On refusal it removes the `Authorization` header and returns `http.ErrUseLastResponse` ([`client.go#L534–L547`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L534-L547)). The check enforces same origin only when `Auth.Origin` is set, which is the case for every connection client (see above). A same-origin redirect (for example, `www.gutenberg.org/ebooks/21000.epub.noimages` → `…/cache/epub/21000/pg21000.epub`) is followed only on a connection whose origin is `www.gutenberg.org`, which is not a Calibre-Web route.
- **Download checks.** [`Download`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L370) checks the HTTP status and the response media type (EPUB or `application/octet-stream`), and caps the body at `maxResponse` = 100 MiB ([`client.go#L26`](https://github.com/justin-hayes/mouseion/blob/218d336859f66cefd3e56480004710eee4856077/internal/opds/client.go#L26)). It takes **no expected SHA-256**, so the digests in this document are research pins, not a runtime check.
- **Lazy acquisition.** Sync is metadata-first, and content is acquired on "Move to To Read" (see [catalog sync feature](../features/catalog-sync.md), "Lazy content acquisition").
- **Precedent.** `e2e/screenshot/manifest.json` pins Gutenberg numbers and SHA-256 values for the My Books screenshot. Its `$comment` describes those sources as Project Gutenberg **plain-text** editions, so it is not EPUB precedent. `doc/development.md` (around line 226) and `e2e/README.md` (around line 211) mention network access to Gutenberg for that first run. I did not change either file.

## Limits

- **Unrun probes.** I did not run the NLP service, the ingest pipeline, the EPUB extractor, or the catalog sync against these packages. The statements about "the repo would acquire X" are read from code, not from an execution. The EPUB extraction contract (`internal/epub/`) was not run against either package.
- **Census method.** Chunks were parsed with `xml.etree` (every chunk parsed). Class and element counts are exact for these packages. "Segments" are split on `<br/>`, so they include speaker labels and stage text where those share a paragraph (for example, `span.speaker` in #21000). Segment counts are therefore not a verse-line count.
- **Speaker detection in #2229 is a heuristic.** A regex for all-caps run-in labels (`FAUST.`) found roughly 800 matches, with about 100 distinct labels. That is not an exact count, and the regex may also miss labels.
- **Verse vs. prose is not established.** The prose cases above are text-based inferences; I did not verify them against a printed critical edition.
- **Stanza structure is not established for the whole text.** Only the Thule song (in #21000 and #2229) and the Vorspiel speech were examined closely. Other songs and speeches were sampled, not exhaustively checked.
- **Not checked:** Projekt Gutenberg-DE's EPUB; scan or edition-level rights outside the US and Germany; whether any extractor path uses the EPUB 3 "Page List" nav; the cause of the EPUB 2 vs. EPUB 3 heading-token difference; whether a running NLP service reports `de` as ready. Which licence revision (web or embedded) governs a given copy is not established (see *Licence text*).
- **Parent's route test:** an attempted run of the existing route test failed to start with `go: command not found`. That is not a test result, and no Go test has run in this pass.
- **Not run:** the NLP service, the EPUB extractor (`internal/epub/`), the catalog sync, or the downloader. The statements about what the repo would do are static.
- **Gutenberg files change.** Pin by digest, and re-verify the digest when pinning (see the provenance table).
- **No full text is reproduced here.** Samples are short fragments, and sentences are quoted only where they illustrate structure.

## Recommendation (agent recommendation, not acceptance)

For the MVP acceptance source, the **agent-preferred candidate** is Gutenberg #21000, EPUB 2 no-images package, OPDS entry `urn:gutenberg:21000:2` (SHA-256 `93d5a2bd…01`, 179103 bytes). Reasons:

1. It carries explicit speaker and stage-direction markup (922 speaker paragraphs, 194 stage-direction paragraphs), which the map's fidelity target needs. #2229 has none.
2. Verse lines are explicit `<br/>` breaks, and stanza boundaries are paragraph boundaries in the song examined (Thule, six `i12 indent` blocks).
3. Its no-images package is byte-identical in XHTML to the with-images package. The EPUB 3 package has the same class and `<br/>` counts but different chunking, a `toc.xhtml` nav, and about nine tokens of heading-text difference. The EPUB 2 vs. EPUB 3 choice is therefore mostly packaging, with that text difference still unexplained.
4. Its transcriber's notes name an 1808 first edition as the base text (quoted under *Printed-edition statements*), and the transcriber's corrections are explicit in the markup.

The owning human decision should weigh the following before accepting this:

- Verse vs. prose and stanza grouping are not marked and need a text-based or structural rule. The Vorspiel Director speech is one unclassed `<p>` of `<br/>` lines, with no marked segmentation.
- Transcriber corrections (`<ins class="correction">`, 23 in text) show the older reading in visible text. The newer reading is only in the `title`.
- The front matter and Gutenberg licence text are in the spine and fall into the whole-snapshot fallback under ADR 0066.
- Whether the catalog would list one or two Books depends on which OPDS entries a connected feed exposes (see the catalog section). The per-book Gutenberg feed alone is not a sync connection by static reading.
- The §1.E licence obligations are now read in full (see *Licence text*). Whether they bind a given Mouseion use, such as showing, transmitting, exporting, or labelling a fragment with the Project Gutenberg name, is for the owning decision and is not decided here.

Gutenberg #2229 is the comparison candidate. Its verse breaks are explicit too, but it has no speaker or stage-direction `class` markup, and its text uses modern spelling and no printed-edition statement. It would need text-based inference for the speaker and stage-direction features.

This recommendation is a research input. It is not human acceptance of the source, of the structural model, or of the rights position.

## Questions for the owning decision

1. Which connection would browse the Gutenberg-sourced files: a Calibre-Web library (the static route above), or a direct Gutenberg OPDS connection? By static reading, a per-book Gutenberg feed is not a sync connection.
2. Is the ADR 0066 whole-snapshot fallback acceptable for the front matter (title page and transcriber notes) and the licence footer, or should the acceptance case exclude them?
3. Does §1.E require any Mouseion display or export attribution? That is a legal question. The web and embedded texts differ in §1.E.9 about who gives permission for different terms, and which text governs is open.
4. Is the transcriber's "1808 first edition" claim enough for the acceptance case? It has not been checked against a scan or catalogue record.

## Asset links

- Gutenberg #21000 landing page: <https://www.gutenberg.org/ebooks/21000>
- Gutenberg #21000 OPDS: <https://www.gutenberg.org/ebooks/21000.opds>
- Gutenberg #21000 no-images EPUB 2 (redirect target): <https://www.gutenberg.org/cache/epub/21000/pg21000.epub>
- Gutenberg #21000 with-images EPUB 2 (redirect target): <https://www.gutenberg.org/cache/epub/21000/pg21000-images.epub>
- Gutenberg #21000 EPUB 3 with images: <https://www.gutenberg.org/ebooks/21000.epub3.images>
- Gutenberg #2229 landing page: <https://www.gutenberg.org/ebooks/2229>
- Gutenberg #2229 OPDS: <https://www.gutenberg.org/ebooks/2229.opds>
- Gutenberg #2229 EPUB 3 with images: <https://www.gutenberg.org/ebooks/2229.epub3.images>
- Gutenberg licence (web): <https://www.gutenberg.org/policy/license.html>
- Gutenberg permissions policy: <https://www.gutenberg.org/policy/permission.html>
- § 64 UrhG: <https://www.gesetze-im-internet.de/urhg/__64.html>
- § 69 UrhG: <https://www.gesetze-im-internet.de/urhg/__69.html>
- Projekt Gutenberg-DE (not inspected): <https://projekt-gutenberg.org/authors/johann-wolfgang-von-goethe/books/faust-eine-tragoedie/>

The downloaded EPUBs, extracted trees, and census scripts were kept in a
scratch directory outside the repository and are not committed. To reproduce,
re-download each URL above with `curl -L` and check its SHA-256 and byte count
against the provenance table.

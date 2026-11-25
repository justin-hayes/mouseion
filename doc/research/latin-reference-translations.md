# Openly licensed English translations alignable to classical Latin Citations

**Research date:** 2026-11-25

**Scope:** Which English translations can give a Latin card its passage
context, the way Brownson does for Xenophon (ADR 0101), for the classical Works
that Opera Latina Adnotata (OLA) v0.2.0 covers under CC BY-SA text (Research
[#2660](https://github.com/justin-hayes/mouseion/blob/3c1318cfafc5f95b4868d071fa317f37ab8969c5/doc/research/latin-oga-equivalent.md):
205 classical Works, 4.14M words by this study's count, plus 4 borderline).
Translations are Source content (ADR 0111). Primary sources only: the pinned
`PerseusDL/canonical-latinLit` checkout and its files. Nothing here is run by
Mouseion as an analyser. Commits aggregates, ids and digests only; no
translation or Latin text.

## Answer

- **One open source covers 113 of the 205 classical Works, and it is the same
  repository OLA takes its text from.** The `perseus-engN` files in
  `PerseusDL/canonical-latinLit` (pinned `8cef5162…`, 2026-11-16) hold an English
  translation for 113 of the 205 OLA classical Works (122 files, 6 Works with
  more than one), in the same TEI scheme as the Latin edition OLA parsed. Their
  terms are CC BY-SA 4.0, as for the Brownson file (the Source content row in
  `doc/third-party-data.md`). The 92 Works without a file are 36.6 % of the
  words and include all of Nepos, all of Seneca bar the *Apocolocyntosis*, and
  16 Cicero Works.
- **Alignment is good for prose and weak for verse.** Citation alignment is
  measured against the TEI Citation scheme of the Latin edition, not OLA's
  per-token Citation (not downloaded; see Caveats). Tiers (below) put **47 Works
  (31.2 % of words) at tier A** (a translation segment for the same Citation),
  **18 (11.1 %) at B** (one level up, 100–400 words), **45 (21.0 %) at C**
  (book- or act-sized segments, or a partly diverging numbering), 1 at D and 94
  with nothing usable. Of the 68 verse Works, 4 reach A or B and 37 sit at C.
- **Ranking for a prose first slice (translation readiness):** Livy (A, three
  translations, section-level), the Cicero speeches (33 of 35 at A, Yonge),
  Sallust *Catilinae* and *Iugurthinum* (A at chapter level), Suetonius, Tacitus
  (*Historiae* A, *Germania* and *Agricola* B) and Caesar *Bellum Gallicum* and
  *Civile* (B: a translation chapter answers about 5 Latin sections, median 199
  words). Nepos has no file. Verse is paragraph-level or coarser except
  Horace *Carmina*, Catullus and the Tibullus/Sulpicia poems, whose translations
  carry line numbers that match the Latin line count.
- **No analyser is involved.** Everything here is third-party TEI.

## Method

1. Pool: the 205 classical (`core`) Works whose OLA text is BY-SA
   (manifest of #2660), plus 4 borderline Works (209 in all). 203 of the 205
   are in `canonical-latinLit`; `Latin` and `priapeia` Works have no
   translation in the repository.
2. For each Work, parse the Latin edition OLA used (`…perseus-lat2`, or the
   edition named in the manifest) and every `…-eng*.xml` in the same directory
   (`doc/research/latin-reference-translations/align.py`). A text chunk carries
   the Citation state set by `div[@type=textpart]` (`@subtype` = unit),
   old-style `div1..div3` (`@type`), composite `n="text=F:book=5:letter=7"`,
   `milestone` (`@unit`, `@n`, set when met) and `l/@n`.
3. The source defines the Citation units. The translation is projected onto
   the source units it actually has (Livy's `part`, which the translation
   lacks, is dropped on both sides). Leaves shallower than the translation's
   word-weighted modal depth and under 3 % of its words (heads, arguments) are
   dropped.
4. Each source leaf is classed **one** (the translation has a leaf with the
   same Citation), **coarse** (a prefix of it is a translation leaf: one
   segment answers many source Citations), **fine** (translation leaves lie
   under it) or **none**. `plausible` is the share of `one` leaves (source ≥ 5
   words) whose English/Latin word ratio is 0.8–4.0: a translation that only
   reuses the numbers scatters.
5. For verse the line key is trusted only when the translation's line count
   is within 10 % of the Latin's; otherwise the alignment is re-measured one
   level up.
6. `tier.py` ranks each Work by its best translation (ties prefer a print date
   ≤ 1930). A: ≥ 90 % `one` (prose or valid verse line key). B: ≥ 90 % covered
   at a coarser level, median translation segment ≤ 400 words. C: ≥ 90 %
   covered with larger segments, or 50–90 % covered, or keyed leaves 70–90 %
   plausible. D: under 50 % covered, or under 70 % plausible among keyed
   leaves. `-`: no usable file.

Reproduce: `git clone https://github.com/PerseusDL/canonical-latinLit` at
`8cef51623130f2412da410858e32c322eb796c0a`, take
`doc/research/latin-oga-equivalent/ola-conllu-manifest.tsv` from #2660, then
`python3 -I align.py <repo>/data <manifest.tsv> results` and
`python3 -I tier.py results/alignment.json results`.
Results: `doc/research/latin-reference-translations/results/` (`alignment.json`
has per-file digests, translators, print dates, licence element and counts;
`works.tsv` is the ranked list; `summary.json` the aggregates).

## Sources and licence classification (ADR 0111)

| Layer | Class | Instrument in force | Restriction | Status |
|---|---|---|---|---|
| `perseus-engN` translations in `PerseusDL/canonical-latinLit@8cef5162` | Source content | `<licence target=…by-sa/4.0>` in 51 of 122 translation headers; for the other 71, `README.md`: "Unless otherwise indicated, all contents of this repository are licensed under a Creative Commons Attribution-ShareAlike 4.0 International License", plus `license.md` (CC BY-SA 4.0 legal code). No header contains non-commercial or "all rights reserved" terms (grep of all 122). | share-alike | **Admissible**, same footing as the Brownson row. The README also says the Perseus materials "have varying copyright status". |
| Print copyright of the translated books | Source content | Header `imprint/date`. For the 111 Works with a usable file, 108 print ≤ 1930 (US public domain by date). Exceptions: Celsus (Spencer, Loeb 1935–38), Columella (Loeb 1941–55), one file with no print date (Tibullus/Sulpicia, Mahoney). The Livy Loeb file (eng1, header date 1919, editors Foster, Moore, Sage and Schlesinger) spans several volumes of a series that continued after 1930, so part of it is probably later; eng2 (Edmonds/Spillan, 1857) and eng3 (Roberts, 1912) do not. | n/a | **Unverified** for the post-1930 prints, as for Plato *Republic* vol. 6 in the register. |
| Wikisource English translations | Source content | Site text is CC BY-SA 4.0 ("additional terms may apply"); the translations on it are mostly public-domain prints. | share-alike | Candidate, **not measured** (see Other sources). |
| Project Gutenberg translations | Source content | Project Gutenberg licence; the text is US public domain by print date. Catalogue not reachable (HTTP 503 from this host on 2026-11-25). | public domain (print date) | Candidate, **not measured**. |

The Latin editions themselves are the OLA text layer (#2660) and are not
reclassified here. The translation files sit in the same repository and at the
same commit as the BY-SA Latin text, so one pinned checkout serves both layers.
The repository has no release tag; the commit is the pin (Brownson is pinned the
same way at `canonical-greekLit@0189b9c8`).

## Results

### Coverage and tiers (205 classical Works, 4,141,934 words)

| Tier | Works | Words | Share of words |
|---|---:|---:|---:|
| A: same Citation | 47 | 1,293,285 | 31.2 % |
| B: one level up, ≤ 400 words | 18 | 458,739 | 11.1 % |
| C: coarse or partly diverging | 45 | 871,326 | 21.0 % |
| D: numbering does not follow | 1 | 3,011 | 0.1 % |
| none (no file, or no shared Citation unit) | 94 | 1,515,573 | 36.6 % |

Prose (137 Works): A 44, B 17, C 8, D 1, none 67. Verse (68 Works): A 3, B 1,
C 37, none 27. 92 Works have no translation file; 2 more (*Commentariolum
petitionis*, Horace *Ars Poetica*) have a file with no Citation unit in common.
The 4 borderline Works add one C and one D.

### By author (the 20 largest; tier counts in Works)

| Author | Works | Words | Tiers |
|---|---:|---:|---|
| Cicero | 59 | 1,125,624 | A 36, B 1, C 6, none 16 |
| Livy | 1 | 529,789 | A 1 |
| Seneca | 14 | 242,440 | D 1, none 13 |
| Ovid | 10 | 218,670 | C 6, none 4 |
| Plautus | 20 | 178,840 | B 1, C 19 |
| Quintilian | 1 | 172,544 | B 1 |
| Celsus | 1 | 105,656 | A 1 |
| Seneca the Elder | 4 | 99,834 | none 4 |
| Statius | 3 | 95,004 | none 3 |
| Caesar | 2 | 83,686 | B 2 |
| Virgil | 3 | 83,664 | C 3 |
| Valerius Maximus | 1 | 80,292 | none 1 |
| Columella | 1 | 79,255 | C 1 |
| Silius Italicus | 1 | 76,676 | none 1 |
| Curtius Rufus | 1 | 74,199 | none 1 |
| Suetonius | 12 | 70,654 | A 1, B 11 |
| Pliny the Younger | 1 | 67,116 | none 1 |
| Tacitus | 3 | 63,681 | A 1, B 2 |
| Terence | 6 | 61,486 | C 6 |
| Martial | 1 | 58,364 | none 1 |

Also none: Nepos (25 Works, 28,212 words), Florus, Propertius, Juvenal, Valerius
Flaccus. Sallust: A 2 (*Catilinae*, *Iugurthinum*), the *Historiae* fragments
none. Whole-Work coverage of the Works CIRCSE and LDT carry (#2659): Tacitus
*Germania* B; Sallust *Bellum Catilinae* A; Seneca's four tragedies none.

### The likely slice-1 prose candidates, measured in depth

`one / coarse / fine / none` count source Citations at the full source depth.
"Median" is the median translation segment in words. Digests are SHA-256 of the
file prefix (full values in `results/alignment.json`).

| Work | Translation (print) | Source Citation → translation segment | one | coarse | fine | none | Median | Tier |
|---|---|---|---:|---:|---:|---:|---:|---|
| Caesar *Bellum Gallicum* `phi0448.phi001` | McDevitte & Bohn (1869–72), `eng2` `fa4aa7c8` | book.chapter.section → book.chapter | 0 | 2,150 | 0 | 0 | 199 words, 5 sections | B |
| Caesar *Bellum Civile* `phi0448.phi002` | Peskett (1914), `eng2` `90665a4a` (Duncan 1856 `eng3` also) | same | 0 | 1,186 | 0 | 0 | 211 words, 5 sections | B |
| Cicero *In Catilinam* `phi0474.phi013` | Yonge (1856), `eng2` `751291a7` | speech.section → speech.section | 115 | 0 | 0 | 0 | 171 | A |
| Cicero *In Verrem* `phi0474.phi005` | Yonge (1856), `eng2` `e16ff0a2` | actio.book.section → same | 974 | 0 | 0 | 0 | 174 | A |
| Cicero *Philippicae* `phi0474.phi035` | Yonge (1903 print), `eng1` `ea44bb7c` | speech.section | 543 | 0 | 0 | 1 | 160 | A |
| Sallust *Bellum Catilinae* `phi0631.phi001` | Watson (1899), `eng2` `3c68139f` | chapter | 59 | 0 | 0 | 2 | 245 | A |
| Sallust *Bellum Iugurthinum* `phi0631.phi002` | Watson (1899), `eng2` `4a93c8b5` | chapter | 114 | 0 | 0 | 0 | 281 | A |
| Livy *Ab urbe condita* `phi0914.phi001` | Roberts (1912), `eng3` `feae60b4`; Edmonds/Spillan (1857), `eng2` `fd14d053`; Foster et al. Loeb (1919–), `eng1` `c78f717d` | book.chapter.section → same | 20,059 / 20,117 / 19,938 | 0 | 0 | 223 / 165 / 344 | 40 (section) | A |
| Tacitus *Germania* `phi1351.phi002` | Church & Brodribb (1877), `eng2` `c417d45d` | chapter.section → chapter | 0 | 187 | 0 | 0 | 211 words, 4 sections | B |
| Tacitus *Agricola* `phi1351.phi001` | Church & Brodribb (1877), `eng4` `be6317bc` | chapter.section → chapter | 0 | 213 | 0 | 0 | 260 words, 4.5 sections | B |
| Tacitus *Historiae* `phi1351.phi004` | Church & Brodribb, `eng2` | book.chapter | 389 | 0 | 0 | 0 | n/a | A |
| Nepos (25 Lives) `phi0588.abo001`–`025` | none in `canonical-latinLit` | | | | | | | none |

Notes on these rows:

- **Plausibility of the keyed leaves** (English/Latin word ratio 0.8–4.0) is
  0.95–1.00 for Cicero, Sallust and Livy, so the numbers follow the content,
  not only the labels. The overall word ratios are 1.6–1.9 English words per
  Latin word, the usual range.
- **Caesar is coarse by design.** The Latin edition cites book.chapter.section
  and the translation only to the chapter. A card gets a 100–250-word chapter
  for a sentence in one of its ~5 sections, and a sentence that crosses a
  chapter boundary gets both chapters, as Brownson's rule does across 1.2.17–18.
- **Livy has 165–344 Latin leaves with no translation leaf**, mostly
  *periochae*, prefaces and fragments that the English editions order
  differently. The three translations are three independent digitisations.
- **OLA treats Livy as one 518k-word Reading unit** (#2660); the translation
  segments are sections within it, so passage context does not depend on how
  the Reading unit is cut.
- Cicero's letters are weaker: *Ad familiares* is B (keyed 4, coarse 1,276 at
  the letter, median 368 words), *Ad Atticum* C, *Ad Quintum fratrem* C, *De
  officiis* C (250 keyed, 121 none, plausible 0.87). Gellius is D (264 keyed,
  3,699 none, plausible 0.08: the numbering does not follow the Latin).

### Verse

- **Plautus (20) and Terence (6):** Riley's and Colman's prose translations are
  cut by act and scene, with a median of 344–923 words per scene for Plautus and
  444–656 for Terence. The Latin has line numbers; the translation line counts
  are 40–68 % (Plautus) and 49–64 % (Terence) of the Latin's, so a line key does
  not map. Aulularia
  is B; the rest are C. Mapping a Latin line to its scene is possible through
  the Latin TEI's own act and scene divisions.
- **Virgil:** Williams's *Aeneid* has 13,336 numbered lines against 9,897 in the
  Latin (ratio 1.35), so its line numbers are the translator's own. It aligns
  at book level only (12 segments, median 8,618 words, C). The *Georgics* (Rhoades)
  and *Eclogues* are book- or poem-level, C. The `card` divisions (327 in Williams,
  392 in the Latin) are not the same segments, so they cannot be keyed.
- **Horace *Carmina*** (Conington, 1882) and **Catullus** (Smithers, 1894) carry
  line numbers with a line-count ratio of 0.97 and 1.02: A, with 2,954 and
  2,280 keyed lines and plausible 0.996 and 0.998. The Tibullus/Sulpicia poems
  (Mahoney) are A on 40 lines. These are the three verse Works for which a
  card could show the same lines as the Latin.
- **Ovid** (*Metamorphoses* Golding 1567 and More, *Amores*, *Ars*), **Lucan**
  (Ridley) and **Lucretius** (Leonard) align at book or poem level, 460–12,400
  words per segment: C. **Horace *Sermones*** (Smart) C. Seneca's tragedies,
  Statius, Silius, Valerius Flaccus, Martial, Propertius and Juvenal: no file.
- So verse context is **paragraph or poem level at best**, not line level,
  except for the three Works above. This supports the map's note that verse in
  slice 1 depends on basic poetry support ([#2186](https://github.com/justin-hayes/mouseion/issues/2186)).

### OLA's own Citation depth

OLA's tokens carry a modal Citation depth (#2660 `cts.json`). Against the TEI
depth used here, for the 117 OLA editions of Works with a usable translation:
equal in 71 (`ola1_tei1` 39, `ola2_tei2` 18, `ola3_tei3` 13, and Livy's
`ola3_tei4`, whose extra TEI `part` unit the translation lacks), **OLA shallower
in 39** (`ola1_tei3` 26, all of Plautus and Terence; `ola1_tei2` 12, all of
Suetonius; `ola2_tei3` 1, Catullus), and OLA deeper in 7 (`ola3_tei2` 5, among
them Lucan, Phaedrus and Tacitus's *Historiae*; `ola2_tei1` 1; `ola4_tei3` 1).
Where OLA's Citation is shallower than the TEI's, a card's Citation is easier
to match, but Plautus's `ola1` is a bare line number, which needs the Latin TEI's
act and scene boundaries to reach the translation. Where it is deeper, the
alignment here overstates how finely a card's Citation can be matched. Figures:
`results/ola_depth_crosstab.json`.

## Other sources

- **Wikisource (English)**: CC BY-SA 4.0 site text over mostly public-domain
  prints, with a Latin portal listing Nepos (*Lives of the Eminent Commanders*),
  Seneca's tragedies (Bradshaw 1902, Miller 1907), Pliny's letters, Juvenal,
  Martial, Statius, Florus, Suetonius, Caesar, Livy, Cicero's letters and others.
  It covers several of the 92 Works with no Perseus file. It is wiki pages
  rather than TEI, with no stable file digest and variable sectioning; not
  downloaded or measured.
- **Project Gutenberg**: public-domain prints, for example Melmoth's *Letters of
  Pliny* (#2811) and several Watson translations. Plain text or HTML with
  whatever sectioning the print had; the catalogue returned HTTP 503 from this
  host, so coverage is from search results only and unmeasured.
- **Standard Ebooks** (CC0): a search for Latin returned nothing relevant.
- **Loeb Digital Library, Lacus Curtius, Internet Archive scans**: scans or
  copyright-claimed transcriptions; no open instrument. Not candidates.
- **`canonical-greekLit` style translation siblings** exist only inside
  `canonical-latinLit` for Latin; no other Perseus or OGL Latin repository
  carries English.

## Caveats

- **Measured against the TEI scheme, not OLA's per-token Citations.** The PAULA
  layer (1.41 GB) was not downloaded. The two depths agree for 71 of 117 OLA
  editions (above); the rest need the real join.
- **A matching Citation key is not proof of matching content.** Plausible
  word ratios and the verse line-count check reduce, not remove, the risk;
  the Aeneid case (same line numbers, 35 % more lines) shows it. No human check
  of any alignment was made.
- **Print copyright after 1930 is unverified** (Celsus, Columella, parts of the
  Livy Loeb file). Perseus's own README warns of varying status.
- **Apparatus and notes**: notes are excluded from word counts, but `<add>`,
  `<name>` and similar markup inside translation text was not audited.
- Gutenberg and Wikisource coverage is from catalogue pages and search
  snippets, not downloads.

## Recommendation

1. **Use the Perseus `perseus-engN` files as the reference-translation layer
   for Latin**, pinned by commit with a per-file SHA-256, fetched lazily at
   Book preparation like Brownson, and classified as Source content under
   ADR 0111 with per-file deck notices. One checkout serves OLA's text and the
   translation.
2. **Slice 1, prose first**, in translation-readiness order: Livy; Cicero
   speeches (33 Works, 417,877 words at A); Sallust *Catilinae* (the whole
   Work LDT also has); Tacitus; Suetonius; Caesar (needs the "chapter answers
   several sections" rule). For Brownson-like granularity, Livy (40 words per
   segment) and Cicero (about 170) are closest; Caesar and Tacitus give 200–260.
3. **Rule for the card:** show the translation segment whose Citation equals
   or contains the sentence's Citation, and both segments when the sentence
   straddles two, with the label "Not a translation of this sentence", as in
   ADR 0101. If a touched Citation has no segment, the card has no English
   (Livy leaves 165–344 such Citations of about 20,300, under 2 %).
4. **Verse is not ready for line-level context** except Horace *Carmina*,
   Catullus and the Tibullus/Sulpicia poems; Plautus and Terence give scene
   level. Treat verse as a later slice.
5. **Nepos, Seneca (all but the *Apocolocyntosis*), Statius, Pliny's letters
   and 16 Cicero Works have no Perseus file.** Their translations would have
   to come from Wikisource or Gutenberg, which need their own segmenting and
   rights check. The CIRCSE Seneca tragedies are in this gap.

**What would change this:** a real join of OLA's PAULA Citations to the
translation segments; a rights check of the 1931+ prints; a human check of a
sample of aligned sentence/segment pairs.

## Sources

- Pinned repository: https://github.com/PerseusDL/canonical-latinLit/tree/8cef51623130f2412da410858e32c322eb796c0a (README, `license.md`, `data/…`).
- Files used: `data/<author>/<work>/<author>.<work>.perseus-{lat,eng}N.xml` and
  `__cts__.xml`. Per-file SHA-256 in `results/alignment.json`.
- Role fill: ADR 0101 (Brownson passage context), `doc/third-party-data.md`
  (Brownson and Fowler rows), ADR 0111 (Source content), #2660 findings
  (pool and OLA depth).
- Other sources: https://en.wikisource.org/wiki/Portal:Classical_Latin_literature (2025-06-09 revision),
  https://en.wikisource.org/wiki/Author:Cornelius_Nepos, https://standardebooks.org/ebooks.

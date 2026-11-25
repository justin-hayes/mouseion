# Latin dictionary candidates (Lewis & Short and alternatives)

**Research date:** 2026-11-25

**Scope:** Which English-language Latin dictionary can play the role LSJ plays
for Ancient Greek: a matched dictionary entry plus a compact definition
outline on cards (ADR 0101, ADR 0104). Wayfinder map "classical Latin in
Mouseion", ticket "Research: Latin dictionary candidates (Lewis & Short and
alternatives)". Primary sources only: the repositories' own licence files and
data, measured with the scripts under
[`latin-dictionary-candidates/`](latin-dictionary-candidates/). Every figure
below is an aggregate; no third-party text or annotation is committed. The
headword match rate against corpus lemmas is out of scope (it waits for the
corpus choice, the lemma-conventions ticket).

## Answer

- **Lewis & Short (Perseus TEI, `PerseusDL/lexica`) can play the LSJ role.** It
  is the same repository, the same licence (CC BY-SA 4.0), and the same
  `<entryFree>` / `<sense level n>` shape as the pinned LSJ: 51,645 entries,
  101,982 senses in five levels. It is the only candidate with a deep sense
  hierarchy, citations and the historical depth of LSJ.
- **The outline is parseable, but less cleanly than Greek.** LSJ marks
  definitions with `<tr>`; L&S marks them with italics (`<hi rend="ital">`)
  that also carry Latin words, abbreviations and cross-references. The ADR
  0104 positional rule (italic text before the first `<cit>` or `<bibl>`)
  yields a definition for 81.6% of senses and 86.5% of entries (44,688 of
  51,645). On a hand-judged sample of 64 entries, 74.5% of the 55 entries that
  are not bare cross-references (Wilson 95%: 0.62 to 0.84) gave a clean
  outline, 14.5% a partial one, 11% a wrong one. A junk filter and a
  cross-reference resolver are needed; see "Failure modes".
- **Lewis's *Elementary Latin Dictionary*** (same repo, same licence) is a
  clean flat-gloss complement, not a replacement. Its `<tr>` markup is closer
  to LSJ's, so definitions parse almost without noise, but 98.2% of its
  senses are level 0: one flat gloss list, no sense hierarchy to outline.
- **Wiktionary Latin (Kaikki)** is already a register row and licence-clean,
  and has structured glosses and principal parts, but no citations and no real
  hierarchy. It stays the lemma-and-gloss fallback, as for the modern
  languages, not the "dictionary entry".
- **Licence:** L&S and Elementary classify as share-alike **Source content**
  under ADR 0111, admitted. The one published lemma-to-headword mapping (the
  CIRCSE `LewisShort` RDF, linked to the LiLa Lemma Bank) is **CC BY-NC-SA 4.0**
  on its README, an NC layer. It is usable only as an Analysis resource under
  ADR 0111's deploy-time rules, never as Source content.
- **Recommendation:** pin Lewis & Short as the Latin dictionary, derive the
  outline with the ADR 0104 rule plus the two fixes below, and treat the
  Elementary dictionary as an optional second source for short outlines. Do
  not build a new dictionary format first; the existing LSJ loader's shape
  (operator-supplied directory, compiled into the index) carries over.

## Candidates and licences

Classification is by the instrument in force in the pinned release (ADR 0111:
terms are read from the licence instrument, not catalogue pages).

| Candidate | Pin | Instrument in force | ADR 0111 class | Size |
|---|---|---|---|---|
| **Lewis & Short**, *A Latin Dictionary* (1879), Perseus TEI `lat.ls.perseus-eng2.xml` | `PerseusDL/lexica@56061ca1` (the LSJ pin), SHA-256 `a21c3799…1836c`; `@1c34cdf7` (2026-10-26) SHA-256 `933ba8fd…1d32` | Root `README.md`: "licensed under a Creative Commons Attribution-ShareAlike 4.0 International License"; root `license.md` is the CC BY-SA 4.0 legal code; the file's own `<availability>` also states CC BY-SA. Print source: 1879, public domain. | Source content, **share-alike, admitted** | 77.4 MB (51,645 entries) |
| **Lewis, *An Elementary Latin Dictionary*** (1890), `viaf2845558.viaf001.xml` | same repo; `@56061ca1` SHA-256 `16db938a…2ca1`; `@1c34cdf7` `f6087b2f…195e` | Same README and `license.md`; file `<availability>` CC BY-SA. Print source: 1890, public domain. | Source content, **share-alike, admitted** | 14.5 MB (17,582 entries) |
| **Wiktionary Latin** via Kaikki/Wiktextract (`lang_code=la`) | existing pin mechanism (`make dictionary-index`); measured on the kaikki.org Latin file extracted 2026-10-30 from the 2026-10-01 dump, wiktextract `19f9a30`, SHA-256 `7b33773b…c3a1` of the 1.25 GB file | Already registered: CC BY-SA 4.0 / GFDL | Source content, **share-alike, admitted** (existing row) | 893,463 records; 48,720 lemma-like |
| **Whitaker's WORDS** `DICTLINE.GEN` | `mk270/whitakers-words@fb7fb6f0` (2026-11-03), SHA-256 `8f6c0fc8…9abf7569` | `LICENCE.txt`: "Permission is hereby freely given for any and all use of program and data"; "freely available to anyone who wishes to use them, for whatever purpose". Not an enumerated licence. | Source content, **permissive custom grant; not in ADR 0111's list. Needs a ruling** (it is no more restrictive than share-alike, so likely admissible, but the ADR names its admitted kinds) | 6.1 MB, 39,335 lines |
| **DCC Latin Core Vocabulary** | web CSV/XML, no versioned repo | DCC page: CC BY-SA 3.0 Unported. Definitions "adapted from various sources, including … the Oxford Latin Dictionary" (provenance of that adaptation **unverified**). | Source content, share-alike, admitted, **with an unverified OLD provenance flag** | 1,000 entries |
| **CIRCSE `LewisShort`** RDF (L&S linked to LiLa) | `CIRCSE/LewisShort@04b4c99d` (2023-06-12), tarball SHA-256 `71ae6d35…ec99`; Zenodo `10.5281/zenodo.8027557` v1.0.0 | README "Copyright": CC BY-NC-SA 4.0; no licence file; GitHub licence field empty; Zenodo licence `other-open`. The source text it derives from is CC BY-SA 4.0 (Perseus). | The RDF as a whole is **NC (unclear: conflicts with its share-alike source)**. Excluded as Source content; the entry-to-lemma links only as an Analysis resource | 113 MB TTL (9.6 MB gz) |
| **LiLa Lemma Bank** | `CIRCSE/LiLa_Lemma-Bank@1a1ee929` (2026-10-20); `lila_db.sql.gz` 12.8 MB | `LICENSE` file: CC BY-SA 4.0 (GitHub SPDX `CC-BY-SA-4.0`); README repeats it. 215,102 lemmas (JOHD data paper). | Analysis resource, share-alike, acceptable | 12.8 MB |

Not examined: Latin WordNet and the other LiLa-linked lexica (derivational,
valency, etymological, polarity; none is a definition dictionary); Gaffiot
(French), Georges (German) and the Oxford Latin Dictionary (in copyright) fail
the English-language or licence requirement; no other open English Latin
dictionary digitisation was found.

## Headword conventions

Measured on Lewis & Short at `1c34cdf7`; the counts at `56061ca1` are
identical except 1 entry (5,580 versus 5,579 entries whose first sense holds
forms children).

- **Key versus printed headword.** Each `<entryFree>` has an ASCII `key`
  (47,292 of 51,645 are `[A-Za-z]+`; the rest are digit-suffixed or contain a
  space) and a first `<orth>` with combining marks: macron in 27,417, breve in
  34,554. 38,341 entries have a first orth equal to the key once marks and case
  are stripped. 7,345 entries carry more than one `<orth>` (variants).
- **u/v and i/j.** Distinct: keys use `j` (716 keys) and `v` (4,372). A corpus
  that writes `iam` finds no `iam` key; `jam` is the entry. Matching belongs to
  the lemma-conventions ticket.
- **Homograph numbering.** A trailing digit on the key: 4,266 keys, 3,113
  entries in 1,495 groups (largest group 16, for example `ne1`..`ne3`,
  `sum1`..`sum3`). 1,154 digit-suffixed keys have a unique base, because the
  2023 entry splits renumbered only some groups. The number is not printed in
  the orth; the key is the only handle.
- **Case.** 8,543 keys are capitalised (proper nouns). The key does not
  case-fold.
- **Enclitics and affixes.** `-ne` is key `ne2`, orth `-nĕ`; `que` is key `que`,
  orth `quĕ`; prefixes such as `sum-` and `vē-` are separate entries
  (`sum3`, `ve2`). 74 keys contain a space (multiword); 11 are apostrophe or
  `!` forms (`satin'`, `ha!`).
- **Deponents and principal parts.** Principal parts sit in `<itype>`
  (45,249 entries have one) and are partial: `sequor` has only `sĕcūtus`, the
  perfect participle, in its itype; `amo` has `āvi, ātum, 1`. Deponents are
  marked in `<pos>` as `v. dep.` (207 entries). There is no machine-readable
  full principal-parts tuple.
- **Entry types.** `main` 40,042; `greek` 4,917; `hapax` 4,481; `gloss` 1,838;
  `spur` 256; `foreign` 111.
- **Greek script defect.** 1,061 entries carry a Cyrillic `у` in their first
  `<orth>` (Greek-derived words), and 1,078 Cyrillic characters occur in total.
  A headword normaliser must map it.
- **Elementary dictionary.** 17,582 entries, all `main`; 12,902 orths with a
  macron and none with a breve; keys are mostly ASCII (17,508) with 1,180
  digit-suffixed homographs; `v` keys 1,685 and no `j` keys, so it spells
  consonantal `i` as `i`. The source file uses 38 SGML entities (65,452
  references) that need resolving before an XML parser reads it; three
  (`ymacr`, `acutemacr`, `cj`, 70 references) are not in HTML5. The shipped
  loader would need a pre-pass (`prep_elem.py`).
- **Wiktionary Latin.** The lemma `word` is a macron-free page title (14 of
  48,720 lemma records contain a macron); macrons come from the head-template
  expansion (82% of lemma records: 39,851 of 48,631) together with `forms`
  (principal parts, present on 97.6%: 47,552). 10,543 are capitalised and
  1,117 are multiword. It is the only candidate with machine-readable forms.
- **Whitaker.** Four stems per line plus part of speech, declension and
  frequency codes; no macrons, ASCII only; headwords are reconstructed from
  stems and paradigm numbers, which is Mouseion-run logic over the data (not an
  analyser run on text) but still code Mouseion would own.

## Entry structure against ADR 0104

ADR 0104's rule: each sense's definitions are the English glosses given
before the sense's first citation; the card shows a compact outline of
definitions in source order, with the full entry behind a toggle.

### Lewis & Short

TEI shape: `<entryFree key id type>` holding `<orth>`, `<itype>`, `<gen>`,
`<pos>`, `<etym>` and a flat run of `<sense level n id>`. Levels are 1 to 5
(66,114 / 16,749 / 10,238 / 3,603 / 5,278 senses at levels 1 to 5), with labels
roman, upper-case letter, arabic, lower-case letter and parenthesised letter at
levels 1 to 5, as LSJ's flat-siblings-with-level layout. Citations are
`<cit><quote>` plus `<bibl n="urn:cts:…">`, so the CTS URN carries a machine
reference (not required for the outline).

- **Definition marking:** italics (`<hi rend="ital">`, 224,891 uses). L&S gives
  the English definition in italics and cites usages after a colon, mirroring
  LSJ's definition-then-citations order. There is no `<tr>`.
- **Positional rule results (whole file):** 83,184 of 101,982 senses
  (81.6%) have at least one definition; 44,688 of 51,645 entries have a sense
  with one; 44,122 entries have a definition in their first sense. 172,185
  definition strings, mean about two per defined sense.
- **Entries with no `<sense>` at all:** 6,658 (12.9%), almost all
  cross-references, participle pointers ("Part. of X") or glosses; 4,842
  match a cross-reference pattern. Only 19 have entry-level italics. These
  have no definition to outline and need a resolver to the base entry. 8,670
  short (160 characters or fewer) entries match a cross-reference pattern.
- **Sense-level labels repeat.** 65,635 senses at level 1 carry the label `I`:
  the first sense of an entry is often a fragment of the forms paragraph, not
  a sense. 5,579 entries (10.8%) have forms children (`orth`, `itype`, `pos`,
  `gen`) inside the first `<sense>`. The numbering is therefore unusable; the
  `id` order (`n2280.0`, `.1` …) is the stable key.

### Hand-judged sample

Two seeded samples of `main` entries (seed 2662, 24 entries; seed 7, 40
entries; `sample.py`). Each entry was judged against its rendered outline:
**clean** (the outline's definitions are the entry's definitions), **partial**
(a definition fragment or junk string among correct ones), **wrong** (the only
definition is junk), **xref** (no sense; a cross-reference).

| | clean | partial | wrong | xref | total |
|---|---|---|---|---|---|
| Entries | 41 | 8 | 6 | 9 | 64 |

Over the 55 entries with a sense (non-xref): clean 74.5% (Wilson 95% 0.62 to
0.84), clean or partial 89.1% (0.78 to 0.95), wrong 10.9%. Cross-reference
entries are 14.1% of the sample (0.08 to 0.25), matching the whole-file 12.9%
of entries with no sense. This is a small sample: the figures bound the failure
modes but are not a shipping bar.

### Failure modes (L&S)

1. **Abbreviation italics taken as definitions.** `fin.`, `init.`, `med.`,
   grammar labels (`gen`, `inf`, `adj. f`) and `P. a.` are italicised. Whole
   file: 13,524 of 172,185 definition strings are 3 characters or fewer or match
   such a list (7.9%), and 2,137 entries (4.8% of entries with a definition)
   have only such strings. All 6 "wrong" sample entries were of this kind.
   A stop-list plus a minimum length would remove most; this is a closed list.
2. **Split definitions.** An italic gloss broken by a roman name or word
   ("a people of" [name]; "of", "relating to generation"; "To", "for
   nothing"), so a fragment becomes an outline line. Present in 4 of the 8
   partial sample entries.
3. **Forms paragraph in the first sense** (5,579 entries): usually harmless
   for the outline, but a stray definition appears when a gloss sits in the
   forms tail. The first sense without a definition (65,000 level-1 senses
   labelled `I`) cannot be told apart from a real sense by label.
4. **Cross-reference entries** (6,658 with no sense; "v. X", "Part. of X",
   "= X"): no outline, need resolving to the target entry's outline. The target
   is named only in running text.
5. **Duplicated or mislabelled levels** (365 entries have a level-1 label other
   than roman, such as `I. A.`): the outline's numbering should use `id` order
   and level, not `n`.
6. **Mixed script and encoding:** Cyrillic `у` (1,061 entries), beta-code
   Greek in the archival `eng1` file (use `eng2`).
7. **Italics for Latin words and synonyms** ("diligere", "esteem") appear
   inside a sense before its first citation; they are not separable
   positionally. Not counted in the figures above; present in the "partial"
   sample entries.

Compared with LSJ: ADR 0104 measured 137,235 of 140,333 senses with glosses
(97.8%) having definitions on the Greek side, against 81.6% of all L&S senses
here (a different denominator: LSJ counts senses with glosses, L&S counts
every sense including label-only parents). Both dictionaries leave hierarchy
parents such as "Lit." and "Transf." without a definition, which the ADR 0104
"number only" rule already covers.

### Lewis, *Elementary Latin Dictionary*

TEI shape: `<entry key type>` with `<form><orth>`, `<gramGrp><itype>`, a `<sense
level n>` and `<trans><tr>` for the English gloss, `<foreign>` for Latin
quotations, and `<usg>` for the author abbreviation. Whole-file figures:

- 17,582 entries; 17,100 senses; 782 entries without a sense.
- Sense levels: 16,785 at level 0, 252 at level 1, 28 at level 2, 35 at level
  3. The structure is one sense per entry with the glosses inline; the
  hierarchy is a rarity (about 1.8% of senses).
- 15,514 senses (90.7%) and 15,380 entries have a gloss before the first
  `<foreign>`; 16,499 entries contain a `<tr>`; 6,453 have two or more.
- 44,129 `<tr>` strings; 788 are 3 characters or fewer (1.8%); 450 start "P. of"
  or "P. a. of", 86 start "see" (cross-references misplaced as glosses).
- Hand-judged sample (28 entries, seed 2662, `sample_elem.py`): 5 cross-
  reference entries; of the other 23, 18 clean, 5 partial (a stray `n`, a
  trailing fragment such as "my embrace"), 0 wrong. A small sample.

The Elementary dictionary's definitions parse more reliably than L&S's, but it
supplies no sense hierarchy to outline. It could feed a one-line outline for
common words, with the full L&S entry behind the toggle.

### Wiktionary Latin (Kaikki)

Already structured. 48,720 lemma-like records (the 844,743 form-of and
alt-of records are excluded), 73,571 senses, 73,507 with glosses (99.9%),
34,819 lemma records with one gloss-bearing sense, 13,901 with two or more
(groups: 8,526; 2,914; 1,180; 518; 285; 159; 255 at 8 or more). 4,047 senses
carry a nested gloss list (a parent gloss plus a sub-gloss). 7,308 lemma records
have usage examples; 9,341 senses do. Tags mark register and period (for
example "New-Latin" 2,926, "Medieval-Latin" 2,551, "Late-Latin" 1,758
senses) so a classical filter is partly possible. There is no citation to
author and work with a reference. No parse failure mode in the entry shape
itself; the weakness is the content (no citations, uneven depth).

### Whitaker's WORDS

`DICTLINE.GEN` is fixed-width: 39,335 lines, all 39,335 parse against the
column layout, all with a gloss. Parts of speech: N 19,619, ADJ 9,159, V 7,791,
ADV 2,204. Glosses are `;`-separated strings with 1 segment in 16,306 lines,
2 in 11,704, 3 in 6,753, 4 in 3,121 and 5 or more in 1,451. The "age" code is
`X` (unknown) on 28,963 lines (73.6%), so it does not isolate a classical
subset. There are no senses, sub-senses or citations, and no headword: the
headword is rebuilt from stems and a paradigm code. It suits an outline-only
glossary, not an entry.

### DCC Latin Core Vocabulary

1,000 headwords with macronised dictionary forms and principal parts, a gloss,
a part of speech, a semantic group and a frequency rank. One flat gloss each.
Useful as a frequency cross-check, not as the dictionary.

## Existing lemma-to-headword mappings

The only one published by a dictionary project is **LiLa's L&S link**
(`CIRCSE/LewisShort`). Its RDF reuses the Perseus entry `id` as the entry key
(`ls_lexical_entry:nNNNN`) and links each entry to a LiLa Lemma Bank lemma by
`ontolex:canonicalForm`.

- 47,142 lexical entries, all 47,142 present in the current Perseus file by
  `id` (the join is stable across the 2023 snapshot and the 2026-10-26 file).
- 43,914 entries (93.2%) carry at least one lemma link; 237 carry more than
  one; 41,885 distinct lemma identifiers.
- Of the Perseus file's 51,645 entries, 4,503 have no RDF entry (3,760 of
  type `main`); 33,279 of 40,042 `main` entries (83.1%) are linked.
- The README calls it "a work in progress NOT yet complete".
- The licence is NC (see the table). The link is the part of the RDF that is
  CIRCSE's own contribution, so it is the one piece worth keeping: as an
  Analysis resource it would be deploy-time fetched under ADR 0111 conditions
  1 to 3, never shipped. The Lemma Bank itself is CC BY-SA 4.0.
- Whether the chosen corpus's lemmas are LiLa identifiers is a corpus
  question. Treebanks LiLa links (ITTB, UDante, LASLA, Querolus) are
  mostly outside the classical definition, apart from LASLA (the Opera Latina
  corpus); that is for the corpus ticket.

## Mouseion-side implications

- **Carry-over from LSJ:** an operator-supplied directory compiled into the
  dictionary index; a new register row per pin; a `dictionary` notice with the
  pin, as `AncientGreekLSJNotice` does for Greek. L&S shares the LSJ pin
  `56061ca1`, so one `PerseusDL/lexica` checkout serves both.
- **Pin choice:** both pins carry the CC BY-SA statement in the file itself and
  in `license.md`. `56061ca1` keeps the LSJ pin; `1c34cdf7` is the later
  Perseus commit (2026-10-26, 25 changed lines in L&S: licence wording and
  typo fixes, 51,645 entries in both). Choosing `1c34cdf7` would move the LSJ
  pin too unless the two are pinned separately, which the register allows.
- **ADR 0104 changes needed for Latin:** the `<tr>` definition source becomes
  italics plus a stop-list; a cross-reference resolver is new; level labels
  cannot be trusted (use `id` order and `level`). These are implementation
  details of the dictionary adapter, not a change to the card policy.
- **Not decided here:** whether Latin cards keep the "source-evidence" policy
  or gain a Wiktionary fallback for entries with no usable outline; that is a
  card-policy grilling.

## What would change the recommendation

- A lemma-match measurement (the lemma-conventions ticket) showing L&S's
  `j`/`v` keys and `u`/`v` handling lose too many corpus lemmas; Wiktionary
  and Elementary would then rank higher for coverage.
- A ruling that the Whitaker permissive grant is not admissible (it is a
  fallback only).
- A request for definitions without a cross-reference resolver: Elementary
  has a lower cross-reference share by type but a similar 25% of its entries
  are short pointers (4,439 entries of 120 characters or fewer).
- Evidence that the CIRCSE RDF's NC marking is an error (a clarified licence
  from the maintainers); then the link layer could be a plain mapping.

## Reproduction

Scripts in [`latin-dictionary-candidates/`](latin-dictionary-candidates/); each
prints aggregates only. Downloads were kept in an untrusted directory and read
with `python3 -I`. Usage:

- `measure_ls.py`, `headwords_ls.py`, `sample.py <seed> <n>`: Perseus L&S
  `lat.ls.perseus-eng2.xml`.
- `prep_elem.py <in> <out>` resolves the SGML entities of the Elementary file
  into a scratch copy, then `measure_elem.py`, `sample_elem.py` read it.
- `measure_lila_links.py <ttl> <ls.xml>`: the LiLa entry-to-lemma links.
- `measure_whitaker.py DICTLINE.GEN`, `measure_kaikki.py la.jsonl`.

The kaikki.org postprocessed Latin file used here is deprecated upstream
("will be removed"); a Latin pin would filter the raw Wiktextract dump
(24.8 GB, 2.9 GB compressed on 2026-10-30) by `lang_code=la`, as the existing
index does for the other languages, and its SHA-256 and the wiktextract commit
would be recorded then.

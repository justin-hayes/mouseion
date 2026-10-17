# Open Ancient Greek lexica for Anabasis Book 1

Research for [the Greek lexica survey](https://github.com/justin-hayes/mouseion/issues/2133),
feeding [the contextual-gloss prototype](https://github.com/justin-hayes/mouseion/issues/2130)
under [the lexicon map](https://github.com/justin-hayes/mouseion/issues/2127). This is a
source and licence survey plus a lookup-coverage measurement. It is not contextual-sense
evaluation, not gold annotation, and not a claim that any source is accurate enough for
contextual glosses. No production code or contract changed.

Inputs, pins, hashes and rights are in [SOURCES.md](ancient-greek-lexica/SOURCES.md) and
[NOTICE.md](ancient-greek-lexica/NOTICE.md). Scripts are in `ancient-greek-lexica/scripts/`.
Measurement output is in `ancient-greek-lexica/summaries/`.

## Recommendation

1. **LSJ (Perseus `pdllex/grc/lsj`) is the primary candidate for attributable, structured
   citation evidence.** It is the only source here with machine-readable Xenophon
   passage references (`urn:cts:greekLit:tlg0032.tlg006…:book:chapter:section`). 436 of
   1,429 identities have a Book 1 URN candidate. Licence: the component README states
   CC BY-SA 4.0. The 1940 print's status is unresolved (see NOTICE), so it is a
   research candidate, not a shipping one. Pin: `PerseusDL/lexica@56061ca1`.
2. **Kaikki/Wiktextract `grc` is the candidate for part-of-speech and gloss data**, not
   for homograph-safe sense identity. Licence: Wiktionary CC BY-SA 4.0 / GFDL, reusers
   may choose either. Quoted third-party translations are excluded. Its Book 1 Xenophon
   examples are sparse (17). Homograph separation is weak: 156 identities' headwords have
   more than one entry, and most entries have no etymology number. The download has no
   immutable upstream URL and must be mirrored with a hash before production use.
3. **Do not ship Middle Liddell (Perseus `1999.04.0058`) until its licence conflict is
   resolved.** The TEI header says non-commercial; the Perseus hopper page and the fork
   say CC BY-SA 3.0 US. Its Xenophon references are `Xen.` with no book or chapter, so
   Book 1 cannot be measured.
4. **Excluded:** DGE (CC BY-NC-SA 3.0 ES) and Bailly 2020 (CC BY-NC-ND 4.0). Scaife and
   Logeion are not assessed. Autenrieth and Cunliffe are Homeric and Lewis & Short is
   Latin, so they are out of scope for Attic prose.

## Corrections to the first draft

The first draft's numbers were not all reproducible. The table lists each wrong figure
and its replacement from the authoritative measurement JSON
(`summaries/book1-lookup-measurement.json`). Rerunning `measure.py` in this pass produced
that JSON exactly.

| First draft | Corrected | Why |
|---|---|---|
| 1,428 identities (1,407 plain) | 1,429 identities (1,408 plain, 21 numbered) | Rule difference in the first draft's count; the JSON rule is the one used throughout |
| 410 LSJ identities / 585 Book 1 citations | 436 identities with a Book 1 URN candidate; 580 Book 1 URN strings across all LSJ files | The 585 figure was not reproduced |
| 1,304 LSJ any-entry (91.3%) | 1,291 / 1,429 = 90.3% | Recomputed from the JSON |
| Kaikki any-entry 83.0% | 1,185 / 1,429 = 82.9% | Denominator correction |
| Kaikki POS-compatible 74.4% | 1,062 / 1,429 = 74.3% | Denominator correction |
| 6,014 occurrences (6,017 ADR count) | 6,017 | 10,462 printed − 4,267 excluded by class − 178 εἰμί = 6,017 |
| 33 Anabasis refs, 16 in Book 1 | 35 in the standard `Xenophon, Anabasis` form, 17 in Book 1 | One further ref uses another form (`Xenophon, chapter 8, in Anabasis, volume 7`) and is not counted |
| Kaikki translations: 34 of 36 carry translations, 35 have no attribution | Of 35 standard-form examples: 27 real translations, 6 placeholders, 2 name a translator | Placeholders are not translations; Brownson and Dakyns are named in text |
| Middle Liddell 30,440 entries | 30,658 indexed main entries (+18 keys excluded) | Recount |
| LSJ 28 TEI files | 27 files (`eng1` to `eng27`) | Recount |
| Token-weighted 94.9% / 95.0% / 88.2% | 94.9% / 94.3% / 88.1% (Kaikki / LSJ / ML) | Recomputed from 6,017 occurrences |
| LSJ plain key 84.9% | 84.0% (1,182 / 1,408) | Denominator correction |
| Middle Liddell any-entry 76.6% | 76.6% (1,094 / 1,429) | Unchanged |
| Scott's death not verified; EU term hinges on Scott | Scott d. 1887 (Rochester Cathedral; MODS). The last-dying author for the 1889 Middle Liddell is Liddell (d. 1898), so the print is public domain in the EU | Wrong test in the first draft |
| Bailly death 1916 | 1911 (Wikipedia, English and French) | Unsupported figure withdrawn |
| Morpheus `PerseusDL/morpheus@b1b33c56` as a licence candidate | Removed | Not fetched in this pass; OGA's `morpheus:N` field is OGA's, and no Morpheus source is claimed |
| "Morpheus digits" compared with LSJ/ML digits | Never compared | Digit equality is not a match criterion (see below) |

## Source comparison

| Source | Pin | Format | Licence (as stated by the source) | Book 1 Xenophon | Homograph key |
|---|---|---|---|---|---|
| LSJ, Perseus `pdllex/grc/lsj` | `PerseusDL/lexica@56061ca1` | TEI P4 XML, 27 files, 111,584 indexed main entries (+311 keys excluded) | Component README: CC BY-SA 4.0; repo-level Tufts "personal use" wording not reconciled | Yes, URN `…:1:chapter:section` | beta-code key with trailing digit |
| Kaikki / Wiktextract `grc` | Kaikki raw dump, extracted 2026-10-03 (see SOURCES) | JSONL, 68,196 top-level entries, 63,448 distinct headwords | Wiktionary CC BY-SA 4.0 / GFDL (WMF terms); extract has no licence of its own | 35 examples in standard form, 17 in Book 1, as `ref` text | `etymology_number` (1,345 entries) |
| Middle Liddell, Perseus `1999.04.0058` | fork `blinskey/middle-liddell@5db8d88a` | TEI P4 XML, 30,658 indexed main entries (+18 keys excluded) | **Conflict**: TEI non-commercial; hopper and fork CC BY-SA 3.0 US; second copy unlicensed | No: `Xen.` without book or chapter | beta-code key with trailing digit |

## Measurement

Denominator: the pinned OGA file's `Xen. Anab._1.*` labels, 10,572 native nodes. 110
empty nodes (POS `-`) are excluded, leaving 10,462 printed tokens. Excluded by class:
`c` 756, `u` 1,157, `l` 1,047, `r` 579, `p` 721, `i` 7; εἰμί by exact lemma 178.
What remains is 6,017 vocabulary occurrences.

Identities are `(canonical lemma, trailing digit or none, coarse class)`. The canonical
form is NFC, lowercase, final sigma folded to σ, diacritics kept (ADR 0099 identity rule
applied heuristically). The class is a letter map (n/a/d/v → NOUN/ADJ/ADV/VERB).

| Measure | Result |
|---|---|
| Identities | 1,429 (21 numbered, 1,408 plain, 152 capitalised-only) |
| Kaikki headword has any entry | 1,185 / 1,429 = 82.9% |
| Kaikki headword has POS-compatible entry | 1,062 / 1,429 = 74.3% |
| LSJ headword has any entry | 1,291 / 1,429 = 90.3% |
| LSJ plain key present (no digit) | 1,182 / 1,408 = 84.0% |
| Middle Liddell any entry | 1,094 / 1,429 = 76.6% |
| Middle Liddell plain key present | 1,033 / 1,408 = 73.4% |
| Token-weighted any entry (Kaikki / LSJ / ML) | 94.9% / 94.3% / 88.1% |
| Capitalised-only, any entry (Kaikki / LSJ / ML) | 112 / 71 / 64 of 152 |
| Identities with Book 1 Xenophon examples (Kaikki / LSJ URN / ML) | 17 / 436 / not measurable |

Reading these numbers: they measure headword and key existence, not sense coverage.
"Any entry" for a numbered identity means any homograph of that headword, which is a
conflation. The plain-only figures are the conservative ones. The token-weighted figures
are high because frequent words are well covered, and they say nothing about sense
resolution.

## Crosswalk tiers

`summaries/crosswalk-tiers.json` (from `scripts/crosswalk_tiers.py`) counts candidates per
identity. Each source's tiers sum to 1,429.

| Tier | Kaikki (POS) | LSJ (headword) | ML (headword) |
|---|---|---|---|
| Unique match | 989 | 1,221 | 1,059 |
| Multiple candidates (homograph-ambiguous) | 73 | 70 | 35 |
| Entry exists, no POS-compatible entry | 123 | n/a | n/a |
| No entry | 244 | 138 | 335 |

These are candidate counts. A unique candidate is not a correct sense.

## Proposed OGA-to-dictionary crosswalk

This is a proposal for review, not an implemented rule.

1. **Canonical key.** NFC, lowercase, final sigma folded, diacritics kept. Keys that fail
   beta-code conversion are reported as unmatched, not guessed.
2. **Coarse class.** Derived from the OGA POS letter. At implementation, use
   `oga-agdt-upos/4` from ADR 0099 instead of this heuristic class map.
3. **Candidate sets.** Kaikki is filtered by canonical headword and POS class. LSJ and
   Middle Liddell have no POS field, so they are filtered by canonical headword only.
4. **The OGA discriminator is never compared with a dictionary number.** It is kept as
   OGA's evidence only. A numbered dictionary entry is not chosen by digit. Digit-equal
   coincidences were observed (for example, ἐλελίζω: Kaikki etymology 2 and ML `e)leli/zw2`
   both describe the battle-cry sense, but LSJ's `2` is an aorist sense; this is not a
   rule).
5. **Tiers.** Unique match, multi-candidate (homograph-ambiguous, needs a further rule),
   entry without POS match, no entry. Only unique matches can be auto-linked, and even
   then the link is a headword link, not a sense.
6. **Citation evidence (later, not a rule now).** For the 436 identities with an LSJ Book
   1 URN, a candidate sense whose URN matches an OGA token's `ref=Xen. Anab._1.c.s` at
   chapter and section can be treated as citation evidence. That needs its own evaluation
   before use, and no WSD evaluation is done here.

## Entry examples

Short fragments to show structure only.

- **Kaikki `grc`, βάρβαρος** (adj, one entry, four senses, all glossed). First gloss
  "non-Greek-speaking; foreign". Has a Book 1 example at `Xenophon, Anabasis 1.2.14`.
- **Kaikki `grc`, λέγω** (two verb entries, both with no etymology number). Glosses
  "to put in order, arrange, gather" and "to say; to speak". This is the clearest
  homograph risk: a sense-level claim cannot be made without a rule that separates these.
- **Kaikki `grc`, ἕως** (three entries: noun etymology 1 "Attic form of ἠώς"; conjunction
  etymology 2 "until; till"; adverb etymology 2 "for a time"). Etymology numbers are
  present here but not for most entries.
- **Kaikki `grc`, ἐλελίζω** (verb etymology 1, four senses "to whirl around, turn
  around"; verb etymology 2, one sense "to cry ἐλελεῦ").
- **LSJ, key `a)yi/nqion`** (`grc.lsj.perseus-eng1.xml`). Sense A "wormwood, Artemisia
  Absinthium", with several `<bibl n="urn:cts:greekLit:…">` citations, including
  `tlg0032.tlg006.perseus-grc1:1:5:1` (Xenophon, Anabasis 1.5.1). This is the Book 1
  URN form the crosswalk uses.
- **LSJ, keys `e)leli/zw1`, `e)leli/zw2`** (`eng5`), **`le/gw1`, `le/gw2`** (`eng12`).
  Digits distinguish entries here. Middle Liddell has the same keys with the same
  digits, but their senses are not guaranteed to match.
- **Middle Liddell, key `a)/yinqos`** (a spelling variant). Etymology "Deriv. unknown,
  NTest." and sense "wormwood". The Xenophon citations in this file are `Xen.` with no
  book or chapter.
- **OGA CoNLL-U, `Δαρείου`** (lemma `δαρεῖος`, `n-s---mg-`, `ref=Xen. Anab._1.1.1`). The
  proper-name lemma is lowercased. The proper-name rule in ADR 0099 is computed per lemma
  over the whole pinned work; this heuristic does not model it.
- **OGA CoNLL-U, `ἕως`** (AuxC, POS `c`, `ref=Xen. Anab._1.3.11`). It is excluded from the
  vocabulary count under the `c` rule, but see the caveats for `c`.

## Limitations and disclosed conflations

- **Heuristic identity and POS.** The class map is a reimplementation for this measurement.
  It is not the ADR 0099 `oga-agdt-upos/4` mapping. The identity count is close to the
  production figure but not confirmed identical.
- **`c` and εἰμί exclusions diverge from ADR 0099.** `measure.py` excludes all 756 `c`
  tokens. Of those, 277 are AuxC, 473 COORD, 5 AuxY, and 1 ADV. ADR 0099 splits `c` by
  AuxC, so the vocabulary count is an undercount by up to 277 tokens. εἰμί is excluded
  here but mapped to VERB in ADR 0099.
- **Beta-code conversion.** LSJ has 311 keys with unrecognised characters (`^`, `&`, `;`,
  `[`, `]`, `_`, `'`), which are editorial markers and entities. Middle Liddell has 18.
  Those keys may fail to match. The effect is not measured.
- **Homograph digits are not comparable across sources.** OGA's digits come from
  Morpheus. Digit-equal matches in LSJ or Middle Liddell are coincidental in the checks
  made. Numbered identities are reported separately and are not a match criterion. 17 LSJ
  and 12 ML digit-equal matches are marked UNVERIFIED in the JSON.
- **Kaikki homographs.** λέγω has two verb entries with no etymology number, so any
  sense-level claim for it is a conflation. Most Kaikki entries have no `etymology_number`.
- **Kaikki Book 1 examples.** Of the 17 Book 1 examples, 13 carry real translations, 2 are
  placeholders, and 1 is Brownson-attributed. Dakyns is named in one standard-form example
  outside Book 1. The unattributed English should not be used in
  deck output without checking.
- **Wiktionary-quoted translations.** Wiktionary examples can quote third-party
  translations under their own copyright. Kaikki inherits the Wiktionary licence, not the
  rights of quoted translators. This needs a rule before examples are shipped.
- **LSJ Book 1 citations are presence, not sense.** The 580 Book 1 URN strings (436
  identities) are counted from `<bibl>` URN text. They are not checked against the
  Marchant 1904 text, and LSJ sense numbering is not mapped to Kaikki or OGA identities.
- **Middle Liddell cannot be measured for Book 1.** Its Xenophon references have no book
  or chapter. The Middle Liddell zero is "unmeasurable", not "no evidence".
- **Pins.** The Kaikki subset is a derived local file, not an upstream commit. The
  production derivation uses a mutable `raw-wiktextract-data.jsonl.gz` and the installed
  dump's extraction date is not recorded. This pass does not establish which dump is
  installed.
- **Not run.** No contextual-sense evaluation. No human gold labels. No DGE, Bailly, Pape,
  or Passow text was fetched (DGE and Bailly were checked for licence only).

## Executable truths (local audit at `6e02d180`)

The parent's audit reported these. The three code claims were rechecked by grep at the
stated lines; the rest is taken from the audit as written.

- `nlp/scripts/derive_dictionary_index.py:324` stores `glosses[0]` as the Gloss. Nested
  gloss breadcrumbs that end in more specific senses are dropped. (Verified by grep.)
- Examples keep text only. Structured author, ref and translation are dropped. Topics,
  Tags, Phrase, Gender, Article, Plural, and IPA are retained.
- Greek rows (`:454-505`) keep source row boundaries, `etymology_number`, and NFC forms.
  Forms discard tags, and the SQL table has no source row ID.
- `internal/dictionary/greek_index.go:50` selects only `headword, upos, senses_json`. It
  does not select `etymology_number` or `forms`. The lookup at `:112-131` combines rows
  into flat senses by lemma and UPOS, and ignores the discriminator. (Line 50 verified.)
- `internal/enrichment/lexical.go` has no discriminator or morphology in the request
  struct.
- `internal/dictionary/index.go:208-267` deduplicates by hash of (language, lemma, UPOS,
  Gloss, Phrase, Examples, Topics, Tags), not by source entry or etymology ID. Retained
  storage and flattened lookup identities are different things. The current code does
  **not** provide homograph-safe sense provenance.
- `OrderSenses` (`lexical.go:87-173`) is a Lesk-style overlap and tie ordering, not
  source-based sense resolution. (Verified by grep.)
- `internal/dictionary/attribution.go` says CC BY-SA 4.0 / GFDL. That is an attribution
  string, not proof of licence.
- `internal/ancientgreek/ancientgreek.go:23-39` pins the OGA file at 7,438,037 bytes,
  SHA-256 `c51bba2d…eb61`, covering 1.1.1–1.10.19. This pass matched that size and hash
  from the Zenodo zip.
- `cmd/greekhitrate` reports against the installed Browse projection across current
  analyses. It is not a Book 1 denominator. This research's local-file coverage must not
  be confused with that report.

## Documentation conflicts

- ADR 0064 cites Wiktionary as CC BY-SA 3.0 / GFDL. ADR 0079 has no version. The current
  Wiktionary statement and `attribution.go` say CC BY-SA 4.0 / GFDL. The ADRs look stale.
  Not changed here.
- The repo-level LSJ README says Tufts holds the overall copyright and materials are
  "provided for the personal use of students, scholars, and the public", and the
  component statement says CC BY-SA 4.0. The component statement is specific; the
  repo-level wording is not reconciled with it. Whether it blocks redistribution in decks
  needs a decision from the parent.

## Follow-ups

1. Resolve the Middle Liddell licence conflict with Perseus, or exclude it for good.
2. Settle the LSJ print-edition status (McKenzie's death date, and US renewal) and the
   Tufts clause before any LSJ text or definition ships.
3. Mirror a dated Kaikki/Wiktextract `grc` extract with a SHA-256 pin before production
   use. Record the dump date and the extractor commit, and resolve `e3d6d4e`.
4. Replace the heuristic class map with ADR 0099's `oga-agdt-upos/4` and `ancient-greek/1`
   rules, including the AuxC split and εἰμί → VERB. Then rerun the tiers.
5. Agree a source-level homograph rule before reporting sense-level coverage.
6. Fix the CC BY-SA 3.0 references in ADR 0064 and ADR 0079 in a separate change.

# Graded-reader import path: what a second `grc` Classical Book needs

**Research date:** 2026-10-25

**Ticket:** [#2302](https://github.com/justin-hayes/mouseion/issues/2302), under
the wayfinder map [#2299](https://github.com/justin-hayes/mouseion/issues/2299)
(public-domain graded Ancient Greek readers).

**Scope:** A read-only survey of this repository. It asks what it would take to
register a second Classical Book whose Work is a graded reader (for example
Farnell's *Tales from Herodotus*), given the current Ancient Greek import path.
No external sources were fetched and nothing was run. Every claim below is from
the code, migrations, ADRs and `CONTEXT.md` at the commit this branch was cut
from. Line numbers are against that commit.

**Standing rule:** no non-commercial (NC) licence anywhere. Treebanks, models,
lexica and text digitisations are usable only when confirmed not NC. Nothing in
this document changes that.

## Short answer

Registration is almost free. The shared tables (`works`, `editions`,
`reading_units`, `book_reading_units`) are keyed by a definition key and need no
schema change for a second Book. The real work is in the **adapter**, which is
hard-wired to the OGA CoNLL-U dialect and to AGDT tags, and in **passage
context**, which is hard-wired to Brownson's translation of the Anabasis and
silently shares one translation table across every `grc` Book an owner has.

The recommended pilot is one reader as **one Book** (one Reading unit covering
the whole reader), with lesson/section citations written as numeric dotted
citations, a new reader adapter, and **no English** in the first cut. That
withholds passage context entirely, so it needs no migration. A migration is
needed only before any reader that ships English is imported, and it must be
human-reviewed.

## 1. What is Anabasis- or OGA-specific, and what is reusable

### Reusable as is

| Part | Where | Why it is reusable |
| --- | --- | --- |
| `ReadingUnitDefinition` and its `Validate` | `internal/domain/classical.go:63-119` | Generic: key, language, Work title/author, edition label, citation range, pinned file name, size, SHA-256, interpretation version. |
| Pinned, bounded file read | `internal/ancientgreek/import.go:224-257` (`readPinnedBytes`) | Works for any file; refuses size and digest mismatch. |
| Registration service | `internal/ancientgreek/ancientgreek.go:71-98` | Takes any definition. Idempotent per learner and Reading unit. |
| Legacy-row refusal | `sqlc/queries/classical.sql` (`ListLegacyLanguageRows`) | A learner who already owns a registered `grc` Book passes for a second one. |
| Writer and publication | `internal/analysis/greek_import.go:30-120` | Stores source material, run, corpus, tokens and publishes through the normal pipeline. Only the run's identity fields come from the definition. |
| Citation parsing and cut | `internal/ancientgreek/ogasrc.go:364-385`, `:660-693` | Dotted numeric citations of any depth; missing components count as zero. |
| Form search key, headword, discriminator | `ogasrc.go:305-338`, `:539-548` | Generic `grc` token identity. |
| Canonicalisation profile | `internal/canonicalization/ancient_greek.go` | Language-level, not text-level. |
| Concordance, Browse, coverage, prepared decks, LSJ | `sqlc/queries/concordance.sql`, `internal/persistence/*` | Scoped by owner and language, not by Book. A second `grc` Book joins them without code change (see section 4). |

### Anabasis- or OGA-specific (must be replaced or parameterised)

| Part | Where | Note |
| --- | --- | --- |
| Single definition value | `internal/ancientgreek/ancientgreek.go:23-39` (`AnabasisBook1`) | Compiled value, fine to add more. |
| CLI hard-wires one definition | `cmd/greekregister/main.go:102`, `:112` | `Register(..., AnabasisBook1)` and `pinnedSources{Annotation: AnabasisBook1, Translation: BrownsonAnabasisBook1}` are literals. A second definition needs a selector. |
| Parser hard-wired to oga_src | `internal/ancientgreek/import.go:164` (`parseOgaSrc`) | `Import` has no parser seam. A new source format needs a seam here. |
| AGDT part-of-speech mapping | `ogasrc.go:426-487` (`nativePOSValues`, `validateNativePOS`, `mapUPOS`) | Native AGDT values `n v a d l g c r p m i u`, with `AuxC` special-cased. A reader without AGDT tags does not use it. |
| AGDT postag positions | `ogasrc.go:489-535` (`featureSlots`, `featureMappings`) | Nine-character AGDT postag. |
| MISC citation reference | `ogasrc.go:169-181` (`citationFromMisc`) | `ref=<abbr>_<citation>` in oga_src MISC. |
| Ellipsis tokens | `ogasrc.go:551-553` | `-` and `x` are AGDT-only. |
| Proper-name rule label | `import.go:43` (`oga-anabasis-proper-name/1`) | Name is Anabasis-specific; the rule is generic. Rename or version per adapter. |
| Media type constant | `import.go:35`, `internal/domain/classical.go:15` (`application/x-conllu`) | Used by the writer and by `contentSignalFromView` (`internal/persistence/sqlc.go:312`). A non-CoNLL-U artifact gets `ContentNonEPUB`. See section 5. |
| Single unit per Book | `internal/analysis/greek_import.go:145-160` (`importedUnits`) | One unit per imported Reading unit, titled by the Work. This is right for one-Book-per-reader; it is not "units per lesson". |
| Brownson TEI adapter | `internal/ancientgreek/brownson.go` (`parsePerseusEng2`) | Perseus `perseus-eng2` book/chapter/section TEI. A reader's English, if any, needs its own adapter. |
| Passage labels | `internal/cardexport/passage.go:13-16` | Constants `Brownson (1922)` and `Xen. Anab.` are rendered into every passage line. See section 3. |
| Source notice | `internal/dictionary/attribution.go:8-16`; `internal/cardexport/cardexport.go:1525-1540` | `AncientGreekSourceNotice` names OGA and is attached to **every** `grc` deck. A reader deck would wrongly carry OGA's attribution. See section 3. |
| Withheld-translation schema | migration `000041` (`reference_translations`) | Primary key `(owner_id, language, citation)`. Not scoped to a Work or Reading unit. See section 3. |

## 2. Minimum a reader adapter must supply

For a reader to be registered as one Classical Book, the adapter needs:

1. **A `ReadingUnitDefinition` value** (compiled, reviewed data, `internal/domain/classical.go:65-88`):
   - `Key`: unique across all definitions. `works.definition_key`,
     `editions.definition_key` and `reading_units.definition_key` are each
     `UNIQUE` (`migrations/000035_classical_reading_units.up.sql`). A reader's
     key must not collide with `xenophon-anabasis-1`.
   - `Language`: `grc`.
   - `WorkTitle` and `WorkAuthor`: the reader's title and the **reader's** author
     or editor. Per the map's note, the Work is the reader itself, not the
     adapted author. These become the Book's title and author
     (`internal/persistence/classical.go`, `registerReadingUnitBookTx`).
   - `EditionLabel`: the editor and year the learner sees, for example
     `"<editor> <year>"`. Use the source's own imprint; do not guess one.
   - `CitationStart`, `CitationEnd`: an inclusive dotted numeric range. See
     section 2.1.
   - `SourceFileName`, `SourceFileSize`, `SourceFileSHA256`: the pinned artifact.
     The SHA-256 is 64 lowercase hex characters.
   - `InterpretationVersion`: a new string for the adapter's mapping recipe, for
     example `"<reader>-<dialect>/1"`. A new value produces a new analysis run.

2. **A parser** that turns the pinned file into the adapter's intermediate form
   (sentences of tokens with surface, lemma, UPOS, morphology, citation). It must
   refuse malformed input with a stable code and never print source text, as the
   OGA adapter does (`import.go:17-27`, `ogasrc.go:38-42`).

3. **A token mapping** from the source's tags to UPOS and UD FEATS, with every
   unmapped value counted (`FeatureValueCount`, `import.go:98-104`). The current
   mapping is AGDT-specific; the reader's mapping is its own.

4. **Reading units built from the reader's divisions.** The reader's lessons or
   stories are **citations inside one Reading unit**, not separate Reading
   units. Write them as numeric dotted citations, for example
   `<lesson>.<section>` or `<lesson>.<story>.<line>`. `parseCitation` accepts any
   number of integer components (`ogasrc.go:660-671`), but rejects anything else,
   so a reader's `§12a` or `Lesson XII bis` must be normalised to integers. The
   range is then `CitationStart` to `CitationEnd`.

5. **Provenance.** The definition's pin and interpretation version are recorded
   on the run and in the source material (`importedSourceMaterial`,
   `analysis/greek_import.go:131-141`). Nothing else is needed for provenance.

6. **Licence and attribution.** This is the gap. The pipeline has no licence
   field. The only record is `AncientGreekSourceNotice`, a hard-coded OGA string
   (`internal/dictionary/attribution.go:8-11`). The NC rule is currently applied
   by operator review, not in code. A reader adapter should carry its own notice
   and licence in the definition, and the export notice should come from the
   Book's source rather than from the language. See section 3.

### 2.1 Citation shape

- Dotted numeric only. `compareCitation` treats a missing trailing component as
  zero (`ogasrc.go:675-693`), so `1.2` sorts before `1.2.1`. That is acceptable.
- A sentence whose tokens lie partly inside and partly outside the range stops
  the import (`citation_straddle`, `ogasrc.go:364-385`). A reader's sentence
  that crosses a lesson boundary is inside the range if both lessons are in the
  range, so this only matters at the two ends of the Reading unit.
- Sentence `Citations` are the sorted union of token citations
  (`ogasrc.go:635-657`). A reader with one citation per sentence gives one
  entry. This is what passage context keys on (section 3).

## 3. Cards and passage context without an aligned translation

**Without a translation, the card has no English passage and no error.**

- `applyPassageContext` returns early when a deck has no reference translations
  (`internal/cardexport/presentation.go:313-322`).
- `resolvePassage` returns empty values when the pin is empty, the sentence has
  no citations, or **any** cited section lacks a translation
  (`internal/cardexport/passage.go:29-43`). It never renders a partial passage.
- The provider field `passage_context` is `omitempty`
  (`internal/enrichment/llm_codec.go:255`), so an empty passage is left out of
  the payload.
- `ADR 0101` already requires Greek cards to work with the provider stopped.

So a reader with **no English** gives cards built from the sentence, the LSJ
outline and the Greek evidence, and nothing else. That is the pilot's behaviour.

**With English, four things break for a second `grc` Book:**

1. **Translations are per owner and language, not per Book or Work.**
   `reference_translations` has primary key `(owner_id, language, citation)`
   (migration `000041`). `ListReferenceTranslations` filters on owner and
   language only (`sqlc/queries/reference_translations.sql`). Every `grc` deck
   of an owner sees every imported translation.

2. **Citations collide.** A reader that uses `1.2.17`-style citations would be
   matched against Brownson's `1.2.17`. The sentence would then show Brownson's
   English labelled `Xen. Anab. 1.2.17`. `PutReferenceTranslations` upserts with
   `ON CONFLICT ... DO UPDATE` (`sqlc/queries/reference_translations.sql`), so a
   reader's import with the same citation overwrites Anabasis text, and the
   reverse.

3. **Pins are not checked within a language.** `mergedReferenceTranslations`
   rejects two pins only across languages (`internal/persistence/reference_translations.go:91-112`).
   Within `grc`, `listReferenceTranslations` keeps the first row's pin
   (`reference_translations.go:64-84`). With two sources in one language the
   frozen `BrownsonPin` could name the wrong file.

4. **Labels and notices are Brownson-specific.** `passageEditionLabel` and
   `passageWorkLabel` are constants (`passage.go:13-16`). `BrownsonSourceNotice`
   names Xenophon. `AncientGreekSourceNotice` names OGA and is attached to every
   `grc` deck (`cardexport.go:1525-1540`). Any reader with English needs labels
   and notices taken from the translation's own source.

**Recommendation:** for the pilot, ship no English. This avoids all four issues
and needs no migration. Before a reader with English is imported, scope
`reference_translations` by the Reading unit or source (the definition key),
and take the passage label and notice from that source. This is a schema change
under `migrations/` and CODEOWNERS (human review), and per ADR 0110 and
`CONTEXT.md` it needs an ADR first.

## 4. Concordance, coverage and prepared-deck surfaces

- **Concordance** joins `corpus_tokens` on owner and language
  (`sqlc/queries/concordance.sql:113`), so a second `grc` Book's tokens take part
  without change. `GetConcordancePriorityBook` takes one priority Book from the
  learner's goal (`concordance.sql:151-160`). That is not per Work. ADR 0110's
  "first within the author's works, then across the language" is therefore
  **not** implemented as such; it would need a Work-author match, not verified
  in code here.
- **Coverage and Browse** are per Book and per owner through
  `vocabulary_browse_counts` and the Browse projection (`ADR 0099`). `CONTEXT.md`
  says distinct editions and overlapping Reading units contribute separately.
  Per-reader coverage therefore already works. A cross-Book difficulty ladder is
  not built and is listed as "not yet specified" on the map.
- **Prepared decks** freeze their inputs (`ADR 0101`). A second Book's decks
  freeze the same way. The only shared input that changes meaning is the
  translation table in section 3.
- **Legacy-row refusal** passes for a learner who already has a registered
  `grc` Book (`classical.sql`, `ListLegacyLanguageRows`, `registered_owners`).
  A learner's first `grc` registration is refused if they have `grc` rows from
  some other path. Nothing in this survey suggests such rows exist.

## 5. Schema and migration implications

| Change | Needed for a pilot (no English)? | Needed for a reader with English? | Review |
| --- | --- | --- | --- |
| Register a second Work, Edition, Reading unit, Book | No. Existing tables and unique keys suffice. | No | None |
| Scope `reference_translations` by Reading unit or source | No | **Yes**, before any import with English | Migration under `migrations/` (CODEOWNERS), ADR first |
| New media type for a non-CoNLL-U artifact | No, if the artifact is CoNLL-U-shaped | Only if the pinned file is not CoNLL-U | Persistence switch (`sqlc.go:312`), not a migration |
| Licence / notice field on the definition | No | No | Code only (`ReadingUnitDefinition`); no schema |
| Cross-Book difficulty ladder | Out of scope here | Out of scope here | Map says not yet specified |

No migration is needed for the pilot.

## 6. Recommendation

1. **Pilot shape:** one reader = one Classical Book = one Reading unit. The
   reader is the Work. Lessons or stories are citations inside that unit, as
   integers. Do not make one Book per lesson; that is the `book.chapter.section`
   shape ADR 0110 describes, and it would multiply Books without a learner-facing
   reason.
2. **Adapter:** a new reader adapter alongside `oga_src`. Add a parser seam to
   `Import` (`import.go:164`), keep the writer and the run identity, and give
   the definition its own `InterpretationVersion`. Do not reuse the AGDT mapping.
3. **Registration:** add a definition selector to `cmd/greekregister`. Keep the
   Anabasis path unchanged.
4. **No English in the first cut.** Passage context is then withheld, which the
   code already handles. The Anabasis notice must not be attached to a reader
   deck; with no translation the source notice still comes from
   `deckIsAncientGreek`, so that line needs a per-Book source notice before the
   pilot ships.
5. **Licence:** record the reader's licence and notice in the definition, and
   confirm not NC before pinning (standing rule). Exocortex's CC BY-NC 4.0
   digitisations are discovery only.
6. **Schema:** none for the pilot. Scope `reference_translations` by source
   before any reader with English, via an ADR and human-reviewed migration.

**Open decisions for a human:**

- ADR 0110 says a Classical Book is divided by "conventional citations (book,
  chapter, section)". A reader's lesson structure is not one of those. Either
  amend ADR 0110 or treat the reader's lesson numbering as its citation scheme.
  This is a product decision, not a code one.
- Whether the reader's own English running vocabulary and notes (map: "not yet
  specified") count as translation for passage context, and therefore whether
  section 3's migration is needed at all.

## Not verified

- Nothing was run: no tests, no database, no adapter prototype.
- The concordance "author's works first" ordering is not confirmed in code.
- Whether any real reader's citation structure fits the dotted numeric scheme
  is unknown until a reader is chosen (other map tickets).
- The licence of any candidate reader is out of scope for this survey and is
  covered by other tickets under the NC rule.

# Dictionary gloss and morphology enrichment (Wiktextract/Kaikki)

Status: **Implemented** · Extended by ADR 0068 (pronunciation, principal parts)
and ADR 0069 (LLM sense selection and fallback gloss) · Date: 2026-09-14

This describes the current dictionary provider and card meaning path. The
accepted, not-yet-implemented [contextual gloss preparation](contextual-gloss-preparation.md)
uses dictionary material as evidence rather than the final gloss text.

## Motivation

Today a card's English content exists only when an operator configures an
external LLM *and* the learner consents (ADR 0007); otherwise `English` and
`EnglishSentence` export empty, and the cached `gloss` is never rendered on any
card. The definite article is derived from morphology heuristics
(`nounArticle`), and plural/forms are absent. ADR 0007 explicitly deferred a
built-in dictionary provider; the `DictionaryLookup` seam exists
(`internal/enrichment/enrichment.go`) with no production implementation.

Open dictionary data solves this: Wiktionary extracted by Wiktextract and
published as Kaikki.org JSONL provides concise English glosses, senses with
tags/topics/examples, inflection forms, and IPA for German, Italian, and Modern Greek,
regenerated from Wiktionary dumps roughly weekly. This makes card glosses and
morphology deterministic, local, and consent-free, without an LLM.

## Goal

A built-in dictionary enrichment provider: a local lexical lookup over a
build-time-derived read-only SQLite index (the **dictionary index**) that
supplies per-candidate (i) a compact, context-ordered set of English glosses
and (ii) definitive morphology (gender, article, plural), resolved when the
prepared-deck manifest is frozen and carried on the frozen entries. German and
Italian symmetric. The external LLM remains an optional upgrade for contextual
sentence translation only.

## Scope

- **Index as a build artifact**: a `make` target using the Python standard
  library downloader
  derives a compact per-language SQLite file from the raw Wiktextract JSONL
  (enwiktionary `de`/`it`/`el` entries — English glosses), capturing the dump and
  extraction date as `provider_version`. Not the deprecated per-language
  downloads; not a Postgres import. See the [operator refresh instructions](../../README.md#refreshing-the-dictionary-index).
- **In-process Go provider**: the index is read at startup via pure-Go
  `modernc.org/sqlite` (no CGO); a local lexical-provider implementation
  resolves glosses and morphology from it. No new gRPC service; the NLP service
  is untouched (ADR 0023 scope unchanged).
- **Gloss rendering**: a compact ordered sense set — top-N senses (default
  three), each gloss truncated to a token budget (≤ 10 tokens), joined with
  `·`. The set is the floor; context ranking only reorders display.
- **Sense ordering without an LLM**: a deterministic context score (simplified
  Lesk): overlap between the sentence context (content words ±5 around the
  bolded target, plus its dependency relations) and each sense's context words
  (gloss tokens, example-sentence tokens, topic/category labels). Fixed-phrase
  senses ("zu Hause", "a casa") get a large boost when the chunk appears.
  Tie or low score → Wiktionary primary sense (sense #1).
- **Morphology**: gender/article (der/die/das; il/lo/la; ο/η/το) and plural
  from the ranked top sense's forms; replaces the morphology heuristic.
- **Pronunciation**: the entry's IPA, normalized in the derivation step toward a
  standard phonemic `/…/` form (optional segments and regional variants
  discarded), surfaced as a dedicated `IPA` note field that self-suppresses when
  absent. See [ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).
- **Principal parts**: a German verb's third-person singular present, preterite, and
  past participle (`geht · ging · gegangen`) from the Wiktextract headword
  summary, surfaced as a dedicated `PrincipalParts` note field. Greek verbs do
  not use the Germanic extractor.
  Self-suppressing, since form coverage is partial. See
  [ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).
- **Card contract**: the `Gloss` note field is the card's single meaning block;
  `English` is retained but no longer rendered, and `EnglishSentence` remains the
  contextual sentence translation (ADR 0021 distinction preserved). See
  [ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).
- **Freeze and provenance**: gloss and morphology are resolved when the
  prepared-deck manifest is frozen and carried on the frozen entries; the index
  `provider_version` is recorded for provenance. The dictionary is local and
  deterministic, so it does **not** use the consent-gated external
  `enrichment_cache`; the immutable prepared `.apkg` provides reproducibility.

## Non-goals

- Frequency data — absent from Kaikki; ADR 0018 removed global frequency.
- Native-language definitions (de/it *edition* extracts) — a separate later
  phase.
- Audio on cards — the index carries no media. IPA pronunciation is now in scope
  ([ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md)).
- LLM sense selection and fallback gloss — a separate feature
  ([llm-sense-selection.md](llm-sense-selection.md), ADR 0069).
- Example-sentence backfill for lemmas whose best sentence fails the quality
  gate.
- A dictionary gRPC service, or Postgres storage of raw dictionary data (ADR
  0018 precedent).
- Changing representative-sentence selection or the recognition front.

## Requirements

### Index derivation

- Derived from the raw Wiktextract JSONL (enwiktionary `de`/`it`/`el`, `lang_code`
  filter), not the deprecated post-processed per-language downloads.
- Regenerated by a `make` target from the weekly Kaikki dumps; the build
  records the dump date, extraction date, and wiktextract commit as
  `provider_version`.
- Read-only SQLite, mmap-friendly, loadable at startup; lookups keyed by
  normalized lemma (Unicode lowercase + German post-1996 canonicalization,
  matching the vocabulary identity).
- License: CC BY-SA 3.0 / GFDL dual; attribution notice retained with the
  index.

### Gloss

- Default rendering is a compact ordered sense set (top-N ≤ 3, ≤ 10 tokens per
  gloss, `·`-joined); the set is never empty when the index has the lemma.
- Ordering is the deterministic context score above; primary-sense fallback on
  tie/low score; stable across repeated exports.
- When external translation is enabled, the context-aware LLM may reselect and
  reorder the display over a frozen candidate sense set, or supply a fallback
  gloss; the dictionary remains the consent-free default. See
  [llm-sense-selection.md](llm-sense-selection.md) and
  [ADR 0069](../adr/0069-llm-sense-selection-and-fallback-gloss.md).

### Morphology

- Gender/article and plural for nouns resolved from the ranked top sense's
  forms; the `nounArticle` heuristic is replaced for indexed lemmas.
- The article is a genuine nominative definite article (`der`/`die`/`das`;
  `il`/`lo`/`la`/`l'`; `ο`/`η`/`το`) or empty: `article_for` must not accept a form that is
  also tagged plural or otherwise inflected, and a render-time whitelist in
  `nounArticle` ignores any explicit `Article` value outside the set and falls
  back to gender. See [ADR 0067](../adr/0067-recognition-card-morphology-presentation.md).
- Unknown-in-index lemmas fall back to the existing behavior.

### Coverage

- Deck freeze emits a structured `gloss_coverage` log line: per language and
  POS, the count of selected lemmas with and without a resolved gloss. It drives
  the decision to broaden dictionary sources; the consent-gated LLM fallback is
  specified separately in [llm-sense-selection.md](llm-sense-selection.md)
  (ADR 0069), where fallback usage is also counted.

### Card contract

- The generated Anki/TSV note gains a `Gloss` field on the back between `POS`
  and `English`; artifact fixtures and completeness tests are updated.
- The generated note also gains a `Plural` field immediately after `Lemma`,
  rendered by the template as `Article Lemma (Pl. …)`. The plural is removed
  from `Gloss`; it renders whenever the index supplies a non-empty plural,
  including when it equals the lemma. See
  [ADR 0067](../adr/0067-recognition-card-morphology-presentation.md).
- The generated note gains an `IPA` field and a `PrincipalParts` field after
  `Plural`; the resulting field order is `Identity`, `Text`, `Article`, `Lemma`,
  `Plural`, `IPA`, `PrincipalParts`, `POS`, `Gloss`, `English`,
  `EnglishSentence`, `BookTitle`. See
  [ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).
- `English` is retained in the contract but is no longer rendered on the card;
  `EnglishSentence` keeps its contextual-translation semantics.
- TSV artifacts retain the dictionary attribution as an Anki-compatible comment
  when dictionary data is present.

### Freeze and provenance

- Resolved at manifest freeze and frozen onto the entries; the index
  `provider_version` (dump/extraction date) is recorded.
- The dictionary does not use the consent-gated external `enrichment_cache`,
  which stays reserved for the external translation path.

## Acceptance criteria

- [x] A lemma with no LLM configured exports a populated `Gloss` field,
      deterministically, from the index
- [x] The gloss is a compact ordered sense set; the set is never empty for an
      indexed lemma
- [x] Sense ordering is deterministic and stable across repeated exports
- [x] A fixed-phrase sense ranks first when the sentence contains the phrase
      ("zu Hause", "a casa")
- [x] German, Italian, and Modern Greek nouns resolve gender/article/plural from
      the index; unindexed lemmas keep the current heuristic
- [x] Gloss and morphology are frozen into the prepared-deck manifest with the
      index `provider_version` recorded; the external cache is not used
- [x] No dictionary data in Postgres; no new service; NLP service unchanged
- [x] Attribution notice retained per CC BY-SA / GFDL
- [x] A form tagged plural is never accepted as a noun's article, and an
      explicit non-article `Article` value falls back to gender
- [x] German and Italian nouns render their dictionary plural beside the lemma
      in a dedicated `Plural` field, not inside `Gloss`
- [x] Deck freeze emits a per-language/POS `gloss_coverage` summary
- [x] IPA is normalized in the derivation and rendered as a dedicated `IPA`
      field; an absent IPA leaves the card valid
- [x] German verbs render principal parts as a dedicated `PrincipalParts` field;
      Greek verbs do not use the Germanic extractor, and an absent value leaves
      the card valid
- [x] The card back renders one meaning block; the `English` field is retained
      but not rendered

## References

- [ADR 0064: Built-in dictionary enrichment provider](../adr/0064-dictionary-enrichment-provider.md)
- [ADR 0007: Enrichment providers, caching, and privacy](../adr/0007-enrichment-providers-caching-privacy.md)
- [ADR 0018: Remove global frequency dataset (DWDS) import](../adr/0018-remove-dwds-frequency.md)
- [ADR 0021: Contextual sentence translation cache](../adr/0021-contextual-translation-cache.md)
- [ADR 0029: Recognition-card sentence presentation](../adr/0029-recognition-card-sentence-presentation.md)
- [Recognition-card sentence presentation](recognition-card-sentence-presentation.md)
- [Kaikki.org](https://kaikki.org/) — Wiktextract extraction of Wiktionary, JSONL dumps
- [Wiktextract](https://github.com/tatuylonen/wiktextract) — the extractor

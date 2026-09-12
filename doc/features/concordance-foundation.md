# Concordance Foundation

Status: Proposed · Date: 2026-09-12

## Motivation

A future KWIC (key-word-in-context) concordancer — at the scope of a Book or of
a study language's analyzed library — needs per-occurrence context that the
analysis pipeline currently discards. The NLP boundary produces the full
normalized corpus (sentences plus tokens, each with surface, lemma, POS,
morphology, named entity, and character offsets), but after analysis only lemma
aggregates, corpus statistics, and a filtered content-word subset with embedded
sentence refs survive ([ADR 0001](adr/0001-go-core-python-nlp-service.md) and
[ADR 0003](adr/0003-postgresql-persistence.md) already name concordance as the
future growth path). The data must be captured at analysis time or a future
concordancer cannot exist without re-running NLP on demand.

This feature builds the foundation only: persist the normalized corpus so a
concordancer is possible later. No learner-facing concordance surface is built
here.

## Goal

Persist the normalized corpus (sentences and tokens, full fidelity, with
offsets) in owner-scoped relational tables during analysis, and provide a Go
query layer that returns occurrence rows for lemma and surface queries at both
Book and study-language scope. The foundation is rolled forward: the database
is dropped rather than backfilled, so concordance data exists only for analyses
run after this ships.

## Scope

- New tables `corpus_sentences` and `corpus_tokens`, written in the analysis
  worker's existing transaction.
- A Go query layer returning occurrence rows for a queried identity.
- Exact-match lookup by canonical lemma + UPOS and by surface form.
- Unit-relative offsets with structural provenance derived at query time.

## Non-goals

- Any learner-facing concordance or KWIC view, route, or UI.
- Context-window derivation or a fixed window width; the token stream is
  persisted so windowing stays a future rendering concern.
- Prefix, fuzzy, or full-text search (e.g. trigram, `tsvector`).
- Backfilling existing analyses (the app is not in production; the database is
  dropped and analysis rolls forward).
- Changing the selection pipeline or retiring
  `selection_candidates.eligible_sentence_refs` / `example_sentences`; the new
  tables are purely additive.
- Changes to the NLP protobuf, gRPC boundary, or `SourceLocation.chapter` /
  `section` population.

## Requirements

### Data model

- `corpus_sentences` holds one row per emitted sentence per analysis run:
  corpus reference, unit id, sentence ordinal, sentence text (as produced by
  the analyzer), and unit-relative start/end offsets. Every emitted sentence is
  stored, including sentences with zero tokens, so sentence counts remain
  consistent with `corpora`.
- `corpus_tokens` holds one row per token: corpus reference, sentence
  reference, token ordinal, surface, raw lemma, canonical lemma, UPOS,
  morphology JSONB, named entity (nullable), and unit-relative start/end
  offsets. `owner_id` and `language` are denormalized onto tokens so corpus-
  level queries are single-table scans.
- Offsets are unit-relative Unicode code points, matching the analyzer
  contract. Book-absolute spans and chapter labels are derived by joining
  `source_material_units` at query time; nothing is denormalized.
- Indexes support exact lookup by `(owner_id, language, canonical_lemma, upos)`
  and by `(owner_id, language, surface)`.

### Write path

- Sentences and tokens are written in the same transaction as the existing
  analysis artifacts, lemmas, and corpus statistics, in batched inserts,
  committed atomically. The merged corpus already exists in memory at that
  point; no separate job re-derives it.
- The tables are keyed by analysis run and retained as immutable history like
  `analysis_runs`; the corpus-level view reads only current analyses through
  `book_current_analyses`.

### Query layer

- A Go package exposes occurrence queries by canonical lemma + UPOS and by
  surface form, at Book scope (one current analysis) and study-language scope
  (every analyzed Book in the language, per the active study language).
- Each occurrence row returns: sentence text, target token offsets
  (sentence- and unit-relative), book + unit + chapter identity, and book-order
  position.
- The query layer does not compute context windows; it returns the sentence
  text and target offsets so any rendering style is possible later.

## Acceptance criteria

- A completed analysis persists one `corpus_sentences` row per emitted sentence
  and one `corpus_tokens` row per emitted token, including function words,
  named entities, and zero-token sentences.
- Occurrence queries by canonical lemma + UPOS and by surface return
  deterministic, owner-scoped results at Book and study-language scope with
  sentence text, target offsets, and structural provenance.
- Analyses run before this feature shipped have no concordance data (roll
  forward; the database is dropped).
- `selection_candidates` and `example_sentences` are unchanged.

## References

- [ADR 0059: Persisted normalized corpus for future concordance](../adr/0059-persisted-normalized-corpus-for-concordance.md)
- [ADR 0001: Go core with Python as an ingest-time NLP producer](../adr/0001-go-core-python-nlp-service.md)
- [ADR 0003: PostgreSQL as the initial persistence backend](../adr/0003-postgresql-persistence.md)
- [ADR 0009: Home-lab authentication and corpus-artifact isolation](../adr/0009-home-lab-auth-corpus-isolation.md)
- [ADR 0013: Size-based NLP analysis chunking](../adr/0013-size-based-nlp-chunking.md)
- [ADR 0026: Explainable structural text profile](../adr/0026-structural-text-profile.md)
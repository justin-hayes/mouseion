# ADR 0059: Persisted normalized corpus for future concordance

Status: **Accepted** · Date: 2026-09-12

## Context

A future concordance feature needs every occurrence of a queried word with its
surrounding context, at the scope of a Book or of a study language's analyzed
library. The analysis pipeline already produces this data — the typed
sentence/token contract with per-token surface, lemma, POS, morphology, named
entity, and character offsets — but the persisted artifact deliberately retains
only lemma aggregates, corpus statistics, and a filtered candidate subset
([ADR 0026](0026-structural-text-profile.md)). Every other token is discarded
once analysis commits, so a concordancer today would require re-running NLP on
demand.

## Decision

Persist the normalized corpus at analysis time in two owner-scoped relational
tables, `corpus_sentences` and `corpus_tokens`, written in the analysis
worker's existing transaction alongside the artifacts, lemmas, and corpus
statistics, with batched inserts and an atomic commit. Keyed by analysis run,
retained as immutable history like `analysis_runs`; the corpus-level view reads
only current analyses through `book_current_analyses`.

- Every emitted sentence row is stored, including zero-token sentences.
- Full token fidelity: surface, raw and canonical lemma, UPOS, morphology,
  named entity, and unit-relative code-point offsets.
- `owner_id` and `language` are denormalized onto tokens; exact-lookup indexes
  cover `(owner_id, language, canonical_lemma, upos)` and
  `(owner_id, language, surface)`.
- Book-absolute spans and chapter labels are derived from
  `source_material_units` at query time, not denormalized.
- The foundation rolls forward: existing analyses are not backfilled; the
  database is dropped.
- `selection_candidates` and `example_sentences` are untouched; the index is
  purely additive, so a later pass can derive sentence selection from it.
- No NLP protobuf or gRPC change is required.

## Consequences

- A future concordancer can query occurrence rows without re-running NLP.
- Analysis commits become larger transactions with high row counts; inserts are
  batched to bound statement size.
- Books analyzed before this ships have no concordance data until a content
  revision re-analyzes them naturally (roll-forward).
- Source text and sentence persistence remain owner-scoped per
  [ADR 0009](0009-home-lab-auth-corpus-isolation.md); a concordance surface is
  per-owner private data.

## Alternatives considered

- **Minimal occurrence index only.** Rejected: loses sentence adjacency and
  arbitrary surface queries, and reproduces the selection pipeline's blind spot
  for function words.
- **Re-run NLP on demand.** Rejected: couples a future UI to a whole-book Stanza
  pass per query and forgoes the already-paid-for analysis write point.
- **Re-run the full analysis pipeline for backfill.** Rejected: churns the
  analysis lifecycle for data that belongs in the new tables; analysis identity
  dedupe (ADR 0050 schema) blocks identical reruns. The app is not in
  production, so the database is dropped instead.
- **Store all normalized sentences.** Rejected by ADR 0026 as broader than the
  structural profile required; this decision reverses that stance for the
  purpose of enabling concordance.

## References

- [Concordance Foundation feature](../features/concordance-foundation.md)
- [ADR 0001: Go core with Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md)
- [ADR 0003: PostgreSQL as the initial persistence backend](0003-postgresql-persistence.md)
- [ADR 0009: Home-lab authentication and corpus-artifact isolation](0009-home-lab-auth-corpus-isolation.md)
- [ADR 0013: Size-based NLP analysis chunking](0013-size-based-nlp-chunking.md)
- [ADR 0026: Explainable structural text profile](0026-structural-text-profile.md)

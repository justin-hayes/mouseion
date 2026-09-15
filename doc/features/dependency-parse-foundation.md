# Dependency Parse Foundation

Status: Implemented · Date: 2026-09-12 · Updated: 2026-09-15

## Motivation

The concordance foundation ([ADR 0059](adr/0059-persisted-normalized-corpus-for-concordance.md),
[concordance-foundation.md](concordance-foundation.md)) persists the normalized
sentence/token stream at analysis time so a future concordancer can query
occurrences without re-running NLP. That foundation stores no syntax: a token's
sentence is captured, but not the role the token plays in it.

Two consumers need per-token dependency structure:

- **Grammar-aware concordance.** The natural next step of the foundation is a
  concordance that answers "where does *laufen* appear as a subject?" or "what
  are the objects of *laufen*?" — questions that require each token's dependency
  relation and head.
- **Deterministic sentence-quality scoring.** The GDEX-style rubric for choosing
  representative example sentences needs a finite-verb-and-subject check and
  subtree/hypotaxis membership, both of which are computed from dependency
  structure. Persisting the structure means the scorer reads the persisted
  corpus instead of re-running NLP.

## Goal

Dependency parsing is an always-on analysis capability. Each token's basic
dependency relation and head is persisted at analysis time, and the concordance
query layer exposes dependency-aware queries. The persisted data also supports
the separate sentence-quality (GDEX) scorer.

## Scope

- NLP pipeline: add the `depparse` processor (always on, core like `pos`/`lemma`),
  advertise it in `supported_features`, and emit `dependency` + `head` per token.
- Boundary contract: extend proto `Token` with `dependency` and `head`;
  `schema_version` becomes `1.1.0` (additive minor).
- Schema: `corpus_tokens` gains `dependency text NOT NULL` and `head bigint NOT
  NULL`, plus an index on `(owner_id, language, dependency)`; this is part of the
  current-state baseline.
- Write path: extend the existing batched `corpus_tokens` insert.
- Analysis gating: the analysis worker fails fast when the NLP service does not
  advertise `depparse`.
- Query layer: the four existing occurrence queries carry dependency context for
  the target token; two dependency-aware query families are added (role filter
  and dependents), at both Book and study-language scope.

## Non-goals

- Any learner-facing concordance or KWIC surface.
- Enhanced (non-basic) dependencies.
- Collocation queries.
- Any sentence-quality (GDEX) scoring; this milestone only guarantees the data
  is sufficient for it.
- Changes to `selection_candidates`, `example_sentences`, or card export.
- Backfilling pre-depparse analyses (roll-forward; the database is dropped).

## Requirements

### Boundary contract

- proto `Token` gains two additive fields: `dependency` (the UD dependency
  relation label, e.g. `nsubj`, `obj`, `advcl`, `root`) and `head`.
- `head` is the head token's **0-based ordinal within the sentence**, matching
  the existing `token_ordinal` convention. The root token of a sentence is its
  own head (`head == its own ordinal`) with `dependency == "root"`.
- `schema_version` bumps to `1.1.0`; field numbers remain frozen per the
  contract comment on `NormalizedCorpus`.

### NLP pipeline

- Processors become `tokenize,pos,lemma,depparse`,
  unchanged in order; `depparse` requires `pos`, which is already core.
- The producer maps Stanza's 1-based `word.head` (`0` = root) to the 0-based
  convention above. Because tokens are already flattened from
  `sentence.tokens[i].words`, head references must be mapped through a Stanza
  word-id → flattened-ordinal table so multiword-token words (Italian `l'`,
  `dell'`) resolve correctly.
- `supported_features` advertises `depparse` in the base set.

### Data model

- `corpus_tokens` gains:
  - `dependency text NOT NULL`
  - `head bigint NOT NULL` (0-based token ordinal; self for root)
  - index `corpus_tokens_owner_language_dependency_idx`
    ON `corpus_tokens(owner_id, language, dependency)`.
- The invariant "head is a token ordinal in the same sentence" is documented,
  not a foreign key (`token_ordinal` is not unique outside the row key).

### Write path

- The batched insert in `persistNormalizedCorpus` writes `dependency` and `head`
  for every token in the same analysis transaction.

### Analysis gating

- Before invoking `Analyze`, the analysis worker requires the NLP service's
  `supported_features` for the target language to include `depparse`; otherwise
  the job fails with a clear misconfiguration error. This is distinct from
  outage degraded mode.

### Query layer

- The four existing queries (`ListBookOccurrencesByLemma`, `BySurface`,
  `ListStudyLanguageOccurrencesByLemma`, `BySurface`) each add the target
  token's dependency context: its relation, its head ordinal, and the head's
  surface form.
- **Role filter (A)**: occurrences of a queried identity restricted to
  `dependency = R` (e.g. every occurrence of *laufen* as `nsubj`), Book and
  study-language scope.
- **Dependents (B)**: given a governor identity (canonical lemma + UPOS) and a
  relation R, return the governor's dependents in that role with full sentence
  context (e.g. the `obj`s of *laufen*), Book and study-language scope.
- Result rows use the existing `ConcordanceOccurrence` shape; no context
  windows are computed (windowing remains a rendering concern).

## Acceptance criteria

- Every persisted token in a completed analysis has non-null `dependency` and
  `head`; sentence roots are self-headed with `dependency = "root"`.
- Analysis fails fast with a clear error when the NLP service lacks `depparse`.
- Role-filter and dependents queries return deterministic, owner-scoped rows at
  both Book and study-language scope.
- The four existing occurrence queries carry dependency context.
- **GDEX-readiness**: the persisted data is sufficient to compute, without
  re-running NLP, (i) finite-verb-and-subject presence per sentence and (ii)
  subtree/hypotaxis membership of a target token — the two dependency-based
  checks in a GDEX-style sentence scorer.
- `selection_candidates` and `example_sentences` are unchanged.

## References

- [ADR 0060: Persist dependency parses in the normalized corpus](../adr/0060-persist-dependency-parses.md)
- [ADR 0059: Persisted normalized corpus for future concordance](../adr/0059-persisted-normalized-corpus-for-concordance.md)
- [Concordance Foundation](concordance-foundation.md)
- [ADR 0001: Go core with Python as an ingest-time NLP producer](../adr/0001-go-core-python-nlp-service.md)
- [ADR 0011: gRPC as the Go↔Python transport for the NLP service](../adr/0011-grpc-go-python-transport.md)
- [ADR 0013: Size-based NLP analysis chunking](../adr/0013-size-based-nlp-chunking.md)

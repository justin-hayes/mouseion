# ADR 0060: Persist dependency parses in the normalized corpus

Status: **Accepted** · Date: 2026-09-12 · Author: Justin + opencode

## Context

ADR 0059 persisted the normalized sentence/token stream at analysis time so a
future concordancer could query occurrences without re-running NLP, and
explicitly required no NLP protobuf or gRPC change. That stream stores a token's
sentence but not the role the token plays in it.

Two consumers need per-token dependency structure. A grammar-aware concordance
must answer "where does *laufen* appear as a subject?" and "what are the
objects of *laufen*?". And deterministic sentence-quality scoring (a GDEX-style
rubric, the next milestone) needs a finite-verb-and-subject check and
subtree/hypotaxis membership — both computed from dependency structure. Without
persisting the structure at analysis time, either consumer re-runs NLP on
demand, which the foundation exists to avoid.

## Decision

Add dependency parsing to the NLP boundary and persist each token's basic
dependency relation and head, shipping before any sentence-quality scoring.

- **Always-on processor.** The NLP pipeline runs `tokenize,pos,lemma,depparse`
  (plus optional `ner`); `depparse` is advertised in `supported_features` and is
  required for analysis. The analysis worker fails fast when a language's
  capabilities lack it.
- **Boundary contract.** proto `Token` gains two additive fields: `dependency`
  (UD relation label) and `head` (0-based token ordinal within the sentence;
  root tokens are self-headed with `dependency = "root"`). `schema_version`
  becomes `1.1.0`; field numbers stay frozen.
- **Persistence.** `corpus_tokens` gains `dependency text NOT NULL` and
  `head bigint NOT NULL`, plus an index on `(owner_id, language, dependency)`,
  written in the analysis worker's existing transaction (migration 000070).
- **Basic relations only.** Enhanced dependencies are not produced or stored.
- **Roll-forward.** The database is dropped; NOT NULL is safe because only
  post-ship analyses exist.

This amends ADR 0059, whose decision stated that no NLP protobuf or gRPC change
is required.

## Consequences

- Analysis is more expensive (a parser per sentence) and artifacts carry
  syntax; this is paid once per immutable analysis in a background job.
- Content-addressed artifact hashes change, so post-ship analyses produce new
  artifact identities; old rows do not exist after the database is dropped.
- Analysis-supported languages must provide `depparse`; readiness and catalog
  sync (which walk only ready languages) naturally exclude languages without
  it.
- The concordance query layer can answer role-filtered and dependents queries,
  and the future sentence-quality milestone can compute its dependency-based
  checks entirely from persisted data.
- Multiword tokens (Italian `l'`, `dell'`) require mapping Stanza word ids to
  the flattened token-ordinal space when resolving `head`.

## Alternatives considered

- **Enhanced dependencies.** Rejected: the default Stanza `depparse` processor
  emits only HEAD/DEPREL, the GDEX rubric reads only basic relations, and a
  later additive migration covers a grammar-query use case that materialises.
- **Optional capability like NER.** Rejected: multiplies capability states for a
  knob no learner sees; both consumers simply assume dependency data exists.
- **Nullable columns tolerating pre-depparse analyses.** Rejected: the app is
  not in production, so roll-forward avoids permanent NULL guards in the query
  layer.

## References

- [Dependency Parse Foundation feature](../features/dependency-parse-foundation.md)
- [ADR 0059: Persisted normalized corpus for future concordance](0059-persisted-normalized-corpus-for-concordance.md)
- [ADR 0001: Go core with Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md)
- [ADR 0011: gRPC as the Go↔Python transport for the NLP service](0011-grpc-go-python-transport.md)
- [ADR 0013: Size-based NLP analysis chunking](0013-size-based-nlp-chunking.md)
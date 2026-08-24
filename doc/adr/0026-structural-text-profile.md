# ADR 0026: Explainable structural text profile

Status: **Accepted** · Date: 2026-08-24 · Author: Justin + Codex

## Context

An analyzed book needs structural signals alongside learner-specific lexical
coverage. The existing normalized-corpus contract already supplies ordered
sentences and tokens during ingestion, but the persisted artifact does not keep
the sentence stream. Any later calculation would therefore be irreproducible.
The product explicitly rejects an opaque overall difficulty score and an
unsupported CEFR claim.

## Decision

Compute and persist these aggregates while the normalized corpus is available:

- analyzer-provided sentence count;
- median normalized-token count per sentence;
- nearest-rank 90th-percentile normalized-token count per sentence;
- sentence count and rate with more than 35 normalized tokens;
- total normalized-token count; and
- analyzer-provided sentence count with zero tokens.

Sentence length includes every token in the normalized sentence, including
punctuation. For an even number of sentences, the median is the arithmetic mean
of the two middle lengths. Nearest-rank p90 selects sorted rank `ceil(0.90*n)`.
The long-sentence boundary is strictly greater than 35 tokens.

The UI presents these as separate descriptive values. It shows analyzable
tokens against total normalized tokens and emits concrete warnings for no
sentences, zero-token sentences, or normalized tokens with no vocabulary-
analyzable tokens. It does not combine the values into a score or map them to a
proficiency level.

No NLP protobuf change is required: the typed sentence/token contract already
contains the authoritative inputs. Legacy corpora retain NULL structural
aggregates because their sentence streams cannot be reconstructed; reanalysis
is required.

## Consequences

- Profiles are deterministic for a given normalized corpus and explainable in
  the book UI.
- Punctuation contributes to sentence length but remains excluded from the
  vocabulary-coverage denominator.
- Analysis-quality inputs are retained without guessing at a broad quality
  grade.
- Persisted aggregate columns add migration and ingestion responsibilities.

## Alternatives considered

- **Derive from selection candidates.** Rejected because candidates omit token
  classes and do not preserve all sentences.
- **Store all normalized sentences.** Rejected as broader than the profile
  requires and duplicative of source-derived data.
- **Use analyzable tokens for sentence length.** Rejected because structural
  length should not change with vocabulary-selection filters.
- **Publish a composite score or CEFR label.** Rejected because current signals
  do not support either claim.

## References

- [Analysis Insights feature](../features/analysis-insights.md)
- [ADR 0001: Go core with Python NLP producer](0001-go-core-python-nlp-service.md)
- [ADR 0025: Analysis coverage and threshold metric contract](0025-analysis-coverage-threshold-metrics.md)

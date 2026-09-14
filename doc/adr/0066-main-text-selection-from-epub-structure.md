# ADR 0066: Identify a Book's main text for analysis from declared EPUB structure

Status: **Accepted** · Date: 2026-09-14 · Author: Justin + opencode

Supersedes the "Always the complete scope" clause of
[ADR 0047](0047-acquisition-folded-into-analysis.md). Reverses the Phase 1
non-goal "automatic front/main/back-matter classification or recommendation" in
the EPUB analysis scope feature.

## Context

ADR 0047 made analysis always process every readable unit of the acquired EPUB,
after the classifier-led scope workflow was retired (ADR 0039). That was the
right correction to a classifier that could drop real chapters: with no reliable
signal, the safe default is everything.

Two things have changed. First, the extracted-unit snapshot already preserves the
EPUB 3 landmark `epub:type` tokens resolved to each unit, plus manifest
provenance, and nothing consumes them. Second, whole-book analysis pays for
ancillary units: a declared bibliography, index, or glossary inflates the
coverage denominator and recurring-vocabulary counts and spends analyzer time on
text the learner is not studying.

The signal to do better is **declared EPUB structure** rather than inference.
EPUB 3 landmarks declare a `bodymatter` start, and landmarks declare terminal
reference material. Trusting a declaration is categorically different from the
retired classifier's text heuristics, and it is available without re-parsing the
EPUB or changing the snapshot.

## Decision

Analysis identifies a Book's **main text** from declared EPUB 3 structure and
analyzes that, falling back to the whole snapshot whenever the structure is
absent or ambiguous.

- **Structure-driven only.** Exclude only units the EPUB declares ancillary;
  never infer from titles, text, length, or density.
- **Definition.** Main text is the whole-unit run from the single unit carrying a
  `bodymatter` landmark to the first subsequent unit carrying a
  back-matter-class landmark (`backmatter`, `bibliography`, `index`, `glossary`,
  `colophon`), or to the end when none is declared.
- **Whole-unit granularity.** The extractor already discards href fragments and
  excludes `linear="no"`, `nav`, non-XHTML, and empty resources, so decisions are
  made over whole readable units.
- **Fail-safe.** No `bodymatter`, more than one `bodymatter`, or an empty run
  means no identified main text and the whole snapshot is analyzed.
- **Derived, not persisted.** The selection is a pure function over the immutable
  snapshot's persisted provenance, computed at analysis time. No snapshot
  mutation, no new table or column.
- **Versioned identity.** When selection changes the analyzed set, the run's
  configuration identity is `selection-maintext-landmarks-v1`; when it does not,
  it stays `selection-default-v1`. The analyzer name and version are unchanged.
  This makes a selection change a distinct, re-runnable analysis without a
  content revision.
- **Provenance.** The applied selection is recorded in the run's existing
  processing-history detail (algorithm version, boundaries, excluded unit IDs,
  counts).
- **No learner-facing surface.** Selection is operations provenance, not a
  review or override.
- **Lazy rollout.** Existing analyses are not backfilled or invalidated; a Book
  adopts selection when it is next analyzed.

Deferred: EPUB 2 `guide` (it would extend the extraction contract and snapshot
provenance) and per-unit declared-ancillary exclusion without a `bodymatter`
boundary.

The behavior is specified in the
[main text selection feature](../features/main-text-selection.md).

## Consequences

- Coverage, thresholds, and recurring vocabulary reflect the main text, which is
  the intended quality improvement.
- False positives are treated as correctness bugs; the fail-safe default is the
  whole snapshot. A mislabelled `bodymatter` target remains the known, accepted
  failure mode: it can drop content before the boundary. This is deliberate,
  because `bodymatter` is a declaration rather than an inference.
- Coverage bases can differ across a library: books whose selection applied and
  books whose did not are not directly comparable on the raw denominator. This is
  documented rather than normalized.
- Prepared decks and immutable corpora remain bound to the completed analysis
  that produced them; a new analysis supersedes the current pointer through the
  existing ADR 0040 flow.
- ADR 0047's catalogue-derived and acquisition-folded decisions stand; only its
  "always the complete scope" clause is superseded.

## Alternatives considered

- **Persist the selection or flip the existing `selected` column.** Rejected:
  `source_material_units` is immutable by trigger, and deterministic
  recomputation makes persistence unnecessary.
- **Bump the configuration identity globally.** Rejected: it needlessly re-runs
  books that selection cannot help and churns prepared decks.
- **Eagerly backfill the library.** Rejected: a migration-enqueued re-analysis is
  a separate, deliberate operation; lazy adoption is sufficient for a quality
  improvement.
- **Require per-unit ancillary labels before dropping anything, trusting nothing
  positionally.** Rejected: front matter is frequently unlabelled, which would
  make the feature almost never fire; the `bodymatter` declaration is a
  sufficient boundary.
- **Revive the text/structural classifier.** Rejected: it was retired after false
  positives dropped real chapters.

## Related decisions and specifications

- [ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived](0047-acquisition-folded-into-analysis.md)
- [ADR 0039: Drop retired EPUB classifier schema](0039-drop-retired-epub-classifier-schema.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)
- [ADR 0049: Reading intent triggers analysis](0049-reading-intent-triggers-analysis.md)
- [ADR 0025: Analysis coverage and threshold metric contract](0025-analysis-coverage-threshold-metrics.md)
- [Main text selection feature](../features/main-text-selection.md)
- [Phase 1: Preserve EPUB analysis structure](../features/epub-analysis-scope.md)
- [Analysis Insights](../features/analysis-insights.md)

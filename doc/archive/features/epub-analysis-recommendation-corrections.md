# Historical: EPUB analysis recommendation corrections

Status: Superseded historical record · Implemented 2026-08-25 · Superseded 2026-09-01

> **Historical archive.** This document is retained for implementation history
> and is not part of the current product contract.

This document records the former Phase 5 effort to correct classifier
recommendations. It is retained as implementation history and rationale; it is
not a current product requirement.

## Historical problem

A German regression fixture exposed a mismatch between structural
classification and recommendation policy: substantive chapters could be
excluded, while unknown reference-like units were included by a whole-book
fallback. The former work separated category confidence from recommended
inclusion, added structural markers, and made the policy versioned and
explainable.

Historical rules distinguished explicit EPUB structure from generic text shape,
recognized German, Italian, and language-neutral editorial/reference markers,
and kept recommendations reviewable. The recorded UI showed category,
confidence, recommendation, reasons, fallback state, and selected-versus-all
character totals. These concepts describe the retired classifier workflow only.

The historical regression fixture recorded a 21-unit German reading order and a
1.4.0 classifier/policy outcome in which nine substantive chapters were the
only automatic selections. Existing classifier runs, reviewed scopes, and
analysis corpora remained immutable across policy versions.

## Compatibility

Historical recommendation and classification rows remain on disk and may be
referenced by old reviewed scopes or analysis history. They remain readable for
relationship integrity and audit, but current application behavior does not
interpret them, display them as evidence, or use them to choose a new scope.

The current behavior is defined by
[EPUB analysis scope review](epub-analysis-scope-review.md): project reliable
top-level EPUB 3 TOC entries into all-on checkboxes, or use the flat readable
persisted-unit fallback. It persists the canonical ordered selected-unit set
and uses no classifier or recommendation pipeline for new workflow behavior.

## Retired concepts

The following are historical only and must not be reintroduced as current
scope-review requirements: recommendation policy, recommendation/main-matter
presets, classifier confidence, classifier warnings, evidence reasons,
hierarchy groups, fallback inclusion policy, and prior-scope comparison.

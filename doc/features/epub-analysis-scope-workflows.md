# Historical: EPUB structure and reusable scope workflows

Status: Superseded historical record · Implemented 2026-08-25 · Superseded 2026-09-01

This document preserves the former Phase 4 workflow for repository history. It
is not a current product specification. New implementations must follow the
[canonical EPUB scope-review contract](epub-analysis-scope-review.md).

## Historical behavior

The former workflow refined a classifier-led scope page with derived hierarchy
groups, unit-level classification evidence, recommendation controls, and reuse
of prior selections. Groups were inferred from navigation labels, landmarks,
and resource directories; contradictory or non-contiguous groups fell back to
flat spine order. Group controls expanded to persisted unit IDs.

Learners could historically start from a recommendation, all readable units,
or a prior selection, compare added and removed units, inspect category,
confidence, ordered reasons, and warnings, and override individual units.
Those controls and concepts belonged to the retired classifier workflow.

The former server boundary remains useful: browser input was never authoritative
for text, ordering, ownership, or snapshot contents; the server reloaded
owner-scoped persisted units and rejected stale, foreign, duplicate, or empty
selections. Confirming a selection created immutable scope history without
starting analysis, and later analysis used selected persisted units in spine
order.

## Historical limitations and fixtures

The former hierarchy was a conservative projection rather than a complete EPUB
navigation editor. Directory-derived nesting could reflect publisher packaging,
and comparison was limited to one owner and one source snapshot. The Phase 4
fixtures covered grouping, partial selection, classifier refinement, scope
history, owner isolation, stale snapshots, legacy full-text results, and
accessible native controls.

## Current replacement

The current review has one presentation model: reliable top-level EPUB 3 TOC
entries, each rendered as an initially checked checkbox and expanded to the
persisted units in spine order. If a complete, non-overlapping TOC-to-unit
partition cannot be projected, the page renders one initially checked checkbox
per readable persisted unit in flat spine order. Nested TOC entries are never
controls. Check all, Uncheck all, the non-empty selection rule, explicit
confirmation, owner validation, CSRF protection, stale-snapshot rejection,
and separate analysis submission remain part of the current contract.

There are no current hierarchy groups, recommendation presets, prior-scope
comparison, classifier warnings, or confidence controls. Historical records
remain readable and are not rewritten.

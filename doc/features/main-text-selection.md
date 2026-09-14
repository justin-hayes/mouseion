# Main text selection for EPUB analysis

Status: Proposed · Date: 2026-09-14

## Problem

The whole-book analysis policy ([ADR 0047](../adr/0047-acquisition-folded-into-analysis.md))
sends every readable unit of the acquired EPUB to NLP. Ancillary units — declared
front matter and terminal reference material such as a bibliography, index, or
glossary — inflate the coverage denominator and recurring-vocabulary counts and
consume analyzer time without representing the work being read.

## Goal

Identify the Book's **main text** from declared EPUB structure so that analysis
processes the body of the work, while never dropping body text on a guess. When
structure cannot identify the main text, the whole snapshot is analyzed exactly
as today. The decision and its rationale are recorded in
[ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md).

## Non-goals

- A learned or title/text-similarity classifier. The retired
  `mouseion-epub-structure` classifier (ADR 0039) is not revived.
- Learner-facing selection, review, or override controls.
- Persisting a selection as part of the extracted-unit snapshot; the snapshot
  stays complete and immutable.
- EPUB 2 `guide` support in this version.
- Rewriting existing analyses or prepared decks (see Rollout).

## Structure inputs

Selection reads only provenance the extractor already persists on each readable
unit: the EPUB 3 landmark `epub:type` tokens resolved to the unit
(`landmark_types`). The extractor already excludes `linear="no"`, `nav`,
non-XHTML, and empty resources, and discards href fragments, so every decision
is made over whole readable units in spine order.

## Algorithm (`selection-maintext-landmarks-v1`)

- **Back-matter-class token**: one of `backmatter`, `bibliography`, `index`,
  `glossary`, `colophon`. `appendix`, `notes`, and `endnotes` are deliberately
  excluded; they are frequently desired learner content.
- **Body start `S`**: the single unit whose `landmark_types` contains
  `bodymatter`.
- **Back-matter start `E`**: the first unit at or after `S` whose
  `landmark_types` contains any back-matter-class token; if none, the end of the
  unit list.
- **Main text**: the units with order in `[S, E)`.
- **Ancillary text**: the units with order in `[0, S)` and `[E, len)`.

Token matching is case-insensitive.

## Fail-safe

If any of the following holds, there is no identified main text and every unit is
analyzed:

- no unit carries `bodymatter`; or
- more than one unit carries `bodymatter`; or
- the back-matter start is the body start (an empty main-text run).

Ambiguity is judged only over emitted units: a landmark whose target did not
survive extraction is not visible and is not by itself a trigger. Fragments are
discarded at extraction, so a boundary that pointed into a document is treated as
that whole document. This can include a document's leading ancillary text (a
false negative) but never drops a later body document (a false positive).

## Analysis identity

Selection is derived, not persisted: a pure function of the immutable snapshot
and the algorithm version, computed at analysis time. The algorithm version
travels in the analysis configuration identity:

- `selection-maintext-landmarks-v1` when main text is a strict subset of the
  snapshot;
- `selection-default-v1` when the result is a no-op (fail-safe, or no ancillary
  units), so those books stay byte-identical to the existing whole-book analysis
  and are not re-run.

The analyzer name and version are unchanged.

## Provenance

The run records the applied selection in its existing processing-history detail:
algorithm version, `S` and `E`, the excluded unit IDs, and the selected/total
unit counts. No new table or column is introduced.

## Coverage and presentation

Coverage, threshold, and unknown-vocabulary statistics are computed from the
analyzed main text, exactly as they are computed from the analyzed corpus today.
A selection that excludes units therefore changes the reported basis; that is
the point of the feature. The learner-facing text continues to describe the
metric without a separate scope-detail section.

## Rollout

Lazy and per-book. Existing completed analyses are not backfilled or
invalidated. A Book adopts main-text selection the next time it is analyzed (for
example after its content changes, or on a learner-triggered re-analysis); until
then it keeps its whole-book result. Coverage bases can therefore differ between
books that selection helped and books it did not. This is an accepted, documented
consequence, not normalized away.

## Verification

Fixtures and tests alongside the existing extraction fixtures:

- `epub3-main-text` provides a real imported EPUB with a linear front-matter
  unit before the `bodymatter` target and a terminal `bibliography` landmark,
  proving `[0, S)` and `[S, E)` are excluded from the analysis corpus while the
  extraction snapshot remains complete;
- no `bodymatter`, duplicate `bodymatter`, and a back-matter token on the
  body-start unit, each asserting the whole snapshot is analyzed.

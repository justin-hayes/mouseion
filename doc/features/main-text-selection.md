# Main text selection for EPUB analysis

Status: Implemented · Date: 2026-09-14

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
as today. A distributor's published packaging convention can also be
recognised: Project Gutenberg header, licence and post-licence text is excluded
from analysis ([ADR 0108](../adr/0108-distributor-text-excluded-from-analysis.md)).
The decision and its rationale are recorded in
[ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md) and
[ADR 0108](../adr/0108-distributor-text-excluded-from-analysis.md).

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
(`landmark_types`), the manifest id (`manifest_id`), and the unit text. The
extractor already excludes `linear="no"`, `nav`, non-XHTML, and empty resources,
and discards href fragments, so every decision is made over whole readable units
in spine order.

The landmark rules read only `landmark_types`. The Project Gutenberg rule reads
`manifest_id` and the fixed marker lines in unit text; it never infers from the
meaning of other text.

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

## Project Gutenberg Distributor text (`selection-distributor-gutenberg-v1`)

**Distributor text** is text a distributor wraps around the work, recognised
only from that distributor's published packaging convention. Project Gutenberg is
the only recognised convention.

A snapshot follows the convention only when all of these hold:

- exactly one readable unit has manifest id `pg-header`;
- exactly one `*** START OF THE PROJECT GUTENBERG EBOOK … ***` line occurs in
  the snapshot, and it is in the `pg-header` unit;
- exactly one `*** END OF THE PROJECT GUTENBERG EBOOK … ***` line occurs in the
  snapshot, after the START line, in the same unit or a later one.

The `pg-footer` manifest id is not required: the END line's unit is recognised by
its text, so a licence unit with a generic manifest id still counts.

When recognised, these are excluded:

- in the `pg-header` unit, everything up to the end of the START line;
- in the END line's unit, everything from the start of the END line;
- every readable unit after the END line's unit, whole.

Unit text is never altered. The header and END units are still submitted to the
analyzer unchanged, and every sentence whose span overlaps Distributor text is
dropped before tokens, lemmas, statistics, and the artifact are derived. A
sentence crossing a marker line is dropped. Source locations of retained
sentences are identical to a whole-book analysis.

When a `bodymatter` landmark also applies, Distributor text is removed from the
landmark-selected run. If recognition fails for any reason — no `pg-header`, a
missing or repeated marker, END before START, or START outside `pg-header` —
nothing is recognised and selection is exactly the landmark rule above.

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

- `selection-maintext-landmarks-v1` when landmark selection alone excludes
  units;
- `selection-distributor-gutenberg-v1` when recognised Distributor text alone
  excludes something;
- `selection-maintext-landmarks-distributor-gutenberg-v1` when both rules
  exclude something;
- `selection-default-v1` when the result is a no-op (fail-safe, no ancillary
  units, or no recognised Distributor text), so those books stay byte-identical
  to the existing whole-book analysis and are not re-run.

An existing Book is offered an **Analysis update** when the selection identity
derived from its unchanged snapshot differs from the published run's identity.
The text scan runs only for snapshots with a `pg-header` unit; all of ADR 0107's
confirmation, eligibility, and frozen-evidence rules still apply.

The analyzer name and version are unchanged.

## Provenance

The run records the applied selection in its existing processing-history detail:
algorithm version, `S` and `E`, the excluded unit IDs, and the selected/total
unit counts. For recognised Distributor text it also records the convention,
the trimmed offset ranges in the header and END units, and the whole-unit
exclusions. No new table or column is introduced.

## Coverage and presentation

Coverage, threshold, and unknown-vocabulary statistics are computed from the
analyzed main text, exactly as they are computed from the analyzed corpus today.
A selection that excludes units therefore changes the reported basis; that is
the point of the feature. The learner-facing text continues to describe the
metric without a separate scope-detail section.

## Rollout

Lazy and per-book. Existing completed analyses are not backfilled or
invalidated. A Book adopts main-text or Distributor selection when it is next
analyzed; a Book whose snapshot now derives a different selection identity is
offered a learner-confirmed Analysis update ([ADR 0107](../adr/0107-analysis-pipeline-identity-and-analysis-updates.md)).
Until it publishes, the Book keeps its earlier result. Coverage bases can
therefore differ between books that selection helped and books it did not. This
is an accepted, documented consequence, not normalized away.

## Verification

Fixtures and tests alongside the existing extraction fixtures:

- `epub3-main-text` provides a real imported EPUB with a linear front-matter
  unit before the `bodymatter` target and a terminal `bibliography` landmark,
  proving `[0, S)` and `[S, E)` are excluded from the analysis corpus while the
  extraction snapshot remains complete;
- `epub3-gutenberg` provides a synthetic Gutenberg-shaped EPUB (no real Gutenberg
  file) with a `pg-header` preamble and START line, book text in the header
  unit, a chapter, a `pg-footer` END line and licence, and a post-licence "back"
  page, proving only book sentences reach the corpus with unchanged offsets;
- no `bodymatter`, duplicate `bodymatter`, and a back-matter token on the
  body-start unit, each asserting the whole snapshot is analyzed;
- missing `pg-header`, repeated or missing markers, END before START, and START
  outside `pg-header`, each asserting the landmark fallback;
- a licence unit with a generic manifest id, and landmark plus Distributor
  exclusion combined.

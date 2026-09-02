# Historical: EPUB unit classification and recommendations

Status: Superseded historical record · Implemented 2026-08-25 · Superseded 2026-09-01

This document records the classifier pipeline that shipped during the earlier
EPUB scope phases. It is retained so existing classification rows, reviewed
scopes, analyses, and provenance remain intelligible. It is not a current
product requirement and must not guide new scope-review behavior.

## Historical purpose

The former Phase 2 pipeline assigned extracted units structural categories,
confidence, ordered reasons, and a recommended inclusion state. The categories
were front_matter, main_matter, back_matter, and unknown. A deterministic
classifier named mouseion-epub-structure, including the shipped 1.2.0 rule
set, produced versioned results and persisted them beside extracted units.

Historical signals included EPUB landmarks and epub:type, navigation labels,
unit titles, manifest/spine properties, paths, spine position, and text shape
such as citation density and sentence density. Versioned output preserved the
classifier name/version, category, confidence, ordered reason codes, and
recommended inclusion. These values described the former implementation only.

The classifier was deliberately explainable rather than a language model. It
recognized selected German, Italian, and language-neutral structural markers,
including contents, prefaces, chapters, bibliography, notes, indexes,
glossaries, appendices, captions, and repeated short edges. Strong conflicting
signals produced low-confidence unknown; non-linear and navigation-only
resources were excluded when supplied to the former pipeline.

## Historical behavior and fixtures

Recommendations were metadata, not an immediate analysis action. The former
import path classified a persisted extracted-unit snapshot and atomically
replaced the same classifier run for identical content. A changed source
created a new snapshot and removed classifications tied to the old snapshot.
Older classifier versions and immutable reviewed scopes retained their original
identities.

The historical fixtures covered EPUB 2/NCX and EPUB 3/landmark examples,
German and Italian labels, ordered reasons, fallback behavior, owner isolation,
legacy sources, and unchanged full-text compatibility input. The later
recommendation-correction work also recorded a regression in which substantive
chapter prose could be excluded while reference material was included; that
record explains why the policy was versioned and reviewable.

## Compatibility

The historical classifier tables and scope metadata were removed by migration
000043 after the classifier pipeline was retired. Existing reviewed scopes and
analyses remain readable through their active source, snapshot, and selected-
unit identities, but classifier identity, category, confidence, reasons, and
recommendation provenance are not restorable. Current application behavior
does not reinterpret or display those values as active selection guidance.

The current scope-review contract is defined by
[EPUB analysis scope review](epub-analysis-scope-review.md): reliable top-level
EPUB 3 TOC entries are all-on checklist choices, with a flat readable-unit
fallback. No new workflow classifies units or writes recommendations.

## Non-goals for current work

- do not add or evolve a classifier or recommendation policy;
- do not use historical categories, confidence, reasons, or recommendations to
  choose new scope rows; and
- do not rewrite or delete historical classification, scope, analysis, or
  source data.

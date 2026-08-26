# Phase 4: EPUB structure refinement and reusable scope workflows

Status: Implemented · Date: 2026-08-25

## Problem

Phase 3 lets learners review a flat ordered unit list and analyze a selected scope. EPUBs often have nested parts/chapters, repeated front/back matter, and ambiguous classifications. Learners also need to revisit or reuse scope decisions without rebuilding them manually.

## Goal

Make structure and scope decisions easier to understand and reuse while keeping classification deterministic, explainable, and user-overridable.

## Implemented behavior

### Hierarchy visualization and selection

- Mouseion derives nested part → chapter → section groups from repeated navigation labels, landmarks, and stable resource directories. Groups and members always follow persisted spine order.
- Contradictory or non-contiguous grouping evidence is discarded. When no reliable grouping remains, the review displays the flat deterministic spine order.
- Each group displays its unit count, character count, and token estimate. Include/exclude controls submit group IDs; the server reloads the current owner-scoped snapshot and expands those IDs to exact unit IDs.
- A group reports included, excluded, or partially selected state as individual checkboxes change. Native buttons, checkboxes, labels, fieldsets, legends, headings, and live status regions keep the workflow keyboard- and assistive-technology-accessible.

### Explainable classifier refinement

- The versioned deterministic classifier recognizes observed German, Italian, and language-neutral structural patterns, including editorial matter, captions, structural fragments, bibliography clusters, and repeated short headers or footers.
- Every result retains ordered reason codes and learner-facing explanations. Conflicting or structural-fragment evidence remains unknown/review-required instead of being silently promoted.
- Recommendations are only starting points. Learners can accept them, select main matter, include all readable units, exclude all units, or override any individual unit.
- A refined classifier run is stored under its classifier version. It does not rewrite the classifier identity, selected units, source snapshot, or reasons attached to historical scope/classification records.

### Reusable and historical scopes

- A learner can start from the current recommendation, all readable units, or a prior selection. Prior and proposed scopes are compared by stable unit ID and title with added/removed units and estimated sizes.
- Confirming creates or resolves an immutable reviewed-scope revision without starting analysis. Reusing the same snapshot and selection produces the same ordered future analysis input; changing the selection creates distinct scope history.
- A separate explicit analysis action reloads the saved scope and sends only its selected persisted units to the analyzer. Completed corpora link back to the exact reviewed scope and retain ordered selected-unit provenance.
- Historical result pages identify either the reviewed scope and selected units or the legacy/full-text behavior used by older corpora.

## Security boundaries

- Browser input is never authoritative for text, classification, ordering, ownership, or snapshot contents. It contains only CSRF tokens, the reviewed snapshot ID, and selected unit/group IDs.
- The server reloads the source, extracted units, classifications, groups, and prior scopes through the authenticated owner. Cross-owner source, unit, group, prior-scope, job, and corpus access is rejected.
- Scope confirmation rejects stale snapshot IDs, duplicate/foreign units, unknown/contradictory groups, and empty selections.
- Historical scopes and corpora are immutable. Reimporting an EPUB creates a new extracted-unit snapshot; stale scopes cannot be submitted against it, while completed historical results retain their original links.
- Legacy EPUBs without extracted units continue through full-text analysis and are labeled as legacy/full-text results.

## Current limitations

- Hierarchy is a conservative projection, not a complete EPUB navigation-tree editor. Single-unit groups, non-contiguous repeated labels, and contradictory overlaps fall back to the flat spine.
- Directory-derived nesting can reflect publisher packaging rather than semantic book structure, so learners must still review the included units.
- Group controls are server-submitted actions. Individual checkbox and preset summaries update in the browser, but the server remains the final validator.
- Comparison is limited to scopes for one owner and one source snapshot; there is no cross-book composition or fuzzy matching after a reimport changes unit identity.
- Classifier confidence is rule evidence, not a probability, reading-level estimate, or claim about linguistic difficulty.

## Non-goals

- No CEFR/difficulty claims;
- no automatic promotion of uncertain content;
- no cross-book scope composition;
- no Anki synchronization;
- no general-purpose EPUB editor.

## Future research candidates

- Evaluate more publisher layouts and languages with consented, provenance-bearing fixtures, especially ambiguous navigation and deeply nested sections.
- Measure whether reason wording and confidence bands help learners make consistent overrides without implying probabilistic accuracy.
- Explore a versioned mapping assistant for comparing reimported snapshots while keeping every proposed match reviewable and owner-scoped.
- Study richer accessible tree interaction only if it improves on the current native-control workflow across keyboard and screen-reader combinations.

Machine learning, ranking, or external corpus lookup remains out of scope. Any such work requires a separate architecture decision, an evaluation dataset, explicit privacy boundaries, and a migration plan that preserves historical decisions.

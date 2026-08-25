# Phase 4: EPUB structure refinement and reusable scope workflows

Status: Proposed · Date: 2026-08-25

## Problem

Phase 3 lets learners review a flat ordered unit list and analyze a selected scope. EPUBs often have nested parts/chapters, repeated front/back matter, and ambiguous classifications. Learners also need to revisit or reuse scope decisions without rebuilding them manually.

## Goal

Make structure and scope decisions easier to understand and reuse while keeping classification deterministic, explainable, and user-overridable.

## Workstreams

### Hierarchy visualization

- derive/display nested part → chapter → section relationships where EPUB navigation or heading structure supports them;
- retain a flat deterministic fallback;
- show aggregate unit counts/estimated tokens per group;
- allow group-level include/exclude controls with clear partial-selection state.

### Classifier refinement

- use Phase 2/3 user overrides as evaluation evidence, not silent training data;
- add versioned rules for observed German, Italian, and language-neutral patterns;
- improve confidence/reason explanations;
- detect repeated headers, footers, captions, bibliography clusters, and editorial blocks;
- preserve an unknown/review-required outcome when evidence conflicts.

### Reusable scope workflows

- clone a prior reviewed scope for reanalysis;
- compare current extraction/classification with a prior scope;
- provide presets such as recommended main matter, all readable units, and prior selection;
- preserve immutable historical scopes and corpus provenance;
- make scope changes explicit before queueing analysis.

## Safety and compatibility

- No automatic deletion or rewriting of historical corpora.
- No machine-learning classifier or external corpus lookup.
- Browser submits IDs/preset choices only; server reloads units.
- Owner isolation, CSRF, and deterministic ordering remain mandatory.
- Legacy flat/full-text sources remain readable.
- Every analysis displays the exact scope used.

## Non-goals

- No CEFR/difficulty claims;
- no automatic promotion of uncertain content;
- no cross-book scope composition;
- no Anki synchronization;
- no general-purpose EPUB editor.

## Handoff

Phase 4 should produce a more understandable scope workflow and an evidence-backed classifier backlog. Any future ML/ranking work requires a separate architecture decision and evaluation dataset.

# Phase 2: Classify EPUB units and recommend analysis scope

Status: Proposed · Date: 2026-08-25

## Problem

Phase 1 preserves ordered EPUB units, provenance, and metadata, but does not explain whether a unit is likely main reading content, front matter, reference material, or another structural category. The app therefore cannot make a transparent recommendation about which units should be prioritized for NLP.

## Goal

Add a deterministic, explainable classifier that assigns each extracted unit a category, confidence, reasons, and a recommended inclusion state. Phase 2 produces a recommendation; Phase 3 will let the learner review and override it before analysis.

## Categories

Initial categories:

- `front_matter` — title pages, copyright, contents, prefaces, acknowledgments.
- `main_matter` — introductions, parts, chapters, conclusions, epilogues, ordinary reading content.
- `back_matter` — notes, bibliography, references, index, glossary, appendices, further reading.
- `unknown` — insufficient or contradictory evidence.

Categories are heuristics, not claims about authorial intent.

## Classification signals

Use deterministic signals with versioned rules:

- EPUB navigation landmarks and `epub:type`;
- manifest/spine properties and `linear`;
- navigation labels and unit titles;
- position in spine;
- filename/path hints;
- text signals such as citation density, years, ISBN/URL markers, page references, and sentence density.

Signals must produce human-readable reasons. Title/path hints must be language-aware and must not be the only basis for a high-confidence main-matter decision.

## Recommendation policy

- High-confidence `main_matter`: recommend include.
- High-confidence front/back/reference material: recommend exclude.
- `unknown` or low-confidence units: recommend include only when excluding them would leave no main content; otherwise surface for review in Phase 3.
- `linear="no"` and explicit navigation-only resources remain excluded from ordinary content.

Phase 2 must not silently change the existing NLP analysis scope. It computes and persists recommendations alongside units. Phase 3 will apply reviewed selections.

## Reproducibility

Persist:

- classifier name and version;
- category;
- confidence;
- ordered reasons/signals;
- recommended inclusion;
- classification timestamp or analysis snapshot identity.

Re-running the same classifier over the same unit snapshot must produce deterministic output.

## Non-goals

- No machine-learning classifier;
- no external corpus lookup;
- no opaque difficulty score;
- no user selection UI;
- no automatic reanalysis using only the recommendation;
- no deletion of excluded units;
- no language-specific assumption that cannot be represented as a versioned rule.

## Phase 3 handoff

Phase 3 will display the unit tree and recommendation explanations, allow overrides, persist the reviewed scope, and send only the reviewed units to NLP. Coverage metrics must state the selected scope.

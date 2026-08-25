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

## Classifier output contract (v1)

Each recommendation is a JSON object with `schema_version: 1`, a `classifier`
object containing non-empty stable `name` and `version` tokens, and a
`source_unit_snapshot` object containing the immutable snapshot ID, extracted
unit schema version, and unit ID. This identity binds a result to one unit in
one source snapshot; consumers must not match results by title, path, or array
position.

`category` is exactly one of `front_matter`, `main_matter`, `back_matter`, or
`unknown`. `confidence` is a deterministic integer from 0 through 100,
inclusive, and measures confidence in the category only. Values 0 through 49
are low confidence, 50 through 79 are medium confidence, and 80 through 100 are
high confidence; `unknown` must be low confidence. A classifier version must
not change the meaning or calculation of a confidence value.

`reasons` is a non-empty array in classifier precedence order. Every entry has
a unique stable `signal` token and a non-blank human-readable `message`.
Writers and readers preserve this order; it is part of the explanation and is
not reconstructed from a map. Duplicate signals, leading or trailing message
whitespace, and control characters are invalid. `recommended_inclusion` is an
explicit boolean policy result, not a value consumers derive from category or
confidence.

For high-confidence `main_matter`, the policy result is `true`; for
high-confidence front or back matter it is `false`. An `unknown` or other
low-confidence result may carry either value: it is `true` only when the
classifier's whole-book fallback determines that exclusion would leave no
main content, and is otherwise `false` for Phase 3 review. The ordered reasons
must explain that decision. Phase 3 therefore consumes the boolean directly
and never invents a fallback.

Serialization uses fixed object fields and the supplied reason array order.
Invalid output is rejected before serialization, so repeated serialization of
the same validated result is byte-for-byte stable.

### Non-linear and navigation-only resources

The Phase 1 v1 extracted-unit snapshot does not emit `linear="no"` spine items
or manifest items marked `nav`, so they ordinarily have no classifier output
at all and remain outside ordinary content. If a future extracted-unit schema
records either as a metadata-only unit, its v1 classification recommendation
is explicitly `recommended_inclusion: false`; a stable reason such as
`linear_no` or `navigation_only` records why. Its structural category may be
`unknown` at low confidence because confidence describes the category, not
the certainty of the exclusion rule.

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

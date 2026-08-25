# Phase 2: Classify EPUB units and recommend analysis scope

Status: Implemented · Date: 2026-08-25

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

## Implemented behavior

EPUB import now classifies the persisted extracted-unit snapshot with
`mouseion-epub-structure` version `1.0.0` and atomically persists one result
per unit. Results are owner-scoped and bound to the immutable snapshot and
unit identities. Importing identical content replaces the same classifier run
without changing its output; importing changed content creates a new snapshot
and removes classifications tied to the old snapshot. Sources created before
extracted-unit snapshots report classifications as unavailable.

Rule evaluation and reasons use a fixed precedence: explicit landmark or
`epub:type`, title/navigation label, package path, spine position, then text
shape. Exact German and Italian markers are supported for the v1 rules,
including contents, preface, bibliography, notes, index, glossary, appendix,
chapter, part, introduction, conclusion, and epilogue equivalents. Strong
conflicting category signals yield low-confidence `unknown`; malformed or
unrecognized optional metadata is ignored and insufficient evidence is
reported explicitly. Navigation-only and non-linear units are explicitly
excluded if supplied to the classifier, although the Phase 1 extractor
normally omits them.

Recommendations remain metadata only. The source material's existing
`full_text` is still the input used by analysis jobs, including text from a
unit that the classifier recommends excluding. Classification neither queues
reanalysis nor changes the current NLP request document.

### Examples

- `Inhaltsverzeichnis` at the front of an EPUB 2 spine is classified as
  `front_matter`; `Kapitel 1` with sustained prose is high-confidence
  `main_matter` and recommended for inclusion.
- EPUB 3 landmarks such as `bodymatter chapter`, `bibliography`, `index`,
  `endnotes`, and `appendix` take precedence and appear first in the ordered
  reasons.
- Italian `Bibliografia`, `Indice analitico`, `Note finali`, and `Appendice`
  are classified as `back_matter` and recommended for exclusion at high
  confidence when corroborated by structure or path.
- A unit labeled `Capitolo` but carrying a `bibliography` landmark is
  `unknown` with a `contradictory_evidence` reason instead of silently choosing
  either category.

Readable, deterministic EPUB 2/German and EPUB 3/Italian snapshot fixtures
under `internal/epub/testfixtures/classifier/` lock the exact category,
confidence, recommendation, classifier identity/version, and ordered reasons.
PostgreSQL import coverage verifies the complete extraction-to-persistence
path, owner isolation, replacement, legacy behavior, and unchanged analysis
text.

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

## Limitations

- The v1 rules recognize a deliberately small English, German, and Italian
  vocabulary; other languages and unconventional labels can remain unknown.
- Text-shape signals are coarse counts, not semantic analysis, and short prose
  can remain low confidence.
- EPUB 2 has no EPUB 3 landmarks, so NCX labels, headings, paths, position, and
  text provide its available evidence.
- Missing, malformed, or vendor-specific navigation metadata reduces evidence
  but does not make otherwise readable EPUB content fail import.
- A recommendation is not a learner decision and is not applied to NLP in
  Phase 2.

## Phase 3 handoff

Phase 3 should read the persisted boolean and ordered explanations directly,
display the unit tree for learner review, persist overrides separately from
the classifier result, and construct an explicit reviewed text scope for NLP.
It must retain snapshot identity when applying selections, handle stale
reviews after re-import, and report the selected scope in coverage metrics.
Until that reviewed-scope path exists, analysis must continue using the
unchanged `full_text` compatibility input.

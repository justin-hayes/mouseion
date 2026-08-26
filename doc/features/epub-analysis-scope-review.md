# Phase 3: Learner-reviewed EPUB analysis scope

Status: Implemented · Date: 2026-08-25

## Problem

Phase 2 classifies EPUB units and persists transparent recommendations, but the learner cannot inspect or override the proposed scope. NLP analysis still operates on the existing full-text behavior.

## Goal

Show the extracted unit tree and classifier recommendations before analysis, let the learner include/exclude units, persist the reviewed scope, and analyze exactly that scope.

## Workflow

```text
EPUB imported
  → units extracted and classified
  → learner reviews recommended scope
  → learner accepts or overrides selection
  → immutable reviewed scope revision is persisted
  → learner explicitly starts analysis for that revision
  → analysis job processes selected units only
  → metrics report the selected scope
```

## UI requirements

Show for each unit:

- ordered hierarchy/flat spine position;
- title and fallback indicator;
- category and confidence;
- human-readable classifier reasons;
- recommendation;
- estimated size/token count where available;
- include/exclude control.

Provide:

- select recommended scope;
- select all main matter;
- include all;
- exclude all;
- validation that at least one readable unit is selected;
- clear warning when the learner overrides high-confidence exclusions;
- explicit analyzed-scope summary before confirmation.

## Scope semantics

A reviewed scope is an immutable selection snapshot associated with a source-material unit snapshot and analysis job. Reanalysis creates a new scope snapshot; it does not mutate historical corpus scope.

The analysis job sends only selected unit text to NLP, in deterministic order, while preserving unit IDs and source provenance. Coverage and structural metrics state the selected scope.

## Reviewed-scope contract (version 1)

The Go representation lives in `internal/epub/scope_contract.go`. A reviewed
scope records its schema version and immutable scope ID; owner and source
material IDs; the exact extracted-unit snapshot ID and schema version; the
classifier name and version whose recommendations were reviewed; selection
mode (`recommended` or `overridden`); and selected unit references. Each
reference contains only a unit ID and its original snapshot order. References
must be unique, readable members of that exact owner-scoped source snapshot and
must appear in strictly increasing source order. Empty selections, unsupported
schema versions, mismatched identities, duplicate IDs, forged order metadata,
and units outside the snapshot are invalid.

The browser may submit this identity and selection metadata, but never unit
text. The server validates against the persisted snapshot and reloads selected
text from those persisted units for later analysis. Fixed object fields and the
selected-unit array make serialization deterministic without map iteration.

Confirmation creates or resolves an immutable reviewed-scope revision but does
not queue analysis. A separate explicit submission creates the analysis run
bound to it. Retrying the same logical run resolves to its durable run and
attempt history. Confirming a changed selection creates a new scope ID; a later
submission creates distinct analysis and corpus history while earlier results
remain unchanged.
The corpus stores its reviewed scope ID and an ordered copy of selected-unit
provenance for later display and auditing.

A legacy source material with no extracted-unit snapshot returns
`ErrReviewedScopeUnavailable`. Absence is not an empty reviewed selection: the
existing analyze action retains the full-text path. Its corpus and insights are
explicitly labeled legacy/full-text and do not claim reviewed-unit provenance.

## Safety and compatibility

- Owner isolation and CSRF are mandatory.
- Existing books without extracted units retain the legacy analysis path or clearly report scope unavailable.
- Existing analyzed corpora are not silently rewritten.
- Empty/invalid selections are rejected before queueing NLP work.
- Unit text is never trusted from the browser; the server reloads persisted units.

CSRF protection applies to scope confirmation, and every source, snapshot,
classification, scope, job, corpus, and selected-unit query is owner-scoped.
Unknown IDs, duplicate IDs, forged order, stale snapshots, and cross-owner
references fail before NLP is queued. The analyzer receives one source document
per selected unit in spine order; excluded units are never concatenated into or
sent with that input. Consequently token counts, vocabulary, coverage,
projections, and structural profiles describe only the selected scope.

## Validation fixtures

The Phase 3 suite covers German EPUB 2/NCX and Italian EPUB 3/landmark fixtures
from deterministic extraction and classification through persisted review,
selected-unit analysis provenance, and scope-aware insight rendering. It also
covers accepting recommendations, individual overrides, high-confidence
exclusion warnings, unknown and contradictory units, empty/invalid selections,
CSRF, owner isolation, deterministic same-scope retries, changed-scope corpus
history, and the legacy full-text path. Review controls use native buttons,
checkboxes, labels, fieldsets, and legends; dynamic summaries are polite live
regions, and validation errors are exposed as focusable alerts.

## Phase 4 candidates

- Persist optional learner notes explaining overrides.
- Add bulk review filters for unusually large books without changing classifier output.
- Compare scopes and their metrics side by side.
- Offer an explicit re-extraction migration for legacy EPUBs.
- Improve hierarchical navigation display when EPUB structure supports it.

## Non-goals

- No automatic classifier redesign;
- no machine-learning classification;
- no automatic analysis or mastery changes;
- no EPUB editing;
- no cross-book scope composition;
- no external corpus lookup.

## Phase 3 acceptance

A learner can review a German or Italian EPUB’s unit recommendations, override them, confirm a scope, and verify that the resulting analysis and metrics contain only selected units with reproducible provenance.

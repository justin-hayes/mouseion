# Design roadmap

This document is the living implementation plan for the Mouseion design system.
It maps the canonical product architecture and accepted interaction decisions
(Phases 0 and 1) and the visual/component foundations (Phases 2 and 3) to
concrete implementation phases and bounded worker-agent issues.

Phases 0 through 3 are merged. Their durable decisions live in:

- [Terminology](terminology.md)
- [Screen inventory](screen-inventory.md)
- [Information architecture](information-architecture.md)
- [Design system](design-system.md)
- [Components](components.md)
- [Workflows](workflows/book-analysis-and-deck.md) (and the sibling workflow
  documents for acquisition, Learning, and Settings)

The GitHub issues behind each remaining phase are tracked under the matching
Milestone. Issue numbers below are the implementation issue that opens each
phase; the rest are linked from it and from the Milestone.

## Status

| Phase | Title | Status | Anchor |
|---|---|---|---|
| 0 | Design documentation baseline | Merged | #366 |
| 1 | Product and interaction decisions | Merged | #367 |
| 2 | Visual and design foundations | Merged | #368 |
| 3 | Reusable interaction/component layer | Merged | #369 |
| 4 | Core book workflow | Merged | #370 |
| 5 | Workflow extension | Merged | #373 |
| 6 | Quality gates | Merged | #378 |

## Sequencing and concurrency

Remaining phases depend on the canonical learner lifecycle and on the Phase 3
component layer. Workers must start from current `origin/main`, keep PRs to
their issue's vertical slice, and not absorb adjacent work.

- **Phase 4** is sequential on the shared book/result journey.
- **Phase 5** issues may run in parallel from fresh branches once the Phase 4
  result and preparation surfaces are merged.
- **Phase 6** issues depend on the browser harness and may run in parallel
  after rebasing on it.
- **Phase 6 reconciliation** runs last and must not become opportunistic
  redesign.

Phase 4, 5, and 6 worker notes are embedded in each issue rather than repeated
here.

## Phase 4 — Core book workflow (Milestone 1)

Applies the accepted system to the canonical path from library and scope
review through an exact, immutable analysis result and a server-rendered deck
preparation. Preserves explicit transitions: scope confirmation does not
analyze, analysis completion does not prepare a deck, and preparation does not
start a campaign.

| # | Issue | Dependencies | Purpose |
|---|---|---|---|
| 370 | Exact book-centered analysis result | None | Owner-scoped, immutable result at `/books/{id}/analyses/{run}` with insights, provenance, and history links; the backend identity/read contract (lookup by immutable `analysis-run-id`, not corpus or job ID) is the primary blocker and must land before other workers link to results |
| 371 | Library, book detail, and scope review express the next lifecycle action | 370 | Single dominant next action and durable analysis history at each lifecycle state |
| 372 | Deck preparation on the exact result with a server-rendered baseline | 370 | Explicit prepare/monitor/retry/cancel/download with a non-JavaScript path |

Order: 370 → 371; 372 may run after 370, in parallel with 371 from a fresh
branch if the worker first checks for overlapping result-template edits. The
backend read contract inside 370 is the gate for any parallel worker that adds
result links, history, or completed-job transitions.

## Phase 5 — Workflow extension (Milestone 2)

Extends coherent hierarchy, states, terminology, progressive enhancement, and
reusable components across acquisition, Learning, Settings, and operational
recovery.

The underlying verticals (OPDS acquisition, campaign lifecycle, settings, and
known-vocabulary import) are already implemented on `main`. These issues are
quality-hardening and design-system alignment passes over that shipped
behavior, not new feature work; they must not reopen or rewrite working domain
semantics. Each keeps product behavior aligned with the existing docs/ADRs and
adds the missing hierarchy, state, accessibility, and progressive-enhancement
coverage.

| # | Issue | Dependencies | Purpose |
|---|---|---|---|
| 373 | Distinguish destination navigation from the Add books action | Phase 4 | Three peer destinations plus a persistent, distinguishable acquisition action |
| 374 | Complete the connection-to-library acquisition workflow | Phase 4, 373 | Full acquisition hub, browse/search state preservation, and multi-add |
| 375 | Complete Learning campaign hierarchy and outcome confirmations | 372 | Active/remaining/consequence/queue/history ordering and explicit outcomes |
| 376 | Consolidate study languages and known vocabulary in Settings | Phase 4 | One coherent Settings workflow and `/known-vocab` consolidation |
| 377 | Align operational status and recovery surfaces | 370, 372 | Consistent async status/feedback/recovery with durable result navigation |

## Phase 6 — Quality gates (Milestone 3)

Adds durable browser, accessibility, responsive, theme, realistic-content, and
residual-consistency evidence for the design-system rollout.

| # | Issue | Dependencies | Purpose |
|---|---|---|---|
| 378 | Fixture-driven browser acceptance harness | Phase 5 | Deterministic, CI-runnable authenticated fixtures; no live catalog/NLP |
| 379 | Keyboard, focus, and asynchronous-state acceptance | 378 | Focus management, live-region behavior, and non-JavaScript paths |
| 380 | Responsive, theme, and realistic-content regression coverage | 378 | Compact/wide + light/dark layout and contrast evidence |
| 381 | Reconcile residual UI, routes, and design documentation | all of Phases 4–6 | Remove verified obsolete paths and align durable docs with shipped behavior |

## Definition of done for each issue

Before an implementation issue is complete its PR must:

- satisfy every acceptance criterion;
- add or update tests for changed behavior;
- keep generated Templ output committed and reproducible;
- pass `templ generate`, `go test ./...`, `go build ./...`, `go vet ./...`,
  `make lint`, and `git diff --check`;
- run the applicable integration harness when the environment supports it;
- preserve native server-rendered forms/links with HTMX only as enhancement;
- keep the diff limited to the issue's vertical slice;
- reference the GitHub issue and be opened for review, not left as a draft.

## Product decisions without a follow-up issue

Phase 1 defined several boundaries that remain out of scope and are not
represented as implementation issues yet. Do not introduce these as superficial
UI controls without a separate product contract:

- reopening or undoing a completed campaign;
- removing individual known-vocabulary entries;
- direct EPUB upload without a shipped feature contract;
- a catalog-management/administrator role.

## Current known limitations

- Browser-based visual verification is automated by the Phase 6 fixture-driven
  harness (#378); the keyboard, focus, and asynchronous-state gate (#379) and
  the responsive, theme, and realistic-content gate (#380) have landed. The
  reconciliation pass (#381) ships this final residual-consistency gate.
- A full local stack (PostgreSQL, NLP service, Go web server) is required for
  integration tests; Phase 6 fixtures remove the external-network dependency
  for browser acceptance.

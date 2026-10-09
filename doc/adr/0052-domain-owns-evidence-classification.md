# ADR 0052: The domain owns evidence classification

Status: **Accepted** · Date: 2026-09-09 · Author: Justin + opencode

## Context

Whether a Book is "analyzed", "stale", "acquired but unassessed", or "unavailable"
— and whether a Reading Journey member may become the Primary Goal — was a
decision restated in three places. The persistence read computed an
`EvidenceState` in SQL, the webapp re-derived it in a fallback function when the
SQL field came back empty, and the Journey classified the same concept again by
scanning status strings (calling it "current"/"unassessed" instead of
"analyzed"/"acquired_unassessed"). The copies already disagreed on semantics and
source tables, and the same raw analysis signals were reachable only by
bouncing between SQL, handler functions, and templ formulas.

## Decision

**The classification is a derivation on the domain types, not stored state.**
The `domain` module owns one `EvidenceState()` that reads the raw analysis
signals on `SourceMaterialSummary` (its `AnalysisStatus`, content revision, and
acquired-ness) and returns a single `BookEvidenceState` enum. My Books, the
Reading Journey, and the corpus/route insights all call the same method.

- **One classification everywhere.** `MyBookEvidenceState` is renamed
  `BookEvidenceState`; the SQL CASE that assigned `EvidenceState` and the
  `MyBook.EvidenceState` field are removed. `MyBook.EvidenceState()` returns
  `not_acquired` when there is no acquired source and otherwise delegates to
  `Acquired.EvidenceState()`.
- **Domain classifies, the webapp owns prose.** State and eligibility decisions
  live in the domain; English labels, descriptions, and reason strings stay in
  the webapp. `PrimaryGoal.IsActive()` and a goal-eligibility predicate join the
  domain; the predicate returns an enum reason code
  (`needs-current-content / analysis-in-progress / failed / cancelled / stale /
  no-completed-analysis / eligible`) that the webapp maps to prose.
- **Stale is the only stale signal.** The "needs review" substring matches in
  the webapp matched nothing persistence ever produced and are dropped.
- **Read-only projection, enforcement unchanged.** The goal-eligibility
  predicate is a display projection only. `CreatePrimaryGoal` / `ChangePrimaryGoal`
  still enforce eligibility at the persistence layer (ADR 0049).

## Considered options

- **Keep the SQL CASE as the authority, with the domain as a fallback.**
  Rejected: preserves the dual model and the drift risk this ADR removes.
- **Keep My Books and Journey classifications separate.** Rejected: they are the
  same underlying concept with different names, and unifying is the point.
- **Move prose into the domain too.** Rejected: user-facing copy does not belong
  in the domain layer; webapp keeps labels and reason strings.
- **A separate projection package.** Rejected: the domain type-space is already
  the shared leaf for all consumers, and these are pure derivations over domain
  fields.

## Consequences

- The change touches `internal/domain`, `internal/persistence`, `internal/webapp`,
  `internal/analysisinsights`, and `internal/fixtures` — the fixtures fake already
  classified from `AnalysisStatus` and simplifies to call the method.
- A future evidence state (e.g. a new analysis outcome) is added once in the
  domain and flows to every surface; no SQL CASE, fallback, or Journey scan to
  keep in step.
- Coverage percentages remain in the webapp/templ layer, deliberately deferred:
  they are numeric display derived from `AnalysisCoverage`, a separate concern
  from evidence classification.

## Related

- [ADR 0049: Reading intent triggers analysis](0049-reading-intent-triggers-analysis.md)
- [CONTEXT.md: Analysis evidence](../../CONTEXT.md)

# ADR 0052: The domain owns evidence classification

Status: **Accepted** · Date: 2026-09-09

## Context

Whether a Book is analyzed, stale, acquired but unassessed, or unavailable was
being restated in persistence SQL, a webapp fallback, the Reading Journey, and
the corpus insights. Those copies could disagree because they used different
raw signals.

## Decision

Evidence classification is a derivation on domain types, not persisted state.
`SourceMaterialSummary.EvidenceState`, `MyBook.EvidenceState`, and
`LanguageCorpusBookEvidence.EvidenceState` share the `BookEvidenceState` enum.
The persisted My Books projection and its SQL CASE are removed.

The domain also owns `PrimaryGoal.IsActive` and the read-only goal-eligibility
reason code. The webapp maps those reason codes to user-facing prose. The
`needs review` string is not a stale signal; only `stale` is classified as
stale. Persistence remains the enforcement boundary for Goal promotion.

## Consequences

- My Books, Reading Journey, and analysis insights use one evidence derivation.
- New evidence states are added once in the domain rather than in SQL and
  multiple presentation paths.
- Coverage percentages and other explanatory prose remain presentation logic.
- Corpus reads retain the raw current-content identities needed to distinguish
  unavailable content from unassessed evidence.

## Related

- [ADR 0049: Reading intent triggers analysis](0049-reading-intent-triggers-analysis.md)
- [CONTEXT.md](../CONTEXT.md)

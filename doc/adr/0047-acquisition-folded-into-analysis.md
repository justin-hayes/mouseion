# ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived

Status: **Accepted; the "Always the complete scope" clause is partially superseded by [ADR 0066](0066-main-text-selection-from-epub-structure.md)** · Date: 2026-09-06 · Author: Justin + opencode

## Context

Mouseion's current flow separates content acquisition from analysis as two
explicit learner steps. A synced Book sits in My Books as metadata-only
(ADR 0041); the learner then acquires an EPUB via `POST /opds/acquire`
(ADR 0035's validated source snapshot), and only then can they start analysis.
The acquisition step is signed-form round-trip ceremony, and it leaves a
learner-facing `acquired_unassessed` checkpoint between acquiring and analyzing.

The learner is the product's only user and the app is pre-production. Two
consequences follow. First, the explicit acquire step is friction without value:
the only thing a learner does with an acquired EPUB is analyze it, so acquisition
is better folded into the analysis action and pulled server-side. Second, because
there is no multi-user or legacy population to protect, the cleaner long-term
shape is preferred over non-destructive deprecation, even where that means
removing previously-shipped features and deleting data.

## Decision

**Content acquisition becomes a server-side side effect of submitting a Book for
analysis, and the library becomes strictly catalogue-derived.**

- **Acquisition is folded into analysis.** The metadata-only Book's analysis
  action resolves its current EPUB target server-side
  (`cataloguesync.FindAcquisitionTarget`), downloads the EPUB, imports and
  persists it (idempotent by content digest), and submits analysis in one flow.
  The signed acquisition-target form round-trip (`AcquisitionTargetKey`,
  `encode`/`decodeAcquisitionTarget`, the hidden field) is deleted; the CSRF
  token on the analysis form protects the action. The `acquired_unassessed`
  state remains as an honest internal transient (content persisted, analysis not
  yet complete) but is no longer a designed learner-facing step. On failure the
  request fails inline with the existing OPDS error messaging; a retry re-imports
  idempotently.
- **The catalogue is the source of truth for Book metadata.** Synced Books are
  corrected only by re-syncing. The manual "Add a book" creation path and the
  "Fix book language" edit path are removed, along with
  `MetadataProvenanceManualEntry`, the manual-dedup branch of `CreateBook`, and a
  destructive migration of existing `manual_entry` rows. The `unknown` language
  state is no longer creatable (legacy rows remain readable); synced Books are
  created with a chosen language from the catalogue feed.
- **EPUB-only.** Plain-text analysis is removed; acquisition requires an EPUB.
- **Always the complete scope.** Analysis always processes every readable unit of
  the current extracted snapshot. The residual reviewed-scope machinery
  (`epub_reviewed_scopes` writes, `SubmitScopedAnalysis`, the scoped worker path,
  `CreateEPUBReviewedScope`/`FindFullBookScope`) is removed; analysis is one
  snapshot-based path keyed on the immutable source-content revision. The
  learner-facing scope-review UI had already been retired; this removes the
  internal provenance that no longer has a consumer.

This ADR resolves the open acquisition-trigger questions left by ADR 0041:
acquisition is triggered by the analysis action, runs inline in the request, and
is not a separately exposed learner operation.

## Destructive schema deferred

Clean code now; destructive schema as separate, reviewed migrations. The
`epub_reviewed_scopes` table, the `analysis_runs.scope_id` / `analysis_jobs`
`reviewed_scope_id` columns, and the `manual_entry` data drop are deferred because
deck preparation and coverage currently read scope IDs. Legacy scope rows and
`manual_entry` provenance remain readable until those references are unwound.

## Alternatives considered

- **Keep explicit acquisition as a separate action and add a shortcut.**
  Rejected: the learner only acquires to analyze; a separate step is pure
  friction once the signed-form ceremony exists.
- **Do the download in the River worker.** Rejected: it forces scope building and
  submission into the worker and delays failure feedback; the request-path import
  reuses the existing importer unchanged.
- **Deprecate rather than remove** manual books, plain text, and scope review.
  Rejected: deprecation leaves dead code paths and permanent
  "who still depends on this?" questions. For a pre-production single-user app,
  true removal is simpler to verify (grep for zero references, run the suite) and
  the catalogue is the recoverable source of truth for deleted data.

## Consequences

- One learner action (analysis) now covers acquisition; failures surface inline
  and retry idempotently.
- My Books is populated only from a connected catalogue; the empty state reflects
  "connect and sync" as the sole entry.
- The analysis contract becomes snapshot-based and EPUB-only; the ordinary
  `full_text`-in-args path and the scoped path collapse into one.
- Destructive schema removal is a reviewed follow-up, not part of this change.
- Existing analyzed corpora, decks, and known vocabulary remain intact; only the
  code paths that create them change.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)
- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
- [EPUB analysis scope review feature (retired)](../archive/features/epub-analysis-scope-review.md)
- [Catalogue sync feature](../features/catalog-sync.md)

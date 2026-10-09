# Set Aside retirement: release-readiness evidence (issue #1593)

This records what was verified for slice 13 of #1580 and what is still open.
It is evidence for review, not an authorization to run the production cutover
(see `2026-10-set-aside-retirement.md`). **Overall status: implementation and
runbook ready for review; scale evidence is NOT recorded and remains a
release-readiness blocker.**

## Implementation verification

Run on the branch head before opening the pull request (2026-10-09). Outcomes are copied
from the actual runs; a failed or skipped item is listed as such.

| Check | Result |
| --- | --- |
| `make test` (Go unit + pytest) | passed (Go all packages ok; pytest 63 passed) |
| `make test-integration` (uncached, `-count=1`, real PostgreSQL) | passed (all packages ok, including the six cutover/structural tests) |
| `make lint` (golangci-lint over the whole module + ruff) | passed (0 issues; ruff clean) |
| `make browser-smoke` (Playwright, `cmd/fixtureserver`) | passed (534 passed, 2 skipped by existing config) |
| Generated freshness: `make templ`, `make sqlc`, `make check-frontend-css` | `make sqlc` regenerated `gen/sqlc/models.go` for the new tables (committed); templ and stylesheet checks clean |

Behavior covered by tests (all earlier slices own their behavior tests; this
slice adds the retirement and conversion tests):

- **Retirement.** `POST /library/books/{id}/set-aside` and the old
  `/reading/.../set-aside` paths are unregistered and return 404 (they are not
  mapped to Hide or End); `?disposition=set_aside` is an unknown filter handled
  like any unknown value; no Set Aside control, filter, or confirmation renders;
  `domain.BookDisposition("set_aside").Validate()` fails; the persisted
  constraint rejects the value. Finish still leaves Inbox and End still leaves
  To Read. Retired Journey, selection, and Custom-deck boundaries are unchanged
  and no Custom-deck record or artifact is deleted.
- **Structural migration `000033`.** Rejects the retired value for new and changed
  rows while leaving legacy rows unconverted and visibility uninitialized; a
  database with no legacy row is validated by the migration; the down migration
  restores the three-value constraint without mapping data.
- **Data conversion** (`internal/persistence/book_disposition_cutover_integration_test.go`,
  PostgreSQL with the `integration` tag): seeds unread, assertion, completion,
  and mixed-history legacy Books for two owners, an active Current reading with
  its snapshot, Known vocabulary, and preparing/ready/historical deck artifacts,
  plus pre-existing Inbox, To Read, and Hidden Books. Verifies: only legacy rows
  convert (to Inbox, revision + 1); unread Books project Inbox and history Books
  project Read; no Book becomes Hidden or To Read; missing visibility becomes
  visible and existing Hides are untouched; every preserved table is unchanged
  by count and content hash; commit, checkpoint, and audit agree; reruns and a
  lost acknowledgement converge with one checkpoint and no reset of later
  learner edits; an injected failure mid-transaction leaves nothing half-mapped
  and a retry converges; unexpected state (a Set Aside Current reading, a
  Current reading that is not To Read, drift of a preserved table, an edited
  manifest) is refused without repair; and guarded forward correction refuses
  every row a learner changed after cutover.
- **Server guard.** `cmd/server` refuses to start while a legacy disposition
  remains (`RequireNoLegacyDispositions`), so reopening before conversion fails
  closed.

## Cross-slice acceptance map

| Area | Evidence |
| --- | --- |
| Hidden lifecycle, bucket and count participation, Show hidden | `internal/webapp/my_books_visibility_integration_test.go`, `internal/persistence` visibility and browse tests |
| Failure-safe actions, commitment-bound Finish/End/Switch | `internal/webapp/goal_*`, `reading_chooser_*`, `reading_start_failure_integration_test.go` |
| Supported Browse-Concordance-Study round trips | `internal/webapp` round-trip tests (#1592) and `e2e` specs |
| Language precedence | `internal/webapp/active_language_test.go` |
| No-JavaScript and enhanced safety | `internal/webapp` handler tests and `e2e/tests` |
| Retirement | tests above |

## Scale and performance evidence: NOT recorded (blocker)

The acceptance targets (from `doc/features/vocabulary-browse-and-concordance.md`
and ADR 0084) are:

- On a documented **warm** reference corpus of roughly 500 analyzed Books and
  50 million tokens in one language: p95 of completed Browse and ordinary
  Concordance lookup/paging requests at most 2 seconds.
- Enhanced requests show pending feedback by about 1 second and end failed
  interactive work by 10 seconds with honest recovery.
- No evidence is sampled to meet a deadline.
- The candidate threshold of ten versus seven validation obligation is kept as
  is; this change alters no threshold.

**No measurement has been taken.** The repository contains no reference-corpus
generator or latency harness, and a 500-Book / 50-million-token corpus was not
built or exercised in this work. Nothing here claims the targets are met. Until
a run is recorded below, the release is not scale-ready and the maintenance
window should not proceed on performance grounds.

Record, when measured (this section must be filled by the person who runs it):

| Field | Value |
| --- | --- |
| Hardware (CPU, RAM, storage, PostgreSQL version and settings) | _not measured_ |
| Source mix (languages, Books, tokens, analysis versions) | _not measured_ |
| Workload (request mix, concurrency, warm-up, sample count) | _not measured_ |
| Measurement boundary (client-observed complete response, server only, etc.) | _not measured_ |
| Browse p95 (completed) | _not measured_ |
| Ordinary Concordance lookup/paging p95 (completed) | _not measured_ |
| Enhanced pending feedback time | _not measured_ |
| Failed interactive recovery time | _not measured_ |
| Threshold ten-versus-seven validation outcome | _not measured_ |

## Remaining human steps

1. Justin reviews the pull request (workflows, `migrations/`, `doc/adr/`,
   `doc/product.md`, and `internal/auth`-adjacent paths are in the human-review
   boundary) and decides whether the ADR 0086 status wording is acceptable.
2. Justin runs the scale measurement and fills in the table above.
3. Justin performs the maintenance window using
   `2026-10-set-aside-retirement.md`; nothing in this change executes it.

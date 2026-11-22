# ADR 0121: One status vocabulary for Book preparation, and durable EPUB acquisition

Status: **Accepted** · Date: 2026-11-22 · Author: Justin + OpenCode

Amends [ADR 0047](0047-acquisition-folded-into-analysis.md) ("runs inline in
the request" and its rejection of downloading in the River worker) and
[ADR 0117](0117-one-catalog-my-books-holds-chosen-books.md) ("Acquisition still
runs in the request" and its deferred alternative). Decided in a grilling
session on 2026-11-22 for
[issue #2502](https://github.com/justin-hayes/mouseion/issues/2502).

## Context

Three kinds of durable work reach `/jobs`, and each describes itself
differently.

- **Analysis** has its own run states, a progress bar and a free-text error.
  The stored error is one of two fixed sentences, with no stable code. A run
  whose finalization failed shows Completed in the list and Queued on its detail
  page.
- **Classical preparation** shows the River job state with a stable stage and
  code, and no link.
- **Catalogue sync** shows a state as plain text, with its own mapping from
  River states.

Each has its own function for turning a River state into a label and tone.

EPUB acquisition is not durable work at all. It runs inside the Add, Move to To
Read or Retry request (ADR 0047), so the request waits for a download and an
import. ADR 0117 began recording its failure per Book, reusing Classical
preparation's stage and code names, and deferred the question of moving it to a
job.

ADR 0047 rejected downloading in the worker for two reasons. Neither still
holds:

- *It forces scope building and submission into the worker.* ADR 0047 itself
  removed reviewed scopes, and the analysis worker already re-derives the
  main-text scope from the current snapshot.
- *It delays failure feedback.* Since ADR 0117, Add creates the Book before its
  EPUB exists, and a failure is recorded as Book evidence with Retry. The
  learner no longer depends on the response to learn about a failure.

The two failure tables, `classical_preparation_failures` and
`epub_acquisition_failures`, share a key, `(owner_id, book_id)`, and the columns
`stage`, `code` and `failed_at`. They differ only in provenance columns. The
glossary names Classical preparation but has no word for what an EPUB Book goes
through.

## Decision

The vocabulary and durable acquisition were decided separately. Either could
have been adopted without the other.

### Book preparation is the subject

**Book preparation** is the durable work that makes a To Read Book readable. It
has two kinds:

- **Classical preparation**, unchanged.
- **EPUB preparation**: acquire the Catalog entry's Catalog file, then analyze
  it. EPUB acquisition is its first stage.

Every Book preparation is described by three things: a lifecycle state, a stage
and, on failure, a code.

### One lifecycle state

The lifecycle is exactly **Queued, Running, Completed, Failed, Cancelled**. It
is one domain type, and one mapping turns a River job state into it. Analysis's
"Processing" label becomes "Running".

Catalogue sync shares the lifecycle state and its tone on `/jobs`, and nothing
else. Its subject is a connection, not a Book, so it has no stage or code and
writes no Book preparation failure.

### One stage list

| Stage | Meaning | Kind |
|---|---|---|
| `entry` | The Catalog entry has left the catalog index | EPUB |
| `catalog` | The Classical catalog precondition failed | Classical |
| `withdrawn` | The Reading-unit definition was withdrawn | Classical |
| `fetch` | The file could not be downloaded | Both |
| `source` | The file was refused | Both |
| `import` | The file could not be imported | Both |
| `analysis` | The analysis run | EPUB |
| `finalize` | Work after the analysis is published | EPUB |

The first six keep their current names, so no stored value is translated.
`entry` and `withdrawn` stay separate because their recoveries differ: Retry can
help the first and not the second.

`finalize` replaces the logical states `finalizing`, `finalization-failed` and
`finalization-cancelled`. They become Running, Failed and Cancelled at stage
`finalize`.

### One code vocabulary

Codes are one domain type shared by both kinds and by analysis. Existing code
values are unchanged. Analysis gains three: `analysis_failed`,
`dependency_parsing_unavailable` and `analysis_interrupted` (the worker
disappeared mid-run).

### One Book preparation failure record

`classical_preparation_failures` and `epub_acquisition_failures` merge into
`book_preparation_failures`.

- The key stays `(owner_id, book_id)`. A Book is only ever one kind, so keys
  cannot collide.
- It carries `stage`, `code` and `failed_at`, plus each kind's provenance
  columns (Reading unit key and definition revision; connection and entry
  identifier). These are nullable, and a constraint requires exactly one kind's
  set. An EPUB row may omit both catalog identities only at the `entry` stage,
  when acquisition could not resolve an entry at all; every other failure names
  its entry.
- The stage check allows the six stages before analysis.
- The stage is read from the row. The two existing fallbacks that infer a stage
  from an unrecognised code disagree (`fetch` for EPUB, `import` for Classical)
  and are removed.
- Clearing stays per kind. An EPUB row is deleted when acquisition succeeds. A
  Classical row is deleted when preparation succeeds, and is treated as cleared
  when its definition revision is behind the current one.

The glossary term is **Book preparation failure**. Classical preparation
failure and EPUB acquisition failure remain as the names of its two kinds.

### An analysis failure stays on the run

An analysis failure's code is stored on the run and its attempt, in a new
`failure_code` column on `analysis_runs` and `analysis_run_attempts`. Its stage
is implied: `analysis`, or `finalize` once the run is completed. It is not
written to `book_preparation_failures`.

The rule: the failure record holds failures that happen before a run exists, and
the run holds the rest.

The learner-facing sentence is derived from the code when it is shown. The
`last_error` and `error` text columns stop being written.

### EPUB acquisition is a durable job

Acquisition moves out of the request into a new River job, `epub_acquisition`,
on its own queue. The job downloads and imports the EPUB, then submits the
existing analysis job unchanged. EPUB preparation is therefore two chained jobs,
not one job with stages.

- **One attempt per job.** The job uses `MaxAttempts: 1`, like analysis and
  Classical preparation. A retry is a new job, started by the learner's Retry.
- **One live job per Book.** A second Add or Retry while an acquisition is
  queued or running does nothing.
- **Re-check at work time.** The worker confirms the Book is still To Read
  before downloading. A Book removed from My Books while queued is skipped.
- **No Cancel action** for acquisition. Analysis keeps the one on its detail
  page.
- **A worker that dies between import and submission** leaves the Book acquired
  but unanalyzed. Book evidence already classifies that state, and Retry already
  submits the analysis without downloading again.

### The response confirms; the Book carries the outcome

Add, Move to To Read and Retry return at once with one plain confirmation that
the Book is being downloaded and analyzed. None of them reports a download
failure in the response. While the preparation is queued or running, My Books
shows the Book's state as it does for a classical Book. A failure appears there
as Book evidence, with Retry where retrying can help.

This replaces ADR 0047's inline failure and ADR 0117's messages appended to the
Add and Move responses.

### `/jobs` has two tables

- **Book preparations**: Classical preparation, EPUB acquisition and analysis
  jobs, one row per job, with the same columns for all: kind, Book, status,
  stage, code, progress where known, and created. Each row links to its Book. An
  EPUB preparation appears as two rows.
- **Catalog sync**: its own table, with the shared status badge, the connection
  and the error text.

`/jobs/{id}` remains for analysis and catalogue sync. Classical preparation and
EPUB acquisition rows link only to the Book.

## Alternatives considered

- **Share only the lifecycle state.** Rejected: `/jobs` and Book evidence would
  still have two ways to say why something failed, a stage and code for one path
  and free text for the other.
- **One job with stages for EPUB preparation.** Rejected: an analysis run is
  keyed on a content revision and a snapshot, which do not exist until the
  import finishes, so the run model would have to change. A download would also
  wait behind a long analysis on the single-worker `analysis` queue, and
  retrying a failed analysis would download again.
- **Record analysis failures in the failure table too.** Rejected: the run is
  already the durable record of an analysis, with attempt history, and Book
  evidence already reads the latest run. A second copy would be a second source
  of truth.
- **Keep two failure tables and share only the Go types.** Rejected: once
  acquisition is a job the two kinds have the same shape end to end, and two
  tables would keep two of every query and classifier for no difference in
  meaning.
- **Fold `entry` and `withdrawn` into one stage.** Rejected: both mean the
  offering is gone, but their recoveries differ.
- **Give catalogue sync stages and codes.** Rejected: sync never produces Book
  evidence, and Book-shaped stages do not describe it.
- **Supersede ADR 0047.** Rejected: its other decisions (the catalogue-derived
  library, EPUB-only acquisition, the complete scope as amended by ADR 0066)
  remain in force.

## Consequences

- Add no longer waits for a download. The learner learns of a failure from My
  Books, not from the response.
- The schema changes need human review under CODEOWNERS and follow ADR 0038 and
  ADR 0070: structural migrations are kept separate from the data backfill, and
  the destructive cleanup is its own reviewed migration.
- The backfill into `book_preparation_failures` copies rows under the same key
  with unchanged stage and code values. It is idempotent and safe to retry, and
  it is reversed by reading the old tables again, which stay in place until the
  cleanup.
- Delivery in six slices:
  1. The shared lifecycle state, stage and code types; one label and tone
     mapping for `/jobs`; `finalize` as a stage. No schema change.
  2. Analysis failure codes: the additive `failure_code` columns and the
     classifier. Independent of slice 3.
  3. `book_preparation_failures`: the table, then the data-only backfill, then
     reads and writes switched to it.
  4. The `epub_acquisition` job, and the new Add, Move to To Read and Retry
     confirmation.
  5. `/jobs` in two tables.
  6. Cleanup: drop the two old failure tables and the unused error-text columns.
- `CONTEXT.md` gains Book preparation, EPUB preparation and Book preparation
  failure. `doc/features/catalog-sync.md`, `doc/design/screen-inventory.md` and
  `doc/design/terminology.md` are reconciled as the slices ship.
- Not decided here: every historical catalogue-sync row on `/jobs` shows Failed
  when the connection's latest sync failed. That is a query defect, tracked
  separately.

## Related

- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
- [ADR 0047: Content acquisition is folded into analysis](0047-acquisition-folded-into-analysis.md)
- [ADR 0052: The domain owns evidence classification](0052-domain-owns-evidence-classification.md)
- [ADR 0070: Migration and documentation reboot](0070-migration-and-documentation-reboot.md)
- [ADR 0116: A curated Classical catalog of Ancient Greek Reading units](0116-classical-catalog.md)
- [ADR 0117: One Catalog for every source; My Books holds only chosen Books](0117-one-catalog-my-books-holds-chosen-books.md)

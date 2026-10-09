# ADR 0078: Book dispositions and current reading

Status: **Accepted** · Date: 2026-09-26 · Author: Justin + OpenCode

Amended by [ADR 0086](0086-reading-working-desk-hidden-visibility-and-concordance.md) §9: the three-value disposition, Set Aside, bucket precedence, stop/set-aside wording, and cutover/route clauses are superseded; its retained clauses stand. Code is complete in the repository; no production data conversion is needed because no Book was ever Set Aside.

Records the shipped Reading workflow specified by
[issue #1187](https://github.com/justin-hayes/mouseion/issues/1187). The issue
and repository history contain no separately accepted ADR 0078 predating this
record; this ADR does not claim that a missing document was accepted earlier.
It supersedes the retired learner-facing Journey/Goal contract in
[ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md), and the
conflicting ordering and role clauses of ADRs 0034, 0049, and 0051. Their
unrelated retained decisions continue to apply.

## Context

The shipped Journey and Primary Goal model made learners maintain a speculative
Book order and exposed sequential forecasts alongside the actual current
reading commitment. The replacement workflow is shipped: My Books owns each
Book's disposition; Reading owns the current Book or a neutral chooser. The
feature specification describes present behavior, but its reference to an
unpublished ADR 0078 falsely implied a pre-existing accepted record.

## Decision

### Disposition and current-reading state

Each owner-and-Book pair has one disposition: **Inbox**, **To Read**, or
**Set Aside**. Catalog sync initializes Inbox only for a new Book and never
resets an existing disposition. Current reading is a separate role, limited to
one analyzed To Read Book per owner and study language. Read history is
independent of disposition and current-reading state, append-only, and records
current-reading completions plus one idempotent previously-read assertion. The
assertion records when the learner made it, not an inferred reading date. The
visible bucket is derived in this precedence: Current reading, To Read,
Read (when history exists), Inbox, then Set Aside. The To Read browse tab
includes Current reading, labeled distinctly; previously-read Inbox Books
appear in Read without rewriting their underlying disposition.

My Books owns collection browsing and disposition changes. Reading owns
starting, stopping, setting aside, switching, and finishing current reading.
Its between-Books chooser groups trustworthy To Read Books by exact current
coverage bands and sorts neutrally by title and author; it does not maintain an
order, recommend a Book, or forecast a sequence. There is no automatic next
Book.

Starting freezes the exact analysis and immutable vocabulary snapshot for the
current reading. Reserved vocabulary derives only from that active snapshot;
it is not Known. Local deck preparation may be dispatched from the snapshot,
but dispatch or artifact failure cannot roll back the reading. External
translation remains separately consented. Finishing records append-only
completion, accepts eligible snapshot identities into modeled Known vocabulary
with set semantics, ends the role and reservation, and is idempotent. Stopping
or setting aside releases the reservation but preserves snapshot and artifact
provenance. A switch is atomic; stale or failed mutations make no partial
changes.

### Migration and route boundaries

The cutover preserves source and analysis evidence, active snapshot and
reservation provenance, completions, Known and generated vocabulary,
prepared artifacts, and operational history. Its idempotent backfill migrates
the active Primary Goal to current reading with its exact snapshot, reservation,
and artifacts; other Journey Books become To Read. Books with history project
to Read unless current or To Read; other active collection Books become Inbox;
previously removed Books become Set Aside. Journey order is discarded.
Structural DDL and data-only backfill remain separate under
[ADR 0038](0038-schema-change-governance.md); the baseline remains immutable.

`/reading` is canonical. `/journey` permanently redirects there. Authorized
Book and exact-analysis bookmarks redirect only for Current or To Read Books;
other or unreachable Books do not regain a generic detail surface. Retired
Journey and Goal mutation routes are rejected, not translated. My Books can
express To Read intent but cannot start or switch current reading.

## Consequences

- Reading intent, active commitment, reading history, and deck artifacts remain
  distinct state with explicit ownership and failure boundaries.
- Learners choose a Book when they are ready to start; no stored sequence or
  forecast competes with that choice.
- Existing evidence and provenance survive the cutover, while Journey order
  and Goal terminology are historical rather than current product concepts.
- ADR 0072 remains available as historical context, not as the current learner
  contract.

## Related

- [Issue #1187: Replace Reading Journey with Book dispositions and current reading](https://github.com/justin-hayes/mouseion/issues/1187)
- [Reading workflow specification](../features/reading-workflow.md)
- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0072: Goal-owned vocabulary and Journey forecast (historical)](0072-goal-owned-vocabulary-and-journey-forecast.md)

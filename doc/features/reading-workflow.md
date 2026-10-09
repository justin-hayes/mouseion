# Reading workflow

Status: **Implemented** · Date: 2026-09-26. Set Aside retirement and independent Hidden visibility are code-complete in the repository under ADR 0086; the production data cutover is pending.

The accepted [Reading-owned Book vocabulary](reading-owned-book-vocabulary.md)
contract is being implemented in slices. New Book-deck preparation is now
available only for the Current reading and uses its frozen snapshot; starting,
switching, and finishing do not enqueue or depend on deck work. The existing
three-occurrence selection floor remains in effect until the corpus-qualified
two-occurrence slice ships. Custom-deck retirement is also pending.

## Goal

Support an explicit, reversible path from catalog discovery to reading without
turning a set of intended books into an ordered plan. My Books records each
Book's workflow disposition; Reading owns at most one current Book per study
language, or presents the learner with an unordered chooser between Books.

## Shipped workflow

1. Catalog sync adds new Books to **Inbox** as metadata-only records. Books with
   no supported language remain in the display-only **Needs language** list
   until catalog metadata is corrected and synchronized again.
2. In My Books, move an Inbox Book to **To Read** to express intent. This records
   the disposition and ensures content acquisition and current analysis when
   needed. Analysis may be queued, running, ready, stale, failed, or unavailable;
   evidence status does not create a recommendation or order.
3. Reading shows either the current Book or To Read candidates grouped by their
   current lexical-coverage band, with incomplete evidence kept distinct. While a
   Book is current, the same page hosts its vocabulary Browse (the working desk)
   and lists no other To Read candidates. The learner can start a supported
   candidate, or end or switch a current Book through explicit confirmations. End releases only the Reserved vocabulary,
   records no completion or Known acceptance, and leaves the Book in To Read
   with its snapshots, history, and artifacts intact; reading never ends from
   inactivity. End, Finish, Switch, and new deck preparation, retry, or
   re-preparation must name the exact expected commitment (Book and snapshot);
   missing or stale identity, including after a same-Book restart, is rejected
   with no change. A replay is accepted only when durable facts prove it. The
   retired Set Aside action and its mutation are removed, not translated.
   A successful Switch releases the former reservations and freezes a fresh
   snapshot for the replacement in one transaction; the former Book stays in To
   Read. A failed, stale, review-gated, or count-readiness-blocked Switch changes
   nothing: no release, completion, or Known acceptance.
4. Starting current reading freezes the vocabulary snapshot. Returning to a Book
   after End or Switch, or rereading it, is always a fresh start with a fresh
   freeze reflecting the then-current Known and Reserved vocabulary; a released
   snapshot is never resumed or expanded. Finishing records a
   completion, accepts eligible snapshot identities into modeled Known
   vocabulary with set semantics, clears the current role and reservations, and
   sets the underlying disposition to **Inbox**, all atomically; a failure
   leaves no partial history or Known change. The Book is neither hidden nor
   replaced by a next Book: the receipt only links to the chooser. Finish does
   not depend on a deck, its preparation state, omitted cards, or live Browse
   counts, and works with an empty frozen snapshot. Replaying the same Finish
   adds no history or acceptance, and a stale snapshot of the same Book cannot
   finish a newer reading.
5. My Books shows each Book in exactly one visible workflow bucket. In
   precedence order, that is **Currently reading**, **To Read**, **Read** when
   completion history exists, then **Inbox**. The To Read tab
   includes the current Book, labeled Currently reading, as well as other To
   Read Books. A historical Inbox Book appears in Read without
   changing its persisted disposition or completion provenance; a historical
   To Read Book remains To Read. A finished Book is Inbox underneath and
   projects as Read through its history. **Read again** moves a Read Book to To
   Read without starting it, changing visibility, erasing history, or adopting a
   released snapshot. Starting it creates a fresh current-reading snapshot while
   retaining earlier completions. A previously-read assertion is an idempotent
   assertion-time fact that changes neither disposition nor visibility and
   accepts no vocabulary. Finishing claims no mastery.
6. Hiding is a separate, reversible visibility choice: a Hidden Book is omitted
   from default My Books and the default chooser, keeps its disposition, history,
   and artifacts, and is recovered with **Show hidden books** and Unhide. Read
   history is append-only and does not imply Known vocabulary. There is no
   ordinary Remove from My Books action; the retired removal request cannot
   remove membership or erase the Book, its evidence, history, or artifacts.

The current product and architecture decision is [ADR
0078](../adr/0078-book-dispositions-and-current-reading.md). ADR 0072 records
the superseded Goal/ordered-Journey model as historical context.

## Interaction and recovery contract

- The chooser has no learner-authored sequence, ranking, forecast, or suggested
  next Book. Coverage bands describe current evidence only.
- Every consequential current-reading mutation names its Book and uses a
  confirmation. Forms carry expected Book/snapshot identity so stale submissions
  cannot mutate a newer current-reading state.
- Server-rendered links, forms, labels, and status text remain usable without
  enhancement. Mutation outcomes preserve a useful route and announce feedback;
  focus and keyboard order follow document order.
- Empty To Read, failed or stale analysis, unsupported-language, and recovery
  states retain a clear path back to My Books, Catalogs, or the relevant retry.
- Working desk dependency states stay distinct and never become a successful
  zero: missing or stale analysis withholds Browse and keeps identity, Finish,
  Switch, End, and retained downloads; **Updating vocabulary counts** and
  **Vocabulary counts unavailable** (bounded retries exhausted) both offer a
  plain **Refresh status** link that only re-checks and never requeues work or
  reruns analysis; **No eligible vocabulary in this analysis** differs from an
  empty frozen selection; a no-match prefix is retained with **Clear prefix**;
  all-accounted-for explains Known/Reserved/prepared inclusion without implying
  mastery. Preparation failure stays preparation-specific with Retry bound to
  the same frozen snapshot, and Finish depends only on frozen acceptance.
- Compact and desktop layouts retain Book identity, status, and primary actions
  without page-level horizontal scrolling. Reduced-motion preferences are
  respected.

## Acceptance coverage

Playwright browser smoke covers the catalog-to-My-Books path, disposition
controls, current-reading selection and completion, Read history and rereading,
language-change recovery, recovery states, keyboard/focus semantics, and compact/desktop
layouts. PostgreSQL integration tests cover cutover migration invariants,
including disposition contraction, active reservation snapshot contents,
completion/history provenance, and prepared-deck provenance. See
[`../design/screen-inventory.md`](../design/screen-inventory.md) for current
screen responsibilities and [`../../e2e/README.md`](../../e2e/README.md) for
running browser acceptance.

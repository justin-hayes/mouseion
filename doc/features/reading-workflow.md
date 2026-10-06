# Reading workflow

Status: **Implemented** · Date: 2026-09-26

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
   current lexical-coverage band, with incomplete evidence kept distinct. The
   learner can start a supported candidate, or stop, set aside, or switch a
   current Book through explicit confirmations.
4. Starting current reading freezes the vocabulary snapshot. Finishing records a
   completion, accepts eligible snapshot identities into modeled Known
   vocabulary, and returns a receipt with the next-choice link. It does not
   automatically select another Book.
5. My Books shows each Book in exactly one visible workflow bucket. In
   precedence order, that is **Currently reading**, **To Read**, **Read** when
   completion history exists, **Inbox**, then **Set Aside**. The To Read tab
   includes the current Book, labeled Currently reading, as well as other To
   Read Books. A historical Inbox or Set Aside Book appears in Read without
   changing its persisted disposition or completion provenance; a historical
   To Read Book remains To Read. **Read again** moves a Read Book to To Read;
   setting it aside again returns it to Read. Starting it creates a fresh
   current-reading snapshot while retaining earlier completions.
6. Set Aside keeps a Book visible in My Books and reversible to To Read. Read
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
- Compact and desktop layouts retain Book identity, status, and primary actions
  without page-level horizontal scrolling. Reduced-motion preferences are
  respected.

## Acceptance coverage

Playwright browser smoke covers the catalog-to-My-Books path, disposition
controls, current-reading selection and completion, Read history and rereading,
language handoff, recovery states, keyboard/focus semantics, and compact/desktop
layouts. PostgreSQL integration tests cover cutover migration invariants,
including disposition contraction, active reservation snapshot contents,
completion/history provenance, and prepared-deck provenance. See
[`../design/screen-inventory.md`](../design/screen-inventory.md) for current
screen responsibilities and [`../../e2e/README.md`](../../e2e/README.md) for
running browser acceptance.

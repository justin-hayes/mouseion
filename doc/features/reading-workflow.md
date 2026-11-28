# Reading workflow

Status: **Implemented** · Date: 2026-09-26 · Updated: 2026-11-28. Set Aside retirement and independent Hidden visibility are code-complete under ADR 0086; ADR 0117 retired Inbox as a visible state; at the Study cutover [ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md) merged the Reading page into My Books and made finishing a Book record history only.

The Reading page, the Current-reading vocabulary snapshot, Reserved vocabulary and the Start/Switch/End/Finish confirmations are retired. There is no Known vocabulary: finishing a Book changes no vocabulary, and the Current reading only puts its words first in [Study](study.md) until it reaches Target coverage.

## Goal

Support an explicit, reversible path from catalog discovery to reading without
turning a set of intended books into an ordered plan. Reading records each
Book's workflow disposition, pins the Current reading (at most one per study
language) and shows the others in neutral title order.

## Shipped workflow

1. Catalog sync maintains each connection's **Catalog index**; it never creates
   a Book or Reading membership. A fresh learner's Reading stays empty until
   they choose an entry.
2. Add on the Catalog, including the Classical catalog, creates or restores the
   Book as **To Read** and starts the same download and preparation as moving a
   Book to To Read. Analysis may be queued, running, ready, stale, failed, or
   unavailable; evidence status does not create a recommendation or order.
3. Reading, one page per study language, opens with the **Current reading** hero
   at the top with its progress toward Target coverage (Mature coverage,
   in-study coverage and roughly how many words are left to introduce), then
   **To Read**, then **Read**. The Current reading is also listed under To Read,
   labelled Currently reading. To Read is in title order. The optional **most
   readable** sort orders it by Mature coverage; the choice is kept in the
   address (`?sort=readable`), and the Read tab never takes it.
4. **Read next** is on every To Read Book that has a chosen Book language and a
   current analysis. One action starts a reading or, when another Book is
   current, switches to this one and leaves the former Book in To Read. It
   takes no confirmation and freezes nothing. It is refused, with its reason,
   when the Book is not To Read, not analysed or has no Book language, and a
   Book that is already current says so. **End** ends the Current reading
   without a completion and leaves the Book in To Read. **Finish** appends a
   Reading completion, clears the current role and the To Read disposition, and
   changes no vocabulary. None of the three takes a confirmation token. Reading
   never ends from inactivity.
5. Reading shows each Book in exactly one visible workflow bucket. In
   precedence order, that is **Currently reading**, **To Read**, then **Read**
   when completion history exists. A Book with reading history appears in Read
   without changing its persisted disposition or completion provenance. A
   finished Book is not To Read underneath and projects as Read through its
   history. **Read again** moves a Read Book to To Read without starting it or
   erasing history. A **Previously read** assertion is an idempotent
   assertion-time fact. From the Catalog, **I've read this** creates or
   restores the Book as Read; Reading offers **Mark as previously read** only
   on an unstarted To Read Book and moves that Book to Read. Finishing claims
   no mastery and has no vocabulary effect.
6. A Book that needs action shows a **status line** on its entry: a failed
   acquisition or preparation, a failed or out-of-date analysis, an Analysis
   update or a main-text selection update available, or a missing Book
   language. A **needs action** filter (`?needs=…`) lists only those Books; a
   Book with nothing to do has no status line.
7. Every Book in Reading has a **Book detail page** at `/books/{id}`. It hosts
   the Book's analysis actions (retry, Analysis update and Source structure
   update), shows its Book language, holds its **Reading position** section
   (below), and lists the Book's words in a read-only list of the Book's words by Study state
   ([Vocabulary Browse and Concordance](vocabulary-browse-and-concordance.md)).
   `/reading?reading={bookID}`, an address of the retired working desk, goes to
   that Book's detail page, and the former `/library` address redirects to Reading.
8. **Remove from Reading** is the opposite of Add: it ends membership without
   erasing the Book, its evidence or history, and the Current reading is
   refused with a reason. It never costs Study progress. Read history is
   append-only.

The current product and architecture decisions are [ADR
0078](../adr/0078-book-dispositions-and-current-reading.md) as amended by
[ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md).
ADR 0072 records the superseded Goal/ordered-Journey model as historical
context.

## Reading position

A Book in Reading has a **Reading position**: the learner's latest known place
in it, a fraction of the whole Book. Each entry is a **Position report**, and
the Reading position is the latest report by recorded time, with insertion order
breaking a tie. A Book with no report shows **No position yet**, which is not
0%. Terms are defined in [`CONTEXT.md`](../../CONTEXT.md).

- **Entry.** On Book detail, any Book in Reading can take a position by
  percentage, whatever its disposition, analysis or Book language. The
  percentage is from 0 to 100 with at most one decimal place. Entry is a
  CSRF-protected `POST /library/books/{id}/position` that redirects back to Book
  detail with a message. A refusal (nothing entered, not a number, outside 0 to
  100) redirects with its reason and keeps the input in the form. A Book not in
  Reading, or another learner's, is not found. The time recorded is when the
  learner entered it; there is no backdating.
- **Display.** The percentage is whole and rounded down, so 99.6% never reads as
  100%. The section shows the date entered (a `<time>` element, the same date
  format as "Reading since") and "entered by you". A lower entry than before is
  allowed and becomes the position.
- **A signal only.** Recording a position never finishes, starts or ends a
  reading and never changes a disposition or the Current reading, and 100% does
  not finish a Book. Finish, End, Read next, Read again and Remove leave the
  reports unchanged; reports belong to the Book, so they return if it is added
  back.
- **Stored per Book**, owner-scoped, never per language or per reading, and
  every report is kept.

Page entry with a remembered page count, undo, the Reading list percentage and
the Current reading hero follow in the same spec.

## Interaction and recovery contract

- Reading has no learner-authored sequence, ranking, forecast, or suggested
  next Book. The most readable sort is optional, and coverage describes current
  evidence only.
- Server-rendered links, forms, labels, and status text remain usable without
  enhancement. Mutation outcomes preserve a useful route and announce feedback;
  focus and keyboard order follow document order.
- Empty To Read, failed or stale analysis, unsupported-language, and recovery
  states retain a clear path back to Reading, Catalog, or the relevant retry.
- The Book detail word list keeps its dependency states distinct and never
  turns one into a successful zero: a Book with no completed analysis lists no
  words and points to its analysis actions; **Updating vocabulary counts** and
  **Vocabulary counts unavailable** (bounded retries exhausted) both offer a
  plain **Refresh status** link that only re-checks and never requeues work or
  reruns analysis; a no-match prefix is retained with **Clear prefix**.
- Compact and desktop layouts retain Book identity, status, and primary actions
  without page-level horizontal scrolling. Reduced-motion preferences are
  respected.

## Acceptance coverage

Playwright browser smoke covers the catalog-to-My-Books path, disposition
controls, Read next, Finish and End, Read history and rereading, the Book detail
page, language-change recovery, recovery states, keyboard/focus semantics, and
compact/desktop layouts. PostgreSQL integration tests cover disposition and
completion/history provenance. See
[`../design/screen-inventory.md`](../design/screen-inventory.md) for current
screen responsibilities and [`../../e2e/README.md`](../../e2e/README.md) for
running browser acceptance.

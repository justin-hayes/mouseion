# Classical Books are whole Works

Status: **Accepted specification — implementation pending** · Date: 2026-11-24 ·
Author: Justin + OpenCode

Decided in a grilling session on 2026-11-24. The decision is recorded in
[ADR 0128: A classical Book is a whole Work in one Edition](../adr/0128-a-classical-book-is-a-whole-work.md),
which amends [ADR 0116](../adr/0116-classical-catalog.md) and
[ADR 0117](../adr/0117-one-catalog-my-books-holds-chosen-books.md).

## Problem

The Classical catalog ships *Anabasis* as seven Reading units and *Republic* as
ten, so a learner who adds either Work owns seven or ten Books. My Books and the
Reading chooser group them, but every other surface treats each constituent
book as a separate Book:

- Reading has one current Book, so the learner reads, finishes and prepares a
  deck for *Anabasis* Book 3, not *Anabasis*;
- each constituent book has its own prepared deck, Browse inventory, coverage
  and analysis insights;
- Concordance lists *Anabasis* up to seven times for one lemma, each run with
  its own "N occurrences on this page";
- the Catalog's Work page asks the learner to choose books, with an Add and an
  I've read this per book.

The learner thinks of the Work as the unit: all of *Anabasis* is under study,
and they want one deck, one vocabulary inventory and one coverage figure for it.

## Learner outcome

A learner adds *Anabasis* once and from then on sees one Book everywhere: one
My Books row, one chooser entry, one current reading, one completion, one deck,
one Browse inventory and one Concordance group. The constituent books show up
only as locations: citations such as `3.2.1` on occurrences, and a contents
list on the Catalog's Work page.

## Scope

In scope:

- whole-work Reading-unit definitions for every Work whose citation scheme
  names books (today *Anabasis* and *Republic*), and withdrawal of their
  per-book definitions;
- retiring the manifest's `constituent-books` split kind, so the generator emits
  one whole-work unit per Edition and records each constituent book's citation
  range and word count for the contents list;
- the surface changes below;
- the design-document updates listed under Documentation.

Out of scope:

- converting existing per-book Books into whole-work Books (ADR 0128);
- a reading position, a table of contents on Reading, or any "where am I"
  tracking;
- filtering Browse, Concordance or a deck by constituent book;
- OPDS series or multi-volume EPUBs.

## Terms

`CONTEXT.md` holds the definitions. This spec relies on **Work**, **Edition**,
**Reading unit**, **Constituent book**, **Citation**, **Book** and **Withdrawn
definition**. A Constituent book is a division inside a Book and never a Book.
On screen it is named with its Work, "*Anabasis*, Book 3", and only where it is
a location in the text, never as something the learner owns or acts on.

## Definitions and existing Books

- Each split Work gets one new whole-work Reading-unit definition per Edition,
  under a new permanent key (for example `xenophon-anabasis`). Its citation
  range spans the whole Edition. ADR 0116's ownership rule still applies: a
  sentence belongs to the unit that holds its first token, so a whole-work unit
  never refuses a sentence that crosses a constituent-book boundary.
- The per-book definitions (`xenophon-anabasis-1`…`7`,
  `plato-republic-1`…`10`) become withdrawn definitions. They are hidden from
  the Catalog and cannot be added or prepared.
- A learner's existing Books on withdrawn definitions are left as they are.
  They stay readable with their analysis, decks, decisions and history, and
  appear in My Books as ordinary rows, one per Book, with no grouping. The
  learner can remove them. A current reading on one of them stays current until
  the learner finishes or ends it.
- Adding the whole Work creates a new Book. Nothing from the per-book Books
  carries over, including Lemma corrections and Occurrence exclusions.

## Surfaces

### Catalog Work page

- One **Add** and one **I've read this** per Edition. Adding queues one
  Classical preparation for the whole Work.
- The "Choose books" table, the per-book Add and I've read this, and the
  "Add and prepare the remaining books" label are removed.
- "In My Books" and the Add check look at the whole-work Reading unit, not the
  Work key. A learner who owns only withdrawn per-book Books of *Anabasis* can
  still add the whole Work.
- A read-only contents list names each constituent book with its word count.
  It has no actions and shows no preparation state.
- The index's length strip and word count describe the whole Work.

### My Books

- A whole-work Book is an ordinary row. The grouped Work row, its Book-order
  strip and cells, "N Books in M works", **Move the work to To Read**,
  **Remove the work**, the grouped Analysis update notice and bulk action, and
  the collapsed preparing line are removed.
- Two Editions of one Work are two ordinary rows, told apart by their Edition
  labels.
- Search no longer matches a trailing Book number.

### Reading

- The chooser lists one entry per Book. The grouped Work entry and its band
  table are removed; a whole-work Book has an ordinary margin band.
- Reading shows no "Book N of M" line for a whole-work Book.
- Start, Switch, finish, the completion receipt and the prepared deck apply to
  the whole Work.

### Concordance

- A whole-work Book forms one group with one occurrence count.
- Each classical occurrence shows its Citation as its location, in place of a
  chapter title or "sentence N".

### Sentence study and Vocabulary Browse

- Both are Book-scoped already and need no structural change.
- Wherever they show a classical occurrence's location, they show its Citation.

### Analysis update

- A whole-work Book gets the ordinary per-Book notice. The grouped Work
  notice, bulk action and confirmation are removed.

## Documentation

When the change ships, update the shipped-behavior documents to match:
`doc/design/screen-inventory.md` (My Books, Between-Books chooser),
`doc/design/components.md` (`AnnotatedBookRow`, `ReadingChooser`, grouped
Analysis update), `doc/design/terminology.md` (Move to To Read, Coverage band,
I've read this), `doc/design/information-architecture.md` and
`doc/features/collection-browsing.md`. Then mark ADR 0128 and this spec
implemented in `doc/product.md`.

## Acceptance matrix

| # | Behavior | Verified by |
|---|---|---|
| W1 | The manifest has one whole-work definition per Edition of *Anabasis* and *Republic*, with a new key and a citation range covering the Edition, plus each constituent book's citation range and word count | generator + manifest validation test |
| W2 | The per-book definitions are withdrawn: hidden from the Catalog and refused by Add and preparation | integration |
| W3 | Startup reconciliation accepts the change without touching learner state; existing per-book Books stay readable | integration |
| W4 | The generator no longer accepts the `constituent-books` split kind | unit |
| W5 | The Work page shows one Add and one I've read this per Edition, and a contents list with no actions | handler/template test + browser smoke |
| W6 | Adding a whole Work creates one Book and queues one Classical preparation, which publishes one analysis for the whole Work | integration (complete classical loop) |
| W7 | My Books shows a whole-work Book, and each leftover per-book Book, as an ordinary row with no grouping or Work actions | handler/template test |
| W8 | The chooser shows one ordinary entry per Book; Reading shows no "Book N of M" | handler/template test |
| W9 | Concordance shows one group and one count for a whole-work Book, and a Citation location on each classical occurrence | handler/template test + browser smoke |
| W10 | Sentence study and Browse show a Citation wherever they show a classical occurrence's location | handler/template test |
| W11 | A whole-work Book's Analysis update offer is the ordinary per-Book notice | handler test |
| W12 | The grouped Work code paths (grouped row, Work handlers and routes, chooser Work entry, Work Analysis update) are removed, not left unreachable | review |

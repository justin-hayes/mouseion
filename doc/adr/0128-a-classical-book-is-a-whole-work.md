# ADR 0128: A classical Book is a whole Work in one Edition

Status: **Accepted; implemented (#2634)** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in a grilling session on 2026-11-24. Amends
[ADR 0116](0116-classical-catalog.md) and
[ADR 0117](0117-one-catalog-my-books-holds-chosen-books.md). The feature is
specified in [Classical Books are whole Works](../features/classical-whole-work-books.md).

## Context

ADR 0116 made a classical **Reading unit** one **Constituent book** whenever the
pinned Edition's citation scheme names books, so the learner gets one Book per
constituent book: *Anabasis* is seven Books and *Republic* ten. To keep a long
Work from filling My Books and the Reading chooser with one row per book,
ADR 0116 and ADR 0117 added Work-level display rules on top: a grouped Work row
with bulk actions in My Books, a band table in the chooser, a grouped Analysis
update, and a refusal to ever name "the next Book". It rejected a Work-level
state because that would duplicate per-Book disposition and invite "next Book"
recommendations.

That grouping reaches only three screens. Reading, prepared decks, Vocabulary
Browse, Concordance, analysis insights and Sentence study all treat each
constituent book as its own Book. The learner thinks of *Anabasis* as one text:
it is under study as a whole, and a deck, a vocabulary inventory or a coverage
figure is wanted for the whole Work. Concordance shows the gap most clearly: one
lemma lists *Anabasis* up to seven times, each run with its own "N occurrences
on this page".

## Decision

**A classical Book is a whole Work in one Edition.** For a Work whose citation
scheme names books, the Reading unit is the whole Work in that Edition, and
the Constituent books become a citation level inside it, like chapters inside
an EPUB. Every surface that already handles one Book — disposition, current
reading, completion, analysis, coverage, prepared deck, Browse, Concordance,
Sentence study — therefore handles the whole Work without new Work-level rules.
Two Editions of one Work stay two Books.

A learner never holds a Book for one constituent book. Focusing on part of a
Work is navigation or filtering inside the Book; it never creates a second
Book. No such filter is built now.

Where an occurrence's location is shown, a classical occurrence shows its
**Citation** (`3.2.1`), so the constituent book stays visible without grouping.

The Work-level display rules are retired, not kept for the rare case of two
Editions: the grouped Work row and its bulk actions, the chooser band table,
the grouped Analysis update notice, the per-unit "Choose books" table and
"I've read this" per Reading unit in the Catalog, and "Book N of M" on Reading.
The Catalog's Work page offers one Add and one **I've read this** per Edition,
with a read-only contents list of the constituent books and their word counts.
"Previously read" therefore covers a whole Work or nothing.

The per-constituent-book definitions (`xenophon-anabasis-1`…`7`,
`plato-republic-1`…`10`) become withdrawn definitions, replaced by one new
whole-work definition per Work under a new key, as ADR 0116 requires for a
changed extent. Existing Books on the withdrawn definitions are not converted:
they stay readable as ADR 0116's withdrawn tombstones, and the learner adds the
whole Work afresh and removes them when ready. Decisions on them do not carry
over, because ADR 0127 carries decisions only within the same extracted-unit
snapshot.

## Considered options

- **Keep one Book per constituent book and aggregate per Work on every
  surface.** Rejected: every screen would need a Work view next to its Book
  view, and "Book" would mean two different things to the learner. This is the
  Work-level state ADR 0116 rejected, reached screen by screen.
- **Offer both whole-work and per-book units.** Rejected: ADR 0116 forbids
  overlapping units, overlapping Books would count the same text twice in
  Personal analyzed-Book occurrences, and the learner does not want the
  per-book form.
- **Convert existing per-book Books into one whole-work Book.** Rejected for
  now: merging dispositions, completions, current-reading snapshots, decks and
  occurrence decisions across seven Books costs far more than re-adding a Work
  on a single-learner installation. It can be reopened for a specific kind of
  state if one turns out to matter.

## Consequences

- ADR 0116's "a Reading unit is one Constituent book when the citation scheme
  names books", its "A Book's place in its Work is a fact, never a pointer"
  section, its grouped Analysis update notice and its grouped `ReadingChooser`
  contract are superseded. Its "never names the next Book" rule holds
  trivially.
- ADR 0117's "Adding a whole Work queues one preparation per Reading unit",
  "I've read this … per Reading unit only" and "Remove the work" are
  superseded: a Work is one Book, with one preparation, one I've read this and
  the ordinary Remove.
- A whole-work Book is up to ten times larger than a constituent-book Book
  (*Republic* is about 89,000 words), so one Classical preparation, one deck
  and one Browse inventory cover that much more text. The deck's size is
  governed by the existing selection parameters, not by splitting the text.
- Future Works divided into books, such as the *Iliad*, ship as one whole-work
  Reading unit per Edition; the manifest's `constituent-books` split kind is
  retired.

# ADR 0136: Study replaces Known vocabulary and the Prepared deck

Status: **Proposed** · Date: 2026-11-26 · Author: Justin + OpenCode

Decided by Justin in a grilling session on future product direction. Accepted
by merging this ADR's pull request.

It would amend or retire parts of:

- [ADR 0036](0036-primary-goal-justified-graduation.md) and
  [ADR 0078](0078-book-dispositions-and-current-reading.md): Known vocabulary,
  graduation and the current-reading snapshot;
- [ADR 0085](0085-reading-owned-book-vocabulary.md),
  [ADR 0086](0086-reading-working-desk-hidden-visibility-and-concordance.md) and
  [ADR 0090](0090-library-total-book-vocabulary-selection.md): Reading-owned
  Book vocabulary, the working desk and deck selection;
- [ADR 0081](0081-learner-owned-occurrence-lemma-corrections.md): Lemma review
  as a Book-scoped queue;
- [ADR 0114](0114-idiom-cards-from-dictionary-evidence.md): Idiom cards in
  Prepared decks;
- [ADR 0117](0117-one-catalog-my-books-holds-chosen-books.md): My Books and
  Reading as separate pages;
- [ADR 0134](0134-study-in-mouseion-vocabulary-review.md): Study's
  separation from Known, its supported card policies and its new-word order;
- [ADR 0135](0135-study-reports.md) §2: where a Wrong-word report is
  repaired.

## Context

The learner has two related goals: to read particular works in a language, and
to build general comprehension of that language. Mouseion serves them with two
vocabulary models.

1. **Known vocabulary and the Prepared deck.** Starting a Current reading
   freezes a snapshot of Book vocabulary candidates, reserves it, and prepares
   an Anki deck from it. Finishing the Book accepts the snapshot into Known
   vocabulary. Much of the glossary exists to keep this sound: Reserved and
   Generated vocabulary, Omissions, Graduation, the exact-commitment
   confirmations for Start, Switch, End and Finish, and the Lemma review flags
   that block a snapshot before it is frozen.
2. **Study** (ADR 0134). It is in-Mouseion review, owned per word rather than
   per Book. The Current reading only decides which new words come first, and
   Study never consults Known vocabulary.

Study is the better model of what the learner knows. Its maturity comes from
graded reviews, whereas Known vocabulary comes from the claim that a Book was
read. Keeping both doubles every vocabulary surface and ties word knowledge to
a Book's lifecycle. Study also reveals analysis errors as the learner meets
them (ADR 0135), which makes a separate, proactive review queue unnecessary.

## Decision

### 1. Study is the only vocabulary workflow

- Retire the Prepared deck and everything that serves it: the Deck
  specification, Card presentation, every Omission kind, Generated vocabulary,
  Reserved vocabulary, the Current-reading vocabulary snapshot, Book vocabulary
  candidates, the Vocabulary transition and Graduation. There is no Anki
  export.
- Retire Known vocabulary. Knowledge is Study progress, and the one claim of
  knowledge is **Mature**.
- **Already known** is an answer the learner can give when Study introduces a
  word. The word enters study with an interval of at least 21 days, so it is
  Mature at once. After that it is an ordinary word: a lapse ends its
  maturity. It is an answer, not a status, and nothing else marks a word as
  known.

### 2. The Corpus, and how new words are ordered

- The **Corpus** of a study language is the analysed Books in My Books in that
  language. Study draws Study material and Fluency contexts from all of it,
  Read Books included, and Concordance searches it. Adding a Book from the
  Catalog is how the learner includes it in the Corpus. There is no separate
  switch.
- Only **To Read** Books set the priority of new words.
- The Current reading's words come first until its coverage reaches a fixed
  **target coverage** of 98%, counting Mature and in-study words. After that,
  frequency across the To Read Books decides. The Book gets the boost again
  only if its coverage falls below the target. The target is the same for every
  language until reading shows that one value does not fit.
- Removing a Book never costs the learner Study progress. A word keeps its
  progress and its Study material, because Remove keeps the Book's evidence.
  It gets no further Fluency contexts until another Book in the Corpus has one.

### 3. Coverage counts Mature words

A Book's coverage is the share of its analyzable tokens whose vocabulary
identity is Mature. Coverage that also counts in-study words is shown as a
second, lighter figure. The denominator rules of the Vocabulary coverage band
are unchanged: confirmed Lemma corrections apply, and excluded occurrences
stay in the total. Coverage counts word tokens only, so a Mature Idiom does
not change it. The Catalog shows no coverage for entries that are not yet in
My Books.

### 4. Finishing a Book records history only

Reading completion ends the Current reading and clears To Read. It has no
vocabulary effect. A Reading completion and a Previously read assertion differ
only in whether the reading happened inside Mouseion.

### 5. One My Books page per study language

- Reading stops being a page of its own. My Books shows the Current reading
  pinned at the top, then To Read, then Read. "Current reading" stays as a role
  a Book has.
- The pinned Book shows its progress toward the target coverage: Mature
  coverage, in-study coverage, and roughly how many words are left to
  introduce.
- **Read next** starts or switches the Current reading in one action, with no
  confirmation step. It is refused only when the Book is not analysed or has no
  Book language. **Finish** and **End** remain. The exact-commitment tokens and
  replay rules for these actions are retired with the snapshot.
- To Read is in neutral title order by default, with an optional **most
  readable** sort by coverage. The same choice applies wherever a Book is
  chosen to read next.
- Book-level problems appear as a status line on the Book's entry, with a
  filter that shows only Books needing action. These problems are: failed
  acquisition or analysis, stale analysis, a pending Analysis update, a missing
  Book language, and main-text selection.
- Book-scoped vocabulary Browse moves to the Book detail page, for any
  analysed Book. It is read-only and shows each word as Mature, in study or not
  yet introduced.

### 6. Fixes start only from a Study report

- Fixing a wrong vocabulary identity starts only from a Wrong-word report. The
  report opens a remediation screen with the existing preview-and-confirm step
  for a Lemma correction or Occurrence exclusion. No other surface edits the
  results of analysis or enrichment.
- A Lemma correction stays tied to one occurrence. The remediation screen
  lists other occurrences in the Corpus with the same surface and source
  lemma, and the learner ticks which ones to correct too. Nothing becomes a
  language-wide rule.
- Retire Lemma review flags, Lemma suggestions, the lemma-risk assessment and
  the Book-scoped Lemma review queue. Dictionary-evidence risk may survive only
  as a hidden signal that makes Study material prefer occurrences without that
  risk, and only if that is cheap.

### 7. The two goals are separated by surface, not by a mode

The Current reading and My Books serve reading particular works. Today,
Fluency, Concordance and Sentence study serve general comprehension. The two
goals are blended in one place only: the order of new words (§2). No mode
switch exists.

### 8. One cutover

Known vocabulary and the Prepared deck retire together, in one cutover, once
Justin judges that Study is out of pilot. The cutover waits until Study:

- supports the source-evidence card policy (Ancient Greek and Latin);
- introduces and reviews Idioms as items in their own right, each with its
  own Study progress and Learned meaning.

The cutover migrates every Known identity to "Already known" Study progress,
with first reviews spread over several weeks. Words already in study keep their
own progress. The merged My Books page (§5) ships in the same cutover or right
after it.

## Considered options

- **Keep an Anki export.** This was rejected because any export keeps the
  frozen snapshot, Reserved vocabulary and the Omission terms alive, and those
  are what tie vocabulary to a Book's lifecycle.
- **Keep Known vocabulary beside Study.** This was rejected because the two
  facts would disagree, and every surface would have to choose between them.
- **Learner asserts prior knowledge without checking.** A bulk "mark as known",
  or skipping known words, was rejected. "Already known" keeps knowledge as one
  fact and still has later reviews confirm the claim.
- **Current-reading words strictly first.** This was rejected because it keeps
  spending batches on rare words in one Book. A blended weighted score was
  rejected because it has no clear meaning. A target coverage gives the boost a
  stopping point.
- **Language-wide lemma rules.** This was rejected because the same surface
  and source lemma can be right in one sentence and wrong in another (`kam`
  without `umhin` really is `kommen`).
- **Coverage-ranked My Books by default.** This was rejected because it would
  quietly recommend a Book. The most-readable sort stays optional.
- **Cut over one language at a time.** This was rejected because the code
  would run both models side by side for as long as the slowest language
  takes.

## Consequences

- **Large retirement.** Many `CONTEXT.md` terms become retired, marked the way
  Inbox is, and the Prepared-deck, snapshot and Lemma review code is removed.
  Those terms change only after this ADR is accepted.
- **New terms.** Corpus, Already known and target coverage are added to
  `CONTEXT.md`.
- **Migrations need human review.** The Known-to-Study migration and dropping
  the Known, snapshot and reservation tables go through `migrations/`.
- **Study must grow first.** Source-evidence card policy support and Idioms in
  Study are prerequisites. Until then, Ancient Greek and Latin keep Anki.
- **ADR 0135 §2 changes venue.** A Wrong-word report is repaired on the
  remediation screen, not in the Book's Lemma review. It is still one
  preview-and-confirm workflow, not a second copy.

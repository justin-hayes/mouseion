# ADR 0082: Independent saved Custom decks and frozen preparations

Status: **Superseded by [ADR 0085](0085-reading-owned-book-vocabulary.md)** · Date: 2026-10-01 · Author: Justin + OpenCode

Historical decision: substantial portions were implemented before the
Reading-centered cutover was accepted. Do not treat the independent selection,
preparation, or learner-facing Custom deck requirements below as a current
implementation target. Existing stored artifacts are retained pending a
separate, audited cleanup under ADR 0085.

Records the durable boundary agreed in [Specify Vocabulary browse, custom
decks, and Concordance](https://github.com/justin-hayes/mouseion/issues/1330),
especially [Define the custom deck relationship to Reading and prepared
decks](https://github.com/justin-hayes/mouseion/issues/1336) and
[Decide whether Vocabulary needs an ADR alongside its feature
spec](https://github.com/justin-hayes/mouseion/issues/1343). This does not
amend the Book prepared-deck selection or Reading snapshot policy.

## Context

Book Prepared decks derive recurring vocabulary from one Book's exact
analysis; Reading independently freezes one current Book's vocabulary
snapshot, reserves it, and accepts eligible identities as Known on completion
([ADR 0078](0078-book-dispositions-and-current-reading.md)). A learner also
wants to choose identities across currently analyzed Books for Anki study,
including singletons, Known and Reserved identities. Treating that choice as
another Book deck or current reading would silently inherit an unrelated
frequency floor or learner-state transition. Treating it as a one-off export
would lose the learner's intended selection whenever corpus evidence changed.

## Decision

A **Custom deck** is an owner- and study-language-scoped, learner-named saved,
editable set of effective vocabulary identities selected through Vocabulary
Browse. It is neither a Book Prepared deck nor a current-reading snapshot,
and is not owned by a particular Book. The selection retains identities as
Books and occurrence evidence change; losing all current evidence does not
erase a selection or reassign it to a different identity. Multiple saved decks
are possible. Saving, editing, preparing, downloading, or deleting a Custom
deck never starts/finishes Reading, alters a current-reading snapshot or Book
deck, or changes Known, Reserved, or Generated vocabulary state.

On **explicit** preparation, freeze a separate exact specification from the
saved selection and currently eligible cross-Book evidence. Subsequent edits
or evidence changes affect future preparations only, not queued, ready, or
historical generations. Keep prior generations as provenance; a replacement
does not withdraw the last Ready artifact while it runs or if it fails. Once
the replacement succeeds, only that newest Ready preparation is downloadable
through Mouseion. Presentation-only deck revisions remain revisions of the
*same* frozen specification under [ADR 0071](0071-decouple-deck-data-from-presentation.md),
not new selections. Delete withdraws future Mouseion downloads, not APKGs
already imported or saved outside Mouseion. A deck whose language leaves the
derived study-language set is retained as read-only history until that language
returns, not moved to another language.

The first persistence slice stores the unnamed Browse selection as owner,
language, canonical lemma, and UPOS rows with a composite identity key. Saved
Custom decks have an owner-scoped UUID, language, learner name, and a separate
identity relation with the same effective-identity key. Neither relation has a
foreign key to a Book, analysis, or occurrence: losing evidence therefore does
not delete or transfer learner intent. Owner foreign keys cascade on account
deletion; deleting a deck cascades only its identity rows. A unique owner-scoped
creation key makes a retried explicit naming action resolve to its original
deck. Creation and copying the current language's selection into the deck, then
clearing only that selection, are one transaction serialized with selection
add/remove/clear for the same owner and language. The schema is additive; its
down migration removes only these new tables and is not a production rollback
plan.

The [feature specification](../features/vocabulary-browse-concordance-and-custom-decks.md)
owns detailed eligible evidence, cross-Book sentence choice, omissions,
recovery, UI, accessibility, export/privacy, Anki overlap, and acceptance
requirements. Existing ADRs continue to own the normalized corpus
([ADR 0059](0059-persisted-normalized-corpus-for-concordance.md)), parsed
syntax ([ADR 0060](0060-persist-dependency-parses.md)), owner-scoped
occurrence correction ([ADR 0081](0081-learner-owned-occurrence-lemma-corrections.md)),
Reading's snapshot ([ADR 0078](0078-book-dispositions-and-current-reading.md)),
frozen deck data ([ADR 0071](0071-decouple-deck-data-from-presentation.md)),
and Anki note identity ([ADR 0006](0006-anki-export-import-contracts.md),
[ADR 0020](0020-anki-package-output.md)). This decision does not redefine
their keys or imply that Anki isolates copies of the same note in different
decks. If implementation needs to change an existing key, reconcile that ADR
explicitly first.

## Why this trade-off

A saved identity selection respects learner intention without treating a
particular occurrence, current analysis, or Reading commitment as its owner.
Freezing each preparation gives asynchronous work, retries, and imported Anki
artifacts stable inputs without preventing later edits or recovery from changed
evidence. The cost is a distinct durable selection and preparation lifecycle,
plus explicit disclosure when selected identities are temporarily unevidenced
or an older Ready download remains available.

## Alternatives considered

- **Reuse Book prepared decks or their recurring-vocabulary pool.** Rejected:
  would tie cross-Book identity choices to one Book, its recurrence threshold,
  and its current analysis rather than the learner's explicit shortlist.
- **Tie selection to the current reading and its reserved snapshot.** Rejected:
  would change Reading's frozen commitment, conflate study artifacts with
  modeled knowledge, and make a cross-Book shortlist depend on reading state.
- **One-off, unsaved export from current Browse results.** Rejected: changing
  filters, evidence, or analyses would lose explicit selections and give the
  learner no stable place to edit or deliberately prepare again.
- **Live preparation that tracks edits or reanalysis in place.** Rejected:
  would make retries, artifact provenance, and Anki re-import results
  indeterminate. Explicit new generations keep historical evidence intact.

## Consequences

- Custom-deck selection and preparation need their own durable, owner-scoped
  lifecycle and acceptance checks; the existing single-Book path is not a
  substitute. Consequential schema shape and rollback require human review
  under [ADR 0038](0038-schema-change-governance.md); this ADR does not
  prescribe tables or edit the immutable baseline migration.
- Vocabulary can support cross-Book study without adding a fifth destination
  or silently modifying Reading, Book decks, historical completions, or Anki
  collections.

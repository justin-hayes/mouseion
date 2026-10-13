# ADR 0092: Retire known-vocabulary import and delete imported Known vocabulary

Status: **Accepted (implementation pending)** · Date: 2026-10-13 · Author: Justin + OpenCode

Tracked by [issue #2006](https://github.com/justin-hayes/mouseion/issues/2006). Supersedes the known-vocabulary import contract of [ADR 0006](0006-anki-export-import-contracts.md) §3 and the import destination of [ADR 0043](0043-study-languages-derived-settings-removed.md).

## Context

Known vocabulary has two sources: a lemma file the learner uploads at `/vocabulary/import`, and Reading completion, which accepts a finished current reading's frozen snapshot. Known identities never enter a new current-reading snapshot, so imported lemmas never reach a Prepared deck.

The one production import (2026-09-16, German) added 5,020 lemma-wide entries, which count as Known for every part of speech. Those words are now missing from every Book deck, although the learner wants cards for them. An uploaded list is a claim the learner made once, without evidence from their reading. Reading completion grounds Known vocabulary in Books the learner finished, with provenance for each row.

Imported rows are identifiable: every row accepted by Reading completion records its completion provenance (`completion_book_id` and related columns), and every imported row leaves it empty. In production, every row without provenance is lemma-wide and from that single import. No row carried over from the legacy `vocabulary_states` model lacks provenance.

## Decision

1. **Known vocabulary comes only from Reading completion.** No learner-facing or operator path adds Known vocabulary any other way. Deck generation still never marks vocabulary Known ([ADR 0036](0036-primary-goal-justified-graduation.md)).
2. **Retire the import surface.** `GET`/`POST /vocabulary/import` and `/vocabulary/imports/{id}/status` return 404 without a redirect, as `/known-vocab` does. Remove the Vocabulary link to them, the `import_known_vocabulary` River worker, and its package. `/vocabulary` remains Concordance. With a single view left, the Vocabulary peer-view navigation is removed.
3. **Delete imported Known vocabulary in one data-only migration.** The migration has no DDL. It first discards queued or running `import_known_vocabulary` jobs, so none can write after the deletion, then deletes every `known_vocabulary` row without Reading-completion provenance. It is idempotent: a rerun finds nothing to delete. The down migration does not restore rows. Recovery is restoring the pre-migration backup.
4. **Snapshots and history are not rewritten.** An active current reading keeps its frozen snapshot. The released words become eligible at the next reading start or reread. Import records in processing history and each Reading completion's already-known count remain historical facts.

## Consequences

- At the next reading start, previously imported words that recur enough in the Book qualify for its snapshot and Prepared deck. Book decks for the German library grow accordingly.
- Vocabulary coverage bands, Browse Known labels, and Concordance Known state drop for the deleted identities. They now reflect only finished Books.
- Lemma-wide (POS-less) Known entries no longer arise. Readers of Known vocabulary still accept them, so the wildcard match can be removed later without a data change.
- A learner cannot seed Known vocabulary from outside Mouseion. Words learned elsewhere become Known only by finishing a Book that contains them. Mouseion is effectively a single-learner deployment; anyone who wants an import again needs a new decision.
- Playwright scenarios that seeded Known vocabulary through the upload form must seed it from fixture-server data instead.

## Alternatives considered

- **Keep import, delete only the existing rows.** Rejected: the next upload would hide the same words from decks again.
- **Keep the rows and stop excluding imported vocabulary from snapshots.** Rejected: "Known" would mean two different things, and coverage, Browse, and Concordance would still count words that decks treat as unknown.
- **A learner-facing "forget imported vocabulary" action.** Rejected: an ongoing surface for a one-time cleanup.
- **An operator script instead of a migration.** Rejected: unversioned, and not applied to other installs.
- **Re-freeze active snapshots to include the released words.** Rejected: snapshots are immutable by definition ([ADR 0085](0085-reading-owned-book-vocabulary.md)). Ending and restarting the reading gives the same result explicitly.

## References

- [ADR 0006: Anki export and known-vocabulary import contracts](0006-anki-export-import-contracts.md)
- [ADR 0038: Schema change governance](0038-schema-change-governance.md)
- [ADR 0043: Study languages are derived from the library and Settings is removed](0043-study-languages-derived-settings-removed.md)
- [ADR 0085: Reading owns Book vocabulary and deck preparation](0085-reading-owned-book-vocabulary.md)
- [ADR 0086: Reading Working desk, independent Hidden visibility, and corpus-wide Concordance](0086-reading-working-desk-hidden-visibility-and-concordance.md)

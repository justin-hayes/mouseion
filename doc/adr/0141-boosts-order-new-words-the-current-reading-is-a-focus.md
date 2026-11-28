# ADR 0141: Boosts order new words; the Current reading is a focus

Status: **Accepted** · Date: 2026-11-28 · Author: Justin + OpenCode

Decided in [#2842](https://github.com/justin-hayes/mouseion/issues/2842).
Amends [ADR 0078](0078-book-dispositions-and-current-reading.md),
[ADR 0136](0136-study-replaces-known-vocabulary-and-prepared-deck.md) §2 and
[ADR 0137](0137-idioms-are-study-items.md) §4. It settles the stored shape
before any SQL is written (ADR 0038, ADR 0070).

## Context

ADR 0136 §2 lets the Current reading's words come first in Study until the Book
reaches a fixed 98% Target coverage, and ADR 0137 §4 extends that to the Book's
Idioms. The rule is implicit: starting a Book changes the order of new words,
and the learner cannot ask for it, bound it or see when it ends. Target coverage
is a single number for every language, and it depends on the Book being read
now rather than on a choice the learner made about the words.

## Decision

1. **The Current reading has no effect on Study.** It no longer orders new
   words or Idioms, and Target coverage retires. This amends ADR 0078's use of
   the Current reading as a Study input, ADR 0136 §2 (the 98% boost and the
   target) and ADR 0137 §4 (the Current reading's Idioms first). The Current
   reading stays the learner's focus in Reading.
2. **A Boost is the only way a Book's words come first.** The learner starts a
   Boost on a Book from Book detail. While it is not complete, its remaining
   items lead the new-word stream, ranked as the Boost ranked them. Everything
   else keeps the ordering ADR 0136 and ADR 0137 define, by frequency across
   the To Read Books.
3. **Sentence preference follows the Boost.** Where Study material or an
   Idiom's sentence preferred the Current reading's Book, it prefers the
   Boosted Book instead, and has no Book preference when none is boosted.
4. **Stored shape.** A Boost belongs to one owner, one study language and one
   Book. It records its creation time and, once ended, an end time and a
   reason: removed by the learner, or the Book was removed from Reading. It
   holds a fixed, ranked list of Study items, taken when the Boost starts, by
   the identity Study progress uses (language, canonical lemma, part of speech,
   lexical discriminator, item kind). The list does not change when the Book is
   re-analysed. A Boost follows the language's idiom switch, like Study
   candidates, and includes only Idioms whose counts are ready when it starts.
5. **Completion is derived, not stored.** A Boost is complete when none of its
   items remains: each is in study by the predicate Study already uses, or is
   unpreparable (a reopenable omission counts while it is omitted), or is no
   longer offered after a correction or exclusion. No completion state or time
   is written, so a reopened omission makes its item remaining again.
6. **No backfill.** Existing Current readings start no Boost, and the first
   release has no Boosts until the learner starts one.
7. **Removing a Book from Reading ends its Boosts** with the reason above. The
   Current reading cannot be removed, so this applies to other Books.

## Consequences

- Study ordering takes the active Boosts rather than a Current-reading flag.
  Preparation targets still count pending and ready words toward the window, so
  a Boost's items are prepared first and the batch falls back to ready words
  while they wait.
- Retired with Target coverage: the Target coverage text in Reading, the
  Current-reading coverage read, and the Current-reading tie-breaks in Study.
- A migration adds the Boost tables and no data change. It is forward-only in
  production.

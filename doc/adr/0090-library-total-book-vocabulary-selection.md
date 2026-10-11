# ADR 0090: Select rarer Book vocabulary by library total

Status: **Accepted (implemented)** · Date: 2026-10-11 · Author: Justin + OpenCode

Tracked by [issue #1803](https://github.com/justin-hayes/mouseion/issues/1803). Amends the candidate rule of [ADR 0085](0085-reading-owned-book-vocabulary.md).

## Context

ADR 0085 freezes an identity into a Current-reading snapshot if it occurs at least three times in the Book, or exactly twice there and at least ten times across the learner's currently analyzed same-language Books. Ten was provisional and never measured. The rule cannot reach a word seen once in this Book that recurs heavily across the library, although such a word is often more valuable for later Books than a word that clears the in-Book floor through one Book's characters or topic.

Measured on the author's library (19 German and 15 Italian Books, each scored against the others; `tools/measure-book-vocabulary-selection.sql`), German, per Book on average:

| Rule | Extra cards over the floor | Coverage | Median occurrences in other Books |
|---|---|---|---|
| ≥3 in the Book only | 0 (~860 cards) | 84.5% | 5 (floor cards) |
| ADR 0085: exactly 2 and total ≥10 | +258 | 85.9% | 14 |
| Exactly 2 and in ≥2 other Books | +494 | 87.1% | 4 (beyond ADR 0085) |
| **≥1 in the Book and total ≥15** | **+474** (319 seen once here) | **86.3%** | **21** |

Italian follows the same pattern. No rule tried brings the average Book to 95% coverage through its deck; the most generous averages about 90%. Fluent-reading coverage accrues through Known vocabulary across finished Books, not through one larger deck.

## Decision

An eligible effective `(study language, canonical lemma, analyzed POS)` identity enters a new Current-reading snapshot if it occurs **at least three times in that Book**, or **at least once in that Book and at least fifteen times in total across the owner's currently analyzed same-language Books, including the current Book**. This replaces the exactly-twice exception and reverses ADR 0085's "a singleton is never included by a high cross-Book total". An identity absent from the Book never qualifies.

Everything else in ADR 0085 holds: contributing Books, effective counting after confirmed corrections and exclusions, Known and Reserved exclusions, the count-readiness gate, and the lemma-review impact preview sharing the freeze's eligibility function. Fifteen is a named, provisional, absolute constant with no learner-facing control. The rule applies to new freezes only; active and historical snapshots are unchanged.

## Why this trade-off

A library total is one threshold over one number the system already maintains, and it is the measure of recurrence that best predicts later encounters: at the same card budget, the total admits words about five times more frequent in other Books than a document-frequency rule ("in at least two other Books"), whose additions average four occurrences elsewhere. The single-author case that document frequency guards against is rare in practice: about three identities per German Book qualify under ADR 0085 while appearing in fewer than two other Books.

## Consequences

- Each German Book gains about 470 cards over the floor, about 320 of them seen once in this Book. Finishing the Book adds those identities to modeled Known vocabulary even though the learner met them once while reading it; they recur across the library and each had a card.
- Any unknown identity seen once or twice needs cross-Book counts, so in practice every reading start waits for every contributing Book's count projection. The existing updating and unavailable states cover it.
- A fixed total grows easier to reach as the library grows. Re-measure with the script; the planned next step is a threshold per library tokens, with a minimum library size.
- Accepted limitations: about 29% of German rescues (14% Italian) draw at least half of their other-Book occurrences from one Book, such as topic words of a history or a novel; several editions or volumes of one work count as separate Books.
- No learner-selectable "comprehensive" mode: the long tail of a hard Book runs to thousands of cards, and ADR 0085 deliberately keeps the threshold out of the learner's hands.

## Alternatives considered

- **Document frequency: exactly twice and present in at least two other Books.** Rejected: at a similar card budget it admits the least-recurrent words, and it still cannot reach words seen once.
- **Keep ADR 0085 and only retune ten.** Rejected: cannot reach words seen once.
- **A weighted recurrence score.** Deferred: more parameters than the data can yet calibrate.
- **An external frequency list.** Remains out of scope under [ADR 0018](0018-remove-dwds-frequency.md); the learner's own library defines what recurs for them.

## References

- [ADR 0048: Frequency-floor deck selection](0048-frequency-floor-deck-selection.md)
- [ADR 0085: Reading owns Book vocabulary and deck preparation](0085-reading-owned-book-vocabulary.md)
- [ADR 0087: Vocabulary counts come only from selection](0087-vocabulary-counts-come-only-from-selection.md)
- [Reading-owned Book vocabulary](../features/reading-owned-book-vocabulary.md)

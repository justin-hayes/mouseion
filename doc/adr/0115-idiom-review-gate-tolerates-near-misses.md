# ADR 0115: The idiom review gate grades severity and tolerates a few near-misses

Status: **Accepted** · Date: 2026-11-08 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2389](https://github.com/justin-hayes/mouseion/issues/2389). Amends the
rollout gate of
[ADR 0114](0114-idiom-cards-from-dictionary-evidence.md); everything else in
ADR 0114 is unchanged.

## Context

ADR 0114 enables idiom cards for a language only after an operator review
whose gate allows no mistakes: every control passes and none of 30 sampled
cards teaches a wrong meaning. Italian failed that gate five times on
2026-11-08 (#2367, #2378, #2385, #2389). Each round turned the last run's
failures into new review cases and changed the prompt. Each round fixed some
cases and broke others:

- the comparative *rispetto a* passed at `idiom-card-v2` and failed at
  `idiom-card-v3` with the reasoning setting at `low`;
- *casa isolata* got two different answers in one run, for the same sentence
  and the same dictionary senses;
- *fare le cose fatte bene* was confirmed as the idiom *fare bene* ("to do
  well") in every run, across three prompt versions and two reasoning
  settings.

The provider does not answer the same request the same way every time. A
zero-tolerance gate on such a provider mostly measures luck, and adding a case
for every failure tunes the prompt to our own examples.

The remaining failures also differ in kind from earlier ones. Earlier rounds
produced cards that teach something misleading: "front" (the noun) for the
adverbial *di fronte*, or an idiom meaning for a sentence that uses the words
literally. The latest ones are close: "to do well" for *fatte bene* ("done
well"), "detached house" for a remote *casa isolata*, "in relation to" for a
comparative *rispetto a*. A learner who reads them is not misled about the
sentence.

A grammar check was considered for *fatte bene*. The analyzed corpus stores
each token's Universal Dependencies head and relation, so a rule that the
matched words must be connected is general. It would not catch *fatte bene*,
because *bene* attaches to *fatte*. A narrower rule, that a verbal idiom's verb
is not used as a participial modifier, is also general but is not worth
building while the provider rejects accidental word pairs correctly in almost
every sampled case.

## Decision

**Each card is graded by severity.** The reviewer grades the meaning in
context of every card, in a control or in the sample, as one of:

- **correct** — the Gloss fits the sense the sentence uses;
- **near-miss** — the Gloss is close to that sense and would not mislead a
  learner about the sentence, but a careful reviewer would word it
  differently;
- **misleading** — the Gloss teaches a meaning the sentence does not have.

**The gate tolerates a few near-misses.** A language passes when the committed
report shows all of:

- every positive case is confirmed with a citation-form Gloss;
- every lemma-card control passes;
- no card, in a control or the sample, teaches a misleading meaning;
- at most 3 cards, across the controls and the sample, are near-misses.

**Either omission passes a control.** A literal-use or ambiguous control
passes when its card is omitted as `literal_use` or `ambiguous`. Both keep the
card out of the deck, so the label does not matter to the learner. An
unresolved answer with no kind, an invalid response and a provider error still
fail. A control whose card is confirmed fails its automatic check, which flags
it for grading; the reviewer then grades the card like any other, so it counts
against the near-miss budget or, if misleading, fails the gate.

**The case set is frozen.** `idiom-review-v3` stays the case set for Italian.
A failed review no longer adds its failures as new cases; new cases need their
own decision. The prompt contract stays `idiom-card-v3`.

**One fresh run decides.** Reports written before this ADR are not re-graded
under it. The run of record is a new run made after the gate is in place, at
the reasoning setting the deployment will use, and its result is accepted
without re-running.

The budget of 3 was chosen after seeing the 2026-11-08 runs, which had 2 and 4
cards that would grade as near-misses. That is why the decision rests on a new
run rather than on those reports.

## Considered options

- **Keep zero tolerance and keep iterating on the prompt.** Five rounds show
  that fixes trade one failure for another. Rejected.
- **Remove the failing headwords from the idiom list.** That fixes the review
  without fixing the cards, one example at a time. Rejected.
- **A dependency-parse guard before the request.** General, but the simple
  rule does not catch the observed case. Deferred until real decks show a
  pattern.
- **Track controls across several runs.** More robust, but it multiplies paid
  provider calls for a personal deployment. Rejected for now.

## Consequences

- A language may ship with a few cards whose wording is slightly off. None of
  them teaches a misleading meaning.
- The review command reports one automatic check, "card is omitted
  (`literal_use` or `ambiguous`)", for every literal-use and ambiguous
  control, and asks for a correct, near-miss or misleading verdict.
- German and Modern Greek use the same gate when they come up.

## Related

- [ADR 0112: The Gloss is the lemma's sense in citation form](0112-gloss-is-lemma-sense-in-citation-form.md)
- [ADR 0114: Idiom cards from dictionary evidence, confirmed by the provider](0114-idiom-cards-from-dictionary-evidence.md)

# ADR 0135: Study reports hold a word or replace its context

Status: **Accepted** · Date: 2026-11-26 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Justin decided it in a grilling
session prompted by two faulty Study cards seen in the pilot.

This ADR amends [ADR 0134](0134-study-in-mouseion-vocabulary-review.md):

- §6, which gives each word one Study material context that changes only after
  a confirmed correction;
- §5, which assumes every review the learner answers in Today is graded.

## Context

The Study pilot shows each word in one study context, with a Gloss and a
sentence translation. Two cards in the first days of the pilot were wrong in
different ways:

1. **Wrong word.** In *Angesichts ihres eigenen Reichtums kam die Kirche nicht
   umhin, diese Sekten als ketzerisch zu verdammen*, `kam` was studied as
   `kommen` with the Gloss "to be unable to avoid". The word is the separable
   verb `umhinkommen`. The analyzer did not reattach `umhin`, so the occurrence
   carries the wrong vocabulary identity. The Gloss is right for
   `umhinkommen`, and wrong for `kommen`.
2. **Unsuitable context.** `immer` was studied in a sentence where it is part of
   the Idiom *immer wieder*, with the Gloss "again and again". The identity is
   right. The context teaches the Idiom's meaning, not the word's.

Study has no way for the learner to say so. A Lemma correction can repair the
first case, but only from Reading, and Study picks it up only when the word is
next due. Until then the learner keeps reviewing a wrong card. Nothing can
repair the second case: no Lemma correction applies, because the identity is
right.

## Decision

### 1. The learner reports Study material, not the word

A **Study report** is made from Today, after **Show answer**, on the Study
material in front of the learner. The reported review is not graded: no grade
is stored and the schedule does not move. A report has one of two kinds, and
the learner may add a note.

### 2. A Wrong-word report holds the word until it is repaired

- **Held:** the word gets no reviews, is not part of Today's finish line, and
  does not fall overdue. Its Study progress is kept. A new word that is held is
  not offered in New-word batches, and the batch takes the next candidate.
- **Repair happens in Lemma review.** The Book's Lemma review lists open
  Wrong-word reports next to Lemma review flags and opens the reported
  occurrence. Study gets no correction workflow of its own. Study lists open
  reports, each linking to its occurrence, with **Dismiss**.
- **The report ends** when:
  - the learner confirms a Lemma correction or Occurrence exclusion on the
    occurrence; or
  - an Analysis update gives the occurrence a different effective vocabulary
    identity, or the occurrence no longer exists, for example because its Book
    left the library.

  The word is then re-introduced on the next Study day through ADR 0134 §6's
  existing path. It gets new Study material and a re-anchored Learned meaning,
  and keeps its Study progress. If no valid occurrence is left, it is
  withdrawn, as today.
- **Dismissing** ends the report without a repair. The word's due date moves
  later by the time it was held, so the report never makes it overdue.

A report is an input to Lemma review, like a Lemma review flag. It changes no
vocabulary identity itself.

### 3. An Unsuitable-context report replaces the context at once

- The word is not held. The reported sentence is never again used as this
  word's Study material or Fluency context.
- The word is re-introduced on the next Study day from another context, through
  the same path as §2.
- If no other context passes the sentence-quality gate, the word is withdrawn
  with the reason "no suitable context left". It keeps its progress, and it
  returns when a new Book provides a context.
- The learner can undo the report while the card is still shown. After that,
  the report stands.
- The rejected context's Gloss and translation are kept as evidence, as
  Fluency keeps its disagreeing contexts.

This kind also covers a wrong Gloss or translation on the right word: another
context fixes it without a regenerate action.

### 4. Reports are Today-only

Fluency is ungraded and changes no schedule, so a bad Fluency context costs
little. Reporting in Fluency is left out of this decision.

## Considered options

- **Suspend the word, with no kinds.** This is the first idea, and it is
  close to an Anki suspend. It was rejected because the Unsuitable-context case
  needs no manual repair: holding the word would wait for a fix that never
  comes.
- **Correct the lemma to the multi-word expression** (`immer` → `immer
  wieder`). This was rejected. A Lemma correction changes one token, so
  `wieder` would still count as its own word. It would also give an Idiom a
  vocabulary identity with Known and Unknown states, which `CONTEXT.md` rules
  out, and it would hide the word the learner has not yet been taught.
- **A correction form inside Study.** This was rejected because Lemma review
  already has the preview-and-confirm workflow. Two copies of it would
  diverge.
- **Swap in a new context for a Wrong-word report as well.** This was
  rejected. Without a repair, the same wrong analysis would keep producing
  wrong cards for the same occurrence, and the misattributed occurrence would
  keep counting towards the wrong word.
- **Call it a flag.** This was rejected because a Lemma review flag is raised by
  the system and is advisory. A Study report is the learner's own claim.

## Consequences

- **A new table needs human review.** Study reports and the held state go
  through `migrations/`.
- **A new term is defined.** `CONTEXT.md` defines Study report and its two
  kinds.
- **Study gets two more paths that end a held state** and one more reason to
  withdraw a word. These sit beside the paths ADR 0134 §6 already has.
- **Separate follow-ups.** The two cards also show three separate problems,
  each tracked on its own:
  - the analyzer does not reattach `umhin`;
  - Study highlights only the verb token of a separable verb;
  - Study material preparation does not reject idiomatic or not-this-lemma
    contexts as the Fluency check does.

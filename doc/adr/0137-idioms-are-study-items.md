# ADR 0137: Idioms are Study items

Status: **Accepted; implementation pending** · Date: 2026-11-26 ·
Author: Justin + OpenCode

§4 amended by [ADR 0141](0141-boosts-order-new-words-the-current-reading-is-a-focus.md): the Current reading's Idioms no longer come first; a Boost orders them.

Accepted by merging this ADR's pull request. Decided in
[#2729](https://github.com/justin-hayes/mouseion/issues/2729) in a grilling
session. It satisfies the Idiom prerequisite of the Study cutover in
[ADR 0136](0136-study-replaces-known-vocabulary-and-prepared-deck.md) §8.

At the Study cutover, it amends:

- [ADR 0114](0114-idiom-cards-from-dictionary-evidence.md): Idioms are no
  longer card-only, and a candidate no longer needs an anchor;
- [ADR 0134](0134-study-in-mouseion-vocabulary-review.md): Study progress,
  Study material and the Learned meaning belong to a Study item, which is a
  word or an Idiom, not to a vocabulary identity alone.

## Context

Idiom cards exist only in the Prepared deck (ADR 0114). There, an Idiom is
card-only: it has no vocabulary state, and a candidate counts only when one of
its words is a vocabulary identity frozen in the deck, called its anchor. The
anchor rule and the per-deck bound of 30 idiom cards exist to keep a frozen
deck small.

ADR 0136 retires the Prepared deck. Study keys everything it stores by
vocabulary identity: Study progress, Study material, Fluency contexts and Study
reports. Study chooses a sentence only by its quality and never avoids Idioms,
so a word such as *immer* can be studied inside *immer wieder*. The learner
can then report only an Unsuitable context. Without this decision, Idioms
would stop being taught at all once the deck retires.

## Decision

### 1. An Idiom is a Study item

- A **Study item** is a word (a vocabulary identity) or an Idiom. An Idiom is
  identified by its study language and its dictionary headword, which is the
  key its Anki note already uses. It is one item wherever it occurs.
- An Idiom has its own Study progress and Learned meaning. Its Learned meaning
  is the idiom Gloss confirmed in the sentence that introduces it, under the
  idiom contract's rules (ADR 0114, ADR 0115): citation form, citing a
  dictionary sense.
- An Idiom takes one place in a New-word batch, under the same daily cap and
  backlog brake as words. **Already known** is available for it.
- "Word" keeps meaning a vocabulary identity. Coverage counts word tokens only,
  so Idioms never change it (ADR 0136 §3).

### 2. Candidates come from the Corpus, with no anchor

- Idiom candidates are matched across the whole Corpus by the ADR 0114
  matcher, with the same entries, reviewed lists, gap bound and overlap rule:
  the longest headword wins, and ties go to the earlier position. A candidate
  no longer needs an anchor.
- The provider still confirms each candidate, one sentence at a time, when
  Study material or a Fluency context is prepared, and it never proposes an
  Idiom. An Idiom whose candidates are all literal or ambiguous is unpreparable
  and is skipped, as a word is.

### 3. Idioms and their member words are independent

- An Idiom is introduced whether or not its member words are in study, and the
  reverse also holds.
- When the language's idiom switch is on, a word's Study material and Fluency
  contexts prefer sentences in which the word is not part of an Idiom
  candidate. This is the deck's representative-sentence rule, carried into
  Study.
- Study reports never cross between them. An Unsuitable-context report on
  *immer* in *immer wieder* retires that sentence for *immer* only. It never
  creates, confirms or introduces the Idiom, and the learner's note is kept
  only as evidence for curating the reviewed idiom list. A report on an Idiom
  does not affect its member words.

### 4. New-word order ranks Idioms with words

- Idioms and words form one stream. An Idiom's frequency is its number of
  candidates in the Corpus, compared directly with words' occurrence counts,
  and the existing deterministic tie-break applies.
- The noun-only tier is dropped. It chose which Idioms fit under the deck's
  bound, and Study has no bound.
- The Current reading's Idioms come first together with its words, for as long
  as the Book is below Target coverage. That coverage counts words only, so
  Idioms never extend the boost.

### 5. Reports, corrections and withdrawal follow the word rules

- An Idiom offers only the **Unsuitable context** report, which covers literal
  use and a wrong Gloss or translation. A Wrong-word report would have no Lemma
  correction to repair.
- When an Idiom is due, Study checks that its introducing sentence still
  matches the headword after Lemma corrections, Occurrence exclusions and
  Analysis updates. If it does not, the Idiom is re-introduced from another
  confirmed context and keeps its progress.
- An Idiom with no candidate left in the Corpus is withdrawn and keeps its
  progress. A dictionary or reviewed-list refresh that drops the headword
  counts as no candidate left. The Idiom returns if its evidence reappears.
  Corrections never create reviews.

### 6. Presentation and Fluency

- Today and Fluency use the idiom card layout. The front shows the sentence
  with every matched word in bold. After Show answer, the card shows the
  headword, an "idiom" label, the Gloss and the highlighted translation. It has
  no morphology block.
- Fluency contexts for a Mature Idiom follow the word rules applied to the
  matched span. Contexts in a different inflection or arrangement come first,
  up to 5 are prepared, and each must agree with the Learned meaning.

### 7. Rollout

- Study introduces Idioms only in languages whose idiom switch is on, behind
  the ADR 0115 review gate. Today that is Italian only.
- Turning a switch off stops new Idioms and new Idiom Fluency contexts. Idioms
  already in study keep their progress and keep their reviews.
- German and Modern Greek become available when their matching and review are
  ready. That work does not block the cutover.
- Idioms were never Known, so the cutover migrates no Idiom history from Anki.
  Every Idiom starts as not yet introduced.

## Considered options

- **Introduce an Idiom only after a member word is in study, or only after all
  of them are Mature.** This was rejected. *in gamba* teaches "capable"
  whatever the learner knows of *gamba*. The anchor rule existed only to keep a
  frozen deck small.
- **Rank Idioms by confirmed counts.** This was rejected because it would need
  a provider call for every candidate before ranking.
- **Give Idioms fixed places in each batch.** This was rejected because it
  would add a second quota with no clear meaning. Frequency already puts most
  Idioms behind the common words.
- **Let a report on a member word propose an Idiom.** This was rejected. The
  learner, like the provider, never proposes an Idiom, and detection stays
  anchored in dictionary evidence.
- **Count Idioms in coverage.** ADR 0136 §3 already rejected this.

## Consequences

- **A second Study key.** Study progress, Study material, Fluency contexts and
  Study reports need an item kind, and the Idiom kind is keyed by headword. The
  migration needs human review.
- **Corpus-wide matching is new work.** The matcher is reusable, but candidate
  assembly runs only during deck freeze, against frozen vocabulary. Ranking
  also needs a count of each Idiom's candidates across the Corpus.
- **The German pilot shows no Idioms** until German matching handles reordered
  verb-second clauses and passes its review.
- Book-scoped Browse, Concordance and Sentence study do not show Idioms. A
  later decision may add them.

## Related

- [Idiom cards](../features/idiom-cards.md)
- [Study](../features/study.md)
- [ADR 0135: Study reports](0135-study-reports.md)

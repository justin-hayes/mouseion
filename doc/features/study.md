# Study

Status: **Accepted; pilot not yet implemented** · Date: 2026-11-25 · Decisions:
[ADR 0134](../adr/0134-study-in-mouseion-vocabulary-review.md),
[ADR 0133](../adr/0133-per-language-card-policy.md)

Specified in
[Spec: Study pilot](https://github.com/justin-hayes/mouseion/issues/2691).
The terms **Study**, **Study day**, **Today**, **New-word batch**, **In study**,
**Study progress**, **Mature**, **Study material**, **Learned meaning**,
**Fluency** and **Fluency context** are defined in
[`CONTEXT.md`](../../CONTEXT.md).

## Motivation and goal

A learner reviews Mouseion's vocabulary in their own Anki, on their phone. The
loop is slow: nothing arrives until a whole Prepared deck is ready, downloaded
and imported. The words are tied to one Book, while learning a language is a
continuous project. Each word is seen in one sentence, so the learner learns
the sentence rather than the word in its other forms. And finishing a Book
accepts the whole snapshot into Known vocabulary, whatever was learned.

The goal: Mouseion is the place where the learner learns and reviews
vocabulary from their reading, on their phone, as material becomes ready.

## What Study is

Study is a phone-first page for each study language, at `/{language}/study`. It
is used online only, over the tailnet like the rest of Mouseion. It is one
continuous stream of words from the learner's analysed library, on one card
surface, in three phases each Study day:

1. **Today's reviews**, graded.
2. **New-word batches**, optional.
3. **Fluency**, ungraded and open-ended (milestone 2).

The work has two milestones. Milestone 1 is Today: reviews and New-word
batches. Milestone 2 is Fluency, built during the pilot's first month, because
a word that never lapses first becomes mature about four weeks after it is
introduced. The pilot runs for about eight weeks on German.

## Today

### The Study day

The Study day begins at 05:00 in Europe/Berlin, so reviews after midnight count
towards the day the learner is finishing. The boundary stays correct across
daylight-saving changes: a changeover never makes reviews disappear or doubles
them. The hour is a pilot constant, not a learner setting.

### Reviews

- Each card shows the sentence with the target word marked. **Show answer**
  reveals the Gloss, the sentence translation and the word's citation
  headword.
- There are two grades: **Again** and **Got it**. A word marked Again comes
  back later the same day.
- Intervals come from FSRS, aiming at 90% recall, slightly randomised so that
  words introduced together do not stay bunched. Again maps to FSRS Again and
  Got it to Good. There is no Hard or Easy grade.
- After missed days, every overdue review is due, scheduled by how likely the
  learner is to remember it rather than by a penalty.
- Every graded review is stored with its time, grade, Study day and the
  scheduling state before and after, so the history can be replayed under
  other parameters later.

### The finish line

Today shows how many due reviews remain. It is finished once every review due
at the start of the Study day is answered, including the same-day repeats those
reviews cause. It stays finished until the next Study day, even if more
material becomes ready. New-word introductions and their repeats come after the
finish line and are never part of it.

### New-word batches

- After the reviews, Today offers optional batches of 10 new words, up to 3
  (30 words) a day. Taking or declining a batch is a single tap, and declining
  never blocks the learner.
- **Backlog brake.** Mouseion forecasts the reviews due on the next Study day.
  At 100 or more, at most one batch is offered; at 150 or more, none. These
  thresholds are starting values for the pilot to tune.
- A new word's first appearance is graded: Got it makes it due on the next
  Study day, and Again repeats it the same day.
- Words from a batch that was taken but not finished return to the front of
  the stream on the next Study day.
- A batch contains only words whose Study material is ready. When words are
  still being prepared, Today says so, which is why fewer words may be offered.

### Where new words come from

- Candidates are vocabulary identities in the study language, evidenced by the
  learner's currently analysed Books, that are not yet in study. Eligibility
  follows the same rules as Book vocabulary candidates: confirmed Lemma
  corrections and Occurrence exclusions apply, and nothing that is not
  vocabulary is included.
- Words from the Current reading come first, then all others. Within each
  group, words are ordered by Personal analyzed-Book occurrences, most frequent
  first, with a deterministic tie-break. Frequency comes only from the
  learner's own library (ADR 0018, ADR 0090).
- The order is computed from current evidence, not a frozen snapshot. Study
  works with no Current reading, and the stream continues from the library when
  the Current reading has no words left.
- Known and Reserved vocabulary are not consulted: a word that is Known from a
  finished Book is still eligible as a new word.
- A word with no usable sentence, or for which no meaning can be defended, is
  skipped and the next word is offered.

## Study material

Each word has one **Study material** record: one complete study context, its
Representative sentence, with that sentence's Gloss, translation and any
English target alignment. That one context carries every Today review of the
word.

### Preparation

- Mouseion prepares the material automatically, one word at a time, shortly
  before Today needs it. The learner never downloads or imports a deck. No page
  request calls a provider; durable background jobs do.
- The sentence is chosen by the existing sentence-quality rules, from the
  Current reading's occurrences when the word occurs there, otherwise from the
  library's.
- Preparation is demand-led and stays about one Study day's maximum offer (30
  words) ahead of the learner, so providers are not paid for words the learner
  may never reach. It is triggered when Today is opened, when a batch is taken
  and at the Study day rollover.
- A word whose sentences all fail the sentence-quality gate, or whose meaning
  is omitted by the provider on its merits, is marked unpreparable with its
  reason and skipped. Any other provider omission is retried later.

### Learned meaning

The Gloss the learner sees when a word is introduced becomes its **Learned
meaning**. Later study keeps to that meaning.

### After a correction

Study material reflects the learner's current Lemma corrections, Occurrence
exclusions and published Analysis updates. When a word is next due, Study
checks that its introducing occurrence still resolves to the word's identity.

- **If it does not,** the review becomes a **re-introduction**: new Study
  material is prepared, the card is marked as corrected, the Learned meaning is
  re-anchored to the new sentence, and the learner's Study progress is kept.
- **If the identity has no valid occurrence left in the library,** the word is
  **withdrawn by a correction**. It stops coming up for review and is shown as
  such, its progress is kept, and it returns if evidence reappears.
- The identity a correction points to is never put into study automatically.
  Corrections never create reviews.

## Fluency (milestone 2)

Fluency gives variety once a word is familiar, without grading it.

- **When it opens.** Once Today's reviews are done and every offered batch has
  been taken or declined. A remaining batch can still be taken from inside
  Fluency. A learner with no mature words yet sees a note at the end of Today
  that nothing is mature yet.
- **Which words.** A word is **Mature** when its review interval is at least 21
  days. A word that lapses below 21 days leaves Fluency until it is mature
  again.
- **Fluency contexts.** For each mature word, Fluency shows other sentences
  from the analysed library, the Current reading first, that pass the
  sentence-quality gate. It prefers sentences where the word appears in a
  different form: another tense, person, case or number, or a separable verb
  split from its prefix. Different sentences of the same form come after. Up
  to 5 contexts per word are prepared when the word becomes mature, so
  preparation is limited to words the learner is keeping.
- **Only the Learned meaning.** A Fluency context is kept only if the word
  carries its Learned meaning there. Contexts that do not agree are discarded.
- **Responses.** Each card has **Show answer**, then **Recognised** or **Didn't
  recognise it**. A context marked Didn't recognise it returns later in the
  same sitting and more often afterwards; Recognised lowers its weight again
  over time.
- **Rotation, not a schedule.** The weighted rotation prefers forms not seen
  recently and words the learner finds hard in Today. Fluency continues for as
  long as the learner likes.
- **Never changes progress.** Nothing in Fluency changes Study progress or the
  schedule, so a bad Fluency day cannot push a mature word back. Responses are
  kept as evidence for deciding later whether Today should rotate sentences.

## Scope

### Languages

Study is offered only for study languages whose card policy
([ADR 0133](../adr/0133-per-language-card-policy.md)) is the modern-language
provider route, because Study needs a Gloss and a sentence translation. The
pilot runs on German. For Latin and Ancient Greek, whose source-evidence cards
have no defensible Gloss to anchor a Learned meaning, the Study page says
plainly that Study is not available for the language yet.

### Separation from the rest of Mouseion

- Study progress belongs to the word, per learner and study language. It never
  belongs to a Book, a deck or a sentence.
- Study never makes a word Known, and Known never ends, excludes or demotes a
  word in study.
- Starting, switching, ending or finishing a Book changes only which new words
  come first.
- Reading, Reading completion, Known vocabulary, Reserved vocabulary,
  current-reading snapshots and Prepared decks keep their current behaviour.
  Anki remains the route for languages Study does not cover.
- Existing Anki history stays in Anki; Study neither imports nor exports it.

### What the pilot leaves out

- Offline use and a native app.
- Importing, exporting or syncing Anki history.
- Study for Latin and Ancient Greek.
- One shared Today across several languages.
- Browsing upcoming words and saving words for later.
- Rotating sentences in Today's graded reviews: Today uses one sentence per
  word.
- Progress per meaning, and "this word also means...".
- Idiom cards in Study, production cards (English to German) and audio.
- External frequency lists.
- Cognates, etymology and recommendations.
- An FSRS parameter optimiser, Hard and Easy grades, and words leaving study.
- A fixed split of new words between the Book and the library, and fading words
  learned for one Book only.

## Open questions

These stay open for after the pilot.

1. Should Study progress eventually feed or replace Known vocabulary? This is
   the main question for a follow-up ADR.
2. Should batches use a fixed split between the Book and the library?
3. Should words learned only for one Book fade after the learner finishes it?
4. Should repeated Didn't recognise it in Fluency pull a word back into Today?
5. Should Today's mature reviews rotate sentences? Fluency's evidence should
   decide.
6. How should Study work for Latin and Ancient Greek, perhaps with the Learned
   meaning anchored to a dictionary entry?
7. Should progress be tracked per meaning?
8. What are the right backlog thresholds?
9. Should the Study day hour be a learner setting?

## What to observe in the pilot

Daily time spent; whether the third batch is taken; whether tomorrow ever feels
like a penalty; how much Fluency is used and how often Didn't recognise it is
marked.

# Study

Status: **Accepted; implemented in the pilot** · Date: 2026-11-25 · Decisions:
[ADR 0134](../adr/0134-study-in-mouseion-vocabulary-review.md),
[ADR 0135](../adr/0135-study-reports.md), [ADR 0133](../adr/0133-per-language-card-policy.md),
[ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md)
(direction after the pilot, see [After the pilot](#after-the-pilot))

Specified in
[Spec: Study pilot](https://github.com/justin-hayes/mouseion/issues/2691).
The terms **Study**, **Study day**, **Today**, **New-word batch**, **In study**,
**Study progress**, **Mature**, **Study material**, **Learned meaning**,
**Fluency**, **Fluency context** and **Study report** are defined in
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

Study is a phone-first page for each study language, at `/{language}/study`, laid out
for a compact screen with large tap targets so it can be used one-handed. It
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
  (30 words) a day. Declining a batch is a single tap, so the learner is never
  forced to learn new words to move on.
- **Backlog brake.** Mouseion forecasts the reviews due on the next Study day.
  At 100 or more, at most one batch is offered; at 150 or more, none. These
  thresholds are starting values for the pilot to tune.
- A new word's first appearance is graded: Got it makes it due on the next
  Study day, and Again repeats it the same day.
- Words from a taken batch that are not yet introduced return to the front of
  the stream on the next Study day.
- A batch contains only words whose Study material is ready. When words are
  still being prepared, Today says so, which is why fewer words may be offered.

### Where new words come from

- Candidates are vocabulary identities in the study language, evidenced by the
  learner's currently analysed Books, that are not yet in study. Eligibility
  uses the same vocabulary-identity rules as Book vocabulary candidates: confirmed Lemma
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
  may never reach. It is triggered when Today is opened, when a batch is taken,
  at the Study day rollover and when a Book's analysis is published with its
  Browse counts, in every language Study serves, whether or not the learner has
  opened Study for it yet. The first open is then usually instant. The window
  is the same, so a trigger costs at most about one day's maximum offer of
  provider calls, and nothing is sent without configuration and consent.
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

### Study reports

Decided in [ADR 0135](../adr/0135-study-reports.md). When Study material is
wrong, the learner reports it from Today. **Report** appears after **Show
answer**, beside the grades. The reported review is not graded. The learner
picks one of two kinds and may add a note.

- **Wrong word.** The occurrence belongs to another vocabulary identity, for
  example `kam … umhin` studied as `kommen` rather than `umhinkommen`.
  - The word is **held**. It gets no reviews, is not part of Today's finish
    line, and does not fall overdue. Its Study progress is kept. A new word that
    is held is not offered in New-word batches, and the batch takes the next
    candidate.
  - The repair happens in the Book's **Lemma review**, which lists open
    Wrong-word reports next to Lemma review flags and opens the reported
    occurrence.
  - The report ends when a Lemma correction or Occurrence exclusion is
    confirmed on the occurrence. It also ends when an Analysis update gives the
    occurrence a different effective vocabulary identity, or when the
    occurrence no longer exists. The word is then re-introduced on the next
    Study day, as after a correction.
  - **Dismiss** ends the report without a repair. The word's due date moves
    later by the time it was held.
- **Unsuitable context.** The identity is right, but the context does not teach
  the word's own meaning, for example `immer` inside the Idiom *immer wieder*,
  or a wrong Gloss or translation.
  - The word is not held. The sentence is never used again as the word's Study
    material or Fluency context, and the word is re-introduced from another
    context on the next Study day.
  - The learner can undo the report while the card is still shown.
  - The rejected context's Gloss and translation are kept as evidence.
- **Nothing left.** If no valid context remains, the word is withdrawn as
  after a correction, with its own reason when every context was reported as
  unsuitable. It keeps its progress.
- **Reported words.** Study lists open Wrong-word reports, each with a link to
  its occurrence and **Dismiss**.
- Reports are made in Today only, not in Fluency.

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
  carries its Learned meaning there. Each context has its own Gloss check and
  translation, and contexts that do not agree are never shown. A disagreeing
  context keeps its reason (a different meaning, an idiomatic use, not this
  lemma, or no defensible answer), its Gloss and its translation as evidence
  of the word's other meanings.
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

### Reading Fluency's evidence

Study has three aims: acquiring new words (New-word batches), retaining them
(Today), and recognising them in unfamiliar contexts (Fluency). Retaining a
word and recognising it in other contexts are related but separate, so Fluency
evidence is read alongside Today's grades rather than on its own.

- **Retention or transfer.** A Didn't recognise it is read against the word's
  next Today review, the graded test in its familiar sentence. Got it there
  means a transfer problem: the word is retained but not recognised elsewhere.
  Again there means a retention problem, which FSRS already handles. Fluency has
  no follow-up card in the familiar sentence: it would come straight after the
  Gloss was revealed, and so would measure nothing.
- **Form or sentence.** Fluency shows different-form contexts first and
  same-form contexts after, so both kinds are answered. A transfer gap on
  same-form contexts means the sentence has been memorised; a gap only on
  different-form contexts means the forms have not been learned.
- **Other meanings.** Disagreeing contexts give how often a mature word turns
  up in a different meaning, and what that meaning is. The sample undercounts,
  because a word's search stops once it has enough agreeing contexts.

## Scope

### Languages

Study is offered only for study languages whose card policy
([ADR 0133](../adr/0133-per-language-card-policy.md)) is the modern-language
provider route, because Study needs a Gloss and a sentence translation. The
pilot runs on German. For Latin and Ancient Greek, whose source-evidence cards
have no defensible Gloss to anchor a Learned meaning, the Study page says
plainly that Study is not available for the language yet. These are the
languages where Fluency's variety of form should pay off most, because of how
heavily they inflect; their Learned meaning would plausibly be anchored to the
lexical discriminator and a dictionary entry rather than a Gloss (open
question 6).

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
- Importing Anki history, and exporting or syncing Study progress to Anki.
- Study for Latin and Ancient Greek.
- One shared Today across several languages.
- Browsing upcoming words and saving words for later.
- Rotating sentences in Today's graded reviews: Today uses one sentence per
  word.
- Progress per meaning, and "this word also means...".
- Idiom cards in Study, production cards (English to German) and audio.
- External frequency lists.

### After the pilot

[ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md)
makes Study the only vocabulary workflow, at one cutover. Until then the
separation above holds. The cutover waits until Study supports the
source-evidence card policy (Ancient Greek and Latin) and introduces Idioms as
items with their own Study progress
([ADR 0137](../adr/0137-idioms-are-study-items.md)). Both of those are left
out of the pilot above. At the cutover:

- Known vocabulary and the Prepared deck retire. Each Known word becomes
  **Already known** Study progress, with first reviews spread over several
  weeks.
- **Already known** becomes an answer at introduction, and it makes the word
  Mature at once.
- New words come from the **Corpus**. The Current reading's words come first
  until the Book reaches **Target coverage**. After that, frequency across the
  To Read Books decides.
- A Book's coverage counts Mature words, with in-study coverage shown as a
  second figure.
- A Wrong-word report opens a remediation screen instead of the Book's Lemma
  review, which retires.
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
4. ~~Should repeated Didn't recognise it in Fluency pull a word back into
   Today?~~ **Closed: no** (2026-11-26). A retention failure already shows as
   Again at the word's next Today review. A transfer failure would not be
   helped by Today, whose familiar sentence the learner already recognises. The
   question returns only as part of rotating sentences in Today (question 5).
5. Should Today's mature reviews rotate sentences? **Decision rule, agreed
   before the evidence (2026-11-26):** take Fluency contexts on their first
   showing whose word was Got it at its next Today review, and measure the share
   marked Didn't recognise it, separately for same-form and different-form
   contexts. Consider rotating sentences in Today's mature reviews when the
   same-form share is at least 20% over at least 100 first showings. Below 10%,
   keep one sentence. In between, keep observing. A gap only on different-form
   contexts argues for weighting Fluency further towards forms, not for
   rotating Today. The thresholds are starting values.
6. How should Study work for Latin and Ancient Greek, perhaps with the Learned
   meaning anchored to a dictionary entry?
7. Should progress be tracked per meaning? **Decision rule (2026-11-26):**
   measure the share of mature words with at least one disagreeing context
   whose reason is a different meaning. Above about 25%, the next step is an
   ungraded Fluency card showing the word in its other meaning with that
   meaning's Gloss ("this word also means..."), not separate progress. Progress
   per meaning is considered only if that card is built and its Didn't
   recognise it rate stays high. It would first need an ADR for a stable
   meaning identity, because a Learned meaning is a Gloss, not an identity.
   Until then, Study progress belongs to the vocabulary identity alone.
8. What are the right backlog thresholds?
9. Should the Study day hour be a learner setting?

## What to observe in the pilot

Daily time spent; whether the third batch is taken; whether tomorrow ever feels
like a penalty; how much Fluency is used and how often Didn't recognise it is
marked.

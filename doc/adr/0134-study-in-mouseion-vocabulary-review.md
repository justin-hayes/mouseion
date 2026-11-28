# ADR 0134: Study is in-Mouseion vocabulary review, separate from Known

Status: **Accepted** · Date: 2026-11-25 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Justin decided it in a grilling
session that shaped the pilot, and in the prototype that followed it
(branch `prototype/study-sentence-variation`, README section "Verdict"). The
pilot is specified in
[Spec: Study pilot](https://github.com/justin-hayes/mouseion/issues/2691).

This ADR amends:

- the non-goal in [ADR 0036](0036-primary-goal-justified-graduation.md): "No
  Anki review-log / AnkiConnect synchronization or spaced-repetition mastery
  model";
- the exclusion of "in-app spaced repetition" in
  [Vocabulary Browse, Concordance and custom decks](../archive/features/vocabulary-browse-concordance-and-custom-decks.md);
- the Reading-owned vocabulary boundary of
  [ADR 0085](0085-reading-owned-book-vocabulary.md), for Study only;
- the list of pgx importers in [ADR 0091](0091-go-package-layering.md) §3, to
  add one job package.

## Context

A learner reviews Mouseion's vocabulary in their own Anki, on their phone:

1. A Current reading freezes a vocabulary snapshot.
2. The learner prepares a deck from it.
3. They download the deck and import it into Anki.
4. Finishing the Book accepts the whole snapshot into Known vocabulary,
   whatever was studied.

This has four costs:

- **Nothing arrives early.** The learner gets nothing until a whole deck is
  ready, downloaded and imported.
- **Reviews are tied to one Book.** Each deck belongs to one reading.
- **One sentence is memorised.** Each word is seen in one sentence, so the
  learner learns to recognise that sentence rather than the word in its other
  forms.
- **Known is coarse.** It records that the learner finished a Book, not what
  they learned.

Earlier decisions kept review out of Mouseion: ADR 0036's non-goal, and the
feature contract's exclusion of in-app spaced repetition. ADR 0085 also
retired vocabulary selection across Books because it "distracts from reading
and finishing Books".

## Decision

### 1. Study is a surface of Mouseion

Study is a per-language page, `/{language}/study`. It is phone-first, online
only, and reached over the tailnet like the rest of Mouseion (ADR 0009).

- **Scope:** only study languages whose card policy is the modern-language
  provider route ([ADR 0133](0133-per-language-card-policy.md)). Latin and
  Ancient Greek have no Gloss to anchor a learned meaning, so they are left
  out. The pilot runs on German.
- **Anki:** Prepared decks keep working unchanged for the learner's own Anki.
  Study neither imports nor exports Anki history.

### 2. Study progress belongs to the word

Study progress is owned by learner, study language and vocabulary identity,
including its lexical discriminator. It never belongs to a Book, a deck or a
sentence.

### 3. Study is separate from Known and from the reading lifecycle

- **Known:** Study progress never makes a word Known, and Known never ends,
  excludes or demotes a word in Study.
- **The reading lifecycle:** starting, switching, ending or finishing a
  Current reading only changes which new words come first.
- **Where new words come from:** the learner's currently analysed Books in the
  study language, from current evidence, not a frozen snapshot. They are
  ordered by Personal analyzed-Book occurrences, with words from the Current
  reading first.
- **Unchanged:** Reading completion, Known vocabulary, Reserved vocabulary,
  current-reading snapshots and Prepared decks keep their present behaviour.

For Study only, this reopens ADR 0085's choice to select vocabulary from a
single Book. ADR 0085 still governs Book vocabulary candidates and Prepared
decks.

### 4. Frequency comes only from the learner's library

Study ranks words only by the learner's own analysed library.
[ADR 0018](0018-remove-dwds-frequency.md) and
[ADR 0090](0090-library-total-book-vocabulary-selection.md) stand: no
external frequency list is used.

### 5. Scheduling uses FSRS through `go-fsrs`

- **Library and settings:** Study schedules reviews with FSRS through the
  `go-fsrs` library (MIT), using its default parameters, a target recall rate
  of 0.9 and interval fuzz.
- **Grades:** there are two. Again maps to FSRS Again, and Got it maps to
  Good.
- **Review history:** every graded review is stored with the FSRS state before
  and after it, so the history can be replayed under parameters fitted later.

### 6. Today's reviews use one sentence per word

- **Study material:** each word's Study material is one introducing context,
  its Representative sentence, with that sentence's Gloss and translation. It
  is prepared from current evidence, with the learner's corrections applied.
- **Every review:** that one context carries all of the word's reviews in
  Today.
- **Learned meaning:** the Gloss shown at introduction becomes the word's
  learned meaning.
- **After a correction:** if a confirmed correction shows the introducing
  context was not this word, the next review re-introduces the word and
  re-anchors its learned meaning, keeping its Study progress. A word with no
  valid occurrence left in the library is withdrawn from review, and its
  progress is kept.

### 7. Variety belongs to Fluency, which is ungraded

- **What Fluency is:** after Today's reviews and new-word batches, Fluency
  shows **mature** words in Fluency contexts. Mature means the current
  interval is at least 21 days. Fluency contexts agree with the word's learned
  meaning and preferably show a different form.
- **What it records:** Fluency responses are kept as evidence. They never
  change Study progress or the schedule.

### 8. Study material is prepared by a new job package

- **How:** Study material and Fluency contexts are prepared one word at a
  time by durable River jobs, shortly before they are needed. No page request
  calls a provider.
- **Amendment to ADR 0091 §3:** the jobs belong to a new **study job
  package**. It needs pgx for the same reason as `internal/prepareddeck`: a
  job is enqueued in the same transaction as the preparation state it writes
  ([ADR 0010](0010-river-job-queue.md)).

## Considered options

- **Keep Anki as the only review surface.** This was rejected because the
  deck path is slow and tied to one Book, and Mouseion cannot see Anki's
  review history (ADR 0027).
- **Let Study progress make words Known, or let Known end a word's study.**
  This was rejected for now. Known can't be undone, and one model of
  knowledge must not silently rewrite the other during a pilot. Whether Study
  progress should eventually feed or replace Known is left to a later ADR.
- **Draw new words from the frozen Current-reading snapshot.** This was
  rejected. Finishing a Book would drop every word it hadn't yet introduced,
  and Study would stop between Books.
- **A fixed interval ladder.** This was easy to explain and was what the
  prototype used. FSRS was chosen instead because it handles overdue reviews
  by how likely the learner is to remember, and because the stored history
  can be fitted later.
- **Rotating sentences in Today's graded reviews.** This was deferred. The
  prototype showed a short trial couldn't measure the effect, and the risk of
  memorising one sentence builds up over months. Fluency gathers the evidence
  instead.
- **Browsing upcoming words and saving them for later.** This was dropped from
  the pilot in favour of Fluency.

## Consequences

- **New tables need human review.** Study progress, the review log, Study
  material, the per-day batch ledger, and in milestone 2 Fluency contexts and
  responses all go through `migrations/`. Their shape is the one in the spec.
- **A new dependency needs human review.** `go-fsrs` is added to `go.mod`.
- **The web app gets an injectable clock.** Study's day boundary is 05:00
  Europe/Berlin, and its tests need to control time.
- **Mouseion holds two models of knowledge side by side:** modelled Known
  vocabulary, owned by Reading, and Study progress, owned by Study.
- **Study terms are defined.** `CONTEXT.md` defines Study, Study day, Today,
  New-word batch, In study, Study progress, Mature, Study material, Learned
  meaning, Fluency and Fluency context.
- **Prior art.** `jacbz/anki_french` rotates example sentences and orders
  words by frequency. Its CC BY-NC-SA data is not used
  ([ADR 0111](0111-third-party-licence-posture.md)).

# PROTOTYPE: Study sentence variation

Throwaway. Not for main. This branch keeps it as a primary source for the Study
pilot (see the Study terms in `CONTEXT.md` and ADR 0133).

**Question:** when a word you're learning comes back in a sentence you've never
seen, does "Got it" still mean you know the word, rather than that you worked out
the sentence?

- If yes, the MVP keeps sentence variation: up to 3 sentences per word, all with
  the word's Learned meaning.
- If no, Study material is one sentence per word.

## Verdict (closed 2026-11-25, without running the trial)

**The MVP uses one sentence per word. Sentence variation is deferred.** The
learner checked all 60 sentences and approved the page. They then closed the
prototype before the trial began, so no grades were recorded. The decision rests
on four points:

- A two-week trial couldn't settle the question. With the real ladder each word
  gets about three scheduled reviews, so there would be about 30 reviews on each
  side, and a gap between the Again rates would be within noise.
- Either Again result can be read both ways: as variation being too hard, or as
  the familiar sentence inflating Got it.
- The main case for variation is that the familiar sentence gets memorised. That
  builds up over months, not two weeks.
- Variation can be added later without migrating anyone's progress, because Study
  progress belongs to the word, not to a sentence. One sentence is the cheaper
  default that can still be reversed.

If variation is revisited, agree the decision rule first. Use the guessing flag
and the learner's notes, not Again rates alone. For example, if guessing is
flagged on more than about a quarter of Got its on new sentences, keep one
sentence.

Explore, as built here, is ungraded card flipping over the words not yet in study.
The learner confirmed that is how they picture it.

## Run it

One static file; its data is embedded. Serve it over the tailnet and keep the same
URL for the whole trial, because grades live in that origin's `localStorage`:

```sh
python3 -m http.server -d internal/webapp/prototype-study-sentence-variation 8127 --bind 100.77.140.13
# phone: http://dev01.taildc9a7d.ts.net:8127/   (use the same host name every time)
```

`?test` switches to separate storage and adds a "Jump to next Study day" control,
so a walkthrough never touches the real log.

## What it mimics

- Recognition only: the German sentence, then the Gloss (the Learned meaning) and the
  sentence translation. **Again** / **Got it**, plus an optional one-tap "felt like
  guessing from context" flag on reviews.
- A word is introduced in its first sentence. Its scheduled reviews then cycle
  F, N1, F, N2 (even introduction order) or N1, F, N2, F (odd), so half the words
  meet a new sentence on their first review and new sentences aren't confined to
  the longer intervals.
- Spacing ladder 1, 3, 7, 16, 35, 77, 169, 180 days, **not compressed**. Got it
  climbs one rung; Again drops two, and the word comes back once more before Today
  finishes (Got it on that repeat schedules at the dropped rung without climbing).
  In two weeks a word reaches about three scheduled reviews.
- The Study day begins 05:00 Europe/Berlin. Today is finished once the reviews due
  at its start are answered.
- New-word batches after the reviews, take or decline. **Scaled for 20 words:**
  batches of 5 (the pilot uses 10), at most 3 a day, so a third-batch decision is
  still possible.
- Explore, once Today is finished: the same card flipping as Today, but ungraded
  (Show answer, then Next), over the words not yet in study, in their first
  sentence. On the back, "Save for a batch" makes the word a Study nomination: it
  takes a slot at the front of the next batch, never an extra batch.
  **Prototype limit:** in-study words, and any further sentences, are left out so
  the new sentences stay unseen until a review shows them. That restriction is
  itself a finding for the pilot: an Explore that flips in-study words through
  further sentences would spend the sentences variation depends on.

## Data

20 words from *Der ferne Spiegel* (the German Current reading), three sentences
each, read-only from the local library's current analysis (corpus
`238398d6-faee-4351-b48a-c859eba7f795`; each sentence keeps its `ordinal`). Words
are from the Book's prepared deck manifest, not Known, not in another deck, and
occur in 14–30 sentences of the Book. Sentence 1 is the deck's representative
sentence except for *Mangel*, where that sentence needed its context. Glosses and
translations were drafted by hand and checked by the learner against
`study-material-review.md`.

`words.json` is the source; `python3 embed_material.py` inlines it into
`index.html`.

## Export

**Results & export** shows Again rates for the first sentence, never-seen new
sentences and seen-before new sentences, and downloads the full log as JSON:
every grade with word, sentence slot, first/new, novelty, timestamp, guessing flag
and rung change; batch decisions; Explore views and saves; and the daily notes.

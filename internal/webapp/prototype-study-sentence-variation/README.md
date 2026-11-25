# PROTOTYPE: Study sentence variation

Throwaway. Not for main. This branch keeps it as a primary source for the Study
pilot (see the Study terms in `CONTEXT.md` and ADR 0133).

**Question:** when a word you're learning comes back in a sentence you've never
seen, does "Got it" still mean you know the word, rather than that you worked out
the sentence?

- If yes, the MVP keeps sentence variation: up to 3 sentences per word, all with
  the word's Learned meaning.
- If no, Study material is one sentence per word.

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
- Explore, once Today is finished: ungraded; a saved word becomes a Study
  nomination and takes a slot at the front of the next batch, never an extra batch.
  **Prototype limit:** Explore lists only words not yet in study, and shows only
  their first sentence, so the new sentences stay unseen until a review shows them.
  That restriction is itself a finding for the pilot: Explore browsing "further
  sentences" of a word in study would spend the sentences variation depends on.

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

# PROTOTYPE: a work's To Read Books in Reading's chooser (#2398)

Throwaway. Not for main. With a work moved to To Read as a whole (#2323), Reading's
between-Books chooser can hold 20+ Books of one work. This prototype asks how the
chooser groups a work's candidates and shows Known coverage at a glance, without
ranking them or hinting at a next book (#2324).

```sh
python3 -m http.server -d internal/webapp 8124
# open http://127.0.0.1:8124/prototype-work-chooser/?variant=A
```

The page loads the app's real `/static/app.css`. Data is illustrative and every
action is in memory; reloading resets it. The library: the Iliad moved to To Read as
a whole (Book 1 Read, Book 9 hidden, Books 4 and 21–24 preparing, Book 6 not
assessed), Anabasis Books 2–7 To Read, the Apology (a whole-work Book), Histories
Book 1 only; the Odyssey is in Inbox, so it is not a candidate.

- **A, Bands, work folded per band**: the shipped bands stay the page's sections.
  Inside a band, a work with several candidates there is one entry with its book
  numbers; the margin gives the count and coverage range in that band.
- **B, Work entry, band table**: one entry per work, in title order; inside it a
  small table of band → book numbers.
- **C, Work entry, strip**: one entry per work; a strip in book order, each cell
  showing its number and Known coverage (no colour, no ordering by coverage).
- **D, Flat bands + work filter**: the shipped chooser unchanged (one row per Book,
  "Book 3 of 24"), plus a filter by work above the bands.

Shared: choosing a book number opens that Book's ordinary candidate entry
(coverage, words to the next band, Start or Switch confirmation, Review a word).
Single-Book works stay ordinary rows. Coverage shows one decimal, as shipped, so
94.6% never reads as 95%. The bottom-right panel has a 390px compact toggle, a
switch-mode toggle (Iliad Book 2 current), and the full state.

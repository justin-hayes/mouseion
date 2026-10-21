# ADR 0108: Recognise Project Gutenberg Distributor text and exclude it from analysis

Status: **Accepted — implementation pending** · Date: 2026-10-21 · Author: Justin + OpenCode

Decided in triage of [#2246](https://github.com/justin-hayes/mouseion/issues/2246).
Extends main-text selection ([ADR 0066](0066-main-text-selection-from-epub-structure.md))
without changing its landmark rules, and amends how
[ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md) judges the
selection part of an Analysis pipeline identity.

## Context

Project Gutenberg EPUB3 files declare no `bodymatter` landmark, so ADR 0066
identifies no main text and the whole snapshot is analysed
(`selection-default-v1`). In the #2166 dictionary-coverage run, English header,
licence and illustration-page text was analysed as French: *the*, *of*,
*Gutenberg*, *license* and *you* counted toward vocabulary, coverage and
Browse, and the NOUN *back* from post-licence illustration pages was the most
frequent dictionary miss. The same happens in every language, and Gutenberg
EPUBs are common in the Calibre libraries Books are pulled from.

A probe of seven Gutenberg EPUB3s (French, German, Italian, Spanish, English)
found:

- Every file has a spine unit with manifest id `pg-header`. It holds an English
  preamble of about 800 characters, then a `*** START OF THE PROJECT GUTENBERG
  EBOOK … ***` line, then book text. In Werther (eBook 2407) it holds the
  entire work, so excluding the header unit whole would drop the Book.
- Every file has exactly one `*** END OF THE PROJECT GUTENBERG EBOOK … ***`
  line, at the start of the unit holding the licence. That unit's manifest id
  is `pg-footer` in some files and a generic `itemN` in others (eBooks 1012 and
  2000).
- Illustrated editions add units after the licence unit whose only text is a
  "back" link.
- Some files carry a landmarks nav that is really a page list, with no
  `bodymatter`. None has an EPUB 2 `guide`.

ADR 0066 excludes only what the EPUB declares and never infers from text. The
marker lines are text, but they are a fixed, published packaging convention of
one distributor, not a judgement about what the text says.

## Decision

### Distributor text

**Distributor text** is text a distributor wraps around the work, recognised
only from that distributor's published packaging convention. It is never part
of the main text. Project Gutenberg is the only recognised convention.

### Recognition rule

A snapshot follows the Gutenberg convention only when all of these hold:

- exactly one readable unit has manifest id `pg-header`;
- exactly one `*** START OF THE PROJECT GUTENBERG EBOOK … ***` line occurs in
  the snapshot, and it is in the `pg-header` unit;
- exactly one `*** END OF THE PROJECT GUTENBERG EBOOK … ***` line occurs in the
  snapshot, after the START line, in the same unit or a later one.

The `pg-footer` manifest id is not required. If any condition fails, nothing is
recognised as Distributor text and selection is exactly as ADR 0066 decides.
Unit text is never otherwise inspected.

### What is excluded

- In the `pg-header` unit, everything up to the end of the START line.
- In the END line's unit, everything from the start of the END line.
- Every readable unit after the END line's unit, whole.

### Mechanism

- Whole-unit exclusions are removed from the analysed units, as ADR 0066 does.
- Inside the header and END units, the unit text is analysed unchanged, and
  every sentence whose span overlaps Distributor text is dropped before
  tokens, lemmas and the artifact are derived. A sentence running across a
  marker line is dropped. Snapshot text and offsets are never altered, so
  source locations and reader spans are unaffected.
- When the EPUB also declares a `bodymatter` landmark, both rules apply:
  Distributor text is removed from the landmark-selected run.
- The selection stays derived, not persisted, and is computed from the
  immutable snapshot at analysis time.

### Identity and provenance

- When recognised Distributor text excludes anything, the run's selection
  configuration identity is `selection-distributor-gutenberg-v1`, or
  `selection-maintext-landmarks-distributor-gutenberg-v1` when landmark
  selection also applies. Otherwise the ADR 0066 identities are unchanged.
- The processing-history detail records the recognised convention, the trimmed
  offset ranges in the header and END units, and the excluded unit ids.
- There is no learner-facing surface for the selection.

### Existing Books

ADR 0107 treats a published selection identity as current whenever the
analyzer still produces it. That no longer holds: a Book analysed under
`selection-default-v1` may now select differently. The Analysis update check
derives the selection identity that applies to the Book's unchanged snapshot
now, and the Book is offered an **Analysis update** when it differs from the
published one. The text scan runs only for snapshots that have a `pg-header`
unit. All of ADR 0107's rules apply unchanged: learner confirmation,
eligibility, unchanged evidence until publication, and frozen boundaries.

## Consequences

- A Gutenberg Book's header, licence and post-licence pages contribute no
  tokens to vocabulary, coverage, Browse or cards. The Book text is analysed
  in full.
- EPUBs with declared landmarks and no Gutenberg packaging, and non-Gutenberg
  EPUBs without landmarks, select exactly as before and keep their identities.
- Short English credits after the START line ("Produced by …", transcriber's
  notes, an English contents list) remain, because they carry no marker.
  Catching them would mean inferring from text. This is a known limitation.
- A Gutenberg file that breaks the convention (a missing or repeated marker, or
  no `pg-header`) is analysed as today. False recognition is a correctness bug;
  the fail-safe is the ADR 0066 selection.
- Coverage bases differ between Books whose Distributor text was removed and
  those not yet updated, as ADR 0066 already accepts for landmark selection.
- Other distributors' conventions need their own decision; this ADR does not
  generalise to marker-free heuristics.

## Alternatives considered

- **Exclude the `pg-header` unit whole.** Drops the entire work when the book
  text shares that unit, as in eBook 2407.
- **Recognise the footer by manifest id `pg-footer`.** Misses the files whose
  licence unit has a generic id.
- **Trim unit text before analysis.** Shifts offsets within the unit or forces
  an offset base through the analyzer contract, for about 800 characters per
  Book.
- **Text heuristics for English or licence language.** Rejected for the same
  reason ADR 0066 rejected the classifier: it can drop real text.
- **Lazy adoption only.** Already-analysed Gutenberg Books would keep the
  English vocabulary until some unrelated change caused reanalysis, which in
  practice may never happen.

## Related

- [ADR 0066: Identify a Book's main text for analysis from declared EPUB structure](0066-main-text-selection-from-epub-structure.md)
- [ADR 0107: Analysis pipeline identity and learner-confirmed Analysis updates](0107-analysis-pipeline-identity-and-analysis-updates.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)
- [Main text selection feature](../features/main-text-selection.md)

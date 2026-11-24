# ADR 0125: Construction parallels in Concordance

Status: **Accepted; implemented** · Date: 2026-11-24 · Author: Justin + OpenCode

Decided by Justin in
[Grilling: Concordance construction parallels — query, scope and ranking](https://github.com/justin-hayes/mouseion/issues/2610#issuecomment-7106742314),
under [Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597).
Implements [ADR 0110](0110-philological-reading-competence-boundary.md)'s
"parallels within the author's works, then across the study language". Narrows
the `CONTEXT.md` **Concordance** rule that excluded occurrences are hidden from
every query. The feature is specified in
[Ancient Greek syntax and constructions](../features/ancient-greek-syntax-and-constructions.md#concordance-parallels).

## Context

Concordance looks up a lemma or a form (`match=lemma`, `match=form`), with a
captured current-Book priority, a revision guard and 25-row global paging
([Vocabulary Browse and Concordance](../features/vocabulary-browse-and-concordance.md)).
`ConcordanceLookup` has no author, Work or construction predicate, and the old
relation queries were retired in #1614.

[ADR 0123](0123-constructions-are-a-derived-reading.md) stores construction
occurrences per run with a readiness marker. A Greek Book's Work and author are
reachable through Book → Reading unit → Edition → Work (`works.author`). Lemma
corrections and Occurrence exclusions are vocabulary decisions about words, and
construction detection ignores them.

## Decision

1. **A third match kind, not a new mode.** `match=construction`, with the
   construction id as the term and an optional subtype. It reuses the lookup URL,
   captured-priority slot, revision guard, 25-row global paging and KWIC
   rendering. The search box never recognises a construction name. The match
   kind is offered only for a language whose analysis has a construction scheme
   (`grc` today); for any other language a construction URL gets the existing
   safe-restart notice.
2. **Entry point: Sentence study only.** A learner reaches parallels from a
   construction's **Parallels** link. Every query therefore has an origin
   sentence, which supplies the origin Book and author. There is no construction
   picker on the Concordance page yet.
3. **Three labelled scope tiers in one global sequence:**
   1. **This Book**: the origin Book, captured like today's priority Book.
   2. **This author**: the author's other Books. Other Reading units of the same
      Work come first, then the author's other Works by title. Same-Work-first is
      an order inside the tier, not a fourth tier.
   3. **Other authors**: by author, Work title, then Reading unit order.

   The author is matched by exact equality on the reviewed catalog's Work author
   string. This is Classical- and Greek-specific, not an author identity. A Book
   with no Work has no author tier and goes from "This Book" to "Other Books".
4. **Textual order inside a tier:** Work, Reading unit, sentence, anchor token.
   No "best example", quality or nesting rank, which would be a new unmeasured
   judgment on top of an interim-accuracy detector.
5. **The origin occurrence is omitted** (same run, sentence, construction and
   anchor). The results heading links back to the origin sentence.
6. **Filters.** An optional lemma filter applies to the **anchor's** effective
   lemma only, never to other extent tokens. An optional subtype filter is
   offered only for shipped Subtype labels. Parallels opens construction-only and
   offers "only with {anchor lemma}"; a labelled subtype opens filtered to it and
   offers "all {construction}".
7. **Corrections and exclusions.** A construction-only query applies neither. A
   construction + lemma query treats the anchor exactly as a lemma lookup does:
   corrected effective lemma, and an excluded anchor is hidden. Excluded
   occurrences are therefore hidden from every **word** query, not every query.
8. **Books that can't contribute are named.** One quiet line names the
   study-language Books with no construction evidence, for example "Not
   searched for constructions: *Apology* — Analysis update available". This
   covers runs on the old `absent` interpretation and any Stanza-path `grc`
   Book, following the "unavailable is not zero" rule. A rebuilding run stays
   silent ([ADR 0123](0123-constructions-are-a-derived-reading.md)).
9. **Revision guard.** For a construction query, the evidence revision also
   covers the construction detector version and each eligible run's readiness
   marker, so a rebuild mid-paging returns *Current evidence changed* with
   Refresh.
10. **Rows.** Each KWIC row is centred on the anchor. The expanded row shows the
    Sentence study construction extract as-is. **Study** opens Sentence study
    with that construction focused.

### Query shape (ADR 0038 brief)

- Construction rows carry a denormalised `language`, indexed on (owner,
  language, construction id, subtype) ([ADR 0123](0123-constructions-are-a-derived-reading.md)).
- The query follows `ListVocabularyConcordance`'s fenced stages: candidates
  from construction rows, then eligible current analyses with a current
  readiness marker, then the tier join through Reading unit → Edition → Work,
  then rank and page at 25.
- A lemma filter still starts from construction rows and filters on the anchor
  token, keeping one plan shape.
- Eligibility matches the existing three statements of Concordance eligibility
  (the view, the listing and `ConcordanceLemmaEvidenced`).

## Alternatives considered

- **A separate construction mode or page:** rejected; it would duplicate paging,
  the revision guard and KWIC rendering.
- **Recognising construction names in the search box:** rejected; it overloads
  word search and needs a construction vocabulary in every language.
- **Ranking by example quality or nesting:** rejected for the first slice (see
  decision 4).
- **Applying exclusions to construction-only queries:** rejected; an exclusion
  is a vocabulary decision about one word, and detection ignores it.
- **An author identity table:** deferred until Latin or learner-added Works
  need one.

## Consequences

- Concordance gains a third match kind whose results depend on derived rows and
  their readiness.
- The `CONTEXT.md` **Concordance** entry changes "excluded occurrences are hidden
  from every query" to "from every word query".
- Learners with Books still on the `absent` interpretation see them named, with
  the Analysis update as the route.

**What would reopen this:** an author identity beyond the curated catalog's
string (Latin, or learner-added Works); a construction picker without an origin
sentence; a need to rank examples by quality.

## Related

- [ADR 0123](0123-constructions-are-a-derived-reading.md): the rows queried.
- [ADR 0116](0116-classical-catalog.md): Works, Editions and Reading units.
- [Vocabulary Browse and Concordance](../features/vocabulary-browse-and-concordance.md):
  captured priority and paging reused here.

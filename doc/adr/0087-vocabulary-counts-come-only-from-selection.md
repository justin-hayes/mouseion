# ADR 0087: Vocabulary counts come only from selection

Status: **Proposed** · Date: 2026-10-10 · Author: Justin + OpenCode

Tracked by [issue #1682](https://github.com/justin-hayes/mouseion/issues/1682). Amends [ADR 0084](0084-browse-effective-count-projection.md).

## Context

The vocabulary eligibility rule decides which analyzed occurrences count as vocabulary and under which effective vocabulary identity: confirmed Lemma corrections replace the lemma, Occurrence exclusions contribute nothing, only content parts of speech qualify, separable particles are skipped, and the lemma must contain a letter. Go `selection.Project` implements it; candidate generation and the Current reading snapshot freeze use it.

Other consumers re-encoded the rule or skipped it:

- The Browse count projection builder and its per-decision recompute each repeat it as a SQL predicate, with a letter test (`[[:alpha:]]`) that is not Go's.
- Reading chooser coverage bands and Reading view coverage count raw corpus lemmas, ignoring corrections, exclusions, and particles, while My Books coverage reads the projection. The same Book can show two different coverage figures.
- Lemma review impact counts raw corpus lemmas and hard-codes the three-occurrence threshold, missing the two-occurrence cross-Book route of ADR 0085.

ADR 0084 already requires the projection to use the same eligibility rules for new analyses and rebuilds, but it limits adoption: "Browse alone adopts this count projection initially; other vocabulary consumers continue to use their existing evidence queries."

## Decision

1. **The rule lives only in `selection`.** SQL may fetch persisted tokens, sentences, and occurrence decisions, and may store and aggregate counts, but it never filters on vocabulary eligibility or derives an effective vocabulary identity. The projection builder and the per-decision recompute run `selection.Project` over persisted facts. The recompute projects only the occurrences of the affected identities; `Project` decides each occurrence independently, so those counts are exact.
2. **Every consumer of per-Book effective vocabulary counts reads the projection.** This replaces ADR 0084's "Browse alone" sentence. Browse, My Books coverage, Reading coverage, and lemma review impact share one per-Book count; Reading coverage withholds a band and reports Updating while a Book's projection is not ready, as Browse and My Books do.
3. **Thresholds are `selection`'s.** Lemma review impact and the snapshot freeze use the same Reading eligibility function (three occurrences in the Book, or two with ten across the learner's currently analyzed Books in the language).
4. **A rule change bumps the builder version.** Existing readiness markers then become stale and the durable rebuild of ADR 0084 repairs every Book; this decision's first implementation does so.

Concordance is out of scope. It deliberately lists every occurrence, including non-vocabulary ones, and only shares the correction and exclusion lookup.

## Consequences

- A performance fix may not reintroduce a SQL eligibility predicate; it changes how facts are fetched or how counts are stored instead.
- Builds and recomputes move token rows into the Go process. Builds run in background jobs; recomputes stay proportional to the affected identities, not the Book.
- The first deploy rebuilds every projection. Browse, My Books coverage, and Reading coverage report Updating per Book until its rebuild completes, and the snapshot freeze's two-occurrence route counts only ready Books, as it already does.
- Counts can change slightly where the SQL and Go letter tests disagreed, and Reading coverage changes for Books with corrections, exclusions, or separable particles.

## Related

- [ADR 0081: Learner-owned occurrence lemma corrections](0081-learner-owned-occurrence-lemma-corrections.md)
- [ADR 0084: Project effective vocabulary counts for Browse](0084-browse-effective-count-projection.md)
- [ADR 0085: Reading owns Book vocabulary and deck preparation](0085-reading-owned-book-vocabulary.md)

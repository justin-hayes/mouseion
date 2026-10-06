# ADR 0084: Project effective vocabulary counts for Browse

Status: **Accepted (partially implemented; corpus-scale acceptance measurement pending)** · Date: 2026-10-03 · Author: Justin + OpenCode

## Context

Vocabulary Browse ranks the Current reading's Book by its eligible effective
occurrences, then by counts across the owner's currently analyzed Books. Its
request-time evidence query times out on a local corpus of roughly 3.2 million
tokens. Restricting cross-Book aggregation to identities in the Current Book
still exceeded a nine-second query deadline: a long Book shares enough
identities with other Books that it still reads substantial token evidence.
The accepted [Browse contract](../features/vocabulary-browse-and-concordance.md)
requires exact, current counts and a warm reference-corpus p95 of at most two
seconds; a timeout cannot become a partial or zero result.
The prohibition on persisting the retired advisory Journey-order projection
in [ADR 0037](0037-cross-book-projection-advisory-ordering.md) does not govern
this Book-level count read model.

## Decision

Introduce an owner- and Book-scoped derived count projection for Browse, keyed
to the exact current completed analysis and effective `(language, canonical
lemma, analyzed POS)` identity. It stores eligible occurrence counts, not
source evidence, Known/Reserved state, or Book-deck inclusion. Analyzer tokens,
selected analysis scope, and confirmed occurrence decisions remain authoritative.
Browse alone adopts this count projection initially; other vocabulary consumers
continue to use their existing evidence queries.

Build counts from persisted corpus tokens and sentences, the exact selected
source units/snapshot, and applicable owner decisions, using the same evidence
eligibility rules for new analyses and background rebuilds. Do not separately
derive new-run counts from the analysis worker's in-memory NLP result: that
would create two implementations of the count rules and make a rebuild unable
to reproduce a published count. The builder must be able to read a completed
run before it becomes the Book's current-analysis pointer.

Complete and retain a new run's immutable corpus before projecting its counts.
Durable finalization work builds those counts and promotes the analysis with
its ready projection atomically; a failed projection retries finalization
without rerunning NLP. Until promotion, the preceding matching current
analysis remains authoritative where eligible; a changed source must not
resurrect stale evidence. If no prior matching analysis remains, expose a
distinct pending or failed finalization state rather than calling the Book
analyzed. When a learner corrects or excludes an occurrence in a Book with
ready counts, update affected projected counts in the same transaction as the
decision, or roll back both. During the initial rebuild, when a Book has no
ready counts yet, allow the decision to commit, invalidate
any in-progress rebuild for that Book, and retry against the new decision;
Browse remains gated until the retry completes. Do not represent an unready or
inconsistent projection as current data.

The rebuild must not hold a Book's write lock for its full evidence scan. It
builds from a consistent snapshot, then briefly locks and verifies the exact
current analysis and decision revision before publishing. If either changed,
discard the candidate build and retry; a concurrent correction must not wait
for a long scan or be overwritten by a stale build.

Store counts per Book, not a second maintained language-wide total. Browse
derives across-Book counts by summing projected rows for identities evidenced in
the Current Book, including every identity needed for accurate pagination and
tie-breaking, not only the displayed page. Existing analyzed Books are rebuilt
by automatically enqueued, durable River work per Book; readiness and failures
are observable, and operators can retry failed work. Until all relevant current
Books in the owner's study language have complete counts, Browse withholds
incomplete results and reports **Updating**. If rebuilding
repeatedly fails, it reports a distinct unavailable state with a recovery path
after bounded automatic retries, rather than indefinitely claiming progress.

The cutover is contingent on measuring representative 500-Book/50-million-token
Browse requests against the feature's p95 target. If summing per-Book rows misses
that target, evaluate a language-wide rollup separately rather than adding a
second set of mutable counts without evidence.

## Consequences

- Browse does not recompute owner-wide vocabulary by joining raw token and
  sentence rows on every request.
- Analysis publication and occurrence decisions carry additional derived-data
  work; failures must preserve the completed corpus and previous eligible
  visible counts, not publish incomplete results or repeat NLP analysis.
- The backfill is a separate, observable, retry-safe data operation from schema
  creation. It needs a completion check covering every current Book that can
  affect Browse's across-Book counts, and a fenced publish step so retries and
  concurrent decisions cannot overwrite newer counts.
- The projection needs a readiness marker even for a Book with zero eligible
  identities; missing rows alone cannot distinguish an empty Book from an
  unbuilt projection. A change to eligibility rules invalidates affected
  markers and requires rebuilding before Browse can use the new rules.

## Related

- [Vocabulary Browse and Concordance](../features/vocabulary-browse-and-concordance.md)
- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0078: Book dispositions and current reading](0078-book-dispositions-and-current-reading.md)
- [ADR 0081: Learner-owned occurrence lemma corrections](0081-learner-owned-occurrence-lemma-corrections.md)

# ADR 0081: Learner-owned occurrence lemma corrections before vocabulary freeze

Status: **Accepted (implemented)** · Date: 2026-09-28 · Author: Justin + OpenCode

Amended by [ADR 0086](0086-reading-working-desk-hidden-visibility-and-concordance.md) §9: "explicitly stopping without completion" now reads End current reading. Corrections still never rewrite active snapshots.

Amends [ADR 0005](0005-vocabulary-identity-normalization-ranking.md) only in
how a learner's effective identity for an analyzed occurrence is derived;
the canonical normalization profile and immutable analyzer evidence remain
unchanged. Preserves [ADR 0078](0078-book-dispositions-and-current-reading.md)'s
frozen current-reading vocabulary and [ADR 0071](0071-decouple-deck-data-from-presentation.md)'s
immutable deck specification. Follows [ADR 0024](0024-learner-owned-catalogs-no-admin.md):
there is no in-app administrator role.

## Context

The analyzer can assign `Drach` to an occurrence of `Drachen` whose sentence
means *dragon*. Checking only for a Kaikki entry does not prove an error:
`Drach` is absent in the local index, while `Drache` and `Drachen` both have
dragon-related evidence, and valid compounds may also be absent. Replacing
every surface match, or changing an identity during deck translation, risks
conflating a *dragon* occurrence with one meaning *kite*. More importantly,
Reading freezes vocabulary before deck preparation: a deck-time replacement
would disagree with Reserved vocabulary and later Known graduation.

## Decision

Treat a possible lemmatization error as a **learner review flag**, never a
machine verdict. For an exact analyzed Book, a learner can keep the analyzer
assignment, replace the canonical lemma for one occurrence, or exclude that
occurrence from vocabulary selection. A **lemma suggestion** from an index,
fuzzy comparison, or optionally an on-demand LLM is non-authoritative until
the learner confirms it; a missing dictionary entry or frequency prior does
not certify either the analyzed or suggested lemma. Permit well-formed manual
corrections even when no dictionary entry exists. A learner may inspect an
unflagged content-word occurrence by exact observed form, and may explicitly
select additional reviewed occurrences for the same decision. The first
workflow does not change analyzed POS or introduce an administrator role.

Preserve the shared normalized corpus, source spans, raw analyzer lemma, and
profile version. Record durable, owner-scoped decisions for occurrences in one
Book's exact analysis and derive **effective vocabulary identities** from
that evidence plus those decisions. Use effective identities consistently for
owner-specific occurrence counts, the recurring floor, Known/Reserved
eligibility, vocabulary insights, sentence selection, current-reading
snapshot, and prepared-deck specification and enrichment. Exclusion omits an
occurrence from vocabulary selection while preserving source evidence and
the analyzable-token coverage denominator; it does not count it as Known or
Reserved or inflate coverage. Corrections and exclusions do not apply
automatically to another occurrence, Book, learner, or subsequent analysis.
If a dictionary or model refresh produces new evidence for the same analysis,
keep the learner's decision; do not reopen it as a blocking flag.

Prioritize only high-risk automatic flags. The first detector is German-only:
an exact Kaikki miss needs a credible competing lemma, not merely absence,
and plausible corrections that could cross the recurrence threshold are in
scope. Record flags with their evidence provenance and resolution state for
the exact analysis; no flags because evidence was unavailable must not be
presented as a confirmed clean analysis. Before either Start reading or
direct Prepare deck freezes identity, pause on unresolved high-risk flags
known for that analysis; the learner resolves them by keep, correct, or
exclude. The review and both freezes must agree on the same effective
vocabulary even under concurrent requests. If index evidence is unavailable,
do not manufacture flags or prevent manual review; a later flag cannot
retroactively modify an active reading. An LLM
can be requested during review for a bounded, privacy-preserving suggestion,
but must never be required for Reading or used to change lemma identity in
the already-frozen deck translation call. Manual review works in German,
Italian, and Modern Greek; automatic flags expand only after language-specific
precision review.

Decisions can be revised freely before the affected current-reading snapshot
or deck specification freezes. After a reading starts, correcting its
vocabulary requires explicitly stopping without completion and starting again
with a new snapshot. If a deck is already ready, accepting a later correction
requires explicit re-preparation into a new generation; the old artifact
remains intact. Where both exist, stop, correct, restart, and re-prepare.
Completed readings and their Known facts are never retroactively rewritten;
historical Known reconciliation is separate work. Re-importing an
identity-changed deck may leave the old note in the learner's Anki collection,
so communicate the manual cleanup rather
than promising automatic deletion or an in-place note update.

## Why this trade-off

Owner-scoped occurrence decisions preserve faithful analyzer evidence and
handle context-sensitive homographs without forcing a dangerous global
spelling map, a new administrator role, or an external call before Reading.
The cost is a durable derived-vocabulary layer and an explicit review gate
for genuinely high-risk candidates. That gate must be conservative: before
German activation, reviewed fixtures including the `Drach` example, valid
unindexed compounds, and ambiguous `Drachen` uses must have no incorrect
blocking flags. False negatives remain manually correctable. Exact analyses,
snapshots, prepared artifacts, and completions retain their historical
provenance rather than being silently backfilled.

## Related

- [Learner review of analyzer lemmas](../features/lemma-review-and-correction.md)
- [ADR 0018: Remove global frequency dataset](0018-remove-dwds-frequency.md)
- [ADR 0065: Canonicalize pre-1996 German ß spellings](0065-german-pre-1996-sharp-s-canonicalization.md)
- [ADR 0076: Roll forward when a ready deck requires re-preparation](0076-reprepare-ready-deck.md)

# ADR 0095: Greek personal counts and deferred external frequency

Status: **Withdrawn, never implemented. Superseded by spec [#2098](https://github.com/justin-hayes/mouseion/issues/2098); background in [#2097](https://github.com/justin-hayes/mouseion/issues/2097).** · Date: 2026-10-14 · Author: Justin + OpenCode

**Candidate-scope amendment:** [ADR 0098](0098-shared-core-classical-acquisition.md)
reconciles the historical human-reviewed Greek MVP requirement below with the
human-authorized automatic-evidence scope. Its documentation review is pending;
personal-count, identity and frozen learner-state contracts are unchanged.

Recorded through the live decision in
[Choose how Greek corpus evidence relates to personal vocabulary counts](https://github.com/justin-hayes/mouseion/issues/2029).
Justin accepted all four recommendations and confirmed the consolidated contract.
This records planning only, not production code, schema, backfill, data acquisition,
or deployment authorization. Repository promotion requires human review.

## Context

[ADR 0090](../../adr/0090-library-total-book-vocabulary-selection.md) selects vocabulary
using the learner's currently analyzed same-language Books, not global frequency.
Its 3/15 thresholds were measured on German and Italian, not Ancient Greek.
[ADR 0093](0093-classical-identity-and-reading-units.md) permits distinct
edition/unit Books whose text overlaps; shared sources do not share learner state.

The reviewed
[source inventory](https://github.com/justin-hayes/mouseion/issues/2022#issuecomment-6161601492)
identifies Diorisis, GLAUx and OGA as possible external evidence sources, not
approved interchangeable frequency datasets. Their reported word/token sizes
are not directly comparable. The reviewed
[matching evidence](https://github.com/justin-hayes/mouseion/issues/2024#issuecomment-6161715262)
does not establish corpus-wide token correspondence or deduplicated overlap.
[Licensing policy](https://github.com/justin-hayes/mouseion/issues/2035#issuecomment-6163336026)
requires output-specific rights review for statistical artifacts.

## Decision

### Personal counts only in the first Greek slice

Use **Personal analyzed-Book occurrences**, not external corpus frequency, for
the annotation-backed Ancient Greek MVP. Defer external frequency selection,
advisory ranks/counts, and frequency-dataset acquisition. No external dataset is
selected or approved. This does not restrict acquisition of a selected Book's
text and annotation evidence under its separately reviewed contracts.

Repository sync, catalog discovery, and acquisition or reuse of shared evidence
do not themselves make external texts participants in a learner's personal
counts. A contribution belongs to that owner's Book with usable current analysis,
not to every text offered by a repository or available in a shared evidence store.
Other owners' Books never contribute; `grc` remains separate from `el`.

### Retain the provisional 3/15 rule

An eligible Unknown identity enters a new Current-reading vocabulary snapshot
when it occurs at least three times in that Book, or at least once there and
at least fifteen times across the owner's currently analyzed same-language Books,
including that Book. An identity absent from the Book never qualifies.

Inherit ADR 0090's contributing-Book and count-readiness rules: Hidden, Inbox,
To Read and Read Books contribute when their analysis is current, complete and
source-matching. Cross-Book selection waits for the required exact count
projections rather than treating pending or unavailable counts as zero. Apply
confirmed Lemma corrections and Occurrence exclusions to effective counts;
Known and Reserved identities remain excluded from selection. Generated
vocabulary is provenance, not an exclusion state.

Three and fifteen remain provisional absolute thresholds, without new learner
controls, Greek-specific weights or a coverage guarantee. Greek-specific
measurement is required before later retuning, especially given smaller bounded
Reading units. No Greek calibration has been demonstrated by this decision.
Retuning applies to new snapshots only; modern-language changes require a
separate explicit decision.

### Count occurrences equally, without conflating evidence

Every admitted vocabulary-ready personal Book contributes ordinary eligible
occurrence counts. Do not weight or filter these counts by human, human-reviewed
automatic, automatic or unknown provenance. Technical vocabulary readiness is
independent of review status under
[ADR 0094](0094-source-independent-annotation-and-provenance.md); the
human-reviewed Greek MVP still has its own candidate-specific admission
requirement. Provenance remains inspectable at its documented layer and scope;
syntax review does not establish lemma review or statistical accuracy.

Aggregate only established effective canonical identities under the accepted
[Greek normalization decision](https://github.com/justin-hayes/mouseion/issues/2033#issuecomment-6163487386).
Matching lemma strings or native POS labels do not resolve scheme, UPOS or
scheme-qualified lexical homograph differences. Unresolved mappings cannot be
used to pool distinct identities. Count eligible text-attested lexical occurrences,
not additional multiword containers or unprinted nodes; retain segmentation
provenance rather than imply producer-independent totals.

### Accept separate contributions from editions and overlapping units

Keep ADR 0090's per-Book summation for Greek. Verified aliases for the same
Edition and Reading unit resolve to one owner's Book under ADR 0093. Distinct
editions, distinct bounds and uncertain identities remain separate Books; do not
deduplicate their counts using work identity, CTS strings, similar text or bytes.

For example, Book I and Books I–II are distinct Books. A shared passage's
occurrences contribute through each usable current analysis. Two editions can
likewise inflate totals. This is an accepted MVP limitation, not a claim that
the learner encountered those passages twice. Describe the total as personal
analyzed-Book occurrences, not unique textual encounters or unbiased Greek
frequency. No cross-edition alignment or passage-deduplication mechanism is
authorized here.

### Preserve frozen evidence and learner state

Count/provenance changes, newly analyzed Books, source refreshes and future
policy changes cannot rewrite active or historical snapshots, reservations,
deck specifications, completions or Known facts. Corrections remain owner- and
exact-analysis-scoped. Preserve owner-scoped evidence adoption and the
End → review → Start route for changing active-reading vocabulary. This decision
changes no existing modern-language behavior or stored learner state.

## Trade-offs and remaining scope

Advisory external frequency would still require a trustworthy interpretation,
not merely a UI label. Pooling corpora could double-count editions, passages and
imported treebanks, conflate lexical schemes, and hide genre, author, period,
coverage and machine-annotation biases. These uncertainties remain unresolved,
not approved by deferral. Any future external-frequency extension needs its own
explicit dataset, composition, duplication, mapping, provenance, statistical
meaning and output-rights decisions; it cannot silently replace personal counts.

Retaining the shared rule avoids speculative weighting and alignment work but
accepts library-size dependence, edition/overlap inflation and uncalibrated Greek
thresholds. Candidate-specific readiness and review validation remain with
[Choose the smallest useful annotation-backed Ancient Greek slice](https://github.com/justin-hayes/mouseion/issues/2030).
Measurement/acceptance planning and implementation implications remain with
[Validate the shared-core architecture and staged implementation handoff](https://github.com/justin-hayes/mouseion/issues/2031).
External frequency is outside this MVP handoff, not remaining fog on its route.

This confirms ADR 0018/0090's personal-library direction and settles ADR 0093's
Greek overlap question without changing the production selection implementation.

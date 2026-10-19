# ADR 0107: Analysis pipeline identity and learner-confirmed Analysis updates

Status: **Accepted — implementation pending** · Date: 2026-10-19 · Author: Justin + OpenCode

Decided in triage of [#2240](https://github.com/justin-hayes/mouseion/issues/2240).
Extends the reanalysis rules of [ADR 0028](0028-explicit-scoped-analysis-lifecycle.md)
and [ADR 0040](0040-one-current-analysis-per-book.md), follows the adoption
boundaries of [ADR 0103](0103-source-structure-and-poetry-evidence.md), and
corrects the re-analysis claim in [ADR 0102](0102-french-language-support.md).

## Context

The NLP producer versions its own behaviour: each language's normalization
profile carries a version, and French moved from 1 to 2 for the dialogue
segmentation repairs (#2234, #2235). The persisted artifact records that
version, but nothing reads it to decide whether a Book needs reanalysis.

Analysis runs are deduplicated on owner, content revision, analyzer name and
version, and selection configuration identity. The NLP normalization version
is not part of that key. Submitting analysis for a Book whose run completed
returns the existing run, and no learner surface offers Re-analyze for a
completed analysis: the action appears only for stale evidence and failed or
cancelled runs. Job retry never reruns a completed analysis.

The only lever is a hand-bumped Go-side analyzer version, which #2234 and #2235
did not bump. Even when bumped, a changed run identity does not mark the Book
stale or offer anything, so the learner cannot reach the new analysis. ADR 0102
claimed existing French Books "pick up the change through the existing
re-analyse action"; that was never true.

Unlike a Source structure update, nothing about the Book's content or
extraction changes. The published analysis remains valid evidence for the
current content; it came from an older pipeline. Detecting an actual
difference, as ADR 0103's check does by re-extracting, would mean running the
NLP over every Book, so a version comparison can only say the output *may*
differ.

## Decision

### Analysis pipeline identity

An analysis's **Analysis pipeline identity** is the Go analyzer name and
version and selection configuration identity, together with the language's
NLP normalization profile name and version.

- The running NLP service reports each language's normalization profile name
  and version in its capabilities, alongside the existing model version. This
  is an additive proto change; the current identity is discovered from the
  running service, not from a web-app constant.
- Analysis-run deduplication includes the normalization profile and version,
  so an analysis under a newer identity creates a new run, while double submit
  and job retry under the same identity converge on one run.
- A run created before this decision has its identity read from its published
  artifact's recorded normalization profile and version plus the run's
  analyzer version and configuration identity. No backfill is needed.
- The model version and dependency versions are **not** part of the identity.
  A model or dependency bump only produces an update when a developer judges
  that it can change evidence and bumps the normalization version.
- The normalization version is the producer's declaration that evidence may
  change. Bump it whenever normalization, segmentation or other analyzer output
  that can change a Book's evidence changes. Go-side analyzer versions no
  longer need hand-bumping for NLP-side changes; they still mark Go-side
  analysis contract changes.

### Analysis update

A Book is offered an **Analysis update** when the Analysis pipeline identity of
its published analysis differs from the current one.

- **Offer only.** Nothing is queued automatically, for any Book. The learner
  confirms each update, as in ADR 0103.
- **Not stale.** Analysis evidence stays *analyzed*. Counts, coverage and
  readiness are unchanged, and the published analysis stays in effect while the
  update is queued, running, failed or cancelled, under the existing
  shadowed-reanalysis rules. When the update publishes, ADR 0040's replacement
  semantics apply.
- **Eligibility.** Only To Read Books outside the active current Reading are
  offered the action. For the current Reading's Book, the notice says to End
  the Reading first; after End the Book is To Read and the offer applies.
  Restarting uses the new evidence after re-preparation. Other dispositions get
  no offer.
- **Unknown identity offers nothing.** If the NLP service is unreachable, the
  Book's language is not ready, or capabilities do not report the language, the
  Book is treated as current.
- **Imported analyses are excluded.** Imported Ancient Greek analyses have no
  NLP producer and are never offered an update by this mechanism.
- **A current analysis is a no-op.** Submitting analysis for a Book whose
  published identity matches the current one returns the existing run.

### Learner surface and confirmation

The offer is a Book-level notice, "A newer analysis is available", on the
Reading chooser item and the Book's My Books lifecycle card. There are no
per-card or app-wide banners. The action leads to the existing
consequential-confirmation pattern, which states that:

- the published analysis stays in effect until the update publishes,
- the number of confirmed Lemma corrections and Occurrence exclusions on the
  published analysis, which do not carry over to the new analysis, and
- prepared decks and earlier Readings keep the evidence they froze.

The confirmation carries the published run and the target Analysis pipeline
identity it offered. On confirmation, the owner/source transaction locks the
Book's source and revalidates both. If the published analysis changed or the
service now reports a different identity, nothing is submitted and the learner
sees the refreshed offer. The flow works without JavaScript.

### Frozen boundaries

An Analysis update never remaps Lemma corrections or Occurrence exclusions, and
never rewrites Reading vocabulary, Reserved or Known facts, completed history,
current-reading snapshots, corpora or prepared decks. An active Reading keeps
its exact evidence and preparation.

## Alternatives considered

- **A single Go-side "pipeline generation" constant.** Simpler, but it repeats
  the failure that left #2234 and #2235 unreachable: the NLP change ships and
  nobody bumps a constant in another language's codebase.
- **Including the model version.** Every routine dependency bump would mark
  every Book in the language outdated, whether or not evidence changed.
- **Automatic reanalysis of non-current Books.** ADR 0103 already rejects
  automatic reanalysis. It would silently replace evidence on learner surfaces
  (ADR 0040), drop Lemma corrections without disclosure, and queue CPU-heavy
  work for a whole library on each normalization bump.
- **Marking outdated evidence stale.** The published analysis still matches the
  current content. Making it stale would remove the Book from counts and
  Current reading eligibility for a change that may not alter its evidence.
- **Comparing actual analysis output before offering.** Accurate, but it runs
  the NLP over every Book to decide whether to offer running the NLP.
- **Amending ADRs 0028, 0040 and 0102 in place.** Spreads one decision across
  three documents.

## Consequences

- Learners can reach analysis fixes for Books they already analysed, and see
  that one is available, without data loss beyond the disclosed corrections.
- An update offer means the output *may* differ. A Book whose evidence would be
  identical is still offered the update, and accepting it costs one analysis
  run.
- NLP authors own the normalization version bump. A golden-output test that
  fails without a bump is a possible follow-up, not part of this decision.
- The capabilities RPC gains normalization profile fields, and the run
  deduplication key gains the normalization identity. Any required DDL is a
  successor migration that needs CODEOWNER review.
- ADR 0102's statement that existing French Books pick up the #2234 change
  through the existing re-analyse action is superseded: they pick it up through
  an Analysis update once this ADR is implemented.

## Related

- [#2240](https://github.com/justin-hayes/mouseion/issues/2240): triage and
  implementation tracking.
- [ADR 0028](0028-explicit-scoped-analysis-lifecycle.md),
  [ADR 0040](0040-one-current-analysis-per-book.md): analysis lifecycle and
  reanalysis replacement.
- [ADR 0102](0102-french-language-support.md): French normalization version.
- [ADR 0103](0103-source-structure-and-poetry-evidence.md): Source structure
  update adoption, the model for offer, confirmation and frozen boundaries.

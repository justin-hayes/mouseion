# ADR 0042: Derive a per-language corpus view without a persisted corpus object

Status: **Accepted** · Date: 2026-09-02 · Author: Justin + Codex

## Context

Known vocabulary changes the current coverage of every same-language analyzed
book, but Mouseion currently presents that evidence one book at a time.
Cross-book effects are numerically present and experientially absent. The
superseded discovery evidence in
[`corpus-campaign-horizon-discovery.md`](../design/corpus-campaign-horizon-discovery.md)
identified a reading-field lens over books as one way to expose that cumulative
effect; its learner-facing Corpus / Campaign / Reading Horizon architecture is
not current guidance.

ADR 0040's proposed one-current-analysis-per-book contract makes the aggregate
unambiguous: each analyzed book contributes at most one current body of evidence.
The question is whether to persist a new learner corpus, merge book analysis
artifacts, or derive a private comparison lens from existing evidence.

## Decision

Mouseion derives a read model for each `(owner, language)` from:

- the one current analysis per book defined by ADR 0040;
- normalized corpus artifacts containing lemma-level and statistical data only,
  under ADR 0009's privacy boundary; and
- the owner's current language-scoped known vocabulary.

The initial read model presents:

- analyzed-book count;
- aggregate known coverage across the analyzed set;
- highest-impact unknown vocabulary by contribution across that set; and
- per-book spread, retaining each book's identity and evidence state.

It is computed at render time. Coverage and unknown-vocabulary contribution
reuse ADR 0037's owner/language scoping, current-vocabulary treatment, lemma
wildcards, exact occurrence arithmetic, and on-demand projection approach. The
aggregate does not persist a learner-specific mastery snapshot or a new
first-class `Corpus` object.

The language view is an **evidence-only lens**. Scope review, analysis, deck
preparation, and every consequential action stay on `/books/{id}`. The view does
not combine reviewed scopes, merge analysis results, build aggregate decks, or
claim that several books share one analysis. It never pools source text,
sentences, learner state, or aggregate evidence across learners.

Learner-facing copy must not call this surface **Corpus**. That word already
names an immutable internal analysis artifact through `CorpusID` and
`normalized_corpus_artifacts`; using it for a collection lens would collapse two
different meanings. Suitable labels include **Language view**, **Analyzed
books**, and **Coverage across German**. The internal and feature name
**language corpus view** remains acceptable in technical documentation.

The initial home is a per-language panel within My Books. A learner zooms from
its per-book spread into `/books/{id}`. Promoting the lens to a separate
destination requires a future explicit reconciliation at
`information-architecture.md#contract-changes-requiring-planneradr-work`; it is
not implied by this decision.

## Alternatives considered

- **Resurrect Explore or Reading Horizon as primary destinations.** Rejected.
  These are alternatives from superseded discovery evidence, and the canonical
  information architecture explicitly permits only My Books, Reading Journey,
  and Settings plus the distinct Add books action.
- **Persist a first-class Corpus object.** Rejected. This is derived evidence
  over current analyses and vocabulary; render-time computation avoids a new
  lifecycle and stale aggregate state.
- **Merge scopes or analyses and create aggregate decks.** Rejected. These
  actions entangle separate book evidence with deck and known-vocabulary
  contracts and weaken ADR 0028's learner-authority guarantees.
- **Rename My Books to Corpus.** Rejected as historical alternative A from the
  superseded discovery evidence. It would obscure the collection's
  bibliographic role and collide with internal terminology.
- **Make Campaign the sole object or place Books under a strict Corpus parent.**
  Rejected historical alternatives B and C. Mouseion's objects form a
  relationship graph, and this lens neither owns Books nor replaces their
  lifecycles.

## Consequences

- Learners gain a calm same-language view of cumulative evidence without a
  dashboard, recommendation, or new navigation destination.
- The initial implementation has no schema burden; derived values remain
  current as known vocabulary or current analyses change.
- The panel inherits missing, stale, incomplete, and warning states from its
  per-book evidence rather than manufacturing aggregate certainty.
- Promotion to a destination remains deliberately gated by future information-
  architecture reconciliation.

## Open questions

- Which exact aggregate quantities should survive usability testing? The
  analyzed count, aggregate known coverage, highest-impact unknown vocabulary,
  and per-book spread are the starting set.
- When, if ever, should the panel graduate to a destination? Any promotion must
  pass through the documented architecture checkpoint.

## Related

- [ADR 0009: Home-lab authentication and corpus-artifact isolation](0009-home-lab-auth-corpus-isolation.md)
- [ADR 0025: Analysis coverage and threshold metric contract](0025-analysis-coverage-threshold-metrics.md)
- [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](0028-explicit-scoped-analysis-lifecycle.md)
- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0037: Cross-book vocabulary projection and advisory Journey ordering](0037-cross-book-projection-advisory-ordering.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)
- [Language corpus view feature](../features/language-corpus-view.md)
- [Superseded discovery evidence](../design/corpus-campaign-horizon-discovery.md)

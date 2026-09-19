# ADR 0037: Cross-book vocabulary projection and advisory Journey ordering

Status: **Superseded by ADR 0072** · Date: 2026-08-31 · Author: Justin + Hermes

Defines the reproducible cross-book projection and route-comparison objective that
[ADR 0034](0034-reading-journey-identity-ordering.md) deliberately deferred
("efficient ordering is advisory only and covered by a separate projection
contract") and that Reading Journey surfaces as the optional **vocabulary-efficient
alternative** to the learner's canonical order. Recorded for issue #463 as part of
the My Books / Reading Journey / Primary Goal migration (#480); resolves contract
item #6 (cross-book projection and route comparison) and blocker B5.

## Context

Reading Journey is a fluid, provisional, learner-canonical ordering of
learner-selected books. The learner's own order is authoritative; the Journey may
*also* compare it against a vocabulary-efficient alternative that uses only the
same learner-selected books and optimizes exactly one named lexical property. The
canonical architecture ([`doc/design/information-architecture.md`](../design/information-architecture.md))
and the migration plan both insist that this alternative is **advisory comparison
evidence, never an "optimal route" or "best next book"**, and that no composite
difficulty or literary-value score may be invented.

Today there is no cross-book projection: [ADR 0025](0025-analysis-coverage-threshold-metrics.md)
defines exact per-book current coverage, threshold investment, and deterministic
selection semantics, and `analysisinsights` already returns current + active-campaign
projection + thresholds + top-unknowns per book. But none of that orders the Journey.
Extending it to a route comparison requires an explicit contract for the objective,
eligible evidence, threshold and transition assumptions, treatment of incomparable
books, and invalidation — precisely the contract this ADR records.

**[ADR 0036](0036-primary-goal-justified-graduation.md)** governs what may be
treated as a *justified* change to known vocabulary: only the single justified
graduation transition (a `learning_campaign_vocabulary` identity atomically linked
to generated provenance, once confirmed by deck review) adds knowledge. Reading a
book never makes its projected vocabulary known. This ADR inherits that epistemic
rule: the cross-book projection recomputes from current state and may show a
reserved-but-unconfirmed transition only as *conditional*, never as current.

## Decision

### Inputs

The projection is **owner-scoped and language-scoped** (per ADR 0025 and ADR 0034).
Its inputs are:

- the owner's current **Reading Journey membership** in learner-canonical order,
  including any **fixed-order constraints** (positions the learner pinned; the
  current **Primary Goal**, when a Journey member, is anchored at the front as a
  fixed constraint);
- for each member book, its **current analyzed corpus** with reproducible
  analyzable-token occurrence counts and total (ADR 0025 denominator), known-token
  coverage, structural signals (ADR 0026), and analysis/evidence quality;
- the owner's **known vocabulary** (current, language-scoped, including graduated
  entries and lemma-wildcard / empty-UPOS entries), per ADR 0025;
- the **current active/residual campaign reservation snapshot** for the single
  justified graduation (ADR 0036), when present;
- a chosen **planning target** `T` drawn from ADR 0025's targets (95, 97, 99),
  used only to express per-book threshold investment — never the objective itself.

### Comparability: what may be ordered

A Journey book is **comparable** for the route objective only when it has a
**trustworthy current analyzed corpus** that yields reproducible ADR 0025 coverage
and ADR 0026 structural signals in the modeled study language. A book is
**incomparable** and excluded from ordering whenever any of the following hold:

- **unassessed** — no completed analysis instance and no usable corpus;
- **stale** — its corpus was superseded by a newer analyzed scope/reanalysis, or
  its analysis/corpus no longer represents the current book scope;
- **incomplete or low-quality** — the analyzed corpus cannot reproduce coverage or
  structural aggregates;
- **different study language** — coverage is language-scoped, so a book in another
  language cannot be coverage-ranked against books in the modeled language;
- **unavailable** — no current, trustworthy, reproducible analysis result is
  addressable for it.

Incomparable books **stay exactly where the learner placed them**, are excluded
from the ordering objective and from all aggregate totals, and receive **no rank**:
the UI explains the evidence gap rather than moving, demoting, or fabricating a
position for them. The alternative is computed only over the comparable books.

### Objective: exactly one named lexical property

The single named lexical property optimized by the vocabulary-efficient alternative
is **current known-token coverage** as defined by ADR 0025 (analyzable-token
denominator, integer comparison authoritative). Among all orderings of the
comparable books that respect the fixed-order constraints, the objective maximizes
**cumulative current known-token coverage read in order** — equivalently, minimizes
the cumulative unknown-token load encountered at every prefix.

Because reading graduates nothing (ADR 0036), a book's current known-token coverage
is **static for a fixed current vocabulary set**: it does not change with reading
position. The objective is therefore achieved exactly by the greedy ordering that
places comparable books in **descending order of current known-token coverage**
(respecting fixed-order constraints). This is deterministic, reproducible, and
trivially optimal: any divisor of the best-covered prefix can only lower cumulative
coverage at that prefix.

The objective is **not** a "least-difficult route" — it deliberately says nothing
about literary value, desire, or a composite difficulty grade. Structural signals,
evidence quality, and learner desire are never folded into it.

### Deterministic order and tie-breaking

The alternative order is:

```text
primary:    descending current known-token coverage
secondary:  ascending learner-canonical position   (preserve learner preference among equals)
tertiary:   ascending (language, book_id)          (stable, independent of DB row order)
```

The secondary key uses the learner's canonical position, which is itself
deterministic under ADR 0034's comparator, so the result is **stable and
reproducible under any write contention**. The tertiary key guarantees a total order
even when two books share both coverage and learner position at the moment of
computation.

### Current versus projected state; justified versus conditional

Two coverage states are computed for each comparable book and for the ordering as a
whole, and they are **never conflated**:

- **Current state** — coverage from the owner's current known vocabulary, including
  justified graduates (ADR 0025 + ADR 0036). This is the state the objective uses
  and the only state presented as a present fact.
- **Conditional projected state** — coverage recomputed *as if* the current
  active/residual campaign's reserved identities graduated through the single
  justified transition (ADR 0036). This recomputation applies when and only when a
  reservation snapshot is present and its deck review is not yet confirmed. It is
  **explicitly conditional** — "reads without X", a projected-future transition,
  never a current fact, and never the order that becomes active.

A separately labeled conditional variant of the alternative (the advisory order
under the projected graduation) may be shown, and must be marked "if the pending
study is confirmed" and kept visibly distinct from the current order. No knowledge
from reading is ever projected.

**Thresholds** are ADR 0025 threshold-investment values (`95`, `97`, `99`): per
comparable book, the additional lemmas needed to reach the target `T`, using the
full analyzable-token denominator and exact integer comparison. They are supporting
per-book data shown alongside coverage; they are **not** ordering inputs and cannot
change the objective.

### Vocabulary-category treatment

The projection honors ADR 0025 and ADR 0027/0036 category semantics:

- **Graduated known vocabulary** counts as known (current numerator).
- **Current active/residual campaign reservations** are reserved, not known; they
  appear only in the *conditional projected* state (the single justified
  graduation's would-be graduates), never in current coverage.
- **Abandoned campaign vocabulary** returns to unknown/eligible and is not known.
- **Legacy generated rows** are provenance, never known (ADR 0019), and contribute
  to unknown coverage only.
- **Lemma-wildcard / empty-UPOS known entries** match all UPOS for that lemma across
  every comparable book (ADR 0025 wildcard semantics), and so raise current
  coverage wherever that lemma occurs.

### Unavailable-data behavior

Books that are incomparable as defined above are never ranked, never moved, never
counted in totals, and never given a fabricated coverage value. The alternative is
silent or explanatory about them: it reports which books were excluded and why the
evidence is missing, and leaves the learner's chosen order untouched for those
positions.

### Invalidation and recalculation

The cross-book projection and the advisory alternative are **computed on demand
from current state**; they are **never persisted as a projection or stale truth**
(requirement: explanation data without persisting projections). The computed
alternative is invalidated whenever any of its inputs change:

- Journey membership or order changes (add/remove/reorder/pin/Goal change);
- known vocabulary changes (import, justified graduation, abandonment returns to
  eligible, removal);
- the active/residual reservation snapshot appears or resolves (campaign created,
  graduated, or abandoned);
- a book is (re)analyzed, its scope changes, or its analysis/corpus is removed —
  re-running comparability for that book;
- study language or NLP capability changes the set of comparable books.

Recalculation is **neutral** (ADR 0034's neutral language) and never rolls back or
rewrites the learner's canonical order. After recalculation, the alternative is
presented as the current-order comparison, current-versus-projected kept distinct.

### Explanation data

For each position, the comparison exposes: the book, its current known-token
coverage, its per-target threshold investment, the reason it is placed there
(its coverage rank and whether it was constrained by a fixed-order constraint), and
— for excluded books — the reason it is incomparable. All of this is **derived on
demand** and presented as current data, not persisted as a projection that can go
stale.

### Learner order stays canonical; the alternative is advisory only

The learner's order remains the authoritative, stored, active order (ADR 0034). The
vocabulary-efficient alternative is **comparison evidence only**: it never writes to
membership, never reorders the Journey, never auto-selects a Primary Goal, and never
presents itself as "optimal route" or "best next book". The learner's order is
always the active one; the alternative merely answers "how would reordering change
one stated lexical property (current known-token coverage)?"

## Alternatives considered

- **Optimize a composite difficulty route.** Rejected: no opaque difficulty score is
  supported by the signals (ADR 0026), and it would conflate lexical coverage,
  structure, and evidence quality — explicitly forbidden.
- **Reorder using structural signals or evidence quality as inputs.** Rejected: this
  would give those dimensions ordering power they are not entitled to and would blur
  the "exactly one named lexical property" rule.
- **Persist a computed efficient order or projection for display.** Rejected: it
  becomes stale truth as soon as vocabulary or analysis state changes, contradicting
  ADR 0025's on-demand computation and the requirement not to persist projections.
- **Assume reading makes the projected vocabulary known between books.** Rejected:
  ADR 0036's justified-graduation rule forbids it; only a confirmed justified
  transition may ever move coverage from unknown to known.
- **Rank or demote unassessed/stale books.** Rejected: fabricating readiness is
  explicitly disallowed; such books stay at the learner's position and are excluded.
- **Use a floating-point/rounded coverage for ordering.** Rejected: ADR 0025 exact
  integer/denominator semantics are authoritative; presentation may round, ordering
  never does.

## Consequences

### Positive

- The Journey route comparison is reproducible: inputs, objective, transition
  function, deterministic order, unavailable-data behavior, and invalidation rules
  are all specified.
- The advisory alternative optimizes exactly one lexical property and never
  threatens the canonical learner order or implies literary/quality judgment.
- Current and conditional projected states stay distinct at every step, preserving
  ADR 0036's honesty about reserved-but-unconfirmed vocabulary.
- Owner/language scoping and ADR 0025 integer/denominator semantics are preserved;
  no new denominator or composite score is introduced.
- Incomparable books are handled explicitly without fabricating a rank.

### Costs

- Implementation (Phase 7 of the migration plan) must recompute per-book coverage
  and ordering on demand across all comparable Journey books, adding a route-comparison
  surface; this is new analysis capability beyond ADR 0025's single-book metrics.
- The UI must clearly separate current vs conditional projected state and explain
  exclusions — additional copy and state handling.
- The alternative is deliberately *not* a literary or difficulty recommendation,
  which limits how useful it is as guidance (by design, not a defect).
- No SQL schema is added by this ADR; the comparison is derived, storage-optional
  (computed at request time). Any migration remains gated by #449 schema-change
  governance if persistence is later chosen (not required here).

## Non-goals

- No implementation in this decision.
- No automatic reorder, auto-selection of a Primary Goal, or suggestion of a "best
  next book".
- No difficulty score, literary-value score, or any composite metric.
- No projection that reading a book makes its vocabulary known; only the single
  justified graduation transition (ADR 0036) may change current coverage, shown
  conditionally until confirmed.
- No persisting of projections as stale truth.
- No re-ranking, demoting, or moving of unassessed/stale/incomparable books.

## Related

- Issue #463 (this decision); part of #480. Resolves info-architecture contract
  item #6 (cross-book projection and route comparison) and migration blocker B5.
- [ADR 0034](0034-reading-journey-identity-ordering.md) — Journey identity, canonical
  ordering, and the deferred "separate projection contract" this ADR is.
- [ADR 0025](0025-analysis-coverage-threshold-metrics.md) — exact coverage /
  threshold / denominator semantics and deterministic tie-breaking inherited here.
- [ADR 0026](0026-structural-text-profile.md) — structural signals kept separate
  from the lexical objective.
- [ADR 0036](0036-primary-goal-justified-graduation.md) — the single justified
  graduation transition that bounds what may be projected as conditional.
- [ADR 0027](0027-learning-campaigns.md) and [ADR 0019](0019-generated-vocabulary-exclusion.md)
  — reservation/abandon/legacy-generated category semantics preserved.
- [`doc/design/information-architecture.md`](../design/information-architecture.md)
  (Reading Journey hierarchy; contract item #6) and
  [`doc/features/analysis-insights.md`](../features/analysis-insights.md).

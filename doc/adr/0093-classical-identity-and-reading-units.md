# ADR 0093: Classical identity and stable reading units

Status: **Accepted; implementation pending** · Date: 2026-10-14 · Author: Justin + OpenCode

Recorded through the live decision in
[Choose classical identity and reading units within the Book domain](https://github.com/justin-hayes/mouseion/issues/2026).
This is a domain direction, not authorization for production code, schema,
backfill, or deployment changes. Its repository promotion requires human review.

## Context

Classical repositories can offer multiple editions of a work, independently
acquired annotations, and coverage of only selected passages. Reviewed
[Test CTS identity and cross-corpus text-to-annotation matching](https://github.com/justin-hayes/mouseion/issues/2024#issuecomment-6161715262)
establishes that CTS distinguishes declared work/version/exemplar/passage
identity, but does not prove token correspondence. Normalized similarity and
reference coverage do not establish complete, safe annotation attachment.
The reviewed
[Audit Mouseion compatibility with annotation-backed Ancient Greek](https://github.com/justin-hayes/mouseion/issues/2025#issuecomment-6161582471)
identifies conditional reuse of exact evidence, Reading snapshots, counts, and
artifact preparation, not a tested Ancient Greek pipeline.

## Decision

### Bibliographic identity and learner identity are distinct

**Work** groups the intellectual composition's textual versions. **Edition**
names a particular textual version, not its repository or annotation producer.
A **Reading unit** is a defined extent of one Edition: the whole work, a
constituent book, or a bounded passage. Its extent does not change with annotation
availability or source revisions.

**Book** remains a stable, owner-scoped learner identity, now explicitly for a
Reading unit rather than an abstract Work. Work, Edition, Reading unit, and
source bindings are separate associated concepts, not a default set of classical
nullable fields on Book. Classical entries identify their edition and unit;
existing EPUB Books retain their IDs, declared analysis scope, and learner state.
No speculative Work/Edition backfill is required by this decision.

Internal identities are stable; CTS and repository identifiers are aliases and
provenance. Across repositories, entries resolve to the same owner's Book only
when they are verified to identify the same Edition and Reading unit. Different
editions or bounds remain distinct. Titles, authors, matching identifiers,
overlapping text, and identical acquired bytes alone do not justify merging.
Uncertain matches remain separate; conflicting evidence requires reconciliation.
Repeating acquisition for an established edition/unit identity is idempotent.

Missing CTS or full bibliographic metadata need not exclude otherwise verified,
licensed text. An explicitly unidentified Edition can be backed by pinned source
evidence; it must not impersonate a guessed named edition. Later verified
reconciliation preserves Book IDs and historical provenance. This does not
introduce an implementation or general Book-merge workflow.

### Acquired sources and exact bindings are independent

A **Text Source** is acquired text evidence from an origin, with immutable
revisions; one acquisition may support several Reading units. An
**Annotation Source** is independently acquired linguistic evidence with its own
origin, passage coverage, and immutable revisions. Neither term names the
repository or producer itself.

Each **Text–annotation binding** identifies the exact text revision, annotation
revision, and passage bounds and retains validated correspondence and
transformation provenance. Analyses additionally identify the adapter and
configuration used. CTS locates candidates; attaching annotations requires
verified token correspondence to the chosen exact text. Accent- or
punctuation-stripped similarity is not sufficient proof, and no alignment
tolerance or normalization algorithm is approved here.

Annotation-carried text may itself be the chosen text authority when its text,
bounds, provenance, and licensing are verified. It must not silently substitute
for another edition. Missing independently acquired text is not permission to
attach annotations to a different available edition.

### Reusable evidence does not share learner state

Work/Edition descriptions and immutable text/annotation evidence may be shared
where licensing and access permit. Each learner still owns their Book identity,
source/analysis adoption, disposition, visibility, corrections/exclusions,
Reading snapshots, history, Known/Reserved vocabulary, and artifacts. Credentials,
restricted acquisitions, and another learner's corrections are not exposed by
reuse. This permits evidence reuse; it does not require a shared-storage design.

Each owner's Book resolves its own current text and published analysis. A shared
source refresh or another owner's producer replacement does not globally switch
Books. Revisions within the same Edition and Reading unit keep the Book ID but
require newly validated evidence; changing Edition or unit creates a different
Book, not an in-place replacement.

At most one current published analysis remains per Book. New text/annotation
revisions or a replacement producer create new immutable evidence. Active
readings, their deck preparation, and historical artifacts remain bound to the
original exact evidence. Corrections/exclusions do not automatically transfer
to another analysis. Known vocabulary and completions are never retroactively
rewritten by replacement.

### Stress cases

| Case | Accepted result |
| --- | --- |
| Two editions of one Work | Separate Books for their respective edition/unit identities, grouped by Work without shared completion. |
| Whole-work text, annotations only for Book I | Book I can become eligible after verified required coverage and canonical evidence; the whole-work Book is not presented as fully analyzed for that purpose. Do not shrink its denominator or scope to available fragments. |
| Book I and Books I–II | Distinct Books may overlap; disposition, snapshots, and history stay separate. Completing either does not complete the other. |
| Producer A replaced by B | New validated analysis, no new Book solely because of the producer; original Reading evidence and learner decisions remain intact. Other owners do not switch implicitly. |

## Why this trade-off

A Work-level Book conflates editions and makes passage completion ambiguous.
An Edition-only Book cannot express useful partial readings. A producer- or
annotation-fragment-owned identity lets availability changes rewrite the
learner's reading intent. Stable edition/unit Books preserve meaningful history
while allowing reusable evidence underneath them. The costs are explicit
reconciliation, overlap, revision/adoption, and access/retention decisions rather
than unsafe automatic deduplication or text substitution.

## Compatibility and remaining decisions

This amends ADR 0035's abstract-work wording and its blanket prohibition on
cross-owner acquired-source reuse for the future classical evidence model;
owner authorization and private learner artifacts remain strict. It preserves
ADR 0044's connection-scoped catalog aliases and does not make ISBN or CTS a
Book primary key. A classical catalog entry may offer several Reading units;
its connection-scoped identity is distinct from each Book's edition/unit
identity. Existing one-entry-to-Book alias mechanics are not assumed sufficient
for that flow; its representation belongs in the
[Choose the Greek repository sync, browse and acquisition flow](https://github.com/justin-hayes/mouseion/issues/2028).
Existing production ownership constraints are unchanged.

Preserve ADR 0040's single current published analysis, ADR 0078/0085's
Book-anchored Reading/history and exact frozen snapshots, and ADR 0081's
exact-analysis corrections. ADR 0090 already counts editions/volumes as separate
Books; no counting change is made here. Overlapping selections introduce the same
duplication risk and require the explicit
[Choose how Greek corpus evidence relates to personal vocabulary counts](https://github.com/justin-hayes/mouseion/issues/2029).

Canonical annotation fields, validation/readiness and partial-coverage rules,
source selection/adoption interaction, normalization, licensing/access,
operational refresh/retention, and concrete schema/migration design belong to
their downstream decision tickets. The separate
[Choose classical evidence refresh, adoption and retention](https://github.com/justin-hayes/mouseion/issues/2036)
must settle operational policy before the architectural handoff. No corpus,
arbitrary-passage UI, shared cache layout, alignment algorithm, or automatically
analyzed repository is selected by this ADR.

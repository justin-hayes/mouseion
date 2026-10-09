# ADR 0053: Book-anchored vocabulary consolidation — the learning campaign dissolves into the Book

Status: **Superseded by ADR 0072** · Date: 2026-09-09 · Author: Justin + Hermes · Learner-facing analysis-action wording superseded by [ADR 0054](0054-retire-standalone-analysis-action.md)

Mouseion currently models reading and vocabulary acquisition as two parallel
tracks joined by an internal **learning campaign** object. This decision
dissolves the campaign as a separate plan and reservation object and anchors
vocabulary state directly onto the **Book**, so that one unit — the Book —
carries both the reading facts and the vocabulary-study facts of the learner
loop. It consolidates the loop into a single, reachable path and removes the
books-vs-decks seam that made the final transition (deck study → vocabulary
graduation) unreachable in production.

## Context

The product sits between two learner intents: reading to enjoy and understand
literature, and reading to acquire vocabulary as an end in itself. These map to
two object realms today:

- **Reading/experience realm:** My Books, Reading Journey, Primary Goal. The
  Book is the unit.
- **Acquisition realm:** analysis, coverage, deck preparation, study,
  graduation. The deck is the artifact.

The **learning campaign** (`learning_campaigns`, `learning_campaign_vocabulary`)
was introduced (ADR 0027) to bind one book to one prepared deck and reserve that
deck's snapshotted vocabulary during study, graduating it to known on confirmed
review. Later decisions ([ADR 0036](0036-primary-goal-justified-graduation.md),
[#470](https://github.com/justin-hayes/mouseion/issues/470)/[#472](https://github.com/justin-hayes/mouseion/issues/472)) demoted the campaign to an internal,
never-learner-facing reservation/graduation mechanism behind the Primary Goal.

Two independent problems followed from keeping the campaign as a separate
object:

1. **The loop tail became unreachable.** `CreateLearningCampaign` exists in
   persistence and fixtures but has no production route or caller; `POST
   /campaigns` is asserted `405`. Campaigns are only ever created by seeds. So
   "Start learning" (`views.templ:713`) is unreachable and
   `FinishReadingPrimaryGoal`'s graduation branch can never fire in normal use.
   The promised loop `catalogue → book → analysis → deck → study → vocabulary
   graduation` is the only one Mouseion promises, and its last transition is a
   dead end.
2. **The campaign is a books-vs-decks seam.** It re-introduced a second
   internal "plan" with its own status machine (queued/active/complete/abandoned)
   and its own progress facts (book progress, deck progress) that partially
   duplicate facts already carried by the Book, the Reading Journey, and the
   Primary Goal. Every attempt to make it coherent produced another layer
   (retired queue, activate route, residual-vocabulary resolution) rather than
   removing the underlying split.

A decisive structural observation: **vocabulary is book-scoped.** The recurring
selection pool is a Book's vocabulary; the coverage denominator is a Book's
corpus; graduation provenance is a Book's snapshot. A deck is always *the deck
of a Book*. Consolidating "on decks" is therefore structurally unstable — one
immediately has to ask "a deck of what?" and lands back on the Book (or expands
to a corpus/language unit, abandoning the book-based coverage model). The Book
is the natural, stable anchor.

## Decision

**The Book is the single unit of the learner loop, and it carries both the
reading facts and the vocabulary-study facts.**

The learning campaign is dissolved as a separate plan/reservation object. Its
two responsibilities move onto the Book:

- **Reservation** becomes a book-scoped vocabulary-study state: a Book's
  vocabulary is reserved while its prepared deck is being studied, and released
  (made eligible again) if that study is abandoned.
- **Graduation** remains the single justified transition already defined by ADR
  0036 — a Book's snapshotted, provenance-linked vocabulary identity graduates
  into known vocabulary on confirmed deck review — but the snapshot is anchored
  to the Book rather than to a campaign row.

Concretely, the learner-facing plan is exactly one: the Reading Journey and its
Primary Goal (per study language). There is no second plan. A Book in the
library has two independent, book-scoped facets:

- **Reading state** — not started, in progress, or finished (carried by the
  Primary Goal / Journey).
- **Vocabulary-study state** — not started, studying, reviewed (graduated), or
  abandoned.

These two facts are independent: reading progress never graduates vocabulary,
and confirmed deck review graduates vocabulary regardless of reading progress
(consistent with ADR 0036's independence of `book_status` and `deck_status`).

### The consolidated learning loop

```text
catalogue → My Books → add to Reading Journey → prepare deck (Book's vocabulary)
    → study (in the learner's own Anki) → confirm deck review → vocabulary
    graduates into known (book-anchored)
```

Each step is reachable and anchored to a Book:

1. **Catalogue sync** produces a Book in My Books (unchanged).
2. **Reading intent / analysis** — add a Book to the Reading Journey; the Book
   gets a completed current analysis (the standalone action described here was
   later retired by ADR 0054).
3. **Prepare deck** — from a completed analysis, build a Book's deck (unchanged;
   preparation never starts study and never marks vocabulary known).
4. **Study this Book's vocabulary** — a learner action on a ready, non-empty
   deck. It creates the Book's vocabulary-study state as **studying** (the
   equivalent of today's create-and-activate), reserving the Book's snapshotted
   vocabulary. One Book's vocabulary is studied at a time per owner.
5. **Confirm deck review** — the learner confirms they reviewed the deck;
   the Book's vocabulary graduates into known vocabulary (book-anchored, per ADR
   0036's justified transition). Reading progress stays independent.
6. **Reading finished** — recorded on the Primary Goal / Journey as a reading
   fact; it graduates nothing by itself.

The previously-missing transition is now a plain, reachable Book action — no
separate campaign, no activation route, no queued state, no one-active-campaign
gate. The only remaining concurrency rule is the natural one: **one Book's
vocabulary is studied at a time per owner.**

### Study-state placement (implementation shape)

A **Book has one current deck at a time.** There is no value in generating
multiple concurrent decks for the same book: deck retry reuses the same
preparation, and coverage and graduation are per-book. The only path that
produces a second deck is content revision and re-analysis from Reading Journey,
and that **replaces** the current deck rather than stacking
one beside it — the retired deck remains as immutable history for provenance,
and the new deck becomes the Book's single current deck. This collapses the
current `UNIQUE(owner_id, source_material_id, content_hash)` schema (which
permitted stacked decks per content snapshot) to one current deck per Book.

The vocabulary-study facts live on the immutable **deck**, not on `books` and
not on a separate campaign object. `deck_preparations` gains nullable
`studying_at`, `reviewed_at`, and `graduated_at` columns. A deck with
`studying_at` set is the currently-studied one and its snapshotted vocabulary
is reserved; `graduated_at` is terminal. The one-studied-at-a-time rule is a
partial unique index `ON deck_preparations(owner_id) WHERE studying_at IS NOT
NULL`, replacing `learning_campaigns.one_active_per_owner`. The Book page and
Reading Journey resolve "the Book's vocabulary" to the Book's current
ready/studying deck; the surface is book-anchored while the facts are
deck-attached (where the analysis provenance already lives).

**A graduated deck is not restudied.** Graduation is irreversible and
provenance-linked; re-reading or re-analyzing a book produces a fresh deck from
the new analysis, which is what gets studied next. The graduated deck keeps its
`graduated_at` as immutable history.

**Learner actions and copy.** A single **"Study this Book's vocabulary"** action
begins study (sets `studying_at`, reserving vocabulary, with copy stating the
reservation and that reserved vocabulary is not counted as known). A separate
**"Confirm deck review"** action is the justified graduation transition (ADR
0036), presented with a consequential-transition confirmation naming how many
identities will graduate.

### What is retained and what is retired

Retained (from prior ADRs, not re-litigated):

- The single justified graduation transition: snapshot + provenance link +
  confirmed deck review (ADR 0036). Graduation is never implied by generation,
  assignment, reading, or Goal choice.
- Reading-finished is an independent fact and never graduates vocabulary.
- Generated-vocabulary provenance is immutable; no rewrite or deletion.
- Reading Journey and Primary Goal identity (per study language, ADR 0050/0051).
- One-vocabulary-reserved-at-a-time exclusivity and deterministic overlap.

Retired (the campaign's own scaffolding):

- The `learning_campaigns` and `learning_campaign_vocabulary` tables as the
  learner-facing reservation/graduation object. The equivalent state moves to
  book-scoped fields.
- The queued campaign state and its "Start learning"/activate path.
- The `one_active_per_owner` partial unique index on campaign status (its
  semantics move to a book-scoped "one studied at a time" rule).
- `CreateLearningCampaign` as a separate persistence entry point with no caller.

### Migration posture

Per ADR 0038, this is a consequential shape change and must ship as new,
immutable migrations (never edit history). Existing `learning_campaigns` and
`learning_campaign_vocabulary` rows are conservative: an existing campaign
whose deck is not reviewed does not graduate anything during migration, and
nothing converts generated vocabulary into known vocabulary. Graduation history
already recorded is retained. The migration maps each historical campaign's
reservation/graduation facts onto the book-scoped fields without rewriting
provenance. Exact column design and backfill ownership are deferred to the
implementation ticket.

## Alternatives considered

- **Consolidate on decks** (the initially-proposed direction). Rejected: decks
  are not self-anchoring. Vocabulary identity, selection pool, coverage
  denominator, and graduation provenance are all book-scoped; a deck-primary
  model either falls back to the Book or expands to a corpus/language unit that
  abandons the book-based coverage model and the literature identity Mouseion
  is built around.
- **Keep two intents and merely clean the seam.** Rejected: it does not remove
  the underlying tension, only makes it quieter, and it keeps a dead-end
  reservation object that has already produced the unreachable-loop bug.
- **Keep the campaign but give it a production entry point.** Rejected: this
  re-attaches a second internal plan with its own status machine, which is the
  very redundancy this decision removes.

## Consequences

### Positive

- The learning loop becomes a single, reachable path with no books-vs-decks
  seam.
- Removing the campaign removes the queued state, the activate route, the
  one-active-campaign gate, and `CreateLearningCampaign`'s orphaned status — a
  net reduction in machinery.
- The Book carries both reading and vocabulary facts, so the product keeps both
  identities (reading for enjoyment; vocabulary acquisition) under one unit.
- Vocabulary knowledge still changes only through the justified transition, so
  coverage and provenance guarantees are preserved.

### Costs

- A book-scoped schema change (governed by ADR 0038) and a migration of existing
  campaign rows to book-anchored vocabulary-study state.
- UI rework: the "Prepared books and decks" and "Campaign history & operations"
  sections on Reading Journey collapse into a Book-anchored vocabulary section;
  campaign/deck copy and tests change.
- A targeted sweep of docs and copy that still describe graduation as requiring
  both book-finished and deck-reviewed, to align with ADR 0036 (already the
  governing rule).
- The historically-seeded campaigns used by integration and fixture tests are
  re-keyed to book-scoped state.

## Non-goals

- No in-app study/review (spaced-repetition) surface; study remains in the
  learner's own Anki with manual review confirmation (unchanged).
- No abandonment of the reading/literature identity; it is preserved by making
  the Book the anchor.
- No deletion or rewrite of generated-vocabulary provenance.
- No per-language relaxation of the one-studied-at-a-time rule in this decision
  (ADR 0051's open question stands, now phrased as book-anchored vocabulary
  study rather than per-language campaigns).

## Open question

Whether per-(owner, language) vocabulary study is ever warranted. Carried
forward owner-wide (one deck studied at a time across all languages) for now,
mirroring the prior `one_active_per_owner` posture; ADR 0051's open question is
re-expressed here as book-anchored vocabulary study. Revisit only if real
learners need concurrent study in multiple languages.

## Related

- [ADR 0027](0027-learning-campaigns.md) — the Campaign reservation object this
  decision dissolves; its justified-graduation intent is retained and made
  book-anchored.
- [ADR 0036](0036-primary-goal-justified-graduation.md) — single justified
  graduation transition and reading/deck independence, retained unchanged.
- [ADR 0034](0034-reading-journey-identity-ordering.md) — Reading Journey as the
  single learner plan; this decision completes that direction by removing the
  campaign as the last vestige of a second plan.
- [ADR 0038](0038-schema-change-governance.md) — governs the consequential
  schema migration.
- [ADR 0050](0050-active-study-language.md) / [ADR 0051](0051-reading-journeys-and-goals-per-language.md) —
  per-language surfaces; the open per-language-vocabulary-study question is
  reframed here as book-anchored.

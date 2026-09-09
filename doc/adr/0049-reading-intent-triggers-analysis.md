# ADR 0049: Reading intent triggers analysis

Status: **Accepted** · Date: 2026-09-07 · Author: Justin + opencode

## Context

Analysis of a Book has always been a manual, per-book act: the learner opens the
book page and presses **Start analysis**, which (since ADR 0047) acquires the
EPUB server-side and analyzes the whole snapshot. Catalogue sync (ADR 0041)
stays metadata-only and deliberately never downloads or analyzes. Neither adding
a book to Reading Journey nor choosing a Primary Goal has ever triggered
analysis; those actions are defined as consequence-free and fully reversible
(ADR 0034, terminology).

But the Journey's advisory ordering (`JourneyProjection`, ADR 0037) consumes
each member's *current analysis* to compute the vocabulary-efficient
alternative, and books without one are marked incomparable with reasons like
"unassessed: no current analyzed corpus." The analysis data is what lets the
learner weigh candidates against each other *before* committing to an order or
a Goal. Requiring a separate explicit Start analysis per candidate before the
Journey is useful turns the comparison into an unstated chore.

The principle: **OPDS synchronization discovers books. Reading Journey
expresses reading intent. Analysis is an automatic consequence of reading
intent.** A book is not being "read or studied next" simply because sync found
it; it becomes a candidate for study only when the learner voluntarily adds it
to their Journey (or sets it as the Primary Goal). That voluntary act is the
moment analysis becomes worth running.

## Decision

**Analysis is an automatic, ensure-once consequence of expressing reading
intent, and Primary Goal is a promotion out of the Journey backlog.**

- **Journey membership expresses reading intent, and its consequence is
  analysis.** Adding a metadata-only Book to Reading Journey acquires its EPUB
  server-side (reusing the ADR 0047 acquisition path) and submits analysis in
  one flow, exactly as the explicit Start analysis action does today. The
  Journey becomes the place where "I may read and study this" is decided, and
  the data needed to weigh it against other candidates is prepared as a side
  effect.
- **Ensure-once semantics.** The consequence is "ensure a current analysis
  exists for the book's current content revision": submission happens only when
  no completed or in-flight analysis exists for that revision. Re-adding,
  re-ordering, and re-promoting never re-submit. Removing a book from the
  Journey never cancels or discards an analysis, and never invalidates evidence
  (analysis is durable Book-level evidence that survives removal and is useful
  on re-add). Ordering actions stay pure.
- **Membership is intent, not success.** Adding to the Journey succeeds even
  when no acquisition target currently resolves (catalogue unavailable, entry
  removed). The entry then shows the existing "cannot currently assess" /
  "unavailable" incomparable state and analysis proceeds once content exists.
- **Standalone analysis action superseded.** ADR 0054 retires the manual action
  from the learner surface. Reading Journey membership remains the initial
  acquisition-and-analysis trigger; its Journey entry owns stale and failed
  analysis recovery while analysis remains durable Book evidence.
- **Primary Goal is a promotion.** Choosing a Primary Goal is the act of
  promoting a book out of the Journey backlog. The affordance is available only
  from the Reading Journey screen and requires the book to be a Journey member
  with a successfully completed current analysis. Selecting a Goal is
  consequence-free beyond the promotion itself: it does not start or re-run
  analysis, because a Goal is only reachable for an already-analyzed Journey
  member.
- **The goal/membership invariant is enforced at the persistence layer, not the
  UI.** `CreatePrimaryGoal` / `ChangePrimaryGoal` reject a book that is not an
  active Journey member with a current completed analysis; the Journey screen
  surfaces the underlying incomparable reason when the affordance is ineligible.
  Removing the current Goal's book from the Journey clears the Goal (promotion
  ends when the pool membership ends; the analysis evidence survives and
  re-promotion is trivial).
- **Prospective only.** Existing Primary Goals that predate this tightening and
  violate the new invariant are not destructively reconciled. They render as
  ineligible with a clear/re-add affordance and are resolved on the learner's
  next interaction.
- **No background watcher.** Nothing re-analyzes a Journey member in the
  background when a newer catalogue revision appears. Ensure-once is evaluated
  at the moment intent is expressed; Journey recovery actions are the
  re-analysis lever after content changes.

This ADR reverses the residual stance of ADR 0041/0047 that analysis is only
ever a manual per-book act, and redefines the weight of Journey membership and
Primary Goal: they are no longer consequence-free, but the only consequence is
the automatic preparation of the evidence that lets the learner decide.

## Alternatives considered

- **Keep analysis exclusively manual.** Rejected: it makes the Journey's
  candidate comparison (ADR 0037) depend on an unstated per-candidate Start
  analysis step, leaving freshly-added candidates incomparable until the learner
  separately remembers to analyze them.
- **Auto-add to Journey when choosing a Goal.** Rejected. The user clarified
  that choosing a Goal is a *promotion of an existing Journey member*, not a
  membership-creating action; membership is a prerequisite, not a side effect,
  of Goal-setting.
- **Enforce the Goal/membership invariant only in the UI.** Rejected: a durable
  state-machine rule belongs at the persistence layer so a forged or direct POST
  cannot violate it (the same reason the Journey guards mutations with revisions
  and Goal-setting guards against stale Goal state).
- **Continuously re-analyze Journey members when content changes.** Rejected:
  it reintroduces the ambient cost machine the metadata-first/lazy-acquisition
  decoupling (ADR 0041) was built to avoid, for a rare event, with no learner
  action.

## Consequences

- Adding a Book to Reading Journey now downloads its EPUB and queues whole-book
  NLP. Members are analyzed by construction; freshly-added metadata-only books
  show queued/running/complete analysis status on the Journey.
- Primary Goal is only choosable for an analyzed Journey member, from the
  Journey screen; the Goal-set affordance leaves My Books and the book page.
- Choosing a Goal no longer starts analysis (it could not, given the
  prerequisite), so no goal action creates an analysis job — consistent with the
  existing integration assertion that Goal-setting creates zero jobs.
- Journey membership and Primary Goal are no longer consequence-free; the single
  consequence is automatic, idempotent, durable evidence preparation. Reversing
  membership costs nothing beyond the already-sunk analysis.
- The cost profile changes: a learner who adds many books to the Journey queues
  many analyses on the single-worker analysis queue. This is accepted: the
  Journey is learner-curated, analysis is a sunk-cost prep step rather than a
  refundable charge, and the ensure-once semantics bound redundant work.
- Existing docs that describe analysis as an exclusively manual act
  (terminology, product summary, workflow documents) must be updated to reflect
  the automatic-consequence model.

## Related

- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived](0047-acquisition-folded-into-analysis.md)
- [ADR 0034: One implicit Reading Journey with learner-canonical ordering](0034-reading-journey-identity-ordering.md)
- [ADR 0036: Deck-independent Primary Goal and single justified vocabulary-graduation transition](0036-primary-goal-justified-graduation.md)
- [ADR 0037: Cross-book vocabulary projection and advisory Journey ordering](0037-cross-book-projection-advisory-ordering.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)

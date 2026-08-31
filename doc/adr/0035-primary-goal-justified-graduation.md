# ADR 0035: Deck-independent Primary Goal and single justified vocabulary-graduation transition

Status: **Accepted** · Date: 2026-08-31 · Author: Justin + Hermes

Supersedes the learner-facing completion and vocabulary-graduation semantics of
[ADR 0027](0027-learning-campaigns.md) and amends its support after reading
finishes while vocabulary work remains. Retained by
[ADR 0034](0034-reading-journey-identity-ordering.md) as internal reservation
state and by this decision as the sole justified graduation path. Recorded for
issue #462 as part of the Primary Goal decision in the My Books / Reading
Journey / Primary Goal migration (#480).

## Context

The accepted learner-facing architecture (merged PR #459 and
[`doc/design/information-architecture.md`](../design/information-architecture.md))
defines **Primary Goal** as the one book the learner currently intends to
finish. Primary Goal is the only meaningful commitment in the product, it is
embedded at the start of Reading Journey, and a learner may have no Primary
Goal. The same architecture and the Reading Journey workflow
([`doc/design/workflows/learning-campaign.md`](../design/workflows/learning-campaign.md))
require Primary Goal to be meaningful **before** analysis or a prepared deck
exist, and to permit reading without Anki.

Today **ADR 0027** requires every learning campaign to reference a prepared
deck, derives campaign completion from `book_status = finished` **and**
`deck_status = reviewed`, and graduates the campaign's snapshotted
`learning_campaign_vocabulary` only at that combined transition. Those semantics
cannot represent a Primary Goal that is chosen before analysis or deck
preparation, a Goal read without a deck, or a finished book whose vocabulary
work remains.

Reading completion is not evidence of vocabulary knowledge. A learner may
finish a book without having confirmed study of any assigned vocabulary; a
learner may also confirm review of a deck at a different time from finishing the
book. Graduation must be driven by **justified study confirmation of
snapshotted, generated-provenance vocabulary** — not by reading alone.

## Decision

### Primary Goal identity: one per learner, deck-independent

There is exactly **one Primary Goal per learner** (scoped by `owner_id`). It is
an owner-scoped reference to one book the learner currently intends to finish.
A learner may have **no Primary Goal**; that is an ordinary between-goals state,
not a failure. Primary Goal is distinct from Reading Journey membership: a Goal
references a book that is typically also a Journey member and is anchored in the
Journey's presentation, but choosing or clearing a Goal never adds, removes, or
reorders Journey membership (per ADR 0034).

A Primary Goal is meaningful **before** any of these exist: analysis, a
reviewed scope, a prepared deck, a campaign, reading progress, or vocabulary
work. A Goal does not require a deck and may be read without Anki. Selecting a
Goal never starts analysis, prepares a deck, marks vocabulary as known, or
reserves vocabulary.

### When a Goal gains a campaign/reservation; Campaign remains secondary

A Goal on its own **never reserves vocabulary**. Vocabulary reservation happens
only through a **learning campaign**, which remains the internal
reservation/graduation mechanism and stays secondary — it is never a
learner-facing plan.

A campaign (and its reservation) is associated with a Goal when the learner
elects to do vocabulary work for that book: a prepared deck is bound to the
book and the learner starts the reserved study flow. Until then the Goal is
deck-independent and reading-only, with no campaign and no reservation. An
active campaign remains exclusive per ADR 0027/0034 (`one_active_per_owner`):
at most one campaign reserves vocabulary for a learner at a time. Choosing a
new Goal by itself neither creates the new Goal's reservation nor closes the
previous one.

### Reading-finished is a fact independent of deck-reviewed

Recording **book finished** is a factual reading acceptance, not completion of
the Journey and not proof of vocabulary knowledge. It is independent of
`deck_status`. The campaign keeps `book_status` and `deck_status` as separate
facts; neither silently drives the other. A finished book moves out of the
current Primary Goal role and remains in My Books and history even while
vocabulary work remains.

### The single justified vocabulary-graduation transition

Vocabulary becomes known **only** through the accepted graduation transition:

```text
learning_campaign_vocabulary identity
  AND atomically linked to generated_vocabulary provenance
  AND justified study confirmation (deck_status = reviewed)
  -> graduate that identity into the learner's known vocabulary
```

Graduation applies **only** to identities snapshotted in
`learning_campaign_vocabulary` for the associated campaign. The
`learning_campaign_vocabulary` row already carries the immutable link to
`generated_vocabulary` provenance via the foreign key `(owner_id, language,
canonical_lemma, upos)`. Only those exact snapshotted identities may graduate;
anything merely generated, actively reserved, or assigned is never known.

**Justified study confirmation** is the learner's confirmation that they
reviewed the prepared deck (the campaign's `deck_status = reviewed`). It is the
condition that justifies graduation. Reading-finished is **not** a graduation
trigger on its own: completing a book without confirmed review graduates
nothing. The initial product lets the learner mark study confirmation manually;
future Anki review-log or AnkiConnect integration may confirm it automatically.

Graduation is a learner-state transition, not a rewrite or deletion of
`generated_vocabulary` history. It records the graduated identity, the source
campaign, book, deck, and graduation timestamp. The one-active-campaign rule and
the snapshot guarantee make the transition deterministic: at most one campaign
can graduate a given identity, and only identities bound to that campaign's
immutable provenance.

### State after reading finishes while vocabulary work remains

When a book is finished but the justified study confirmation (deck reviewed)
has not occurred, the vocabulary work is **residual**:

- **current** values for later Journey books stay unchanged — no vocabulary from
  this book has been added to known;
- the effect that *would* follow from completing the residual study is kept
  **explicitly conditional** (a projected future transition, never a current
  fact);
- the reading achievement is stated truthfully and independently from the
  vocabulary gap: finishing reading never rewrites reserved vocabulary as known,
  and the residual work never blocks the finished reading record.

### Finish outcome: four beats, then explicit Where next?

Finishing a Primary Goal is a **four-beat outcome** — each beat is a distinct,
independently-verifiable fact, and the beats never become a single unlabelled
"done":

1. **Reading outcome.** State that the book was finished as a factual reading
   record.
2. **Justified vocabulary transition, or explicit absence.** State either the
   exact set of identities that graduated (per the single justified transition)
   or that no vocabulary graduated because study is not yet confirmed — never
   imply readiness gains that did not happen.
3. **Recalculation from actual state.** Replace any prior projection for each
   remaining Journey book with a recalculation from the current known-vocabulary
   and reservation facts; show current-versus-projected values separately,
   never as one number.
4. **Where next?** End with the learner's explicit available next decisions —
   choose a new Goal, continue residual vocabulary work, reorder the Journey, or
   pause. The terminal copy offers choices; it never auto-selects, never
   auto-advances, and never implies the residual vocabulary work is complete.

**A Goal is never auto-selected.** Choosing the next Primary Goal is always an
explicit learner action; the four-beat outcome surfaces options and stops.

### Choosing a new Goal while residual vocabulary work exists

A learner may choose a new Primary Goal while the previous book's vocabulary
work is residual (a campaign reserves vocabulary whose deck is not yet
reviewed). This is decided explicitly:

- **Choosing a new Goal does not abandon the residual campaign.** The residual
  reservation and its snapshot remain, because reading-finished is independent
  of deck-reviewed and a learner may change reading intent without discarding
  unfinished vocabulary work.
- **Only one campaign reserves per owner.** Because the active reservation is
  exclusive (`one_active_per_owner`), starting *new* reserved vocabulary work for
  a new Goal requires the residual campaign to be resolved first:
  - **graduate** the residual campaign's identity once its study confirmation
    completes, or
  - **abandon** the residual campaign, which returns its reserved identities to
    eligible (unless independently known), per ADR 0027.
- **Overlap is deterministic.** Identities shared between the residual
  campaign's snapshot and the new book's selection remain reserved by the
  residual campaign until that campaign is resolved; they are not double-counted
  and not selected for the new Goal while reserved. After graduation they are
  known; after abandonment they become eligible again.
- **No silent state change.** The UI explains what will happen to the residual
  reservation and available vocabulary when a learner chooses a new Goal with
  residual work present, and requires an explicit resolution (graduate or
  abandon) before new reservation. It never rolls back, deletes, or fabricates
  graduation.

### Migration and compatibility

Existing `learning_campaigns` rows keep their current status, timestamps, and
graduation history; `learning_campaign_vocabulary` and `generated_vocabulary`
are untouched at migration time. Nothing converts generated vocabulary into
known vocabulary: an existing campaign whose `book_status = finished` but whose
deck is not `reviewed` does **not** graduate anything during migration. A
campaign that already has `vocabulary_graduated_at` set simply retains its
history. The Campaign object remains the internal reservation/graduation
mechanism during and after migration; Primary Goal rides on top of it but never
replaces its provenance guarantees. The staged schema/UI rollout of Goal
persistence follows the #449 schema-change governance and ADR 0034's staged
rollout sequence.

## Alternatives considered

- **Make `book_status = finished` sufficient to graduate.**
  Rejected: reading completion is not evidence of vocabulary knowledge, and this
  decision's requirement is that graduation requires justified study
  confirmation of snapshotted provenance.
- **Graduate all reserved identities when a Goal is chosen.**
  Rejected: choosing a Goal is a commitment, not a mastery event; it would
  silently convert reserved vocabulary to known without study confirmation.
- **Allow multiple simultaneous reservations so a new Goal can reserve while
  residual work remains.**
  Rejected: it breaks the deterministic one-active-campaign rule and the
  rank/tie invariants in ADR 0025; explicit graduate-or-abandon resolution is
  simpler and predictable.
- **Auto-abandon the residual campaign when a new Goal is chosen.**
  Rejected: a learner may intend to finish residual vocabulary work; silently
  discarding the reservation and its snapshot loses the work and contradicts the
  independence of reading and study facts.
- **Make graduation depend on `complete` campaign status alone.**
  Rejected: the combined-transition `complete` status conflates reading with
  study; graduation must key on the snapshot + confirmed review, not on a status
  that also encodes reading.

## Consequences

### Positive

- A Primary Goal can be chosen, changed, or cleared before any analysis or deck
  exists, and a book can be read without Anki.
- Vocabulary knowledge changes only through the single justified transition
  (snapshot + confirmed review + atomic provenance), never from reading or Goal
  choice alone.
- Reading completion, deck review, campaign history, and graduation remain
  distinct, testable transitions.
- Residual-vocabulary / new-Goal behavior is deterministic and implementable with
  the existing `one_active_per_owner` constraint and snapshot table.
- Current-versus-projected labels in coverage stay honest: residual work is
  explicitly conditional until confirmed.

### Costs

- A Primary Goal persistence/role model and a campaign-relationship mapping are
  required (schema/UI milestone; CODEOWNER-blocked new-SQL-migration PRs are
  merged manually, not a CI failure).
- The UI must present a clear graduate-or-abandon resolution when a learner
  chooses a new Goal with residual vocabulary work, adding explicit copy and
  state handling.
- ADR 0025/0019 integer and denominator semantics remain authoritative and must
  not be weakened: this decision changes when knowledge is *earned*, not how
  coverage is *denominated*.
- Initial product relies on manual study confirmation until review integration
  exists.

## Non-goals

- No implementation in this decision.
- No Anki review-log / AnkiConnect synchronization or spaced-repetition mastery
  model.
- No graduation from reading alone or from Goal choice alone.
- No deletion or rewrite of `generated_vocabulary` or
  `learning_campaign_vocabulary` provenance.
- No multiple simultaneous vocabulary reservations for one learner.
- No automatic Primary Goal selection.

## Related

- Issue #462 (this decision); part of #480.
- [ADR 0034](0034-reading-journey-identity-ordering.md) — Reading Journey
  identity/ordering and campaign-queue migration; this decision is its B3
  Primary Goal successor.
- [`doc/design/information-architecture.md`](../design/information-architecture.md)
  — Primary Goal identity (#3), completion/graduation (#4), and new-Goal-with-
  residual-work (#5) contract items resolved here.
- [`doc/design/workflows/learning-campaign.md`](../design/workflows/learning-campaign.md).
- [ADR 0027](0027-learning-campaigns.md) — completion and graduation semantics
  superseded here; Campaign reservation object retained.
- [ADR 0025](0025-analysis-coverage-threshold-metrics.md) and
  [ADR 0019](0019-generated-vocabulary-exclusion.md) — denominator/provenance
  contracts preserved.
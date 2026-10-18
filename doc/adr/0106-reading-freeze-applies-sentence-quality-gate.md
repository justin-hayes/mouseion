# ADR 0106: The Reading freeze applies the sentence-quality gate

Status: **Accepted** · Date: 2026-10-18 · Author: Justin + opencode

Amends the snapshot rule of [ADR 0085](0085-reading-owned-book-vocabulary.md)
as revised by [ADR 0090](0090-library-total-book-vocabulary-selection.md): a
frequency-eligible identity also needs a sentence that passes the
sentence-quality gate of [ADR 0062](0062-derived-sentence-quality-scoring.md).
Finish semantics in [ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md)
and ADR 0085 are unchanged.

## Context

Starting a Reading freezes its vocabulary by frequency alone. The Book deck then
applies the sentence-quality gate and omits every identity none of whose
sentences pass. Finishing the Reading still accepts the whole snapshot into
Known vocabulary, card or not (ADR 0072, ADR 0085). On Herkunft, 3,237 identities
were frozen and reserved, while the deck carded 2,429. Finishing would have made
808 words Known without the learner ever studying them, and removed them from
selection for every later Book.

The gate does not depend on the deck. It is a deterministic function of a
candidate's sentence references, its observed forms, and the persisted sentences
and dependency tokens (ADR 0062). Those are exactly the facts the freeze already
loads to project the analysis (#2217). The deck scores every reference and keeps
the best one, so the deck carries an identity exactly when at least one
reference passes.

## Decision

### 1. A frozen identity needs a sentence that passes the gate

Starting or switching a Reading freezes an identity only when it meets the
ADR 0090 frequency rule **and** at least one of its sentence references in this
Book passes the sentence-quality gate, using the identity's language rubric. The
freeze evaluates the gate in its own transaction over the evidence it projects.

Frequency counting is unchanged. In-Book and across-Books counts still count
every eligible occurrence, not only occurrences in passing sentences. The gate
is an extra filter, not a new counting rule.

### 2. One gate, shared by the freeze and the deck

The freeze and the deck call the same scorer, which moves out of card export
into a package both depend on. The deck keeps gating. For a snapshot frozen
under this decision it should omit nothing for quality. It still protects
snapshots frozen earlier.

### 3. The gate version is frozen with the snapshot

Each frozen identity's provenance records the gate version that admitted it.
Later gate changes, such as #2219, apply to the next freeze. They never rewrite
an active or historical snapshot (ADR 0085). A learner who wants a corrected gate
on a Book rereads it.

### 4. Words that fail the gate stay unreserved and unknown

An identity with no passing sentence in this Book is neither frozen nor
reserved, so it is never accepted by this Reading's Finish. It stays eligible
for a later Book, where a better sentence can earn it a card.

### 5. Finish still accepts the whole snapshot

Finish needs no deck and accepts every frozen identity that is not already
Known, as before. The only identities left without a card are now meaning
omissions, where the provider found no defensible meaning in context, and
retryable provider omissions (ADR 0100). There were 23 such identities on
Herkunft. They had a passing sentence and are still accepted.

## Consequences

- The reserved count and the "words become Known" count match the deck, apart
  from meaning and provider omissions. A learner who reads without a deck gains only words
  that appeared in a usable sentence.
- Starting a Reading costs more. Measured on Herkunft (8,023 sentences), scoring
  every projected identity's 36,665 references took about 0.8 s, after about
  0.25 s to load the evidence. The freeze gates only frequency-eligible
  identities and can stop at the first passing reference, so this is an upper
  bound.
- Snapshot sizes shrink. On Herkunft, 2,169 of the 2,209 identities with three
  or more in-Book occurrences have a passing sentence under the #2219 gate
  (2,050 under the gate before it). Among rarer identities, 4,594 of 6,236
  do (3,324 before #2219), before the across-Books threshold applies.
- Snapshots frozen before this decision keep whole-snapshot semantics. A learner
  restarts a Reading to adopt the gate.
- Browse and Concordance are unaffected. They show evidence, not selection.

## Alternatives considered

- **Finish accepts only identities with a card in a ready deck.** This was the
  first draft of this ADR. It makes the knowledge transition depend on deck
  preparation, contrary to ADR 0072. It also needs rules for a deck that was
  never prepared, failed, or is still running, which the freeze-time gate avoids.
- **Keep whole-snapshot acceptance and only report the gap.** This is cheapest,
  but it keeps writing unstudied words into Known vocabulary.
- **Gate at Finish rather than at the freeze.** The reservation would still hold
  ungateable words for the whole Reading, keeping them from other Books for no
  benefit.

## Related

- [ADR 0062](0062-derived-sentence-quality-scoring.md),
  [ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md),
  [ADR 0085](0085-reading-owned-book-vocabulary.md),
  [ADR 0090](0090-library-total-book-vocabulary-selection.md),
  [ADR 0100](0100-retryable-provider-omissions-preparation.md)
- #2217: the freeze projects the analysis; #2219: the GDEX knock-out fix

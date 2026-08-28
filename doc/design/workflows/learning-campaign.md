# Learning campaign workflow

## Goal

Help a learner choose one book-and-deck campaign, understand what is active or
queued, record reading and deck-review progress, and graduate vocabulary only
through an explicit, informed completion transition.

The product behavior is defined primarily by:

- [ADR 0027: Single-active learning campaigns and vocabulary graduation](../../adr/0027-learning-campaigns.md)
- [Analysis Insights](../../features/analysis-insights.md)
- [ADR 0022: Prepared decks](../../adr/0022-prepared-decks.md)

## Starting state and outcome

The workflow starts with an immutable prepared deck tied to one owned book and
completed scoped analysis. It ends in one of three durable states:

- queued for later;
- complete, with assigned campaign vocabulary graduated to known;
- abandoned, with assigned vocabulary eligible again unless independently
  known.

At most one campaign is active. A prepared deck, queued campaign, or active
campaign does not by itself make vocabulary known.

## Primary path

```text
Analysis result
    -> Add to learning queue
    -> Learning
    -> Activate campaign when no other campaign is active
    -> Read book and review deck
    -> Record first progress condition
    -> Confirm the second, campaign-completing condition
    -> Campaign history and updated known vocabulary
```

## Learning screen hierarchy

The Learning destination answers questions in this order:

1. What am I learning now?
2. What remains before this campaign is complete?
3. What will happen to its vocabulary when I complete it?
4. What is queued next?
5. Which campaigns are complete or abandoned?

The active campaign is the dominant object. Prepared-but-not-queued decks are
available actions, not a peer history section. Queue order and history are
secondary to the current book, deck, and two progress facts.

## Queue and activation

Adding a ready deck creates or resolves a campaign without activating it.
Duplicate submission does not create competing campaigns.

A queued campaign may start only when no other campaign is active. If activation
is blocked, the interface identifies the active campaign and links to it rather
than returning a generic conflict.

Activation must explain that its assigned vocabulary becomes reserved for this
campaign but is not counted as known. Queue and future-book projections may
change because only one active campaign can reserve vocabulary.

## Recording progress

Book and deck progress are independent facts:

- book: reading or finished;
- deck: studying or reviewed.

The interface presents them as two labeled conditions, not as one ambiguous
percentage. Mouseion cannot infer either condition from the APKG or Anki review
history, so the learner records them manually.

When one condition remains incomplete, its action may be direct but must state
that the campaign will remain active. Neither **Mark book finished** nor **Mark
deck reviewed** may use mastery language.

## Completion decision

The action that satisfies the second condition is a separate consequential
transition. Before submission, the interface presents a confirmation step or
modal dialog containing:

- book and prepared-deck identity;
- both resulting progress facts;
- the number of assigned lemma identities not already independently known that
  will be added to known vocabulary, when available;
- an explanation that generated provenance remains intact;
- an explanation that future coverage and queue projections will be
  recalculated;
- an explicit statement that Mouseion does not claim a spaced-repetition grade
  or general mastery;
- an explicit statement that campaign completion cannot currently be undone in
  the application.

The primary confirmation label is outcome-based, for example:

> Complete campaign and add 132 lemmas to known vocabulary

If the count cannot be loaded safely, use **Complete campaign and add its
vocabulary to known** rather than silently omitting the consequence. The cancel
action returns to the unchanged active campaign.

Completion is one atomic learner-state transition. A partially graduated
campaign must never be shown as complete. After success, the screen shows the
campaign in history and confirms that coverage values will reflect the newly
known vocabulary.

## Correction semantics

ADR 0027 defines graduated vocabulary as permanently known and does not define
campaign reopening. Phase 1 therefore establishes these boundaries:

- completing a campaign has no inline undo;
- changing a completed campaign back to active must not be introduced as a
  superficial UI control;
- any future correction workflow must operate on known-vocabulary provenance,
  explain downstream coverage effects, and receive its own product contract;
- until that workflow exists, the completion confirmation must disclose that
  the action cannot be undone in Mouseion.

This preserves the accepted data semantics rather than implying a reversible
state the product does not support.

## Abandonment

Abandoning an active or queued campaign does not delete the book, deck, or
generated-vocabulary history. Any active reservation is released; assigned
vocabulary remains or becomes eligible unless independently known.

Because abandonment changes future selection, it requires confirmation that
states:

- which campaign will be abandoned;
- that its book and prepared deck remain available;
- that any active reservation is released and eligible vocabulary may appear in
  future decks again;
- that the campaign will remain in history.

The action label is **Abandon campaign**, not **Delete**.

## State model

| State | Required presentation | Primary action |
|---|---|---|
| No active campaign | Explain the single-active rule and show eligible queue items. | Start learning |
| Active; both conditions incomplete | Show book, deck, and two independent progress facts. | Record one progress fact |
| Active; book finished | Explain that deck review remains and completion will graduate vocabulary. | Review completion consequence |
| Active; deck reviewed | Explain that book completion remains and completion will graduate vocabulary. | Review completion consequence |
| Completion confirmation | Show exact outcome and finality. | Complete campaign and add vocabulary to known |
| Complete | Show completion/graduation time and retained provenance. | View history or choose next campaign |
| Queued | Show position and why it is not active. | Start when available or abandon |
| Activation blocked | Identify and link the current active campaign. | Return to active campaign |
| Abandon confirmation | Explain released reservation and retained artifacts. | Abandon campaign |
| Abandoned | Explain that vocabulary is eligible again. | View book or prepared deck |
| Mutation failure | Preserve current state and explain recovery. | Retry when safe |

## Accessibility and responsive contract

- The two progress facts use labeled text, not color or icon alone.
- Consequential confirmation receives focus, has a programmatic name, traps
  focus only while modal, returns focus on cancel, and remains usable without a
  pointer.
- Server-side validation remains authoritative if client confirmation is
  bypassed or stale.
- On narrow screens, the order remains active identity, progress, consequence,
  action, queue, then history.
- Completion and abandonment results are announced once without replacing the
  learner's context with a raw response.

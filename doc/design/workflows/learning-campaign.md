# Reading Journey and Primary Goal workflow

Status: **Canonical learner-facing workflow; not yet a shipped domain contract.**
The current Campaign and vocabulary-graduation behavior remains owned by
[ADR 0027](../../adr/0027-learning-campaigns.md) until the explicit planner/ADR
work in
[`information-architecture.md`](../information-architecture.md#contract-changes-requiring-planneradr-work)
is completed.

The filename is retained to preserve existing links. **Learning campaign** is no
longer the primary learner-facing name for this experience.

## Goal

Help a learner keep a fluid idea of books they may read, choose one book they
intend to finish, understand preparation evidence without surrendering
judgment, and return to a changed road ahead after finishing the book.

Only Primary Goal carries commitment. Reading Journey remains provisional even
when Mouseion can compare a vocabulary-efficient alternative.

## Starting state and desired outcome

The workflow may begin with:

- books in My Books but no Reading Journey;
- a Journey but no Primary Goal;
- a Primary Goal with or without analysis, a prepared deck, reading progress,
  or vocabulary work;
- a finished Primary Goal whose vocabulary transition is complete;
- a finished Primary Goal with vocabulary work remaining.

The desired outcome is not Journey completion. It is one informed next choice:
keep reading the current Goal, reshape the provisional Journey, choose a new
Goal, or remain between Goals.

## Primary path

```text
My Books
    -> Add learner-selected books to Reading Journey
    -> Arrange Your order
    -> Optionally compare a Vocabulary-efficient alternative
    -> Choose one book as Primary Goal
    -> Prepare and read, preserving independent facts
    -> Record Reading finished
    -> Apply only justified vocabulary transitions
    -> Recalculate remaining Journey from actual state
    -> Where next?
    -> Choose, reorder, add/remove, or remain between Goals
```

Acquisition, scope confirmation, analysis, deck preparation, Journey membership,
Goal choice, reading completion, and vocabulary graduation remain explicit
transitions. None silently triggers the next.

## 1. Shape Reading Journey

**Learner question:** Which books do I currently imagine reading, and in what
order?

A learner can add books from My Books, remove provisional books, and reorder
them freely. The Journey presents one unambiguous **Your order**. Later books do
not look scheduled, overdue, locked, or committed.

The interface must answer:

- Which book is the Primary Goal, if any?
- Which books are provisional?
- How can I add, remove, move earlier, or move later?
- Which evidence is current, projected, stale, unavailable, or absent?
- Will changing my order alter commitment? It does not, except when the learner
  explicitly chooses a different Primary Goal.

Primary Goal is anchored before the provisional sequence. It is not draggable
into an ordinary later position. Changing or clearing it is an explicit Goal
decision, not an incidental reorder.

Unassessed and incomparable books retain the learner's chosen position. Mouseion
names the evidence gap and excludes those books from numerical comparison
rather than moving them silently.

## 2. Compare route evidence

**Learner question:** Would another ordering of these same books change the
modeled vocabulary preparation?

Mouseion may present a **Vocabulary-efficient alternative** beside or after
**Your order**. The alternative:

- contains only the learner-selected books;
- optimizes one clearly stated lexical property;
- names the selected threshold, evidence scope, and transition assumptions;
- keeps incomparable books visible but outside unsupported totals;
- never becomes active without explicit learner choice.

The hierarchy is:

1. the two book orders, with titles and authors;
2. the plain-language difference, such as **247 fewer modeled additional
   vocabulary identities across these books**;
3. per-book current and conditional effects where they explain the change;
4. aggregate totals and method as supporting evidence;
5. peer actions to keep Your order, adopt the alternative, or adjust manually.

Do not lead with large totals or describe the alternative as best, optimal,
recommended, or the correct reading order.

A manual move produces a neutral preview and recalculation. For example:

> Moving this book here adds approximately 63 modeled identities across the
> remaining Journey under the selected assumptions.

This is information, not a warning. The learner's literary preference remains
canonical.

## 3. Choose or view the Primary Goal

**Learner question:** Which one book do I intend to finish now?

**Choose as Primary Goal** is an explicit action available from My Books,
Reading Journey, or a book context when the underlying product contract permits
it. Selection does not silently start analysis, prepare a deck, claim reading
has begun, or mark vocabulary known.

The Primary Goal region leads with:

1. title, author, and relevant edition identity;
2. the fact that this is the learner's current Goal;
3. reading state;
4. preparation/vocabulary-work state;
5. concise current evidence and clearly conditional projections;
6. one next useful decision, with supporting actions demoted.

A Goal can be meaningful before evidence exists. In that case, the interface
preserves the commitment and explains whether Mouseion can assess the work,
what prerequisite is missing, and which action is available. Lack of evidence
must not make a desired Goal look invalid.

## 4. Prepare and read

**Learner question:** What would help with this book, and what has actually
happened?

Analysis and deck preparation remain book-centered supporting workflows. They
serve the Goal but do not define it. The interface keeps these facts distinct:

- reading not started, in progress, or finished;
- analysis absent, queued, failed, stale, or complete;
- deck absent, preparing, ready, or downloaded;
- vocabulary work not started, in progress, or complete;
- vocabulary currently known;
- vocabulary that would become known only after a justified future transition.

A current coverage value and an after-transition projection may appear together
only when the condition is explicit. Reading progress is never a Journey
progress percentage.

The learner may leave and return while analysis or preparation runs. Operational
status remains secondary to book identity and current reading purpose.

## 5. Finish the Primary Goal

**Learner question:** What did finishing this book change, and what remains
unfinished?

Finishing the book is a factual reading achievement, not completion of the
Journey or proof of vocabulary knowledge. Once reading is finished, the book no
longer occupies the current Primary Goal role; it remains in My Books and
history, and any unfinished vocabulary work remains visible as a separate fact.
No next Goal is created automatically. This target role transition is part of
the Campaign/Goal planner boundary below.

The outcome view uses a restrained, book-led receipt rather than celebration
chrome.

### Reading finished and vocabulary transition complete

When the accepted conditions justify a vocabulary transition:

1. state **Reading finished**;
2. name the vocabulary work that completed;
3. state exactly how many eligible vocabulary identities were added to known;
4. recalculate the remaining Journey from actual known vocabulary;
5. show the books whose current preparation evidence changed, with precise old
   and new labels;
6. end with **Where next?**

Old projections are not presented as though they remain current. The new values
come from actual state after the transition.

### Reading finished while vocabulary work remains

When the book is finished but the accepted vocabulary transition has not
occurred:

1. acknowledge **Reading finished** without qualification;
2. state **Vocabulary work remains** as a separate fact;
3. state that no vocabulary from this work has yet been added to known;
4. keep current values for later books unchanged;
5. keep any possible future effects explicitly conditional;
6. return to **Where next?** without claiming readiness gains.

The reading achievement must not be withheld because vocabulary work remains.
Conversely, achievement copy must not imply the vocabulary transition happened.

Current ADR 0027 does not yet permit this learner-facing rhythm to be implemented
by separating Campaign completion from graduation or by starting another active
Campaign while review remains. That mismatch requires planner/ADR resolution;
the UI must not simulate it with labels alone.

## 6. Where next?

**Learner question:** Given what is true now, what do I want to do next?

After the factual outcome and recalculation, return attention to the remaining
Journey. The first provisional book may be introduced as **First in your current
order**. Actions are neutral peers:

- **Choose as Primary Goal**;
- **Reorder Reading Journey**;
- **Choose another book** from My Books;
- **Remove from Reading Journey** where relevant;
- take no new Goal yet.

No action is preselected, automatic, or labeled optimal. If the Journey is
empty, invite the learner back to My Books without presenting an empty backlog
or a completed plan.

## Alternate and edge paths

- **No Primary Goal:** explain that Mouseion is between Goals and preserve the
  Journey as editable, useful context. Do not manufacture urgency.
- **No planned next book:** show the finished outcome, then offer My Books or a
  calm option to stop without a new commitment.
- **No Journey:** My Books remains fully useful; the Journey empty state explains
  what provisional ordering can do without requiring setup.
- **Desired but unassessable Goal:** keep desire and Goal identity primary;
  explain the unsupported source, language, or missing prerequisite.
- **Stale or questionable evidence:** keep the book in place, label the evidence,
  omit unsafe comparisons, and provide an appropriate evidence-recovery path.
- **Failed recalculation:** preserve the learner's accepted order and action,
  state that updated evidence is unavailable, and provide a retry. Never roll
  back the preference silently.
- **Changing or clearing a Goal:** state what happens to reading history,
  prepared artifacts, vocabulary reservation, and unfinished work. The exact
  consequences await the Campaign/Goal contract and must not be invented in a
  generic confirmation.
- **Historical completed or abandoned Campaigns:** keep them understandable as
  reading/preparation/vocabulary-transition history without restoring Campaign
  as principal navigation.

## State model

| State | Required presentation | Primary decision |
|---|---|---|
| No Journey books | Calm explanation; My Books remains the source collection. | Add from My Books |
| Journey, no Primary Goal | Provisional order and evidence; no failure or idle warning. | Choose a Goal or reorder |
| Primary Goal, no assessment | Book and commitment first; name missing evidence. | Start the relevant evidence workflow when supported |
| Primary Goal, reading/preparation active | Independent reading and vocabulary facts; current versus conditional evidence. | Continue the learner-chosen activity |
| Route comparison available | Your order first; alternative and method secondary. | Keep, adopt, or adjust |
| Order recalculating | Preserve the accepted order; identify updating evidence. | None |
| Order recalculation failed | Preserve order and prior trustworthy evidence; explain failure. | Retry |
| Reading finished; transition complete | Factual outcome, exact justified vocabulary change, actual recalculation. | Where next? |
| Reading finished; vocabulary remains | Reading achievement, no known-vocabulary change, unchanged current evidence, conditional future effect. | Where next? or continue vocabulary work |
| No remaining Journey book | No completion framing; offer My Books and no-action option. | Choose another book or remain between Goals |
| Evidence stale/unavailable | Book remains in place; reason and excluded comparison are explicit. | Review or refresh evidence when supported |

## Accessibility and responsive contract

- Reading Journey is a semantic ordered list. Visual alignment or spatial
  treatment never replaces readable titles, authors, positions, and evidence.
- Reordering has visible **Move earlier** and **Move later** controls usable by
  keyboard, switch input, and touch. Drag is optional enhancement.
- After a move, retain focus on the moved book, announce its new position, and
  announce the recalculation result in a scoped polite live region.
- Route comparison uses headings and ordered lists before any visual connectors.
  It does not depend on color, relative position, or animation alone.
- Current, prior, projected, and remaining values use full text labels; `+` and
  `−` never carry meaning without units and direction.
- On narrow screens, stack Your order and the alternative while preserving the
  same comparison sequence. Keep per-book actions adjacent to their book and
  avoid page-level horizontal scrolling.
- Long titles, multiple authors, absent publication years, translated editions,
  and 200% text zoom must not hide order controls or status text.
- Server-rendered forms provide coherent add, remove, choose, and reorder
  outcomes before drag, animated transitions, or in-place recalculation enhance
  them.

## Product-contract boundary

This workflow deliberately does not decide:

- how My Books, Reading Journey order, or Primary Goal are persisted (Journey
  membership and order persistence are decided in
  [ADR 0034](../../adr/0034-reading-journey-identity-ordering.md);
  Primary Goal persistence awaits its own schema/UI milestone, while its
  identity and graduation/Goal-vs-Campaign semantics are decided in
  [ADR 0036](../../adr/0036-primary-goal-justified-graduation.md));
- how a Goal maps to or differs from a Campaign (decided in
  [ADR 0036](../../adr/0036-primary-goal-justified-graduation.md): a Goal never
  reserves vocabulary itself; Campaign remains the internal reservation/
  graduation mechanism, created when the learner elects deck-based vocabulary
  work);
- whether another Goal can begin while vocabulary work remains (decided in
  [ADR 0036](../../adr/0036-primary-goal-justified-graduation.md): an explicit
  graduate-or-abandon resolution, one-active exclusivity preserved, deterministic
  overlap);
- which cross-book optimization algorithm or invalidation scheme is accepted;
- new routes, APIs, migrations, event schemas, or undo behavior.

Journey identity, ownership, ordering, stale-write behavior, and the migration
of queued/active/complete/abandoned Campaign records are resolved in
[ADR 0034](../../adr/0034-reading-journey-identity-ordering.md). Primary
Goal identity, the single justified graduation transition (reading-finished
independent of deck-reviewed; snapshot + confirmed review), and new-Goal-with-
residual-work semantics are resolved in
[ADR 0036](../../adr/0036-primary-goal-justified-graduation.md). Remaining items
are the explicit planner/ADR work listed in
[`information-architecture.md`](../information-architecture.md#contract-changes-requiring-planneradr-work).
Until the cross-book projection contract (item 7) and routes/terminology
rollout (item 8) are accepted, ADR 0027's Campaign mutation behavior and ADR 0036
govern vocabulary reservation/graduation, and the existing feature documents
remain authoritative for domain behavior.

# Reading Journey and Primary Goal workflow

Status: **Canonical shipped learner-facing workflow.** Reading Journey and
Primary Goal are shipped; vocabulary study is book-anchored and its reservation
and graduation details are governed by [ADR 0053](../../adr/0053-book-anchored-vocabulary-consolidation.md)
(as re-expressed from ADRs 0027, 0034, and 0036, which the Book-anchored
decision consolidates). Reading-intent acquisition and analysis are governed by [ADR
0049](../../adr/0049-reading-intent-triggers-analysis.md), with the standalone
action retired by [ADR 0054](../../adr/0054-retire-standalone-analysis-action.md).
Reading Journey and
Primary Goal are one per study language ([ADR 0051](../../adr/0051-reading-journeys-and-goals-per-language.md));
this workflow describes the active study language's Journey and Goal.

The filename is retained to preserve existing links. **Learning campaign** is
retired as a learner-facing and internal plan object; a Book's vocabulary study
and its deck carry the reservation/graduation facts (ADR 0053).

## Goal

Help a learner keep a fluid idea of books they may read in the active study
language, choose one of those books to intend to finish, understand preparation
evidence without surrendering judgment, and return to a changed road ahead after
finishing the book. How many Goals are active across languages is the learner's
own discipline, not an enforced invariant.

Only Primary Goal carries commitment. Reading Journey remains provisional even
when Mouseion can compare a vocabulary-efficient alternative.

## Starting state and desired outcome

The workflow may begin with:

- books in My Books but no Reading Journey;
- a Journey but no Primary Goal;
- a Journey with members whose current analysis is queued, running, complete,
  stale, unavailable, or absent;
- a Primary Goal with a successfully completed current analysis, a prepared
  deck, reading progress, or vocabulary work;
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

Acquisition, analysis, deck preparation, Goal choice, reading completion, and
vocabulary graduation remain distinct transitions. Adding a Book to Reading
Journey is the learner-initiated trigger that also acquires its current EPUB and
ensures whole-book analysis; it does not silently trigger from catalog sync,
reordering, or any other transition. Goal choice remains a separate explicit
promotion after current analysis is complete.

## 1. Shape Reading Journey

**Learner question:** Which books do I currently imagine reading, and in what
order?

A learner can add books from My Books, remove provisional books, and reorder
them freely. The Journey presents one unambiguous **Your order** for the active language.
Later books do not look scheduled, overdue, locked, or committed.

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

**Learner question:** Which one book in this language do I intend to finish now?

**Choose as Primary Goal** is an explicit action available only from the Reading
Journey screen. It promotes a Journey member whose current analysis completed
successfully. Selection does not start or re-run analysis, prepare a deck, claim
reading has begun, or mark vocabulary known.

The Primary Goal region leads with:

1. title, author, and relevant edition identity;
2. the fact that this is the learner's current Goal;
3. reading state;
4. preparation/vocabulary-work state;
5. concise current evidence and clearly conditional projections;
6. one next useful decision, with supporting actions demoted.

An unassessed or unavailable Journey member remains visible in its learner-chosen
position, but it cannot be promoted to Primary Goal. The interface names the
evidence gap and offers the supported recovery path without treating membership
as failed.

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
No next Goal is created automatically. ADR 0036 defines this role transition and
keeps any residual vocabulary work explicit.

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

ADR 0036 permits this learner-facing rhythm: reading-finished is independent of
deck-reviewed, and residual vocabulary work remains explicit until it is
graduated or released. The UI states those facts separately.

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
- **Desired but unassessable Journey member:** keep the Book in its chosen
  position, explain the unsupported source or missing evidence, and do not expose
  Goal promotion until current analysis is complete.
- **Stale or questionable evidence:** keep the book in place, label the evidence,
  omit unsafe comparisons, and provide an appropriate evidence-recovery path.
- **Failed recalculation:** preserve the learner's accepted order and action,
  state that updated evidence is unavailable, and provide a retry. Never roll
  back the preference silently.
- **Changing or clearing a Goal:** state what happens to reading history,
  prepared artifacts, vocabulary reservation, and unfinished work according to
  ADR 0036; do not invent different consequences in a generic confirmation.
- **Historical graduated or released vocabulary study:** keep a Book's past
  decks understandable as reading/preparation/vocabulary-transition history
  without restoring a separate plan as principal navigation.

## State model

| State | Required presentation | Primary decision |
|---|---|---|
| No Journey books | Calm explanation; My Books remains the source collection. | Add from My Books |
| Journey, no Primary Goal | Provisional order and evidence; no failure or idle warning. | Choose a Goal or reorder |
| Journey member, no current assessment | Keep the Book in place; name missing/unavailable evidence. | Re-analyze or recover acquisition |
| Primary Goal, current analysis complete | Book and commitment first; show current evidence and independent reading state. | Continue the learner-chosen activity |
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
  [ADR 0034](../../adr/0034-reading-journey-identity-ordering.md) and
  [ADR 0036](../../adr/0036-primary-goal-justified-graduation.md));
- how vocabulary reservation and graduation work (decided in
  [ADR 0053](../../adr/0053-book-anchored-vocabulary-consolidation.md): a Book
  carries the vocabulary-study facet; its one current deck reserves and, on
  confirmed review, graduates the Book's snapshotted vocabulary. A Goal never
  reserves vocabulary itself);
- whether another Goal can begin while vocabulary work remains (decided in
  [ADR 0036](../../adr/0036-primary-goal-justified-graduation.md) and re-expressed
  in [ADR 0053](../../adr/0053-book-anchored-vocabulary-consolidation.md): an
  explicit graduate-or-release resolution, one-study exclusivity preserved,
  deterministic overlap). One study is exclusive owner-wide, even though Goals
  may be active in parallel languages under
  [ADR 0051](../../adr/0051-reading-journeys-and-goals-per-language.md); whether
  a per-language vocabulary study is ever warranted remains an open question
  recorded in ADR 0053;
- implementation details beyond the route, interaction, and state contracts
  recorded here and in ADRs.

Journey identity, ownership, ordering, and stale-write behavior are resolved in
[ADR 0034](../../adr/0034-reading-journey-identity-ordering.md), with per-study-
language Journey and Goal identity resolved in
[ADR 0051](../../adr/0051-reading-journeys-and-goals-per-language.md). Primary Goal
identity, the single justified graduation transition (reading-finished
independent of deck-reviewed; snapshot + confirmed review), and new-Goal-with-
residual-work semantics are resolved in
[ADR 0036](../../adr/0036-primary-goal-justified-graduation.md) and made
book-anchored in [ADR 0053](../../adr/0053-book-anchored-vocabulary-consolidation.md).
Cross-book projection and route/terminology rollout are resolved by ADR 0037 and
the shipped implementation respectively. The historical Campaign contract
remains provenance where not superseded by ADR 0053.

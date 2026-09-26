# Reading Journey and Primary Goal workflow

Status: **Canonical learner-facing workflow contract.** Reading Journey and
Primary Goal are shipped; Goal-owned vocabulary snapshots, per-language
reservation, completion, and sequential forecast are governed by [ADR 0072](../../adr/0072-goal-owned-vocabulary-and-journey-forecast.md).
Reading-intent acquisition and analysis are governed by [ADR
0049](../../adr/0049-reading-intent-triggers-analysis.md), with the standalone
action retired by [ADR 0054](../../adr/0054-retire-standalone-analysis-action.md).
Reading Journey and
Primary Goal are one per study language ([ADR 0051](../../adr/0051-reading-journeys-and-goals-per-language.md));
this workflow describes the active study language's Journey and Goal.

The filename is retained to preserve existing links. **Learning campaign** is
retired as a learner-facing plan; legacy campaign and deck records remain
supporting provenance and do not define active reservation or knowledge state.

## Goal

Help a learner keep a fluid idea of books they may read in the active study
language, choose one of those books to intend to finish, understand preparation
evidence without surrendering judgment, and return to a changed road ahead after
finishing the book. How many Goals are active across languages is the learner's
own discipline, not an enforced invariant.

Only Primary Goal carries commitment. Reading Journey remains provisional, and
its one stored order is explained by current, after-Goal, and on-arrival
coverage rather than a competing route.

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
    -> Review current, after-Goal, and on-arrival forecast
    -> Choose one book as Primary Goal
    -> Prepare and read, preserving independent facts
    -> Record Reading finished
    -> Accept the Goal snapshot into modeled Known vocabulary
    -> Recalculate remaining Journey from actual state
    -> Choose what to read next (`/reading`)
    -> Choose, reorder, add/remove, or remain between Goals
```

Acquisition, analysis, Goal choice, completion, and deck artifact preparation
remain distinct transitions. Choosing a Goal freezes its recurring-vocabulary
snapshot. Adding a Book to Reading
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

Unassessed and otherwise untrustworthy books retain the learner's chosen
position. Mouseion names the evidence gap, gives no fabricated coverage, and
labels downstream values as lower bounds when an earlier contribution is
unavailable.

## 2. Read the Journey forecast

**Learner question:** What coverage is true now, what follows from my Goal, and
what should I expect when I arrive at each Book in my order?

Mouseion presents one **Your order** with three distinct meanings:

- **Current coverage** uses current Known vocabulary;
- **After-Goal coverage** adds the active Goal's Reserved snapshot;
- **On-arrival coverage** adds trustworthy modeled recurring vocabulary from
  earlier Books in this order.

An unavailable or stale earlier Book contributes no invented identities. Later
on-arrival values are labeled **lower bound** when such a predecessor could have
contributed vocabulary. Reordering recalculates the forecast without changing
Known vocabulary, Reserved vocabulary, or the active Goal. The hierarchy is:

1. the one learner order, with titles and authors;
2. the three labeled coverage meanings;
3. lower-bound or unavailable evidence where it changes interpretation;
4. internal method and threshold data retained by the analysis services, not a
   competing learner-facing destination.

Do not describe any Book as best, optimal, recommended, or the correct reading
order.

A manual move produces a neutral forecast recalculation. For example:

> Moving this book here changes its on-arrival coverage; later values are
> recalculated from the new order.

This is information, not a warning. The learner's literary preference remains
canonical.

## 3. Choose or view the Primary Goal

**Learner question:** Which one book in this language do I intend to finish now?

**Choose as Primary Goal** is an explicit action available only from the Reading
Journey screen. It promotes a Journey member whose current analysis completed
successfully and freezes its exact recurring-vocabulary snapshot. Selection does
not start or re-run analysis, claim reading has begun, or mark vocabulary Known.
Local deck production starts from that same snapshot, but artifact readiness
or failure does not change the Goal.

The Primary Goal region leads with:

1. title, author, and relevant edition identity;
2. the fact that this is the learner's current Goal;
3. reading state;
4. Reserved vocabulary and artifact state;
5. current coverage and coverage after completion, with no on-arrival stage for
   the Book the learner has already reached;
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
- Goal snapshot absent, empty, or populated;
- modeled Known vocabulary and Reserved vocabulary;
- vocabulary that would become Known only after accepted Goal completion.

Current, after-Goal, and on-arrival coverage may appear together only when each
condition is explicit. Reading progress is never a Journey progress percentage.

The learner may leave and return while analysis or preparation runs. Operational
status remains secondary to book identity and current reading purpose.

## 5. Finish the Primary Goal

**Learner question:** What did finishing this book change, and what remains
unfinished?

Completing the Goal records that the learner finished the Book and accepts its
frozen snapshot into modeled Known vocabulary, not proof of per-card mastery.
The book no longer occupies the current Primary Goal role; it is removed from
the active Journey and remains in My Books, history, and provenance. No next Goal
is created automatically. ADR 0072 defines this atomic transition.

The outcome view uses a restrained, book-led receipt rather than celebration
chrome.

### Goal completed with a non-empty snapshot

When the Goal has a non-empty frozen snapshot:

1. state **Reading finished** and identify the Book;
2. state the exact newly-Known and already-Known identity counts;
3. offer **Choose what to read next**, which opens the candidate chooser at
   `/reading` without selecting another Goal.

The receipt is deliberately restrained: it does not replay forecasts or offer a
direct next-Goal shortcut. The candidate chooser presents current choices after
completion.

### Goal completed with an empty snapshot

When the Goal's frozen snapshot is empty:

1. acknowledge **Reading finished** and identify the Book;
2. show zero newly-Known and zero already-Known identities;
3. offer **Choose what to read next**, returning to the candidate chooser.

The reading achievement and modeled vocabulary transition are stated separately.
Deck artifact readiness or review is not required, and the UI must not imply
verified mastery.

## 6. Choose what to read next

**Learner question:** Given what is true now, what do I want to do next?

After the factual receipt, return the learner to the candidate chooser at
`/reading`. The chooser presents eligible To Read books and evidence states,
without prescribing an order or advancing automatically. The learner may also
return to My Books to add or reconsider a Book.

Read history remains independent of workflow disposition. **Read again** returns
the completed Book to To Read; starting it later creates a new current reading
and a fresh snapshot rather than reusing the completed snapshot.

The learner explicitly starts a To Read candidate when ready. No action is
preselected, automatic, or labeled optimal. If there are no eligible
candidates, invite the learner to My Books without presenting an empty backlog
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
  snapshot, prepared artifacts, and language-scoped reservation according to
  ADR 0072; do not invent different consequences in a generic confirmation.
- **Historical graduated or released vocabulary state:** keep a Book's past
  decks understandable as reading/preparation/vocabulary-transition history
  without restoring a separate manual-study plan as principal navigation.

## State model

| State | Required presentation | Primary decision |
|---|---|---|
| No Journey books | Calm explanation; My Books remains the source collection. | Add from My Books |
| Journey, no Primary Goal | Provisional order and evidence; no failure or idle warning. | Choose a Goal or reorder |
| Journey member, no current assessment | Keep the Book in place; name missing/unavailable evidence. | Re-analyze or recover acquisition |
| Primary Goal, current analysis complete | Book, frozen snapshot, and current/after-Goal evidence first. | Continue the learner-chosen activity |
| Primary Goal, reading/preparation active | Reading, Reserved vocabulary, artifact, and forecast facts remain distinct. | Continue the learner-chosen activity |
| Journey forecast available | Your order first; current, after-Goal, and on-arrival meanings are labeled. | Reorder or choose a Goal |
| Order recalculating | Preserve the accepted order; identify updating evidence. | None |
| Order recalculation failed | Preserve order and prior trustworthy evidence; explain failure. | Retry |
| Goal completed; non-empty snapshot | Restrained receipt with Book title and exact newly-Known/already-Known counts. | Choose what to read next (`/reading`) |
| Goal completed; empty snapshot | Restrained receipt with Book title and zero counts. | Choose what to read next (`/reading`) |
| No remaining Journey book | No completion framing; offer My Books and no-action option. | Choose another book or remain between Goals |
| Evidence stale/unavailable | Book remains in place; reason and excluded comparison are explicit. | Review or refresh evidence when supported |

## Accessibility and responsive contract

- Reading Journey is a semantic ordered list. Visual alignment or spatial
  treatment never replaces readable titles, authors, positions, and evidence.
- Reordering has visible **Move earlier** and **Move later** controls usable by
  keyboard, switch input, and touch. Drag is optional enhancement.
- After a move, retain focus on the moved book, announce its new position, and
  announce the recalculation result in a scoped polite live region.
- Enhanced reorder keeps that polite live region stable while replacing only the
  forecast content. It announces recalculation immediately, then announces the
  saved position and forecast result; failed enhancement leaves a reload/retry
  action and preserves the native form path.
- Forecast uses one ordered list and full text labels before any visual
  alignment. It does not depend on color, relative position, or animation alone.
- Current, after-Goal, on-arrival, lower-bound, and remaining values use full
  text labels; `+` and `−` never carry meaning without units and direction.
- On narrow screens, stack the labeled forecast values while preserving the same
  sequence. Keep per-book actions adjacent to their book and avoid page-level
  horizontal scrolling.
- Long titles, multiple authors, absent publication years, translated editions,
  and 200% text zoom must not hide order controls or status text.
- Server-rendered forms provide coherent add, remove, choose, and reorder
  outcomes before drag, animated transitions, or in-place recalculation enhance
  them.

## Product-contract boundary

This workflow deliberately does not decide:

- how My Books and Reading Journey order are persisted (Journey membership and
  order persistence are decided in
  [ADR 0034](../../adr/0034-reading-journey-identity-ordering.md));
- how Goal choice, snapshot ownership, reservation, completion, migration, and
  forecast work (decided in
  [ADR 0072](../../adr/0072-goal-owned-vocabulary-and-journey-forecast.md));
- implementation details beyond the route, interaction, and state contracts
  recorded here and in ADRs.

Journey identity, ownership, ordering, and stale-write behavior are resolved in
[ADR 0034](../../adr/0034-reading-journey-identity-ordering.md), with per-study-
language Journey and Goal identity resolved in
[ADR 0051](../../adr/0051-reading-journeys-and-goals-per-language.md). Goal
identity, snapshot ownership, completion acceptance, per-language reservation,
and sequential forecast are resolved in
[ADR 0072](../../adr/0072-goal-owned-vocabulary-and-journey-forecast.md). The
historical Campaign contract remains provenance only.

# Interface components

Status: **Established implementation components plus shipped learner-facing
patterns; the My Books cover-grid target is shipped and Reading Journey
thumbnails remain accepted but not shipped.**
Workflow-specific Journey and Primary Goal markup may remain in the owning
views; ADRs define persistence and historical behavior.

This document defines Mouseion's reusable server-rendered interface patterns.
Established patterns are the durable contract between product design, Templ
markup, shared CSS, and accessibility tests. The shipped My Books, Reading
Journey, and Primary Goal surfaces use these patterns even where exact component
boundaries remain workflow-specific. This document complements the semantic
tokens and responsive rules in [`design-system.md`](design-system.md); it does
not define product lifecycle or storage behavior.

## Implementation boundary

The reusable layer is deliberately small:

- `internal/webapp/components.templ` owns semantic component markup;
- `internal/webapp/components.go` owns closed variants, view adapters, and safe
  state-to-presentation mappings;
- `internal/webapp/static/app.css` owns component layout and appearance through
  Mouseion semantic tokens;
- `internal/webapp/components_test.go` owns rendered structure, semantics,
  states, and keyboard-relevant contracts;
- `internal/webapp/foundations_test.go` verifies adoption on representative
  product surfaces and the accessible application shell.

Use a component when its purpose and behavior are the same across workflows.
Keep workflow-specific copy, eligibility rules, routes, and state transitions in
the owning view or handler. A shared CSS class is acceptable when a specialized
view needs the same visual and responsive contract but different semantic
markup.

## Established component index

| Pattern           | Purpose                                                                    | Variants or states                                | Adopted surfaces                                           |
| ----------------- | -------------------------------------------------------------------------- | ------------------------------------------------- | ---------------------------------------------------------- |
| Application shell | Consistent landmarks, primary navigation, and skip navigation              | Authenticated and anonymous                       | Every full page                                            |
| `PageHeader`      | Establish the page goal, context, and highest-priority action              | Optional description, status, and actions         | My Books, Reading Journey, Jobs, Vocabulary |
| `NextAction`      | Explain the current learner-facing lifecycle state and its next action      | State-specific description                         | My Books, Reading Journey |
| `Breadcrumb`      | Return from a nested resource to its parent context                        | One parent link in the focused-task/Job workflow | Focused deck preparation and analysis status |
| `StatusBadge`     | Compactly identify a resource state                                        | Neutral, information, success, warning, danger    | Current library, Goal and artifact surfaces            |
| `Feedback`        | Explain a result, degraded state, or blocking error                        | Information, success, warning, error              | Core book workflow, Jobs, Vocabulary                         |
| `EmptyState`      | Explain why a collection is empty and the next useful action               | With or without an action                         | Current library, Jobs, Vocabulary                            |
| `ResourceCard`    | Group one resource's identity, metadata, status, and action                | Content-defined; not a generic marketing card     | My Books, book actions, prepared books, Vocabulary           |
| `ActionGroup`     | Keep peer actions together while preserving reading order                  | Primary, secondary, and consequential children    | Job status and Goal completion                            |
| `StatGroup`       | Compare a small set of labeled numeric or categorical facts                | Optional detail per item                          | Coverage thresholds, Journey projections, preparation progress |
| `MetadataList`    | Present term-value facts with native definition-list semantics             | Content-defined                                   | Book and job facts                                         |
| `ResponsiveTable` | Contain tabular overflow without creating page-level horizontal scrolling  | Labeled focusable region                          | Jobs and known vocabulary                                  |
| `AsyncStatus`     | Present one live asynchronous operation with progress and recovery actions | Busy or settled; optional progress                | Analysis job status                                        |
| `Confirmation`    | Reveal consequences before submitting a consequential server action        | Neutral or danger; copy remains workflow-specific | Goal completion and catalog connections                  |
| Active language switcher | Change the learner's stored active study language from the authenticated shell | Active, no active language, newly arrived, no books/read-only | Every authenticated screen |

The application shell is visually an index margin on wide viewports and a top
index on compact viewports. It remains one navigation landmark with the same
document and keyboard order in both forms. Current context uses a textual link
and annotation rule; do not add icon-only destinations, counters, or a second
navigation system.

## Canonical shipped patterns

These names describe durable interaction purposes. They do not require a
one-pattern/one-Templ-component implementation.

| Pattern | Purpose | Required states | Canonical surfaces |
|---|---|---|---|
| `BibliographicBookItem` | Keep title, author, and edition identity primary while pairing intent, evidence state, and one contextual action. | Primary Goal, in Journey, outside Journey, reading finished, unassessed, stale/questionable, cannot assess, long/missing metadata | My Books, Reading Journey, Where next? |
| `PrimaryGoalSummary` | Present the one current commitment and independent reading, preparation, and vocabulary facts without dashboard-card dominance. The Goal leads with Book identity, shows current and after-completion coverage only, and keeps deck actions snapshot-bound. | No evidence, analysis active/failed/complete, reading active/finished, vocabulary work active/complete, deck missing/active/ready/failed/empty | Reading Journey, outcome transition |
| `JourneyOrder` | Present one semantic ordered list with explicit provisional membership and accessible reordering. | Empty, no Goal, recalculating, recalculation failed, unavailable evidence, compact viewport | Reading Journey, Where next? |
| `JourneyForecast` | Show current, after-Goal, and on-arrival coverage in the learner's one stored order, including lower-bound labeling. | No Goal, active Goal, unavailable predecessor, recalculating | Reading Journey, Goal outcome |
| `EvidenceDelta` | State a current, prior, conditional, or lower-bound value and its exact unit/basis without relying on sign, color, or position alone. | Actual change, unchanged current value, future conditional effect, lower bound, stale/unavailable evidence | Journey books, Goal outcome |
| `OutcomeSummary` | Acknowledge the factual reading outcome, exact modeled vocabulary change, changed forecasts, and the next choice. | Transition complete, no snapshot identities, no remaining Journey book | Primary Goal outcome / Where next? |

The patterns above should compose mostly through typography, ordered lists,
definition lists, actions, disclosures, and fine rules. They are not permission
to wrap every region in a card.

## Book-cover patterns

[Book Covers](../features/book-covers.md) owns the complete cover experience. The
My Books cover-grid pattern below is shipped; its Reading Journey thumbnail
variant remains an accepted target for a later implementation.

### `BookCoverMedia`

**Answers:** Is catalog-supplied visual identity available for this Book without
replacing its bibliographic identity?

Use one fixed portrait frame that preserves the complete image. The My Books
variant leads visually at approximately 2:3; the Reading Journey variant is a
modest aligned thumbnail. Both variants support valid image, no image advertised,
retrieval pending, and advertised image unavailable. A replacement in progress
continues showing the prior validated image.

Real images use empty alternative text when title text is adjacent. Placeholder
visuals are hidden from the accessibility tree. A cover and title that navigate
to the same Reading Journey anchor form one link and one focus stop; when no
anchor exists, cover media does not become an implicit control. Reserve intrinsic
space, lazy-load below-viewport images, preserve visible focus on the containing
link, and never make cover content or actions hover-only.

### `MyBooksCoverItem`

**Answers:** Which Book is this, is it in my Reading Journey, and what can I do
with it?

Use `BookCoverMedia`, then full title and available author, a restrained **In
Reading Journey** marker when applicable, one labeled **Add to Reading Journey**
or **View in Reading Journey** action, and a labeled native **More actions**
disclosure. The disclosure owns eligible metadata refresh and confirmed My Books
removal. Do not add analysis, acquired-content, evidence, Primary Goal, or
next-action status to this pattern.

Repeated items form a native unordered list laid out with CSS Grid, not an ARIA
grid. Compact layouts retain two ordinary columns where the viewport permits,
reduce columns without horizontal overflow, and keep title, author, membership,
and controls in document and keyboard order. Search-result replacement uses one
scoped live region and does not announce every cover or item.

## Shared rules

### Server-rendered baseline

Every pattern must be complete and usable in the initial HTML response. HTMX may
refresh `AsyncStatus`, but retry and cancellation remain ordinary forms. The
`Confirmation` pattern uses native `details` and `summary`; revealing and
submitting it does not require JavaScript.

JavaScript may enhance an interaction only after the server-rendered path has a
coherent destination, error state, and recovery action. An HTMX fragment must
preserve the same component root and live-region semantics when it replaces
itself.

### Content hierarchy

- A full page has one `h1`, normally supplied by `PageHeader`.
- Breadcrumbs precede the page header and name the parent resource, not the
  current page.
- The page description explains the task or decision. It is not a second title,
  status message, or implementation note.
- Page-header actions are limited to the highest-value next action. Secondary
  resource actions belong beside the resource they affect.
- On book-led surfaces, title, author, and relevant edition identity precede
  intent, evidence, status, and action. Metrics never become a surrogate title.
- Reading Journey exposes one learner order. Forecast stages explain that order
  and never become a second route or recommendation.
- A status badge never replaces a heading, explanatory sentence, progress
  summary, or error message.

### State and tone mapping

Tone communicates meaning consistently:

- **neutral** — known state, learner preference, or conditional evidence without
  urgency or positive/negative judgment;
- **information** — queued, running, preparing, recalculating, or other active
  work;
- **success** — a completed learner action or ready artifact, not the Primary
  Goal role or a high readiness value;
- **warning** — degraded capability, stale/questionable evidence, or a state
  requiring review;
- **danger** — failed, cancelled, discarded, or destructive consequence.

State text is always visible; color is supplemental. Workflow code maps domain
states to these closed variants instead of constructing arbitrary class names.

### Focus and announcements

- The application shell begins with a visually hidden skip link targeting the
  focusable `main` landmark.
- `Feedback` errors use `role="alert"`, assertive announcement, and
  `tabindex="-1"` so a workflow can move focus to the message when appropriate.
- Success and informational feedback use `role="status"` with polite
  announcement. Warnings use `role="note"` with polite live behavior; warnings
  must not be used for blocking errors.
- `AsyncStatus` is atomic, polite, and exposes `aria-busy`; its progress element
  has an accessible label.
- A responsive table's scroll container is keyboard-focusable, visibly focused,
  and labeled by purpose.
- Journey reordering exposes visible Move earlier / Move later controls. After a
  move, focus stays with the moved book and a scoped polite live region announces
  its position and evidence-recalculation outcome.
- Drag-and-drop, animation, connectors, and spatial alignment never provide the
  only way to understand or operate a sequence.
- Components do not automatically steal focus after a full-page response.

### Responsive behavior

At the compact breakpoint:

- page-header content and actions stack in reading order;
- bibliographic items preserve title, author, relationship, evidence, then action;
- resource-list layouts and action groups stack without reordering document
  order;
- Journey order remains a semantic list and its per-book move controls stay
  adjacent to the affected book;
- labeled forecast values stack in document order rather than compressing into
  unreadable columns;
- metadata lists use a single column;
- buttons remain native controls and may occupy the available width where the
  surrounding workflow requires it.

Tables retain semantic table markup. Their labeled region owns horizontal
overflow; the page must not scroll horizontally. Stat groups use auto-fitting
columns and expand at the wide-data breakpoint without increasing reading-line
length.

## Pattern contracts

### Application shell

**Answers:** Where am I in the product, what are the primary destinations, and
how can I reach the main task quickly?

Use native `header`, `nav`, and `main` landmarks. The navigation label is
`Primary navigation`. The canonical authenticated destinations are exactly My Books,
Reading Journey, Vocabulary, and Catalogs. Catalogs owns catalogue setup and sync
maintenance at `/catalogs`; My Books is the sole browse surface and its rows own
per-book acquisition.
Primary Goal belongs inside Reading Journey. The shipped shell marks the current
context while compatibility routes redirect without exposing Learning as a peer
destination.

The established shell provides landmarks, skip navigation, and explicit route
context for the active destination or workflow action.

The authenticated shell also carries the active language switcher. It is a
native, labeled `select` backed by an ordinary POST form; JavaScript may submit
it immediately on change, while a no-script submit button remains available.
Current study languages are selectable, newly arrived languages carry a visible
`new` marker, and known-vocabulary-only languages carry `no books` and remain
selectable for read-only Vocabulary. The effective selection is resolved on each
request from the stored learner pointer and current language context; rendering
never writes a default.

### `PageHeader`

**Answers:** What page is this, what can I accomplish here, and what is the most
important next action?

Use one concise title, an optional task-oriented description, an optional
non-interactive status, and at most one primary action group. Status belongs
with page identity rather than in the action slot. Do not place long metadata,
warnings, progress, or multiple resource actions in the header.

### `NextAction`

Use a concise state description to make the learner-facing lifecycle state
explicit. The action itself remains a native link or form button owned by the
screen; the summary must not introduce a second competing route. My Books rows
keep metadata-only actions local to the row. Reading Journey names the
intent-triggered ensure-once analysis consequence and owns **Re-analyze** for
stale members.

On Journey surfaces, `NextAction` must not turn the first provisional book or a
forecast value into a recommendation. Use plain relationship copy such as
**First in your current order** and learner-controlled actions such as **Choose
as Primary Goal**.

### `Breadcrumb`

Use for a nested resource where returning to the parent preserves task context.
The link label names the destination. Do not use breadcrumbs as a duplicate of
primary navigation or as a generic browser-back control.

### `StatusBadge`

Labels are short, explicit phrases such as **Deck ready**, **Analysis queued**,
**In Reading Journey**, or **Evidence needs review**. Never encode state only
through an icon or color. A badge is not interactive. Avoid ambiguous labels
such as **Active**, **Reading**, **Ready**, or **Completed** without the fact they
qualify. Primary Goal is a relationship role and should not receive generic
success styling.

### `Feedback`

Feedback contains an optional short title and a message that answers:

1. What happened?
2. What remains unchanged when that matters?
3. What can the learner do next?

Use error only when the requested task failed or cannot proceed. Use warning for
degraded capability that leaves the current task or saved data intact. Do not
nest forms or unrelated content inside feedback.

### `EmptyState`

Name the absent collection or resource, explain why it matters, and offer one
useful next action when one exists. Do not use an empty state when content is
still loading or when an error prevented loading.

### `ResourceCard`

Lead with resource identity, then supporting metadata or relationship, evidence
state, and action. Cards in a list must use the same internal order. Avoid
nested cards and avoid using a card solely to add decoration around prose.

The shipped repeated My Books and Journey items normally use a bibliographic
row/list-item treatment with fine rules, not `ResourceCard`. The accepted
`MyBooksCoverItem` deliberately replaces only the My Books row with a cover-grid
item; it is not a generic card license. Reserve a stronger contained surface for
a genuinely distinct region such as the one Primary Goal or a consequential
outcome; even there, typography should carry more hierarchy than border, shadow,
or background.

### `ActionGroup`

Group actions that affect the same resource or transition. The primary action
comes first in document order. Destructive or abandonment actions are visually
secondary and use `Confirmation` when the consequence is material.

### `StatGroup`

Use for two or more facts that benefit from comparison. Each item has a value,
a concise label, and optional detail. Numeric values use tabular numerals. A stat
group summarizes data; explanatory methodology and provenance remain prose or
details immediately after it.

Do not use `StatGroup` as the hero of My Books, Reading Journey, Primary Goal, or
Where next? Forecast values are supporting evidence after the books and the
plain-language consequence. Current, after-Goal, on-arrival, lower-bound, and
remaining values use full labels and units rather than color or a bare signed
number.

### `MetadataList`

Use native `dl`, `dt`, and `dd` for stable term-value facts. Do not use it for a
sequence of actions or for unrelated paragraph content.

### `ResponsiveTable`

Use only for genuinely tabular data whose rows share columns. Supply a unique,
task-oriented region label. Preserve headers and native row order. Do not replace
a table with visually aligned `div` elements at narrow widths.

### `AsyncStatus`

The title names the operation's current state; the summary describes useful
progress or recovery context. Busy operations may refresh their component root
with HTMX. Settled states stop polling. Retry and cancel are server-rendered
forms grouped after status and error content.

A failed operation uses `Feedback` inside the status region and provides an
available recovery action. A settled successful state provides the next
workflow action; operational completion must not skip the learner-facing result
screen.

### `Confirmation`

Use native disclosure for consequential actions rather than an inline
`window.confirm`. The summary names the proposed action. The body explains what
changes, what remains, and whether the action can be undone. The final button
uses explicit outcome language; generic **Confirm** or **Mark complete** copy is
insufficient.

Use neutral confirmation for an irreversible positive transition and danger for
deletion or material abandonment. Goal completion confirmation must state that
the exact eligible identities in the frozen snapshot become modeled Known
vocabulary and that the transition cannot currently be undone. Clearing or
changing a Goal must state that its reservation is released while snapshot,
deck, and provenance history remain.

The shipped Primary Goal workflow names the accepted completion transition and
the independent artifact facts. Confirmations must not imply verified mastery or
that deck readiness is required.

## Shipped pattern contracts

### `BibliographicBookItem`

**Answers:** Which book is this, why is it here, what evidence is trustworthy,
and what can I do with it?

Use a semantic list item or article with title as the primary link, author
immediately adjacent, and edition/year/language where evidence identity needs
it. Relationship text such as **Primary Goal** or **In Reading Journey** precedes
concise evidence state. Keep at most one primary contextual action visible; put
provenance and secondary actions behind ordinary links or disclosure.

### `PrimaryGoalSummary`

**Answers:** What one book am I committed to finishing, what is actually true
about reading and vocabulary work, and what decision is next?

Lead with book identity and Goal role. Present reading, Reserved vocabulary,
artifact, and modeled-vocabulary facts independently. Pair current,
after-Goal, and on-arrival evidence only when each condition is written in full.
Do not use a destination flag, progress trophy, oversized metric, or generic
success card.

### `JourneyOrder`

**Answers:** What is my current order, which books are provisional, and how can I
change it?

Use an `ol` for the learner's sequence. Primary Goal is a separate anchored
region before the provisional list. Each later item has visible Move earlier and
Move later buttons with unavailable boundary actions disabled or omitted
consistently; compact layouts may place the two move controls side by side.
Choose as Primary Goal is the principal visible action for an eligible item.
Removal and uncommon secondary actions use a native More actions disclosure and
material consequences use Confirmation. Exceptional evidence states keep their
supported recovery action in the affected row. Removal does not delete the book
from My Books. Recalculation must not block acknowledging the accepted learner
order.

### `JourneyForecast`

**Answers:** What coverage is true now, what follows from the active Goal, and
what may be available when I arrive at each later Book in my order?

Use one semantic ordered list. Label **Current coverage**, **After Primary Goal
coverage**, and **On arrival coverage** in full, and keep the active Goal's
frozen snapshot and any lower-bound condition explicit. Each available stage
retains its one-decimal percentage; equal stages add **No change**, while
changed stages name the comparison stage and signed percentage-point delta.
Exact token counts and calculation context live in a compact evidence disclosure.
An unavailable or stale predecessor contributes no invented identities;
downstream values are labeled as lower bounds. Reordering recalculates forecasts
without changing learner state or creating another order. The anchored Primary
Goal uses only Current coverage and After Primary Goal coverage.

### `EvidenceDelta`

**Answers:** What value is this, when is it true, and what changed?

Every delta names its measure, unit, basis, and time/condition: for example,
**On-arrival coverage: 82% (lower bound; one earlier Book is unavailable)**. A
signed value alone is invalid. Pair old/new values only when both are
comparable. Missing evidence remains explicitly unavailable or lower-bound.

### `OutcomeSummary`

**Answers:** What happened, what changed in modeled Known vocabulary and the
forecasts ahead, and what can I choose now?

Use this order: factual reading outcome; exact modeled Known-vocabulary change;
changed/unchanged later forecasts; **Where next?** Avoid celebration chrome,
Journey completion language, or an automatically emphasized next book. If no
book remains, provide My Books and a no-new-Goal path without framing the state
as failure.

## Adding or changing a component

Before expanding this layer:

1. identify at least two occurrences with the same user goal and behavior;
2. document purpose, content, variants, states, focus, announcements, keyboard
   behavior, and compact layout;
3. add a failing rendered contract before changing markup or CSS;
4. preserve the server-rendered path and valid native semantics;
5. adopt the pattern on representative real screens and realistic edge states;
6. regenerate Templ output and run focused and full repository verification;
7. update this document when the reusable contract changes.

Do not add variants only to make one screen cosmetically different. If a new
workflow needs different semantics or interaction behavior, keep it local until
the boundary is understood.

# Interface components

This document defines Mouseion's reusable server-rendered interface patterns. It
is the durable contract between product design, Templ markup, shared CSS, and
accessibility tests. It complements the semantic tokens and responsive rules in
[`design-system.md`](design-system.md); it does not define product lifecycle
behavior.

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

## Component index

| Pattern           | Purpose                                                                    | Variants or states                                | Current pilot surfaces                                     |
| ----------------- | -------------------------------------------------------------------------- | ------------------------------------------------- | ---------------------------------------------------------- |
| Application shell | Consistent landmarks, primary navigation, and skip navigation              | Authenticated and anonymous                       | Every full page                                            |
| `PageHeader`      | Establish the page goal, context, and highest-priority action              | Optional description, status, and actions         | Library, book, scope review, Learning, Jobs, Settings      |
| `NextAction`      | Explain the current learner-facing lifecycle state and its next action      | State-specific description                         | My Library, book detail, scope review                       |
| `Breadcrumb`      | Return from a nested resource to its parent context                        | One parent link in the Phase 3 contract           | Book and scope review                                      |
| `StatusBadge`     | Compactly identify a resource state                                        | Neutral, information, success, warning, danger    | Library, book, campaigns                                   |
| `Feedback`        | Explain a result, degraded state, or blocking error                        | Information, success, warning, error              | Core book workflow, Learning, Jobs, Settings               |
| `EmptyState`      | Explain why a collection is empty and the next useful action               | With or without an action                         | Library, Jobs, Settings                                    |
| `ResourceCard`    | Group one resource's identity, metadata, status, and action                | Content-defined; not a generic marketing card     | Library, book actions, prepared books, campaigns, Settings |
| `ActionGroup`     | Keep peer actions together while preserving reading order                  | Primary, secondary, and consequential children    | Job status and campaigns                                   |
| `StatGroup`       | Compare a small set of labeled numeric or categorical facts                | Optional detail per item                          | Book text profile, coverage, thresholds, projections       |
| `MetadataList`    | Present term-value facts with native definition-list semantics             | Content-defined                                   | Campaign progress                                          |
| `ResponsiveTable` | Contain tabular overflow without creating page-level horizontal scrolling  | Labeled focusable region                          | Jobs and known vocabulary                                  |
| `AsyncStatus`     | Present one live asynchronous operation with progress and recovery actions | Busy or settled; optional progress                | Analysis job status                                        |
| `Confirmation`    | Reveal consequences before submitting a consequential server action        | Neutral or danger; copy remains workflow-specific | Learning campaigns and catalog connections                 |

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
- A status badge never replaces a heading, explanatory sentence, progress
  summary, or error message.

### State and tone mapping

Tone communicates meaning consistently:

- **neutral** — known state without urgency or positive/negative outcome;
- **information** — queued, running, preparing, or other active work;
- **success** — ready, active, analyzed, completed, or another successful state;
- **warning** — degraded capability or a state requiring review;
- **danger** — failed, cancelled, discarded, or abandoned.

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
- Components do not automatically steal focus after a full-page response.

### Responsive behavior

At the compact breakpoint:

- page-header content and actions stack in reading order;
- resource-list layouts and action groups stack without reordering controls;
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
`Primary navigation`. The Phase 3 shell establishes consistent landmarks and
skip navigation; indicating the current destination remains a later shell
improvement because the current `Layout` call does not receive route context.
Do not infer current navigation from the page title.

### `PageHeader`

**Answers:** What page is this, what can I accomplish here, and what is the most
important next action?

Use one concise title, an optional task-oriented description, an optional
non-interactive status, and at most one primary action group. Status belongs
with page identity rather than in the action slot. Do not place long metadata,
warnings, progress, or multiple resource actions in the header.

### `NextAction`

Use a concise state description to make the learner-facing lifecycle action
explicit. The action itself remains a native link or form button owned by the
screen; the summary must not introduce a second competing route. My Library
and book detail use the same state projection, while scope review names
`Confirm this scope` and explains that confirmation does not start analysis.

### `Breadcrumb`

Use for a nested resource where returning to the parent preserves task context.
The link label names the destination. Do not use breadcrumbs as a duplicate of
primary navigation or as a generic browser-back control.

### `StatusBadge`

Labels are short noun or adjective phrases such as `Deck ready`, `Queued`, or
`Analysis required`. Never encode state only through an icon or color. A badge
is not interactive.

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

Lead with resource identity, then supporting metadata or status, then the
resource action. Cards in a list must use the same internal order. Avoid nested
cards and avoid using a card solely to add decoration around prose.

### `ActionGroup`

Group actions that affect the same resource or transition. The primary action
comes first in document order. Destructive or abandonment actions are visually
secondary and use `Confirmation` when the consequence is material.

### `StatGroup`

Use for two or more facts that benefit from comparison. Each item has a value,
a concise label, and optional detail. Numeric values use tabular numerals. A stat
group summarizes data; explanatory methodology and provenance remain prose or
details immediately after it.

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
changes, what is retained, and whether the action can be undone. The final
button uses explicit language such as `Complete campaign` or `Confirm
abandonment`.

Use the neutral confirmation tone for irreversible positive transitions such
as campaign completion. Use the danger tone for deletion and abandonment.

Campaign completion confirmation explains that assigned vocabulary becomes
known and that completion cannot currently be undone. Campaign abandonment
explains that the deck and history remain while vocabulary reservations are
released. Completion and abandonment never share generic `Confirm` copy.

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

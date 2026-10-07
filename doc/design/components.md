# Interface components

Status: **Established implementation components plus shipped learner-facing
patterns.** My Books compact-list and Reading title-page/chooser patterns are shipped. The
JourneyOrder, JourneyForecast, and PrimaryGoalSummary sections below are
historical contracts, not current Reading UI. Current behavior is defined by the
[Reading workflow](../features/reading-workflow.md).

This document defines Mouseion's reusable server-rendered interface patterns.
Established patterns are the durable contract between product design, Templ
markup, shared CSS, and accessibility tests. The shipped My Books and Reading
surfaces use these patterns even where exact component boundaries remain
workflow-specific. This document complements the semantic
tokens and responsive rules in [`design-system.md`](design-system.md); it does
not define product lifecycle or storage behavior.

## Implementation boundary

The reusable layer is deliberately small:

- `internal/webapp/components.templ` owns semantic component markup;
- `internal/webapp/components.go` owns closed variants, view adapters, and safe
  state-to-presentation mappings;
- `internal/webapp/static/app.css` owns component layout and appearance through
  Mouseion semantic tokens; Tailwind Preflight is confined to its base layer,
  with Mouseion document typography, prose lists, and native-control behavior
  restored by the shared foundation;
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
| `PageHeader`      | Establish the page goal, context, and highest-priority action              | Optional description, status, and actions         | My Books, Reading, Catalogs, Jobs, Vocabulary |
| `NextAction`      | Explain the current learner-facing lifecycle state and its next action      | State-specific description                         | My Books, Reading Journey |
| `Breadcrumb`      | Return from a nested resource to its parent context                        | One parent link in the focused-task/Job workflow | Focused deck preparation and analysis status |
| `StatusBadge`     | Compactly identify a resource state                                        | Neutral, information, success, warning, danger    | Current library, Goal and artifact surfaces            |
| `Feedback`        | Explain a result, degraded state, or blocking error                        | Information, success, warning, error              | Core book workflow, Jobs, Vocabulary                         |
| `EmptyState`      | Explain why a collection is empty and the next useful action               | With or without an action                         | Current library, Jobs, Vocabulary                            |
| `ResourceCard`    | Group one resource's identity, metadata, status, and action                | Content-defined; not a generic marketing card     | My Books, book actions, prepared books, Vocabulary           |
| `ActionGroup`     | Keep peer actions together while preserving reading order                  | Primary, secondary, and consequential children    | Job status and Goal completion                            |
| `MetadataList`    | Present term-value facts with native definition-list semantics             | Content-defined                                   | Book and job facts                                         |
| `ResponsiveTable` | Contain tabular overflow without creating page-level horizontal scrolling  | Labeled focusable region                          | Jobs and import rejection details                          |
| `AsyncStatus`     | Present one live asynchronous operation with progress and recovery actions | Busy or settled; optional progress                | Analysis, catalog-sync, enrichment, and prepared-deck status |
| `Confirmation`    | Reveal consequences before submitting a consequential server action        | Neutral or danger; copy remains workflow-specific | Goal completion and catalog connections                  |
| Active language switcher | Change the learner's stored active study language from the authenticated shell | Active, no active language, newly arrived, no books/read-only | Every authenticated screen |

### Vocabulary Browse and import controls

Vocabulary's peer views share one compact destination header: a 25px
“Vocabulary” heading followed by native links to Browse, Concordance, and
Import Known words. The links sit beside the heading when space permits and
wrap below it on compact screens. Mark the actual destination with
`aria-current="page"`; do not repeat a larger view title above its primary
task.

Vocabulary Browse uses the shared `.input` search field and `.btn` actions for
search and paging. The include-all checkbox remains native; its glyph stays
compact while its associated label provides a 44px-or-larger click target. The
default Known, Reserved, and prepared-Book-deck exclusions remain visible next
to the include-all control, and applied search context stays with the results.
The known-vocabulary upload also remains a native file control and multipart POST,
with an explicit label, platform picker, visible in-flight status when enhanced,
and server-rendered validation/recovery when enhancement is unavailable. Do not
add select, radio, or textarea component styles to these workflows: they are not
used here.

The application shell is one top bar at every width. Its document and keyboard
order is wordmark, four destinations, study language, then account. Wide screens
show that same order on one horizontal bar when space permits, with the study
language and account at the trailing end. Compact screens intentionally use two
visual rows: wordmark, active study language, and account disclosure first; the
four destinations second. This visual arrangement keeps the destinations
together and learner controls within easy reach while preserving the stable
keyboard sequence. It remains one navigation landmark; long labels and enlarged
text reflow without clipping. Current context uses a textual link and annotation
rule; do not add icon-only destinations, counters, or a second navigation
system. All four authenticated destinations share the same native-link treatment. The
current destination is identified by `aria-current="page"` and restrained
annotation-blue underline emphasis, never a filled button treatment or button
role. The active study-language switcher keeps its label, native POST behavior,
return destination, and no-JavaScript submit button. The account disclosure is a
native `details` element: its summary identifies the learner on wide screens
and says “Account” on compact screens; its panel always names the learner and
contains the native logout form. Keep its open state visible and all shell
controls keyboard-operable without JavaScript.

## Canonical shipped patterns

These names describe durable interaction purposes. They do not require a
one-pattern/one-Templ-component implementation.

| Pattern | Purpose | Required states | Canonical surfaces |
|---|---|---|---|
| `BibliographicBookItem` | Keep title, author, and edition identity primary while pairing intent, evidence state, and one contextual action. | Primary Goal, in Journey, outside Journey, reading finished, unassessed, stale/questionable, cannot assess, long/missing metadata | My Books, Reading Journey, Read history |
| `CurrentReadingTitlePage` | Identify the current Book and reading context first; keep lifecycle actions beneath its bibliographic identity and concise preparation/vocabulary evidence in a narrow adjacent margin. | Current date and title, long/missing cover, available/unavailable evidence, ready/busy/failed preparation, compact reflow | Reading |
| `ToReadCandidates` | Present other To Read Books as an unordered list with adjacent evidence and no ordinal or recommendation cues. | Empty, eligible, unavailable/stale evidence, compact reflow | Reading |
| `ReadingChooser` | Group neutral To Read candidates by current Known-vocabulary coverage band; require explicit confirmation to start or switch. | Empty chooser, no comparison, in-progress analysis, attention required, start/switch confirmation | Between-Books Reading chooser |
| `PrimaryGoalSummary` | Present the one current commitment and independent reading, preparation, and vocabulary facts without dashboard-card dominance. The Goal leads with Book identity, shows current and after-completion coverage only, and keeps deck actions snapshot-bound. | No evidence, analysis active/failed/complete, reading active, vocabulary work active/complete, deck missing/active/ready/failed/empty | Reading Journey |
| `JourneyOrder` | Present one semantic ordered list with explicit provisional membership and accessible reordering. | Empty, no Goal, recalculating, recalculation failed, unavailable evidence, compact viewport | Reading Journey |
| `JourneyForecast` | Show current, after-Goal, and on-arrival coverage in the learner's one stored order, including lower-bound labeling. | No Goal, active Goal, unavailable predecessor, recalculating | Reading Journey |
| `EvidenceDelta` | State a current, prior, conditional, or lower-bound value and its exact unit/basis without relying on sign, color, or position alone. | Actual change, unchanged current value, future conditional effect, lower bound, stale/unavailable evidence | Journey books |
| `OutcomeSummary` | Identify the completed Book, report exact newly-Known and already-Known counts, and return to the neutral chooser without implying mastery or selecting another Book. | Non-empty snapshot, empty snapshot, zero counts, retry/idempotent completion | Reading completion receipt |

The patterns above should compose mostly through typography, ordered lists,
definition lists, actions, disclosures, and fine rules. They are not permission
to wrap every region in a card.

## Book cover patterns

[Book Covers](../features/book-covers.md) owns the complete cover experience. The
My Books one-column list pattern below and its Reading thumbnail variant are
shipped.

### `BookCoverMedia`

**Answers:** Is catalog-supplied visual identity available for this Book without
replacing its bibliographic identity?

Use one fixed portrait frame that preserves the complete image. The My Books
variant leads visually at approximately 2:3; the Reading Journey variant is a
modest aligned thumbnail. Both variants support valid image, no image advertised,
retrieval pending, and advertised image unavailable. A replacement in progress
continues showing the prior validated image.

Real images use empty alternative text when title text is adjacent. Placeholder
visuals are hidden from the accessibility tree and carry the first title initial
over a tinted surface. Ignore a leading German (*Der*, *Die*, *Das*), Italian
(*Il*, *La*, *Lo*, *Gli*, *Le*, *L'*) or Modern Greek (*Ο*, *Η*, *Το*, *Οι*)
article; when the title is only an article, retain its initial. Keep the visible
pending, unavailable, or no-cover state label. A cover and title that navigate
to the same Reading Journey anchor form one link and one focus stop; when no
anchor exists, cover media does not become an implicit control. Reserve intrinsic
space, lazy-load below-viewport images, preserve visible focus on the containing
link, and never make cover content or actions hover-only.

### `MyBooksCoverItem`

**Answers:** Which Book is this, is it in my Reading Journey, and what can I do
with it?

Use `BookCoverMedia` in a compact one-column list row, then full title and
available author, a restrained workflow placement and **In Reading** marker when
applicable, one labeled contextual action, and a labeled native **More actions**
disclosure. Keep the cover small and the title authoritative. Disposition and
evidence sit in the right margin on wider screens and below the identity on
compact screens. The primary action is
**Move to To Read** for Inbox or Set Aside Books, and **View in Reading Journey**
for a Journey member. The disclosure owns **Set aside**, eligible metadata
refresh, and confirmed My Books removal. Do not add analysis, acquired-content,
evidence, Primary Goal, or next-action status to this pattern.

Repeated items form one native unordered list at every width, not an ARIA grid.
Keep cover, title, author, action, and note content in document and keyboard
order, with long bibliographic text wrapping without truncation. Search-result
replacement uses one scoped live region and does not announce every cover or
item.

My Books search remains a native GET search form. Its shared input contract fills
and can shrink within the available search track; the submit button stays
adjacent at wide widths and the group stacks in compact layouts. HTMX may enhance
submission, history, recovery, and focus without replacing the server-rendered
form or result states. My Books actions use shared button/input states while
keeping navigation as links, mutations as submit buttons, and native
confirmation disclosures for consequential actions.

Reading and focused deck-preparation actions follow the same shared control
contract: routes and downloads remain links; state changes remain submit buttons;
secondary and destructive consequences use the shared outline and danger
variants. Native confirmations retain a visible disclosure marker and their
consequential copy without requiring JavaScript. Scope checkboxes and radios
remain native small controls inside generously clickable labels rather than
being enlarged into distorted glyphs.

### Catalog connections and operational jobs

Catalog connection create/edit forms use explicitly associated labels, shared
`.input` fields, and native POST submission. Bounded create forms have deliberate
interior spacing; connection identity and actions lead, while sync state and its
explanation sit as a quiet margin note beside the connection. Their workflow
stylesheet owns only the two-column field composition and connection/status
layout; it does not
restate control color, boundary, focus, validation, theme, disabled, or motion
rules. These form selectors are scoped to the connection forms and cannot alter
the shell's study-language switcher.

Catalog sync, job retry/cancellation, and recovery use the shared `.button`,
`.button--outline`, and native `disabled` contracts. A running sync leaves
**Sync now** disabled and keeps the visible **Sync already in progress**
explanation and job-status link. Disabled appearance must remain distinct from
an enabled primary action in both themes; its contrast is not measured as an
enabled-control requirement. Sync status remains a text-backed live region.

Job history remains semantic table markup inside a labeled, keyboard-focusable
overflow region. Job detail and recovery keep `AsyncStatus`, its native progress
element, polite live updates, and ordinary retry/cancellation forms. Native
`details`/`summary` confirmations retain their visible disclosure affordance,
consequences, and submission behavior without JavaScript.

Analysis history and detail keep the Book title distinct from the operational
job number; detail keeps task state, progress, and recovery together, with
Book/connection identity, creation/finish timestamps, attempt, and provenance in
an adjacent definition-list margin note. Catalog-sync detail names its connection and
retains the metadata-only explanation. Focused Book deck preparation uses the
same compact page heading and an intentionally spaced bounded form; Book and
analysis identity, eligibility, external-translation consent, artifact state,
download, cancellation/retry, and re-prepare consequences remain explicit.

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

Every native disclosure uses the same chevron immediately before its visible
summary text. The cue points right while closed and turns down while open; it is
not replaced by browser markers, text glyphs, or workflow-specific variants.
Native Enter/Space behavior, visible keyboard focus, a 44px minimum target, and
reduced-motion support remain intact.

Concordance lookup, applied-scope filters, paging, and occurrence review use the
shared control and feedback states from the application foundation. Workflow
styles own only query/result composition and review layout; they must not replace
shared field, action, focus, or disabled-state treatment. Book and grammar filters
and occurrence decisions remain labeled native forms inside native disclosures.
Disclosure state is visible without color alone, checkable labels retain a
generous target around the native-size checkbox, and the result list remains one
server-rendered ordered list (with an explicit list role when CSS removes native
markers). Results read as a keyword-in-context list: each Book's title and
occurrence count label its group, dividers separate groups rather than individual
occurrences, and the matched form sits directly between its right-aligned left
context and left-aligned right context. One sentence names the applied lookup,
Book scope, grammar filter, and result range. Every occurrence has a visible
“Study” link; sentence study remains a separate named link, and return navigation keeps
its occurrence/results-summary focus contract. Playwright role/name assertions
are automated browser accessibility-tree evidence only; they are not real
VoiceOver verification. No manual VoiceOver verification is claimed by this
slice.

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
- When a Book is current Reading, its full title is the page's single `h1` in
  Literata, preceded by the real “Reading since” date; cover, author, and
  lifecycle confirmations remain with that identity. The linked title uses ink
  rather than the accent-link treatment and gains a visible hover/focus cue.
  Finish, switch, stop, and set-aside controls stay together beneath the author.
  Separate “Book deck”, “Reserved vocabulary”, and “Analysis” notes keep
  preparation, learner vocabulary, and analysis evidence distinct; their actions
  stay beside the relevant note. The cover is prominent beside the identity.
  Margin notes move below the Book on compact screens, after its identity and
  lifecycle actions.
- Every To Read candidate collection is unordered. The between-Books chooser
  groups candidates by truthful Known-vocabulary coverage bands; bands describe
  evidence, not learner intent, difficulty, or a recommended order.
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
overflow; the page must not scroll horizontally.

## Pattern contracts

### Application shell

**Answers:** Where am I in the product, what are the primary destinations, and
how can I reach the main task quickly?

Use native `header`, `nav`, and `main` landmarks. The navigation label is
`Primary navigation`. The canonical authenticated destinations are exactly My Books,
Reading, Vocabulary, and Catalogs. Catalogs owns catalogue setup and sync
maintenance at `/catalogs`; My Books is the sole browse surface and its rows own
per-book acquisition.
Reading owns current reading or the between-Books chooser. The shipped shell
marks the current context while compatibility routes redirect without exposing
Learning as a peer destination.

The established shell provides landmarks, skip navigation, and explicit route
context for the active destination or workflow action.

The authenticated shell also carries the active language switcher. It is a
native `select` backed by an ordinary POST form and labeled “Study language”;
visually hide the label on compact screens while preserving its accessible
name, and keep the short visible label on wider screens. JavaScript may submit
it immediately on change, while a no-script submit button remains available.
Name options by their language endonyms (Deutsch, Italiano, Ελληνικά), without
language codes.
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

Pair visible status words with a decorative shape hidden from assistive
technology: filled dot for current/ready, ring for neutral or in-progress, and
diamond for failure. Keep the shape distinguishable in both themes and
forced-colors mode; it supplements rather than replaces the label.

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

The shipped repeated My Books and Reading items normally use a bibliographic
row/list-item treatment with fine rules, not `ResourceCard`. `MyBooksCoverItem`
is a compact text-led list item with a small cover, not a cover-grid item or a
generic card license. Reserve a stronger contained surface for a genuinely
distinct region such as a consequential outcome; even there, typography should
carry more hierarchy than border, shadow, or background.

### `ActionGroup`

Group actions that affect the same resource or transition. The primary action
comes first in document order. Destructive or abandonment actions are visually
secondary and use `Confirmation` when the consequence is material.

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

For a previewed occurrence-lemma decision, show the selected sentence contexts
and projected identity, recurrence, Known/Reserved matching, and eligibility
before applying it. Keep the preview read-only; the final button names the
specific decision and selected occurrences, and reject it if the relevant state
has changed since preview.

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

**Answers:** Which Book finished, how many vocabulary identities became Known,
and where can I choose again?

Use this order: identify the completed Book; show exact newly-Known and
already-Known counts, including zeros; link to the `/reading` candidate chooser.
Do not replay forecasts, select a Goal, use celebration chrome, or imply Journey
completion. The chooser handles empty candidate states and the path back to My
Books.

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

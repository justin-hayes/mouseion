# Interface components

> **Study cutover.** [ADR 0136](../adr/0136-study-replaces-known-vocabulary-and-prepared-deck.md) merged the Reading page into My Books, moved Book-scoped vocabulary Browse to the Book detail page (`/books/{id}`), retired the Prepared deck and Known vocabulary, and made [Study](../features/study.md) the only vocabulary workflow. The Reading title-page and chooser patterns, and any deck pattern, are retired with their pages. Where this document names a Reading page or destination, a working desk, a prepared deck or Known vocabulary, read it as the retired design; the current contract is in the [Reading workflow](../features/reading-workflow.md).

Status: **Established implementation components plus shipped learner-facing
patterns.** My Books compact-list and Reading title-page/chooser patterns are shipped. The
JourneyOrder, JourneyForecast, and PrimaryGoalSummary sections below are
historical contracts, not current Reading UI. Current behavior is defined by the
[Reading workflow](../features/reading-workflow.md). The explicitly marked
[source-line target](#accepted-source-line-presentation-target) is accepted but
not implemented.

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
| `PageHeader`      | Establish the page goal, context, and highest-priority action              | Optional description, status, and actions         | My Books, Reading, Catalog, Jobs, Vocabulary |
| `Breadcrumb`      | Return from a nested resource to its parent context                        | One parent link in the focused-task/Job workflow | Focused deck preparation and analysis status |
| `StatusBadge`     | Compactly identify a resource state                                        | Neutral, information, success, warning, danger    | Current library, Goal and artifact surfaces            |
| `Feedback`        | Explain a result, degraded state, or blocking error                        | Information, success, warning, error              | Core book workflow, Jobs, Vocabulary                         |
| `EmptyState`      | Explain why a collection is empty and the next useful action               | With or without an action                         | Current library, Jobs, Vocabulary                            |
| `ResourceCard`    | Group one resource's identity, metadata, status, and action                | Content-defined; not a generic marketing card     | Book detail summary and Reading's current Book (page markup using the `resource-card` class) |
| `ActionGroup`     | Keep peer actions together while preserving reading order                  | Primary, secondary, and consequential children    | Job status and Goal completion                            |
| `ResponsiveTable` | Contain tabular overflow without creating page-level horizontal scrolling  | Labeled focusable region                          | Jobs and import rejection details                          |
| `AsyncStatus`     | Present one live asynchronous operation with progress and recovery actions | Busy or settled; optional progress                | Analysis, catalog-sync, enrichment, and prepared-deck status |
| `Confirmation`    | Reveal consequences before submitting a consequential server action        | Neutral or danger; copy remains workflow-specific | Goal completion and catalog connections                  |
| Study language picker | Open the same kind of page in another study language (ADR 0131) | Current, newly arrived, no books/read-only | Per-language pages only: My Books, Reading, Vocabulary |

### Vocabulary Browse controls

Vocabulary has one view, Concordance, under a compact 25px “Vocabulary”
heading with no peer-view links; do not repeat a larger view title above its
primary task.

Browse, on Reading's working desk for the current Book, uses the shared `.input` search field and `.btn` actions for
search and paging. The include-all checkbox remains native; its glyph stays
compact while its associated label provides a 44px-or-larger click target. The
default Known, Reserved, and prepared-Book-deck exclusions remain visible next
to the include-all control, and applied search context stays with the results.
Do not add select, radio, or textarea component styles to this workflow: they
are not used here.

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
role. The study-language picker keeps its label, native GET navigation, and
no-JavaScript submit button. The account disclosure is a
native `details` element: its summary identifies the learner on wide screens
and says “Account” on compact screens; its panel always names the learner and
contains the native logout form. Keep its open state visible and all shell
controls keyboard-operable without JavaScript.

## Study report controls

The Report control is a native `details` disclosure on the answer side of a
Study card, with a radio fieldset for the two report kinds, an optional note and
one **Send report** button; the form carries the CSRF token, language, card,
reps and material like a grade does. The reported card is a card face with
**Undo** (outline) and **Next** (primary), each its own CSRF form, so neither is
a bare link. Reported words entries reuse the Study card's sentence with its
marked target and one **Dismiss** form. See
[`screen-inventory.md`](screen-inventory.md#study-reports).

An Idiom's card (ADR 0137) offers one report kind only, **Unsuitable context**,
pre-selected and worded for an idiom: there is no Wrong-word choice, since an
Idiom is matched on a headword and has no single word to be wrong. While the
language's idiom switch is off the card has no Report control, because its
replacement material could not be prepared.

## Idiom card face

An Idiom's Study card, in Today and in Fluency alike, uses one face (ADR 0137,
the deck's idiom card, ADR 0067). The sentence marks every matched word in
bold, not one target; on Today the words come from the positions the material
stored, and a Fluency context, which stores its matched surfaces only, marks
them by surface. The answer side swaps the target rows for a definition list:
the headword with an **idiom** label, the Meaning, and the sentence translation
with its aligned excerpts highlighted. Today's Idiom card and Fluency's Idiom
card are the same markup (`cardFaceView`), and only the grades and responses
below it differ.

## Study source sections

A source-evidence word's (Ancient Greek, Latin) Study card shows two more
native `details` disclosures on the answer side, below the usual rows (ADR 0138
§3, [`0138`](../adr/0138-study-for-source-evidence-languages.md)). Both start
collapsed, so the answer stays the card's focus, and are keyboard-operable
without JavaScript. **Dictionary** is shown when a cited entry is still in the
dictionary: the compact outline of each cited entry (Lewis & Short entries
carry their number, and a senseless entry outlines the entry its cross-reference
names), with the senses the Gloss rests on marked only while the stored
dictionary pin is the dictionary's own. **Passage context** is shown only when
the word stores a passage whose translation the register credits: its label
states that it is not a translation of the sentence. A section with nothing to
show is omitted, never shown empty. A modern-language card has neither. The
imported-source notices, each with its pin and licence, sit at the foot of the
Study page, whether or not a card is shown.

## Canonical shipped patterns

These names describe durable interaction purposes. They do not require a
one-pattern/one-Templ-component implementation.

| Pattern | Purpose | Required states | Canonical surfaces |
|---|---|---|---|
| `AnnotatedBookRow` | Keep title and author in a text column beside a cover, with status in a visibly edged margin and actions under the identity. | Current reading, To Read, Read; a classical Book as an ordinary row with its Work title, Edition label and position (none for a whole Work), two Editions of one Work as two rows; the ordinary per-Book Analysis update notice with current-Book End wording; per-Book preparation notes with their own recovery; long title/author; compact reflow | My Books; reusable by Reading and chooser |
| `CurrentReadingTitlePage` | Identify the current Book and reading context first; keep lifecycle actions beneath its bibliographic identity and concise preparation/vocabulary evidence in a narrow adjacent margin. | Current date and title, long/missing cover, available/unavailable evidence, ready/busy/failed preparation, compact reflow | Reading |
| `ToReadCandidates` | Present To Read Books as an unordered list with adjacent evidence and no ordinal or recommendation cues; shown only when no Book is current. | Empty, eligible, unavailable/stale evidence, compact reflow | Reading, between Books |
| `ReadingChooser` | List neutral To Read candidates in title order; one ordinary entry per Book; require explicit confirmation to start or switch. | Empty chooser, no comparison, analysis in progress, not assessed, start/switch confirmation, the ordinary per-Book Analysis update notice | Between-Books Reading chooser |
| `PrimaryGoalSummary` | Present the one current commitment and independent reading, preparation, and vocabulary facts without dashboard-card dominance. The Goal leads with Book identity, shows current and after-completion coverage only, and keeps deck actions snapshot-bound. | No evidence, analysis active/failed/complete, reading active, vocabulary work active/complete, deck missing/active/ready/failed/empty | Reading Journey |
| `JourneyOrder` | Present one semantic ordered list with explicit provisional membership and accessible reordering. | Empty, no Goal, recalculating, recalculation failed, unavailable evidence, compact viewport | Reading Journey |
| `JourneyForecast` | Show current, after-Goal, and on-arrival coverage in the learner's one stored order, including lower-bound labeling. | No Goal, active Goal, unavailable predecessor, recalculating | Reading Journey |
| `EvidenceDelta` | State a current, prior, conditional, or lower-bound value and its exact unit/basis without relying on sign, color, or position alone. | Actual change, unchanged current value, future conditional effect, lower bound, stale/unavailable evidence | Journey books |
| `OutcomeSummary` | Identify the completed Book, report exact newly-Known and already-Known counts, and return to the neutral chooser without implying mastery or selecting another Book. | Non-empty snapshot, empty snapshot, zero counts, retry/idempotent completion | Reading completion receipt |
| `ClassicalWorkPage` | Show one Work's Editions, each with its Opening, length, one Add and one I've read this, above a read-only contents list. | Owned or not owned; Work with or without constituent books; compact reflow | Classical work page |
| `CatalogSourceSection` | Group Catalog results under the source that offers them, with one search and the Language filter above every source; every source uses the one `CatalogEntryCard`. | Classical or OPDS section; paged; no sources, no match, nothing in this language; owned or unowned entry; unowned entry offering Add and I've read this; retained cover or Title plate; compact reflow | Catalog |

The patterns above should compose mostly through typography, ordered lists,
definition lists, actions, disclosures, and fine rules. They are not permission
to wrap every region in a card.

### `ClassicalWorkPage`

**Answers:** What does this Work contain, in which Editions, and how long is
each, before I add one?

The Classical catalog has no index page of its own: its Works are cards in the
Catalog's Classical section (see `CatalogSourceSection`), one card per Reading
unit, whose title links here. `GET /catalogs/classical` and the retired
author-detail URLs permanently redirect to the Catalog filtered to Ancient
Greek.

Each Edition shows its identity (Work title with its Greek title, Edition
label), the Opening set whole in the reading typeface, marked as Ancient Greek
and preceded by its starting citation and a visually hidden "Begins at", a
caption stating the word count and how many constituent books divide the Work,
and the passage range. A Work is one Book (ADR 0128), so each Edition offers one
**Add** and one **I've read this**, or an **In My Books**
link once owned, above a read-only contents list in a responsive table of
constituent book, passage and word count. The contents list has no actions and no
preparation state. A page-level attribution line credits the Greek text to its
source layer and licence. The page shows no preparation state, coverage or
difficulty.

Title links and every submit button follow document order and keep the shared
focus ring and 44px target. Add posts the existing form with CSRF and returns to
the confirmation through the shared `Feedback` live region. No interaction
requires a pointer. On compact screens the table reflows without sideways
scrolling.

Canonical surface: Classical work page.

### `CatalogSourceSection`

**Answers:** Which sources offer something for my search in this language, and
what does each one hold?

The Catalog has one search field and one language filter above every source.
The filter defaults to the Last study language and offers **All languages**;
a learner with no study language starts on all languages. Both controls and any
per-source page are ordinary GET parameters carried in real links and forms, so
the surface works without JavaScript. The Language filter is the Catalog's only
language control: it always offers Ancient Greek, and the shell language picker is
not shown here. Results are grouped by source and keep document order: the
Classical section first, shown only when the filter is Ancient Greek or all
languages, then each OPDS connection in its configured order.

Every source uses one `CatalogEntryCard`: image, title, author, one detail line,
ownership, **Add** and **I've read this**. A Classical card is one Reading unit
(a Work in one Edition) with its Greek title and Edition label, and its title
opens the Work page; an OPDS card shows its entry. Each section is a grid ordered
by title then author and paged 12 at a time, independently of other sections. A
search matches author, English title and Greek title through the Go search key. The
image slot shows the entry's retained cover, served from Mouseion through an
owner-scoped endpoint, or the Title plate (see `BookCoverMedia`) while none is
retained; the Catalog never sends an upstream cover URL to the browser. An entry linked to an active Book
shows **In My Books** as a link to that Book's My Books row. An unowned entry
offers **Add**, whose copy says "Adding prepares the text for reading."; Add creates or restores the Book as To Read and starts the same
preparation as **Read again**. The confirmation names
the Book, links to My Books, and receives focus after the ordinary POST
redirect. Entries never show disposition, analysis or preparation state.

There is one result-summary live region, which announces the number of results
and sources after every search, filter or page change. Distinct empty states
explain no OPDS sources, no match for the query, nothing in this language, and
nothing to browse yet; each says whether to connect a source, rephrase, or
change the filter. The cover grid reflows to one column on compact screens
without horizontal scrolling, and search, filter, paging, source controls and
ownership links follow document order with a 44px target.

Canonical surface: Catalog.

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

Real images use empty alternative text when title text is adjacent. Where no image
is retained, the frame shows the **Title plate** (ADR 0130): the text's own title,
its Greek title when it has one, and its author, composed on a tinted surface. It
is presentation only, never stored or called a cover, and the Catalog, My Books
and Reading use it alike, including for every Classical Reading unit. Plates are
hidden from the accessibility tree because the adjacent visible title and author
identify the text. Keep the visible pending, unavailable, or no-cover state label
where the surface shows one. A cover and title that navigate
to the same Reading Journey anchor form one link and one focus stop; when no
anchor exists, cover media does not become an implicit control. Reserve intrinsic
space, lazy-load below-viewport images, preserve visible focus on the containing
link, and never make cover content or actions hover-only.

### `AnnotatedBookRow`

**Answers:** Which Book is this, where does it sit in my reading life, and what
can I do with it?

Use a semantic list item with a reserved cover column on the left, full title
and author directly beneath it in the text column, and contextual actions under
that identity in the same column. Keep the right-hand margin visibly edged with
a light rule or quiet surface; it carries one `StatusBadge` for the canonical
visible bucket (**Currently reading**, **To Read**, or **Read**) and any concise reading-history note. Do not use a “Disposition:”
label. Rows separate through whitespace and the margin edge, not an outer box
or heavy list rule. On compact screens, keep the cover beside title and author,
then place the margin note and actions below the text column.

Use one labeled contextual action and a native **More actions** disclosure.
**View in Reading** is primary for the current Book and To Read Books; a Read
Book offers **Read again** in More actions. Do not add
standalone analysis/acquisition controls or next-action recommendations. Its
margin note may state concise current evidence: analysis running, failed,
unavailable, not yet analysed, or current token-weighted Known coverage and its
gap to the next coverage band. Coverage annotates evidence; it is never
readiness, recommendation, or failure framing. When there is no evidence, show
no note rather than `0%`. A Read Book may show its latest completion date. Deck
state is shown only for the current-reading Book. Failed-analysis and
unavailable-content notes retain an adjacent, valid recovery action whenever
recovery is available; do not show a dead route or offer refresh for the current
Book.

A classical Book is an ordinary row. The identity carries the Work title, the
Edition label that tells two Editions of one Work apart, the Work position such as
"Histories, Book 1" and "Book 1 of 9" for a leftover per-book Book (a whole Work
has none), the Greek title, and Classical catalog provenance. A leftover Book on a
withdrawn definition stays an ordinary row with its own Remove. My Books shows two
tabs, **To Read** and **Read**. On compact screens the cover stays beside title and
author, while the margin moves below the text column.

Reading's between-Books To Read candidate list reuses the same cover/identity/edged-margin
composition, but not My Books' bucket note or action set. The section heading
already supplies the To Read context, so rows do not repeat that badge. The
margin carries current Known vocabulary coverage as a rounded percentage, or an
analysis/content problem with its recovery action; it never ranks candidates or
frames coverage as readiness. A quiet **Switch current reading** link remains
under the identity when the Book is eligible. Keep the list unordered.

Neither the chooser nor My Books has a Work-level Analysis update offer: each
Book carries the ordinary per-Book notice.

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

Book preparations and Catalog sync are the two operational tables on `/jobs`:
one Book preparations table carries Classical preparation, EPUB acquisition and
analysis rows with the same columns (kind, Book, status, stage, code, progress
where known, created), while Catalog sync carries the shared status badge, its
connection and its error text. A preparation row links to its Book only while
that Book is still a current My Books member, and a failed row keeps the raw
stage and code with the learner reason My Books explains for that Book as quiet
text beneath the code. Analysis detail keeps the Book title distinct from the
operational job number; detail keeps task state, progress, and recovery
together, with Book/connection identity, creation/finish timestamps, attempt,
and provenance in an adjacent definition-list margin note. Catalog-sync detail
names its connection and retains the metadata-only explanation. Focused Book deck
preparation uses the same compact page heading and an intentionally spaced
bounded form; Book and analysis identity, eligibility, external-translation
consent, artifact state, download, cancellation/retry, and re-prepare
consequences remain explicit.

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
markers). Results read as one uninterrupted keyword-in-context list across Book
boundaries: no Book headings, dividers, group gaps, numbered keys, or per-row
titles. Only the first occurrence in each page-local Book group carries a quiet
static source label (a single-line, visually ellipsized title plus an explicitly
page-local count) in a margin column; the label is plain text with no
disclosure, chevron, tooltip, or tab stop, keeps its complete text in the
DOM, and cannot enlarge the row or leave a gap after a one-hit Book. The
matched form sits directly between its right-aligned left context and
left-aligned right context. On narrow screens closed rows stay compact and the
expanded sentence takes the full row width; under severe enlargement the source
label moves after the example. One sentence names the applied lookup,
Book scope, grammar filter, and result range. In the shipped implementation,
every occurrence has a visible “Study” link; the accepted source-line target
below adds the speaker-overlap exception. Sentence study remains a separate
named link, and return navigation keeps its occurrence/results-summary focus
contract. Playwright role/name assertions
are automated browser accessibility-tree evidence only; they are not real
VoiceOver verification. No manual VoiceOver verification is claimed by this
slice.

### Accepted source-line presentation target

Status: **Accepted — implementation pending.** Promoted from the human-reviewed
[Prototype: line-aware Faust excerpts in sentence study and Anki](https://github.com/justin-hayes/mouseion/issues/2149#issuecomment-6239826608),
variant A at `d01dfe62`, through
[Decision: accept the Faust I poetry specification and handoff](https://github.com/justin-hayes/mouseion/issues/2151).
[Basic poetry support](../features/basic-poetry-support.md) owns behavior and
release checks; [ADR 0103](../adr/0103-source-structure-and-poetry-evidence.md)
owns exact-snapshot roles and frozen inputs. Do not copy the throwaway prototype
into production or treat it as analyzer/provider/browser acceptance evidence.

Reuse the current disclosure, focus/return, text-alignment and safe-fallback
patterns. This changes source presentation, not navigation or the study-context
unit:

- A one-line Concordance summary substitutes a muted ` / ` for each authorial
  break; retain existing boundary-space handling. Literal source `/` remains
  ambiguous until expansion.
- Expanded context, sentence-study source text and the Anki front show actual
  source lines, with hanging indentation for viewport soft wraps. No-break prose
  keeps its shipped layout. Do not show containing-group orientation, surrounding
  lines or line numbers.
- Sentence-study morphology uses one row of aligned word units per source line
  at every width, with hanging wraps. Preserve gaps, punctuation and existing
  local/whole-line alignment fallback. Its complete text alternative says
  **end of source line** after the last word of each line; never fabricate a word
  for an unaligned gap. Other source text is heard as a continuous sentence.
- Quiet small-caps metadata above the sentence shows the **speaker name only**,
  omitting nested directions and optionally the terminal period. A direction
  sentence uses **Stage direction** instead. Unknown attribution leaves the slot
  absent without a warning. Escape metadata as text; it is neither sentence text
  nor emphasized target content.
- Keep raw speaker-label Concordance evidence searchable and identify it as such.
  An entire sentence overlapping a speaker label has no Study action, even when
  some direction words are eligible vocabulary. Do not visually remove a label
  to make a sentence appear studyable; server-side access follows the same rule.
- Anki puts frozen attribution on the front in its separate appended
  `Attribution` field. `Text` uses explicit per-source-line markup, escaping
  segments before adding markup; raw LF collapses in HTML. Keep target and
  separable-particle emphasis within the correct source lines. Old manifests
  have no attribution and never fetch it from newer evidence.
- Translation stays one complete English prose paragraph, without line pairing.
  No attribution, source notice or orientation enters the provider payload.

Use existing typography and semantic colors, not a script/facsimile treatment.
Check 375px and 1200px, both themes, enlarged text, no horizontal page overflow,
native keyboard/disclosure operation and no-JavaScript operation. Automated
role/name assertions are not manual screen-reader verification. Actual Anki
import/re-import and line/field compatibility are release checks, not claims
established by prototype HTML.

**Source structure update** is a Book-level notice and existing native
`Confirmation`, not a per-occurrence/card banner or a new destination. Before
submission, explain the source switch/reanalysis, the number of confirmed
corrections/exclusions that will not apply, and that active Reading/decks remain
pinned until explicit End/restart/re-preparation. Reuse existing asynchronous
status and retry patterns. No-change checks are quiet; original-unavailable and
failed-check states must not masquerade as unchanged or promise an automatic
repair. Source lifecycle belongs to the feature/ADR, not this component guide.

### Accepted construction-extract target

Status: **Accepted; implemented.** Promoted from the human-reviewed
[Prototype: constructions and syntax in Sentence study](https://github.com/justin-hayes/mouseion/issues/2609#issuecomment-7127948787),
variant C on the throwaway branch
[`research/grc-sentence-study-constructions-prototype`](https://github.com/justin-hayes/mouseion/tree/1c3bd586/doc/research/grc-sentence-study-constructions-prototype).
[Ancient Greek syntax and constructions](../features/ancient-greek-syntax-and-constructions.md#sentence-study)
owns behavior and the full entry contents. Do not copy the prototype into
production.

A **construction extract** shows which words of a sentence belong to one
Construction occurrence. Sentence study's Constructions list and Concordance's
expanded construction row use the same extract, one Construction per extract.

- Reprint the whole sentence. Set the Construction extent at full weight, and
  the rest of the sentence muted and at regular weight. Underline the anchor.
- A gap made only of punctuation counts as part of the extent. A discontinuous
  extent needs no further notation: the non-member words between its parts stay
  muted.
- Never mark extents inside the main sentence with brackets, colour alone or
  tiered rules. Nesting is stated in words ("Inside construction N").
- The extract is `aria-hidden`. Pair it with a visually hidden "Words: …" line
  listing the extent in textual order, with "…" between discontinuous parts.
- A focused entry (arriving from Concordance's Study) gets a thicker accent edge
  plus visible text "· from Concordance", and is scrolled into view; other
  entries are not dimmed.
- With nothing to show, render no region: no heading and no empty state.

Use existing typography and semantic tones. Check 375px and 1200px, both themes,
enlarged text, long Greek sentences without horizontal page overflow, and
no-JavaScript operation.

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
  Finish, switch, and end controls stay together beneath the author.
  Separate “Book deck”, “Reserved vocabulary”, and “Analysis” notes keep
  preparation, learner vocabulary, and analysis evidence distinct; their actions
  stay beside the relevant note. The cover is prominent beside the identity.
  Margin notes move below the Book on compact screens, after its identity and
  lifecycle actions.
- Every To Read candidate collection is unordered. The between-Books chooser
  lists candidates in neutral title order, one ordinary entry per Book,
  including every Book of a Work. Each entry reuses the annotated Book row: cover
  beside full title, author, start/switch confirmation and word review beneath
  the author, and the band name plus rounded coverage and next-band
  distinct-word counts in a visibly edged margin. A Book's Analysis update offer
  is the ordinary per-Book notice. Bands describe evidence, not learner intent,
  difficulty, or a recommended order. Analysis
  progress and unavailable coverage occupy that margin while their review/retry
  action stays under the identity. Retry is an outline action. On compact
  screens the margin follows the identity and actions, and native disclosure,
  route, and stale-write safeguards remain unchanged.
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
Reading, Vocabulary, and Catalog. Catalog owns the Classical catalog section, every OPDS source section and source status at `/catalogs`;
Manage sources owns catalogue setup and sync maintenance at `/catalogs/sources`.
My Books is the sole browse surface and its rows own per-book acquisition.
Reading owns current reading or the between-Books chooser. The shipped shell
marks the current context while compatibility routes redirect without exposing
Learning as a peer destination.

The established shell provides landmarks, skip navigation, and explicit route
context for the active destination or workflow action.

Per-language pages (My Books, Reading, Vocabulary and their deep pages) carry
the study-language picker; job status, the Catalog, and Book-addressed pages
(lemma review, deck preparation) do not. It is a native `select` backed by an
ordinary GET form that navigates to the same kind of page under the chosen
language prefix, and labeled “Study language”;
visually hide the label on compact screens while preserving its accessible
name, and keep the short visible label on wider screens. JavaScript may submit
it immediately on change, while a no-script submit button remains available.
Name options by their language endonyms (Deutsch, Italiano, Ελληνικά), without
language codes.
Current study languages are selectable, newly arrived languages carry a visible
`new` marker, and known-vocabulary-only languages carry `no books` and remain
selectable for read-only Vocabulary. The selected option is the page's language,
read from the URL prefix; the stored Last study language never decides it, and
rendering never writes a default.

### `PageHeader`

**Answers:** What page is this, what can I accomplish here, and what is the most
important next action?

Use one concise title, an optional task-oriented description, an optional
non-interactive status, and at most one primary action group. Status belongs
with page identity rather than in the action slot. Do not place long metadata,
warnings, progress, or multiple resource actions in the header.

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
diamond for failure. A badge is set as a margin note, words and shape in the
status colour without a box or fill, and neutral states use pencil tone. Keep the shape distinguishable in both themes and
forced-colors mode; it supplements rather than replaces the label.

### `Feedback`

Feedback contains an optional short title and a message that answers:

1. What happened?
2. What remains unchanged when that matters?
3. What can the learner do next?

Use error only when the requested task failed or cannot proceed. Use warning for
degraded capability that leaves the current task or saved data intact. Do not
nest forms or unrelated content inside feedback. A confirmation may carry one
adjacent link to the next surface, such as opening the Books an Add created.

### `EmptyState`

Name the absent collection or resource, explain why it matters, and offer one
useful next action when one exists. Do not use an empty state when content is
still loading or when an error prevented loading.

### `ResourceCard`

Lead with resource identity, then supporting metadata or relationship, evidence
state, and action. Cards in a list must use the same internal order. Avoid
nested cards and avoid using a card solely to add decoration around prose.

The shipped repeated My Books and Reading items normally use a bibliographic
row/list-item treatment with fine rules, not `ResourceCard`. `AnnotatedBookRow`
is a compact text-led list item with a small cover, not a cover-grid item or a
generic card license. Reserve a stronger contained surface for a genuinely
distinct region such as a consequential outcome; even there, typography should
carry more hierarchy than border, shadow, or background.

### `ActionGroup`

Group actions that affect the same resource or transition. The primary action
comes first in document order. Destructive or abandonment actions are visually
secondary and use `Confirmation` when the consequence is material.

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

### `BibliographicBookItem` (generic identity contract)

**Answers:** Which book is this, why is it here, what evidence is trustworthy,
and what can I do with it?

This is the generic bibliographic-identity guidance for surfaces that do not
use the canonical [`AnnotatedBookRow`](#annotatedbookrow) list composition. On
My Books, and when Reading/chooser rows adopt the shared margin composition, use
`AnnotatedBookRow` instead. Keep title/author identity primary, make any
relationship explicit, and put provenance and secondary actions behind normal
links or disclosures.

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

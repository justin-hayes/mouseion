# Stitch view synthesis — My Books / Reading Journey / Primary Goal

Status: **Design evidence for review — not a specification or canonical product decision**

## Evidence reviewed

- **My Books scholarly catalogue** — `my_books_scholarly_catalogue`
- **Reading Journey folio** — `journey_folio_unified_navigation`
- **Completion / Where next? folio** — `outcome_folio_unified_navigation`

The supplied PNGs show these three views. The referenced `mouseion` folder did not have a separately identifiable screenshot in the supplied set. Interaction, focus, narrow-viewport, empty, loading, error, and unfinished-vocabulary behavior therefore remain unverified.

## Connected-experience assessment

The exploration establishes a promising **editorial, book-led shell** and a credible completion-to-reconsideration rhythm. My Books is the strongest visual direction. The Journey view contains the right conceptual regions, but its aggregate metrics dominate and its sequence is not visibly manipulable. The completion view successfully returns to changed books, then undermines learner authority by declaring an “optimal next text.” Navigation, branding, terminology, and typography vary enough across the views that they do not yet feel like one product.

## Adopt

- **Book-led editorial character.** Serif titles, generous reading space, fine rules, and restrained color feel scholarly without themed decoration.
- **My Books as a bibliographic catalogue rather than a dashboard.** Title, author, year, and language precede analysis. Cards and metrics do not overwhelm the collection.
- **Primary Goal embedded in the Journey rather than made a separate top-level destination.** Isolating the current book above the provisional sequence gives it unique commitment weight.
- **A plainly named provisional region.** “Provisional Sequence” and “Reorderable” communicate that later books are not commitments.
- **Side-by-side route evidence.** Comparing “Your order” with a “Vocabulary-efficient alternative” is the right relationship: learner intent first, analysis second.
- **Neutral recalculation after manual preference.** Showing a recalculated quantity and `+63` without warning styling supports informed divergence.
- **Completion as a factual transition.** “Book finished,” the exact 142-identity vocabulary transition, recalculated values, and a return to the Journey form the correct narrative sequence.
- **Restrained completion tone.** No confetti, progress trophy, Journey percentage, or artificial culmination appears.
- **The “Where next?” moment.** Ending on a new choice rather than Journey progress or automatic advancement is structurally correct.

## Adapt

- **Unify the product shell.** Use one Mouseion identity, one navigation model, and one typographic system. The screenshots alternate between Mouseion and Scholar Workspace, between one and two navigation bars, and between different type scales and density.
- **Keep only principal navigation.** My Books, Reading Journey, and Settings are sufficient. Primary Goal belongs inside the current experience; after completion it should not persist as an apparently active sidebar destination. Remove duplicated top-and-side navigation.
- **Make My Books denser and more complete.** Preserve the catalogue treatment, but show more than three books per desktop page, add search, and expose meaningful evidence states such as unassessed or analysis running. Replace “Add OPDS Source” with ordinary acquisition language.
- **Clarify book states.** “Reading” should identify the Primary Goal when that is what it means. “Completed” must say whether the book was read, vocabulary work completed, or both.
- **Restore bibliographic identity throughout.** Journey and completion rows need authors and, where relevant, edition or evidence context. Titles alone make the later views feel like ranked line items.
- **Demote route totals.** The 1,742 and 1,495 figures currently become the page’s visual subject. Lead with the two book orders and the consequence—“247 fewer modeled additional vocabulary identities”—then place totals and method in supporting evidence.
- **Correct route terminology.** “Total coverage mapped” mislabels an identity count. Use explicit language such as “modeled additional lemma identities across these books,” with threshold and conditional basis available.
- **Resolve the three-order ambiguity.** “Your order,” “Elena’s manual preference,” and the provisional sequence appear to represent different states without showing their book orders. Show one current learner order, one alternative, and any pending rearrangement as an explicit preview.
- **Make reordering real and accessible.** Provide visible Move earlier / Move later controls, keyboard operation, focus retention, and textual recalculation. Drag may enhance but cannot be the only mechanism.
- **Pair Journey books with stage-specific evidence.** Show enough current and conditional coverage or additional-identity evidence to explain cumulative effects; keep exact details behind disclosure rather than converting the whole view into a table.
- **Label completion deltas precisely.** `+164 additional` is ambiguous. Say “164 additional lemma identities to 97%” and distinguish actual current coverage, previous coverage, and any remaining conditional preparation. Do not use a plus sign as both gain and requirement.
- **Make the next choice neutral.** Replace “optimal next text” with either “first in your current order” or a clearly labeled vocabulary-efficient alternative. Replace “Begin” with “Choose as Primary Goal”; keep Reorder and Choose another book as peers.
- **Separate reading from knowledge.** Remove the unsupported “reading comprehension phase complete” claim. The completion receipt should state only observed reading and justified vocabulary transitions.
- **Design the unfinished-vocabulary branch.** When reading is finished but vocabulary work remains, confirm the reading achievement, state that no vocabulary has yet been added to known, preserve current values, and keep projected changes conditional. The learner may still reconsider what comes next.
- **Design responsive transformations, not simple shrinkage.** The wide sidebars, large whitespace, comparison columns, and long titles will not translate directly. On narrow screens, use a compact header, stack route alternatives, keep book order intact, and preserve evidence labels.
- **Verify accessibility.** Increase contrast for muted text and disabled controls, do not use italics or position alone to encode hypothetical status, label icon and ellipsis actions, expose selected tabs, and test focus order and reduced motion.

## Reject

- **“Target Destination.”** Primary Goal is the current committed book, not an endpoint.
- **“Optimal next text.”** Mouseion may optimize a lexical property; it cannot declare the correct reading choice.
- **Optimization-led primary action.** A prominent “Begin Il Gattopardo” based on vocabulary gains turns evidence into prescription.
- **“Reading comprehension … complete.”** Mouseion has no evidence for a general comprehension claim.
- **Aggregate metrics as the Journey’s hero content.** This makes the experience an optimization dashboard rather than a provisional reading plan.
- **A static numbered sequence that only says “Reorderable.”** Without direct controls and consequences it reads as a queue.
- **Generic `Reading` and `Completed` pills.** They collapse distinct reading, preparation, and vocabulary states.
- **Learner-facing OPDS terminology.** Acquisition infrastructure should not become the collection’s primary action language.
- **Persistent oversized “Open Reader” chrome.** It competes with the current book and is disconnected from the shown state.
- **Flag iconography for Primary Goal.** It adds destination/project symbolism without clarifying commitment.

## Open

- How should the embedded Primary Goal balance bibliographic identity, reading progress, vocabulary work, and action without becoming a large dashboard card?
- What exact interaction lets the learner compare, partially adopt, or dismiss an alternative order while always preserving one clear current order?
- Should the completion screen show all later books, only materially changed books followed by the full Journey, or a concise change summary first?
- How does “Where next?” behave with no Primary Goal, an empty Journey, or a first Journey book that lacks comparable analysis?
- How visible should unfinished vocabulary work remain after the book ceases to be the Primary Goal?
- What compact mobile representation preserves route comparison and exact current-versus-projected evidence?
- What belongs directly in My Books versus book detail so unassessed and incomplete works remain legible without reducing catalogue calm?
- The unfinished-vocabulary, empty, error, keyboard, reduced-motion, and responsive states require another prototype pass; the screenshots provide no evidence for them.

## Recommended canonical experience

Use a single Mouseion shell with **My Books**, **Reading Journey**, and **Settings** as principal destinations. My Books is a moderately dense, searchable bibliographic catalogue with precise lifecycle labels and analysis kept secondary. Reading Journey begins with one embedded Primary Goal, followed by a visibly provisional, directly reorderable sequence. Learner order is canonical; vocabulary-efficient ordering appears as optional comparison evidence with exact assumptions and neutral consequences.

Completion proceeds in four beats: factual reading outcome; justified vocabulary transition or explicit absence of one; recalculated changes to later books; **Where next?** The first book in the current learner order is a natural candidate, never an automatic or “optimal” choice. If vocabulary work remains, the Journey returns without claiming readiness gains.

If approved, later documentation work should update learner-facing terminology, information architecture, the Primary Goal / internal Campaign boundary, completion workflow, route-comparison component rules, and accessibility/responsive contracts. The old Stitch handoff should remain historical evidence. **No canonical documentation should change until this synthesis is reviewed.**

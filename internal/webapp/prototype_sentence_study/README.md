# Sentence study tree prototype — THROWAWAY, NOT FOR MERGE

Question: can a complete, legible dependency tree explain a long sentence without
the excessive width of the first top-down prototype? Starting from the preferred
width-aware tree, compare curved edge labels with separate role waypoints and
contour packing. Both include the word-level morphology companion.

Decision ticket: [Choose a dependency visualization through a sentence study prototype](https://github.com/justin-hayes/mouseion/issues/1986).
Map: [Find the way to pedagogically useful sentence study](https://github.com/justin-hayes/mouseion/issues/1983).

## Run

```sh
make prototype-sentence-study
```

Open <http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=curves&example=de-long>.
The bottom bar compares `curves` (curved edge labels, previous B's subtree-row
placement) and `roles` (separate role waypoints with width-aware contour packing).
Both arrows cycle through these layouts; arrow shortcuts avoid controls,
and the diagram. `example=de`, `it`, `el`, or `de-long`
selects an invented fixture. The long sentence is now the default.

Words and morphology are plain text: no links, selections or pop-over inspector.
The Text alternative control replaces the graphic with the full relationship
list, including raw POS, lemma and morphology. View options exposes diagram
dimensions, an optional punctuation filter, and a manual width budget. With
JavaScript, both layouts use the diagram viewport's width
by default; without JavaScript, set the budget and manual-width checkbox yourself.
All tokens, including punctuation, are included by default. There is no automatic
word folding, diagram cutoff, text shrinking, prompt, reveal, or nearby-context
control, and no permanent content beneath the tree.

## Live feedback and earlier evidence

The [first comparison, captured at its original commit](https://github.com/justin-hayes/mouseion/tree/7c9dc172/internal/webapp/prototype_sentence_study),
compared tree, arcs, and selected-word-led layouts. Justin chose the tree, agreed
to tucked-away raw labels/morphology, and rejected the prompting/reveal and nearby
context controls after trying them. He asked to concentrate on the visualization,
with long-sentence width treated as a common-use problem rather than accepted
because a horizontal scroll region exists.

[Live feedback and confirmed next pass](https://github.com/justin-hayes/mouseion/issues/1986#issuecomment-6145928770)
records that this revises the prompt/reveal and context preferences in
[Define the sentence study learning and review experience](https://github.com/justin-hayes/mouseion/issues/1984#issuecomment-6144870770).
The final ticket resolution/map must explicitly supersede those preferences.
Comprehension remains the learning job; fallible evidence is not automatic grading.
No final layout winner or production acceptance boundary is chosen yet.

## Layout experiments

Reuse the accepted Margin notes typography, semantic palette and real app shell:
Commissioner for controls/annotations; Literata for complete source and words.
Keep the sentence as the reading anchor; let the tree dominate the working area.
Bare words are static, with no visible boxes or
word numbers in the graphic. Source ordinals remain in accessible names and
the text alternative to distinguish repeated words.

```text
A: original sentence → morphology line → curved, width-aware tree with edge labels
B: original sentence → morphology line → curved tree with separate role waypoints
```

A retains the previous B's full-subtree rectangular packing, but replaces the
angular shared bus with individual curves. First-row branches fan out directly;
later rows use the head's reserved connector gutter.

B gives each role/word pair reserved space. Its contour packing lets neighboring
subtrees share unused space across staggered rows, rather than reserving an
entire tall sibling's rectangle. Each connection stops before the role text and
resumes below it toward the word. Roles are visual waypoints, not additional
tokens or grammatical heads; the underlying head/dependent relationships are
identical in A/B. The experiment changes packing as well as annotation placement:
any smaller diagram is not evidence that adding role labels alone saves space.

Both keep heads above dependents without aligning all equal-depth words on one
row. No source-order layout, all-sentence fitting, or crossing-free guarantee
is claimed. Deep nesting and long words/labels can still exceed a narrow budget.

Both use separately hand-supplied feature labels in `feature_labels.json`, not
an automatic morphology converter. They cover selected evidence, not a complete
analysis. Missing labels are dashes, not claims of absent features. The source
contraction appears once above separately labeled expanded words. The fixtures'
`Span` values are manually authored source strings, not an offset-grouping
algorithm. There are no lexical glosses, morpheme boundaries, or translations.
See the [corrected Leipzig research](https://github.com/justin-hayes/mouseion/blob/849309d3de9c0596f286603ddf49b905556ac441/doc/research/sentence-study-leipzig-glossing.md)
for the distinction and production data limitations. Persisted word morphology
exists but is not currently projected into the shipped Sentence study view.

Words are HTML text positioned over a decorative server-rendered SVG. The DOM
retains source order and each word's accessible description includes ordinal,
POS, role and head. Native Text alternative/Show tree links work without JS;
the enhancement only fits the diagram to the viewport. Raw versus effective
lemma evidence must remain distinct in a real implementation; no corrections
are simulated here.

The user supplied two Aretusa screenshots as readability references. Both
variants use 20px/550 verbs, 18px lexical words, 16px function words, and smaller
13–14px role annotations. POS-based coloring uses Mouseion's existing foreground
tokens (red verbs/auxiliaries, ink nouns/names, blue adjectives/pronouns, ochre
adverbs, pencil function words). These colors are an explicit prototype word-class
legend, not error/warning status. Faint low-contrast article colors are not copied.
No Aretusa-specific grammar labels, inferred empty words or synthetic heads are
introduced into the invented UD fixtures.

## What to judge

1. Compare A/B: do independently spaced role annotations and contour packing
   make the long hierarchy easier to follow, or just more densely intertwined?
2. Are the curves, font sizes, weights and word-class colors legible on short
   German/Italian/Greek examples as well as the long fixture?
3. Does static morphology plus raw evidence in the text alternative remove the
   unwanted interaction without losing useful explanation?
4. Compare Greek source `στην` with the two separate illustrated nodes `σε` and
    `την`, both discoverable as sharing the source span. Source text is not reconstructed
   from expanded words.
5. At 320 CSS px, use the diagram's own scroll region or Text alternative. The
   experiment leaves the final narrow-screen tree behavior open for human review.

## Boundaries

This standalone process serves only invented data, using the real compiled CSS,
local fonts, and shell at the existing Sentence study route. Its shell learner
is fictional; other shell destinations are not wired. It has no database, NLP,
provider calls, persistence, grading, or real auth behavior. Production handlers,
stores, schema, dependencies, and durable product docs are untouched. Do not merge.

Parses, morphology and Book/location labels are manually supplied illustrations,
not evidence of model accuracy. The German `am` expansion is a synthetic stress
fixture, not a claim about the deployed tokenizer. Geometry is disposable design
evidence, not a production renderer or validated grammar ontology. Source context
is absent from this proposed UI, not declared irrelevant to pronoun interpretation.

No formal prototype test suite is added. Creation checks use compilation, repo
lint/tests and browser probes; no merge-readiness or complete accessibility-audit
claim follows. The earlier checks are recorded in the first commit's README.

Second-pass checks: Go lint and `go test ./...` passed. Chromium probes covered
both layouts and all four fixtures at 320 CSS px with JavaScript on and off:
no horizontal page overflow or node-box overlaps, working native selection,
raw-evidence access, Close and complete text alternatives. Keyboard node
selection/Escape dismissal, inspector placement, and selected-word continuity
between variants were checked. Screenshots were inspected. The 34-token fixture
measured 1,944 × 864px top-down and 1,124 × 1,680px side-growing; the desktop
chart had 1,166px available width. Those measurements describe one synthetic
fixture, not a general sentence-length threshold or all-language guarantee.

Whole-project lint again stopped at the unchanged Python S310 warning in
`nlp/scripts/derive_dictionary_index.py`; coverage remained blocked because the
shared Python environment lacks `pytest-cov`. Neither unrelated code nor the
environment was modified to work around these blockers.

Third-pass feedback retained top-down over side-growing, requested bare words
with edge labels, and permitted the staggered/feature-line comparison. That pass
supersedes the second comparison's visible boxes and word numbers. The earlier
side-growing prototype remains captured at commit `4dd8c5da`.

Third-pass browser probes covered A/B/C and all four fixtures at 320 CSS px with
JavaScript on/off: no horizontal page overflow, overlapping word hit areas or
overlapping edge labels; working selection, expandable evidence, Close, complete
text alternatives and shared-span feature selection. Automatic viewport fitting
on resize and Escape dismissal were checked. These checks are geometric and
functional smoke probes, not an accessibility audit or proof of parse readability.
The final long-fixture diagrams measured A **1,590 × 688px**, B/C
**1,095 × 1,468px**, with **1,166px** desktop chart width available. At 320px,
B measured **276px** wide in a **286px** diagram viewport; with JavaScript off
and a manual 270px budget it measured **270px** wide. These are fixture-specific
observations, not a universal fitting contract. Go lint and `go test ./...`
passed; whole-project lint/coverage retain the unrelated blockers above.

Fourth-pass feedback: Justin prefers third-pass B but says it still needs work.
He wants curves throughout, likes the Leipzig-ish morphology companion, and
rejects its links and the pop-over inspector. This explicitly supersedes the
earlier selected-word/inspector preference. He proposed Aretusa-like independently
spaced function annotations and praised its font sizes, weights and colors;
neither the waypoint layout nor a final production direction is yet confirmed.
The third comparison remains captured at commit `d83d36e8`.

Fourth-pass checks: Go lint and `go test ./...` passed. Chromium probes covered
all 32 combinations of four fixtures, two layouts, JavaScript on/off, and 320px/
1280px viewports. No page-horizontal overflow, overlapping words or labels,
clipped text, straight-line path commands, or sampled connector/word-glyph
collisions were found. Static word/morphology presentation, full text alternatives,
shared-span grouping, punctuation filtering and automatic resize fitting passed.
Word contrast on the Greek example was at least 5.65:1 in light and 7.58:1 in
dark appearance. Screenshots were inspected. The floating prototype switcher can
temporarily cover chart content or a control at a given scroll position; scrolling
repositions that content. This prototype chrome is not a product interaction.

The long fixture measured A **1,095 × 1,500px**, B **1,164 × 1,148px**, against
**1,166px** available desktop width. This is one fixture and combines packing
and annotation changes, not a validated general optimization. Connector/connector
crossings and long routed curves remain readability questions for live review.
Whole-project lint/coverage retain the unrelated blockers above.

No ticket is closed until live human feedback settles the remaining questions
and the final direction is explicitly confirmed.

# Sentence study tree prototype — THROWAWAY, NOT FOR MERGE

Question: can a complete, legible dependency tree explain a long sentence without
the excessive width of the first top-down prototype? Compare a bare compact
tree, a width-aware staggered tree, and that same tree with a word-level feature
line before hiding words or folding clauses.

Decision ticket: [Choose a dependency visualization through a sentence study prototype](https://github.com/justin-hayes/mouseion/issues/1986).
Map: [Find the way to pedagogically useful sentence study](https://github.com/justin-hayes/mouseion/issues/1983).

## Run

```sh
make prototype-sentence-study
```

Open <http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=tree&example=de-long>.
The bottom bar compares `tree` (bare compact top-down), `stagger` (width-aware
top-down), and `gloss` (the staggered tree plus a word-level morphology line).
Both arrows cycle through these layouts; arrow shortcuts avoid controls,
the diagram, and the word inspector. `example=de`, `it`, `el`, or `de-long`
selects an invented fixture. The long sentence is now the default.

Select a word **in the tree** to inspect its relation and expandable raw labels
and morphology. Close the inspector using Close; Escape also works with JS.
The Text alternative control replaces the graphic with the full relationship
list. View options exposes diagram dimensions, an optional punctuation filter,
and a manual width budget. With JavaScript, B/C use the diagram viewport's width
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
Bare words are selectable, with roles on incoming edges: no visible boxes or
word numbers in the graphic. Source ordinals remain in accessible names and
the text alternative to distinguish repeated words.

```text
A: original sentence → root at top → contour-packed dependent subtrees
B: original sentence → root at top → width-budgeted, vertically staggered subtrees
C: original sentence → word-level feature line → same tree as B
```

The packing algorithm measures the contour occupied at each depth, then places
the next sibling subtree just beyond that contour. Unlike a column per leaf,
this permits neighboring subtrees to use each other's empty space. None of
the tree layouts claims to keep linear word order.

B/C let sibling subtrees occupy different vertical rows. A shared head reaches
wrapped rows through an empty connector gutter reserved for that head. All heads
stay above their dependents, but dependency depth no longer implies a single
horizontal row. This trades width for height; deep nesting and long words/labels
can still exceed a narrow budget. No all-sentence fit guarantee is claimed.

C uses separately hand-supplied feature labels in `feature_labels.json`, not
an automatic morphology converter. They cover selected evidence, not a complete
analysis. Missing labels are dashes, not claims of absent features. The source
contraction appears once above separately labeled expanded words. The fixtures'
`Span` values are manually authored source strings, not an offset-grouping
algorithm. There are no lexical glosses, morpheme boundaries, or translations.
See the [corrected Leipzig research](https://github.com/justin-hayes/mouseion/blob/849309d3de9c0596f286603ddf49b905556ac441/doc/research/sentence-study-leipzig-glossing.md)
for the distinction and production data limitations. Persisted word morphology
exists but is not currently projected into the shipped Sentence study view.

Nodes are ordinary HTML links positioned over a decorative server-rendered SVG.
Every label is text; color is supplemental. Links remain in source order in
the DOM. Selecting a node exposes a dismissible inspector rather than a separate
under-chart word strip. With JS, selection preserves diagram/page scrolling and
positions the inspector beside the selected node where space permits. Without
JS, native node/evidence/Close links and the text alternative still work; there
is no promise of retaining the diagram's scroll offset across full-page requests.

## What to judge

1. On the long sentence, compare A/B. Does the narrower staggered tree remain
   easy to follow, or do long shared connectors and page height defeat it?
2. Compare B/C. Is word-aligned morphology useful beside the tree, or does it
   compete with the source sentence and structure? This does not yet settle
   a full morphemic-gloss alternative.
3. Select a node and inspect morphology without losing your place. Raw versus
   effective lemma evidence must remain distinct in a real implementation.
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
with edge labels, and permitted the staggered/feature-line comparison. This pass
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

No ticket is closed until live human feedback settles the remaining questions
and the final direction is explicitly confirmed.

# Sentence study tree prototype — THROWAWAY, NOT FOR MERGE

Question: can a complete, legible dependency tree explain a long sentence without
the excessive width of the first top-down prototype? Compare compact top-down
and side-growing layouts before hiding words or folding clauses.

Decision ticket: [Choose a dependency visualization through a sentence study prototype](https://github.com/justin-hayes/mouseion/issues/1986).
Map: [Find the way to pedagogically useful sentence study](https://github.com/justin-hayes/mouseion/issues/1983).

## Run

```sh
make prototype-sentence-study
```

Open <http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=tree&example=de-long>.
The bottom bar compares `tree` (compact top-down) and `side` (side-growing).
Both arrows switch between these two layouts; arrow shortcuts avoid controls,
the diagram, and the word inspector. `example=de`, `it`, `el`, or `de-long`
selects an invented fixture. The long sentence is now the default.

Select a word **in the tree** to inspect its relation and expandable raw labels
and morphology. Close the inspector using Close; Escape also works with JS.
The Text alternative control replaces the graphic with the full relationship
list. View options exposes diagram dimensions and an optional punctuation filter.
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
Node borders mark actual selectable graph objects, not a generic card layout.

```text
A: original sentence → root at top → tightly packed dependent subtrees
B: original sentence → head at left → dependent subtrees extend to the right
```

The packing algorithm measures the contour occupied at each depth, then places
the next sibling subtree just beyond that contour. Unlike a column per leaf,
this permits neighboring subtrees to use each other's empty space. Neither
layout claims to keep linear word order; numbers preserve source positions.

Side-growing uses the same packing principle on the vertical axis. Its width
depends primarily on dependency depth and word/label widths, not leaf count.
It trades width for ordinary page-height/vertical scrolling. It can still be
too wide for a deep parse or a narrow viewport; this experiment does **not**
claim all sentences now fit all screens.

Nodes are ordinary HTML links positioned over a decorative server-rendered SVG.
Every label is text; color is supplemental. Links remain in source order in
the DOM. Selecting a node exposes a dismissible inspector rather than a separate
under-chart word strip. With JS, selection preserves diagram/page scrolling and
positions the inspector beside the selected node where space permits. Without
JS, native node/evidence/Close links and the text alternative still work; there
is no promise of retaining the diagram's scroll offset across full-page requests.

## What to judge

1. On the long sentence, compare the whole hierarchy in both layouts. Does the
   narrower side-growing tree remain easy to follow across vertical scrolling?
2. Compare shorter German/Italian examples. Is one orientation better universally,
   or is an explicit orientation control useful?
3. Select a node and inspect morphology without losing your place. Raw versus
   effective lemma evidence must remain distinct in a real implementation.
4. Compare Greek source `στην` with the two separate illustrated nodes `σε` and
   `την`, both labeled with the shared source span. Source text is not reconstructed
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

No ticket is closed until live human feedback settles the remaining questions
and the final direction is explicitly confirmed.

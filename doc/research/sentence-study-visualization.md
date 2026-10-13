# Sentence study dependency visualization research

**Research date:** 2026-10-13 (all external sources accessed 2026-10-13)
**Ticket:** [#1985](https://github.com/justin-hayes/mouseion/issues/1985), under map [#1983](https://github.com/justin-hayes/mouseion/issues/1983)
**Base commit:** `e6f59628` (`main`)
**Status:** Throwaway research asset. Not for merge. No prototype, library, or design decision is made here.

## Question

What can Mouseion learn from Arethusa, and what are the practical trade-offs of
an owned SVG branching tree, word-order-preserving arcs, and a selected-word
relationship view for the Stanza Universal Dependencies data that already
exists? The answer should cover annotation conventions, UD structure,
SVG and accessibility constraints, long or non-projective sentences,
punctuation, multiword tokens, keyboard and non-visual access, narrow screens,
and server-rendered feasibility.

Languages in scope are German (`de`), Italian (`it`), and Modern Greek (`el`).
Ancient Greek is out of scope.

## Summary

Verified facts are labelled **[V]**. Recommendations are labelled **[R]** and
are inputs to a later human-feedback prototype, not decisions.

- **[V]** The data already persisted is sufficient for all three candidate
  views. Each token has `dependency` and a 0-based `head` ordinal, with the root
  self-headed (ADR 0060; `doc/features/dependency-parse-foundation.md`). The
  current Study page already shows relation and head text per token.
- **[V]** Persisted arcs are *basic* dependencies only. UD says the basic
  representation is a tree, while enhanced dependencies are a graph and
  optional. Mouseion does not store enhanced arcs (ADR 0060).
- **[V]** The repo's design rules require server-rendered, no-JavaScript
  coherence, no horizontal page scrolling, and a text alternative for any
  graphic (`doc/design/design-system.md`).
- **[R]** Start with a **focused relationship view** as a server-rendered,
  no-JavaScript text list. It is the cheapest option, needs no new layout
  logic, and is the accessibility baseline for any visual.
- **[R]** Treat **word-order arcs** as the first visual candidate. They keep
  reading order, which matches the learner's text. Keep them owned and
  server-rendered, bounded to a sentence-length limit, and put them in a
  labelled scroll region on narrow screens.
- **[R]** Defer a **top-down branching tree**. It moves words away from reading
  order, and it gets wider and harder to read with long sentences and
  non-projective structure.
- **[R]** Hide punctuation arcs by default, with a control to show them.
- **[R]** Make the Greek shared-span multiword token a visible, deliberate case
  in any visual, not an incidental overlap.

## 1. Local baseline

- **[V]** Sentence study is an existing server-rendered page: `VocabularySentenceStudyPageView`
  in `internal/webapp/concordance.templ:122-160`. It shows the full sentence,
  the identified target, and an ordered list of tokens. Each item shows surface,
  UPOS, raw and effective lemma, `Dependency`, and head surface with 1-based head
  ordinal. Missing syntax shows an explicit message with no visual.
- **[V]** The domain type `SentenceStudyToken` (`internal/domain/domain.go:456-462`)
  carries `Surface, RawLemma, EffectiveLemma, UPOS, Dependency, HeadSurface`,
  `Ordinal`, `HeadOrdinal`, `Corrected`, and `Excluded`. It does **not** carry
  morphological features. The map says morphology is persisted but not selected
  for this page, so showing it would be a domain query change, not a schema
  change.
- **[V]** The repository has no existing `<svg>` in `internal/webapp/*.templ`.
  Concordance and Study are plain HTML and HTMX. There is no client-side graph
  library in use.
- **[V]** The fixture at `internal/fixtures/vocabulary.go:261-266` contains the
  German sentence *Das Haus sieht gut aus .* with `det`, `obj`, `root`, `advmod`,
  `compound:prt`, and `punct` relations. It is a useful small synthetic test case.
- **[V]** Greek MWT is handled in the producer. Stanza word ids are mapped to
  flattened token ordinals, and the expanded words of a contraction such as
  *στο* keep the same source span while both point to the governing noun
  (`doc/features/dependency-parse-foundation.md`, "NLP pipeline").
- **[V]** Current language pipelines are `tokenize,mwt,pos,lemma,depparse` for
  Greek and `tokenize,pos,lemma,depparse` for German and Italian
  (`doc/linguistics.md`, table in section 2).
- **[V]** Morphological features are persisted in the corpus and linked to
  the vocabulary (`doc/linguistics.md`, section 2). They are outside the
  current Study token type.
- **[V]** Terms are fixed by `CONTEXT.md`: *Dependency relation*, *Head*,
  *Dependent*, and *Concordance*. The map asks to use the existing "Sentence
  study" term, which the page title already does.

## 2. Arethusa: inspiration and annotation differences

**Verified facts**

- **[V]** The README's title is "Arethusa: Annotation Environment". It links a
  hosted instance run by the Perseids project, points to the wiki for design
  documentation, and says code documentation is "in progress"
  (<https://github.com/perseids-project/arethusa>, README; accessed 2026-10-13).
  It does not state a licence or the annotation schemes it supports.
- **[V]** The Arethusa wiki's visible pages are a small set (Home, Concept,
  Configuration Design Notes, Morphology Handling, Reference Plugin Design
  Discussion, Examples, and Getting Started). The Home page was last edited
  in May 2016. Several subpages did not load (<https://github.com/alpheios-project/arethusa/wiki>;
  accessed 2026-10-13).
- **[V]** The Arethusa app is a JavaScript single-page application served from
  `perseids.org/tools/arethusa/app/`. The map's inspiration link points to a
  specific document (`doc=27701`) in that app.
- **[U]** Which annotation schemes Arethusa supports is not stated in the README.
  Do not assume it is UD-only or multi-scheme without checking the wiki or app.

**Not verified**

- **[U]** I did not verify the exact colour scheme, arc direction, or label
  set in the screenshot named in the map. The screenshot was not available as
  a file I could read, and the Arethusa wiki does not document its dependency
  tree rendering. Any statement about "Arethusa's colours" would be
  unsupported. The human owner should attach or describe the screenshot before
  the visual vocabulary is chosen.

**Differences to keep in mind**

- **[V]** The README describes Arethusa as an *annotation environment*, which is
  an annotation tool.
  Mouseion is a *reader* of analyzer output. Mouseion does not edit
  dependencies here (`doc/features/dependency-parse-foundation.md`, non-goals;
  `doc/features/lemma-review-and-correction.md` covers lemma correction only).
- **[R]** Borrow the *idea* of reading a sentence's syntax against its word
  order, and of focusing one token's relations. Do not copy an Arethusa label
  set or colour scheme. The labels should be UD relations plus the existing
  learner-facing glosses, and colour should never be the only signal (section 5).

## 3. Universal Dependencies facts that constrain the view

- **[V]** The basic representation is a **tree**: exactly one word is the head of the
  sentence, attached to a notional ROOT, and all others have exactly one head
  (<https://universaldependencies.org/u/overview/syntax.html>; accessed 2026-10-13).
  The CoNLL-U format says the same for HEAD/DEPREL
  (<https://universaldependencies.org/format.html>; accessed 2026-10-13).
- **[V]** The enhanced representation is "in general not a tree but a general
  graph structure". It is stored in a separate DEPS column, and it is optional
  in treebanks. Shared dependencies propagate to conjuncts
  (<https://universaldependencies.org/u/overview/enhanced-syntax.html>; accessed 2026-10-13;
  <https://universaldependencies.org/format.html>, DEPS).
  **Implication:** a visual built on enhanced arcs would show relations that
  the basic tree does not contain, and the two sets can disagree. Mouseion
  currently persists only basic arcs. Keep that boundary explicit in any view.
- **[V]** Function words are leaves in general. Content words are internal nodes,
  and auxiliaries and determiners attach to content words, not to each other
  (`syntax.html`, "The Status of Function Words"). This matters for the
  branching tree: a function word will usually be a leaf with no arc of its own
  beneath it.
- **[V]** Punctuation attaches to the head of its unit, and "a punctuation mark
  is attached at the highest possible node that preserves projectivity"
  (<https://universaldependencies.org/u/dep/punct.html>; accessed 2026-10-13).
  Paired punctuation attaches to the same word where possible. Punctuation is
  therefore part of the tree, not decoration.
- **[V]** Multiword tokens are given as range IDs (for example `3-4`) with
  their own surface FORM. Each syntactic word in the range has its own ID, lemma,
  UPOS, and HEAD. The range line itself has `_` for those fields
  (<https://universaldependencies.org/format.html>, "Words, Tokens and Empty
  Nodes"; <https://universaldependencies.org/u/overview/tokenization.html>, accessed 2026-10-13).
  The existing persistence stores expanded words, not range lines, which is
  consistent with this.
- **[V]** In CoNLL-U, a range line's FORM is the string that occurs in the
  sentence, and each component word has its own FORM, lemma, and HEAD line.
  Ranges must not overlap. The format has no field that aligns component FORMs
  to the range FORM; the example pairs them only by order. The map names *στο*
  and *στην* as the concrete Greek case.
- **[V]** The UD relation set is versioned. The `punct` and `root` relations
  are part of the basic 37-relation core. Subtypes such as `compound:prt` and
  `nsubj:pass` are language-specific or documented extensions
  (`format.html`, `enhanced-syntax.html`).

**Non-projectivity**

- **[V]** The punctuation guideline above attaches punctuation at the highest
  node that "preserves projectivity". The UD syntax overview does not discuss
  projectivity. Neither fetched UD page states whether non-projective trees are
  permitted.
- **[U]** I did not find a primary UD statement on how often non-projective
  arcs occur in German, Italian, or Modern Greek. That needs measurement on a
  public UD treebank, which is out of scope for this ticket (no private book
  text, no paid APIs).

## 4. Prior art for server-rendered arcs: displaCy

- **[V]** spaCy's `displacy` renders dependency parses as SVG in Python. The
  `DependencyRenderer` lays words out at a fixed `distance` (default 175 px,
  or 150 px in compact mode) and assigns each arc a vertical "level" by its
  length, so short arcs sit low and long arcs stack higher. Arc paths are cubic
  Bézier curves with arrowheads, and labels sit on the arc
  (<https://raw.githubusercontent.com/explosion/spaCy/master/spacy/displacy/render.py>; accessed 2026-10-13; the
  pinned commit is not recorded, so this is the master branch on that date).
- **[V]** The SVG width is `offset_x + len(words) * distance`. A 40-word
  sentence is therefore about 7,000 px wide by default. spaCy documents
  "visualizing long texts" by rendering one sentence at a time, and it documents
  a known label-wrapping and overflow issue in compact mode
  (<https://spacy.io/usage/visualizers>; accessed 2026-10-13).
- **[V]** spaCy's own docs recommend against server-side HTML for web apps
  because of XSS risk, and suggest client-side rendering from JSON
  (`visualizers`, "Using displaCy in a web application"). displaCy's Python
  renderer escapes token text with `escape_html` (`render_word`, `render.py`), but
  it passes the tag, lemma, and arc label into its templates without that call.
  Mouseion should not copy displaCy's escaping. Templ escapes interpolated strings
  by default (<https://templ.guide/syntax-and-usage/expressions/>; accessed
  2026-10-13), and `concordance.templ` interpolates user text with `{ ... }`
  (for example `value={ lookup.Term }`).
- **[V]** The spaCy repository is MIT licensed (`LICENSE`, `master`; accessed
  2026-10-13). MIT requires the copyright notice to travel with copied code. The
  level assignment (`get_levels`) and arc path (`get_arc`) are plain functions in
  `render.py`, and the renderer is not a dependency Mouseion would need to add.
- **[U]** The size of an owned port is not estimated here. A size estimate needs
  a prototype.
- **[R]** Use displaCy as a *reference for geometry*, not as a dependency. The
  choice of library is out of scope for this ticket. Any owned renderer should
  stay small enough to test directly.

## 5. Accessibility: SVG, keyboard, and non-visual access

**Verified facts**

- **[V]** The WAI-ARIA Graphics Module defines `graphics-document` for the root
  of a graphic whose layout conveys meaning, and `graphics-object` for distinct
  sub-components
  (<https://www.w3.org/TR/graphics-aria-1.0/>; accessed 2026-10-13).
- **[V]** `graphics-symbol` is for a simple graphical meaning and makes its
  children presentational. A symbol is atomic (graphics-aria). A dependency arc
  is not a symbol; it is a relation between two words.
- **[V]** SVG-AAM describes how SVG `<title>` and `<desc>` are exposed, and
  how `aria-labelledby` and `aria-describedby` apply to SVG elements. An SVG
  element that is focusable or has an accessible name gets a role mapping
  (<https://www.w3.org/TR/svg-aam-1.0/>; accessed 2026-10-13). The SVG-AAM text
  notes that its version is a working draft dated 2026-10-12; this is the
  version fetched.
- **[V]** WCAG 2.2 SC 1.4.10 (Reflow) requires text to reflow at 320 CSS px
  without two-dimensional scrolling. Exceptions include "images required for
  understanding (such as maps and diagrams)", and data tables, which may scroll
  in two dimensions inside their own container. The exception applies only to
  that section, and the rest of the page must still reflow
  (<https://www.w3.org/WAI/WCAG22/Understanding/reflow.html>; accessed 2026-10-13).
- **[V]** WCAG 2.2 SC 2.1.1 (Keyboard) requires all functionality to be operable
  from a keyboard. A non-interactive graphic needs no keyboard operation. Any
  interactive node or arc must be keyboard-reachable
  (<https://www.w3.org/WAI/WCAG22/Understanding/keyboard.html>; accessed 2026-10-13).
- **[V]** Design rules in this repo: native landmarks and lists first; a coherent
  server-rendered state before HTMX or custom JavaScript; "Horizontal page
  scrolling is not an accepted fallback; a deliberately scrollable data table
  must label and contain its own overflow"; colour and position are supplemental,
  and labels are text (`doc/design/design-system.md`, "Accessibility
  foundation" and "Responsive"; accessed locally at `e6f59628`).

**Recommendations**

- **[R]** The text list is the accessibility baseline. Every visual must have a
  text equivalent that gives the same information: per token, its relation,
  head surface, and head token number. The existing Study page already does
  this, so a visual can be an enhancement beside the list, not a replacement.
- **[R]** Give any SVG a `<title>` and `<desc>` that state what the diagram
  shows and point to the list, and reference them via `aria-labelledby` and
  `aria-describedby`. Mark the graphic as `role="img"` or
  `graphics-document` as the implementation supports. Do not put focus stops on
  arcs or nodes unless they are actionable.
- **[R]** If a node or arc opens a relation focus, implement it as a native
  link or button that changes the URL or fragment. This is the no-JavaScript
  path, and it satisfies SC 2.1.1 with no custom key handling.
- **[R]** Use relation labels as text, not colour alone. Position and colour are
  supplemental, as the design system says.

## 6. Options compared

| | Focused relationship view | Word-order arcs | Top-down branching tree |
| --- | --- | --- | --- |
| Reading order | Preserved (text list) | Preserved (words stay in sentence order) | Not preserved (root at top) |
| Server-rendered, no JS | Yes, trivially | Yes, as static SVG | Yes, as static SVG |
| Owned, small code | Yes, no geometry | Yes, about the displaCy geometry | Yes, but needs tree layout |
| Non-projective sentences | Fine; relation list is order-free | Arcs cross; readable with focus mode | Crossings are more numerous |
| Long sentences (40+ words) | Scales as a list | Needs a bounded, scrollable region or sentence-length cutoff | Gets deep and wide |
| Narrow screens (320 px) | Reflows | Needs contained horizontal scroll or a stacked fallback | Needs contained scroll; harder to stack |
| Keyboard/no-JS | Native links | Native links on tokens | Native links on nodes |
| Explains the relation | Yes, with a label | Yes, with a label on each arc | Yes, with a label on each edge |
| Learner value for "how it works" | Strong for head/dependent questions | Strong for word order and clause shape | Strong for hierarchy, weak for word order |

**Recommendations for the prototype brief [R]**

1. Keep the existing text list as the always-present baseline.
2. Add a **focused relationship view** beside each token: relation, head, and
   dependents, as links and text. This needs no geometry.
3. Prototype **word-order arcs** as a second layer, with a per-token focus mode
   that dims all arcs except the chosen token's.
4. Do **not** prototype the branching tree first. Revisit it only if learners
   say word order is the wrong mental model for the target sentences.

## 7. Long and non-projective sentences

- **[V]** The displaCy layout width is linear in token count (section 4).
  Word-order arcs therefore cannot use a fixed-width page at 40+ tokens.
- **[V]** Arc crossings are unavoidable in word-order layouts when a sentence is
  non-projective. The fetched UD pages do not state whether non-projective trees
  are permitted or how common they are.
- **[R]** Set a token-count threshold above which the visual is replaced by the
  text list plus focused relations, and say so in the UI. The threshold should
  be chosen with human review on real sentences, not on book text uploaded
  outside the repo.
- **[R]** Render long visuals only inside a labelled scroll region with its own
  focusable container, as WCAG 1.4.10 and the design system allow. Do not let
  the page scroll horizontally.

## 8. Punctuation

- **[V]** UD attaches punctuation at the highest node that preserves
  projectivity. Its `punct` arcs are part of the basic tree, so they are present
  in the stored data (`punct.html`; `fixtures/vocabulary.go:266`).
- **[V]** Persisted tokens include punctuation as tokens. The current Study list
  shows them as tokens with `punct`. Whether `SpaceAfter=No` (the MISC feature in
  UD) is persisted is not verified in this ticket.
- **[R]** In word-order arcs, hide `punct` arcs by default and offer a toggle.
  The text list keeps them. This reduces clutter without losing data.
- **[R]** Keep punctuation tokens clickable in the list. They are still part
  of the tree and can head nothing, since UD does not give punctuation
  dependents.

## 9. Shared-span multiword tokens (Greek)

- **[V]** Stanza's Greek pipeline uses MWT expansion (`doc/linguistics.md`,
  section 2; <https://stanfordnlp.github.io/stanza/tokenize.html> and
  <https://stanfordnlp.github.io/stanza/depparse.html>, both accessed 2026-10-13).
  `depparse` requires `tokenize`, `mwt`, `pos`, and `lemma` first.
- **[V]** In Mouseion, the expanded preposition and article share one source
  span and point to the governing noun (`dependency-parse-foundation.md`).
  In a word-order view, two nodes would sit over one surface span.
- **[R]** Render the expansion as a group: one surface label spanning both
  words, each word's own relation below it. The text list should say explicitly
  that "*στο* is two words here".
- **[R]** Do not collapse the expansion into one node. Its two words have
  separate heads and relations in the UD data.

## 10. Server-rendered feasibility

- **[V]** The app is Go + templ with HTMX (`doc/design/design-system.md`,
  "Dependency policy" and "Accessibility foundation"). The Study page is a templ
  view (`concordance.templ`). No CSP header was found in Go source under
  `internal` or `cmd` (`grep -i Content-Security-Policy`, accessed locally at
  `e6f59628`).
- **[V]** The displaCy geometry is arithmetic over token positions and arc spans
  (`DependencyRenderer` in `render.py`). Nothing in the existing stack requires
  client-side JavaScript.
- **[R]** A server-rendered SVG works with no JavaScript, is cacheable, and
  keeps all relation text in the HTML. Hover-only labels would fail the design
  rules, so labels should always be visible or expanded by a native disclosure.
- **[R]** Any interactive focus mode should use links that reload the Study
  page with a fragment or query parameter, which matches the existing
  return-navigation rules. A client-side focus mode is an enhancement, not the
  first version.

## 11. Limitations

- I did not see the Arethusa screenshot named in the map. Arethusa claims in
  section 2 are limited to documented facts about the project, and the visual
  scheme is unverified.
- The Perseids app is a JavaScript single-page application. I did not inspect
  its rendered tree, because doing so would require running the app.
- The Arethusa wiki is mostly empty and last changed in 2016.
- The displaCy source and docs were read from `master` on the access date, not
  from a pinned commit. Verify before copying any geometry.
- SVG-AAM is a working draft (2026-10-12). Its normative status should be
  rechecked before the accessibility notes are relied on.
- No Mouseion sentences from private books were used. The fixture is
  synthetic. Non-projectivity and punctuation rates on real Mouseion sentences
  are unknown.
- No prototype was built, and nothing was measured on real analyzer output.
- I did not evaluate Stanza or Greek model accuracy. That is covered by
  `doc/research/modern-greek-nlp.md`.

## 12. Follow-up unknowns for human review

1. What does the Arethusa screenshot actually show (colours, arc direction,
   labels), and which of those are learner-visible in Mouseion's terms?
2. What is the sentence-length threshold above which the visual is replaced by
   the text list?
3. How often do real German, Italian, and Modern Greek sentences in a public UD
   treebank contain non-projective arcs? This needs a measurement run on a
   public corpus.
4. Is `SpaceAfter=No` persisted per token? If not, the punctuation display
   cannot reconstruct spacing reliably.
5. Should morphology appear in the focused view? That needs a domain change to
   `SentenceStudyToken`, which is not a schema change.
6. Does the Greek shared-span group need a visual label beyond the text list?

## Sources (accessed 2026-10-13)

- Universal Dependencies, syntax overview: <https://universaldependencies.org/u/overview/syntax.html>
- Universal Dependencies, enhanced dependencies: <https://universaldependencies.org/u/overview/enhanced-syntax.html>
- Universal Dependencies, tokenization and word segmentation: <https://universaldependencies.org/u/overview/tokenization.html>
- Universal Dependencies, `punct`: <https://universaldependencies.org/u/dep/punct.html>
- Universal Dependencies, CoNLL-U format: <https://universaldependencies.org/format.html>
- Stanza, dependency parsing: <https://stanfordnlp.github.io/stanza/depparse.html>
- Stanza, tokenization: <https://stanfordnlp.github.io/stanza/tokenize.html>
- spaCy, displaCy source (`master`): <https://raw.githubusercontent.com/explosion/spaCy/master/spacy/displacy/render.py>
- spaCy, visualizers: <https://spacy.io/usage/visualizers>
- Arethusa, README and repository: <https://github.com/perseids-project/arethusa>
- Arethusa wiki (upstream fork source): <https://github.com/alpheios-project/arethusa/wiki>
- WAI-ARIA Graphics Module 1.0: <https://www.w3.org/TR/graphics-aria-1.0/>
- SVG Accessibility API Mappings (working draft, 2026-10-12): <https://www.w3.org/TR/svg-aam-1.0/>
- WCAG 2.2 Understanding SC 1.4.10 Reflow: <https://www.w3.org/WAI/WCAG22/Understanding/reflow.html>
- WCAG 2.2 Understanding SC 2.1.1 Keyboard: <https://www.w3.org/WAI/WCAG22/Understanding/keyboard.html>

Local sources, at base commit `e6f59628`: `doc/features/dependency-parse-foundation.md`,
`doc/adr/0060-persist-dependency-parses.md`, `doc/linguistics.md`,
`doc/research/modern-greek-nlp.md`, `doc/design/design-system.md`,
`doc/features/vocabulary-browse-and-concordance.md`, `CONTEXT.md`,
`internal/webapp/concordance.templ`, `internal/domain/domain.go`,
`internal/fixtures/vocabulary.go`.

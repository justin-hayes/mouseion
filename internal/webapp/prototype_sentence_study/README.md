# Sentence study visualization prototype — THROWAWAY, NOT FOR MERGE

Question: which presentation best supports noticing the structure of a sentence
from reading: a branching tree, word-order arcs, or selected-word relationships?

Decision ticket: [Choose a dependency visualization through a sentence study prototype](https://github.com/justin-hayes/mouseion/issues/1986).
Map: [Find the way to pedagogically useful sentence study](https://github.com/justin-hayes/mouseion/issues/1983).

Run from this branch's repository root:

```sh
make prototype-sentence-study
```

Open <http://127.0.0.1:8106/vocabulary/concordance/sentence?variant=tree>.
Variants are `tree`, `arcs`, and `focus`. The bottom bar cycles them; left/right
keys do so outside controls and the scrollable graphic. `example=de`, `it`, `el`,
or `de-long` selects the invented example. `support=1` opens analysis immediately.

This standalone process uses the real Mouseion shell, compiled CSS, and local
fonts, on the existing Sentence study route. The shell learner is fictional;
other shell destinations are deliberately not wired. No production handlers,
stores, database, NLP calls, schema, or auth contracts change. This command is
not referenced by the production server/image. Do not merge this branch.

## Design plan

- Reuse the accepted Margin notes identity: porcelain `#f3f6f7`, raised leaf
  `#fcfdfb`, quiet blue-grey `#e3ebf0`, carbon `#17232d`, pencil `#586873`,
  annotation blue `#2457b2`; all CSS uses the existing semantic tokens.
- Commissioner for interface text; Literata for complete source and words.
- Keep source Book/location, sentence, nearby context, optional reveal and
  return-to-reading intent fixed. Change the visual relationship model only:

```text
A: source → reveal → root / branching hierarchy → words + relationship margin
B: source → reveal → arcs / linear word sequence → words + relationship margin
C: source → reveal → flowing word links | selected head / dependents in margin
```

- Left-align content; retain a readable source measure. Spend visual emphasis
  on the sentence and its selected relation, not dashboards or grammar colors.
- Reviewed against the brief: vary hierarchy and reading-order model, not
  palette. No new general-purpose UI component, client library or persistence.

## What to try

1. Before revealing, read the source and optionally open nearby context.
2. Reveal, then compare all three variants on the same sentence. Does the
   separable particle's relation help explain `steht … auf`?
3. Select `gli` in Italian; inspect its head and raw morphology. Context, not
   the parse alone, is evidence for the pronoun's referent.
4. Compare `στην` in the original Greek source with the separately selectable
   illustrated syntactic words `σε` and `την`; never duplicate the source.
5. Try the long sentence. Compare diagram scrolling with flowing relationships,
   and selected-edge emphasis with the full set of connections.
6. At 320px, with keyboard only, and with JavaScript disabled, try Reveal,
   selecting a word, and the all-token text alternative.

## Evidence limits / unresolved decisions

All sentences, bibliographic labels, and parses are invented. The German `am`
split is a hand-supplied stress fixture, not a claim about the deployed German
tokenizer. There is no real model accuracy evidence, no linguistic correction
workflow, and no generated translation or explanation. The relation glosses
are cautious display labels, not a grammar ontology or semantic-role inference.

Both diagrams render on the server, with their full text alternative. Geometry
is deliberately simple: ordered leaves for the tree, greedy interval lanes for
arcs. Their actual pixel costs are exposed in Prototype state. This demonstrates
layout pressure, not a production layout limit or a finalized algorithm.

The focused view can be used without either diagram. No diagram cutoff or
automatic narrow-screen switch is chosen yet. The full token list is available
under a native disclosure, not deleted. Human feedback must decide which view
leads, whether graphics complement the list, evidence depth, punctuation defaults,
shared-span presentation, and accessible/narrow-screen behavior.

No decision is resolved until live human feedback and explicit confirmation.
Capture the verdict in the decision ticket, not by promoting this code.

## Checks during creation

- Go lint and `go test ./...` passed, including compilation of this command.
- Chromium probes covered all three variants and four examples at 320 CSS px,
  with JavaScript on and off: no horizontal page overflow; word selection and
  raw-evidence disclosures worked. Arrow shortcuts do not hijack diagram scrolling.
- Desktop and narrow-screen screenshots were inspected. This is not a complete
  screen-reader or production accessibility audit.
- Whole-project `make lint` stopped at an unchanged Python `S310` warning in
  `nlp/scripts/derive_dictionary_index.py`; no unrelated source was changed.
- `make coverage-new` could not run because the existing shared Python environment
  lacks the `pytest-cov` plugin. No coverage or merge-readiness claim is made.
- No formal prototype test suite was added; this is disposable decision evidence,
  not an implementation handoff or production patch.

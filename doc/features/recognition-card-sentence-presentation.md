# Recognition-card sentence presentation

## Product contract

Mouseion generates **recognition cards**. The complete selected German source
sentence appears on the front, and the word under test is bolded. The word is
not hidden behind an Anki cloze deletion.

Long sentences are handled by deterministic quality policy and presentation:

- sentence selection prefers manageable examples;
- the export gate rejects examples that are too short, fragmented, structurally
  noisy, or beyond the configured maximum length;
- an accepted sentence is never truncated, subset-selected, or paraphrased;
- Anki styling presents the complete sentence in a readable, left-aligned,
  constrained-width block.

The back repeats the recognition front as appropriate and supplies the answer
information:

- the headword line: article, lemma, noun plural, pronunciation, principal
  parts, and part of speech;
- one meaning block — the dictionary gloss;
- the complete English contextual translation;
- source/book metadata where useful.

The back does **not** show a second `SourceSentence` block. The card contract
has one learner-facing German source sentence, not two copies of it.

The generated note fields are `Identity`, `Text`, `Article`, `Lemma`, `Plural`,
`IPA`, `PrincipalParts`, `POS`, `Gloss`, `English`, `EnglishSentence`, and
`BookTitle`, in that order, followed by TSV tags. `Identity` is the Anki
sort/deduplication field. `Text` is the complete source sentence with
Mouseion-owned `<b>` markup around the target; `SourceSentence` is not a field
in either export. `English` is retained in the contract but is not rendered: the
card's meaning is the gloss.

## Target form and bolding

The bolded target is the full vocabulary identity, not only the finite surface.
For a separable verb whose particle detaches (`rief … entgegen`, lemma
`entgegenrufen`), every component is bolded: the observed verb form and each
`compound:prt` particle whose head is that verb token, resolved from the
persisted dependency parse. Attached forms are unchanged; when no parse is
available, only the observed form is bolded.

## Morphology presentation

The noun morphology renders in the headword line as
`Article Lemma (Pl. Plural) · POS`:

- `Article` is the noun's nominative definite article (`der`/`die`/`das`;
  `il`/`lo`/`la`/`l'`), or empty when the dictionary gives no unambiguous
  article. A derived value that is not a genuine article is ignored and the
  gender fallback applies.
- `Plural` is the noun's dictionary plural, rendered beside the singular it
  inflects whenever the index supplies one — including when it equals the lemma
  (`der Gauner (Pl. Gauner)`). It is empty for non-nouns and plural-only lemmas.
- The plural is not part of `Gloss`.

See [ADR 0067](../adr/0067-recognition-card-morphology-presentation.md).

## Pronunciation and principal parts

The headword line carries two further dictionary-derived forms, each rendered
only when the index supplies it:

- `IPA` is the entry's pronunciation, normalized in the derivation step toward a
  standard phonemic `/…/` form (optional segments and regional variants
  discarded). It self-suppresses when the index has no usable pronunciation.
- `PrincipalParts` shows a verb's inflectional forms beside the infinitive
  headword: third-person singular present, preterite, and past participle
  (`geht · ging · gegangen`). It is German-first; the extraction is
  language-agnostic so Italian can follow as a data-only change. It
  self-suppresses when no forms are available. Auxiliary and mood are out of
  scope.

Both fields are frozen onto the prepared-deck manifest like gloss and
morphology. Coverage is partial, so an absent value must leave a valid card
unchanged.

## Meaning block

The back has one meaning block, sourced from the dictionary `Gloss`: a compact,
context-ordered sense set. The separate `English` lemma translation is retained
in the note contract but no longer rendered; the contextual `EnglishSentence`
translation remains distinct. See
[ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).

When external translation is consented, the LLM may reselect and reorder the
dictionary senses, or supply a fallback gloss when no dictionary sense fits. See
[llm-sense-selection.md](llm-sense-selection.md).

## English target highlighting

The contextual English translation may bold the English word or phrase that
corresponds to the German target. This is best-effort, not a required property
of every card. The enrichment provider should return the complete translation
plus a plain-text target phrase or structured alignment. Mouseion validates the
phrase against the translation, escapes the text, and adds the HTML emphasis
itself. The canonical provider response field is
`sentence_translation_target`.

If the phrase is absent, ambiguous, idiomatic, or otherwise cannot be validated,
the complete English translation is shown without highlighting.

Provider-generated HTML is not trusted.

## Implementation slices

1. Remove `context_sentence` generation, validation, caching, and front-side
   selection. Preserve the complete source sentence and complete English
   translation.
2. Change the Anki model/templates from cloze presentation to recognition
   presentation with deterministic German target bolding.
3. Remove the duplicate `SourceSentence` field/block from the generated Anki
   and TSV contract, updating compatibility fixtures and completeness tests.
4. Add validated English target-phrase/alignment handling and deterministic
   bold rendering with an unhighlighted fallback.

See ADR 0029 for the accepted decision and the GitHub issues for delivery
scope and verification.

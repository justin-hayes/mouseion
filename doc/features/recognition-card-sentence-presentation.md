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

- lemma and morphology/POS;
- concise English lemma translation;
- complete English contextual translation;
- source/book metadata where useful.

The back does **not** show a second `SourceSentence` block. The card contract
has one learner-facing German source sentence, not two copies of it.

The generated note fields are `Text`, `Lemma`, `POS`, `Morph`, `English`,
`EnglishSentence`, and `BookTitle`, in that order, followed by TSV tags. `Text`
is the complete source sentence with Mouseion-owned `<b>` markup around the
target form; `SourceSentence` is not a field in either export.

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

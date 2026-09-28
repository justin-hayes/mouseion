# Recognition-card sentence presentation

## Product contract

Mouseion generates **recognition cards**. The complete selected source sentence
(German, Italian, or Modern Greek) appears on the front, and the word under test
is bolded. The word is not hidden behind an Anki cloze deletion.

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
`compound:prt` particle whose head is that verb token, resolved at render from
the dependency parse. The parse is a frozen render input carried on the
prepared-deck manifest (`render_payload`), so a durable run resolves the same
spans when it finalizes as it did when it was frozen. Attached forms are
unchanged; when no parse is available, only the observed form is bolded.

## Morphology presentation

The noun morphology renders in the headword line as
`Article Lemma (Pl. Plural) · POS`:

- `Article` is the noun's nominative definite article (`der`/`die`/`das`;
  `il`/`lo`/`la`/`l'`; or Greek `ο`/`η`/`το`), or empty when the dictionary gives no unambiguous
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
  (`geht · ging · gegangen`). It remains German-first; Greek principal parts are
  deliberately absent because the Germanic extractor does not apply. It
  self-suppresses when no forms are available. Auxiliary and mood are out of
  scope.

Both fields are frozen onto the prepared-deck manifest like gloss and
morphology. Coverage is partial, so an absent value must leave a valid card
unchanged.

## Meaning block

The back has one meaning block, the contextual `Gloss`: a compact cue for the
tested sense, informed by dictionary evidence and the sentence. The separate
`English` lemma translation is retained in the note contract but no longer
rendered; the contextual `EnglishSentence` translation remains distinct. See
[ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md),
[contextual-gloss-preparation.md](contextual-gloss-preparation.md) and
[ADR 0079](../adr/0079-contextual-glosses-require-llm.md).

## Presentation preview

The frozen Anki template and stylesheet
(`internal/cardexport/templates/recognition_card_back.html` and
`recognition_card.css`) are produced from the static preview at
`internal/cardexport/testdata/recognition_card_preview.html`. The preview uses
the design-system semantic tokens, supports Anki dark mode, keeps the headword
dominant over its inflectional forms, and collapses absent fields cleanly. It
was reviewed and signed off (PR #906) before the exporter changes landed; see
[ADR 0068](../adr/0068-recognition-card-meaning-and-form-presentation.md).

## English target highlighting

The contextual English translation may bold the English word or phrase that
corresponds to the source-language target. This is best-effort, not a required
property of every card. The enrichment provider should return the complete
translation plus a plain-text target phrase or structured alignment. Mouseion
validates the phrase against the translation, escapes the text, and adds the
HTML emphasis itself. The current single-phrase provider response field is
`sentence_translation_target`.

If the phrase is absent, ambiguous, idiomatic, or otherwise cannot be validated,
the complete English translation is shown without highlighting.

Provider-generated HTML is not trusted.

### Planned multi-span English alignment (not yet implemented)

Extend the optional English target alignment to discontinuous correspondences,
such as **knocks** and **over** in “knocks Piero over.” The external translation
provider proposes an ordered set of exact excerpts from its complete English
translation, choosing only the smallest words or phrases that convey the tested
vocabulary. It does not enlarge an uncertain alignment to an entire clause or
sentence. New provider prompts return only a structured list of excerpts: one
excerpt for a contiguous phrase, multiple for discontinuous correspondences.
The legacy single-phrase field remains readable for old cached responses but
is not another answer in a new response. Mouseion validates that each excerpt
has one unambiguous, word-bounded occurrence, that the spans do not overlap,
and that their combined coverage does not include every lexical word of a
multiword sentence. Mouseion owns all escaping and emphasis; if any excerpt is
missing, repeated, or otherwise invalid, the complete translation and gloss
remain available but the English sentence is unhighlighted. Validate before
writing the immutable cache (storing no alignment if invalid) and again at
render as defense in depth. Minimality beyond these checks is an instruction
to the provider, not something Mouseion can prove from the translation alone.

This requires new provider output. An offline re-render of an existing ready
deck cannot produce a new alignment from its immutable cached translation; the
learner must explicitly re-prepare the deck with the configured provider to
obtain the new provider output. Prepared-deck translation follows the existing
required-LLM policy, not a new per-learner consent setting. Re-rendering neither
calls the provider nor infers multi-span alignment from old results.
Re-preparation runs the current translation-and-gloss workflow, so it may also
change the English wording and Gloss; there is no separate alignment-only
provider call to retrofit an old translation. Old decks remain usable and are
not flagged as presentation-stale solely for lacking this new provider data;
no new upgrade prompt is added. The existing single-phrase alignment remains
the current behavior until this extension ships.

The same optional alignment contract applies to prepared decks in every
supported study language when they have an English sentence translation. Before
shipping, evaluate curated contiguous, discontinuous, repeated, idiomatic, and
no-alignment cases across German, Italian, and Modern Greek where examples are
available. Reviewed examples must show no misleading emphasis; missing emphasis
is acceptable when the correspondence is uncertain. Use fixed provider-response
fixtures for repeatable automated tests rather than calling a live provider in
CI, and review representative cases against the configured model before
rollout. Matching validity is not a substitute for assessing whether the
provider chose the right meaning-bearing words.

See [ADR 0080](../adr/0080-llm-proposed-english-target-alignment.md) for the
decision amending ADR 0029's distributed-correspondence fallback.

## Original implementation slices

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

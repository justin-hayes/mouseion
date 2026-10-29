# ADR 0112: The Gloss is the lemma's sense in citation form

Status: **Accepted — implementation pending** · Date: 2026-10-29 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2325](https://github.com/justin-hayes/mouseion/issues/2325) and recorded by
[#2326](https://github.com/justin-hayes/mouseion/issues/2326). Amends the gloss
clause of [ADR 0079](0079-contextual-glosses-require-llm.md) ("one short English
gloss for the target's meaning in its representative sentence"). ADR 0079's
other decisions, and the Ancient Greek evidence-assisted pass of
[ADR 0101](0101-ancient-greek-cards-from-source-evidence.md) and
[ADR 0105](0105-greek-evidence-assisted-pass-always-on.md), are unchanged.

## Context

ADR 0079 defined the Gloss as the target's meaning *in this sentence*. It never
said the Gloss is a dictionary-form equivalent, and it never related the Gloss
to the **lemma translation**: the plain English equivalent of the lemma that the
same provider response returns in its `translation` field, stored in the
hidden `English` note field. It is not the whole-sentence translation. Under prompt
`translation-v15-short-handles`, the model treated the two fields as different
jobs. Of 6,044 contextual Glosses for `de`, `it` and `el` in the operator's
cache, 2,348 do not contain the lemma translation at all.
Three failures recur:

- **Definition paraphrase.** *kalt* → "having a low temperature" instead of
  "cold"; *höflich* → "in a well-mannered way" instead of "polite".
- **Sentence material in a lemma card.** *erschießen* → "shot dead",
  *poggiare* → "rested my wrist", *macchina* → "cars". The Gloss carries the
  sentence's tense, number and arguments.
- **Glossing the wrong word.** Analysis tagged the separable particle in
  *Großmutter sieht im Wohnzimmer fern* as an adverb, so the target became
  *sehen* and the Gloss became "to watch television". An upstream lemma error
  became the card's meaning.

Some contextual Glosses are genuine improvements and must survive: *palazzo* →
"apartment building", *schießen* → "to gush out".

ADR 0079 required quality to be evaluated on representative cards, and the
feature specification asked for a reviewed sample with a rubric. That review
was never performed, so v15 shipped unreviewed.

## Decision

**The Gloss is the English equivalent of the target lemma, in the sense its
representative sentence uses, in citation form.** It follows the conventions of
the lemma translation: verbs as "to …", nouns singular without an article,
adjectives in their base form. The sentence's inflection, number and arguments
belong only in the highlighted English sentence, never in the Gloss.

**The common equivalent is the default.** The Gloss is the plain equivalent
unless the sentence clearly uses a different or narrower sense. A
definition-style paraphrase is never a Gloss when a short equivalent exists.
ADR 0079's brevity rules remain: one to three words, close comma-separated
synonyms only when one word would mislead, and a longer phrase for function
words or idioms where a short cue would mislead.

**The Gloss equals the lemma translation unless the sense differs.** The
provider's `translation` and `gloss` fields now have an explicit relationship: they are the same text when the sentence uses the common sense,
and differ only when it uses another sense. The card's meaning block still
renders only the Gloss; the `English` note field stays off the card back
([ADR 0068](0068-recognition-card-meaning-and-form-presentation.md)).

**A sentence that uses a different lexeme yields a `lemma_mismatch` outcome.**
When the representative sentence evidently uses a different word than the
target lemma (for example, a separable verb whose particle analysis missed, or
an idiom headed elsewhere), the provider returns an unresolved outcome of kind
`lemma_mismatch` instead of glossing the sentence. The response's unresolved
kind is `ambiguous` or `lemma_mismatch`. It is empty when a Gloss is resolved
and required when the Gloss is empty. As ADR 0079 already provides, a resolved
Gloss with a stray unresolved reason is accepted as resolved, and the stray
reason and kind are discarded. An unknown kind is an invalid provider response
and follows the existing retry path
([ADR 0100](0100-retryable-provider-omissions-preparation.md)). A
`lemma_mismatch` card is omitted, like an `ambiguous` one, and the deck result
reports and counts it separately as "the sentence uses a different word than
this lemma". The omission is recorded through the existing omission reason with
a reserved, domain-owned prefix; no schema change is needed. Correcting the
lemma itself is out of scope.

**A new prompt version ships only after review.** The contract change gets a new
prompt contract version, so the cache identity never reuses results from the
previous version. Before that version is accepted, an operator runs a frozen
review case set against the configured provider. The set is built from the real
failures and wins above, plus function-word, idiom, Italian and Modern Greek
spot checks. The new version must keep every protected contextual win, produce
no definition paraphrase and no sentence-inflected Gloss, and return
`lemma_mismatch` for the *fernsehen* case. The dated report is committed under
`doc/reviews/`. The review never runs in CI. The corpus-wide rate of Glosses
containing the lemma translation is reported for information, not as a gate. Every
later prompt or model change is reviewed the same way.

**Existing decks are unchanged.** Ready decks keep their frozen results. A
learner gets the new Glosses by re-preparing a deck explicitly
([ADR 0076](0076-reprepare-ready-deck.md)).

## Why this trade-off

The root cause was the contract, not the model, so the fix is a sharper
contract. Defaulting to the plain equivalent gives up some contextual nuance
for short, predictable cards, but keeps contextual help where the sense really
differs. Omitting a `lemma_mismatch` card costs a card that would otherwise
teach a confidently wrong meaning; the separate count makes analysis errors
measurable rather than hidden. A review gate adds manual work to every prompt
change, but v15 shows that a prompt validated only for response shape can
degrade most cards unnoticed.

## Related

- [Contextual gloss preparation specification](../features/contextual-gloss-preparation.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md)
  (separable-verb presentation)
- [English target alignment review](../reviews/english-target-alignment.md)

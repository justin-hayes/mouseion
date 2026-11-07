# ADR 0114: Idiom cards from dictionary evidence, confirmed by the provider

Status: **Accepted — implementation pending** · Date: 2026-11-07 · Author: Justin + OpenCode

Accepted by merging this ADR's pull request. Decided in
[#2351](https://github.com/justin-hayes/mouseion/issues/2351). Builds on
[ADR 0112](0112-gloss-is-lemma-sense-in-citation-form.md), whose
`lemma_mismatch` outcome is unchanged, and keeps the separation of Generated
and Known vocabulary from [ADR 0036](0036-primary-goal-justified-graduation.md)
and [ADR 0072](0072-goal-owned-vocabulary-and-journey-forecast.md).

## Context

A recognition card teaches one lemma. When the representative sentence uses the
lemma inside an idiom, the card has nowhere to put the idiom's meaning. Italian
*È un ragazzo in gamba* means "he is a capable young man"; *gamba* alone means
"leg". The Gloss either teaches "leg", which does not explain the sentence, or
teaches "capable", which is not the lemma's meaning. ADR 0112 lets the provider
return `lemma_mismatch` for "an idiom headed elsewhere", which omits the card but
teaches nothing.

The v16 Gloss review (#2331) recorded three suspected idiom cases. Only one is
an idiom:

- *in gamba* is a lexeme of its own. The local Wiktionary index has an entry
  "in gamba" (adjective: "capable, on the ball").
- Modern Greek *Κάνει πολύ κρύο απόψε* with target *κάνω* is not an idiom
  problem. The index already lists the *κάνω* sense "to be (weather: cloudy,
  hot, etc.)". v16's Gloss "to be cold" carries the sentence's argument *κρύο*,
  which ADR 0112 already forbids. The correct lemma Gloss is "to be (of
  weather)". It belongs to the Gloss prompt and its review case set (#2352,
  #2353), not to this decision.
- German *Die Nachricht haut mich um* is the separable verb *umhauen* in its
  dictionary sense "to blow someone's mind", a contextual sense like
  *schießen* → "to gush out".

The dictionary index already keeps multi-word Wiktionary entries as their own
lemmas. After removing inflected-form and alternative-form entries, about
10,500 remain for Italian, 4,400 for German and 1,600 for Modern Greek. Only a
small minority carry Wiktionary's `idiomatic` tag (*in gamba* does not), so the
tag cannot identify idioms on its own. Stanza's dependency labels (`fixed`,
`compound`) mark some fixed grammatical expressions but not idioms.

## Decision

**An idiom is a multi-word lexeme with its own dictionary entry.** Mouseion
treats any such entry as a possible idiom, whether it is figurative (*in
gamba*, *κάνω τα στραβά μάτια* "to turn a blind eye") or a fixed compound
(*macchina da scrivere* "typewriter"). Both cases need the same card.

**Detection is dictionary-anchored and provider-confirmed.**

1. *Candidates come only from lexical evidence.* While freezing a deck,
   Mouseion matches the multi-word entries of the dictionary index, plus a
   small reviewed list per language, against the sentences of the selected
   vocabulary. A match needs every word of the entry's headword to match a
   sentence token by surface form or analyzer lemma, in headword order, with a
   bounded number of other tokens between them. The bound is a reviewed,
   versioned parameter. Entries that are only inflected or alternative forms,
   or longer than four words, are not candidates.
2. *The provider confirms, and never proposes.* Each candidate that would
   become a card goes to the provider in its own per-item request with the
   idiom's headword, the matched words, the sentence and the entry's senses.
   The provider either glosses it or reports that this sentence uses the words
   literally. It cannot introduce an idiom that was not supplied.

Precision comes before recall. A missed idiom leaves the deck as it is today. A
false idiom card teaches a wrong meaning.

**An idiom gets its own card, and the lemma card moves to another sentence.**
An idiom card shows the sentence with every matched word in bold (as for a
separable verb, ADR 0067), the idiom's headword and a Gloss in citation form
under ADR 0112's rules, applied to the idiom. It has no morphology block. When
choosing a lemma's representative sentence, Mouseion prefers an accepted
sentence in which the target is not part of an idiom candidate. If every
accepted sentence has the target inside a candidate, the existing behaviour
applies: the provider may return `lemma_mismatch` and the lemma card is
omitted.

**Idioms are card-only, not vocabulary.** An idiom has no Known, Reserved,
Unknown or Generated state, and it never enters a current-reading vocabulary
snapshot. Including an idiom card records no Generated vocabulary, and
omitting one is not a vocabulary Omission. An idiom card is included only when
one of its words is a vocabulary identity frozen in the deck (its *anchor*).
This ties idiom cards to the words the learner is studying and bounds the deck.
The Anki note key of an idiom card has its own namespace, so it never collides
with a lemma note and stays stable across decks. A later ADR may promote idioms
to vocabulary identities. Nothing in this decision blocks that.

**The schema change is a kind on frozen deck items.** The frozen manifest item
and the contextual provider cache identity gain an item kind, `lemma` or
`idiom`, with `lemma` for every existing row. An idiom item stores its headword
and matched spans in place of a lemma and part of speech. No vocabulary-state,
snapshot or Known table changes. The dictionary index adds a lookup from a word
to the multi-word entries that contain it. That index is a derived SQLite
artifact, not application state.

**Rollout is per language, behind its own review.** Idiom cards use a new
prompt contract version, separate from the lemma Gloss contract. A language is
enabled only after an operator review in the style of ADR 0112. The review
committed under `doc/reviews/` must show that:

- every positive case is confirmed with a citation-form Gloss;
- no literal-use control is confirmed;
- a sample of 30 confirmed idiom cards from one real Book in that language
  contains no card that teaches a wrong meaning.

Italian goes first. Its index has the best coverage, and its idioms are mostly
contiguous. German follows once matching handles reordered verbal idioms
(*jemanden auf den Arm nehmen* in a verb-second clause). Modern Greek follows
once its reviewed list makes up for the thin Wiktionary coverage. Ready decks
change only by explicit re-preparation (ADR 0076).

## Considered options

- **Provider-proposed idioms.** Letting the model name any idiom it sees gives
  more recall but no precision bound and no stable identity to review. Rejected.
- **Dictionary match without confirmation.** This is deterministic, but every
  literal co-occurrence (*tagliò la corda del pacco* against *tagliare la
  corda* "to run away") would become a wrong card. Rejected.
- **Gloss the idiom on the lemma card.** The card would then teach an idiom
  under a lemma's headword, which breaks ADR 0112's rule that the Gloss is the
  lemma's sense. Rejected.
- **Idioms as vocabulary identities now.** This would need multi-token
  occurrences, idiom Known state, snapshot membership and changes to lemma
  review, which a schema-wide change cannot justify before idiom cards prove
  useful. Deferred.

## Consequences

- Lemma cards stop teaching idiom meanings whenever another accepted sentence
  exists, which should reduce `lemma_mismatch` omissions.
- Idiom cards can repeat across Book decks, because an idiom has no Known
  state. The stable note key makes a repeat update the existing Anki note
  rather than add a duplicate.
- An idiom whose words are all Known or not selected gets no card. This is
  accepted for now.
- Idiom detection depends on Wiktionary coverage and on the reviewed lists.
  Their growth is reviewed data work, not code.

## Related

- [Idiom cards specification](../features/idiom-cards.md)
- [Contextual gloss preparation](../features/contextual-gloss-preparation.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md)
- [ADR 0071: Deck specification versus presentation](0071-decouple-deck-data-from-presentation.md)
- [ADR 0113: German advmod separable-verb recovery](0113-german-advmod-separable-verb-recovery.md) (reviewed-list precedent)

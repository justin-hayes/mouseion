# Idiom cards

Status: **Specified; implementation in progress** · Date: 2026-11-07 · Decisions:
[ADR 0114](../adr/0114-idiom-cards-from-dictionary-evidence.md),
[ADR 0112](../adr/0112-gloss-is-lemma-sense-in-citation-form.md)

Decided in [#2351](https://github.com/justin-hayes/mouseion/issues/2351).

## Motivation and goal

A recognition card teaches one lemma, so an idiom in its representative
sentence has nowhere to go. In *È un ragazzo in gamba* ("he is a capable young
man") the lemma *gamba* means "leg". Its card either teaches "leg" against a
sentence about competence, or is omitted as a `lemma_mismatch`. Neither teaches
the learner *in gamba*.

The goal: when an analyzed Book uses an idiom built on a word the learner is
studying, the prepared deck teaches the idiom on its own card, and the word's
own card uses a sentence where the word means what it usually means.

## Worked examples

**Idiom card: *in gamba* (Italian).** The deck freezes *gamba* (NOUN). One of
its accepted sentences is *È un ragazzo in gamba.* The index has an entry
"in gamba" (ADJ, "capable, on the ball"), and both its words occur in the
sentence in order, so the occurrence is an idiom candidate anchored on *gamba*.

- The *gamba* lemma card prefers another accepted sentence such as *Mi fa
  male la gamba* and gets the Gloss "leg".
- A separate idiom card shows *È un ragazzo **in gamba**.*, the headword *in
  gamba*, the Gloss "capable", and the highlighted English sentence.
- If the provider reports that the sentence uses the words literally, the
  idiom card is omitted, labelled as an idiom omission, and the *gamba* card
  is unchanged.

**Lemma-card fix, not an idiom: *κάνει κρύο* (Modern Greek).** *Κάνει πολύ
κρύο απόψε* with target *κάνω* was glossed "to be cold" under v16. "Cold"
comes from *κρύο*, so the Gloss carries a sentence argument, which ADR 0112
forbids. The index already has the *κάνω* sense "to be (weather: cloudy, hot,
etc.)". The expected lemma Gloss is "to be (of weather)". This case belongs to
the Gloss review case set and the v17 prompt (#2352, #2353). Wiktionary has no
*κάνει κρύο* entry, so no idiom card is produced unless the Greek reviewed list
later adds one.

**Contextual sense, not an idiom: *umhauen* (German).** *Die Nachricht haut
mich um* uses the separable verb *umhauen* in its dictionary sense "to blow
someone's mind". The lemma card's Gloss handles it, as with *schießen* → "to
gush out".

## Scope and behavior

### Detection

- An **idiom** is a multi-word lexeme with its own entry in the dictionary
  index or in Mouseion's reviewed idiom list for the language. Figurative
  idioms and fixed compounds (*macchina da scrivere* "typewriter") are both
  idioms here. Transparent compounds such as *sistema solare* ("solar system")
  stay idioms by design: their cards are correct, so they are ranked last
  within the per-deck bound rather than removed (#2379).
- Candidate entries: two to four words, excluding entries whose senses are all
  inflected-form or alternative-form references, Wiktionary proper-name
  entries (surnames, places, institutions), and entries whose only sense is
  Wiktionary's literal-use marker ("Used other than figuratively or
  idiomatically: see …"). Beside a real sense, the marker sense stays as
  evidence of literal use. The dictionary derivation
  adds a lookup from each word to the multi-word entries containing it.
- The **reviewed idiom list** is Mouseion-owned data per language, maintained
  like the reviewed separable-verb pairs (ADR 0113). Each row gives a headword
  and its words. It supplements Wiktionary, mainly for Modern Greek, and starts
  empty for Italian and German.
- An **idiom candidate** is a match of an entry in one sentence. Every word of
  the headword matches a sentence token, either by case-folded surface form or
  by analyzer lemma after the language's canonicalization profile. Matches
  appear in headword order, with at most a bounded number of other tokens
  between consecutive words. The bound is a named, versioned matching
  parameter, initially 2. Matching is deterministic and runs while the deck is
  being frozen; it changes neither analysis nor the current-reading snapshot.
- The **anchor** is a frozen vocabulary identity of the deck whose occurrence
  is one of the matched tokens. A candidate without an anchor is ignored.
- Several candidates may overlap. The longest headword wins, and ties go to
  the earlier sentence position.
- The provider confirms each idiom card's candidate in that card's own
  request. It sees only what the matcher found and cannot add an idiom.

### Lemma card

- Representative-sentence ranking gains one rule ahead of the existing ones:
  among accepted sentences, prefer a sentence in which the target occurrence
  is not part of an idiom candidate. The existing sentence-quality order (ADR
  0062) breaks ties.
- If every accepted sentence has the target inside a candidate, the lemma card
  keeps that sentence. ADR 0112's `lemma_mismatch` outcome, omission and
  advisory flag apply unchanged.
- The lemma Gloss contract is unchanged.

### Idiom card

- One idiom card per distinct idiom per deck. Its representative sentence is
  the best accepted sentence containing a candidate for that idiom, under the
  existing sentence-quality ranking.
- Front: the complete source sentence with every matched word in bold, using
  the multi-span target presentation of separable verbs (ADR 0067).
- Back: the idiom's headword in citation form, a Gloss and the highlighted
  English sentence. The Gloss follows ADR 0112's rules applied to the idiom:
  the English equivalent in citation form, never a paraphrase and never
  carrying the sentence's inflection or arguments. There is no morphology
  block, article, gender or IPA. The part-of-speech label reads "idiom".
- Request: one per-item provider request per idiom card. It sends the
  headword, the matched forms, the sentence and the bounded candidate senses,
  and nothing about the Book or the learner. When the dictionary lists the
  headword under several parts of speech, their senses are pooled in
  part-of-speech order before the bound applies. The response has the same
  shape as the lemma contract under a separate prompt contract version
  (`idiom-card-v2`). Its unresolved kinds are `ambiguous` and `literal_use`
  ("this sentence uses the words literally").
- The Gloss renders the sense the sentence uses, not the first listed sense,
  and agrees with the translation. A candidate sense that only marks literal
  use ("Used other than figuratively or idiomatically") never supports a
  Gloss. When no candidate sense matches the sentence's meaning, the answer is
  `ambiguous` and the card is omitted: a confirmed idiom card always cites a
  dictionary sense (#2378).
- Every candidate sense carries a stable evidence ID, so a confirmed Gloss can
  cite it. An unresolved answer may leave the translation blank, since the
  translation is the idiom in the confirmed sense (#2385).
- An unresolved idiom request omits only the idiom card. The deck result
  counts idiom cards and idiom omissions separately from vocabulary cards and
  vocabulary Omissions, and labels `literal_use` as "the words are used
  literally here". An idiom omission never raises a lemma review flag.
- Provider failures follow the existing durable retry path (ADR 0100). A run
  in which only idiom cards fail does not fail the deck, and a run whose only
  successes are idiom cards is still "all omitted" and fails.
- The Anki note key is namespaced to idioms by language and headword, so it is
  stable across decks and never collides with a lemma note. Idiom cards carry
  a `mouseion::idiom` tag.
- A per-deck bound of **30 idiom cards** is fixed during implementation and
  reviewed with the case set. Ranking has two tiers. A **noun-only idiom**,
  whose dictionary entries for the headword are all nouns, ranks after every
  other idiom; a headword with mixed or missing parts of speech, or from the
  reviewed idiom list, stays in the first tier. Within each tier, anchors with
  more sentence occurrences go first; ties break by earliest sentence, then
  headword, then language. The ranking records no version: it only decides
  which idioms are frozen, and Ready decks change only by re-preparation. The
  tiers only decide which idioms are kept; kept idiom cards follow the
  within-tier order. The ready deck's status view and JSON report
  any overflow. Overflowed idioms are not idiom omissions and raise no review
  flag.

### Domain model

- Idioms are card-only. They have no Known, Reserved, Unknown or Generated
  state and never enter a current-reading vocabulary snapshot, Known
  vocabulary, Vocabulary Browse or Concordance.
- Including an idiom card records no Generated vocabulary; generation still
  never marks anything Known (ADR 0036). Reading completion is unchanged.
- Schema: the frozen manifest item and the contextual provider cache identity
  gain an item kind (`lemma` | `idiom`; existing rows are `lemma`). An idiom
  item stores its headword, matched spans, anchor identity and the dictionary
  evidence provenance in place of a lemma and part of speech. No vocabulary,
  snapshot or Known schema changes.
- A frozen idiom item is part of the deck specification (ADR 0071). A
  presentation-only rerender reads it unchanged, and a dictionary or list
  refresh reaches a deck only through explicit re-preparation (ADR 0076).
- The frozen manifest also records the per-deck idiom overflow count as
  additive metadata, so the deck result can report it without re-running
  detection. It is not an omission and changes no vocabulary, snapshot or Known
  schema.

### Language rollout and review

- Each language has an idiom-card switch, off by default. Turning it on
  requires a committed review report for that language.
- The operator review extends the Gloss review command with an idiom case set
  (`idiom-review-v2`), run outside CI at the deployed model and reasoning
  setting. It contains:
  - positive cases: *in gamba*, *in bocca al lupo* ("good luck"), *macchina da
    scrivere*, *κάνω τα στραβά μάτια* ("to turn a blind eye");
  - literal-use controls in which the same words are meant literally, such as
    *Tagliò la corda del pacco* against *tagliare la corda* ("to run away")
    and *Sie nimmt das Kind auf den Arm* against *jemanden auf den Arm
    nehmen* ("to pull someone's leg");
  - ambiguous controls in which the sentence uses a sense no candidate lists,
    such as the comparative *rispetto a* ("compared with");
  - lemma-card controls confirming that the anchor's card moves to an
    idiom-free sentence.
- Gate per language: every positive case is confirmed with a citation-form
  Gloss, no literal-use or ambiguous control is confirmed, and a sample of 30 confirmed
  idiom cards from one real Book in that language contains no card that
  teaches a wrong meaning. The dated report goes under `doc/reviews/`.
- The sample comes from an in-memory freeze of the Book's current deck
  preparation, with idiom cards on and no per-deck bound. It writes nothing and
  needs no recorded assessment, so a language can be reviewed before its
  switch is turned on (#2375).
- Because the sample ignores the bound, the report also lists the idioms a
  prepared deck would keep, in rank order with headword, anchor, occurrence
  count and a noun-only marker, followed by the overflow count. It needs no
  provider calls and is not a gate criterion (#2379).
- Order: Italian first. German follows once matching handles reordered
  verb-second clauses. Modern Greek follows once its reviewed list has enough
  entries to sample 30 cards.

## Non-goals

- Idioms as vocabulary identities with Known state, snapshot membership,
  multi-token occurrences or lemma review. A later ADR may add them.
- Provider-proposed idioms, or detection from dependency labels alone.
- Idiom cards in custom vocabulary decks or for Ancient Greek.
- Showing idioms in Vocabulary Browse, Concordance or Sentence study.
- Changing the lemma Gloss contract. *κάνει κρύο* is handled there (#2352,
  #2353).
- Automatically migrating Ready decks.

## Acceptance criteria

- [ ] Accepted ADR settling detection, card outcome, card-only domain model and
      the item-kind schema change (ADR 0114).
- [ ] The dictionary derivation exposes a lookup from a word to the two-to-four
      word multi-word entries containing it, excluding form-of and alt-of only
      entries, proper-name entries and literal-use-marker-only entries, and
      loads the reviewed idiom list for each language.
- [ ] Freezing a deck finds idiom candidates for its frozen vocabulary, prefers
      idiom-free sentences for lemma cards, and freezes one idiom item per
      distinct anchored idiom up to the bound, with evidence provenance.
- [ ] An idiom item gets one provider request under its own prompt contract
      version. A resolved answer yields an idiom card. `literal_use` or
      `ambiguous` omits only that card, and the deck result counts and labels
      it separately.
- [ ] Idiom cards record no Generated vocabulary and change no snapshot, Known
      or Reserved state.
- [ ] The idiom card renders with bold matched words, headword, Gloss and
      highlighted English, with no morphology block, under a stable namespaced
      note key.
- [ ] The per-deck bound ranks noun-only idioms after every other idiom, and
      the idiom review report lists the idioms a prepared deck would keep
      (#2379).
- [ ] Italian review report committed and passing the gate before Italian is
      enabled. German and Modern Greek each follow with their own report.
      The [2026-11-08 Italian review](../reviews/idiom-review-2026-11-08.md)
      failed the gate (three wrong-meaning sample cards); Italian stays off
      until a new report for `idiom-card-v2` and `idiom-review-v2` (#2378)
      passes it (#2367). The
      [2026-11-08 v2 re-run](../reviews/idiom-review-v2-2026-11-08.md) also
      failed: idiom senses carry no evidence IDs, and unresolved answers with
      an empty translation are rejected; Italian stays off (#2385).

## Related

- [ADR 0114: Idiom cards from dictionary evidence](../adr/0114-idiom-cards-from-dictionary-evidence.md)
- [Contextual gloss preparation](contextual-gloss-preparation.md)
- [Dictionary gloss enrichment](dictionary-gloss-enrichment.md)
- [Recognition card sentence presentation](recognition-card-sentence-presentation.md)
- [Gloss review case set](../reviews/README.md)

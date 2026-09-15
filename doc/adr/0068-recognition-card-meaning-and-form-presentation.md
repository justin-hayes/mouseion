# ADR 0068: Recognition-card meaning and form presentation

Status: **Accepted** · Date: 2026-09-14 · Author: Justin + opencode · Amended by [ADR 0071](0071-decouple-deck-data-from-presentation.md)

Amends the card contract of **ADR 0029** and **ADR 0064**, and extends the
presentation decisions of **ADR 0067**.

## Context

ADR 0064 made gloss and morphology local and consent-free, and ADR 0067 refined
noun morphology (`Plural`) and multi-span target bolding. Card-quality review of
generated German decks surfaced a further set of issues that are not morphology:

1. The back renders three separate English blocks — `Gloss` (the dictionary
   sense set), `English` (the lemma translation), and `EnglishSentence` (the
   contextual sentence translation). Two of them say the same thing, and the
   learner cannot tell which is which. The meaning content is duplicated.
2. The dictionary index already carries per-entry IPA, but no pronunciation
   reaches the card; `LexicalProvider.Lookup` never reads the `entries.ipa`
   column, and `ipa_for` takes the raw first sound with no normalization.
3. Verb cards carry no inflectional information. Wiktextract's headword summary
   supplies principal parts (`geht · ging · gegangen`; `vàdo · andài · andàto`),
   but the derivation captures forms only for noun plural and article.
4. The card has no visual hierarchy: headword, gloss, lemma translation, and
   sentence render as near-identical unstyled blocks.

## Decision

1. **One meaning block.** The back has a single meaning block sourced from the
   dictionary `Gloss` — a compact, context-ordered sense set. The separate
   `English` lemma translation is no longer rendered; its field is retained in
   the note/TSV contract for compatibility, and its removal is a separate
   decision. `EnglishSentence` remains the contextual sentence translation
   (ADR 0021 distinction preserved).
2. **Pronunciation is a dedicated note field.** The index's per-entry IPA is
   surfaced as `IPA`, rendered beside the headword and self-suppressing when
   empty. Quality is owned by the derivation step: prefer a standard phonemic
   `/…/` form and drop optional-segment and regional noise, rather than rendering
   the raw first sound.
3. **Principal parts are a dedicated note field, German-first.** The card renders
   a verb's `PrincipalParts` — third-person singular present, preterite, and past
   participle (`geht · ging · gegangen`), the infinitive already being the
   headword — self-suppressing when absent. The extraction is language-agnostic so
   Italian may be added as a data-only change; Italian display is deferred.
   Auxiliary and mood are out of scope.
4. **Meaning leads; forms support.** The back presents, in order, the headword
   line (headword, noun plural, IPA, principal parts, POS), then the single
   meaning block, then the contextual sentence translation. Within the headword
   line, inflectional forms are visually secondary to the headword. Styling
   follows the design system's semantic tokens and information hierarchy rather
   than bespoke decoration; the Anki template and CSS are prototyped as a static
   preview before they are frozen into the exporter.
5. **Frozen like other dictionary-derived data.** IPA and principal parts are
   resolved when the prepared-deck manifest is frozen, carried on the frozen
   entries, and versioned by the index `provider_version`, consistent with
   ADR 0064 and ADR 0067. This bumps `ManifestSchemaVersion` to 5 and shifts TSV
   columns.
6. **Roll forward.** Existing generated decks and imported notes are not
   rewritten; the new fields and presentation reach a learner only when a deck is
   re-prepared.

## Consequences

- The recognition-card field order becomes `Identity`, `Text`, `Article`,
  `Lemma`, `Plural`, `IPA`, `PrincipalParts`, `POS`, `Gloss`, `English`,
  `EnglishSentence`, `BookTitle`, then TSV tags. `English` is retained but
  unrendered. Fixtures, schema-regression tests, and TSV tests are updated.
- The dictionary index must be regenerated to carry normalized IPA and principal
  parts; `provider_version` moves, and existing decks keep their frozen values
  until re-prepared.
- IPA and principal-parts coverage is partial — a minority of entries carry
  usable data — so the card degrades gracefully to an unchanged, valid card when
  either field is absent.
- The meaning block is simpler and unambiguous; dropping the rendered `English`
  line removes redundancy without changing stored semantics.
- The visual contract now depends on a reviewed preview; typography and colour
  remain presentation decisions, not domain decisions.

## Alternatives considered

- **Keep rendering `English` as a separate lemma translation.** Rejected:
  duplicates the gloss and the contextual sentence translation; three
  near-synonymous English blocks is worse than one.
- **Append IPA or principal parts to the gloss or headword text.** Rejected for
  the reason ADR 0067 kept `Plural` separate: mixing derived fields with display
  text has no clean seam and does not survive TSV round-trips.
- **Ship Italian principal parts now.** Deferred: the German-first posture
  matches sentence-quality scoring (ADR 0062), and a language-agnostic extraction
  keeps Italian a data-only follow-up.
- **Store the raw first-sound IPA.** Rejected: the raw field mixes
  phonemic/phonetic notation, regional variants, and optional segments;
  normalization belongs to the index, where it can be tested.
- **Keep three English blocks and rely on styling to distinguish them.**
  Rejected: styling cannot make duplicated content non-duplicated; the redundancy
  is a content problem.

## References

- [Recognition-card sentence presentation](../features/recognition-card-sentence-presentation.md)
- [Dictionary gloss and morphology enrichment](../features/dictionary-gloss-enrichment.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0021: Contextual sentence translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0029: Recognition-card sentence presentation](0029-recognition-card-sentence-presentation.md)
- [ADR 0062: Sentence-quality scoring derived from the persisted corpus](0062-derived-sentence-quality-scoring.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md)

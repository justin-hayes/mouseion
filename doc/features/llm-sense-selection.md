# LLM sense selection and fallback gloss

Status: **Proposed** · Date: 2026-09-14 · Decision:
[ADR 0069](../adr/0069-llm-sense-selection-and-fallback-gloss.md)

## Motivation

The dictionary provider (ADR 0064) renders a compact, context-ordered sense set,
but its ordering is a deterministic simplified-Lesk score over bag-of-words
overlap. It cannot recover a correct sense that it ranked below the display cut,
and it cannot recognize that none of the dictionary senses fit the sentence. The
consented external translation call already carries the representative sentence,
and its response includes a `gloss` field that is required, validated, and
cached — then discarded, because the card's `Gloss` is dictionary-frozen.

ADR 0067 left dictionary gloss coverage measured but not acted on: broadening
sources or adding a consent-gated fallback were later decisions. This feature
gives the LLM the judgement task it is better at than lexical overlap, and uses
the measured coverage gap to fill missing glosses.

## Goal

When external translation is consented, the LLM selects and orders the
dictionary senses that fit the representative sentence, and supplies a
consent-gated fallback gloss when the dictionary has no gloss or none of its
senses fit. The dictionary remains the authored source of sense text and the
deterministic default; the LLM only reorders, and authors text solely as a
fallback.

## Scope

- **Selection over a frozen candidate set**: the prepared-deck manifest freezes a
  bounded candidate sense list (top-8 in dictionary order) alongside the
  deterministic rendered gloss. The model returns ordered indices into that list
  (a subset, capped at the display limit). Dictionary sense text is never
  authored by the model.
- **Fallback gloss**: one concise English gloss, returned only when no candidate
  sense fits or no dictionary gloss exists. It is the only LLM-authored meaning
  text on a card.
- **One call, one consent**: selection and fallback are fields of the existing
  per-item external translation request, riding `external_translation_consent`.
  No new provider, consent surface, or operator flag; dictionary senses are
  public and the sentence already transits under that consent.
- **Post-freeze application**: selection and fallback are produced in the
  translation phase and applied by the finalizer as a render overlay, like
  translation outcomes. The no-consent path renders the frozen deterministic
  gloss unchanged.
- **Cache identity**: the result is cached with the dictionary identity, so a
  cached selection can never be mapped onto a different candidate set from a
  regenerated index.
- **Honest reporting**: the card stays clean; fallback-gloss counts appear in the
  deck-preparation completeness summary and in freeze/finalize metrics.

## Non-goals

- Authoring or paraphrasing dictionary senses generally. The model selects; it
  does not write sense text.
- A separate sense-selection call or provider. It is part of the consented
  translation call.
- A learner-visible economy or quality toggle, and any on-card fallback marker.
- Native-language definitions or frequency data.
- Changing representative-sentence selection or the recognition front.

## Requirements

### Request and response

- The request carries the representative sentence and the frozen candidate sense
  list (index-correlated), in addition to the existing translation fields.
- The response adds `sense_order` — ordered integer indices into the candidate
  list, a subset, deduplicated, capped at the display limit — and
  `fallback_gloss` — an English gloss, non-empty only when no candidate applies
  or none exist.
- The general LLM `gloss` field is retired; required-field validation no longer
  demands a gloss. Translation remains required under the existing consented
  standard-run policy.
- Prompt-versioning is the cache-bypass: existing cached translations are
  orphaned, not deleted (ADR 0021).

### Validation and failure

- Out-of-range or duplicate indices invalidate the selection response; markup and
  over-length fallback text are rejected.
- A malformed or missing selection falls back to the deterministic order and
  logs a warning. Selection never fails a run; only translation fails a
  consented standard run closed.

### Rendering

- With a valid selection, the meaning block renders the chosen senses in the
  model's order; without one, it renders the frozen deterministic gloss. A
  fallback gloss renders as the meaning block when no dictionary sense applies.
- The meaning block is unchanged in kind — a compact sense set — and the card
  does not mark fallback provenance.

### Metrics

- Freeze records dictionary `gloss_coverage` per language/POS (ADR 0067).
- Finalize records fallback-gloss usage, so the fallback rate is measurable.

## Acceptance criteria

- [ ] A consented run selects over the frozen candidate set and renders senses in
      the model's order; the dictionary remains the text source
- [ ] A fallback gloss is rendered when the dictionary has no gloss or no
      candidate fits, and only then
- [ ] Without consent, no provider, or malformed selection, the deterministic
      frozen gloss renders unchanged
- [ ] Out-of-range/duplicate indices and invalid fallback text are rejected
- [ ] The selection is cached under a key that includes the dictionary identity
- [ ] A selection failure logs a warning and does not fail the run
- [ ] Deck completeness reports the fallback-gloss count; freeze/finalize record
      coverage and fallback-rate metrics

## References

- [ADR 0069: LLM sense selection and fallback gloss](../adr/0069-llm-sense-selection-and-fallback-gloss.md)
- [Dictionary gloss and morphology enrichment](dictionary-gloss-enrichment.md)
- [Recognition-card sentence presentation](recognition-card-sentence-presentation.md)
- [ADR 0007: Enrichment providers, caching, and privacy](../adr/0007-enrichment-providers-caching-privacy.md)
- [ADR 0021: Contextual sentence translation cache and privacy](../adr/0021-contextual-translation-cache.md)
- [ADR 0064: Built-in dictionary enrichment provider](../adr/0064-dictionary-enrichment-provider.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](../adr/0067-recognition-card-morphology-presentation.md)

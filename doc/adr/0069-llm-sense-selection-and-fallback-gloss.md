# ADR 0069: LLM sense selection and fallback gloss

Status: **Accepted** · Date: 2026-09-14 · Author: Justin + opencode

Amends **ADR 0007** (gloss classification and privacy), the card contract of
**ADR 0029**, and **ADR 0064** (sense ordering and fallback alternatives);
references **ADR 0021** and **ADR 0032** (cache identity and execution).

## Context

ADR 0064 made gloss local, deterministic, and consent-free, retaining the
external LLM only for contextual sentence translation. The LLM response still
returns a `gloss` field that is required, validated, and cached, yet never
rendered: `applyExactEnrichment` copies only translation fields, and
`Entry.Gloss` is assigned solely by the local dictionary lookup.

The only sense-selection mechanism is the deterministic simplified-Lesk ordering.
It scores bag-of-words overlap and drops senses below the display cut, so it
cannot recover a correct sense it ranked low, and it has no way to recognize that
none of the dictionary senses fit the sentence. ADR 0067 left two consequences
open: dictionary gloss coverage is measured (`gloss_coverage`), and broadening
dictionary sources or adding a consent-gated LLM gloss fallback are later
decisions driven by that metric. This ADR takes the fallback decision and gives
the LLM the role it is genuinely better at than lexical overlap — judging which
provided sense the sentence uses.

## Decision

1. **The LLM selects and orders dictionary senses; it does not author them.**
   When external translation is consented, the model receives the representative
   sentence and a bounded, frozen candidate sense set, and returns ordered
   indices into that set (a subset, capped at the display limit). The dictionary
   remains the authored source of selected sense text; the deterministic order is
   the fallback.
2. **The LLM may supply a fallback gloss.** When the dictionary has no gloss for
   the lemma, or none of its candidate senses fit the sentence, the model may
   return one concise English fallback gloss. A fallback gloss is the only
   LLM-authored meaning text on a card, and it is distinct from the contextual
   sentence translation.
3. **One call, one consent.** Selection and fallback are folded into the existing
   per-item external translation request. They ride the existing
   `external_translation_consent`; no new consent, provider, or configuration
   surface is introduced, and no new category of data leaves the lab (dictionary
   senses are public, and the sentence already transits under that consent).
4. **Frozen candidates; post-freeze application.** The manifest freezes a bounded
   candidate sense list (top-8 in dictionary order) alongside the deterministic
   rendered `Gloss`, bumping `ManifestSchemaVersion` to 6. Selection and fallback
   are produced during the post-freeze translation phase and applied by the
   finalizer as a render overlay, exactly like translation outcomes; the
   no-consent path renders the frozen deterministic `Gloss` unchanged.
5. **The result is cached with dictionary identity.** `enrichment_cache` gains
   `sense_selection` and `fallback_gloss`, and `dictionary_provider_version`
   joins its key, so a cached selection can never be mapped onto a different
   candidate set produced by a regenerated index.
6. **The standing LLM gloss field is retired.** The provider response no longer
   carries a general gloss, and required-field validation no longer demands one.
   `sense_order` and `fallback_gloss` are validated: indices must be in range and
   deduplicated, the fallback is non-empty only when triggered, and both are
   length- and markup-bounded.
7. **Selection failures degrade, never fail.** A malformed or missing selection
   falls back to the deterministic order and logs a warning; only translation
   continues to fail a consented standard run closed.
8. **Clean card, honest surface.** The card does not mark a fallback gloss.
   Provenance is surfaced where the learner makes decisions: the deck-preparation
   completeness summary reports the fallback-gloss count, and freeze/finalize
   record dictionary-coverage and fallback-rate metrics.
9. **Prompt versioning is the cache-bypass.** Adding selection changes
   `llmPromptVersion`, so prior cached translations are bypassed (ADR 0021
   mechanism); they are orphaned, not deleted. This one-time re-billing is
   accepted.

## Consequences

- Gloss ordering is context-aware when the learner consents and deterministic
  when they do not or when no provider is configured. The learner-visible meaning
  block is unchanged in kind — still a compact sense set — but better chosen.
- The LLM no longer writes dictionary senses. Wrong-sense risk is bounded to
  index-position selection errors, which are validated against the frozen
  candidate list, and to the fallback gloss, which is explicitly a fallback and
  reported as one.
- `enrichment_cache`'s key and columns change, `ManifestSchemaVersion` moves to
  6, and artifact fixtures and completeness tests change.
- A prompt change orphans all previously cached translations; a re-prepared deck
  may re-translate. This is the established provider-version bypass (ADR 0021).
- The candidate set is frozen, so selection is reproducible against the exact
  manifest and independent of later index changes.
- `gloss_coverage` (ADR 0067) now has a consumer: coverage gaps can be filled by
  a fallback gloss, and the fallback rate is measurable.

## Alternatives considered

- **A separate sense-selection call.** Rejected: doubles external requests and
  needs its own consent and cache home; the translation call already carries the
  sentence and is changing anyway.
- **Operate only on the frozen top-N rendered glosses.** Rejected: top-N is the
  deterministic ranking itself, so the model could never recover a sense Lesk
  ranked below the cut — the failure the selector exists to fix.
- **Let the LLM rephrase or author glosses generally.** Rejected: trades
  attributable, dictionary-authored text for model text and reopens ADR 0064's
  classification for no selection benefit.
- **Dictionary only, as fallback when the LLM is absent.** Rejected as before
  (ADR 0064): the dictionary stays the deterministic default; the LLM is an
  optional upgrade, not a requirement.
- **Mark fallback glosses on the card.** Rejected: clutters a study aid; the
  completeness summary and metrics carry the honesty signal where decisions are
  made.
- **Gate selection behind a new operator flag.** Rejected: it rides an
  already-consented call and an already-changing prompt; a separate flag adds
  configuration with no distinct data or privacy boundary.

## References

- [LLM sense selection and fallback gloss](../features/llm-sense-selection.md)
- [Dictionary gloss and morphology enrichment](../features/dictionary-gloss-enrichment.md)
- [Recognition-card sentence presentation](../features/recognition-card-sentence-presentation.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0021: Contextual sentence translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0029: Recognition-card sentence presentation](0029-recognition-card-sentence-presentation.md)
- [ADR 0032: Standard-first prepared-deck translation](0032-standard-first-prepared-deck-translation.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md)

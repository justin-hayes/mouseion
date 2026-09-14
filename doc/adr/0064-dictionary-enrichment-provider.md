# ADR 0064: Built-in dictionary enrichment provider

Status: **Accepted** · Date: 2026-09-13 · Author: Justin + opencode

Amends **ADR 0007** (gloss classification) and the card contract of **ADR 0029**.

## Context

ADR 0007 classified gloss as bundled with translation — **external, optional,
consent-gated** — and deferred a built-in dictionary provider (dict.cc / Leo /
Wiktionary) to a later iteration. Today, card English content exists only when
an external LLM is configured *and* the learner consents; without it, `English`
exports empty and the cached `gloss` is never rendered on any card. The
definite article comes from a morphology heuristic, and plural/forms are
absent.

Open dictionary data is available and self-hostable: Wiktionary, extracted by
Wiktextract and published by Kaikki.org as JSONL (regenerated weekly), contains
concise English glosses, ordered senses with tags/topics/examples, inflection
forms, and IPA for both German and Italian. The `DictionaryLookup` provider
seam already exists in `internal/enrichment` but has no production
implementation.

The DWDS frequency import was previously loaded into Postgres and then removed
(ADR 0018); raw dictionary datasets are large, weekly-moving artifacts that do
not belong in application-state Postgres.

## Decision

1. **A built-in dictionary provider, in-process.** Add a local lexical-provider
   seam and implement it over a build-time-derived **read-only SQLite index**
   (the dictionary index) consumed at startup by the Go web/River process via
   pure-Go `modernc.org/sqlite`. The existing test-only `DictionaryLookup`
   adapter is translation-shaped and external-gated, so it is repurposed to
   this local seam rather than wired into the external translation slot. No new
   gRPC service; the NLP service is unchanged.
2. **The index is a build artifact, not database state.** A `make` target and a
   Python standard library downloader derive a compact per-language SQLite file
   from the raw Wiktextract JSONL (enwiktionary `de`/`it` entries — English glosses),
   recording dump/extraction date as `provider_version`. Not the deprecated
   per-language downloads; not a Postgres import.
3. **Gloss becomes local, default-on enrichment.** The dictionary supplies a
   compact, context-ordered set of English glosses (top-N senses, truncated to
   a token budget). Sense ordering is deterministic without an LLM: a
   simplified Lesk context score over the representative sentence (content
   words ±5 around the bolded target + dependency relations vs. each sense's
   gloss/example/topic words, with a fixed-phrase boost), primary sense
   fallback. This reclassifies gloss from external+consent-gated to local.
4. **Morphology from the dictionary.** Gender/article and plural resolve from
   the ranked top sense's forms, replacing the `nounArticle` heuristic for
   indexed lemmas.
5. **Card contract gains a `Gloss` field.** The Anki/TSV note adds `Gloss` on
   the back; `English` remains the contextual sentence translation (ADR 0021
   distinction preserved).
6. **Resolved at manifest freeze, not the external cache.** The dictionary
   lookup is local and deterministic, so its resolved gloss and morphology are
   computed when the prepared-deck manifest is frozen and carried on the frozen
   entries, with the index `provider_version` recorded for provenance. It does
   **not** flow through `enrichment_cache`, which is consent-gated and reserved
   for the external translation path; the immutable prepared `.apkg` already
   provides reproducibility.
7. **German and Italian symmetric.** The index, provider, and card field are
   language-agnostic; both languages are built by the same derivation step.

## Consequences

- Cards become complete without any LLM: English gloss and morphology are
  deterministic, local, and consent-free.
- A weekly Kaikki regeneration is a build-step artifact swap, not a migration
  or data-loading job; the ADR 0018 Postgres-import mistake is not repeated.
- Sense ordering is explainable and testable; it is the same class of
  deterministic selection as sentence-quality scoring (ADR 0062), not a
  black-box call.
- The recognition-card note contract changes (new `Gloss` field), requiring
  updated artifact fixtures and import/export tests, per the ADR 0029 pattern.
- The local lexical-provider seam replaces the test-only, translation-shaped
  `DictionaryLookup` types; a later move to a gRPC service stays possible behind
  the same interface.
- Wiktionary-derived gloss text (CC BY-SA 3.0 / GFDL) ships inside downloaded
  decks; the attribution notice is retained and the data portion stays
  share-alike.

## Alternatives considered

- **A dictionary gRPC service (standalone or on the NLP service).** Rejected:
  card generation currently has no runtime dependency on the NLP service;
  introducing one for static data adds a coupling, failure modes, and per-card
  RPC cost for no benefit at this scale. Swappable later behind the interface.
- **Import into Postgres.** Rejected: the ADR 0018 precedent; weekly-moving
  large datasets churn migrations and bloat application state.
- **The deprecated per-language Kaikki downloads.** Rejected: slated for
  removal; the raw Wiktextract JSONL filtered by `lang_code` is the maintained
  path.
- **Dictionary only as fallback when the LLM is absent.** Rejected: gloss is
  local and deterministic, so it is the always-on default; the LLM remains an
  optional upgrade for contextual sentence translation.
- **Single primary sense only.** Rejected: a compact sense set is
  pedagogically richer and eliminates wrong-sense risk; ranking reorders
  display but never removes a sense.

## References

- [Dictionary gloss and morphology enrichment feature](../features/dictionary-gloss-enrichment.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0018: Remove global frequency dataset (DWDS) import](0018-remove-dwds-frequency.md)
- [ADR 0021: Contextual sentence translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0029: Recognition-card sentence presentation](0029-recognition-card-sentence-presentation.md)
- [Kaikki.org](https://kaikki.org/) — Wiktextract extraction of Wiktionary, JSONL dumps

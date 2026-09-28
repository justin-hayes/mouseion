# ADR 0080: LLM-proposed English target alignment on recognition cards

Status: **Accepted (not yet implemented)** · Date: 2026-09-28 · Author: Justin + OpenCode

Amends the distributed-correspondence fallback of
[ADR 0029](0029-recognition-card-sentence-presentation.md). Preserves its
complete-translation, best-effort emphasis, and no-provider-markup decisions;
the frozen-overlay and explicit re-preparation boundaries of
[ADR 0071](0071-decouple-deck-data-from-presentation.md) and
[ADR 0076](0076-reprepare-ready-deck.md) remain in force. Prepared-deck
translation follows [ADR 0079](0079-contextual-glosses-require-llm.md): it
requires the configured LLM and adds no learner consent setting.

## Context

A German `umhauen` card translates the target as “knocks … over,” separated by
the object in its complete English sentence. The current provider returns the
plain phrase `knocks over`; the renderer can emphasize only one contiguous
match, so it leaves the sentence unhighlighted. ADR 0029 deliberately treated
distributed correspondences as too uncertain to highlight. Splitting every
cached phrase into words would improve this case, but cannot express a broader
semantic correspondence and can select the wrong occurrence in a repeated or
idiomatic sentence.

## Decision

English target alignment remains **optional**: a complete English translation
is useful without emphasis. For new prepared-deck translations, ask the LLM to
propose an ordered list of exact excerpts from its complete translation: one
excerpt for a contiguous target phrase, multiple for a discontinuous one. It
should identify only the smallest meaning-bearing words or phrases for the
tested vocabulary, not a larger clause to force a match. New provider responses
use this structured field alone; a separate legacy single-phrase field remains
readable for historical cached results. Cache the structured alignment alongside
the translation under a new prompt/provider version, leaving old rows immutable.
Both standard and Batch preparation use the same response contract.

Treat the LLM's alignment as a proposal, never as markup or an instruction to
trust blindly. Mouseion validates that each excerpt occurs exactly once with
word boundaries in the complete translation, in the supplied order and without
overlap; an alignment may not collectively emphasize every lexical word of a
multiword sentence. For an absent or invalid proposal, discard the *entire*
alignment before caching and render the complete escaped English translation
and contextual Gloss without emphasis. Validate again at render. These checks
cannot prove semantic minimality, so that remains a provider instruction and
a quality-evaluation concern. Mouseion alone generates escaped note markup.

Apply this to prepared decks in every supported study language with an English
sentence translation. Existing ready decks continue using their cached
single-phrase behavior (including separate presentation fixes) and are not
marked presentation-stale or offered a new upgrade prompt solely for lacking
structured alignment. An offline re-render cannot request provider output or
infer new spans from the old phrase. A learner who wants the new alignment
explicitly re-prepares the deck under the current provider contract, which may
also change its translation and Gloss; there is no alignment-only LLM call.

## Consequences

- A new structured cache field and prompt/provider-version change are needed;
  existing immutable cache rows and prepared-deck artifacts remain intact.
  Re-import updates existing Anki notes when the re-prepared card identity is
  unchanged; re-preparation is not guaranteed to preserve its selected sentence.
- No provider-generated HTML enters the note. Invalid or ambiguous alignment
  never blocks a usable translation or contextual Gloss.
- Before rollout, curated German, Italian, and Modern Greek examples where
  available must cover contiguous, discontinuous, repeated, idiomatic, and
  no-alignment cases. Reviewed examples must contain no misleading emphasis;
  missing emphasis on uncertain cases is acceptable. Automated tests use fixed
  provider-response fixtures rather than live calls, followed by representative
  review against the configured model.

## Alternatives considered

- **Split cached target phrases deterministically.** Would improve some
  existing decks through presentation-only re-rendering but cannot express
  all correspondences and risks false matches; rejected in favor of provider
  proposals plus validation.
- **Use provider HTML or unvalidated spans.** Rejected: it gives an external
  provider control of card markup and can misleadingly emphasize unrelated
  words or the whole sentence.
- **Call an alignment-only provider for old translations.** Rejected: it adds a
  second enrichment workflow across the immutable cache boundary merely to
  retrofit historical decks.

## Related

- [Recognition-card sentence presentation](../features/recognition-card-sentence-presentation.md)
- [ADR 0021: Contextual sentence translation cache and privacy](0021-contextual-translation-cache.md)
- [ADR 0071: Decouple prepared-deck data from presentation](0071-decouple-deck-data-from-presentation.md)
- [ADR 0076: Roll forward when a ready deck requires re-preparation](0076-reprepare-ready-deck.md)

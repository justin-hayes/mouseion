# ADR 0067: Recognition-card morphology and multi-span target presentation

Status: **Accepted** · Date: 2026-09-14 · Author: Justin + opencode

Amends the card contract of **ADR 0029** and **ADR 0064**, and the
"no learner-facing surface" non-goal of **ADR 0061**.

> **Amendment (2026-09-15):** The durable prepared-deck manifest persists
> `SentenceTokens` in its per-item `render_payload` (see
> [ADR 0030](0030-durable-prepared-deck-translation.md)). §3's
> "no new cardexport column" and the rejected "persist resolved bold spans
> on the entry" alternative assumed the dependency parse was available at
> render time; a durable run reloads the frozen manifest in its finalizer and
> had no parse, so multi-span bolding silently degraded to the observed form
> only. The frozen parse is now a durable render input, and resolution stays at
> render time.

## Context

Card-quality review of a German prepared deck surfaced four issues:

1. Some cards render an article that is a lowercase copy of the noun — the
   noun's plural — e.g. `gauner Gauner` and `feigheiten Feigheit`. The defect is
   in the dictionary index, not the renderer: `article_for`
   (`nlp/scripts/derive_dictionary_index.py`) accepts the first form whose tags
   include `article`/`definite`/`definite article`, and for these German noun
   entries that form is the plural (`Gauner`, `Feigheiten`). `nounArticle`
   (`internal/cardexport/cardexport.go`) then trusts the explicit `Article`
   morphology value and lowercases it. A definite-tagged form could equally be
   a non-nominative article (`den`, `dem`, `des`) or an Italian plural or
   prepositional article.
2. The plural is appended to the `Gloss` field (`building (Pl. Häuser)`), where
   it sits among the English senses instead of beside the singular it inflects.
3. Leakage from the dictionary index leaves some lemmas with no gloss at all;
   there is no measurement of the gap and no decided fallback.
4. For separable verbs, only the finite verb is bolded. In
   `»…«, rief Karam dem jungen Scheich entgegen.` the lemma is `entgegenrufen`
   but only `rief` is bolded; the card contradicts its own answer. The particle
   is already persisted as a `compound:prt` token with a `Head` ordinal
   (ADR 0060) but is dropped from candidates and never recorded as a boldable
   span. ADR 0061 explicitly excluded learner-facing surfaces.

## Decision

1. **The article is a definite article or nothing.** `article_for` derives only
   genuine nominative definite articles; it must not accept a form that is also
   tagged plural or otherwise inflected. `nounArticle` adds a render-time
   whitelist (`der`/`die`/`das`; `il`/`lo`/`la`/`l'`) as defense-in-depth: an
   explicit `Article` value outside the set is ignored and the gender fallback
   applies. Derive-and-guard, both languages; the fix is a bug fix, not a new
   contract.
2. **Plural is its own note field.** A `Plural` field is added to the
   recognition-card contract, positioned immediately after `Lemma`, and the
   back template renders `Article Lemma (Pl. …)`. The plural is removed from
   `Gloss`. It renders whenever the index returns a non-empty plural, including
   when it equals the lemma (`der Gauner (Pl. Gauner)`); plural-only and
   non-noun lemmas carry an empty plural and self-suppress. Back-of-card only.
3. **Bold every component of the full lemma.** The exported `Text` bolds each
   span of the target: the observed verb form plus any `compound:prt` particle
   token whose head is that verb token, resolved at render from the dependency
   parse (no new cardexport column; the durable manifest carries the parse as a
   render input, per the 2026-09-15 amendment). Attached forms are unchanged;
   when no parse is available the observed form alone is bolded.
4. **Coverage is measured, not guessed.** Emit a structured `gloss_coverage`
   log line at deck freeze (per language/POS counts of selected lemmas with and
   without a gloss). Broadening dictionary sources or adding a consent-gated
   LLM gloss fallback (ADR 0007 territory) is decided from the measured gap, in
   a separate change; the dictionary stays the deterministic default.
5. **Both languages, resolved at freeze.** The `Plural` field, article guard,
   and multi-span bolding are language-agnostic and resolved when the
   prepared-deck manifest is frozen, carried on the frozen entries like gloss
   and morphology (ADR 0064).
6. **Roll forward.** Existing generated decks and imported notes are not
   rewritten; a fix reaches a learner only when a deck is re-prepared.

## Consequences

- The recognition-card note contract changes (new `Plural` field and a changed
  `Gloss` for nouns), requiring updated artifact fixtures and completeness
  tests, and shifting TSV columns for positional consumers. The TSV is not a
  served HTTP surface, so the compatibility cost is confined to tests.
- A malformed derived morphology value can no longer reach the card; the index
  fix removes the source of the defect and the guard contains its class.
- Separable verbs are presented consistently with their full-lemma identity.
- The frozen dependency parse is a durable manifest render input. The run is
  immutable and roll-forward, so a deck frozen before the fix keeps the
  observed-form-only bold until the deck is re-prepared.
- The article defect in already-frozen decks persists until re-preparation;
  this is the established roll-forward posture for generated artifacts.
- Dictionary source breadth and any LLM gloss fallback remain an explicit later
  decision driven by the coverage metric.

## Alternatives considered

- **Keep the plural in `Gloss`, only reposition it.** Rejected: it conflates
  morphology with the English sense set and leaves no clean seam for the
  template to own presentation, unlike `Article`.
- **Splice the plural into the `Lemma` text.** Rejected: mixes a derived field
  with a display field; a dedicated field mirrors `Article` and survives
  TSV/import round-trips.
- **Persist resolved bold spans on the entry.** Rejected: the persisted parse
  is already the render-time source for `TargetWord`, so deriving spans from it
  avoids a cardexport schema migration (ADR 0038) with no loss of determinism.
- **Fix the index only, without the render guard.** Rejected: derived data is
  untrusted input and the same malformed class can reappear from a future dump.

## References

- [Recognition-card sentence presentation](../features/recognition-card-sentence-presentation.md)
- [Dictionary gloss and morphology enrichment](../features/dictionary-gloss-enrichment.md)
- [German separable-verb lemmatization](../features/separable-verb-lemmatization.md)
- [ADR 0007: Enrichment providers, caching, and privacy](0007-enrichment-providers-caching-privacy.md)
- [ADR 0029: Recognition-card sentence presentation](0029-recognition-card-sentence-presentation.md)
- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](0060-persist-dependency-parses.md)
- [ADR 0061: German separable-verb lemmatization from dependency data](0061-german-separable-verb-lemmatization.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)

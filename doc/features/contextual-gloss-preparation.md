# Contextual gloss preparation

Status: **Core implemented** · Date: 2026-09-27 · Scope revised: 2026-09-28 · Decision:
[ADR 0079](../adr/0079-contextual-glosses-require-llm.md)

The FreeDict/PanLex source expansion in the original plan was canceled
(#1263–#1265). The implemented path uses the existing Wiktionary/Kaikki index
and sentence context. Explicit re-preparation of older Ready decks under these
rules remains a separate follow-up (#1266).

## Motivation and goal

The earlier pipeline froze a Wiktionary-derived sense list, optionally
let the LLM select senses, and used the LLM to write a fallback only when no
sense fit. This could leave an ambiguous list instead of a simple answer to
"what does this word mean *here*?" Prepare each new card with a short English
cue for **one contextual meaning**, alongside its existing whole-sentence
English translation. A few comma-separated near-synonyms are useful; unrelated
senses and gratuitous length are not. A longer phrase is acceptable where a
one-to-three-word cue would mislead, especially for function words or idioms.

## Scope and behavior

- The configured OpenAI-compatible LLM is required for every newly prepared
  card, whether deck preparation is requested directly or dispatched from
  Reading. Remove the per-submission consent/decline path; do not make merely
  starting or continuing Reading depend on successful preparation. If no
  provider is configured, report deck preparation as unavailable/retryable
  after configuration, not as a local-only success.
- Preserve the existing per-item call, which translates the complete
  representative sentence and target as needed. Ask it additionally for one
  contextual gloss, supporting evidence IDs (or context-only inference), and
  an explicit unresolved outcome when it cannot give a defensible meaning.
  Do not add a second gloss-only call. Continue sending only the target lemma,
  tested form, one sentence, and public lexical candidates; no whole Book,
  title, learner metadata, or reading history.
- Collect local English meaning evidence for `de`, `it`, and `el` from the
  Wiktionary/Kaikki index. Freeze a **bounded** candidate payload for the target
  lemma/POS, labeling a missing-POS match as weaker. Retain stable candidate
  IDs, source, origin, version, and sense text. Report how many candidates were
  omitted by the bound. Keep Wiktionary attribution in deck/export metadata,
  not on individual cards.
- Do not import FreeDict or PanLex as part of this feature. Overlap, mixed
  source terms, and provenance handling add complexity without an established
  improvement in contextual gloss quality. Reconsider another source only
  after a comparative quality sample and a separate rights review.
- Freeze candidate text, source/provenance/version, and selection identity on
  the deck specification before the LLM call. Keep results tied to that
  evidence and the prompt/provider version. A source refresh must not change a
  ready deck or a presentation-only rerender. Explicit re-preparation creates a
  new generation and may gather newer evidence; old generations stay available.
  Do not move raw dictionary datasets into application-state Postgres or add a
  runtime dictionary service merely to support this feature.
- Validate response shape, English text/markup/length bounds, and referenced
  candidate IDs; validation does not claim to verify meaning. The LLM may infer
  a gloss from sentence context when no supplied meaning fits, and must record
  that distinction. If meaning is still uncertain, omit the item rather than
  guessing. A malformed/invalid provider outcome or transport failure follows
  durable retry and fails the run without a dictionary-only fallback. A valid
  outcome with an unresolved gloss omits only that card; an all-omitted run
  fails rather than producing an empty deck.
- The preparation result reports source coverage, context-only inference,
  omitted candidates, and omitted cards with item-level reasons. Only included
  cards count as generated vocabulary. Omitting card material does not alter
  the current-reading snapshot, Reserved vocabulary, Known vocabulary, or
  reading completion; the learner may explicitly omit an identity at completion.
  Existing ready decks remain unchanged unless explicitly re-prepared.

## Non-goals

- Automatic migration of old cards or reinterpretation of an existing frozen
  specification during a presentation-only rerender.
- A learner opt-out, a separate gloss provider, or a deterministic dictionary
  card when the LLM is unavailable.
- Marking a context-inferred gloss on the Anki card or changing the card front,
  representative-sentence selection, or morphology/IPA presentation.
- Multi-source dictionary imports or a redistributed combined dataset.

## Acceptance criteria

- [x] All new preparation entry points use the LLM without a consent checkbox;
      Reading works when preparation cannot. Provider failures never yield a
      dictionary-only deck.
- [x] One call per item returns whole-sentence translation and one brief,
      context-specific gloss; one meaning rather than a concatenated sense list.
- [x] Frozen Wiktionary evidence includes source/version, candidate IDs and text;
      bounded retrieval can expose a relevant sense without implying additional
      independent source confirmation.
- [x] References to unknown candidate IDs and malformed/unsupported gloss
      responses are rejected; a valid unresolved item is omitted and reported;
      an all-omitted run fails. Context-only inferences and source coverage are
      inspectable in the deck result, not stamped onto cards.
- [x] Previous decks are unchanged, presentation rerenders read frozen data,
      and Wiktionary credits are present in deck/export metadata.
- [ ] Explicit re-preparation may use refreshed index/prompt versions without
      rewriting older generations (#1266).
- [ ] A reviewed sample across German, Italian, and Modern Greek compares old
      and new card glosses for contextual correctness, brevity, and unsupported
      claims. Include polysemy, function words, idioms, absent evidence, evidence
      at odds with sentence context, and invalid/ambiguous model responses;
      retain a rubric and cases for later prompt regressions.

## Sources and prior contracts

- [ADR 0064: Built-in dictionary enrichment provider](../adr/0064-dictionary-enrichment-provider.md)
- [ADR 0069: LLM sense selection and fallback gloss](../adr/0069-llm-sense-selection-and-fallback-gloss.md)
- [ADR 0071: Deck specification versus presentation](../adr/0071-decouple-deck-data-from-presentation.md)

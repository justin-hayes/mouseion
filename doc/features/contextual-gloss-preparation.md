# Contextual gloss preparation

Status: **Core implemented; citation-form Gloss pending** · Date: 2026-09-27 · Scope revised: 2026-10-29 · Decisions:
[ADR 0079](../adr/0079-contextual-glosses-require-llm.md),
[ADR 0112](../adr/0112-gloss-is-lemma-sense-in-citation-form.md)

The FreeDict/PanLex source expansion in the original plan was canceled
(#1263–#1265). The implemented path uses the existing Wiktionary/Kaikki index
and sentence context. Explicit re-preparation of older Ready decks under these
rules remains a separate follow-up (#1266).

The Gloss contract was revised by #2325 (ADR 0112) after prompt
`translation-v15-short-handles` produced definition paraphrases ("having a low
temperature" for *kalt*), sentence-inflected Glosses ("rested my wrist" for
*poggiare*), and Glosses of the wrong word ("to watch television" for *sehen* in
a *fernsehen* sentence). The citation-form rules, the `lemma_mismatch` outcome
and the review gate below are not yet implemented.

## Motivation and goal

The earlier pipeline froze a Wiktionary-derived sense list, optionally
let the LLM select senses, and used the LLM to write a fallback only when no
sense fit. This could leave an ambiguous list instead of a simple answer to
"what does this word mean *here*?" Prepare each new card with a short English
**Gloss**: the equivalent of the target lemma, in the sense its sentence uses,
in citation form, alongside the existing whole-sentence English translation.
A few comma-separated near-synonyms are useful; unrelated senses and
gratuitous length are not. A longer phrase is acceptable where a
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
- Write the Gloss in citation form, following the conventions of the lemma
  translation (the response's `translation` field, the lemma's plain English
  equivalent stored in the hidden `English` note field): verbs as "to …",
  nouns singular without an article, adjectives in their base form. The Gloss equals the lemma translation unless the sentence
  clearly uses a different or narrower sense (*palazzo* → "apartment
  building"). Never return a definition-style paraphrase when a short
  equivalent exists, and never carry the sentence's inflection, number or
  arguments; those stay in the highlighted English sentence. The meaning block
  still renders only the Gloss; the `English` field stays off the card back.
- Classify every unresolved outcome with an `unresolved_kind` of `ambiguous`
  or `lemma_mismatch`. The kind is empty when a Gloss is resolved and required
  when the Gloss is empty. Return `lemma_mismatch` when the sentence evidently
  uses a different lexeme than the supplied lemma, such as a separable verb
  whose particle analysis missed or an idiom headed elsewhere. Omit that card,
  record the omission with a reserved, domain-owned reason prefix, and count
  and label it separately in the deck result ("the sentence uses a different
  word than this lemma"). `ambiguous` omissions keep their current
  presentation. An unknown kind is an invalid response. No schema change.
- Give the revised contract a new prompt contract version, so cached results
  from `translation-v15-short-handles` are never reused. The Ancient Greek
  evidence-assisted pass is unchanged.
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
  that distinction. A response that supplies a resolved gloss and also a stray
  unresolved reason is accepted as resolved; the stray reason is discarded and
  never stored, cached, or rendered, and the same applies to a stray
  `unresolved_kind`. If meaning is still uncertain, omit the
  item rather than guessing. A malformed/invalid provider outcome or transport failure follows
  durable retry and fails the run without a dictionary-only fallback. A valid
  outcome with an unresolved gloss omits only that card; an all-omitted run
  fails rather than producing an empty deck.
- The preparation result reports source coverage, context-only inference,
  omitted candidates, and omitted cards with item-level reasons. Only included
  cards count as generated vocabulary. Omitting card material does not alter
  the current-reading snapshot, Reserved vocabulary, Known vocabulary, or
  reading completion; the learner may explicitly omit an identity at completion.
  Existing ready decks remain unchanged unless explicitly re-prepared.
- Review every new prompt contract version before rollout. An operator command,
  never run in CI, sends a frozen, versioned review case set through the real
  codec and the configured provider at the deployed model and reasoning
  setting, sending only what preparation sends. It writes a dated Markdown
  report under `doc/reviews/` with the date, model, reasoning setting, prompt
  contract version, and each case's request summary, Gloss, lemma translation,
  automatic checks ("Gloss equals translation", "outcome is
  `lemma_mismatch`") and a blank reviewer verdict. The seed cases come from
  v15: plain-equivalent cases (*kalt*, *Feuer*, *höflich*, *Finne*, *fähig*),
  sentence-material cases (*erschießen*, *poggiare*, *eröffnen*, *biegen*,
  *macchina*), protected wins (*palazzo*, *schießen*), the *sehen*/*fernsehen*
  mismatch, and function-word, idiom, Italian and Modern Greek spot checks.
  The rubric checks citation form, no definition paraphrase, no sentence
  inflection or arguments, mismatch detected, and protected win kept. The
  corpus-wide rate of Glosses containing the lemma translation is reported for
  information only.

## Non-goals

- Automatic migration of old cards or reinterpretation of an existing frozen
  specification during a presentation-only rerender.
- A learner opt-out, a separate gloss provider, or a deterministic dictionary
  card when the LLM is unavailable.
- Marking a context-inferred gloss on the Anki card or changing the card front,
  representative-sentence selection, or morphology/IPA presentation.
- Multi-source dictionary imports or a redistributed combined dataset.
- Fixing the German separable-particle misparse in analysis, or turning
  `lemma_mismatch` outcomes into learner lemma-review flags.
- Dropping the lemma `translation` field, showing the `English` field on the card
  back, or changing the configured model or its reasoning setting.

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
- [ ] Glosses are citation-form equivalents of the lemma's sense: verbs as
      "to …", nouns singular without an article, adjectives in base form; equal
      to the lemma translation unless the sense differs; no definition paraphrase;
      no sentence inflection, number or arguments (#2325).
- [ ] A `lemma_mismatch` response omits the card with the reserved reason, and
      the deck status view and JSON count and label it separately from
      `ambiguous` omissions; malformed `unresolved_kind` values are rejected
      and a stray kind beside a resolved Gloss is discarded (#2325).
- [ ] The prompt contract version is bumped from
      `translation-v15-short-handles`, and the encoded request states the
      citation-form rule, the gloss/translation relationship and the
      `lemma_mismatch` instruction (#2325).
- [ ] Review gate: an operator review command runs the frozen case set and
      rubric against the configured provider, outside CI, and its dated report
      is committed under `doc/reviews/`. The new prompt version ships only when
      it keeps every protected win (*palazzo* → "apartment building",
      *schießen* → "to gush out"), produces no definition paraphrase and no
      sentence-inflected Gloss, and returns `lemma_mismatch` for the
      *fernsehen* case. This replaces the original reviewed-sample criterion,
      which was never completed (#2325).

## Sources and prior contracts

- [ADR 0064: Built-in dictionary enrichment provider](../adr/0064-dictionary-enrichment-provider.md)
- [ADR 0069: LLM sense selection and fallback gloss](../adr/0069-llm-sense-selection-and-fallback-gloss.md)
- [ADR 0071: Deck specification versus presentation](../adr/0071-decouple-deck-data-from-presentation.md)

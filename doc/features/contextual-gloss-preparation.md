# Contextual gloss preparation from multiple lexical sources

Status: **Planned** · Date: 2026-09-27 · Decision:
[ADR 0079](../adr/0079-contextual-glosses-require-llm.md)

## Motivation and goal

The implemented pipeline freezes a Wiktionary-derived sense list, optionally
lets the LLM select senses, and uses the LLM to write a fallback only when no
sense fits. This can leave an ambiguous list instead of a simple answer to
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
- Collect local meaning evidence for `de`, `it`, and `el` to English from
  Wiktionary/Kaikki, FreeDict, and rights-eligible PanLex material. Search the
  available source entries before choosing a **bounded**, relevance-ranked
  payload: exact vocabulary identity first, then recognized forms/spellings or
  entries with missing POS, labeled as weaker matches. Include a source and
  stable entry ID, source/version, evidence kind (sense explanation versus
  lexical translation), and provenance for each candidate. Report how much
  was omitted by the bound; an apparent duplicate with shared ancestry is not
  independent corroboration. Do not treat translations as full definitions.
- FreeDict `deu-eng` is Ding-derived and has mixed GPL/AGPL terms; `ita-eng`
  and `ell-eng` are WikDict/DBnary derivatives of Wiktionary under CC BY-SA
  3.0. Retain per-source origins, versions, license metadata, and attribution.
  PanLex entries are eligible only when directly attested together under a
  source with affirmatively reviewed reuse rights. Exclude unknown-permission,
  permission-on-request, permission-only-to-PanLex, and unexplained license
  categories; preserve all contributing provenance. Do not generate indirect
  pivot-language translation chains as evidence. PanLex's database has
  CC BY-NC-SA 4.0 terms in addition to source-level rights.
- The build/import scripts may be published, but imported FreeDict and PanLex
  data stay in private local artifacts for this installation. Do not bundle the
  combined derived index in Git or public images, or share PanLex-bearing
  decks, without a separate redistribution review. Credit used lexical
  sources, licenses, and versions in deck/export metadata, even for paraphrased
  glosses; keep cards themselves free of provenance labels. Verify a current
  official PanLex snapshot or permitted retrieval channel before implementing
  its importer; do not claim PanLex coverage until ingestion is demonstrated.
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
- Publishing a redistributed multi-license dictionary dataset or claiming
  PanLex availability before a current authorized data route is established.

## Acceptance criteria

- [ ] All new preparation entry points use the LLM without a consent checkbox;
      Reading works when preparation cannot. Provider failures never yield a
      dictionary-only deck.
- [ ] One call per item returns whole-sentence translation and one brief,
      context-specific gloss; one meaning rather than a concatenated sense list.
- [ ] Frozen evidence includes source/version/ancestry, candidate IDs and text;
      bounded retrieval can recover a contextually relevant sense without
      presenting duplicates as independent confirmation.
- [ ] `deu-eng`, `ita-eng`, `ell-eng` FreeDict data is locally ingestible with
      license/provenance retained. Eligible PanLex entries are locally ingestible
      only once an authorized, current acquisition path is demonstrated;
      unsupported or unavailable PanLex does not prevent other cards.
- [ ] References to unknown candidate IDs and malformed/unsupported gloss
      responses are rejected; a valid unresolved item is omitted and reported;
      an all-omitted run fails. Context-only inferences and source coverage are
      inspectable in the deck result, not stamped onto cards.
- [ ] Previous decks are unchanged, presentation rerenders read frozen data,
      and explicit re-preparation may use refreshed source/prompt versions.
      Used-source credits are present in deck/export metadata.
- [ ] A reviewed sample across German, Italian, and Modern Greek compares old
      and new card glosses for contextual correctness, brevity, and unsupported
      claims. Include polysemy, function words, idioms, absent evidence, source
      disagreements, and invalid/ambiguous model responses; retain a rubric and
      cases for later prompt/source regressions.

## Sources and prior contracts

- [ADR 0064: Built-in dictionary enrichment provider](../adr/0064-dictionary-enrichment-provider.md)
- [ADR 0069: LLM sense selection and fallback gloss](../adr/0069-llm-sense-selection-and-fallback-gloss.md)
- [ADR 0071: Deck specification versus presentation](../adr/0071-decouple-deck-data-from-presentation.md)
- [FreeDict downloads](https://freedict.org/downloads/) and [per-dictionary licensing guidance](https://freedict.org/documentation/)
- FreeDict TEI source packages: [deu-eng 1.9-fd1](https://download.freedict.org/dictionaries/deu-eng/1.9-fd1/freedict-deu-eng-1.9-fd1.src.tar.xz), [ita-eng 2025.11.23](https://download.freedict.org/dictionaries/ita-eng/2025.11.23/freedict-ita-eng-2025.11.23.src.tar.xz), [ell-eng 2025.11.23](https://download.freedict.org/dictionaries/ell-eng/2025.11.23/freedict-ell-eng-2025.11.23.src.tar.xz)
- [PanLex database license](https://panlex.org/license) and [source permissions](https://dev.panlex.org/source-registration/)
- [PanLex snapshots](https://panlex.org/snapshot/) — current official download availability must be checked before implementation

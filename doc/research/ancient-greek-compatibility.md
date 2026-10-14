# Ancient Greek compatibility audit

- Date: 2026-10-14
- Ticket: justin-hayes/mouseion#2025 (child of #2021)
- Baseline: `b262ed0a`
- Scope: evidence only. This note records what the current code and upstream resources do for `grc` (Ancient Greek). It does not choose architecture, MVP, or implementation order.

## Summary

Mouseion has no Ancient Greek path today. `grc` is absent from the NLP model configuration, so no Stanza package is selected for it. Some generic code would still accept `grc` input and run it through German-, Italian-, or Modern-Greek-shaped logic. Several rules would silently change or reject Ancient Greek text. The changes that matter most touch core code and schema, not only adapters:

1. **Analysis gate ignores readiness.** `SupportsLanguage` checks features only, so an unconfigured or unready language can still be submitted for analysis.
2. **Sentence-boundary and length gates are language-blind.** Greek `;` and `·` are not terminal punctuation, and the 50-word and 400-character caps apply to every language.
3. **Surface normalization strips the koronis.** U+1FBD at a word edge is stripped, which changes surface identity.
4. **Folding changes identity.** Python `casefold()` expands iota-subscript letters (U+1FB3 → `αι`). Go `strings.EqualFold` uses simple folding, so the two sides can disagree on the same text.
5. **Corrections and exclusions do not survive re-analysis.** They are keyed by analysis run and offsets.

## Evidence

### 1. NLP configuration and profiles

- `nlp/src/mouseion_nlp/model_config.py:113-136`: only `de`, `el`, and `it` have `LanguageModelConfig` entries. `grc` falls back to defaults, and no package is named for it.
- `nlp/src/mouseion_nlp/producer.py:42`: `DEFAULT_NORMALIZATION_PROFILE = "unicode-casefold"`.
- `nlp/src/mouseion_nlp/producer.py:315-322`: `grc` receives profile `unicode-casefold` 1.2.0.
- `nlp/src/mouseion_nlp/producer.py:320` and `:347-348`: `_is_greek` is true only for `el`, so Greek-specific handling does not run for `grc`. Verify the exact branch before relying on it.
- Go side, `internal/canonicalization/canonicalization.go:119-137` and `unicode.go`: `modern-greek` profile applies to `el`. `grc` falls back to `LanguageNeutral`.

### 2. Capabilities and readiness

- `nlp/src/mouseion_nlp/server.py:18,32-47,80-92`: capabilities list only the warm languages configured through `MOUSEION_NLP_WARM_LANGUAGES`. `Analyze` does not check the requested language.
- `internal/analyzer/capabilities.go:40-48` (`SupportsLanguage`): checks language and feature only. It does not check `Ready`.
- `internal/analyzer/capabilities.go:50-58` (`ReadySupportedLanguages`): filters on `Ready`.
- `internal/cataloguesync/sync.go:1019-1034` (`syncLanguageScope`) and `:892-933` (`eligibleLanguages`, `languageTag`, `isEnglish`): the sync gate uses `Ready`. Catalogue sync is metadata-only (`reconcileSyncEntries`, `:1040-1080`), so it does not analyse text.
- `internal/analysis/jobs.go:223-330`: analysis submission does not check NLP capability.
- `internal/analysis/jobs.go:1240-1260` and `:1909-1922`: the depparse gate uses `SupportsLanguage`, which ignores `Ready` (the call is at `:1917`).

**Consequence:** a learner who moves a `grc` book to To Read will get submission. Whether analysis succeeds depends on the NLP service, not on Mouseion's own gate. This path has not been run end to end.

Entry points that reach analysis:
- `internal/webapp/library_handlers.go:301`
- `internal/webapp/reading_page_handlers.go:298-327`: To Read transition acquires the book, then calls `SubmitToReadBookAnalysis`.

### 3. Catalogue and OPDS

- `internal/opds/client.go:261-284` and `:348-365`: feed-language matching uses `NormalizeLanguage` on the entry title or display name. Whether an OPDS feed labels Ancient Greek with a tag that normalises to `grc` is unverified.

### 4. Tokenisation and surface cleaning

- `nlp/src/mouseion_nlp/producer.py:158-171` (`_clean_surface`): strips edge characters whose Unicode category starts with `P` or `S`, except `'` and `’`.
  - Probed with system `python3` 3.13.5 (`-I`): U+1FBD (KORONIS, category `Sk`) at a word edge is stripped.
  - U+0387 (ano teleia) and U+037E (Greek question mark) are category `Po`, so they are also stripped at edges.
- `nlp/src/mouseion_nlp/producer.py:257-262` (`_map_sentence`): offsets come from the Stanza word's `start_char`/`end_char`, falling back to the token's offsets.
  - Multi-word-token (MWT) behaviour is inferred from the fallback. Stanza `Word` objects may not carry `start_char`, so MWT words could share their token's offsets. **Not verified against a running pipeline.**
- Multi-word tokens: `ptnk` (Septuagint) has MWTs such as `κἀγώ`. `grc` `perseus` default has no `mwt` processor (see §7). Behaviour for an MWT-bearing package is untested.

### 5. Folding and matching

- Python: `casefold()` folds U+1FB3 (ᾳ) to `αι` and folds U+0345 to ι. Final sigma folds to σ.
  - Probed with system `python3` 3.13.5 (`-I`).
- Go, `internal/textmatch/match.go:43-74`: `FoldedWordSpan` uses `strings.EqualFold` (`:64`). `isWordRune` (`:71-74`) counts marks as word runes.
- `strings.EqualFold` uses simple case folding. It will not map one rune to two. A multi-character Python fold (ᾳ → αι) therefore cannot be matched by Go's `EqualFold` against the same text. **Parity was not tested.** Whether any path compares Python-folded strings with Go-folded strings is unverified.
- `isWordRune` treating marks as word runes is consistent with combining diacritics, but a standalone koronis (category `Sk`, not a mark) would be treated as a boundary, while a combining mark on a letter is not. This interaction is untested.

### 6. Sentence and card export

- `internal/cardexport/cardexport.go:370-371`: `maximumSentenceWords = 50`, `maximumSentenceCharacters = 400`.
- `internal/cardexport/cardexport.go:378-525`: text gates apply to every language.
- `internal/cardexport/cardexport.go:520`: sentence must end with `.`, `!`, or `?`. `text[len(text)-1:]` is a byte slice, so a multi-byte final character is never matched (it fails the gate, which is the desired outcome for Greek `;`, `·`, and U+037E).
- `internal/cardexport/cardexport.go:749-756`: capitalised or quoted sentence start required.
- `internal/cardexport/cardexport.go:467-484`: German GDEX overlay applies to `de` only.
- Impact: Ancient Greek punctuation (`;`, `·`, U+037E) does not terminate sentences, and long Greek sentences exceed the caps. Both gates would reject many natural sentences. **Not measured against a real corpus.**

### 7. Upstream Stanza 1.15.0 (`resources_1.15.0.json`, scratch copy)

- `grc` (Ancient_Greek):
  - default: `perseus_nocharlm` (tokenize, lemma, pos, depparse; no `mwt`).
  - `default_accurate`: perseus with pos and depparse `perseus_grc-pranaydeeps`.
  - Packages: `proiel`, `perseus`.
  - `ptnk` package: tokenize `ptnk_nocharlm`, mwt `ptnk`.
  - Pretrain `conll17` required for the pos and depparse variants.
- `nlp/pyproject.toml` pins `stanza==1.15.0`, `transformers==5.19.0`, `torch==2.14.1`.
- Stanza package selection for `grc` is not exercised anywhere in the repo.

### 8. Upstream UD treebanks (licences matter for any bundled data)

- UD `grc_perseus`: CC BY-NC-SA 2.5. 13,919 sentences, 202,989 tokens. Lemmas are converted from non-UD style. No PROPN tag. Elision marks use combining comma U+0313.
- UD `grc_proiel`: CC BY-NC-SA 3.0. 17,082 sentences, 214,005 tokens. NT plus Herodotus. Uses PROPN. Elision uses U+2019.
- UD `grc_ptnk`: CC BY-SA 4.0. Septuagint. 1,576 sentences, 39,499 tokens. Has MWTs. UD v2.13 or later.

Elision mark mismatch between treebanks (U+0313 vs U+2019) means the same elided form can have different surfaces depending on which model was trained on which treebank.

### 9. Persistence and data identity

- `internal/persistence/concordance.go:22-28`: form key uses `canonicalization.For`, falling back to `LanguageNeutral`. An Ancient Greek form key therefore uses neutral folding, not a Greek profile.
- `migrations/000023` (`occurrence_lemma_corrections`): keyed on `(owner, book, analysis_run, source_document_id, start_offset, end_offset)`. Corrections and exclusions are tied to one analysis run. Re-analysis under a new run loses them unless carried forward. **Carry-forward was not verified.**
- `migrations/000001` (`corpus_tokens`): language is free text. Head is stored as an ordinal. Nothing in the schema rejects `grc`.
- Latest migration is `000033`.

### 10. Dictionary and Known vocabulary

- `internal/dictionary/index.go:127-135` and `:365`: gender and article handling only for `el`. Dictionary build covers `de`, `it`, and `el` only (per docs). No Ancient Greek dictionary source is configured.
- Known identity: a Known vocabulary entry keyed on Python-folded lemma and Go-folded form may not match across the two sides (see §5). Generated cards do not mark vocabulary as known (ADR 0036), so the risk is wrong matching, not silent marking.

## Test coverage leads

- `internal/sentence_study_test.go:134` and `internal/sentence_study_captured_test.go:194` reference Greek MWT behaviour. Which language they exercise was not confirmed.
- No test found that runs `grc` through the producer, the cardexport gates, or the textmatch path.
- No test found for `SupportsLanguage` ignoring `Ready`.
- No test found for Python-vs-Go folding parity on iota-subscript forms.

## Unknowns

- Whether Stanza's `grc` `perseus` package loads and runs under the pinned versions in the NLP image.
- Whether Stanza `Word` objects expose `start_char` for MWT components.
- Whether the Dilemma Ancient Greek mode exists and what it does (not checked; optional).
- Whether OPDS catalogues label Ancient Greek with a tag that normalises to `grc`.
- Real-corpus sentence length and punctuation distribution against the cardexport caps.
- Whether `unicode-casefold` 1.2.0 is the right profile for `grc`, or whether a new profile is needed (decision deferred).
- Behaviour of carried-forward corrections across re-analysis.

## New questions (not fog)

1. Should `SupportsLanguage` (or its analysis-gate caller) check `Ready`? Currently it does not.
2. Which sentence-boundary and length rules apply to `grc`, and are the 50-word and 400-character caps intended for it?
3. Does Go's `EqualFold` need to match Python's multi-character `casefold` for iota-subscript forms?
4. Does a standalone koronis at a word edge need to survive `_clean_surface`?

## Fog (not yet answerable from this audit)

- Real behaviour of Stanza `grc` packages on Mouseion text.
- Whether Ancient Greek learners need `ptnk`-style MWT handling.
- Licence implications of bundling any UD treebank data (CC BY-NC-SA for `perseus` and `proiel`).

## Blockers

- None for this audit. The unverified items above need a running NLP service and a sample `grc` document to settle.

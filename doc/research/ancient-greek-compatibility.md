# Ancient Greek compatibility audit

- Date: 2026-10-14
- Ticket: justin-hayes/mouseion#2025 (child of #2021). Reopened by the parent; this revision supersedes the earlier resolution on #2025.
- Code baseline: `b262ed0a` (this commit changes only this document). All code links below use `b262ed0a`.
- Scope: evidence only. Records what the current code and upstream resources do for `grc` (Ancient Greek). It does not choose architecture, MVP, or implementation order, and it does not set up a `grc` NLP environment.

## How claims are labelled

- **Verified by test run:** a Go test ran at this revision and passed (listed in §10).
- **Probed:** a throwaway Go or Python snippet was run and its output recorded. Probe files were deleted afterwards.
- **Read:** established by reading the code cited. Not executed end to end.
- **Unknown:** not settled by this audit.

## Summary

Mouseion has no Ancient Greek path today. `grc` has no entry in the NLP model configuration, so no Stanza package is selected for it. Generic code would still accept `grc` input and run it through German-, Italian-, or Modern-Greek-shaped logic.

1. **Analysis submission does not consult readiness.** The only analysis-side capability gate, `requireDependencyParsing`, calls `SupportsLanguage`, which checks language and feature only. Whether readiness should gate submission is a decision question (§2).
2. **Sentence-boundary gate rejects Greek terminal punctuation by accident.** `hasCompleteBoundary` slices the last byte. Greek question mark U+037E is a question terminal and fails the gate. This is an artefact, not a designed rule (§6).
3. **Length gate is language-blind.** The 50-word and 400-character caps apply to every language (§6).
4. **Surface cleaning strips the koronis.** U+1FBD at a word edge is removed by both the Python and Go cleaners (probed) (§4).
5. **Folding can disagree across components.** Python `casefold()` expands iota-subscript letters (U+1FB3 → `αι`). Go `strings.EqualFold` does not. On the card-export path, a casefolded lemma is matched against raw sentence text, so bolding can miss (probed on the matcher; not tested end to end) (§5).
6. **Corrections and exclusions are scoped to one analysis run by design.** ADR 0081 says they do not carry to a subsequent analysis. This is intended contract, not a defect, unless a carry-forward path is later added (§4, §6).
7. **Dependency-based sentence scoring is German-only.** `scoreGermanGDEXQuality` returns unless the language is `de` (§7).
8. **Dictionary build, dictionary gender handling, and German display rules cover `de`/`it`/`el` only** (§8).

## 1. Book, source, and scope revisions

Identity is carried by these rows and columns (`migrations/000001_initialize.up.sql`):

- `source_materials` (`:412`): holds `current_content_revision_id` and `current_snapshot_id`.
- `analysis_runs` (`:193`): records the `content_revision_id` and `snapshot_id` that the run analysed.
- `book_current_analyses` (`:239`): the published pointer from a Book to its current run.
- `current_analysis_identity` view (`:433`): joins the above and is used by the Current Reading and across-Book counts.
- `corpora` (`:327`) and `corpus_sentences` / `corpus_tokens` (`:360`, `:380`): the analysed corpus for one run.

Publication requires that the run's revision and snapshot still match the source's current values (see §3). Scope (whole Book vs. selected units) is recorded by `source_material_units` (`:467`); how analysis selects main text from those units is in `identifyMainText`/`selectedSnapshotUnits` (`internal/analysis/jobs.go`, not re-read in this pass). **Unknown:** how a `grc` scope with no main text is handled.

## 2. Analysis submission and the depparse gate

- `internal/analyzer/capabilities.go:40-48` `SupportsLanguage(language, feature)`: checks language and feature only. It does not check `Ready`.
- `internal/analyzer/capabilities.go:50-58` `ReadySupportedLanguages`: filters on `Ready`.
- `internal/analysis/jobs.go:1256` calls `requireDependencyParsing` while building the snapshot plan (after the current-revision check). `:1909-1922` defines it and calls `SupportsLanguage(language, FeatureDepparse)` at `:1917`.
- The sole caller of `SupportsLanguage` in `internal/` and `cmd/` is that gate (grep, this pass).
- `ReadySupportedLanguages` is used by `internal/cataloguesync/sync.go:1026` and `:900`, which keep only ready languages for catalogue scope. Catalogue sync is metadata-only.
- Capabilities list only the warm languages configured through `MOUSEION_NLP_WARM_LANGUAGES` (`nlp/src/mouseion_nlp/server.py:18,32-47,80-92`). `Analyze` does not check the requested language.

**What this means:** the depparse gate rejects a language the service does not list with depparse. It does not reject a listed language that is not ready. Whether submission should also require readiness is a decision question (§12, Q1). The helper's name and signature are feature-scoped, so the helper itself reads as intentional. The gap is that no submission path checks readiness.

Entry points that reach analysis: `internal/webapp/library_handlers.go:301` and `internal/webapp/reading_page_handlers.go:298-327` (To Read transition, then `SubmitToReadBookAnalysis`).

**Unverified:** the end-to-end effect of `grc` submission has not been run. This needs a running NLP service with a `grc` package.

## 3. Analysis normalisation, publication, and corpus counts

**Normalisation (Python).**
- `nlp/src/mouseion_nlp/producer.py:42`: `DEFAULT_NORMALIZATION_PROFILE = "unicode-casefold"`.
- `nlp/src/mouseion_nlp/producer.py:351-353` `_canonical_lemma`: a non-German language is lemmatised with `unicodedata.normalize("NFC", ...).casefold()`. So `grc` lemmas are casefolded (Read; the German branch is the only exception).
- `nlp/src/mouseion_nlp/producer.py:158-171` `_clean_surface`: strips edge characters whose Unicode category starts with `P` or `S`, except `'` and `’`. Probed with system `python3` 3.13.5 (`-I`): U+1FBD (KORONIS, `Sk`) is stripped at an edge. U+0387 (ano teleia) and U+037E are category `Po` and are stripped at an edge too.
- Go's `internal/textmatch` `CleanLexicalSurface` strips edge `;` (probed) but not internal `· `.

**Token, sentence, and head persistence (Go).**
- `internal/analyzer/analyzer.go:45-49` (`Sentence`) and `:57-66` (`Token`: `Surface`, `RawLemma`, `CanonicalLemma`, `UPOS`, `Dependency`, `Head uint32`, `Morphology map[string]string`, `Location`). This is the domain adapter seam: the NLP service produces `analyzer.Result`, and the core consumes it.
- `proto/mouseion/v1/normalized_corpus.proto:61` `message Token` is the wire contract. Surface is documented as the form with edge punctuation removed.
- `internal/analysis/jobs.go:1759` `insertCorpusSentences`, `:1789-1835` `corpusTokenInserter`, `:1851-1862` `validateCorpusToken` (head must fall inside the sentence; a root must be self-headed; a dependency is required), `:1867` `corpusTokenRow`. Heads are per-sentence ordinals.
- `corpus_tokens.language` is free text (`migrations/000001` `:380`). Nothing in the schema rejects `grc`.

**Publication.**
- `internal/analysis/jobs.go:168` `noNewerAnalysisRun`.
- `:1644` `publishCompletedRun`, `:1713-1743` `publishEligibleAnalysis`. Eligibility: run completed; `source_materials.current_content_revision_id` and `current_snapshot_id` match the run; no newer run. Then `BuildVocabularyBrowseCountsTx` runs and `book_current_analyses` is upserted.

**Corpus count ownership.**
- `internal/selection/selection.go:203-210` `AnalyzableStatistics`: computes the coverage denominator with `aggregateTokens(corpus, cfg, nil)` (nil decisions). The denominator therefore ignores learner corrections and exclusions (read; the nil-decision call is at the `AnalyzableStatistics` body).
- `internal/analysis/jobs.go:654` and `:697` set `Statistics.AnalyzableTokenCount`. `:1416-1430` `insertCompletedCorpus` stores the counts in `corpora`.
- `internal/persistence/vocabulary_browse_counts.go:299` `BuildVocabularyBrowseCountsTx`: rebuilds the browse inventory for the Book at publication.

## 4. Effective identity, corrections, and exclusions

- `internal/selection/selection.go:229-252` `aggregateTokens`: for each token, `Effective` decides the identity and whether it counts.
- `internal/selection/selection.go:261-275` `Effective`: an excluded decision returns `counts=false`. A correction replaces the lemma. Identity is `(language, trimmed lemma, upper-cased UPOS)`.
- `internal/selection/selection.go:280-290` `OccurrenceEligible`: requires a `lexical.IsLemma` lemma (any Unicode letter), an allowed POS, and not `compound:prt`.
- `internal/lexical/lemma.go:8-15` `IsLemma`: any letter. Greek letters qualify.
- Occurrence identity keys on document and offsets (`occurrenceIdentity`, `selection.go` after `:290`). A potential collision exists if two tokens share offsets. **Unverified:** whether any analyzer output produces that.
- Persistence: `migrations/000023_occurrence_lemma_corrections.up.sql` (unique key `(owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset)`; FK to `corpora(owner_id, id, analysis_run_id)`). `migrations/000024_occurrence_vocabulary_exclusions.up.sql` adds `excluded` and a decision check.
- Single INSERT site for corrections: `sqlc/queries/selection.sql:111`. Grep for `occurrence_lemma_corrections` and `OccurrenceLemmaCorrection` found no carry-forward into a new `analysis_run_id`.
- **Corrections do not survive re-analysis, by contract.** ADR 0081 (`doc/adr/0081-learner-owned-occurrence-lemma-corrections.md:50`) says corrections "do not apply automatically to another occurrence, Book, learner, or subsequent analysis". This is intended, not a defect. (The earlier draft called it a defect; that was wrong.)
- Coverage propagation: `sqlc/queries/books.sql:161-176` `ListMyBooksCoverage` takes the denominator from `corpora.analyzable_token_count` (via `vocabulary_browse_count_readiness`, builder version 3) and the numerator from browse counts joined to Known. Exclusions reduce the numerator only; the denominator is unchanged because `AnalyzableStatistics` ignores decisions (ADR 0081 agrees).
- `internal/persistence/lemma_review.go`: flag and correction paths are keyed on the exact analysis run.
- `internal/persistence/my_books_coverage.go` and `vocabulary_browse_counts.go:169,357` consume corrections.

## 5. Folding and matching

- Python `casefold()` uses Unicode `CaseFolding.txt` status F entries. `CaseFolding.txt` line 909: `1FB3; F; 03B1 03B9` (ᾳ → αι). Also `0345; C; 03B9` and `03C2; C; 03C3` (final sigma → σ). Source: `https://www.unicode.org/Public/UCD/latest/ucd/CaseFolding.txt`, file date 2026-02-03, sha256 `a004797658a457bec4dc11683e39f69249ea3b595b752dbea6721c4c9f587b0d`, fetched 2026-10-14. (The earlier draft cited `SpecialCasing.txt`, which is not the casefold source.)
- Go `internal/textmatch/match.go:43-74` `FoldedWordSpan` uses `strings.EqualFold` (`:64`). `isWordRune` (`:71-74`) treats marks as word runes.
- Probed (Go, this revision): `FoldedWordSpan("ᾳ x","αι")` → ok=false; `EqualFold("ᾳ","αι")` → false; `FoldedWordSpan("ς x","σ")` and `FoldedWordSpan("Σ x","σ")` → ok=true; `FoldedWordSpan("ἀγαθός; x","ἀγαθός")` → ok=true.
- **Real cross-component path (read):** a casefolded lemma from Python reaches Go as `CanonicalLemma`, and card export matches it against raw sentence text. `internal/cardexport/cardexport.go:177-186` (`boldTargets`) calls `textmatch.CleanLexicalSurface` then `FoldedWordSpan(sentence, target)`. `cardexport.go:749-756` (`targetIndex`) does the same. `internal/enrichment/enrichment.go:429` also matches on this path.
- **Effect:** a lemma containing ι-subscript letters (ᾳ and similar, after casefolding) will not bold or match the surface form in the sentence. This is **probe-verified on the matcher**. The card-export path has not been run end to end, and the frequency of such lemmas in Ancient Greek data is **unknown**.

## 6. Sentence boundary, length, and card export

- `internal/cardexport/cardexport.go:370-371`: `maximumSentenceWords = 50`, `maximumSentenceCharacters = 400`.
- `cardexport.go:378-395` `ScoreSentenceQuality` (language-blind gates except as below).
- `cardexport.go:406-423` `scoreLengthQuality`: rejects fewer than `minimumSentenceWords` or invalid UTF-8, and rejects `sentenceTooLong`. Awards 30 points for usable length and 10 for 8–30 words.
- `cardexport.go:516-520` `hasCompleteBoundary`: `strings.ContainsAny(text[len(text)-1:], ".!?")`. This is a **byte** slice. ASCII terminals pass. Any sentence ending in a multi-byte character fails, including Greek question mark U+037E, which is a question terminal in Greek. U+0387 (ano teleia) also fails. The Greek behaviour here is an artefact of the byte slice. **It is not a designed rule, and the earlier claim that rejecting Greek `;`/`·`/U+037E is "desired" was wrong.** Whether Greek terminal punctuation should end a sentence is a decision question (§12, Q2).
- Capitalised or quoted sentence start: not verified in this pass. The earlier draft attributed it to `cardexport.go:749-756`, but those lines are `targetIndex`, which only does the folded match (§5). **Corrected.**
- Tests: `internal/cardexport/cardexport_test.go` and `internal/gdex/gdex_test.go` reference the gates (grep). No test uses `grc`.

## 7. Representative sentence scoring beyond length

- `cardexport.go:378-395`: beyond length, the scorer applies target presence (`scoreTargetQuality`, `:426`), boundary (`scoreBoundaryQuality`, `:445`), and structural noise (`scoreStructuralNoiseQuality`, `:455`). These are language-blind (read).
- `cardexport.go:467-472` `scoreGermanGDEXQuality`: returns unless `isGerman(language)` (`:499-502`, `de` only). Only then does it call `gdex.ScoreSentenceQuality` (`internal/gdex/gdex.go:41`), which uses dependency structure (subordinate-subtree checks, `gdex.go:~140-160`).
- The only caller of `gdex.ScoreSentenceQuality` is that German overlay (grep, this pass).
- **Consequence for `grc`:** the sentence score is length, target, boundary, and noise only. Dependency structure is not scored. This is **read**, and the gate was confirmed by the `de`-only `isGerman` check. No `grc` test exists.

## 8. Dictionary, enrichment, LLM consent, headword and forms, deck and APKG

**Dictionary build (NLP side).**
- `nlp/scripts/derive_dictionary_index.py:21`: `LANGUAGES = {"de", "it", "el"}`. Gate at `:411-412`.
- `Makefile:205` invokes it with `DICTIONARY_*` variables (`Makefile:22-29`).
- The dictionary build is not in Go; `internal/dictionary/` is the reader.

**Dictionary reader (Go).**
- `internal/dictionary/index.go:131`: `if language == "el"` branch (gender handling).
- `internal/dictionary/index.go:283-289`: German compound lookup (`de` only).
- `internal/dictionary/index.go:107` `Lookup`, `:147` `lookupEntryRows`, `:181` `finalizeLookupSenses`.
- `internal/dictionary/index_test.go` covers `de` and `el` (grep). No `grc` test.

**Enrichment consent.**
- `internal/enrichment/enrichment.go:292-294` `ExternalConfigured`: `ExternalEnabled && UserOptIn && translation.Enabled()`. All three are required. Callers use it to avoid no-op work.
- `enrichment.go:300-` `ExternalCacheKey` (immutable identity for the external candidate).
- `enrichment.go:319` `EnrichExternal`.
- `internal/enrichment/llm.go:28`: user consent is a separate `EnrichmentConfig` flag from the provider setting.
- Lexical context for `el` only: `internal/enrichment/lexical.go:323-331` `isContextWord` branches on `el`.

**Headword and forms.**
- `internal/lemmadisplay/display.go:12-19` `Format`: capitalises only for `de` NOUN. Other languages return the lemma unchanged.
- `internal/cardexport/cardexport.go:834-845` `nounArticleForms`: `de` and `it` tables (read; the 12 lines read showed no `el` or `grc` case).
- `internal/persistence/concordance.go:22-28`: form key uses `canonicalization.For`, falling back to `LanguageNeutral`. `grc` therefore gets neutral folding (read). `internal/canonicalization` `modern-greek` profile applies to `el` only.

**Render input, deck, APKG.**
- `internal/cardexport/render_input.go:12-26`: render input keyed by language, canonical lemma, and UPOS.
- `internal/cardexport/manifest.go:191, 239, 251, 277, 303-304, 452-480`: manifest entries are keyed by `(language, target_language, canonical_lemma, upos)`. Language is free text, so `grc` passes through.
- `internal/cardexport/rendering_version.go` and `internal/domain/domain.go:47` (`RenderInputVersion`); `internal/prepareddeck/durable.go:345-369` checks and recovers render-input versions.
- `internal/cardexport/apkg.go`: APKG writer. **Not read in this pass.**
- `internal/cardexport/templates/`: not read in this pass.
- Tests: `internal/cardexport/manifest_*_test.go`, `presentation_lifecycle_test.go`, `sentence_evidence_test.go`, `separable_target_test.go`, and `anki_import_acceptance_test.go` exist (`ls`). None uses `grc`.

## 9. Known vocabulary and reading state

- Known is written from the frozen snapshot, not the live selection (`sqlc/queries/current_reading.sql:250-271`; `ListKnownVocabularyForCurrentReadingSnapshot` at `:257`, `InsertCurrentReadingKnownVocabulary` at `:271`).
- `internal/knownvocab/jobs.go` and `sqlc/queries/core.sql` also insert known rows (`grep`).
- Generated cards do not mark vocabulary as known (ADR 0036).

## 10. Current Reading freeze, exact analysis, and transitions

- `internal/persistence/current_reading_lifecycle.go:46-99` `createCurrentReadingSnapshot`: creates the snapshot with the exact analysis run, content revision, content snapshot, and corpus identity. If the Book has current lemma corrections (`HasCurrentLemmaCorrections`, `:55`), it re-projects candidates from analysis facts with corrections (`correctedCurrentReadingCandidates`, `:265`). Otherwise it uses the precomputed snapshot candidates.
- `:100` `eligibleCurrentReadingCandidates`, `:120` `requestedCurrentReadingIdentities`, `:178` `currentAcrossBookCounts`. The across-Book query (`:189-196`) reads `current_analysis_identity` joined to `books`, filtered on the chosen language state. **Not read in full:** the remaining predicate, and whether the current Book is excluded from its own count (the function takes an `excludeBook` argument; the call site was not checked).
- Transitions and locks: `sqlc/queries/current_reading.sql:121-161` (lifecycle read and `LockCurrentReading*`), `:162-174` (insert, change Book, delete). Stop-and-restart is the only way to correct a reading after it starts (ADR 0081).
- Tests: not checked in this pass. Integration tests need Postgres, and none was run.

## 11. Representative selection rule (ADR 0090)

- `internal/selection/eligibility.go:5` `DefaultRecurringMinOccurrences = 3`; `:10-12` `DefaultAcrossBooksMinOccurrences = 15` (provisional, issue #1803).
- `:41-53` `AllowsBookDeckCandidate`: `inBook >= 3 || (inBook >= 1 && acrossBooks >= 15)`. This matches ADR 0090 (`doc/adr/0090-library-total-book-vocabulary-selection.md`).
- ADR 0090 scopes the rule to the owner's currently analysed same-language Books. Language partitioning is enforced by the `language` argument to `currentAcrossBookCounts` and by `b.language_state` (`current_reading_lifecycle.go:189-196`). **Unknown:** whether the current Book is counted in its own across-Book total (see §10).
- Existing test: `internal/selection/selection_test.go:179` `TestAnalyzableStatisticsUsesSelectionFiltersBeforeLearnerExclusions`, `:194` `TestItalianFixtureFiltersAndAggregatesVocabulary`, `:233` `TestGreekFixtureFiltersAndAggregatesVocabulary` (uses language `el`, Modern Greek, not `grc`). **Verified by test run** (`go test ./internal/selection/`).

## 12. Upstream resources

**Stanza**
- Pinned version: `nlp/pyproject.toml` pins `stanza==1.15.0`, `transformers==5.19.0`, `torch==2.14.1`.
- Tag `v1.15.0` commit `c9a2158a87502dd63f757ab1aa933230b0acbce7` (from `git ls-remote` against `https://github.com/stanfordnlp/stanza`).
- Resources: `https://raw.githubusercontent.com/stanfordnlp/stanza-resources/main/resources_1.15.0.json` (mutable branch URL; identified by sha256 `ae9741f2ac99fb004461dc764827b62786a438cdf651474e81778df673dd4463`, fetched 2026-10-14).
  - `grc` default: `perseus_nocharlm` (tokenize, lemma, pos, depparse; no `mwt`).
  - `default_accurate`: perseus with `perseus_grc-pranaydeeps` for pos and depparse.
  - Packages `proiel`, `perseus`, and `ptnk` exist. `ptnk` has tokenize `ptnk_nocharlm` and `mwt: ptnk`.
  - Pretrain `conll17` is required for the pos and depparse variants (from resources JSON, read in the earlier pass).
- Word offsets for MWT components: `https://github.com/stanfordnlp/stanza/blob/c9a2158a87502dd63f757ab1aa933230b0acbce7/stanza/models/common/doc.py#L728-L736`. sha256 of the fetched file `02d5b345085e55a6d90dbacd073fe87a4070e182c7682f26184299dc5957da05`.
  - `:728`: a single-word token copies the token's offsets.
  - `:730-736`: for a multi-word token, component offsets are found by regex match of the components against the token text (`search_string`). If the match succeeds, each word gets its own `start_char`/`end_char`.
  - **Correction:** the earlier draft said MWT words "could share their token's offsets" without checking. The code shows per-word offsets when the regex matches. The shared-offset fallback applies only when the regex fails. That no-match branch was **not read in full**.
  - Crasis forms such as `κἀγώ` are a plausible no-match case (the concatenation may not reproduce the token text). **Unverified** against a running pipeline.

**UD treebanks** (fetched 2026-10-14 from `https://universaldependencies.org/treebanks/<name>/index.html`):

| Treebank | Licence | Sentences | Tokens | Page sha256 |
|---|---|---|---|---|
| grc_perseus | CC BY-NC-SA 2.5 | 13,919 | 202,989 | `b5f80f0aa16435497bfc67bb7fab08f6b1104b06be3e9dda4492b2dd867447e5` |
| grc_proiel | CC BY-NC-SA 3.0 | 17,082 | 214,005 | `8b03ad239ef3893b73736300b4dd4bc7d6cc1d84157a6dd0f0b994bff1f7dfe6` |
| grc_ptnk | CC BY-SA 4.0 | 1,576 | 39,499 (39,509 syntactic words, MWT signal) | `ba1c13831d5f4edd50c946ecf6d6b13d83b03d985322879b82c145245853de59` |

- Source for stats: the UD treebank index pages above. Read on 2026-10-14. Only the licence, sentence, and token counts were re-checked in this pass.
- Carried from the earlier draft, **not re-verified in this pass:** perseus lemmas converted from non-UD style, no PROPN tag; proiel uses PROPN and U+2019 elision; ptnk is UD v2.13 or later; perseus elision uses combining comma U+0313. Treat these as unverified until checked against the pages.
- Licence implication: `perseus` and `proiel` are CC BY-NC-SA. Bundling their data would carry non-commercial and share-alike terms (decision question, §13).

## 13. Reusable, conditional, incompatible, unknown

| Subsystem | Reusable | Conditional | Incompatible or wrong for `grc` | Unknown |
|---|---|---|---|---|
| NLP model config and profile (§3) | Stanza pipeline loading, `unicode-casefold` profile | Package choice (`perseus` vs `proiel` vs `ptnk`, licence and MWT) | No `grc` entry in `model_config.py`; no Greek profile | Whether `perseus` loads under pinned versions |
| Capabilities and readiness (§2) | `ReadySupportedLanguages`, catalogue sync gate | Submission gate (feature-only today) | No readiness check on submission | Intended readiness policy |
| Surface cleaning (§3, §5) | Generic punctuation stripping | Koronis at an edge (U+1FBD) | Strips U+1FBD and U+0387 at edges | Whether koronis must survive |
| Folding and matching (§5) | `FoldedWordSpan` for ordinary letters and final sigma | Iota-subscript lemmas | Casefold (Python) vs EqualFold (Go) disagree on U+1FB3 | Frequency in real `grc` data |
| Token and head persistence (§3) | `corpus_tokens`, `validateCorpusToken`, the proto `Token` | MWT word-to-token mapping | Heads are per-sentence; MWT behaviour untested | MWT no-match fallback |
| Publication and counts (§3) | `publishEligibleAnalysis`, `insertCompletedCorpus`, browse counts | None identified | None identified | Behaviour under a `grc` run |
| Corrections and exclusions (§4) | Schema, `lemma_review.go`, coverage join | Carry-forward (not built; ADR 0081 says no) | Corrections do not follow re-analysis (intended) | Whether users expect carry-forward |
| Current Reading freeze (§10) | Snapshot creation, lock semantics | Language-partitioned across-Book count | None identified | Own-Book inclusion in across count |
| Representative selection (§11) | Frequency rule and tests | Thresholds (provisional, #1803) | None identified | Thresholds for `grc` corpora |
| Sentence gates (§6) | Length gate, target and noise scoring | Greek terminal punctuation (byte-slice artefact) | `;`/U+037E and `·`/U+0387 fail; 50-word cap | Real sentence-length distribution |
| Dependency sentence scoring (§7) | Nothing for `grc` | German-only overlay | `grc` never scored on dependency structure | Whether `grc` needs it |
| Dictionary (§8) | Reader code (`index.go`) | `el` gender branch as a model | Build covers `de`, `it`, `el` only | Whether any `grc` dictionary source exists |
| Enrichment and LLM consent (§8) | `ExternalConfigured`, cache key, consent flag | Lexical context (`el` only) | None identified | Quality of LLM output for `grc` |
| Headword and forms (§8) | Generic lemma handling | `de` capitalisation and `de`/`it` articles | No `grc` article or display rules | Whether `grc` needs display rules |
| Render input, manifest, deck (§8) | Language-keyed manifest and render input | None identified | None identified | APKG and templates not read |
| Known vocabulary (§9) | Frozen-snapshot graduation | None identified | None identified | None identified |

## 14. Reusable tests and what was run

- **Run (`go test -count=1`, this revision, passed):** `./internal/selection/`, `./internal/textmatch/`, `./internal/gdex/`, `./internal/cardexport/`, `./internal/analysisinsights/`, `./internal/dictionary/`, `./internal/lexical/`.
- Not run: integration tests (need Postgres or Testcontainers), NLP `pytest` (needs the `.venv`), and any `grc` NLP run.
- Reusable tests: `selection_test.go` (`:63`, `:114`, `:179`, `:194`, `:233`); `textmatch/match_test.go` (`:11`, `:30`, `:48`); `cardexport_test.go`, `dictionary_freeze_test.go`, `manifest_*_test.go`, `presentation_lifecycle_test.go`.
- None of these exercises `grc`. The Greek fixture is `el`.
- **Corrected:** `internal/webapp/sentence_study_captured_test.go` (`:13`, `:50`, `:194`) captures Modern Greek (`gdt`) output, not `grc`.

## 15. Adapter-only versus core changes

- **Adapter-only (in principle):** adding a Stanza package and a `model_config.py` entry for `grc`; adding a dictionary source behind `derive_dictionary_index.py`.
- **Needs core changes:** readiness gating on submission (`jobs.go` and `capabilities.go`); a Greek profile for `_canonical_lemma` if casefold is not wanted; the sentence boundary rule (`hasCompleteBoundary`), which is core and not language-keyed; the byte-slice terminal check.
- **Needs schema or interface changes:** none identified for `corpus_tokens` (free-text language). Changing the match semantics (a folding change in `textmatch`) touches every language.

## 16. Verified, corrected, and unknown (this pass)

**Corrected from the earlier draft:**
- `SupportsLanguage` is feature-only by design (helper name and signature). The gap is that no submission path checks readiness.
- Corrections not surviving reanalysis is intended (ADR 0081:50). It is not a defect.
- The Greek punctuation gate rejection is an artefact of the byte slice. It is not a desired rule. The earlier draft's "desired outcome" wording was wrong.
- The Python fold source is `CaseFolding.txt` (line 909), not `SpecialCasing.txt`.
- Python `_canonical_lemma` casefolds non-German lemmas (`producer.py:351-353`). The earlier `_is_greek` claim was not the path that matters.
- Stanza MWT offsets are per-word when the regex matches (`doc.py:728-736`), not shared by default.
- Greek sentence-study captures are Modern Greek, not `grc`.
- Python casefold vs Go `EqualFold` **is** traced through a real path (card export bolding). It is probe-verified on the matcher, not tested end to end.

**Unknown:**
- Whether a `grc` analysis submits and completes against a running NLP service.
- MWT no-match fallback in Stanza (`doc.py` after `:736`).
- Whether the across-Book count includes the current Book.
- APKG writer and templates (`apkg.go`, `templates/`).
- Real sentence length and punctuation distribution for `grc`.
- Frequency of iota-subscript lemmas in `grc` data.
- Whether an OPDS catalogue labels Ancient Greek with a tag that normalises to `grc` (`internal/opds/client.go:261-284`, `:348-365`).

## 17. New decision questions

1. Should analysis submission require readiness, or is feature support enough? Today only feature support is checked.
2. Should Greek terminal punctuation (U+037E, U+0387) end a sentence? The current rejection is an accident of the byte slice.
3. Should Python casefold and Go matching agree on iota-subscript forms (ᾳ vs αι), or should one side change?
4. Should a standalone koronis survive `_clean_surface` at a word edge?
5. Should `grc` get dependency-structure sentence scoring, or is length-plus-target enough?
6. Are `perseus` and `proiel` CC BY-NC-SA terms acceptable if any data is bundled? If not, which package is the baseline?

## 18. Fog

- Real behaviour of `grc` Stanza packages on Mouseion text, under the pinned versions.
- Whether Ancient Greek learners need `ptnk`-style MWT handling.
- Whether corrections should be carried forward (a product choice that ADR 0081 currently rules out).

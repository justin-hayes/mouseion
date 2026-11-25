# Where the code treats `grc` as meaning "classical" (Latin generalisation cost)

**Research date:** 2026-11-25

**Scope:** Every site where the code treats `grc` (or Ancient Greek) as meaning
"classical", and which of them a first Latin slice would have to touch. Slice 1
is vocabulary and cards, with no Constructions. Assessment only: no code
changed. The primary source is this repository at `origin/main`, commit
`a231721556d173dd6f4ece3fe10bd41d6bea04a8` (a2317215). Every `path:line`
below is at that commit. The inventory comes from a repo-wide search for
`"grc"`, `IsAncientGreek`, `GreekTitle`/`greekTitle`,
`ConcordanceConstructionLanguage`, `classical.Language`,
`ancientgreek.Language` and `ancientgreek.` over Go, templ, SQL, JSON, Python
and Playwright files. Test files are cited only where they pin the assumption.

## Answer

- **The `grc` assumption is wide but mostly shallow.** Most sites are one
  constant (`classical.Language`), one string compare or one hard-coded tag.
  Four sites need real design, not renaming:
  1. the OGA adapter, Greek-specific top to bottom (POS mapping, `morpheus:N`
     discriminator, normalization profile `ancient-greek`);
  2. the `IsAncientGreek` card gate, which has no Latin analogue until the card
     policy is decided;
  3. the one-language Classical catalog (`classical.Language` is a single
     constant, and `Definition.Language` is validated against it);
  4. the normalization profile registry, which has no `la` profile.
- **A Latin with no NLP capability is already tolerated.** Study languages come
  from the learner's Books and vocabulary, not from a capabilities allowlist.
  Capabilities supply only display names for catalogue language options and
  analysis-update targets. Greek already works this way.
- **Constructions are not in the way.** The `ConcordanceConstructionLanguage`
  gate fails closed for any language other than `grc`. Latin gets no
  Construction Concordance without any change, which is the intended slice-1
  state.
- **Relative cost (slice 1), largest first:** preparation and import
  (adapter, profile, ImportError coupling) about 40%; cards (gate and
  dictionary and passage-context paths, provider assessment) about 25%;
  catalog and manifest about 20%; UI about 10%; Concordance and Constructions
  about 5% (leave alone). This is a judgment from reading the code, not a
  measurement.
- **No migration is needed for slice 1.** I found no CHECK constraint, enum or
  SQL text pinning `grc`, except one query (below). `greek_title` is the only
  Greek-named column.

## 1. Catalog and manifest

| Site | Assumes | Slice 1 |
|---|---|---|
| `internal/classical/types.go:14-19` | `Language = "grc"` is "the only language the Classical catalog currently ships"; `LanguageDisplayName = "Ancient Greek"` | **Generalise.** One constant becomes a per-Definition language and display name. This is the root of the catalog fan-out. |
| `internal/classical/types.go:88,133-134` | `Definition.Language` exists, but validation rejects any value other than the constant | **Generalise.** Accept a small set of classical languages. |
| `internal/classical/types.go:85,248,436` | `greekTitle` JSON field on definition, work override, work spec | **Generalise (rename or add a neutral original-script title).** For Latin it would be empty, since the Latin title is the same script as the English one. Keeping the field is cheap; it is omitted when empty. |
| `internal/classical/catalog.go:99,145,230` | `GreekTitle` takes part in the change-detection comparison and the catalogue read model | **Leave alone** if the field stays; touch only if renamed. |
| `internal/classical/add.go:17` | `ErrLegacyRows` message built from `Language` | **Generalise** with the constant. |
| `internal/classical/types.go:15` consumers in `internal/classicalmanifest/generate.go:517` | The manifest generator stamps `Language: classical.Language` on every definition | **Generalise.** The generator also calls `ancientgreek.ProposeUnits`, `CheckUnitCoverage`, `SummarizeUnit`, `InspectTranslation` (`generate.go:360-444,535,634`) and names the interpretation version via `ancientgreek` (`:916`). A Latin source needs its own unit-proposal and coverage adapter, or the generator must be given one per source format. |
| `internal/classicalmanifest/generate.go:514,655,682` | `GreekTitle` merged from unit override, work override, previous | **Leave alone** (empty for Latin). |
| `internal/classical/layers.json`, `internal/classicalmanifest/testdata/layers.json:15` | Layer pins are keyed by layer, not language. Source layer, LSJ and Brownson are Greek content; each layer carries its own ADR 0111 class and deployer-register row | **Generalise by adding rows.** Latin layers are new entries, not a code change. The Latin text and annotation layers must be classified per ADR 0111 from the licence instrument in force (that is the corpus ticket's work, not this one's). |
| `internal/classical/types.go:22` (`SourceFormatOGACoNLLU`, "the single-file oga_src CoNLL-U adapter") | One source-format constant | **Generalise.** Add a Latin source format. The switch in `internal/analysis/classical_preparation.go:619-625` selects the adapter by format, so adding a case needs no job reshaping (see section 2). |
| `internal/persistence/classical_read.go:34,72`, `classical_add.go:59,76-78,151`, `classical_manifest.go:238-369` | Reads and Adds stamp `classical.Language` and `LanguageDisplayName` onto the languages row and the Book's `language_tag`; the Greek title is written to `works.greek_title` | **Generalise** (read the language from the Definition). Mechanical. |
| `sqlc/queries/classical.sql:75`, `books.sql:82`; migrations `000048` (`greek_title` column) | Column named `greek_title`, default `''` | **Leave alone.** No constraint pins `grc`. A rename would be a migration and is not needed. |
| `internal/storecontract/classical.go:53,86-154`, `internal/fixtures/classical.go:32,86,111`, `classical_add.go:71,112,124`, `reading_assertion.go:171` | Contract and fixture setup use `classical.Language`; the fixture server registers one classical language | **Generalise** once, to add a Latin fixture work. |

## 2. Preparation and import

| Site | Assumes | Slice 1 |
|---|---|---|
| `internal/analysis/classical_preparation.go:619-625` | `parseClassicalSource` switches on format; only `oga-conllu` is known, default is `CodeUnknownSourceFormat`. Returns `*ancientgreek.ParsedWork` | **Generalise.** The switch is the designed seam (ADR 0116). The return type is Greek-named but structurally generic; a Latin adapter must emit the same `ParsedWork`. Renaming the package is optional. |
| `internal/analysis/classical_preparation.go:313-323` | `classicalPreparationFailureCode` classifies `*ancientgreek.ImportError` codes into stable Book preparation codes | **Generalise.** The generic worker depends on the Greek package's error type. Cheapest route: a Latin adapter reuses `ancientgreek.ImportError` and its codes. Cleaner: move `ImportError` and codes to a neutral package. Either way the code set must stay stable. |
| `internal/analysis/classical_preparation.go:534-595,650-657` | Fetch, pin-mismatch and oversize failures are built as `ancientgreek.ImportError` | **Same as above.** |
| `internal/analysis/classical_preparation.go:265,458,633`; `classical_conversion.go:89,245`; `internal/webapp/jobs_handlers.go:31`, `catalog_handlers.go:312` | `ListLegacyLanguageRows(classical.Language)` and the target definition's `Language: classical.Language` | **Generalise** (take it from the target). |
| `internal/analysis/classical_translation.go:102-109` | Reference translation parsed by `ancientgreek.ParseReferenceTranslation` (the Brownson format) | **Left alone for slice 1 unless Latin gets passage context.** That is a "not yet specified" item on the map. |
| `internal/analysis/greek_import.go` (whole file, `GreekImportWriter`) | The writer persists an imported analysis as an ordinary analysis; its request type is `ancientgreek.ImportWriteRequest` and its media type `ancientgreek.SourceMediaType` | **Generalise by reuse.** The writer is generic in behaviour; only the names are Greek. |
| `internal/ancientgreek/ogasrc.go:314` | Imported result carries `NormalizationProfile{Name: "ancient-greek", Version: "1"}` | **Change.** A Latin result needs its own profile. |
| `internal/ancientgreek/ogasrc.go:522-552` (`nativePOSValues`, `mapUPOS`, scheme `oga-agdt-upos/5`) | Native AGDT POS to UPOS mapping | **Change or add.** Latin treebanks use other tag sets (for example UD, or AGDT-Latin). That adapter is the heart of the work. |
| `internal/ancientgreek/ogasrc.go:611-665` | Morphology mapping follows "UD Ancient Greek Perseus r2.19"; the lemma number becomes a `morpheus:N` discriminator | **Change.** Latin lemmatisation conventions differ (homograph numbering, i/j and u/v). |
| `internal/ancientgreek/ogasrc.go:429` | `SearchKey` derived with the Greek profile | **Generalise.** |
| `internal/ancientgreek/ancientgreek.go:9` | A second `Language = "grc"` constant | **Generalise** (the adapter takes the language). |
| `internal/canonicalization/ancient_greek.go:12-60` | Profile `ancient-greek/1`: NFC, lowercase, `ς`→`σ`, diacritics kept | **Add a `la` profile.** `internal/canonicalization/canonicalization.go:100-156` registers German, Modern Greek, Ancient Greek and French. Any other language falls back to a generic profile, so Latin would work but with no i/j or u/v policy. The lemma-identity decision belongs to a separate ticket. |
| `internal/canonicalization/unicode.go:54-65` (`SearchKey`) | Always applies the Greek profile, then strips combining marks | **Leave alone.** It is display search for titles and authors, harmless on Latin (macrons are stripped, which is wanted). |
| Migrations | No CHECK or enum names `grc` | **Nothing to do.** |

## 3. Cards

| Site | Assumes | Slice 1 |
|---|---|---|
| `internal/sentencequality/sentencequality.go:264-267` | `IsAncientGreek` compares the normalized tag with `"grc"` | **Replace with a policy lookup**, not a rename. The policy is a map from language to a card policy. This is the single most important decision site. |
| `internal/sentencequality/sentencequality.go:200-207` | `hasCompleteBoundary`: for Greek only the terminal boundary is required (lowercase sentence starts; terminators `. ; U+037E · U+0387`) | **Generalise.** Latin editions vary on capitalisation. Needs a Latin boundary rule or the default rule. |
| `internal/cardexport/cardexport.go:504-507` | Wrapper re-exporting `IsAncientGreek` | **Follows the gate.** |
| `internal/cardexport/dictionary_entry.go:20` | `resolveAncientGreekLSJ` returns early unless Ancient Greek; needs `enrichment.AncientGreekLSJProvider` | **Leave alone for slice 1 or add a Latin dictionary provider.** With no Latin dictionary, Latin cards show no entry (it fails closed). Whether it should be a deck failure is the card-policy question. |
| `internal/cardexport/presentation.go:369` | `applyPassageContext` freezes Brownson passages only for Ancient Greek | **Leave alone** until a Latin reference translation exists. |
| `internal/cardexport/cardexport.go:1733-1760,1803-1810` | Attribution notices: `deckIsAncientGreek` adds the Greek source notice; `deckIsNLPAnalysed` treats every non-Greek language as NLP-analysed, so it adds the analysis-models notice | **Change.** Latin without an analyser would wrongly receive the analysis-models notice and no source notice. The rule must become "imported vs NLP-analysed", not "Greek vs rest". |
| `internal/prepareddeck/planning.go:321,487` | `sourceEvidence = cardexport.IsAncientGreek(language)`; a deck with no provider configured is plannable only for a source-evidence deck; otherwise `errTranslationProviderRequired` | **Change.** A Latin deck with no provider would fail with "translation provider required" until Latin is declared source-evidence. |
| `internal/enrichment/provider_assessment.go:75,103` | `ProviderAssessment`s keyed by `Language: "grc"` (the evidence-assisted pass and the LSJ sense summary), recording Justin's reviewed personal-use disposition | **Add.** Latin needs its own reviewed assessment before any payload is sent. This is a product decision, not code. |
| `internal/dictionary/greek_index.go:15`, `index.go:42`; `nlp/scripts/derive_dictionary_index.py:30,1367`; `internal/persistence/greek_dictionary_coverage.go:28` (SQL literal `WHERE language = 'grc'`); `cmd/greekhitrate/main.go:125` | The dictionary index has a Greek-only entries table, a Greek-only derive path from the LSJ TEI files and a Greek-only coverage query | **Leave alone for slice 1 unless the dictionary ticket picks a source.** A Latin dictionary needs a new index format version, derive path and coverage query. |
| `internal/enrichment/ancient_greek_lsj.go`, `internal/dictionary/lsj_index.go` | LSJ is the only classical dictionary | **Leave alone.** |

## 4. Concordance and Constructions

| Site | Assumes | Slice 1 |
|---|---|---|
| `internal/domain/domain.go:516-518` | `ConcordanceConstructionLanguage = "grc"`: "the one study language whose analysis has" Constructions | **Leave alone.** |
| `internal/webapp/concordance_handlers.go:159`, `internal/persistence/concordance_constructions.go:110` | A Construction lookup in any other language gets the safe restart or returns nothing | **Leave alone.** Latin fails closed. |
| `internal/webapp/concordance_handlers.go:259` | `constructionLemmaKey` uses the `grc` profile | **Leave alone.** |
| `internal/webapp/sentence_study_constructions.go:152` | Construction entries are stamped with the `grc` language | **Leave alone.** |
| `internal/constructions/notes.go:21`; `internal/constructions/releasecheck/` | Concept notes are per language (ADR 0124), and `Language: "grc"` is recorded on the notes. The release check parses with `ancientgreek.ParseOgaSource` (`detect.go:68`) | **Leave alone.** ADR 0124 reserves a Latin record and its own notes "without changing the shape". |
| `internal/constructions/…` detection | Detection is conditional on the AGDT scheme (ADR 0123, 0125) | **Leave alone** (out of slice 1). |

## 5. UI

| Site | Assumes | Slice 1 |
|---|---|---|
| `internal/webapp/active_language.go:80-82` | Label switch: `de`, `it`, `el`, then `grc` uses the catalog's curated display name; any other tag falls through to `DisplayName + " (" + code + ")"` | **Generalise (small).** Latin would render "Latin (la)" without a case. Whether a code suffix is wanted is a presentation choice. |
| `internal/webapp/catalog_browse.go:175,385` | The Classical section shows when the filter is empty or `== classical.Language`; the language options add `grc` only | **Generalise.** A catalogue filter on `la` would currently show no Classical section. This is the main UI change. |
| `internal/webapp/catalog_handlers.go:532,562` | "…in Ancient Greek. Your study language is unchanged." and `classicalAddNamesOtherLanguage` compare against `classical.Language` | **Generalise** (per-Work language). |
| `internal/webapp/classical_catalog.templ:38-42,161` | Work page prints the Greek title and the opening with `lang="grc"`; the back link is `/catalogs?language=` + `classical.Language` | **Generalise.** `lang="grc"` is hard-coded on the opening of every Work; a Latin Work would be mislabelled for screen readers and font selection. |
| `internal/webapp/catalog_pages.templ:160-161`, `my_books.templ:99-102` | Greek title with `lang="grc"` | **Generalise** (use the Work's language; field empty for Latin). |
| `internal/webapp/components.templ:46-47` | Title-plate Greek title with `lang="el"` (not `grc`) | **Existing inconsistency.** A Latin plate would show no second line when the field is empty. Fix is optional for slice 1. |
| `internal/webapp/components.go:87-104`, `library_browse.go:444-448`, `reading.templ:65`, `catalog_classical.go:46-70`, `persistence/books.go:290`, `persistence/my_books_reading_units.go:41` | The `GreekTitle` field flows through the plate and the catalogue search | **Leave alone** if the field is kept. |
| `internal/webapp/study_language_pages.go:100-140` | The URL language segment accepts any alphabetic tag; `prefixedLanguagePage` redirects when the learner has no Books or vocabulary in the language (`view.hasLanguage`) | **Leave alone.** `/la/…` already works once a Latin Book exists. (This came with commit 4db04167, "the URL carries the study language".) |
| `internal/webapp/catalog_browse.go:348-392` | Capability names are only a display-name source; a language with no capability falls back to the code | **Leave alone.** A Latin with no NLP capability is tolerated here. |
| `internal/analysis/analysis_update.go:47-54,174,243` | Analysis-update targets come from capabilities, per language | **Leave alone**, but check: imported Latin analyses have no capability, so `outdatedCandidate` must treat them as "not outdated" as Greek already does. |
| `internal/opds/client.go:307-308` | Maps "Greek, ancient (to 1453)" to `grc` | **Leave alone.** (OPDS language mapping for Gutenberg; Latin would be a separate OPDS concern.) |

### Fixtures and browser smoke tests

- `internal/fixtures/classical.go`, `classical_add.go`, `reading_assertion.go`
  and `internal/storecontract/{classical,greek,gate,harness}.go` use
  `classical.Language`, `GreekLanguage = "grc"` (`harness.go:27`) or the Greek
  title. **Generalise** once: add one Latin fixture Work.
- `internal/testutil/corpus_sentence.go:117`: a per-language template sentence
  with a `grc` case; a Latin case is needed for generated test corpora.
- `e2e/tests/classical-catalog.spec.ts` (lines 18-83),
  `catalogue-sync.spec.ts:62`, `construction-parallels.spec.ts`,
  `my-books-remove.spec.ts` pin `grc` in URLs and selects. **Leave alone**;
  add one Latin smoke path only when the slice lands.

## Recommendations

1. **Treat a first Latin slice as a catalog generalisation plus one adapter
   plus a card-policy decision**, not as a refactor of Greek code. Do the
   generalisation in this order: (a) per-Definition language and display name
   (`classical.Language`, `Definition.Language` validation, the manifest
   generator); (b) a neutral home for `ImportError` and its codes; (c) the
   Latin source adapter and `la` profile; (d) the card-policy lookup replacing
   `IsAncientGreek`.
2. **Do not rename `greekTitle`, the `greek_title` column or the `ancientgreek`
   package in slice 1.** Renaming costs a migration and wide churn for no
   behavioural gain.
3. **Keep Constructions fail-closed.** No work is needed; do not generalise
   `ConcordanceConstructionLanguage` until a Latin adapter meets the ADR 0125
   shipping bar.
4. **Settle the card policy before the card work.** The decision is whether
   Latin is a "source-evidence" language (no translation provider; the deck is
   plannable without one). It changes `prepareddeck/planning.go:321,487`, the
   attribution notices and the provider assessment.
5. **What would change this estimate:** a Latin source that already arrives as
   OGA-style `oga-conllu` would shrink the preparation cost sharply; one with a
   different packaging or a non-UD tag set would raise it. A Latin dictionary
   with no Greek-style index would raise the cards cost.

## Not found / limits

- I did not run any code or tests; the figures are static reading of the repo
  at the pinned commit.
- I did not measure the relative costs; the percentages are judgments.
- Documentation sites (`doc/…`, ADR prose) that mention `grc` are not
  inventoried; only code, templates, SQL, scripts and tests were.
- Licence classification of Latin layers is out of scope here (the corpus
  ticket owns it); this note records only that layers are keyed per layer in
  `internal/classical/layers.json` under ADR 0111.

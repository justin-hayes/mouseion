# mouseion — Product Summary

## What it is

Mouseion is a self-hosted web application for advanced foreign-language reading that adds a learner's ready-language catalog as metadata-first entries through automated synchronization, acquires content lazily per Book on learner intent, analyzes declared main text when EPUB structure identifies it and otherwise analyzes the complete snapshot as an ensure-once consequence of Reading Journey membership, explains current known coverage and additional vocabulary investment, and prepares Anki recognition-card decks from eligible unknown vocabulary. It is multi-user: books, known vocabulary, generated cards, vocabulary study (per Book), and OPDS catalog connections belong to each learner. There is no active in-application administrator role. A fresh installation allows first-account onboarding; once an account exists, users enter through normal login.

## Current learner-facing organization

The authenticated shell has exactly four primary destinations: **My Books** at
`/library`, **Reading Journey** at `/journey`, **Vocabulary** at `/vocabulary`, and
**Catalogs** at `/catalogs`. Catalogs owns catalogue setup and sync maintenance;
the legacy `GET /connections` route permanently redirects there while preserving
`book_id`, `message`, and `error`. My Books is the sole browse surface for the synced collection,
and EPUB content is acquired when a Book is added to Reading Journey. The upstream catalog
browser is retired. `/` redirects to My Books. Vocabulary study is a
book-anchored facet (see [ADR 0053](adr/0053-book-anchored-vocabulary-consolidation.md)):
a Book's prepared-deck actions, vocabulary-study state and history, and
vocabulary provenance live on the Journey entry, not on a second learner-facing
plan.

Catalogue synchronization status is part of the learner-owned Catalogs surface
at `/catalogs`, with detailed work under `/jobs`.

Study languages are derived from the distinct normalized language tags of the
learner's active chosen-language Books; there is no study-language preference or
Settings destination. The learner's **active study language** is a stored context
pointing into that derived set: it scopes My Books browse and search, Reading
Journey, and Vocabulary through a shell-level switcher, and Reading Journey and
Primary Goal are one per study language (ADR 0050/0051). Vocabulary owns
owner-scoped, language-scoped known vocabulary. Known-vocabulary import is
explicit and additive and targets the active study language: the learner
uploads a UTF-8 lemma file, with new,
duplicate, and rejected rows reported separately. The direct `/known-vocab` route
remains a compatibility redirect to Vocabulary, and `/settings` redirects to My
Books. When NLP capability discovery is degraded, Vocabulary keeps derived
languages legible from stored display names and does not offer an unrelated
language for import.

## Feature specifications

- [Analysis Insights](features/analysis-insights.md) — learner-facing coverage, threshold, and difficulty information after book analysis.
- [EPUB Analysis Scope — Phase 1](features/epub-analysis-scope.md) — preserves ordered EPUB units, stable identity, provenance, and navigation data for analysis.
- [Main text selection](features/main-text-selection.md) — derives the analyzed main-text run from declared EPUB structure with a whole-snapshot fallback.
- [Language Support](features/language-support.md) — capability-driven German and Italian analysis, deployment, and end-to-end validation.
- [Catalog Sync](features/catalog-sync.md) — metadata-first, ready-language reconciliation from learner-owned catalogs with lazy content acquisition.
- [My Books Collection Browsing](features/collection-browsing.md) — paging and text search scoped to the active study language.
- [Language Mode](features/language-mode.md) — the active study language scopes the shell, My Books, Reading Journey, and Vocabulary.
- [Recognition-card sentence presentation](features/recognition-card-sentence-presentation.md) — complete bolded source sentences, readable long-card presentation, and optional validated English target highlighting.
- [Durable prepared-deck translation](features/durable-prepared-deck-translation.md) — resumable manifests, durable candidate outcomes, and atomic finalization for prepared decks.
- [OpenAI Batch API for prepared-deck translation](features/openai-batch-translation.md) — durable asynchronous Batch execution for optional prepared-deck translation.
- [Fast user-facing prepared-deck translation](features/fast-user-facing-translation.md) — bounded standard translation requests are the interactive default, with Batch retained for explicit offline work.
- [LLM sense selection and fallback gloss](features/llm-sense-selection.md) — consented selection from frozen dictionary senses and fallback glosses during deck preparation.
- [Concordance Foundation](features/concordance-foundation.md) — persists the normalized sentence/token corpus at analysis time so a future book- and study-language-scoped concordancer can query occurrences without re-running NLP.
- [Dependency Parse Foundation](features/dependency-parse-foundation.md) — persists each token's dependency relation and head at analysis time and extends the concordance query layer with grammar-aware role and dependents queries; the data prerequisite for deterministic sentence-quality scoring.
- [German separable-verb lemmatization](features/separable-verb-lemmatization.md) — reattaches German separable particles to verb lemmas in the NLP producer so vocabulary identity is the full lexeme, and excludes particles from content-word candidates.
- [Sentence-quality scoring](features/sentence-quality-scoring.md) — a deterministic GDEX-informed rubric computed at export time over the persisted corpus: a finite-verb-and-subject knock-out plus gradual ranking (subordinate-clause placement, deixis, entity density, length).
- [Dictionary gloss and morphology enrichment](features/dictionary-gloss-enrichment.md) — a built-in dictionary provider over a build-time-derived SQLite index (Wiktextract/Kaikki) supplying consent-free, deterministic English glosses and morphology (article, gender, plural) for German and Italian.

Retired feature records are preserved under [`doc/archive/features/`](archive/features/).

## Current pipeline

1. **Catalog discovery and My Books** — sync metadata from an owner-scoped OPDS catalog whose credentials are encrypted at rest, and browse the resulting local My Books collection. Sync creates or updates metadata-only Books and never downloads content, starts analysis, or invalidates existing evidence. The resulting chosen-language Books derive the learner's study-language set.
2. **Reading intent** — add a Book to Reading Journey from a My Books row when it is a candidate to read. This is the learner-initiated exception to metadata-only sync: it retains Journey membership, acquires the current EPUB when needed, and ensures one analysis for the current content revision, selecting declared main text when safe and otherwise using the complete snapshot. Re-adding and reordering are idempotent and do not create redundant work.
3. **Analysis and insights** — observe an asynchronous analysis producing an immutable completed corpus, then inspect the Journey entry's current analysis for known coverage, vocabulary investment, and deck preparation. Prior runs remain operational history.
4. **Candidate persistence** — aggregate every eligible content-word lemma in the analyzed EPUB, including lemmas occurring once, while excluding proper names, punctuation, and function words.
5. **Deck selection** — classify explicitly known and graduated vocabulary as known, reserve the currently-studied Book's vocabulary without counting it as known, and leave released (abandoned) vocabulary eligible again. Select every eligible unknown lemma appearing at least three times in the analyzed EPUB; the minimum occurrence count is a selection parameter, not yet customizable.
6. **Sentence selection** — use an example from the completed analysis for each selected lemma.
7. **Prepared deck** — from a completed analysis, asynchronously build an owner-scoped `.apkg`
   named `Mouseion::<language>::<book title>`. Each Book has one current deck; the ready deck is available from the book and operational history. Cards remain ordered by each lemma's first
   encounter in the book.

Generated-deck history and known vocabulary are deliberately separate. Generating a card records that the owner was assigned the lemma, with its book/deck provenance, but never by itself adds it to `known_vocabulary`. A Book's reserved vocabulary is held aside for study but is not known. Vocabulary graduates to known only through the single justified transition of [ADR 0036](adr/0036-primary-goal-justified-graduation.md) as re-expressed by [ADR 0053](adr/0053-book-anchored-vocabulary-consolidation.md): a Book's deck-snapshotted, provenance-linked vocabulary identity graduates on confirmed deck review; reading-finished alone graduates nothing. Releasing a study makes its vocabulary eligible again unless it is independently known. Re-generating the same book remains safe and does not duplicate cards or provenance.

## Current stack

- **Go core:** domain logic, PostgreSQL persistence, imports, coverage selection, sentence selection, Anki export, and the web/River worker process.
- **Python/Stanza gRPC service:** long-lived, ingest-time linguistic analysis behind a versioned Protobuf contract; authoritative for advertised language and feature capabilities.
- **PostgreSQL:** application data, per-user learning state and catalog connections, and River jobs.
- **Web application:** server-rendered Templ views enhanced with HTMX and styled with Pico CSS.
- **Deployment:** three processes—PostgreSQL, the Python NLP gRPC service, and the Go web application with River workers.

## Architecture decision records

The current decisions are listed first. Superseded or historical decisions are
listed separately so the current contract does not require following a chain of
amendments.

### Current decisions

1. [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](adr/0001-go-core-python-nlp-service.md) — keeps product logic in Go and isolates Python behind a coarse NLP boundary.
2. [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](adr/0002-multi-user-accounts.md) — gives each learner an isolated account and learner-owned state; its admin/global-resource clauses are superseded by ADR 0024.
3. [ADR 0003: PostgreSQL as the initial persistence backend](adr/0003-postgresql-persistence.md) — uses PostgreSQL for concurrent multi-user persistence and job infrastructure.
4. [ADR 0004: Web application as the sole v1 client](adr/0004-web-only-v1-client.md) — makes the web app the v1 interface while preserving shared core boundaries.
5. [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](adr/0005-vocabulary-identity-normalization-ranking.md) — defines vocabulary identity and German normalization; current selection is defined by ADR 0048.
6. [ADR 0006: Anki export and known-vocabulary import contracts](adr/0006-anki-export-import-contracts.md) — specifies TSV export, note identity, and lemma-list import.
7. [ADR 0007: Enrichment providers, caching, and privacy policy](adr/0007-enrichment-providers-caching-privacy.md) — keeps most enrichment local and bounds external translation data.
9. [ADR 0009: Home-lab authentication and corpus-artifact isolation](adr/0009-home-lab-auth-corpus-isolation.md) — defines local accounts, private source artifacts, and Tailscale-only v1 access.
10. [ADR 0010: Adopt River as the background-job queue](adr/0010-river-job-queue.md) — runs durable background work in PostgreSQL without another broker.
11. [ADR 0011: gRPC as the Go↔Python transport for the NLP service](adr/0011-grpc-go-python-transport.md) — uses a long-lived typed gRPC boundary for analysis.
12. [ADR 0012: Enrichment execution—local inline, external translation via River](adr/0012-enrichment-execution-via-river.md) — separates immediate local enrichment from retryable external work.
13. [ADR 0013: Size-based NLP analysis chunking](adr/0013-size-based-nlp-chunking.md) — bounds analysis requests and aggregates chunk results deterministically.
17. [ADR 0017: Replace frequency-based ranking with coverage-based selection](adr/0017-coverage-based-selection.md) — defines the vocabulary selection basis, with the deck rule amended by ADR 0048.
18. [ADR 0018: Remove global frequency dataset (DWDS) import](adr/0018-remove-dwds-frequency.md) — supersedes the DWDS import contract; coverage-based selection does not need corpus-frequency data.
19. [ADR 0019: Explicit generated-vocabulary exclusion policy](adr/0019-generated-vocabulary-exclusion.md) — distinguishes explicitly known words from generated-deck provenance.
20. [ADR 0020: Anki package output and Mouseion deck hierarchy](adr/0020-anki-package-output.md) — makes `.apkg` the primary export and standardizes `Mouseion::<language>::<book title>` deck names.
21. [ADR 0021: Contextual sentence translation cache and privacy](adr/0021-contextual-translation-cache.md) — separates sentence translation from lemma glosses, prevents context collisions, and preserves the external-provider privacy boundary.
22. [ADR 0022: Asynchronous deck preparation and durable APKG artifacts](adr/0022-prepared-decks.md) — separates preparation from pure download and stores immutable prepared packages durably in PostgreSQL.
23. [ADR 0023: NLP service owns language capabilities](adr/0023-nlp-capabilities.md) — makes the NLP service authoritative for supported languages and features.
24. [ADR 0024: Learner-owned catalogs and removal of the admin role](adr/0024-learner-owned-catalogs-no-admin.md) — moves OPDS ownership to learners and removes the obsolete in-app administrator role.
25. [ADR 0025: Analysis coverage and threshold metric contract](adr/0025-analysis-coverage-threshold-metrics.md) — defines analyzable-token coverage, learner-state categories, threshold denominators, and deterministic selection.
26. [ADR 0026: Explainable structural text profile](adr/0026-structural-text-profile.md) — persists sentence-length and analysis-coverage signals without a composite difficulty or proficiency claim.
28. [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](adr/0028-explicit-scoped-analysis-lifecycle.md) — preserves immutable source and analysis history; its learner-facing scope confirmation is retired.
29. [ADR 0029: Recognition-card sentence presentation](adr/0029-recognition-card-sentence-presentation.md) — replaces LLM-selected short contexts and cloze presentation with complete bolded recognition sentences and removes duplicate source display.
30. [ADR 0030: Durable prepared-deck translation runs](adr/0030-durable-prepared-deck-translation.md) — defines immutable preparation runs, resumable candidate outcomes, and idempotent atomic finalization.
32. [ADR 0032: Standard-first prepared-deck translation](adr/0032-standard-first-prepared-deck-translation.md) — makes durable standard execution the interactive default while retaining Batch for explicit offline/economy work.
33. [ADR 0033: Deterministic in-memory fixture server driven by Playwright for browser acceptance](adr/0033-browser-acceptance-harness.md) — adds a fixture-driven browser acceptance harness (Go in-memory fixture server + Playwright) as the Phase 6 quality-gate substrate.
34. [ADR 0034: One implicit Reading Journey with learner-canonical ordering and campaign-queue migration](adr/0034-reading-journey-identity-ordering.md) — makes Reading Journey the owner-scoped, freely reorderable candidate pool.
35. [ADR 0035: Separate My Books membership from acquired source provenance](adr/0035-my-books-membership-and-source-provenance.md) — models owner-scoped bibliographic membership independently from immutable acquired EPUB evidence and its downstream history.
36. [ADR 0036: Deck-independent Primary Goal and single justified vocabulary-graduation transition](adr/0036-primary-goal-justified-graduation.md) — defines the single justified graduation path and Primary Goal behavior.
37. [ADR 0037: Cross-book vocabulary projection and advisory Journey ordering](adr/0037-cross-book-projection-advisory-ordering.md) — defines the reproducible route-comparison objective (current known-token coverage) for the vocabulary-efficient alternative to the learner's canonical Reading Journey order, with deterministic ordering, current-vs-conditional projection, incomparable-book handling, and on-demand recalculation.
38. [ADR 0038: Schema-change governance and migration review policy](adr/0038-schema-change-governance.md) — requires accepted product/architecture shape before consequential SQL and defines proportionate additive-field, backfill, staged-rollout, reversion-risk, and destructive-change review gates; its shipped-migration immutability clause is superseded by ADR 0070.
39. [ADR 0039: Drop retired EPUB classifier schema](adr/0039-drop-retired-epub-classifier-schema.md) — removes dormant classifier tables and scope metadata while preserving reviewed-scope structure and history.
40. [ADR 0040: One current analysis per book](adr/0040-one-current-analysis-per-book.md) — defines one book-centered learner analysis surface while retaining prior immutable runs as operational audit history; its retired route wording is superseded by ADR 0055.
41. [ADR 0041: Catalogue sync is metadata-first and non-destructive](adr/0041-catalog-sync-metadata-first.md) — defines per-connection ready-language metadata reconciliation, lazy content acquisition, and non-destructive sync.
43. [ADR 0043: Study languages are derived from the library and Settings is removed](adr/0043-study-languages-derived-settings-removed.md) — flips catalogue-sync scope to the library and removes the Settings destination.
44. [ADR 0044: Catalogue-entry alias identity retains the catalogue connection](adr/0044-catalogue-entry-connection-scoped-identity.md) — scopes the catalogue-entry alias to owner plus connection plus entry, threads the connection through sync/refresh/acquisition, and defers multi-connection conflict rules under a soft single-catalogue posture.
46. [ADR 0046: Book language has one canonical base form enforced at the domain](adr/0046-book-language-canonical-base-form.md) — collapses a chosen Book language to its base tag (`de_DE`/`de-de`/`de` all canonicalize to `de`), enforces the form at the domain, converges legacy rows, and simplifies the tolerant SQL.
47. [ADR 0047: Content acquisition is folded into analysis, and the library is catalogue-derived](adr/0047-acquisition-folded-into-analysis.md) — makes the catalog the source of Book metadata and folds acquisition into intent-driven analysis; main-text selection is defined by ADR 0066.
48. [ADR 0048: Frequency-floor deck selection](adr/0048-frequency-floor-deck-selection.md) — replaces the 97% coverage-prefix deck selection with a minimum-occurrence frequency floor (default three), dropping the deck's coverage guarantee.
49. [ADR 0049: Reading intent triggers analysis](adr/0049-reading-intent-triggers-analysis.md) — makes analysis an automatic, ensure-once consequence of Reading Journey membership, defines Primary Goal as a promotion of an analyzed Journey member (choosable only from the Journey screen), and enforces the Goal/membership invariant at the persistence layer. Its standalone-action portions are superseded by ADR 0054.
50. [ADR 0050: The app works in one active study language at a time](adr/0050-active-study-language.md) — makes the active study language a stored context pointing into the derived set, scopes every language-dependent surface through a shell-level switcher, and removes per-screen pickers and the "All languages" default.
51. [ADR 0051: Reading journeys and primary goals are one per language](adr/0051-reading-journeys-and-goals-per-language.md) — partitions Reading Journey and Primary Goal identity by study language, with a per-language revision and a split backfill migration.
52. [ADR 0052: The domain owns evidence classification](adr/0052-domain-owns-evidence-classification.md) — makes the evidence state a single derivation on the domain types read by My Books, the Reading Journey, and corpus/route insights, removes the SQL-assigned `EvidenceState` and its webapp fallback, and keeps goal-eligibility a read-only projection with enforcement at the persistence layer.
53. [ADR 0053: Book-anchored vocabulary consolidation](adr/0053-book-anchored-vocabulary-consolidation.md) — dissolves the learning campaign as a separate reservation/plan object and anchors vocabulary-study state onto the Book, making the Book the single unit of the learner loop with independent reading and vocabulary facts; the campaign's dead-end tail (deck study → graduation) becomes a reachable, book-scoped action.
54. [ADR 0054: Retire the standalone analysis action](adr/0054-retire-standalone-analysis-action.md) — removes the learner-facing analysis trigger and metadata-only Book detail page, making Add to Reading Journey the sole initial acquisition-and-analysis intent; its completed-page route portions are superseded by ADR 0055.
55. [ADR 0055: Retire the standalone Book detail route](adr/0055-retire-book-detail-route.md) — makes the Journey entry the sole analyzed-Book destination, retires `GET /books/{id}`, constrains exact-analysis compatibility redirects to reachable Journey members, and moves learner-facing refresh/deck mutations to their owning surfaces.
56. [ADR 0056: Retire the Campaign learner surface](adr/0056-retire-campaign-learner-surface.md) — removes the Campaign queue, history, and operations from the learner-facing application and makes Book vocabulary-study state and history the canonical surface.
57. [ADR 0057: Retire the Language view panel](adr/0057-retire-language-view-panel.md) — retires the proposed My Books language panel and its read model; per-Book and Journey surfaces remain the evidence contracts.
58. [ADR 0058: Catalog maintenance is a principal destination](adr/0058-catalog-maintenance-principal-destination.md) — promotes learner-owned catalog connection maintenance to a fourth shell destination at `/catalogs`, retires "Add books" as a term, and standardizes the learner-facing spelling on "catalog".
59. [ADR 0059: Persisted normalized corpus for future concordance](adr/0059-persisted-normalized-corpus-for-concordance.md) — persists the normalized sentence/token stream at analysis time in owner-scoped tables so a future book- and study-language-scoped concordancer can query occurrences without re-running NLP.
60. [ADR 0060: Persist dependency parses in the normalized corpus](adr/0060-persist-dependency-parses.md) — adds always-on dependency parsing to the NLP boundary and persists each token's basic dependency relation and head, enabling grammar-aware concordance queries and deterministic sentence-quality scoring; amends ADR 0059's no-protobuf-change line.
61. [ADR 0061: German separable-verb lemmatization from dependency data](adr/0061-german-separable-verb-lemmatization.md) — reattaches separated German verb particles to the verb's canonical lemma in the NLP producer, making the full lexeme the vocabulary identity; amends ADR 0005's normalization and identity interpretation.
62. [ADR 0062: Sentence-quality scoring derived from the persisted corpus](adr/0062-derived-sentence-quality-scoring.md) — derives a GDEX-informed sentence-quality rubric at export time over the persisted corpus (a finite-verb-and-subject knock-out plus gradual ranking); amends ADR 0029's representative-sentence selection mechanism.
63. [ADR 0063: Stanza model provisioning on a Docker volume instead of the image](adr/0063-stanza-models-on-volume.md) — provisions the full Stanza processor bundle into a named volume instead of baking models into the NLP image.
64. [ADR 0064: Built-in dictionary enrichment provider](adr/0064-dictionary-enrichment-provider.md) — makes gloss local, default-on enrichment from a build-time-derived SQLite index (Wiktextract/Kaikki) with deterministic sense ordering and dictionary morphology; amends ADR 0007's gloss classification and ADR 0029's card contract.
65. [ADR 0065: Canonicalize pre-1996 German ß spellings](adr/0065-german-pre-1996-sharp-s-canonicalization.md) — maps explicitly documented pre-reform German spellings to post-1996 canonical lemmas, preserves modern ß and distinct lexemes, and defines the idempotent vocabulary backfill before profile activation.
66. [ADR 0066: Identify a Book's main text for analysis from declared EPUB structure](adr/0066-main-text-selection-from-epub-structure.md) — derives a Book's main text from EPUB 3 landmark declarations, analyzes that run with a fail-safe whole-snapshot fallback, versions the selection in the configuration identity, and supersedes ADR 0047's always-complete-scope clause.
67. [ADR 0067: Recognition-card morphology and multi-span target presentation](adr/0067-recognition-card-morphology-presentation.md) — adds a `Plural` note field beside the lemma, guards the derived article to genuine definite articles, bolds every component of a separable verb's full lemma, and makes dictionary gloss coverage a measured decision; amends ADR 0029, ADR 0064, and ADR 0061's learner-surface non-goal.
68. [ADR 0068: Recognition-card meaning and form presentation](adr/0068-recognition-card-meaning-and-form-presentation.md) — collapses the card back to one dictionary-sourced meaning block, stops rendering the duplicate `English` lemma translation, and adds dedicated `IPA` and `PrincipalParts` note fields (German-first) resolved at manifest freeze; amends ADR 0029 and ADR 0064 and extends ADR 0067.
69. [ADR 0069: LLM sense selection and fallback gloss](adr/0069-llm-sense-selection-and-fallback-gloss.md) — gives the consented external LLM index-based selection over a frozen candidate sense set plus a fallback gloss when no dictionary sense fits, applied post-freeze as a render overlay and cached with dictionary identity; amends ADR 0007, ADR 0029, and ADR 0064.
70. [ADR 0070: Reboot migration history and consolidate superseded documentation](adr/0070-migration-and-documentation-reboot.md) — consolidates the pre-baseline migration history into one current-state baseline migration, retires migration-transition tests, cuts over by recreating databases, and reconciles `product.md`, feature docs, and the ADR index to the present state; partially supersedes ADR 0038's immutability clause.

### Superseded or historical decisions

- [ADR 0008: Global frequency dataset source and import contract](adr/0008-global-frequency-dataset-source-import.md) — superseded by ADR 0018.
- [ADR 0014: Admin and learner roles are orthogonal](adr/0014-admin-learner-roles.md) — superseded by the learner-owned, no-admin model in ADR 0024.
- [ADR 0015: OPDS connections are admin-configured](adr/0015-opds-connection-admin-browse-user.md) — superseded by learner-owned catalogs in ADR 0024.
- [ADR 0016: Separate admin and user account roles](adr/0016-separate-admin-user-roles.md) — superseded by the current account model and ADR 0024.
- [ADR 0027: Single-active learning campaigns and vocabulary graduation](adr/0027-learning-campaigns.md) — its learner-facing campaign model is superseded by ADRs 0034, 0036, and 0053-0056.
- [ADR 0031: OpenAI Batch prepared-deck translation](adr/0031-openai-batch-prepared-deck-translation.md) — its default dispatch path is superseded by ADR 0032; Batch remains available for explicit offline work.
- [ADR 0042: Derive a per-language corpus view without a persisted corpus object](adr/0042-derived-language-corpus-view.md) — superseded by ADR 0057.
- [ADR 0045: Book detail is addressed by owner-scoped Book ID, with source IDs resolving in place](adr/0045-book-detail-book-id.md) — its learner-facing Book-detail route portions are superseded by ADR 0055; the Book identity remains in force.

## Deployment and operations

Run PostgreSQL, the Python NLP gRPC service, and the Go web/River worker process. Key environment variables include:

- `MOUSEION_DATABASE_URL` — PostgreSQL connection string.
- `MOUSEION_NLP_ADDR` — address of the Python gRPC service.
- `MOUSEION_NLP_WARM_LANGUAGES` — comma-separated language pipelines to preload and
  advertise from the NLP service and provision into its model cache. Compose
  defaults to `de,it` and provisions the full Stanza processor bundle into the
  named `stanza-data` volume before starting NLP; manually launched services
  retain the application default of `de` and should run the provisioner first.
  Models are stored under `STANZA_RESOURCES_DIR`. Changing this value adds
  languages without an image rebuild, while a Stanza version change causes the
  marker-driven provisioner to refresh the bundle. The singular
  `MOUSEION_NLP_WARM_LANGUAGE` remains supported for backward compatibility.
- `MOUSEION_ANALYSIS_JOB_TIMEOUT` — maximum duration allowed for an analysis job.
- `MOUSEION_LLM_ENABLED` — set to `true` to enable optional external translation;
  also requires `MOUSEION_LLM_API_KEY` and `MOUSEION_LLM_MODEL`.
- `MOUSEION_LLM_BASE_URL` — optional OpenAI-compatible API root; defaults to
  `https://api.openai.com/v1`.
- `MOUSEION_LLM_TIMEOUT` — optional positive Go duration; defaults to `30s`.
- `MOUSEION_LLM_REASONING_EFFORT` — optional `low`, `medium`, or `high` value;
  defaults to `low`.
- `MOUSEION_LLM_SUPPORTS_REASONING_EFFORT` — set to `true` only when a custom
  OpenAI-compatible endpoint/model supports `reasoning_effort`.
- `MOUSEION_DICTIONARY_INDEX` — optional read-only SQLite artifact generated by
  `make dictionary-index`; the artifact contains the full German and Italian
  Kaikki-derived index and is not imported into Postgres.
- `MOUSEION_DICTIONARY_HOST_DIR` — Compose-only host directory containing the
  index; it is bind-mounted read-only into the web container. The host path is
  a directory rather than a single-file mount so a missing artifact remains
  missing. This is the deployment host-path setting; the application reads the
  mounted file named by `MOUSEION_DICTIONARY_INDEX`.
- `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS` — maximum requests in one
  prepared-deck Batch input file; defaults to `5000` and is capped at `50000`.
- `MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL` — provider Batch polling
  interval; defaults to `30s` and is capped at `24h`.
- Prepared-deck Batch files are temporary: Mouseion requests seven-day
  provider expiration and deletes input/output/error files after reconciliation
  or cancellation. Failed deletion is retried a bounded number of times and
  does not invalidate a reconciled deck.

The dictionary index is refreshed separately from application state. After
`make setup`, run `make dictionary-index DICTIONARY_DUMP_DATE=YYYY-MM-DD
DICTIONARY_WIKTEXTRACT_COMMIT=<commit> DICTIONARY_REFRESH=1` to download the
current weekly raw Kaikki Wiktextract dump and derive the combined German and
Italian SQLite index. Use `KAIKKI_INPUT=/path/to/dump.jsonl.gz` instead for an
offline rebuild from an existing raw dump. The generated
`provider_version` records the dump date, UTC extraction date, and Wiktextract
commit. Replace the read-only artifact under `MOUSEION_DICTIONARY_HOST_DIR`
and restart the web process; no migration or new service is needed. The index
records its source, license, and attribution; see the [refresh
instructions](../README.md#refreshing-the-dictionary-index).

The index contains Wiktionary-derived data from [Kaikki.org](https://kaikki.org/)
under the source's dual CC BY-SA 3.0 / GFDL terms. Preserve its generated
metadata and attribution when shipping or sharing it; see the [refresh
instructions](../README.md#refreshing-the-dictionary-index) for the full notice.

Prepared-deck translation uses durable standard execution by default when
external translation is enabled. Batch remains available for explicit
offline/economy work; those preparations can remain in `preparing` while
OpenAI processes a Batch for up to the provider's 24-hour completion window.
The status endpoint reports the current phase, durable counts, retrying items,
and cancellation control. Cancellation stops local publication first, while
provider file cleanup remains best effort.

The v1 service is intended for a private home-lab deployment reachable only over Tailscale. See the [README](../README.md) for current setup commands and [documentation governance](documentation-governance.md) for the boundary between this present-state summary, repository ADRs, and planning material.

Mouseion does not currently expose open registration or an open/closed
registration setting. Language availability is discovered from the running NLP
service; it is not configured as a separate application-managed resource.

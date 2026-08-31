# mouseion — Product Summary

## What it is

Mouseion is a self-hosted web application for advanced foreign-language reading. It adds books from learner-owned OPDS catalogs, analyzes a learner-confirmed EPUB scope, explains current and projected vocabulary coverage, and prepares Anki recognition-card decks from eligible unknown vocabulary. It is multi-user: books, known vocabulary, generated cards, learning campaigns, and OPDS catalog connections belong to each learner. There is no active in-application administrator role. A fresh installation allows first-account onboarding; once an account exists, users enter through normal login.

## Feature specifications

- [Analysis Insights](features/analysis-insights.md) — learner-facing coverage, threshold, and difficulty information after book analysis.
- [Explicit Scoped-Analysis Workflow](features/explicit-scoped-analysis-workflow.md) — separates OPDS intake, immutable scope confirmation, explicit analysis, insights, and deck preparation.
- [EPUB Analysis Scope — Phase 1](features/epub-analysis-scope.md) — preserves ordered EPUB units and provenance before later classification and selection.
- [EPUB Analysis Classification — Phase 2](features/epub-analysis-classification.md) — assigns deterministic, explainable structural categories and analysis recommendations.
- [EPUB Analysis Scope Review — Phase 3](features/epub-analysis-scope-review.md) — lets learners review unit recommendations and analyze only the persisted selected scope.
- [EPUB Scope Workflows — Phase 4](features/epub-analysis-scope-workflows.md) — improves hierarchy visualization, classifier refinement, and reusable scope decisions.
- [EPUB Recommendation Corrections — Phase 5](features/epub-analysis-recommendation-corrections.md) — makes structural recommendations safe, coherent, and explainable.
- [Language Support](features/language-support.md) — capability-driven German and Italian analysis, deployment, and end-to-end validation.
- [Recognition-card sentence presentation](features/recognition-card-sentence-presentation.md) — complete bolded source sentences, readable long-card presentation, and optional validated English target highlighting.
- [Durable prepared-deck translation](features/durable-prepared-deck-translation.md) — resumable manifests, durable candidate outcomes, and atomic finalization for prepared decks.
- [OpenAI Batch API for prepared-deck translation](features/openai-batch-translation.md) — durable asynchronous Batch execution for optional prepared-deck translation.

## Current pipeline

1. **Add to library** — acquire and validate an EPUB from an owner-scoped OPDS catalog whose credentials are encrypted at rest. Addition does not start analysis and supports adding multiple books without leaving the browser.
2. **Scope review** — review extracted EPUB units and confirm an immutable scope revision. Metadata-only edits do not invalidate it; changed EPUB content requires a new review.
3. **Explicit analysis** — start and observe an asynchronous analysis bound to one confirmed scope, producing an immutable completed corpus with source and scope provenance.
4. **Insights** — inspect coverage, threshold, structural, and quality information for that exact completed analysis.
5. **Candidate persistence** — aggregate every eligible content-word lemma in the analyzed scope, including lemmas occurring once, while excluding proper names, punctuation, and function words.
6. **Coverage selection** — classify explicitly known and graduated vocabulary as known, reserve active-campaign vocabulary without counting it as known, and leave abandoned-campaign vocabulary eligible again. Sort the remaining eligible unknown lemmas by analyzed-scope occurrence count and choose the smallest prefix accounting for at least a fixed 97% of their tokens.
7. **Sentence selection** — use an example from the completed analysis for each selected lemma.
8. **Prepared deck and campaign** — from a completed analysis, asynchronously build an owner-scoped `.apkg`
   named `Mouseion::<language>::<book title>`, then optionally add the ready deck
   to the learner's campaign queue. Cards remain ordered by each lemma's first
   encounter in the book.

Generated-deck history and known vocabulary are deliberately separate. Generating a card records that the owner was assigned the lemma, with its book/deck provenance, but never by itself adds it to `known_vocabulary`. Active-campaign vocabulary is reserved for the current workflow but is not known. Completing both the reading and deck-review conditions of a learning campaign explicitly graduates its assigned vocabulary to known; abandoning the campaign makes that vocabulary eligible again unless it is independently known. Re-generating the same book remains safe and does not duplicate cards or provenance.

## Current stack

- **Go core:** domain logic, PostgreSQL persistence, imports, coverage selection, sentence selection, Anki export, and the web/River worker process.
- **Python/Stanza gRPC service:** long-lived, ingest-time linguistic analysis behind a versioned Protobuf contract; authoritative for advertised language and feature capabilities.
- **PostgreSQL:** application data, per-user learning state and catalog connections, and River jobs.
- **Web application:** server-rendered Templ views enhanced with HTMX and styled with Pico CSS.
- **Deployment:** three processes—PostgreSQL, the Python NLP gRPC service, and the Go web application with River workers.

## Architecture decision records

1. [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](adr/0001-go-core-python-nlp-service.md) — keeps product logic in Go and isolates Python behind a coarse NLP boundary.
2. [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](adr/0002-multi-user-accounts.md) — separates learner-owned state from shared language resources.
3. [ADR 0003: PostgreSQL as the initial persistence backend](adr/0003-postgresql-persistence.md) — uses PostgreSQL for concurrent multi-user persistence and job infrastructure.
4. [ADR 0004: Web application as the sole v1 client](adr/0004-web-only-v1-client.md) — makes the web app the v1 interface while preserving shared core boundaries.
5. [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](adr/0005-vocabulary-identity-normalization-ranking.md) — defines vocabulary identity and German normalization; its ranking and review decisions are superseded by ADR 0017.
6. [ADR 0006: Anki export and known-vocabulary import contracts](adr/0006-anki-export-import-contracts.md) — specifies TSV export, note identity, and lemma-list import.
7. [ADR 0007: Enrichment providers, caching, and privacy policy](adr/0007-enrichment-providers-caching-privacy.md) — keeps most enrichment local and bounds external translation data.
8. [ADR 0008: Global frequency dataset source and import contract](adr/0008-global-frequency-dataset-source-import.md) — historical DWDS contract, superseded by ADR 0018.
9. [ADR 0009: Home-lab authentication and corpus-artifact isolation](adr/0009-home-lab-auth-corpus-isolation.md) — defines local accounts, private source artifacts, and Tailscale-only v1 access.
10. [ADR 0010: Adopt River as the background-job queue](adr/0010-river-job-queue.md) — runs durable background work in PostgreSQL without another broker.
11. [ADR 0011: gRPC as the Go↔Python transport for the NLP service](adr/0011-grpc-go-python-transport.md) — uses a long-lived typed gRPC boundary for analysis.
12. [ADR 0012: Enrichment execution—local inline, external translation via River](adr/0012-enrichment-execution-via-river.md) — separates immediate local enrichment from retryable external work.
13. [ADR 0013: Size-based NLP analysis chunking](adr/0013-size-based-nlp-chunking.md) — bounds analysis requests and aggregates chunk results deterministically.
14. [ADR 0014: Admin and learner roles are orthogonal](adr/0014-admin-learner-roles.md) — allows one account to administer the server and participate fully as a learner.
15. [ADR 0015: OPDS connections are admin-configured; browsing and import are user-level](adr/0015-opds-connection-admin-browse-user.md) — centralizes catalog credentials while keeping book choice in the learner workflow.
16. [ADR 0016: Separate admin and user account roles](adr/0016-separate-admin-user-roles.md) — makes server administration and learner workflows mutually exclusive account types.
17. [ADR 0017: Replace frequency-based ranking with coverage-based selection](adr/0017-coverage-based-selection.md) — selects the minimal unknown-lemma set needed for ≥97% text coverage, ordered by first encounter.
18. [ADR 0018: Remove global frequency dataset (DWDS) import](adr/0018-remove-dwds-frequency.md) — supersedes the DWDS import contract; coverage-based selection does not need corpus-frequency data.
19. [ADR 0019: Explicit generated-vocabulary exclusion policy](adr/0019-generated-vocabulary-exclusion.md) — distinguishes explicitly known words from generated-deck provenance; its future-selection semantics are superseded by ADR 0027.
20. [ADR 0020: Anki package output and Mouseion deck hierarchy](adr/0020-anki-package-output.md) — makes `.apkg` the primary export and standardizes `Mouseion::<language>::<book title>` deck names.
21. [ADR 0021: Contextual sentence translation cache and privacy](adr/0021-contextual-translation-cache.md) — separates sentence translation from lemma glosses, prevents context collisions, and preserves the external-provider privacy boundary.
22. [ADR 0022: Asynchronous deck preparation and durable APKG artifacts](adr/0022-prepared-decks.md) — separates preparation from pure download and stores immutable prepared packages durably in PostgreSQL.
23. [ADR 0023: NLP service owns language capabilities](adr/0023-nlp-capabilities.md) — makes the NLP service authoritative for supported languages and features.
24. [ADR 0024: Learner-owned catalogs and removal of the admin role](adr/0024-learner-owned-catalogs-no-admin.md) — moves OPDS ownership to learners and removes the obsolete in-app administrator role.
25. [ADR 0025: Analysis coverage and threshold metric contract](adr/0025-analysis-coverage-threshold-metrics.md) — defines analyzable-token coverage, learner-state categories, threshold denominators, and deterministic selection.
26. [ADR 0026: Explainable structural text profile](adr/0026-structural-text-profile.md) — persists sentence-length and analysis-coverage signals without a composite difficulty or proficiency claim.
27. [ADR 0027: Single-active learning campaigns and vocabulary graduation](adr/0027-learning-campaigns.md) — models one active book/deck workflow, explicit completion, vocabulary graduation, and abandoned-campaign release.
28. [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](adr/0028-explicit-scoped-analysis-lifecycle.md) — separates acquisition, scope confirmation, analysis, insights, and preparation while preserving source and artifact history.
29. [ADR 0029: Recognition-card sentence presentation](adr/0029-recognition-card-sentence-presentation.md) — replaces LLM-selected short contexts and cloze presentation with complete bolded recognition sentences and removes duplicate source display.
30. [ADR 0030: Durable prepared-deck translation runs](adr/0030-durable-prepared-deck-translation.md) — defines immutable preparation runs, resumable candidate outcomes, and idempotent atomic finalization.
31. [ADR 0031: OpenAI Batch prepared-deck translation](adr/0031-openai-batch-prepared-deck-translation.md) — superseded by ADR 0032; retains the historical Batch transport and reconciliation decisions.
32. [ADR 0032: Standard-first prepared-deck translation](adr/0032-standard-first-prepared-deck-translation.md) — makes durable standard execution the interactive default while retaining Batch for explicit offline/economy work.
33. [ADR 0033: Deterministic in-memory fixture server driven by Playwright for browser acceptance](adr/0033-browser-acceptance-harness.md) — adds a fixture-driven browser acceptance harness (Go in-memory fixture server + Playwright) as the Phase 6 quality-gate substrate.
34. [ADR 0034: One implicit Reading Journey with learner-canonical ordering and campaign-queue migration](adr/0034-reading-journey-identity-ordering.md) — makes Reading Journey the single learner-canonical, owner-scoped, freely-reorderable plan and replaces the derived Campaign queue while retaining Campaign history/vocabulary provenance.
35. [ADR 0035: Separate My Books membership from acquired source provenance](adr/0035-my-books-membership-and-source-provenance.md) — models owner-scoped bibliographic membership independently from immutable acquired EPUB evidence and its downstream history.

## Deployment and operations

Run PostgreSQL, the Python NLP gRPC service, and the Go web/River worker process. Key environment variables include:

- `MOUSEION_DATABASE_URL` — PostgreSQL connection string.
- `MOUSEION_NLP_ADDR` — address of the Python gRPC service.
- `MOUSEION_NLP_WARM_LANGUAGES` — comma-separated language pipelines to preload and
  advertise from the NLP service. The Compose deployment defaults to `de,it`, whose
  Stanza models are provisioned in the NLP image; manually launched services retain
  the application default of `de`. Models must exist in `STANZA_RESOURCES_DIR`
  before startup; the Compose image's immutable cache contains `de` and `it`, and
  another language requires an image rebuild that provisions its model. The singular
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
- `MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS` — maximum requests in one
  prepared-deck Batch input file; defaults to `5000` and is capped at `50000`.
- `MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL` — provider Batch polling
  interval; defaults to `30s` and is capped at `24h`.
- Prepared-deck Batch files are temporary: Mouseion requests seven-day
  provider expiration and deletes input/output/error files after reconciliation
  or cancellation. Failed deletion is retried a bounded number of times and
  does not invalidate a reconciled deck.

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

# mouseion — Product Summary

## What it is

Mouseion is a self-hosted web application for advanced foreign-language reading. It imports books, analyzes their language, and generates Anki study material for the unknown words needed to reach 97% book coverage. It is multi-user: books, known vocabulary, generated cards, and OPDS catalog connections belong to each learner. There is no active in-application administrator role. A fresh installation allows first-account onboarding; once an account exists, users enter through normal login.

## Feature specifications

- [Analysis Insights](features/analysis-insights.md) — learner-facing coverage, threshold, and difficulty information after book analysis.
- [Language Support](features/language-support.md) — capability-driven German and Italian analysis, deployment, and end-to-end validation.

## Current pipeline

1. **Ingest** — import an EPUB directly or from an owner-scoped OPDS catalog whose credentials are encrypted at rest.
2. **Analysis** — extract text and send size-bounded chunks to the Python/Stanza NLP service, producing a normalized corpus.
3. **Candidate persistence** — aggregate every eligible content-word lemma in the book, including lemmas occurring once, while excluding proper names, punctuation, and function words.
4. **Coverage selection** — before calculating the denominator, exclude vocabulary the learner explicitly marked known and vocabulary already assigned in a generated deck for another book. Sort the remaining unknown lemmas by book-local occurrence count and choose the smallest prefix accounting for at least a fixed 97% of their tokens.
5. **Sentence selection** — use an example from the learner's source text for each selected lemma.
6. **Prepared deck and campaign** — asynchronously build an owner-scoped `.apkg`
   named `Mouseion::<language>::<book title>`, then optionally add the ready deck
   to the learner's campaign queue. Cards remain ordered by each lemma's first
   encounter in the book.

Generated-deck history and mastery are deliberately separate. Generating a card records that the owner was assigned the lemma, with its first book/deck provenance, but never adds it to `known_vocabulary`. Re-generating the same book is safe and does not duplicate cards or provenance; marking a word mastered/known remains an explicit learner action.

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
19. [ADR 0019: Explicit generated-vocabulary exclusion policy](adr/0019-generated-vocabulary-exclusion.md) — distinguishes explicitly known words from words already assigned in generated decks, with owner/book provenance.
20. [ADR 0020: Anki package output and Mouseion deck hierarchy](adr/0020-anki-package-output.md) — makes `.apkg` the primary export and standardizes `Mouseion::<language>::<book title>` deck names.
21. [ADR 0021: Contextual sentence translation cache and privacy](adr/0021-contextual-translation-cache.md) — separates sentence translation from lemma glosses, prevents context collisions, and preserves the external-provider privacy boundary.
22. [ADR 0022: Asynchronous deck preparation and durable APKG artifacts](adr/0022-prepared-decks.md) — separates preparation from pure download and stores immutable prepared packages durably in PostgreSQL.
23. [ADR 0023: NLP service owns language capabilities](adr/0023-nlp-capabilities.md) — makes the NLP service authoritative for supported languages and features.
24. [ADR 0024: Learner-owned catalogs and removal of the admin role](adr/0024-learner-owned-catalogs-no-admin.md) — moves OPDS ownership to learners and removes the obsolete in-app administrator role.
25. [ADR 0025: Analysis coverage and threshold metric contract](adr/0025-analysis-coverage-threshold-metrics.md) — defines analyzable-token coverage, learner-state categories, threshold denominators, and deterministic selection.
26. [ADR 0026: Explainable structural text profile](adr/0026-structural-text-profile.md) — persists sentence-length and analysis-coverage signals without a composite difficulty or proficiency claim.
27. [ADR 0027: Single-active learning campaigns and vocabulary graduation](adr/0027-learning-campaigns.md) — models one active book/deck workflow, explicit completion, vocabulary graduation, and abandoned-campaign release.

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

The v1 service is intended for a private home-lab deployment reachable only over Tailscale. See the [README](../README.md) for current setup commands and [documentation governance](documentation-governance.md) for the boundary between this present-state summary, repository ADRs, and planning material.

Mouseion does not currently expose open registration or an open/closed
registration setting. Language availability is discovered from the running NLP
service; it is not configured as a separate application-managed resource.

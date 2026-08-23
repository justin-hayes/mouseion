# mouseion — Product Summary

## What it is

Mouseion is a self-hosted web application for advanced foreign-language reading. It imports books, analyzes their language, ranks useful vocabulary, supports learner review, and exports curated study material to Anki. It is multi-user: learning state belongs to each learner, while server configuration and shared language resources are administered centrally.

## Current pipeline

1. **Ingest** — import an EPUB directly or from an admin-configured OPDS catalog.
2. **Analysis** — extract text and send size-bounded chunks to the Python/Stanza NLP service, producing a normalized corpus.
3. **Selection** — identify vocabulary candidates and exclude known or previously handled items.
4. **Ranking** — rank candidates using corpus evidence and global, language-scoped frequency data.
5. **Enrichment** — add local morphology, frequency, and pronunciation data; external translation work can run through River.
6. **Sentence selection** — choose useful examples from the learner's source text.
7. **Review** — let the learner accept, ignore, edit, or otherwise curate candidates.
8. **Anki export** — export accepted vocabulary as UTF-8 tab-separated Anki notes.

## Current stack

- **Go core:** domain logic, PostgreSQL persistence, imports, ranking, enrichment, review, Anki export, and the web/River worker process.
- **Python/Stanza gRPC service:** long-lived, ingest-time linguistic analysis behind a versioned Protobuf contract.
- **PostgreSQL:** application data, per-user learning state, shared language resources, and River jobs.
- **Web application:** server-rendered Templ views enhanced with HTMX and styled with Pico CSS.
- **Deployment:** three processes—PostgreSQL, the Python NLP gRPC service, and the Go web application with River workers.

## Architecture decision records

1. [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](adr/0001-go-core-python-nlp-service.md) — keeps product logic in Go and isolates Python behind a coarse NLP boundary.
2. [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](adr/0002-multi-user-accounts.md) — separates learner-owned state from shared language resources.
3. [ADR 0003: PostgreSQL as the initial persistence backend](adr/0003-postgresql-persistence.md) — uses PostgreSQL for concurrent multi-user persistence and job infrastructure.
4. [ADR 0004: Web application as the sole v1 client](adr/0004-web-only-v1-client.md) — makes the web app the v1 interface while preserving shared core boundaries.
5. [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](adr/0005-vocabulary-identity-normalization-ranking.md) — defines vocabulary identity, German normalization, lifecycle, selection, and ranking.
6. [ADR 0006: Anki export and known-vocabulary import contracts](adr/0006-anki-export-import-contracts.md) — specifies TSV export, note identity, and lemma-list import.
7. [ADR 0007: Enrichment providers, caching, and privacy policy](adr/0007-enrichment-providers-caching-privacy.md) — keeps most enrichment local and bounds external translation data.
8. [ADR 0008: Global frequency dataset source and import contract](adr/0008-global-frequency-dataset-source-import.md) — standardizes the DWDS dataset and versioned import model.
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

## Deployment and operations

Run PostgreSQL, the Python NLP gRPC service, and the Go web/River worker process. Key environment variables include:

- `MOUSEION_DATABASE_URL` — PostgreSQL connection string.
- `MOUSEION_NLP_ADDR` — address of the Python gRPC service.
- `MOUSEION_NLP_WARM_LANGUAGE` — optional language pipeline to preload in the NLP service.
- `MOUSEION_ANALYSIS_JOB_TIMEOUT` — maximum duration allowed for an analysis job.

The v1 service is intended for a private home-lab deployment reachable only over Tailscale. See the [README](../README.md) for current setup commands and [documentation governance](documentation-governance.md) for the boundary between this present-state summary, repository ADRs, and planning material.

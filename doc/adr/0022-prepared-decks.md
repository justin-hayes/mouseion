# ADR 0022: Asynchronous deck preparation and durable APKG artifacts

Status: **Accepted; generated-vocabulary eligibility amended by ADR 0072** · Date: 2026-08-24 · Author: Justin + Hermes

## Context

The deck endpoint currently selects vocabulary, performs or queues enrichment, renders an APKG, and begins a browser download in one request. This means the first downloaded package can lack contextual translations, and a download request can perform expensive work or mutate study history.

## Decision

Split deck generation into two phases:

1. A POST creates an owner-scoped asynchronous deck preparation.
2. A GET downloads an immutable prepared artifact only after it reaches `ready`.

Preparation states are:

```text
queued → preparing → ready
                    ↘ failed
queued/preparing ─────→ cancelled
```

The preparation worker selects and quality-gates candidates, runs contextual translation work, renders the final APKG, and commits the artifact, completeness metadata, and generated-vocabulary/card provenance atomically. Failed or cancelled preparations do not permanently exclude vocabulary.

The final APKG is stored as a PostgreSQL `bytea` artifact with owner-scoped metadata. This is chosen over local filesystem storage because the application is self-hosted and may restart or run with multiple instances; database storage keeps preparation metadata and artifact durability in one transactional system. Object storage can replace the bytea implementation later without changing the public workflow.

The download endpoint is a pure read: it performs no provider calls, candidate selection, artifact rendering, or study-history mutation. A ready artifact may be downloaded repeatedly.

## Consequences

- Deck preparation can be retried and observed independently of browser downloads.
- External translation latency is removed from the download request.
- Prepared artifacts consume database storage and require retention/cleanup policy in a later operational enhancement.
- A preparation that succeeds is considered assigned vocabulary even if the user never downloads the file; this preserves the rule that a generated deck assignment excludes future decks.
- Owner isolation, CSRF protection, title-derived filenames, and the `Mouseion::<language>::<book title>` hierarchy remain mandatory.

## Alternatives rejected

- **Synchronous download with a longer timeout:** remains fragile and gives poor progress feedback.
- **Local filesystem artifacts:** unsafe across container restarts and replicas without a separate persistent-volume contract.
- **Regenerate at download time:** makes download non-deterministic and reintroduces provider/database work into the GET request.

# ADR 0003: PostgreSQL as the initial persistence backend

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0001 chose a core-first Go architecture and, in its shared-data-layer scope, called for **SQLite** as the initial persistence (see Decision Register, "Use SQLite for initial persistence": *local, portable, inspectable, appropriate for a personal vocabulary database*). That choice was made while the tool was conceived primarily as a personal, single-user application.

Since then the product has changed shape in ways that matter for the persistence layer:

1. **Multi-user, per-user learning state** ([ADR 0002](0002-multi-user-accounts.md)): two real users with cleanly separated corpora, known vocabulary, and decks. Concurrency and ownership become real.
2. **Admin-managed global reference resources** ([ADR 0002](0002-multi-user-accounts.md)): language-scoped reference data (e.g. DWDS frequency) shared across all users — a second class of data with different ownership semantics.
3. **An async-capable job API** ([ADR 0001](0001-go-core-python-nlp-service.md)): analysis runs as background jobs with IDs and status. v1 uses a synchronous local runner, but the abstraction is deliberately queue-ready, and **background-job coordination is where PostgreSQL's advantages are strongest**.
4. A **web application** as the second client over the same Go core, serving multiple concurrent users.

The question is whether a single-writer, file-backed store (SQLite) is the right foundation for a multi-user, web-served application with durable background jobs — or whether we should start on PostgreSQL, accepting a heavier dependency now in exchange for native concurrency, job primitives, and a growth path we expect to reach soon.

## Decision

**Use PostgreSQL as the initial persistence backend**, replacing SQLite. This supersedes the "Use SQLite for initial persistence" entry in the Decision Register and amends ADR 0001 §1's "persistence: SQLite" choice.

Rationale, in the author's words: Postgres has *superior support for background jobs*, and the project will *grow into Postgres's features sooner rather than later* — so starting there avoids a near-term migration.

### 1. PostgreSQL is the shared core's persistence

The Go core libraries (domain, persistence, candidate selection, vocabulary state, enrichment, card export) read and write PostgreSQL. All learning-state tables carry the per-user owner dimension from ADR 0002; global reference resources are stored as language-scoped, owner-less rows. Migrations are versioned and applied forward on a self-hosted Postgres instance.

### 2. Background jobs get real primitives

Postgres provides the primitives the job abstraction (ADR 0001 §4) needs as it grows from a synchronous local runner toward durable queueing — `SKIP LOCKED` for claimable work, `LISTEN/NOTIFY` for completion signaling, and advisory locks. This is the concrete reason to choose it over SQLite: background-job coordination is first-class rather than bolted on.

### 3. Home-lab self-hosting remains the target

Postgres runs alongside the app in the home lab (containerized or native). This is a deliberate, accepted operational cost: a database service to manage, back up, and version. It does not change the self-hostable design goal — it changes the persistence tier from an embedded file to a managed service.

### 4. Concurrent multi-user access is native

Postgres handles concurrent connections and row-level locking natively, matching the two-user (and growing) reality of ADR 0002 without the single-writer contention SQLite imposes across processes/connections.

## Alternatives considered

- **Keep SQLite (status quo).** Rejected: file-backed and single-writer; no native primitives for durable background jobs or clean multi-user concurrency. The product has moved from personal tool to multi-user, job-driven, web-served application, so SQLite's "portable single file" advantages no longer dominate its limitations.
- **Postgres from the start (chosen).** Accepts a heavier self-hosted dependency now; returns native concurrency, job primitives, JSONB, and a clear growth path (e.g. later full-text/concordance features) without a future migration.
- **A dedicated queue broker (e.g. Redis) alongside SQLite.** Rejected for v1: introduces a second operational dependency before it is needed; Postgres already provides the job primitives we need, keeping the stack to one database.

## Consequences

- **Operational cost:** the home lab now runs and maintains a Postgres service (container/native, backups, versioning). This is the trade-off accepted in exchange for concurrency, jobs, and growth.
- **CLI implication:** the thin Go CLI (ADR 0001) reads/writes the same shared core, so a CLI invocation needs access to the Postgres instance (local or the home-lab server), not a self-contained embedded file. Whether the CLI should support an optional embedded/local fallback is an open question below.
- **Deployment becomes two-tier:** app + database. Alignment with the existing home-lab (which already runs services such as Calibre-Web) makes this natural.
- **Job abstraction strengthened:** the queue-ready API from ADR 0001 §4 is backed by primitives that support it from day one.
- **No data-model migration from SQLite** is incurred — we start on the store we will keep.

## Open questions to resolve before finalizing

- **CLI local operation — resolved.** ADR 0004 makes the web application the sole v1 client, so there is no standalone CLI in v1 and no CLI-specific local/offline persistence mode. If a CLI is ever added, the library boundary in ADR 0004 preserves the option to revisit this.
- **Concrete home-lab deployment — resolved: containerized PostgreSQL.** PostgreSQL runs as a container (docker-compose) in the home lab, alongside the app and Calibre-Web, with a **named volume** for `PGDATA` (not a bind mount into the container filesystem) so data survives container recreation. A scheduled `pg_dump` (host cron or an automated backup task) snapshots to a file for restore. Pin a specific Postgres major version for reproducibility.
- **Migration tooling — resolved: golang-migrate.** Use **golang-migrate** with SQL-based, forward/backward migrations embedded into the Go core for a self-hosted deployment. Reference/reference-data seeding (e.g. DWDS frequency datasets) is a separate seed mechanism, not migrations.

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — this ADR amends its §1 "persistence: SQLite."
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — the ownership model the schema implements.
- [Product specification](../product.md) — Decision Register ("Use SQLite for initial persistence" is superseded here).

# ADR 0004: Web application as the sole v1 client (no standalone CLI)

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0001 chose a **core-first** architecture and, in its build order, put a **thin Go CLI as the first client**, with the web application layered on later. The CLI's stated justifications were:

- validate the core on real corpora before front-loading auth and UI work;
- provide a scriptable, reproducible client;
- keep the pipeline independent of the interface.

Since ADR 0001, the product has evolved in ways that change the calculus:

1. **Auth and multi-user are now foundational, not deferred.** ADR 0002 puts user accounts, authentication, and an admin role into the shared core and the web layer as part of v1 scope. The CLI's central benefit — *avoiding* auth and UI by shipping a headless first client — no longer applies, because auth and the web UI are required regardless.
2. **The persistence backend is PostgreSQL** (ADR 0003). A standalone CLI would now be a *second authenticated client* reaching a multi-user, server-hosted core: it needs its own credential handling, server/connection configuration, and job-waiting story — genuine duplicate work that overlaps the web server's auth and job-serving responsibilities.
3. **The web application is the primary interactive surface** for the intended workflow (OPDS browsing, background-job status, vocabulary review, deck download). The product spec already names it the primary surface, with the CLI retained mainly as a scriptable convenience.

The remaining question is whether the v1 scope should still carry a standalone CLI as a deliverable, or whether the first (and only) client should be the web application, with the core validated by tests rather than by a separate CLI.

## Decision

**Make the web application the sole client in v1. Do not build a standalone CLI.** This supersedes ADR 0001 §4's "thin Go CLI as the first client" and its §5 build order.

### 1. The shared Go core remains a library

The business logic stays in Go libraries (domain, persistence, canonicalization, enrichment, card export) imported by the web application. The core is not inlined into the server; it remains a distinct library boundary so a CLI (or any other client) can be added later without rearchitecting. Dropping the CLI for v1 is a scope decision, not a structural one.

### 2. The core is validated by integration tests

The validation the CLI was meant to provide is achieved by integration tests against the Postgres-backed core (and, where useful, a lightweight API layer), plus the web application itself exercising the pipeline. A separate CLI is not required to prove the core works.

### 3. Build order becomes web-first

1. Python NLP service + shared data layer (the hard, durable part) — unchanged.
2. Web application over the same Go libraries: OPDS browsing, analysis jobs, review/curation, deck download.
3. A CLI may be added later only if scriptable batch automation is genuinely needed; it would reuse the same shared libraries.

### 4. The async-capable job API is unchanged

The service API remains designed around jobs with IDs and status (ADR 0001 §4). The web app is the consumer; the CLI's "wait on a job" behavior, if ever built, maps to the same job abstraction.

## Alternatives considered

- **Keep the thin CLI as the first v1 client (status quo from ADR 0001).** Rejected: it duplicates the auth and job-serving story the web app needs anyway, delays the primary interactive surface, and its "validate core without UI" benefit is superseded by integration testing and the now-foundational auth model.
- **Ship both CLI and web in v1.** Rejected for scope: two authenticated clients and two build/deploy surfaces before the core pipeline is proven. High effort, low incremental value.
- **Keep only the web app (chosen).** Simplest v1 surface, matches the intended workflow, and the core library boundary preserves a future CLI option.

## Consequences

- One interactive surface to build, auth, and test in v1.
- The shared Go core stays an importable library, so a CLI remains a low-friction later addition if scriptable automation is needed.
- Build order: NLP producer + shared data layer, then the web application as the first client.
- Open Question 4 in `product.md` (review workflow: TUI vs exported review file vs web) is effectively resolved toward the web UI, since the web app is now the only client.
- No dedicated CLI authentication, connection, or command-surface work in v1.

## Open questions

- If a CLI is later desired for scriptable automation, should it be a thin client over the web API, or import the Go core directly? (Deferred; not a v1 concern.)

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — this ADR supersedes its §4 "thin Go CLI as the first client" and §5 build order.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md)
- [ADR 0003: PostgreSQL as the initial persistence backend](0003-postgresql-persistence.md)
- [Product specification](../product.md) — Decision Register ("Thin Go CLI first; web app layered on later" is superseded here; "Web app as the primary interactive surface" is promoted to Accepted).

# ADR 0010: Adopt River as the background-job queue

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0001 §4 made the service API **async-capable** — processing runs as jobs with IDs and status — but deferred job *queueing* to a later date, using a **synchronous local runner** for v1 and noting that "background job queueing" was out of scope for the first version. ADR 0003 then chose PostgreSQL and, in its rationale for that choice, called out that Postgres provides the primitives a job abstraction grows into (`SKIP LOCKED`, `LISTEN/NOTIFY`, advisory locks), and that the project would *"grow into Postgres's features sooner rather than later."* Issue #13 still says *"Do not add a distributed job queue in v1."*

Reconsidering this as the analysis pipeline (issues #7, #13, #14, #15) comes online, a hand-rolled job/claim model on raw Postgres primitives is significant work to build and maintain correctly: job claiming, retries, backoff, cancellation, status transitions, idempotency, and worker coordination. **River** is a mature, open-source, **Postgres-native background-job queue for Go** that provides exactly this machinery as a library. Because our persistence is already PostgreSQL (ADR 0003), River fits without introducing any new broker or runtime dependency.

This ADR records the decision to adopt River, superseding the "no distributed job queue in v1" deferral.

## Decision

**Adopt River (`github.com/riverqueue/river`) as the background-job queue for `mouseion`, replacing the planned synchronous local runner and the "defer job queueing" stance.** The analysis pipeline (Stanza NLP jobs from issues #7/#13, and future enrichment/card jobs) runs through River.

### 1. River is Postgres-native — consistent with ADR 0003

River stores its job state in the same PostgreSQL database the core uses, via the `riverpgxv5` driver over the existing `pgxpool` connection pool. It introduces **no separate broker, message bus, or runtime dependency** — a single Postgres instance remains the whole data plane. This directly realizes ADR 0003's rationale that Postgres's job features are a reason to standardize on it.

### 2. River provides the job abstraction as a library

River supplies the machinery ADR 0001 §4 wanted (jobs with IDs and status) plus the operational features we would otherwise hand-roll:

- **Transactional enqueueing** — jobs enqueue atomically with the DB transaction that produces them, so "create a corpus + schedule analysis" is one durable unit.
- **Durable job state** — creation, running, retrying, succeeded/failed states, with per-job status, attempt counts, and error retention; a job handle gives a stable ID for waiting/polling.
- **Retries with backoff** and configurable max attempts.
- **Scheduled jobs** (run at a future time) and **periodic/cron jobs**.
- **Unique jobs** by args/period/queue/state — directly useful for preventing duplicate analysis of the same source (idempotency, ADR 0008's content-hash keying).
- **Leader election and multiple workers** for future scale on the home lab.

### 3. Ownership and authz are preserved

River jobs carry args; `mouseion` wraps job args with the **owning user** and enforces ADR 0009's owner checks at the worker and service layer, just as the persistence layer does. The authenticated-identity model from ADR 0009 / issue #11 is unchanged.

### 4. The web layer consumes jobs, not a sync runner

The web application (ADR 0004) submits analysis jobs to River and surfaces their status/progress in the UI (issue #25), waiting/polling on the job handle. Python/Stanza runs only inside analysis workers (ADR 0001 §2), never in the request-serving path.

## Alternatives considered

- **Synchronous local runner (status quo from ADR 0001 §4).** Rejected for the real pipeline: hand-rolling claiming, retries, cancellation, idempotency, and worker coordination on raw Postgres is substantial, error-prone work that River already solves and maintains. Deferring a queue to "later" would force a migration of the job model once concurrency and retries are actually needed.
- **A separate broker (Redis, NATS, etc.) alongside Postgres.** Rejected: introduces a second operational dependency, contradicting ADR 0003's single-Postgres design. River is Postgres-native and avoids this.
- **Hand-rolled job tables + `SKIP LOCKED`/`LISTEN/NOTIFY` directly.** Rejected: River is a maintained implementation of exactly this pattern (it uses Postgres primitives under the hood), saving custom code and giving production-grade retries, scheduling, and uniqueness for free.

## Consequences

- Issue #13's "Do not add a distributed job queue in v1" is **superseded**: River is adopted in v1.
- ADR 0001 §4's "synchronous local runner" and "defer job queueing" are amended; the async-capable job API is now backed by River.
- The analysis job API (issue #13) is implemented against River: job creation, status, wait/poll, ownership, retries, and idempotency.
- The analysis worker invokes the Python/Stanza producer (issue #7); Python remains ingest-time only.
- One Postgres instance remains the data plane (jobs + core + reference data), keeping home-lab operations simple (ADR 0003 §3).
- A new dependency (`github.com/riverqueue/river` + `riverpgxv5`) is added to the Go module, and River's schema migrations run alongside the app migrations.

## Open questions

- Whether the web status UI (issue #25) polls River job handles or subscribes to completion (e.g. via `LISTEN/NOTIFY`), and how progress/percent is surfaced for long analysis jobs.
- Whether non-analysis work (e.g. enrichment, card generation) should also run through River or remain inline (ADR 0007 chose inline enrichment); likely keep enrichment inline and route only genuinely async analysis through River.

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — §4 async-capable job API (this ADR supersedes its "sync runner / defer queueing" note).
- [ADR 0003: PostgreSQL as the initial persistence backend](0003-postgresql-persistence.md) — the Postgres job primitives rationale this realizes.
- [ADR 0009: Home-lab authentication and corpus-artifact isolation](0009-home-lab-auth-corpus-isolation.md) — ownership model preserved through job args.
- [Product specification](../product.md) — updates the Decision Register.

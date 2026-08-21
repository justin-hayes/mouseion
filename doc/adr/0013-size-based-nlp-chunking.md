# ADR 0013: Size-based NLP analysis chunking

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0011 adopted gRPC as the Go↔Python NLP transport. The River analysis job (issue #13) currently sends a **whole book's full text in one `Analyze` RPC**, and the Python Stanza service analyzes it in one `Pipeline` invocation.

Empirical testing against the real service (German, Stanza 1.14) shows two hard limits on a single call:

| Input (chars) | Sentences | Time | Result |
|---|---|---|---|
| 1,170 | 15 | 0.8s | OK |
| 11,700 | 150 | 5.9s | OK |
| 117,000 | 1,500 | 53.7s | OK |
| 585,000 | ~7,500 | >180s | **DeadlineExceeded** |

- **Time/deadline:** analysis scales roughly linearly (~54s per 100k chars). A full-length book (~500k+ chars) exceeds the 180s call deadline.
- **gRPC message size:** the response is ~20 bytes/char, so a full book (~10 MiB) exceeds gRPC's default **4 MiB** per-message limit.

These are properties of *how much work is sent in one call*, not of gRPC itself. A different transport would hit the same walls. The fix is to **chunk the analysis**, not to replace the transport.

## Decision

**Chunk the source text into fixed-size segments and run one `Analyze` RPC per segment**, aggregating the per-segment `NormalizedCorpus` results into a single corpus for downstream processing.

### 1. Chunking unit — size-based

Chunk by **character/word size with a maximum**, not by book structure:

- A maximum segment size (default **~100k characters**, tunable) keeps each call well within the safe time (~54s) and message-size (~2 MiB) bounds.
- Split on **sentence boundaries** where possible (so no sentence is torn across segments); fall back to a hard character cut only if a single sentence exceeds the max.
- This handles all cases uniformly: normal chapters, **huge chapters** (split), and **chapter-less books** (the whole text is one chunk stream). Per-chapter alignment is a *nice-to-have* only; the downstream pipeline consumes the **aggregated** normalized corpus.
- Reproducibility is preserved: every token retains its `SourceLocation` (from the EPUB parser, issue #14), so per-chunk offsets remain meaningful regardless of chunk boundaries.

### 2. Orchestration — in the River worker

The River analysis worker (issue #13) becomes a **loop over chunks**:

- Split the source text into chunks.
- For each chunk, call `Analyze(chunk)` via the gRPC client.
- Update job **progress** per chunk (N/M) — surfaced by the web job-status UI (#25).
- On failure, the River job retries (ADR 0010); a per-chunk failure marks the job failed with an actionable error (idempotent re-analysis on retry).

### 3. Transport — unchanged

gRPC stays (ADR 0011); River stays (ADR 0010). This ADR only changes *how much* each RPC carries. The unary `Analyze` RPC contract is unchanged. (Server-streaming was considered and deferred — see Alternatives.)

## Alternatives considered

- **Chapter-level chunking.** Rejected as the *only* strategy: it fails on books with huge chapters and on chapter-less books (which the EPUB parser flattens to one giant chapter). Kept only as a secondary alignment hint at most; size-based chunking is primary.
- **Server-streaming `Analyze` (stream of per-sentence results).** Deferred: gives incremental progress but requires a new proto RPC and doesn't solve the time/size wall better than chunking. Revisit only if sentence-level progress becomes a requirement.
- **A separate message broker (Kafka/RabbitMQ/NATS).** Rejected: the bottleneck is Stanza analysis, not transport; River (Postgres-backed) already provides durability, retries, and decoupling without a new runtime dependency (ADR 0003, 0010).
- **Raising the gRPC message-size limit / call timeout only.** Insufficient alone: doesn't bound server memory or the deadline for very long books; chunking does.

## Consequences

- The River analysis worker splits source text into size-capped chunks, calls `Analyze` per chunk, aggregates results, and reports per-chunk progress.
- Downstream selection/ranking/sentence-selection consume the **aggregated** corpus (their current inputs are unchanged).
- No proto contract change in v1; `SourceLocation` preserves reproducibility across chunks.
- The max chunk size is configurable/tunable.

## Related

- [ADR 0011: gRPC as the Go↔Python transport](0011-grpc-go-python-transport.md) — unchanged.
- [ADR 0010: Adopt River as the background-job queue](0010-river-job-queue.md) — the worker orchestrates chunking.
- [ADR 0003: PostgreSQL persistence](0003-postgresql-persistence.md) — single data plane; no broker.
- [Product specification](product.md) — updates the Decision Register.

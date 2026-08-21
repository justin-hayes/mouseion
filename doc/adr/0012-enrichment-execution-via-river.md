# ADR 0012: Enrichment execution — local inline, external translation via River

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

ADR 0007 §5 decided that enrichment runs **inline in Go** — not job-based — reasoning that enrichment consumes a finite, curated candidate set, is cached (retries cheap/idempotent), and is bounded. It treated the async job abstraction as reserved for the slow, per-book corpus-analysis step (Stanza).

Two later decisions and the shape of the real pipeline change this calculus:

1. **ADR 0010 adopted River as the job queue** and explicitly named "future enrichment/card jobs" as running through River. ADR 0011's open questions likewise noted that enrichment might route through the job abstraction.
2. In practice, enrichment splits into two very different workloads:
   - **Local providers** (frequency from DWDS, morphology, rule-based IPA pronunciation) are instant, deterministic, and local — trivially inline.
   - **External translation** (LLM default / dictionary) is slow, fallible, and can involve **one call per candidate word**. For a corpus with hundreds of candidate words, that is a genuinely long, failure-prone operation — the kind the web UI (#25) wants to show as a **background job with progress, retries, and cancellation**, not a blocking inline loop.

The question is whether the v1 execution model should remain "all inline" (ADR 0007) or reflect this split.

## Decision

**Split enrichment execution: local providers run inline in Go; bulk external-translation enrichment runs as a River background job.** This supersedes ADR 0007 §5's "enrichment runs inline in Go, not job-based" for the external-translation path.

### 1. Local providers — inline (unchanged)

Frequency (DWDS), morphology (from the candidate/corpus), and rule-based IPA pronunciation are instant, deterministic, and local. They run **inline** in the candidate-processing path, exactly as ADR 0007 intended. No queue involvement.

### 2. External translation — River job

The bulk external-translation enrichment of a candidate set runs as a **River job**:

- A **bulk enrichment job** is enqueued (transactionally) for a curated candidate set that needs external translation.
- A **River worker** processes the candidate set: for each candidate, it calls the external translation provider (LLM/dictionary) with retries/backoff, writes to the immutable, language-scoped, cross-user **enrichment cache** (from #19 / ADR 0007), and records provenance.
- The job exposes **status/progress/retries/cancellation** via River, which the web UI (#25) can surface.
- The external-translation **provider interfaces, privacy controls (lemma + one sentence, lemma-only mode, admin/user opt-in), and caching** from #19 / ADR 0007 are unchanged — only the orchestration moves from inline to a River worker.

### 3. No-external-provider configuration remains fully supported

If external translation is disabled (or no provider configured), the enrichment job is a no-op / not enqueued, and the pipeline runs entirely with local providers inline — consistent with ADR 0007.

## Alternatives considered

- **Keep all enrichment inline (ADR 0007 status quo).** Rejected: bulk external translation of hundreds of words is slow and failure-prone; blocking it inline in the request/review path is poor UX and lacks progress/retry/cancellation. River provides these for free (ADR 0010).
- **Route local providers through River too.** Rejected: local providers are instant/deterministic; queueing them adds needless latency and coordination with no benefit.
- **A separate broker just for enrichment.** Rejected: River is already in place (ADR 0010); no new dependency.

## Consequences

- The enrichment pipeline (#19) keeps its provider interfaces, local inline providers, external translation provider, cache, provenance, and privacy controls; the **orchestration of bulk external translation moves to a River worker**.
- The web UI (#25) can display enrichment job status/progress/retries/cancellation.
- ADR 0007 §5 is amended: "inline in Go" now applies to local providers; external translation is job-based.
- One Postgres instance remains the data plane (River jobs + cache + core).

## Open questions

- Whether a single bulk-enrichment job or one job per candidate (for finer-grained retry/progress) is preferable — likely a single job over the set with per-item progress, but this can be tuned during #25.

---

## Related

- [ADR 0007: Enrichment providers, caching, and privacy policy](0007-enrichment-providers-caching-privacy.md) — §5 amended by this ADR (inline → local inline, external via River).
- [ADR 0010: Adopt River as the background-job queue](0010-river-job-queue.md) — the job queue used for external translation.
- [ADR 0005 / 0008] — identity + frequency; caching key.
- [Product specification](product.md) — updates the Decision Register.

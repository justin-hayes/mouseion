# ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer

Status: **Accepted** · Date: 2026-08-19 · Author: Justin + Hermes

## Context

`mouseion` is a self-hosted reading environment for advanced foreign-language reading. The desired workflow is: browse an OPDS/Calibre-Web library, ingest an EPUB, analyze it linguistically in the background, surface vocabulary ranked by frequency with representative example sentences, let the learner curate the list, and export an Anki deck. The long-term focus is reading at an advanced level, with reading aids and concordance as later enhancements.

Two design questions dominate the architecture:

1. **Interface-first vs. core-first.** The product has both a scriptable CLI and a server-side web application in mind. Which shapes the architecture?
2. **The Go/Python boundary.** The stated preference is to implement as much as possible in Go, calling a Python service only where NLP libraries (Stanza) genuinely require it. Where is that line?

These questions were already resolved for the closely related Vocabulary Acquisition Tool (its ADR 0001, 2026-08-18). `mouseion` realizes the fuller reading-environment vision that the vocab tool's web-app roadmap describes, so it adopts the same architecture.

## Decision

**Adopt a core-first architecture: a shared Go application core, with Python confined to a coarse, ingest-time NLP producer. Build a thin Go CLI as the first client; lay the web application on top later.** *(The "thin Go CLI as the first client" and the §5 build order are superseded by [ADR 0004](0004-web-only-v1-client.md), which makes the web application the sole v1 client while keeping the Go core a shared library.)*

### 1. Shared Go libraries

Put the application's business logic in Go libraries imported by **both** the CLI and the future web application:

- domain: corpus models, candidate selection, ranking, deduplication, filtering, vocabulary state
- persistence: PostgreSQL *(was SQLite; amended by [ADR 0003](0003-postgresql-persistence.md))*
- canonicalization: language/locale normalization profiles (e.g. `daß` → `dass`), applied to the *raw* lemma returned by Python
- enrichment: translation, gloss, frequency lookups, LLM calls where appropriate
- card export: Anki-compatible CSV

The web app is not a separate system — it is a second client over the same Go libraries.

### 2. Python is a batch, ingest-time NLP producer — not a live dependency

Python exists **only** because Stanza has no viable Go binding. Its scope is the raw NLP the model actually performs:

- sentence segmentation, tokenization, raw lemmatization, POS tagging, morphology, named-entity detection

It runs **only during the analysis step**: it turns a source book into a normalized corpus, persists that artifact, and exits. All downstream logic (selection, ranking, filtering, enrichment, card generation) runs in Go against the persisted corpus.

Consequence: the Go web server never needs Python running to serve pages or answer requests. Python is touched only by background analysis jobs.

### 3. The contract is a typed, versioned schema (Protobuf)

The boundary between the systems is a versioned, typed schema of the normalized corpus: sentences plus tokens, each carrying surface form, raw lemma, POS, morphology, and source-location metadata (needed for reproducible sentence selection).

Use **Protobuf** so the same `.proto` defines both the RPC messages and the persisted corpus file format — one schema, two uses, eliminating drift between the Go and Python sides.

### 4. Service contract is async-capable and profile-based from day one

- **Async-capable:** the service API is designed around jobs with IDs and status, even though the CLI calls synchronously. The CLI waits on a job instead of polling.
- **Profile-based, not account-based:** languages, word lists, and OPDS config are modeled as a "profile" in shared persistence. The CLI uses one; the web manages many. This defers the auth/registration question entirely — no accounts in v1. *(Superseded by [ADR 0002](0002-multi-user-accounts.md), which adopts a multi-user account model with an admin role; see the 2026-08-20 session.)*

### 5. Build order

1. Python NLP service + shared data layer (the hard, durable part).
2. Web application as the sole first client over the shared Go core: OPDS browsing, background jobs, review/curation, and deck download. *(Changed from "Go CLI as a thin client" by [ADR 0004](0004-web-only-v1-client.md).)*
3. A CLI may be added later only if scriptable automation is needed, reusing the same Go libraries.

## Alternatives considered

- **Python-first CLI.** Keeps one runtime, but clashes with the stated Go preference and makes the eventual web app a Python rewrite rather than a second client.
- **Go app calling Python per-operation.** Couples Go's runtime to Python for every request and invites per-word round-trips. Rejected: the boundary must be coarse and batched.
- **Full web app first.** Front-loads auth, background queueing, and UI before the core pipeline is validated. Rejected for the first version.
- **spaCy or another Python NLP with Go bindings.** Bindings are immature; Stanza is the justification for Python at all. Not adopted.

## Consequences

- Two runtimes, two build systems, two deploy models, and a contract to keep in sync — the accepted cost of Stanza. Mitigated by keeping the boundary coarse (whole-book batch jobs, never per-word calls).
- Python is not needed at runtime by the web server, which keeps home-lab operations simple.
- The CLI remains useful as a scriptable client even after the web app exists; the web UI becomes the interactive review surface.
- Defers auth, background job *queueing* (build the job abstraction, use a synchronous local runner for v1), and the OPDS *browsing UI* (OPDS as a corpus source is worth having early; the browser is web-only). *(The sync local runner is superseded by [ADR 0010](0010-river-job-queue.md), which adopts River as the job queue; OPDS browsing remains web-only.)*
- Roadmap items (KOReader sync for encounter-count metrics, cross-text concordance, reading environment) are out of v1 scope.

## Open questions to resolve before finalizing

- Whether "user registration" is genuinely needed or whether profiles suffice for a personal tool. Default was profiles only in v1; **resolved by [ADR 0002](0002-multi-user-accounts.md)** in favor of user accounts with an admin role.
- Exact Protobuf schema shape for the normalized corpus.
- Whether enrichment runs inline in Go or is itself job-based.

---

## Related

- [README](../README.md)
- [Product specification](product.md)
- Vocabulary Acquisition Tool: ADR 0001 (same decision), Decision Register, sessions 2026-07-19/20 and 2026-08-18.

# ADR 0007: Enrichment providers, caching, and privacy policy

Status: **Accepted** · Date: 2026-08-21 · Author: Justin + Hermes

## Context

After candidate selection and ranking (ADR 0005), remaining vocabulary items are enriched with external information to make study material useful: **translation, gloss, frequency, morphology, and pronunciation** (product.md "Enrichment"). The spec notes LLMs "may be used where appropriate but should not be required for every enrichment task," and flags an open privacy question: what may leave a self-hosted home lab, and under what consent.

These were Open Questions 6 and 9 in `product.md`, consolidated as issue #28.

The constraints that shape the decision:

- **Self-hosted, multi-user** (ADR 0002): the app runs on the author's home lab; two users with per-user learning state.
- **Identity is `(language, canonical lemma, UPOS)`** (ADR 0005) — the natural key for caching and provenance.
- **Global frequency data (DWDS) is admin-managed and language-scoped** (ADR 0002 §3) — available locally as reference data, not an external call.
- **The core-first architecture** (ADR 0001) keeps NLP analysis (Stanza) as an ingest-time job, but enrichment of a curated candidate set is a distinct, smaller operation.

## Decision

### 1. Enrichment field classification

| Field | Provider | Local/external | Required? |
|-------|----------|---------------|-----------|
| Frequency | DWDS (admin-loaded reference data) | **Local** lookup | yes |
| Morphology | Stanza (ingest-time analysis) | **Local** | yes |
| Pronunciation | Rule-based IPA mapping | **Local** | optional |
| Translation | LLM (default) or a configured dictionary provider | **External** | optional |
| Gloss | Bundled with translation | **External** | optional |

- **Frequency** is the *global* DWDS frequency (across the language corpus), a local lookup against the admin-loaded dataset (#12/#30) — not an external call. *(In-corpus frequency is a separate, always-local selection/ranking statistic per ADR 0005; it is not an enrichment field and never leaves the lab.)*
- **Morphology** is already produced at ingest by Stanza; it is read from the normalized corpus.
- **Pronunciation** is deterministic rule-based German→IPA in v1; optional.
- **Translation (and gloss)** is the **only** enrichment field that requires an external provider. The v1 default is an **LLM**; a user may configure a dictionary provider instead, or disable translation entirely.

### 2. External providers are optional and behind an interface

- The core defines an **enrichment provider interface**. The LLM is the default v1 implementation; a dictionary provider is a configurable alternative.
- **Nothing depends on enrichment.** A user can run the entire pipeline with external providers disabled — everything except translation still works. This satisfies the spec's "LLMs not required for every task."
- Enrichment is applied per-item; a user who already knows a word's meaning can omit translation for that item.

### 3. Caching and provenance

- Every enrichment result is **cached**, keyed by **`(language, canonical_lemma, upos, provider, provider_version)`** — the ADR 0005 identity plus the provider that produced it.
- Cache keys are **language-scoped**, so cached translations are **shared across users** of the same language (two users studying the same German lemma don't each burn an LLM call).
- Cached results are **immutable and versioned**; a provider-version bump invalidates stale cache entries. Results are **never silently re-derived**.
- **Provenance** (provider, provider version, cache time) is persisted alongside every enrichment field, so the review UI can show e.g. *"translation by LLM-X v2, cached 2026-08-21"* — the same inspectability principle as ADR 0005's score components.

### 4. Privacy — what may leave the home lab

- In sentence-context mode, **only the target lemma, its tested surface form, and the one example sentence** are sent to an external provider, and **only for translation/context selection**. The full document, surrounding text, document title, user identity, reading history, and any other corpus/learning data **never leave the lab**.
- The example sentence and tested surface are included specifically for **sense disambiguation** and exact-span validation. A user can configure **lemma-only** context (neither sentence nor tested surface) for stricter privacy.
- **External providers are opt-in, per-user, admin-configured.** An admin enables a provider and supplies credentials; each user can enable/disable translation. Provider privacy policies are surfaced where relevant.
- **A no-external-provider configuration is fully supported** (see §2): the entire pipeline runs with translation off, frequency/morphology/pronunciation all local.

### 5. Execution — local providers inline; external translation via River

- **Local providers** (frequency, morphology, pronunciation) run **inline in Go** as part of the core's candidate-processing path.
- **External translation** of a bulk candidate set runs as a **River background job** (retries, progress, cancellation), per [ADR 0012](0012-enrichment-execution-via-river.md). This amends the earlier "all inline, not job-based" stance for the external-translation path.
- Rationale: local providers are instant/deterministic; external translation is slow and failure-prone (one call per candidate), better served by the job abstraction (ADR 0010).

The OpenAI-compatible implementation defaults to low reasoning effort. It sends
the `reasoning_effort` field only for recognized OpenAI reasoning models or an
endpoint/model explicitly marked as supporting that field, and omits
`temperature` for those requests. Unknown and self-hosted endpoints retain the
legacy request shape for compatibility. Its prompt requires one concise JSON
object with exactly the translation fields; it does not rely on a provider-wide
verbosity parameter.

## Alternatives considered

- **Enrichment through the job abstraction.** Rejected: adds coordination overhead for a bounded, cached operation that runs inline in the core; the job API stays for slow ingest-time NLP.
- **Dictionary provider as the v1 default instead of LLM.** Deferred: a dictionary integration is a per-provider auth/rate-limit integration; the LLM is one generic provider and is easily replaced via the interface. A dictionary remains a configurable alternative.
- **Send full sentence context + document metadata to the LLM.** Rejected on privacy: only the lemma, tested surface form, and one example sentence leave the lab; no document/user metadata.
- **Per-user caching (no cross-user sharing).** Rejected: wasteful — two users studying the same German lemma should not each incur an LLM call. Cache key is language-scoped and provider-versioned.
- **Make translation required.** Rejected: a user who already knows a word's meaning (or runs fully offline) must be able to use the tool without it.

## Consequences

- The enrichment pipeline (issue #19) implements the provider interface, caching, and provenance; translation is the only external-capable field.
- In sentence-context mode, only the lemma, tested surface form, and one example sentence may transit an external provider, under admin-configured, per-user opt-in; lemma-only mode sends only the lemma identity.
- Cached translations are shared across users per language, keyed and versioned by ADR 0005 identity.
- Enrichment runs inline in the Go core; the job abstraction is unchanged (still for ingest-time NLP).
- Open Questions 6 and 9 in `product.md` are resolved; #28 can be closed. Issues #19 and #20 are updated per the decision.

## Open questions

- Whether to ship a built-in dictionary provider (e.g. dict.cc / Leo / Wiktionary) as a first-class alternative in a later iteration.
- Whether pronunciation should extend beyond German (rule-based IPA) as other languages are added.

---

## Related

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — identity key for caching/provenance; distinguishes global vs. in-corpus frequency.
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](0002-multi-user-accounts.md) — per-user opt-in, global DWDS reference data.
- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — core-first; job API for ingest-time NLP.
- [Product specification](product.md) — resolves Open Questions 6 and 9; updates the Decision Register.

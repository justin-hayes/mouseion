# ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources

Status: **Accepted** · Date: 2026-08-20 · Author: Justin + Hermes

## Context

ADR 0001 assumed the product could start with a "profile-based, not account-based" model — languages, word lists, and OPDS config modeled as a *profile* in shared persistence, deferring auth and registration entirely. The rationale was that for a personal, single-user tool, per-language profiles were sufficient, so v1 would have no accounts.

Two decisions from the 2026-08-20 session change that assumption:

1. **Real multiple users are imminent.** The app will be used by two people in short order (the author and his wife). Language features must be cleanly separated **per user**: corpus, known vocabulary, curated/generated vocabulary, Anki decks, and so on. Because user separation is required this soon, it should be designed in from the start rather than retrofitted.
2. **Word frequency data is a global, admin-managed resource.** The app should support adding language frequency data (concrete example: DWDS publishes frequency data over its German corpus). This is an *administrative* feature, independent of individual users, applied **globally across all users** of that language. From the most frequent words we understand a language's core vocabulary; users can see how much core vocabulary they already know and how much a given book would expose them to.

The second decision only makes sense in a multi-user world: a single frequency dataset is shared reference data for every user of a language, not per-user state. That is the key architectural distinction this ADR captures.

## Decision

**Adopt a user-account model with per-user learning state, plus an admin role that manages global, language-scoped reference resources.** This supersedes ADR 0001 §4's "profile-based, no accounts in v1."

### 1. User accounts and authentication

- Introduce **user accounts** and authentication as part of the shared core and the web layer, not deferred to post-v1.
- Each user has a stable identity and owns their learning state.
- The Go CLI still works for personal automation: it authenticates as a user (or runs against a single local user) rather than a nameless profile.

### 2. Per-user learning state

All learner-owned data is scoped to a user:

- corpora (imported/owned books and their analysis)
- known-vocabulary lists
- candidate, accepted, ignored, and generated vocabulary
- curated example-sentence choices
- Anki deck exports / card generation state

Persistence adds an ownership (user) dimension to these aggregates.

### 3. Admin-managed, global reference resources

- A small **admin role** manages resources that are global and language-scoped, shared by all users of that language.
- Initial example: **word frequency datasets** — e.g. data derived from DWDS German frequency analysis, uploaded once by an admin and usable by every user studying that language.
- These reference resources drive **priority vocabulary** in ranking: a word that is highly frequent in the language can be surfaced for study *even when it appears rarely in a given book* (high-frequency cards from low in-corpus usage).
- Global reference resources are language-scoped (a German frequency dataset does not leak into French).

### 4. Data model consequence

- **Learning state** tables carry a user/owner dimension.
- **Reference resources** (frequency datasets, shared priority lists) are global rows scoped by language, not owned by any user.
- Immutable NLP artifacts (the normalized corpus) may be shared/referenced rather than duplicated per user.

### 5. Build-order impact

The account model is foundational to persistence, so it lands with the shared data layer rather than waiting for the web UI. This does not change the core-first direction of ADR 0001 — it adds a user/admin dimension to the core the CLI and web app both share.

## Alternatives considered

- **Keep the ADR 0001 profile-based model (status quo).** Rejected: with two real users, nameless profiles cannot cleanly separate "author" from "spouse" or attribute learning state; per-user corpora, known words, and decks are indistinguishable. Multi-user is imminent, not speculative.
- **Multi-user accounts without an admin role / global resources.** Rejected: DWDS frequency data is shared across all users of a language. Duplicating it as per-user state is wasteful and misrepresents it as learner-owned data rather than a global reference.
- **Defer frequency data to post-v1.** Rejected: frequency/priority lists are already a core ranking input in the spec, and a high-frequency-card capability is cheap and high-value. Designing its global scoping now avoids a later data-model migration.

## Consequences

- Auth and user management are in v1 scope for the web layer; ADR 0001's "defers auth" note is amended for the account model (background *job queueing* remains deferred).
- The persistence schema distinguishes per-user learning state from global, language-scoped reference resources.
- An admin ingestion path is required for uploading frequency datasets (DWDS), and the ranking pipeline gains a "global frequency membership" signal usable independent of in-corpus frequency.
- A small extra scope cost in v1 (account model + admin role) is accepted in exchange for avoiding a retrofitted migration as the user base grows.

## Open questions to resolve before finalizing

- Exact auth mechanism and where credentials live in a self-hosted home-lab deployment (local accounts vs. e.g. an external identity provider).
- Whether corpus NLP artifacts are truly shareable across users or must be analyzed per user (licensing/privacy of a user's private EPUBs).
- The concrete DWDS data source(s) and upload format for the frequency dataset.

---

## Related

- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](0001-go-core-python-nlp-service.md) — this ADR supersedes its §4 "no accounts in v1."
- [Product specification](product.md)
- Vocabulary Acquisition Tool: session 2026-08-20 (source of both decisions); ADR 0001 (same decision set).

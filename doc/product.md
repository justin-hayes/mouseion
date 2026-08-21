# mouseion — Product Specification

> Working title for the self-hosted reading environment for advanced foreign-language reading. This document consolidates the reading-app concept from the Obsidian 2026-08-17 braindump and the Vocabulary Acquisition Tool sessions (2026-07-19/20, 2026-08-18), which describe the fuller server-side web application. It follows the same document structure as the Vocabulary Acquisition Tool spec.

---

# Problem Statement

Reading at an advanced level in a foreign language means encountering many words that are worth learning. Most readers have to stop, look words up, and manually build study material — a slow, manual pipeline that competes with actual reading time.

`mouseion` is a self-hosted reading environment that automates that pipeline: it analyzes the books you already want to read, surfaces the vocabulary worth studying (ranked by frequency, each word tied to a representative example from the text), lets you curate the result, and exports it to an SRS such as Anki.

The goal is to minimize time spent managing vocabulary and maximize time spent reading authentic material.

---

# Design Goals

The application should:

- work from authentic reading material browsed from a self-hosted library (Calibre-Web over OPDS)
- analyze books in the background and return vocabulary ranked by frequency, each item with an example sentence chosen from the text
- keep the learner in control: mark words as known, choose between multiple example sentences, omit or add items
- export the curated result to Anki
- support multiple languages of study with pluggable NLP
- be self-hostable on a home lab
- implement as much as possible in Go, calling a Python service only where NLP genuinely requires it
- keep the processing pipeline independent of the interface, so the same core can serve the web app and, later, other clients

---

# Non-Goals

The first version will not attempt to:

- replace Anki or teach grammar
- automatically determine whether the learner truly knows a word
- generate complete language courses
- be a general-purpose ebook reader (reading aids beyond vocabulary are a roadmap item)
- (roadmap) KOReader progress sync, corpus-wide concordance, and a full in-text reading environment are explicitly out of scope for v1

---

# User Workflow

A typical workflow is:

1. Register an account (admins additionally manage global resources such as language frequency data).
2. Configure a profile for a language of study.
3. Upload (or reference) word lists of known vocabulary.
4. Configure a connection to an OPDS server.
5. Browse the library and choose a book for analysis (EPUB for now).
6. Analysis runs in the background.
7. Receive the text's vocabulary sorted by frequency, each word with a representative example chosen from the text.
8. Customize the list: mark words as known to omit them, change which sentence is representative if several exist, etc.
9. Download the edited list as an Anki deck.

---

# Inputs

## Corpus

One or more EPUBs from an OPDS library. Future: other formats (PDF, plain text, OCR output, HTML).

## Known Vocabulary

Per language-of-study word lists of words already considered learned. Sources: manually maintained lists, exports from previous runs, Anki decks, CEFR/priority lists. Scoped per user.

## Global Reference Data (admin-managed)

Frequency and priority resources that apply across **all** users of a language, managed by an admin rather than individual users. Concrete example: word frequency data derived from the DWDS German corpus. From the most frequent words a language's core vocabulary is understood; users can see how much core vocabulary they already know and how much a given book would expose them to, and can generate study cards for high-frequency vocabulary even when a word appears rarely in a specific book.

## OPDS / Library Connection

Configuration pointing at a self-hosted Calibre-Web OPDS server for browsing and ingestion.

---

# Processing Pipeline

## Ingest & Linguistic Analysis

Each book is analyzed using language-specific NLP tools (tokenization, lemmatization, POS tagging, morphology, sentence segmentation, optional named-entity detection). Output is a normalized corpus persisted for downstream use.

## Vocabulary Selection & Ranking

Words are selected and ranked according to configurable rules (e.g. appears at least N times, belongs to a priority list, part of speech, exclude proper nouns), excluding known vocabulary and previously handled items. Global reference data (e.g. DWDS frequency) supplies a language-frequency signal so a word highly frequent in the language can be surfaced for study even when it appears rarely in a given book.

## Enrichment

Remaining candidates are enriched with external information — translation, gloss, frequency, morphology, pronunciation. LLMs may be used where appropriate but should not be required for every enrichment task.

## Sentence Selection

For each item, choose the most useful example sentence from the text (understandable from context, not excessively long, representative usage).

## Review / Curation

The learner reviews candidates: accept, omit, or edit the example sentence. The curated state persists so future runs avoid duplicating work.

## Export

Produce an Anki-compatible deck of the accepted, curated vocabulary.

---

# Outputs

## Anki Deck

CSV compatible with Anki (front: source sentence / cloze / hint; back: full sentence, translation, target word, lemma, POS, morphology, source document, notes).

## Vocabulary Database

Persist known vocabulary, generated cards, and processing history so future runs accumulate rather than reset.

---

# Architecture (Initial)

The first implementation is **core-first**: a shared Go application core with a coarse, ingest-time Python NLP producer, per the architecture ADR. The **web application is the sole v1 client** over the same Go core; no standalone CLI is built in v1 (see the architecture ADRs).

The product uses a **multi-user account model**: learning state (corpus, known vocabulary, curated/generated vocabulary, decks) is scoped per user, while an **admin role** manages global, language-scoped reference resources (frequency datasets, shared priority lists). See the architecture ADRs.

## Component Boundaries

- **Core library (Go):** corpus models, linguistic-analysis contracts, normalization, candidate selection and ranking, vocabulary state, persistence (PostgreSQL), enrichment, card export. Includes the user/ownership dimension on learning state and admin-managed global reference resources.
- **NLP producer (Python):** batch, ingest-time NLP only (Stanza), producing a typed, versioned normalized-corpus artifact (Protobuf) consumed by the Go core.
- **Import adapters:** EPUB extraction, known-vocabulary imports (CSV, Anki), priority-list imports.
- **One-off scripts:** personal migrations and cleanup, kept out of the stable library API.

## Initial Processing Model

```text
OPDS library → choose book → ingest EPUB
   ↓
background NLP analysis (Python) → normalized corpus
   ↓
candidate selection & ranking
   ↓
review / curation (learner)
   ↓
Anki deck export
```

## Implementation Stack

- **Go** for the core, persistence (PostgreSQL), and the web server.
- **Python** (Stanza) for ingest-time NLP only, behind a project-defined language-analyzer interface.
- **Protobuf** as the typed, versioned contract between Go and Python.
- **Web UI (v1):** the sole client — server-side rendered with JavaScript enhancement (HTMX, Alpine); if requirements demand a full client-side app, prefer Svelte.

---

# Decision Register

This register identifies decisions stable enough to promote to repository documentation and, where noted, to record as ADRs. The repository ADRs explain context, decision, alternatives, and consequences; this table is the planning index.

| Decision | Status | Rationale | ADR candidate |
| --- | --- | --- | --- |
| Core-first: shared Go core used by all clients | Accepted; ADR written | One core, multiple clients; web app (v1) and any future CLI share the same Go libraries. | `0001` |
| Python is an ingest-time NLP producer, not a runtime dependency | Accepted; ADR written | Stanza has no viable Go binding; confined to raw NLP at ingest time. | `0001` |
| Coarse, typed, versioned contract between Go and Python (Protobuf) | Accepted; ADR written | One schema defines RPC messages and the persisted corpus format. | `0001` |
| Thin Go CLI first; web app layered on later | Superseded by ADR 0004 | No standalone CLI in v1; the web app is the sole client over the shared Go core. | `0004` |
| Service API is async-capable (jobs) from day one | Accepted; ADR written | Designed around jobs with IDs/status; the web client tracks job state. | `0001` |
| Multi-user accounts with per-user learning state | Accepted; ADR written | Two real users (author + spouse) need cleanly separated corpus, known words, and decks. | `0002` |
| Admin-managed global reference resources (frequency data) | Accepted; ADR written | Global language data (e.g. DWDS frequency) shared across all users; admin uploads, users consume. | `0002` |
| Persist vocabulary state across corpora | Accepted | The learner model accumulates over months/years. | Yes |
| Separate candidate generation/review from card generation | Accepted | Learner review prevents low-value cards. | Yes |
| Use PostgreSQL for initial persistence | Accepted; ADR written | Native concurrency, job primitives (SKIP LOCKED, LISTEN/NOTIFY), growth path for multi-user + background jobs; supersedes the earlier SQLite choice. | `0003` |
| Use Stanza as the initial NLP backend behind a project-defined interface | Accepted; revisit after evaluation | Multilingual; Python exists only for Stanza. | Yes |
| Preserve source spelling while matching canonical, locale-aware lemmas | Accepted | Avoids duplicates without altering source or conflating lexemes. | Yes |
| Vocabulary identity, normalization profiles, ranking defaults (sense-agnostic, global-first) | Accepted; ADR written | Identity `(lang, canon lemma, POS)`, sense-agnostic; one conservative versioned German profile; global-first ranking blend. | `0005` |
| Anki export + known-vocab import contracts (TSV, custom Cloze note type, lemma-list import) | Accepted; ADR written | UTF-8 TSV; identity-based dedup key first field; v1 import = per-language lemma list. | `0006` |
| Use Calibre-Web / OPDS as a corpus source | Proposed | Reuses home-lab infrastructure as a browseable source. | Possibly |
| v1 is German-first behind the pluggable NLP boundary | Proposed | Keeps v1 scope tight; examples are German. | Usually no |
| Web app as the primary interactive surface (server-side rendered, HTMX/Alpine, Svelte fallback) | Accepted; ADR written | Sole v1 client over the shared Go core; no standalone CLI in v1. | `0004` |

---

# Open Questions

Resolve these before treating the affected behavior as a stable repository contract. Materially architectural answers should be promoted to ADRs.

1. **Relationship to schwab-edition.** Both projects serve advanced German reading. Is schwab-edition a separate scholarly infrastructure (its own eXist-db/TEI app), or should mouseion eventually consume it as a corpus/annotation source? This affects scoping and should be decided early.
2. **Vocabulary identity and state.** What uniquely identifies an entry (language, canonical lemma, POS), and how are homographs, senses, and inflected forms handled? Which states (`candidate`, `accepted`, `generated`, `ignored`, `known`) are required? **Resolved by [ADR 0005](adr/0005-vocabulary-identity-normalization-ranking.md):** identity is `(language, canonical lemma, POS)`, sense-agnostic in v1 (senses handled via multiple example sentences at review); lifecycle states `candidate → accepted → generated` plus `ignored` and `known`, all reversible.
3. **Candidate ranking.** What is the initial ranking formula and threshold? How do frequency, priority-list membership, cross-text recurrence, POS, and proper-noun exclusions interact? **Resolved by [ADR 0005](adr/0005-vocabulary-identity-normalization-ranking.md):** global-first weighted blend (`0.6·global + 0.3·in-corpus + 0.1·priority + 0.05·(cross−1)`); selection = ≥2 in corpus OR priority OR top-5% global; content words, proper nouns excluded (tunable).
4. **Review workflow.** Terminal UI, exported review file, or web UI in v1? Which decisions must the learner be able to record? **Resolved toward the web UI by [ADR 0004](adr/0004-web-only-v1-client.md)** (the web app is the sole v1 client); remaining detail is the concrete review UX, tracked as an issue.
5. **Anki contract.** Exact Anki note/CSV format; which known-vocabulary inputs are supported first? **Resolved by [ADR 0006](adr/0006-anki-export-import-contracts.md):** UTF-8 TSV via Anki's CSV importer; custom Cloze-style note type with an identity-based dedup key as the first field; v1 known-vocab import is a per-language lemma list.
6. **Enrichment policy.** Which data is local/deterministic vs. external/LLM? How are external results cached, reviewed, and privacy-protected?
7. **Normalization profiles.** Which German locale/spelling-reform profiles first? How are rules versioned and applied to persisted data? **Resolved by [ADR 0005](adr/0005-vocabulary-identity-normalization-ranking.md):** one conservative, deterministic, versioned German standard-orthography (post-1996) profile; canonical lemma is a derived field, applied to new items, with explicit re-normalize migrations for persisted data.
8. **Source-text handling.** What source-location metadata must be retained for reproducible sentence selection? Policy on storing excerpts from copyrighted EPUBs?
9. **Go/Python contract details.** Exact Protobuf schema; HTTP vs. gRPC transport; is enrichment inline in Go or itself job-based?
10. **Auth (resolved).** Do we need user registration, or do per-language profiles suffice? **Resolved by [ADR 0002](adr/0002-multi-user-accounts.md):** multi-user accounts with per-user learning state and an admin role managing global resources. Remaining detail: the exact auth mechanism and credential storage in a self-hosted home-lab deployment (local accounts vs. an external identity provider).
11. **Frequency data source & format.** Which DWDS (or other) frequency data source(s) are supported first, and in what upload format? How are they versioned and refreshed?

---

# Related

- [README](../README.md)
- [Documentation governance](documentation-governance.md) — how the vault and repo divide documentation.
- [ADR 0001: Go core with shared libraries, Python as an ingest-time NLP producer](adr/0001-go-core-python-nlp-service.md)
- [ADR 0002: Multi-user accounts with per-user learning state and admin-managed global resources](adr/0002-multi-user-accounts.md)
- [ADR 0003: PostgreSQL as the initial persistence backend](adr/0003-postgresql-persistence.md)
- [ADR 0004: Web application as the sole v1 client (no standalone CLI)](adr/0004-web-only-v1-client.md)
- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](adr/0005-vocabulary-identity-normalization-ranking.md)
- [ADR 0006: Anki export and known-vocabulary import contracts](adr/0006-anki-export-import-contracts.md)
- Obsidian: Vocabulary Acquisition Tool spec and ADRs; Journal 2026-08-17 (reading-app braindump); session 2026-08-20 (accounts + DWDS frequency data).

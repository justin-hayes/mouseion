# Ancient Greek additive analysis: schema and module-interface design

Status: **Proposed design — not approved. No SQL, migration or application implementation is authorized.** · Date: 2026-10-15

Owning spec: [#2062 Design additive analysis for local annotation-backed Ancient Greek](https://github.com/justin-hayes/mouseion/issues/2062).

This document is filled in by the #2062 design tickets. Each ticket fills only
the sections it owns, as listed below. When a ticket finds that another
section contradicts its own, it records the conflict under
[§17 Open decisions](#17-open-decisions) and does not rewrite the other section.

| § | Heading | Owner ticket | Matrix ID prefix |
| --- | --- | --- | --- |
| 1 | Status and authority | [#2064](https://github.com/justin-hayes/mouseion/issues/2064) | — |
| 2 | Scope, controlling sources and non-goals | [#2064](https://github.com/justin-hayes/mouseion/issues/2064) | — |
| 3 | Current-state baseline | [#2064](https://github.com/justin-hayes/mouseion/issues/2064) | — |
| 4 | Bibliographic identity and offerings | [#2065](https://github.com/justin-hayes/mouseion/issues/2065) | `BIB` |
| 5 | Immutable evidence, bindings and interpretations | [#2066](https://github.com/justin-hayes/mouseion/issues/2066) | `EVD` |
| 6 | Vocabulary identity and normalization | [#2067](https://github.com/justin-hayes/mouseion/issues/2067) | `IDN` |
| 7 | Canonical analysis and exact occurrences | [#2068](https://github.com/justin-hayes/mouseion/issues/2068) | `OCC` |
| 8 | Learner-state attachment and compatibility | [#2069](https://github.com/justin-hayes/mouseion/issues/2069) | `LRN` |
| 9 | Retention references | [#2070](https://github.com/justin-hayes/mouseion/issues/2070) | `RET` |
| 10 | Module interfaces and validation | [#2071](https://github.com/justin-hayes/mouseion/issues/2071) | `VAL` |
| 11 | Durable preparation and atomic adoption | [#2072](https://github.com/justin-hayes/mouseion/issues/2072) | `ADP` |
| 12 | Operator-local entry point | [#2073](https://github.com/justin-hayes/mouseion/issues/2073) | `LOC` |
| 13 | Migration sequence, preflight and recovery | [#2074](https://github.com/justin-hayes/mouseion/issues/2074) | `MIG` |
| 14 | Public and local material | [#2075](https://github.com/justin-hayes/mouseion/issues/2075) | `PUB` |
| 15 | Acceptance-test matrix | all (append rows) | — |
| 16 | Requirement traceability | [#2075](https://github.com/justin-hayes/mouseion/issues/2075) | — |
| 17 | Open decisions | all (append rows) | — |
| 18 | Approval record | [#2076](https://github.com/justin-hayes/mouseion/issues/2076) | — |

## 1. Status and authority

Status: **Proposed design — not approved. No SQL, migration or application implementation is authorized.** · Date: 2026-10-15

The implementation specs that follow this design are
[#2061 canonical foundations](https://github.com/justin-hayes/mouseion/issues/2061),
[#2059 local import and adoption](https://github.com/justin-hayes/mouseion/issues/2059),
[#2060 Reading and vocabulary](https://github.com/justin-hayes/mouseion/issues/2060) and
[#2063 cards](https://github.com/justin-hayes/mouseion/issues/2063).
None of them may start until [§18 Approval record](#18-approval-record) records
explicit human approval of this design. A merged pull request that edits this
document is not approval.

These documents override this one. If this design conflicts with any of them,
they win, and the conflict is recorded under [§17](#17-open-decisions):

- [ADR 0093: Classical identity and stable reading units](../adr/0093-classical-identity-and-reading-units.md)
- [ADR 0094: Source-independent annotation and provenance](../adr/0094-source-independent-annotation-and-provenance.md)
- [ADR 0095: Greek personal counts and deferred external frequency](../adr/0095-greek-personal-counts-and-external-frequency.md)
- [ADR 0096: Classical catalogue discovery and intent-driven acquisition](../adr/0096-classical-catalogue-discovery-and-acquisition.md)
- [ADR 0097: Classical evidence refresh, adoption and retention](../adr/0097-classical-evidence-refresh-adoption-and-retention.md)
- [ADR 0098: Shared-core classical acquisition and automatic-evidence admission](../adr/0098-shared-core-classical-acquisition.md)
- [Annotation-backed Ancient Greek: staged implementation handoff](annotation-backed-ancient-greek.md), including its operational envelope
- [#2033 identity resolution](https://github.com/justin-hayes/mouseion/issues/2033#issuecomment-6163487386)
- [#2058 learner minimum](https://github.com/justin-hayes/mouseion/issues/2058#issuecomment-6191383128)
- [#2031 architectural acceptance](https://github.com/justin-hayes/mouseion/issues/2031#issuecomment-6189488434)

## 2. Scope, controlling sources and non-goals

### Scope

This design describes how Ancient Greek reading units, backed by local
annotations, become part of the existing Mouseion installation. They use the
same Book, Reading, snapshot and completion lifecycle as modern languages, and
the design adds only additive structures. It covers:

- Work, Edition, Reading unit and owner-scoped Book records, including how a
  catalogue entry offers a Reading unit. No public catalogue is required.
- Immutable acquired evidence, exact Text–annotation bindings, versioned
  Canonical interpretations, capability diagnostics and transitive retention
  references.
- Rich vocabulary identity: language, canonical lemma, UPOS and an optional
  scheme-qualified lexical discriminator. Source decoding stays separate from
  versioned Ancient Greek normalization.
- Exact-analysis occurrence identity. Printed, shared and discontinuous spans,
  unprinted nodes and container accounting are explicit. Offset equality never
  identifies an occurrence or transfers a decision.
- Additive membership and acceptance records that attach to the existing
  Reading, snapshot and completion anchors. There is no second Greek learner
  state machine and no wholesale backfill.
- The module chain: source interpretation adapter → shared canonical validation
  → preparation and adoption → learner core. Model-produced candidates pass
  through the same validation interface behind a compatibility adapter.
- Bounded, immutable package bytes stored in PostgreSQL with the established
  binary-content mechanism. Packages are data, never executable plugins.
- Atomic Evidence adoption against expected owner, Book, evidence and request
  state, including count projections and protected references.
- A structural migration sequence, a legacy-data preflight, transaction and
  concurrency rules, and a tested restore or roll-forward strategy.
- An operator-local entry point that selects the retained package and frozen
  interpretation without public hosting, automatic downloads or NLP.
- A statement of which material can be reviewed publicly and which stays local.

The design defines records, relationships, invariants, interfaces, transaction
rules, migration ordering and tests. It contains no executable SQL. Any DDL
appears only in fenced blocks headed `-- illustrative, not a migration`.

### Controlling sources

The documents listed in [§1](#1-status-and-authority) control this design. The
code facts the design builds on are recorded in [§3](#3-current-state-baseline).

### Non-goals

Copied verbatim from #2062 "Out of Scope":

> Structural SQL, application implementation, backfills, deployment, public catalog infrastructure, source repinning, new lexical repairs, corpus acquisition, analyzer execution, Greek NLP-service support, Latin, Books 2–7, external frequency and real provider/export operations.

## 3. Current-state baseline

Verified at commit `8341cef8121e47b9915e365059c040c4cfecc4aa` (`main`). Every
`path:line` below refers to that commit.

### Corrections to the #2064 starting points

The ticket's starting points were checked rather than copied. These were wrong
or incomplete:

- `reading_history` is **not** keyed by vocabulary identity. Its key is
  `(owner_id, language, book_id)`, and it carries only per-completion counts.
  Per-identity completion provenance lives on `known_vocabulary` instead.
- Reservations have no table of their own. Reserved vocabulary is the active
  reading's frozen `primary_goal_snapshot_vocabulary` rows, which are
  tuple-keyed.
- `analysisReadyForReading` is not an NLP-readiness check. It requires a
  published analysis run and corpus (an analysis-run-backed evidence
  classification). NLP capability readiness is checked only in catalogue sync.
- The occurrence-decision key collides in the database, but projection rejects
  ambiguous spans instead of silently applying a decision to both tokens. See
  rows B-07 and B-08.
- The Reading gate has more call sites than the three listed:
  `reading_chooser.go`, `reading_entry_links.go` and `reading.templ`.

### Baseline facts

| # | fact | evidence (path:line) | consequence for Greek | owning design section |
| --- | --- | --- | --- | --- |
| B-01 | `corpus_tokens` primary key is `(analysis_run_id, sentence_ordinal, token_ordinal)`. | `migrations/000001_initialize.up.sql:1648` | **Blocks.** A token is identified only by its position in an analysis sentence. A shared-span node, an unprinted node or a multiword container has no slot of its own. | §7 |
| B-02 | `surface`, `raw_lemma`, `canonical_lemma`, `upos`, `start_offset`, `end_offset`, `dependency` and `head` are all `NOT NULL`. `dependency <> ''`, `head >= 0` and `end_offset >= start_offset` are checked. | `migrations/000001_initialize.up.sql:387-404` | **Blocks.** Unprinted nodes (no offsets), discontinuous spans and absent optional syntax cannot be stored without inventing values. | §7 |
| B-03 | `validateCorpusToken` rejects an empty dependency, an out-of-range head and a root that is not self-headed. Every persisted token passes through it. | `internal/analysis/jobs.go:1851-1862`, called at `internal/analysis/jobs.go:1800` | **Blocks.** Optional syntax cannot be represented as unavailable. An annotation without a dependency tree is rejected on write. | §7, §10 |
| B-04 | `corpus_tokens` rows reference `corpus_sentences` by `(owner_id, corpus_id, analysis_run_id, sentence_ordinal)`. | `migrations/000001_initialize.up.sql:2725`, `migrations/000001_initialize.up.sql:360` | **Constrains.** Every token belongs to exactly one complete sentence. Incomplete sentence boundaries at a Reading-unit edge have no representation. | §7 |
| B-05 | Analysis runs are produced only by the analysis job calling the NLP analyzer. | `internal/analysis/jobs.go:1345` | **Blocks.** There is no path that creates an analysis from imported annotations with NLP stopped. | §10, §11 |
| B-06 | `occurrence_lemma_corrections` is unique on `(owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset)`. Exclusions are a flag on the same row. | `migrations/000023_occurrence_lemma_corrections.up.sql:17-18`, `migrations/000024_occurrence_vocabulary_exclusions.up.sql:1-7` | **Blocks.** Two occurrences that share a span collide: one Lemma correction or Occurrence exclusion would address both. | §7, §8 |
| B-07 | `occurrence_lemma_review_flags` has primary key `(owner_id, book_id, analysis_run_id, source_document_id, start_offset, end_offset)`. | `migrations/000025_occurrence_lemma_review_flags.up.sql:19-20` | **Blocks.** One Lemma review flag per span, not per occurrence. | §7, §8 |
| B-08 | `selection.OccurrenceIdentity` is `(SourceDocumentID, StartOffset, EndOffset)`. `requireUniqueOccurrences` rejects a decision whose span matches anything other than exactly one token. | `internal/selection/selection.go:47-51`, `internal/selection/selection.go:150-166` | **Blocks.** A decision on a shared span fails with `ErrInvalidDecision`, so a learner cannot decide either occurrence. This fails safely rather than leaking the decision. | §7 |
| B-09 | Vocabulary identity is `selection.Identity{Language, CanonicalLemma, UPOS}`. The domain mirror is `domain.SnapshotIdentity`. | `internal/selection/selection.go:27`, `internal/domain/current_reading_finish.go:5-7` | **Blocks.** There is no lexical discriminator, so homographs such as the public #2033 anchors PROIEL `εἰ#1`/`εἰ#2` merge. | §6 |
| B-10 | `selection.Effective` builds the identity from the trimmed lemma and upper-cased UPOS. `OccurrenceEligible` also rejects dependency `compound:prt`. | `internal/selection/selection.go:264-285` | **Constrains.** Eligibility reads syntax. When syntax is unavailable, the result depends on an empty dependency, which today cannot occur (B-03). | §6, §7 |
| B-11 | `known_vocabulary` is unique on `(owner_id, language, canonical_lemma, upos)`. | `migrations/000001_initialize.up.sql:1936` | **Blocks.** Known cannot distinguish discriminated identities. A legacy tuple row would match both descendants if read as a wildcard. | §8 |
| B-12 | `reading_history` primary key is `(owner_id, language, book_id)`. Per-completion counts and `goal_snapshot_id` were added later. | `migrations/000004_reading_history.up.sql:9`, `migrations/000009_primary_goal_completion_provenance.up.sql:6-22` | **Does not block identity.** Reading completion is per Book. Its counts must be computed over rich identities. | §8 |
| B-13 | `primary_goal_snapshot_vocabulary` (frozen snapshot, and the source of Reserved vocabulary) has primary key `(owner_id, snapshot_id, language, canonical_lemma, upos)`. | `migrations/000007_primary_goal_snapshots.up.sql:48` | **Blocks.** A Current-reading vocabulary snapshot cannot hold two discriminated identities that share a tuple. | §8 |
| B-14 | `generated_vocabulary` has primary key `(owner_id, language, canonical_lemma, upos)`. `deck_preparation_vocabulary` has primary key `(owner_id, deck_preparation_id, language, canonical_lemma, upos)`. | `migrations/000001_initialize.up.sql:1928`, `migrations/000001_initialize.up.sql:1816` | **Blocks.** Generated status and deck membership merge homographs. | §8 |
| B-15 | Deck specifications carry identity as a tuple: manifest items store `language`, `canonical_lemma`, `upos`. | `migrations/000001_initialize.up.sql:682-684` | **Blocks** for cards (#2063). Recorded here so §8 can reach it. | §8 |
| B-16 | `vocabulary_browse_counts` has primary key `(owner_id, book_id, analysis_run_id, canonical_lemma, upos)`, with a cross-Book index on `(owner_id, language, canonical_lemma, upos, book_id)`. | `migrations/000031_vocabulary_browse_counts.up.sql:11`, `migrations/000031_vocabulary_browse_counts.up.sql:17-18` | **Blocks.** Browse counts merge homographs. | §7, §8 |
| B-17 | `vocabulary_browse_selections` and `custom_vocabulary_deck_identities` are tuple-keyed and restrict `upos` to NOUN, VERB, ADJ and ADV. `doc/product.md` lists their UI as being retired. | `migrations/000026_custom_vocabulary_selections.up.sql:7-11`, `migrations/000026_custom_vocabulary_selections.up.sql:36-41` | **Constrains.** These are tuple-keyed stores outside the current Reading path. See OD-01. | §8 |
| B-18 | `canonicalization.For` returns the registry's language-neutral fallback for any unregistered non-empty language. Only `de` and `el` are registered. `Lookup` also returns the fallback for any language whose requested version is `1.2.0`. | `internal/canonicalization/canonicalization.go:76-88`, `internal/canonicalization/canonicalization.go:91-103`, `internal/canonicalization/canonicalization.go:105-126` | **Blocks.** `grc` silently normalizes with `unicode-casefold` 1.2.0 (`internal/canonicalization/unicode.go:11-12`, `:28-31`). The handoff requires rejecting the generic casefold fallback. See OD-02. | §6 |
| B-19 | Lemma review resolves the profile with `canonicalization.For(p.Language)`, so it inherits the fallback. | `internal/lemmareview/lemmareview.go:466-469` | **Blocks.** A Greek Lemma correction would be normalized by casefold, not `ancient-greek` v1. | §6, §8 |
| B-20 | Catalogue-entry aliases are unique on `(owner_id, connection_id, namespace, value)`. Each entry resolves to exactly one Book. Nothing in the schema stops a Book from holding several aliases. There is no Edition or Reading-unit record between entry and Book. | `migrations/000001_initialize.up.sql:222-231`, `migrations/000001_initialize.up.sql:2189`, `doc/adr/0044-catalogue-entry-connection-scoped-identity.md:25-34`, `sqlc/queries/book_aliases.sql:142-144` | **Blocks** entry-to-unit offerings. An entry identifies a Book, not an Edition and Reading unit. | §4 |
| B-21 | `source_materials.content` and `source_content_revisions.content` are `bytea NOT NULL`. Trigger `source_materials_content_immutable` (function `reject_source_material_content_replacement`) rejects changes to `content`, `full_text` or `content_hash`. Trigger `source_content_revisions_immutable` (function `reject_source_content_revision_mutation`) rejects every update. | `migrations/000001_initialize.up.sql:420`, `migrations/000001_initialize.up.sql:1124`, `migrations/000001_initialize.up.sql:129-140`, `migrations/000001_initialize.up.sql:116-121`, `migrations/000001_initialize.up.sql:2525`, `migrations/000001_initialize.up.sql:2504` | **Enables.** An established immutable-bytes mechanism exists to reuse. Revisions carry a checked `sha256:` digest (`migrations/000001_initialize.up.sql:1127-1128`). | §5, §9 |
| B-22 | Deck artifacts are `bytea`: `deck_preparations.artifact` and `custom_vocabulary_deck_preparations.artifact`. Covers also use `bytea`. | `migrations/000001_initialize.up.sql:896`, `migrations/000027_custom_deck_preparations.up.sql:11`, `migrations/000013_book_covers.up.sql:11` | **Enables.** Binary storage in PostgreSQL is the established practice. | §5 |
| B-23 | The current analysis per Book is `book_current_analyses`, primary key `(owner_id, book_id)`, referencing `(owner_id, source_material_id, book_id)`. | `migrations/000001_initialize.up.sql:239-245`, `migrations/000001_initialize.up.sql:1544`, `migrations/000001_initialize.up.sql:2629` | **Constrains.** One current analysis per (owner, Book), always tied to a source material. Evidence adoption must fit this anchor or extend it additively. | §11 |
| B-24 | `publishEligibleAnalysis` promotes a completed run, builds the Browse projection in the same transaction, and upserts `book_current_analyses` only when the run is still current, its corpus is complete, and `noNewerAnalysisRun` holds. | `internal/analysis/jobs.go:1711-1743`, `internal/analysis/jobs.go:168-176` | **Constrains.** This is the existing atomic-promotion precedent. Supersession is decided by display number or creation time per source material, with no expected-state check from the owner. | §11 |
| B-25 | `analysisReadyForReading` requires `AnalysisInEffect()` plus a non-empty `AnalysisRunID` and `CorpusID`. | `internal/webapp/analysis_evidence.go:58-60`, `internal/domain/book_evidence.go:121-123` | **Constrains.** Reading, deck preparation and Book detail open only for analysis-run-backed Books. No live NLP check, but no annotation-backed path either. | §7, §11 |
| B-26 | Call sites of `analysisReadyForReading`. | `internal/webapp/book_handlers.go:177`, `internal/webapp/prepareddeck_handlers.go:49`, `internal/webapp/prepareddeck_handlers.go:434`, `internal/webapp/reading_page_handlers.go:199`, `internal/webapp/reading_chooser.go:388`, `internal/webapp/reading_entry_links.go:48`, `internal/webapp/reading.templ:121`, `internal/webapp/reading.templ:125`, `internal/webapp/reading.templ:202` | **Constrains.** Every gate must accept a Vocabulary-ready analysis from annotations, or all of them must be changed together. | §11 |
| B-27 | `ReadySupportedLanguages` keeps only languages the NLP service reports as ready. Its only caller is catalogue sync, which also records them as supported languages. | `internal/analyzer/capabilities.go:50-58`, `internal/cataloguesync/sync.go:1026-1034` | **Blocks** catalogue discovery for `grc` while NLP does not report it, which is out of scope. The local entry point must not depend on it. | §12 |
| B-28 | `eligibleLanguages` drops every capability that is not ready or is English. | `internal/cataloguesync/sync.go:897-910`, called at `internal/cataloguesync/sync.go:498`, `:557`, `:1034` | **Blocks** OPDS sync of Greek feeds. Same consequence as B-27. | §4, §12 |
| B-29 | Store-contract interfaces: `Harness`, `BookStateStore`, `Store`, `LemmaReviewStore`, `Seeder`. `Run` runs every scenario against a fresh harness. | `internal/storecontract/harness.go:44-116`, `internal/storecontract/harness.go:124-143` | **Enables** seam (b). Scenario files: `admission.go`, `book_state.go`, `finish.go`, `lemma_review.go`, `lifecycle.go`. | §10, §15 |
| B-30 | Scenarios are fixed to `const Language = "de"`. Seeds take `domain.SnapshotIdentity` tuples, and `OccurrenceSeed` is only `{Surface, Lemma, UPOS}`, with no span, discriminator or syntax. | `internal/storecontract/harness.go:18`, `internal/storecontract/harness.go:21-40`, `internal/storecontract/harness.go:106-110` | **Blocks** Greek scenarios. The harness cannot seed discriminated identities, shared spans or absent syntax. See OD-04. | §10, §15 |
| B-31 | The fixture adapter runs the scenarios in-memory under plain `go test` (`fixtures.NewStore()`). The PostgreSQL adapter runs under the `integration` tag. It uses a cloned database per scenario, two real users, and a River client with the prepared-deck worker. | `internal/fixtures/storecontract_test.go:26-37`, `internal/prepareddeck/storecontract_integration_test.go:1`, `internal/prepareddeck/storecontract_integration_test.go:36-60` | **Enables** seam (b) on both adapters. | §15 |
| B-32 | `testutil.Postgres` builds a migrated template once per migration set and clones it per test with `CREATE DATABASE … TEMPLATE`. The file carries `//go:build integration`. | `internal/testutil/postgres_integration.go:1`, `internal/testutil/postgres_integration.go:37-75`, `internal/testutil/postgres_integration.go:113-126` | **Enables** seams (a) and (d) with per-test isolation. | §15 |
| B-33 | No Go, SQL, Python or proto source handles language code `grc`. `git grep -nIiw grc -- . ':!doc' ':!CONTEXT.md'` returns no matches. Unrestricted `git grep -nIw grc` matches only `doc/adr/0095-…`, `doc/adr/0098-…`, `doc/features/annotation-backed-ancient-greek.md` and `doc/research/modern-greek-nlp.md`. | `git grep -nIiw grc -- . ':!doc' ':!CONTEXT.md'` at `8341cef8` (no output) | **Confirms** there is no legacy `grc` code path. Persisted `grc` rows in an installation are still possible through the generic fallback (B-18), so the §13 preflight applies. | §13 |

## 4. Bibliographic identity and offerings

Verified at commit `b99c9c45ce64327a59495595a03bd941f9e9a1d2` (`main`). Every
`path:line` in this section refers to that commit.

### Summary

- **Work**, **Edition** and **Reading unit** are new records, separate from
  Book. They hold identity and description, never learner state.
- **Book** keeps its existing table and columns unchanged. A classical Book is
  linked to exactly one Reading unit through a separate, immutable
  **Book–Reading unit association**. A Book is classical exactly when that
  association exists. Existing EPUB Books have none and need no backfill.
- External identifiers (CTS URNs, repository identifiers) are **aliases**. A
  **verified identity alias** comes only from reviewed application code and
  resolves identity. A **claimed alias** is what a catalogue advertises. It is
  kept as provenance and never resolves identity.
- A connection-scoped Catalog entry offers Reading units through **Unit
  offering** records, one per declared unit. The existing one-entry-to-one-Book
  alias stays exactly as it is, for OPDS only.
- The operator-local path records a **Local registration** in place of an
  offering. It needs no connection and no Catalog entry.
- Shared and private records: see [OD-06](#17-open-decisions). This section is
  written against the recommended option (3). Records created from reviewed
  identity definitions are shared. Records derived only from one owner's
  unverified catalogue advertisement are private to that owner.

New record names are proposed terms (OD-11). Table names in the illustrative
DDL are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| `books` has `id`, `owner_id`, `title`, `metadata_provenance`, `language_state`, `language_tag` and timestamps; `author` was added later. It has no bibliographic identity columns. | `migrations/000001_initialize.up.sql:268-280`, `migrations/000011_book_author.up.sql:2` | The association and offerings are separate records. No column is added to `books`. |
| `books` is unique on `(owner_id, id)`. | `migrations/000001_initialize.up.sql:1560` | Owner-scoped records reference a Book through `(owner_id, book_id)`, as `book_aliases` and `book_dispositions` already do. |
| Owner-scoped Book children cascade when the Book is deleted. No query deletes a Book; deletion happens only through the owner cascade from `users`. | `migrations/000001_initialize.up.sql:2604-2605`, `migrations/000015_book_dispositions.up.sql:8`, `migrations/000001_initialize.up.sql:2645`; `git grep -niI "delete from books" -- sqlc internal` returns nothing | The association follows the same `ON DELETE CASCADE` from Book. See [Owner deletion](#owner-deletion) for private records. |
| Catalogue-entry aliases are unique on `(owner_id, connection_id, namespace, value)`. A check ties `catalog_entry` to a non-null connection. The connection reference cascades. | `migrations/000001_initialize.up.sql:2189`, `migrations/000001_initialize.up.sql:1519-1520`, `migrations/000001_initialize.up.sql:2596-2597` | Left unchanged. Classical offerings are a separate record (B-20). |
| `ReconcileCatalogueEntry` takes an advisory lock on owner, connection and entry. It then finds or creates the Book, activates membership, inserts an Inbox disposition when it creates the Book, and inserts the alias. | `internal/persistence/books.go:752-779`, `internal/persistence/books.go:788`, `internal/persistence/books.go:851-866`, `sqlc/queries/book_dispositions.sql:20-23` | Classical Book creation reuses the same lock, membership and Inbox steps. The OPDS method itself is not changed. |
| OPDS sync overwrites title, author and language. A language change ends an incompatible Current reading. | `internal/persistence/books.go:832-847` | Classical Books do not take this path (OD-10). |
| Book metadata provenance accepts only `catalog_sync`, `acquisition` and `source_materials_backfill`. The database has no check on the column. | `internal/domain/books.go:14-19`, `internal/domain/books.go:252-256` | The operator-local path needs a distinct provenance value (OD-08). |
| `opds_connections.owner_id` is nullable, and the table has no unique `(owner_id, id)`. | `migrations/000001_initialize.up.sql:1267-1277`, `migrations/000001_initialize.up.sql:1960` | An offering cannot use a composite owner-and-connection foreign key to this table today (OD-09). |
| A Book is read by `(owner_id, id)`. | `sqlc/queries/book_aliases.sql:16-19` | Another owner's Book ID is not found, whatever unit both Books share. |

### Records

#### Work

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Groups the Editions of one composition. Holds descriptive metadata only. | `id uuid`; `owner_id uuid NULL` (NULL = shared); `origin text` ∈ {`reviewed`, `advertised`}; `title text` (non-empty); `author text` (default `''`); `language text` (canonical Book-language tag, for example `grc`); `created_at`, `updated_at timestamptz` | `id` | `(owner_id, id)` unique, as a composite-reference target. No uniqueness on title, author or language: descriptions never identify a Work. | Shared when `owner_id` is NULL; otherwise private to that owner. `origin = reviewed` ⇔ `owner_id IS NULL`. | `id`, `owner_id`, `origin` immutable. Descriptive fields are mutable. A shared row is written only from a reviewed identity definition. A private row is written only by its owner's sync. | `owner_id` → `users(id)` `ON DELETE CASCADE` (private rows only) |

#### Edition

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One textual version of a Work, independent of the repository or producer that supplies it. | `id uuid`; `work_id uuid`; `owner_id uuid NULL`; `origin text`; `identification text` ∈ {`named`, `unidentified`}; `label text NULL` (descriptive label such as an editor and year; required and non-empty for `named`, NULL for `unidentified`); `evidence_pin uuid NULL` (the §5 pinned evidence record that backs an unidentified Edition; required for `unidentified`); `created_at`, `updated_at` | `id` | `(owner_id, id)` unique. No uniqueness on `label`, Work title or evidence: identical labels, titles or bytes never identify an Edition. | Same rule as Work. A shared Edition belongs to a shared Work. A private Edition belongs to a shared Work or to a Work private to the same owner. | `id`, `work_id`, `owner_id`, `origin`, `identification`, `evidence_pin` immutable. `label` is mutable for `named` rows only. An unidentified Edition never becomes named in place; identification is recorded as an [Edition reconciliation](#edition-reconciliation). | `work_id` → Work `NO ACTION`; `evidence_pin` → §5 pinned evidence `RESTRICT`; `owner_id` → `users(id)` `ON DELETE CASCADE` (private rows only) |

An unidentified Edition is displayed as "Unidentified edition" with its pinned
source shown. It never displays a guessed editor or a named Edition's label.

#### Reading unit

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The declared extent of one Edition that a Book represents. | `id uuid`; `edition_id uuid`; `owner_id uuid NULL` (equals the Edition's `owner_id`); `extent_kind text` ∈ {`whole_work`, `constituent_book`, `bounded_passage`}; `citation_scheme text NULL`; `bound_start text NULL`; `bound_end text NULL`; `extent_key text` (canonical serialization of kind, scheme and bounds); `created_at` | `id` | `(edition_id, extent_key)` unique; `(owner_id, id)` unique. | Inherits its Edition's scope. Units of a shared Edition are shared. | **Immutable.** Every update is rejected. | `edition_id` → Edition `NO ACTION`; `owner_id` → `users(id)` `ON DELETE CASCADE` (private rows only) |

Extent rules, as checks:

- `whole_work`: `citation_scheme`, `bound_start` and `bound_end` are NULL, and
  `extent_key = 'whole_work'`.
- `constituent_book`: `citation_scheme` and `bound_start` are non-empty and
  `bound_end` is NULL. For example, `example-work / ed-1 / book 1` is
  scheme `example-section`, start `1`, key
  `constituent_book:example-section:1`.
- `bounded_passage`: all three are non-empty. For example, Books 1–2 is
  `bounded_passage:example-section:1:2`.

Ordering and containment of bounds are scheme-specific. Reviewed adapter code
checks them before insert; the database does not.

A Reading unit has no field for annotation coverage, availability, source
revision, producer or evidence. Those live in §5 and never change the unit.

#### Edition reconciliation

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Records that a verified identification showed one Edition to be the same textual version as another. No ID changes. | `subject_edition_id uuid`; `identified_edition_id uuid`; `owner_id uuid NULL` (equals the subject's `owner_id`); `verified_by text` (reviewed definition identifier and version); `recorded_at` | `subject_edition_id` | At most one reconciliation per subject. Subject ≠ identified. The identified Edition is shared, or private to the same owner, and is never itself a subject (no chains). | Subject's scope. | Append-only; immutable once written. | both → Edition `NO ACTION`; `owner_id` → `users(id)` `ON DELETE CASCADE` (private rows only) |

#### Book (existing, unchanged)

The `books` table, its columns, constraints and every existing query stay as
they are. A classical Book is an ordinary row: `language_state = 'chosen'`,
`language_tag` from the Work, `title` and `author` copied from the Work at
creation (OD-10), `metadata_provenance` set by its origin (OD-08), active
membership, and an Inbox disposition when created.

#### Book–Reading unit association

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| States that an owner's Book stands for exactly one Reading unit. | `owner_id uuid`; `book_id uuid`; `reading_unit_id uuid`; `created_at` | `(owner_id, book_id)` | `(owner_id, reading_unit_id)` unique: at most one Book per owner and unit. `(owner_id, reading_unit_id, book_id)` unique, as the target for offerings and local registrations. | Owner. The referenced unit is shared or private to the same owner. | **Immutable.** Inserted only in the transaction that inserts the Book. | `(owner_id, book_id)` → `books(owner_id, id)` `ON DELETE CASCADE`; `reading_unit_id` → Reading unit `NO ACTION` |

#### Verified identity alias

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| An external identifier that reviewed code has verified to name one shared Work, Edition or Reading unit. | `scheme text` (for example `cts`, `example-repo`); `value text`; exactly one of `work_id`, `edition_id`, `reading_unit_id uuid`; `verified_by text`; `created_at` | `(scheme, value)` | One target per scheme and value. | Shared. Targets only shared records. | Immutable. Inserted only from reviewed identity definitions (compiled pin sets and reviewed adapter mappings), never from sync input. | target → Work, Edition or Reading unit `RESTRICT` |

#### Claimed alias

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| An identifier a Unit offering advertises, kept as provenance. Never used to resolve identity. | `owner_id uuid`; `offering_id uuid`; `scheme text`; `value text`; `claimed_kind text` ∈ {`work`, `edition`, `reading_unit`}; `first_seen_at`, `last_seen_at` | `(owner_id, offering_id, scheme, value)` | None across offerings. Many offerings may claim the same value. | Owner. | Rows are added by sync; only `last_seen_at` changes. | `(owner_id, offering_id)` → Unit offering `ON DELETE CASCADE` |

#### Identity conflict

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| A diagnostic that identity evidence disagrees. Recorded instead of merging. | `id uuid`; `owner_id uuid`; `book_id uuid`; `conflict_kind text` ∈ {`claimed_alias_names_other_edition`, `claimed_alias_shared_across_editions`, `duplicate_after_reconciliation`}; `conflict_subject text` (`scheme:value`, or the other Book's ID); `other_book_id uuid NULL`; `recorded_at` | `id` | `(owner_id, book_id, conflict_kind, conflict_subject)` unique, so a repeat sync records nothing new. | Owner. | Append-only. A resolution workflow is out of scope (ADR 0093 introduces no Book-merge workflow). | `(owner_id, book_id)` and `(owner_id, other_book_id)` → `books(owner_id, id)` `ON DELETE CASCADE` |

#### Unit offering

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One Reading unit that a connection-scoped Catalog entry offers, resolved to the owner's Book. | `id uuid`; `owner_id uuid`; `connection_id uuid`; `entry_namespace text`; `entry_value text`; `offering_key text` (adapter-declared stable unit designation within the entry); `declared_edition_key text` (adapter-declared within the entry); `reading_unit_id uuid`; `book_id uuid`; `advertised_title`, `advertised_author`, `advertised_bounds_label text`; `advertised_availability` (ADR 0096 metadata summary); `first_seen_at`, `last_seen_at`; `withdrawn_at NULL` | `id` | `(owner_id, connection_id, entry_namespace, entry_value, offering_key)` unique. Several offerings, from one entry or several, may name the same unit and Book. | Owner. | Identity fields (`owner_id` through `book_id`) immutable. Advertised fields, `last_seen_at` and `withdrawn_at` are mutable by that owner's sync only. | `(owner_id, reading_unit_id, book_id)` → association `NO ACTION`; `connection_id` → the owner's catalogue connection `ON DELETE CASCADE` (OD-09); `owner_id` → `users(id)` `ON DELETE CASCADE` |

`offering_key` and `declared_edition_key` come from the reviewed adapter's
reading of the entry's declared units. They are never derived from title,
annotation fragments or mutable coverage (ADR 0096).

#### Local registration

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Stands in for "offered by" when the operator-local path (§12) creates the Book from a reviewed definition, with no connection. | `owner_id uuid`; `definition_id text` (identifier of the reviewed pin set compiled into the application); `reading_unit_id uuid`; `book_id uuid`; `registered_at` | `(owner_id, definition_id, reading_unit_id)` | Key only. | Owner. | Immutable. | `(owner_id, reading_unit_id, book_id)` → association `NO ACTION`; `owner_id` → `users(id)` `ON DELETE CASCADE` |

The pinned versions used for a request belong to §11's request record, not here.
A new pin version for the same definition and unit reuses the same Local
registration and Book.

```sql
-- illustrative, not a migration
CREATE TABLE reading_units (
  id uuid PRIMARY KEY,
  edition_id uuid NOT NULL REFERENCES editions(id) ON DELETE NO ACTION,
  owner_id uuid NULL,
  extent_kind text NOT NULL CHECK (extent_kind IN ('whole_work','constituent_book','bounded_passage')),
  citation_scheme text NULL, bound_start text NULL, bound_end text NULL,
  extent_key text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (edition_id, extent_key),
  UNIQUE (owner_id, id)
);
-- plus: a trigger rejecting every UPDATE, and a check enforcing the extent rules.

CREATE TABLE book_reading_units (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  reading_unit_id uuid NOT NULL REFERENCES reading_units(id) ON DELETE NO ACTION,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id),
  UNIQUE (owner_id, reading_unit_id),
  UNIQUE (owner_id, reading_unit_id, book_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES books(owner_id, id) ON DELETE CASCADE
);
-- plus: a trigger rejecting every UPDATE, and a trigger rejecting an insert whose
-- unit's owner_id is neither NULL nor equal to the row's owner_id.
```

#### Owner deletion

Deleting a user is the only path that deletes Books
(`migrations/000001_initialize.up.sql:2645`). It also deletes that owner's
private Works, Editions and units. For that reason, references that can be
removed by the same owner cascade use `NO ACTION` rather than `RESTRICT`:

- unit → Edition
- Edition → Work
- association → unit
- offering or Local registration → association
- reconciliation → Edition

PostgreSQL checks `NO ACTION` at the end of the statement, so the result does
not depend on the order of cascaded deletes. A shared record is never removed
by owner deletion and still cannot be deleted while referenced. Owner deletion
leaves shared records and other owners' rows untouched (BIB-27).

### Resolution procedures

Each procedure runs in one short PostgreSQL transaction, with no network or file
I/O inside it. This follows ADR 0044's rule against holding a transaction
across feed I/O.

**Ensure Book for Reading unit** `(owner, unit, origin)`. This is shared by Unit
offerings and the operator-local path.

1. Reject the call unless the unit is shared or private to `owner`. The error is
   the same as for a missing unit.
2. Compute the unit's **reconciliation class** (proposed term, OD-11). Its
   **root** is the unit with the same `extent_key` in the identified Edition, if
   the unit's Edition is a reconciliation subject and that unit exists.
   Otherwise the root is the unit itself. The class is the root, plus every
   unit with the root's `extent_key` in an Edition reconciled to the root's
   Edition. Because reconciliations do not chain, the class is one level deep.
3. Take a transaction-scoped advisory lock on `owner + ":" + root`. This uses
   the same mechanism as the catalogue-identity lock at
   `internal/persistence/books.go:788`, with its own key class.
4. Look up the owner's associations for units in the class.
   - **None:** insert the Book, active membership, Inbox disposition and an
     association to the *requested* unit. Return the new Book.
   - **One:** return that Book. Its association still names the unit it was
     created for, which may be the other side of a reconciliation. The Book ID
     and association are never re-pointed. Disposition, membership and metadata
     are not changed.
   - **More than one** (the owner held Books on both sides before
     reconciliation): return the Book whose unit is exactly the requested unit.
     Record `duplicate_after_reconciliation` once per pair. Never merge.
5. The unique `(owner_id, reading_unit_id)` constraint is the backstop if the
   lock is bypassed.

So a Book stands for exactly one Reading unit, the one in its association.
After reconciliation, that unit is verified to be the same Edition and extent
as the identified unit. ADR 0093 preserves the Book ID in this case rather than
creating a Book "by changing Edition or unit".

**Resolve a Unit offering** (catalogue sync, one advertised unit at a time):

1. If an offering already exists for `(owner, connection, entry, offering_key)`,
   update its advertised fields and `last_seen_at`, clear `withdrawn_at`, and add
   any new claimed aliases. Its unit and Book never change. Stop.
2. Otherwise choose the Edition:
   1. If reviewed adapter code verifies that the entry names a shared Edition,
      use that Edition, creating it from its reviewed definition if absent.
   2. Otherwise, if an offering of the same `(owner, connection, entry)` with the
      same `declared_edition_key` exists, use its Edition.
   3. Otherwise create a private Work and Edition for `owner` with
      `origin = advertised`, described from the advertisement.
3. Find the unit by `(edition_id, extent_key)`, or create it in the Edition's
   scope.
4. Check every claimed alias.
   - If a verified alias with the same `(scheme, value)` names a different
     Edition (or a unit or Work of a different Edition), record
     `claimed_alias_names_other_edition`.
   - If another of this owner's offerings, from any connection, claims the same
     `(scheme, value)` and resolved to a different Edition, record
     `claimed_alias_shared_across_editions` on both Books.
5. Ensure Book for the unit, insert the offering, and insert its claimed aliases.

An offering missing from a complete sync gets `withdrawn_at`. Its Book,
association, disposition and history are untouched (ADR 0096 "Shared experience
and preservation"). Removing a connection deletes its offerings and claimed
aliases. The Books and associations remain.

**Local registration** (operator-local path, interface owned by §12):

1. Load the reviewed definition by `definition_id` from application code. If it
   is unknown, write nothing.
2. Ensure its shared Work, Edition, unit and verified aliases exist, with
   create-if-absent on their uniqueness keys.
3. Ensure Book for the unit with the local origin.
4. Insert the Local registration if absent. The owner needs no connection.

**Edition reconciliation** is recorded only from a reviewed identity
definition, never from sync heuristics. It inserts one reconciliation row and
nothing else. No Book, association, disposition, membership, history or §5
evidence row changes, and no ID is rewritten.

### Invariants

Each invariant is covered by the `BIB` rows in [§15](#15-acceptance-test-matrix).

1. **Work.**
   - A Work has a stable internal ID. No title, author, language or alias is part
     of its key. Two Works with identical descriptions coexist, and changing a
     Work's description leaves its ID and Editions unchanged (BIB-26).
   - A shared Work's description changes only through a reviewed identity
     definition. One owner's sync never changes a description another owner
     sees (BIB-25).
2. **Edition.**
   - Every Edition belongs to exactly one Work (BIB-11).
   - An unidentified Edition is never resolved to, merged with, or labelled as
     a named Edition because of its title, author, Work or evidence bytes
     (BIB-06, BIB-17).
   - After an Edition reconciliation, every Book ID, association, disposition and
     history row is unchanged. The next acquisition of the identified unit
     returns the existing Book (BIB-07).
   - If an owner held a Book on each side before reconciliation, both remain and
     a `duplicate_after_reconciliation` conflict is recorded (BIB-08).
   - A reconciliation never names its own subject. It is never recorded for an
     Edition that is already a subject, and never targets an Edition that is
     itself a subject (BIB-28).
3. **Reading unit.**
   - At most one Reading unit exists per (Edition, extent) (BIB-11).
   - A unit's kind, scheme and bounds never change after insert (BIB-09).
   - Annotation coverage and source revisions never change a unit's bounds or
     its Book (BIB-10).
4. **Book ↔ Reading unit.**
   - A classical Book references exactly one Reading unit, and that reference
     never changes (BIB-13).
   - At most one Book exists per (owner, Reading unit) (BIB-12).
   - Repeating acquisition of the same Edition and unit returns the same Book,
     including under concurrency (BIB-01, BIB-02).
   - A Book is classical if and only if the store reports a Reading unit for it.
     For every Book created by OPDS sync, EPUB acquisition or the source-material
     backfill, the store reports no Reading unit, and the existing behaviour
     applies. No column on `books` and no flag carries this (BIB-20).
   - A Book never gains an association after its creating transaction, so an
     existing EPUB Book never becomes classical (BIB-20).
   - Sync never changes a classical Book's title, author or language (OD-10,
     BIB-19).
5. **Aliases and verified identity.**
   - A verified alias is unique per scheme and value, and names exactly one
     shared record (BIB-16).
   - Sync input never creates a verified alias or a shared record's description.
     Only reviewed identity definitions do (BIB-25).
   - Verified aliases from two repositories resolve to one owner Book only when
     they name the same Edition and unit (BIB-15).
   - Claimed aliases, titles, authors, overlapping text and identical bytes never
     resolve or merge Books (BIB-14, BIB-17).
   - A claimed-alias disagreement is recorded as an Identity conflict and leaves
     both Books separate (BIB-14).
6. **Entry-to-unit offering.**
   - One Catalog entry offering N declared units yields N Unit offerings and N
     Books, each Inbox when created (BIB-18).
   - A repeat sync creates no Book and changes no disposition. A withdrawn
     offering keeps its Book (BIB-19).
   - OPDS reconciliation (`internal/persistence/books.go:752-866`) writes no
     association or offering. Classical sync writes no `catalog_entry` Book
     alias. The alias index at `migrations/000001_initialize.up.sql:2189` and
     every OPDS query are unchanged (BIB-20, BIB-21).
   - The operator-local path creates the Work, Edition, unit, Book and Local
     registration for an owner with no connection and no Catalog entry
     (BIB-22, BIB-23).
7. **Owner authorization.**
   - Sharing a Reading unit gives no access to another owner's Book, offering,
     Local registration or Identity conflict. Every Book-, offering- and
     conflict-bearing response for owner B contains only B's rows, whatever
     units B shares with A (BIB-03, BIB-29).
   - An operation on owner B's behalf never creates, changes or deletes owner A's
     Book, association, offering, Claimed alias, Identity conflict or Local
     registration (BIB-29).
   - A private Work, Edition or unit is never associated with, or resolved for,
     another owner (BIB-24, BIB-30).
   - **Implementation rules**, checked in review and exercised by the rows
     above:
     - Every query on the owner-scoped records in this section filters by the
       authenticated owner, and joins Books on `(owner_id, book_id)`.
     - Shared records are reached only by a join from an owner-scoped row, or by
       reviewed identity resolution, which returns record IDs and never Books.
     - No web route accepts a Work, Edition or unit ID as authority to reach a
       Book.
8. **Overlap and editions.**
   - Two Editions of one Work, and overlapping units such as Book 1 and Books
     1–2, give distinct Books (BIB-04, BIB-05).
   - Disposition, visibility, membership, Reading, snapshot, history, Known
     provenance and occurrence decisions stay keyed by Book, as today. Completing,
     triaging or hiding one Book never changes another Book of the same Work,
     Edition or overlapping unit (BIB-04, BIB-05).
   - Grouping by Work is a display query only.

### Conflicts and dependencies noted

- `CONTEXT.md:355-357` says Books enter "only through a connected catalog, never
  by manual entry". The operator-local path contradicts this wording (OD-08).
- Issue item 7 calls Work, Edition and Reading unit "shared descriptions". The
  recommended OD-06 option keeps every reviewed identity shared, but makes rows
  derived only from one owner's unverified advertisement private.
- `BIB` rows on seam (b) need the store-contract harness to seed classical
  Books, which is open as OD-04. Rows on seam (b) that involve offerings call
  "Resolve a Unit offering" directly through the shared store interface, with a
  synthetic adapter result. Only rows on seam (a) run a full catalogue sync.
- `advertised_availability` carries the availability and coverage summary that
  ADR 0096 "Browse, intent and acquisition" requires My Books to show. Its shape
  belongs to the Stage 3 discovery spec
  ([handoff §3](annotation-backed-ancient-greek.md#3-authorize-catalog-release-and-implement-durable-acquisitionadoption)).
- `evidence_pin` on an unidentified Edition references §5's pinned evidence
  record. §5 defines that record's key.

## 5. Immutable evidence, bindings and interpretations

Verified at commit `76e6f93d585fe9c8ca66198431e6f6a5562d3f3c` (`main`). Every
`path:line` in this section refers to that commit. No file outside `doc/` changed
between §3's baseline commit and this one, so §3's citations still hold.

### Summary

- Acquired package bytes are stored **once per SHA-256 digest** in a shared
  **Package blob**. The **Package content** record and its **Package members**
  hold the source-content identity: the digest of the reviewed member
  inventory. This identity is kept separate from the transport digest of one
  ZIP build. Member bytes are not stored a second time.
- **Package acquisition** is the owner-scoped provenance row, one per owner,
  origin and release. It is also the **owner evidence access** record. Shared
  physical rows have no owner and are read only through an acquisition.
- **Text Source revisions** and **Annotation Source revisions** name exact
  members, their upstream origin and their release metadata. Unknown metadata
  stays explicitly unknown. Revisions carry no Reading unit.
- A **Text–annotation binding** joins one text revision, one annotation revision
  and one shared Reading unit, whose §4 bounds are unchanged. It also records the
  projection, the correspondence check and the extracted-text digest. Only a
  binding relates evidence to a Reading unit.
- An **Interpretation configuration** pins the analysis-affecting named versions
  and the frozen exception table digest. The same set always resolves to the
  same row.
- An **Exact analysis** is identified by (binding, interpretation, adapter). It
  has exactly one **Capability diagnostic** for each of the five capabilities,
  plus sentence- and node-scoped **Diagnostic findings**. A confirmed defect is
  an appended **Usability restriction** and never an edit.
- **Provenance claims** record attributed review provenance by layer and scope,
  including the repair layer.
- The owner's current classical analysis is a **parallel record** beside
  `book_current_analyses`. An additive trigger keeps the two exclusive, so each
  (owner, Book) has at most one current published analysis. EPUB promotion is
  unchanged (OD-12).

Every record in this section is immutable except the owner's current-analysis
row, which only §11 adoption replaces. New record names are proposed terms
(OD-18). Table names in the illustrative DDL are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| Trigger `source_content_revisions_immutable` runs `reject_source_content_revision_mutation()`, which raises on every `UPDATE`. The baseline reuses the same function for `source_material_unit_snapshots` and `source_material_units`. | `migrations/000001_initialize.up.sql:116-121`, `:2504`, `:2511`, `:2518` | **Mirrored, not reused.** Each new immutable record gets its own `BEFORE UPDATE` trigger, which runs a new function with the same body (raise on every update). Its message names classical evidence. The existing function would report "source content revisions are immutable" for unrelated records, and its body stays untouched. |
| `source_content_revisions` checks `content_digest = 'sha256:' \|\| encode(digest(content,'sha256'),'hex')` unless `digest_version = 0`, using pgcrypto. | `migrations/000001_initialize.up.sql:1127-1128`, `:29` | The Package blob and Extracted text mirror this check, without the legacy `digest_version = 0` escape. |
| Source bytes are owner-scoped. Revisions are unique on `(owner_id, source_material_id, content_digest)`, so identical bytes held by two owners are stored twice. | `migrations/000001_initialize.up.sql:1118-1126`, `:2048` | Classical bytes use a shared blob keyed by digest. Ownership lives in the Package acquisition. |
| The migrations contain no `BEFORE DELETE` trigger. Deleting referenced rows is prevented only by foreign keys. | `grep -l "BEFORE DELETE" migrations/*.sql` returns no file at `76e6f93d` | Delete protection here is `RESTRICT` or `NO ACTION` foreign keys. Transitive retention references belong to §9. |
| `analysis_runs` is owner-scoped and references an owner's `source_content_revisions` row and corpus. Deleting the source material cascades to it. | `migrations/000001_initialize.up.sql:193-215`, `:2573`, `:2581`, `:2589` | A shared exact analysis cannot be an `analysis_runs` row without copying bytes per owner and inventing analyzer fields (OD-12). |
| `book_current_analyses` has primary key `(owner_id, book_id)`. `source_material_id` and `analysis_run_id` are `NOT NULL` and reference `source_materials` and `analysis_runs`. | `migrations/000001_initialize.up.sql:239-245`, `:1544`, `:2613`, `:2629` | Holding a classical analysis there would mean relaxing `NOT NULL` columns and foreign keys on a shipped table (OD-12). |
| The `current_analysis_identity` view joins `book_current_analyses` to `source_materials`, `analysis_runs` and a complete `corpora` row. Code and queries reference it in 61 places. | `migrations/000001_initialize.up.sql:433-444`; `grep -rn --include='*.sql' --include='*.go' current_analysis_identity sqlc/queries internal` (61 lines) | A parallel record leaves the view and every EPUB reader unchanged. |
| `publishEligibleAnalysis` upserts `book_current_analyses` on `(owner_id, book_id)`. | `internal/analysis/jobs.go:1711-1743`, upsert at `:1736-1739` | Unchanged. The new exclusion trigger never fires for an EPUB Book. |
| Test PostgreSQL is 16, and compose runs 18. | `internal/testutil/postgres_integration.go:90`, `compose.yaml:13` | `UNIQUE NULLS NOT DISTINCT` (PostgreSQL 15+) is available for explicitly unknown metadata. |

### Records

All digests are `text` of the form `sha256:<64 lowercase hex>`. "Shared" means
the row has no owner column and is reached only through the
[access path](#owner-evidence-access). Default `ON DELETE` is `RESTRICT`.

#### Package blob

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The exact bytes of one acquired package build (a ZIP), or of one frozen interpretation exception table. | `digest text` (checked equal to the SHA-256 of `content`); `blob_kind text` ∈ {`package`, `interpretation_table`}; `content bytea`; `byte_length bigint` (= `octet_length(content)`, ≤ 16,777,216); `package_content_digest text NULL` (required for `package`, NULL for `interpretation_table`); `created_at` | `digest` | Key only. Identical bytes are stored once, whatever their origin or owner. | Shared. | **Immutable.** Every `UPDATE` is rejected by the mirrored trigger. No delete path is designed (ADR 0097 retains unreferenced evidence by default). A delete while referenced fails on the foreign keys below. | `package_content_digest` → Package content `RESTRICT`. Referenced by Package acquisition and Interpretation configuration, `RESTRICT`. |

A blob's `digest` is the package's **transport identity**. It identifies one ZIP
build and nothing else.

#### Package content

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Source-content identity: the exact member inventory that one or more builds carry. | `digest text` (SHA-256 of the canonical inventory serialization `inventory/1`: one line per member, sorted by path, giving path, role, size and member digest); `inventory_format text` (`inventory/1`); `member_count integer` (1–128); `expanded_bytes bigint` (0–67,108,864); `created_at` | `digest` | Key only. | Shared. | **Immutable.** | None. A deferred constraint trigger checks at commit that `member_count` and `expanded_bytes` equal the count and sum of its Package member rows. |

Two ZIP builds that preserve every member but differ in compression are two
blobs with one Package content. Both resolve to the same revisions, bindings and
analyses (EVD-06).

#### Package member

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One pinned member of a Package content. | `package_content_digest text`; `path text` (normalized relative path: NFC, `/` separators, no leading `/`, no `.` or `..` segment, no empty segment); `role text` ∈ {`text`, `annotation`, `notice`, `manifest`}; `byte_length bigint` (≥ 0); `member_digest text` | `(package_content_digest, path)` | Key only: no duplicate path within a content. | Shared. | **Immutable.** | `package_content_digest` → Package content `RESTRICT` |

`role` comes from the reviewed pins, never from the package. Member bytes are
read from the blob through the safe reader (see [Limits](#limits-and-where-they-are-enforced)).
The reader re-verifies `member_digest` on every read. Reading pages use
Extracted text and analysis rows, not members.

#### Package acquisition (owner evidence access)

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One owner's verified acquisition of one package build from one origin and release. It is that owner's provenance and their only route to the shared evidence. | `id uuid`; `owner_id uuid`; `package_digest text` (a blob with `blob_kind = package`); `origin_kind text` ∈ {`operator_local`, `catalog_release`}; `origin_ref text` (the Local registration `definition_id`, or the catalogue connection ID; text, not a foreign key); `package_release text NULL` (NULL = declared unknown); `pin_set text` (identifier and version of the reviewed pin set it was verified against); `acquired_at timestamptz` | `id` | `(owner_id, id)` unique, as a composite-reference target. `(owner_id, package_digest, origin_kind, origin_ref, package_release)` unique `NULLS NOT DISTINCT`: repeating the same owner, origin and release is idempotent, while another origin, another release or another owner gives another row. | Owner. | **Immutable.** | `owner_id` → `users(id)` `ON DELETE CASCADE`; `package_digest` → Package blob `RESTRICT`. The link to the preparation request belongs to §11. |

`origin_ref` is deliberately not a foreign key. Removing a connection never
removes an acquisition, so provenance and access survive it (ADR 0097
"Retention follows every retained reference").

#### Evidence notice

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| A notice that must travel with the components it covers. It is either a package member or a record that Mouseion issues, such as a modification notice for an interpretation. | `id uuid`; `notice_kind text` ∈ {`member`, `record`}; `package_content_digest text NULL` and `member_path text NULL` (both required for `member`, and the member's role must be `notice`); `notice_text text NULL` and `notice_digest text NULL` (both required for `record`, digest checked); `issued_by text NULL` (reviewed definition identifier, required for `record`); `created_at` | `id` | `(package_content_digest, member_path)` unique for `member`; `notice_digest` unique for `record`. | Shared. | **Immutable.** | `(package_content_digest, member_path)` → Package member `RESTRICT` |

#### Notice coverage

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Links a notice to a component it covers, so that a later export can reproduce every applicable notice. | `notice_id uuid`; exactly one of `text_revision_id`, `annotation_revision_id`, `interpretation_id uuid`, or the pair `package_content_digest` + `member_path`; `declared_by text` (pin set or interpretation definition) | `(notice_id, subject)` | Key only. | Shared. | **Immutable.** | `notice_id` → Evidence notice `RESTRICT`; subject → its record `RESTRICT` |

Rights decisions are **not** modelled here. No record in this section has a
permission, licence-cleared or exportable flag. Operation-specific rights stay
with the external gates
[#2054](https://github.com/justin-hayes/mouseion/issues/2054),
[#2055](https://github.com/justin-hayes/mouseion/issues/2055) and
[#2056](https://github.com/justin-hayes/mouseion/issues/2056). A hash proves
bytes, not rights (handoff "Initial operational envelope").

#### Text Source revision

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One immutable revision of a Text Source: the exact text members from one upstream origin and release. | `id uuid`; `origin text` (reviewed upstream origin identifier, for example `example-text-repo`); `release_identifier text NULL`; `release_revision text NULL` (for example an upstream commit); `text_format text` (reviewed format identifier, for example `example-tei/1`); `package_content_digest text`; `member_set_digest text` (SHA-256 over the sorted (path, member digest) pairs of its members); `created_at` | `id` | `(origin, release_identifier, release_revision, package_content_digest, member_set_digest)` unique `NULLS NOT DISTINCT`. | Shared. | **Immutable.** | `package_content_digest` → Package content `RESTRICT`. §4's unidentified-Edition `evidence_pin` targets this record's `id`. |

#### Text Source revision member

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One member that a Text Source revision consists of. | `text_revision_id uuid`; `package_content_digest text` (equal to the revision's, checked); `member_path text` (role `text`, or `annotation` when annotation-carried text is the verified authority, ADR 0093) | `(text_revision_id, member_path)` | Key only. | Shared. | **Immutable.** Inserted only in the revision's transaction. | `text_revision_id` → Text Source revision `RESTRICT`; `(package_content_digest, member_path)` → Package member `RESTRICT` |

#### Annotation Source revision

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One immutable revision of an Annotation Source. | `id uuid`; `origin text`; `release_identifier text NULL`; `release_revision text NULL`; `annotation_scheme text` (reviewed scheme identifier, for example `example-scheme`); `scheme_version text NULL`; `producer text NULL`; `declared_coverage text NULL` (upstream-declared passage coverage in its own citation terms); `package_content_digest text`; `member_set_digest text`; `created_at` | `id` | `(origin, release_identifier, release_revision, annotation_scheme, scheme_version, package_content_digest, member_set_digest)` unique `NULLS NOT DISTINCT`. | Shared. | **Immutable.** | as Text Source revision |

#### Annotation Source revision member

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One member that an Annotation Source revision consists of. | `annotation_revision_id uuid`; `package_content_digest text` (equal to the revision's, checked); `member_path text` (role `annotation`) | `(annotation_revision_id, member_path)` | Key only. | Shared. | **Immutable.** Inserted only in the revision's transaction. | `annotation_revision_id` → Annotation Source revision `RESTRICT`; `(package_content_digest, member_path)` → Package member `RESTRICT` |

**Explicitly unknown metadata.** In both revision records, a NULL in
`release_identifier`, `release_revision`, `scheme_version`, `producer` or
`declared_coverage` means "the reviewed pin set declared this unknown". The pin
set must state every field, either as a value or as the explicit `unknown`
marker. A missing field stops import. An empty string is rejected by a check.
None of these fields has a "not applicable" meaning: every annotation has a
producer and a coverage, even when they are unknown.
Importers never infer a value from file names, the package manifest or the
current date (ADR 0094 "Provenance, versions…").

One revision can back several Reading units, because only bindings name units.

#### Extracted text

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The exact document text that a binding's spans index into, as half-open Unicode code-point offsets. | `digest text` (SHA-256 of the UTF-8 bytes of `content`, checked); `content text`; `code_point_length integer`; `created_at` | `digest` | Key only. | Shared. | **Immutable.** | None |

#### Text–annotation binding

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The verified correspondence between one Annotation Source revision and one exact Text Source revision over one Reading unit. | `id uuid`; `text_revision_id uuid`; `annotation_revision_id uuid`; `reading_unit_id uuid` (a shared §4 unit; OD-17); `source_bounds text` (the unit's extent in the sources' own citation terms, from the reviewed definition); `projection text` (reviewed transformation/projection identifier and version, for example `example-projection/1`; reviewed adapter code verifies before insert that `source_bounds` denotes exactly the unit's §4 extent, and the database does not, as in §4); `correspondence text` (reviewed correspondence-check identifier and version); `extracted_text_digest text`; `validation_status text` ∈ {`validated`, `rejected`}; `rejection_reasons text[]` (reason codes; non-empty exactly when `rejected`); `created_at` | `id` | `(text_revision_id, annotation_revision_id, reading_unit_id, source_bounds, projection, correspondence)` unique. | Shared. | **Immutable.** A changed binding is a new row with a different key. | `text_revision_id`, `annotation_revision_id` → revisions `RESTRICT`; `reading_unit_id` → §4 Reading unit `RESTRICT`; `extracted_text_digest` → Extracted text `RESTRICT` |

The binding inputs are deterministic. When an import computes a binding whose
key already exists, it reuses the existing row only if `extracted_text_digest`
and `validation_status` are equal. Otherwise the import stops with a
`binding_nondeterministic` failure and writes nothing (EVD-14). A `rejected`
binding is retained for inspection. No analysis may reference it (EVD-15).

#### Interpretation configuration

The issue's "Canonical interpretation configuration".

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The pinned set of analysis-affecting named versions that turns bound evidence into a Canonical interpretation. | `id uuid`; `source_decoding text`; `lemma_mapping text`; `pos_mapping text`; `repair_layer text NULL` (NULL = the definition declares no repair layer, not "unknown"); `reading_terminal text`; `normalization_profile text`; `exception_table_digest text NULL` (blob with `blob_kind = interpretation_table`; NULL only when the reviewed interpretation definition declares that it has no exception table; the retained candidate's definition requires one, #2058 item 3); `contract_version text` (canonical-contract version); `configuration_digest text` (SHA-256 of the canonical serialization `interpretation/1` of the six slots, `exception_table_digest` and `contract_version`; never `id` or `created_at`); `created_at` | `id` | `configuration_digest` unique, and the tuple of every slot plus `exception_table_digest` and `contract_version` unique `NULLS NOT DISTINCT`. The same set therefore always gives the same row. | Shared. | **Immutable.** | `exception_table_digest` → Package blob `RESTRICT` |

Each slot holds a public identifier of the form `name/version`, checked against
`^[a-z0-9][a-z0-9-]*/[1-9][0-9]*$`. For the retained candidate, the handoff names
`oga-greta-lemma/2` (lemma mapping), `oga-agdt-upos/3` (POS mapping, including its
reviewed rule), `oga-anabasis1-research-repairs/1` (repair layer),
`oga-reading-terminal/2` (reading-terminal policy) and `ancient-greek/1`
(normalization profile). The handoff names no separate source-decoding
identifier (OD-13). The selection policy `grc-earliest-capped/1` is pinned by
the same reviewed definition but is **not** a slot here, because a
selection-policy change must not create a new analysis (OD-14).

The **frozen exception-value/precondition table** (#2058 item 3) is referenced
only by `exception_table_digest`. Its bytes come from local material and are
verified against the digest pinned in the reviewed interpretation definition
before they are stored. Its values never appear in public documents, logs or
fixtures (§14).

**Unknown versions stop import.** Before any write, the shared validation
interface (§10) checks every slot identifier, `contract_version` and the table
digest against the registry compiled into the reviewed release. If any one is
unknown, the import fails with `interpretation_version_unknown` and persists no
configuration, analysis or diagnostic (EVD-16). The database pattern check is
only a syntax backstop and does not decide what is known.

#### Exact analysis

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One immutable analysis of one binding under one interpretation by one adapter. | `id uuid`; `binding_id uuid`; `interpretation_id uuid`; `adapter text` (reviewed adapter identifier and version, for example `example-adapter/1`); `vocabulary_ready boolean` (shared validation's verdict, §10); `created_at` | `id` | `(binding_id, interpretation_id, adapter)` unique. | Shared. | **Immutable.** It is written in one transaction with all its Capability diagnostics, Diagnostic findings and §7 occurrence rows, and never partly. | `binding_id` → binding `RESTRICT`, and a trigger rejects a binding whose status is not `validated`; `interpretation_id` → Interpretation configuration `RESTRICT` |

A new mapping version gives a new Interpretation configuration and therefore a
new Exact analysis. The old analysis, its diagnostics and every learner record
that cites it are unchanged (EVD-18). Two owners whose acquisitions resolve to
the same binding and interpretation share one Exact analysis row. Adoption,
decisions and counts remain per owner (§8, §11).

#### Capability diagnostic

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The release-scope state of one capability in one analysis. | `analysis_id uuid`; `capability text` ∈ {`vocabulary`, `sentence_target`, `morphology`, `basic_syntax`, `secondary_syntax`}; `state text` ∈ {`missing`, `unsupported`, `ambiguous`, `invalid`, `usable`}; `reason_codes text[]` (each matching `^[a-z][a-z0-9_]*$`; empty exactly when `usable`) | `(analysis_id, capability)` | Key only. A deferred constraint trigger requires all five capabilities for every analysis at commit. | Shared. | **Immutable.** | `analysis_id` → Exact analysis `RESTRICT` |

#### Diagnostic finding

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| A narrower-scope fact about one capability, such as an invalid tree in one sentence or an unresolved POS on one node. | `analysis_id uuid`; `capability text`; `ordinal integer`; `scope text` ∈ {`sentence`, `node`}; `sentence_ref`, `node_ref` (§7 keys; `node_ref` NULL for `sentence`); `state text` (the same five values); `reason_code text` | `(analysis_id, capability, ordinal)` | `(analysis_id, capability, scope, sentence_ref, node_ref, reason_code)` unique `NULLS NOT DISTINCT`. | Shared. | **Immutable.** | `(analysis_id, capability)` → Capability diagnostic `RESTRICT`; refs → §7 sentence and node `RESTRICT` |

For a given sentence or node, the effective state of a capability is the
narrowest finding that applies. If no finding applies, it is the release-scope
state. Readiness rules over these states belong to §10. Reason codes come from a
versioned registry in reviewed code. Examples are `mapping_unsupported`,
`tree_cycle`, `root_missing` and `native_class_unresolved`.

#### Usability restriction

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| An appended record of a later integrity failure or confirmed defect that restricts capabilities without changing immutable facts (ADR 0097). | `id uuid`; exactly one subject: `package_content_digest`, `text_revision_id`, `annotation_revision_id`, `binding_id`, `interpretation_id` or `analysis_id`; `capabilities text[]` (a non-empty subset of the five); `basis text` ∈ {`integrity_failure`, `confirmed_defect`}; `reason_code text`; `explanation text` (non-empty); `recorded_by text` (operator command or reviewed release identifier); `recorded_at` | `id` | None. Repeated restrictions are allowed and all apply. | Shared (OD-20). A defect is a fact about evidence, so it applies to every owner who uses it. | **Append-only**; each row is immutable. Lifting is OD-19. | subject → its record `RESTRICT` |

A capability is **restricted** for an analysis when any restriction names that
capability on the analysis or on any of its transitive inputs (binding,
revisions, package content, interpretation). Consumers and new adoption treat a
restricted capability as not usable, and the reason is shown. The Capability
diagnostic row itself is unchanged. Only operator commands and reviewed releases
write restrictions; no learner route does.

#### Provenance claim

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| An attributed claim about how one annotation layer was produced, over one scope. | `id uuid`; exactly one subject: `annotation_revision_id` or `interpretation_id` (for the repair layer); `layer text` ∈ {`segmentation`, `lemma`, `pos`, `morphology`, `basic_syntax`, `secondary_syntax`, `repair`}; `scope text` ∈ {`release`, `subset`, `sentence`, `token`}; `scope_ref text NULL` (NULL exactly for `release`); `provenance text` ∈ {`human`, `human_reviewed_automatic`, `automatic`, `unknown`}; `attributed_to text` (the upstream statement or reviewed definition that makes the claim) | `id` | `(subject, layer, scope, scope_ref)` unique `NULLS NOT DISTINCT`. | Shared. | **Immutable.** | subject → its record `RESTRICT` |

The reviewed definition declares a `release` claim for every layer the analysis
uses, as `unknown` when upstream says nothing. A narrower claim overrides a
broader one only for its own scope and layer. A syntax claim says nothing about
lemma or morphology (ADR 0094). Claims are upstream assertions that Mouseion
records, not Mouseion certification. Learner corrections are never claims.

#### Book current classical analysis

Shape only. Adoption and replacement belong to §11.

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The owner's currently published Exact analysis for a classical Book, with the acquisitions that give the Book access to it. | `owner_id uuid`; `book_id uuid`; `analysis_id uuid`; `text_acquisition_id uuid`; `annotation_acquisition_id uuid` (may equal the text acquisition); `adopted_at timestamptz` | `(owner_id, book_id)` | Key only: at most one per (owner, Book). | Owner. | **Mutable** only by §11 adoption, which replaces `analysis_id`, the acquisitions and `adopted_at` together. | `(owner_id, book_id)` → §4 Book–Reading unit association `ON DELETE CASCADE`; `analysis_id` → Exact analysis `RESTRICT`; `(owner_id, text_acquisition_id)` and `(owner_id, annotation_acquisition_id)` → Package acquisition `NO ACTION` (owner deletion cascades both rows) |

At insert and update, a trigger checks the following. The analysis's binding
must name the same Reading unit as the Book's association. The text
acquisition's blob must carry the text revision's Package content, and the
annotation acquisition's blob the annotation revision's. A second additive
trigger on `book_current_analyses` rejects a row whose Book has a §4
association. Together, these keep at most one current published analysis per
(owner, Book) across both anchors (EVD-27).

```sql
-- illustrative, not a migration
CREATE TABLE evidence_blobs (
  digest text PRIMARY KEY CHECK (digest = 'sha256:' || encode(digest(content, 'sha256'), 'hex')),
  blob_kind text NOT NULL CHECK (blob_kind IN ('package', 'interpretation_table')),
  content bytea NOT NULL,
  byte_length bigint NOT NULL CHECK (byte_length = octet_length(content) AND byte_length <= 16777216),
  package_content_digest text NULL REFERENCES evidence_package_contents(digest) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((blob_kind = 'package') = (package_content_digest IS NOT NULL))
);
-- plus: BEFORE UPDATE trigger mirroring reject_source_content_revision_mutation().

CREATE TABLE evidence_package_acquisitions (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  package_digest text NOT NULL REFERENCES evidence_blobs(digest) ON DELETE RESTRICT,
  origin_kind text NOT NULL CHECK (origin_kind IN ('operator_local', 'catalog_release')),
  origin_ref text NOT NULL CHECK (btrim(origin_ref) <> ''),
  package_release text NULL CHECK (package_release IS NULL OR btrim(package_release) <> ''),
  pin_set text NOT NULL,
  acquired_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, id),
  UNIQUE NULLS NOT DISTINCT (owner_id, package_digest, origin_kind, origin_ref, package_release)
);

CREATE TABLE interpretation_configurations (
  id uuid PRIMARY KEY,
  source_decoding text NOT NULL, lemma_mapping text NOT NULL, pos_mapping text NOT NULL,
  repair_layer text NULL, reading_terminal text NOT NULL, normalization_profile text NOT NULL,
  exception_table_digest text NULL REFERENCES evidence_blobs(digest) ON DELETE RESTRICT,
  contract_version text NOT NULL,
  configuration_digest text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (source_decoding, lemma_mapping, pos_mapping, repair_layer,
    reading_terminal, normalization_profile, exception_table_digest, contract_version)
);
-- plus: name/version pattern checks on every slot and the mirrored update trigger.

CREATE TABLE book_current_classical_analyses (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  analysis_id uuid NOT NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  text_acquisition_id uuid NOT NULL,
  annotation_acquisition_id uuid NOT NULL,
  adopted_at timestamptz NOT NULL,
  PRIMARY KEY (owner_id, book_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES book_reading_units(owner_id, book_id) ON DELETE CASCADE,
  FOREIGN KEY (owner_id, text_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id) ON DELETE NO ACTION,
  FOREIGN KEY (owner_id, annotation_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id) ON DELETE NO ACTION
);
-- plus: a BEFORE INSERT OR UPDATE trigger on book_current_analyses rejecting a Book
-- that has a book_reading_units row (exclusion across both anchors).
```

### Pins

A **pin set** is a reviewed definition with an identifier and a version. It
declares:

- the accepted transport digests;
- the Package content digest;
- every member's path, role, size and digest;
- notice coverage;
- each revision's origin and its release metadata, or explicit `unknown`;
- the revision members;
- the provenance claims;
- the Reading unit and `source_bounds`;
- the Interpretation configuration and the selection policy.

Pins are never read from the package. The package's own manifest member is an
ordinary pinned member: its bytes must match their pin, and its contents
are never used to decide what to accept (EVD-07). Pins live either in reviewed
application code or in reviewed operator-local configuration, and OD-15 decides
which one holds the private retained pin set. In both cases the import reads pins
only through one pin-set interface, and the identifier and version are recorded
on every Package acquisition.

A repacked build with a new transport digest needs a pin-set version that lists
it. If it preserves every member, it resolves to the existing Package content
and revisions.

### Limits and where they are enforced

The handoff's operational envelope sets these per-package bounds. The
application enforces the configured value, whose default is the handoff value. The
database enforces the handoff value as a hard ceiling, and raising it needs a
reviewed change (OD-16).

| bound | application enforcement (before any write) | database backstop |
| --- | --- | --- |
| ≤ 16 MiB (16,777,216 bytes) compressed per package | The acquisition stage (§11 durable stage, §12 local stage) reads at most limit + 1 bytes from the source and fails with `package_too_large` before hashing or storing. | `byte_length` check on Package blob, which also covers the interpretation table. |
| ≤ 128 members | The safe ZIP reader counts central-directory entries and stops at entry 129 with `package_too_many_members`. It cross-checks local headers against the central directory. | `member_count` check on Package content, and the deferred count-equals-rows trigger. |
| ≤ 64 MiB (67,108,864 bytes) expanded | The reader decompresses with a running total across all members that ignores declared sizes, and stops at the cap with `package_expanded_too_large`. Each member's actual size must equal its pinned size. | `expanded_bytes` check, and the deferred sum-equals-rows trigger. |
| Exact inventory | Every entry must be pinned, and every pin must be present. Directory entries are rejected, because members are regular files only. Unlisted, missing or duplicate paths, size or digest mismatches, symlinks and special entries, encrypted entries, compression methods other than stored and deflate, and unsafe paths (absolute, `..`, backslash, non-NFC) are rejected with `package_inventory_mismatch` or `package_entry_unsafe`. The computed Package content digest must equal the pinned digest. | Member path check, and `(package_content_digest, path)` key. |

Validation runs on the package bytes in a bounded worker and outside any
database transaction. Only a fully verified package is then written: in one
short transaction that inserts the Package content, members, blob, notices,
revisions, provenance claims and the owner's acquisition. A failure at any
check writes no evidence row, and the request records the reason (§11, §12).
These failures stop immediately and never consume transient retries (handoff
"Initial operational envelope").

### Bytes present versus unit offered

- Inventory verification covers every pinned member, including whole-work text
  and annotation bytes outside the bound unit.
- A Reading unit is offered only by a §4 Unit offering or Local registration
  that names it. No record infers an offering, binding or availability from
  member presence, member paths or the package manifest.
- A binding is created only for the unit and `source_bounds` named by the
  request's reviewed definition. The adapter extracts and validates only that
  extent. Only an Exact analysis of that binding produces occurrences and counts
  (§7, §11).
- So a package that carries Books 1–7 while its definition names Book 1 yields
  revisions that reference the whole-work members, one Book 1 binding and one
  Book 1 analysis. It yields no offering, Book, binding, analysis or count for
  Books 2–7 (EVD-12).

### Owner evidence access

Shared physical rows carry no owner and never grant access. **Owner O may read
evidence E** exactly when O holds a Package acquisition whose blob carries the
Package content that E derives from, transitively:

- a member or notice derives from its Package content;
- a revision derives from its Package content;
- a binding derives from both revisions' contents;
- an analysis, diagnostic, finding or restriction derives from its binding.

The rules:

- Every store method that returns evidence takes the authenticated owner and
  either a Book ID or an acquisition ID, and applies this predicate in the same
  query.
- **Through a Book:** (owner, Book) → Book current classical analysis → its
  acquisitions and analysis. §9's owner-scoped retention references, such as
  frozen snapshots and decisions on an older analysis, are the only other Book
  routes, and they must satisfy the same predicate when written and when read.
- **Through an acquisition:** used during preparation and inspection (§11, §12).
- No web route accepts a blob digest, revision, binding, interpretation or
  analysis ID as authority. An ID unknown to the owner returns the existing
  not-found response, exactly as an absent ID does.
- Owner B's identical upload gives B their own acquisition and, from then on,
  their own access. It never grants access to A's acquisitions, requests or
  current records, and it never changes them (EVD-25).
- Owner deletion cascades the owner's acquisitions and current records. Shared
  rows remain (EVD-29).

### Relationship to `analysis_runs` and `book_current_analyses`

Recommendation OD-12 (2): `analysis_runs`, `corpora`, `corpus_tokens`,
`book_current_analyses`, `current_analysis_identity` and `publishEligibleAnalysis`
are unchanged and keep serving EPUB Books. A classical Book's published analysis
is an Exact analysis named by the Book current classical analysis row. The only
change to a shipped table is the additive exclusion trigger on
`book_current_analyses`. It is new, it never fires for a Book without a §4
association, and §4 guarantees that an EPUB Book never gains one.

How the Reading gates (B-25, B-26) recognize the new anchor stays OD-05. The
storage choice here is compatible with both OD-05 options: the single domain
classification (ADR 0052, ADR 0097) is extended to read the parallel record,
whether or not the gate call sites change.

### Invariants

Each invariant is covered by the `EVD` rows in [§15](#15-acceptance-test-matrix).

1. **Bytes.**
   - Package bytes are never replaced. While any acquisition or interpretation
     references a blob, it cannot be deleted (EVD-01).
   - Identical bytes are stored once. Each distinct (owner, origin, release) gets
     its own acquisition row, and repeating one is idempotent (EVD-02).
   - Transport identity and source-content identity are separate. Two pinned
     builds of the same members share one Package content and resolve to the
     same revisions and analyses (EVD-06).
2. **Inventory and limits.**
   - A package is stored only if its transport digest, every member and its
     content digest match the reviewed pins exactly (EVD-03, EVD-07).
   - The 16 MiB, 128-member and 64 MiB bounds hold against dishonest metadata in
     the application (EVD-04, EVD-05) and as database ceilings (EVD-26).
3. **Notices.** Every component that the pin set declares a notice for has
   Notice coverage, and the coverage can be retrieved with the component
   (EVD-08).
4. **Revisions.**
   - Revisions are immutable (EVD-10).
   - Undeclared upstream metadata is stored and reported as unknown, never
     guessed (EVD-11).
   - Bytes present never create an offering, binding, analysis or count
     (EVD-12).
5. **Bindings.**
   - A binding is unique on its inputs and immutable. A changed binding is a new
     row (EVD-13).
   - Recomputing an existing binding with a different result stops the import
     (EVD-14).
   - No analysis references a rejected binding (EVD-15).
   - A binding's extracted text is immutable and matches its digest (EVD-09).
   - Bindings reference shared Reading units only (EVD-32).
6. **Interpretation.**
   - An unknown slot, contract or table version stops import, and nothing is
     written (EVD-16, EVD-33).
   - The same set resolves to the same Interpretation configuration (EVD-17).
   - A selection-policy change creates no new interpretation or analysis
     (EVD-31).
7. **Analysis.**
   - At most one Exact analysis exists per (binding, interpretation, adapter)
     (EVD-19).
   - A new mapping version gives a new analysis and leaves the old one and its
     learner references unchanged (EVD-18).
   - At most one current published analysis exists per (owner, Book) across both
     anchors (EVD-27).
   - EPUB promotion is unchanged (EVD-28).
8. **Diagnostics.**
   - Every analysis has exactly one state for each of the five capabilities, with
     reason codes when the state is not usable (EVD-20).
   - Diagnostics never change (EVD-21).
   - A confirmed defect is an appended restriction that leaves the analysis and
     diagnostics unchanged (EVD-22), and applies transitively only to the
     capabilities it names (EVD-23).
9. **Provenance.** Claims are kept per layer and scope, including the repair
   layer, and one layer's claim never implies another's (EVD-24).
10. **Access.**
    - Identical bytes never give one owner access to another owner's
      acquisition, Book evidence or requests (EVD-25).
    - Owner deletion leaves shared evidence and other owners intact (EVD-29).
11. **Recovery.** A database backup and restore reproduces every blob, inventory,
    notice, revision, binding, interpretation, analysis, diagnostic, restriction
    and claim exactly, and the restored digests still verify (EVD-30).

### Conflicts and dependencies noted

- Issue item 6 lists the selection policy among the interpretation's pinned
  versions. The handoff ("Accepted evidence and bounded scope") says a
  card-selection policy change applies to a newly prepared specification, not
  to an analysis rewrite. This section keeps the selection policy pinned but
  outside analysis identity (OD-14).
- §4's Edition `evidence_pin` is resolved here as a Text Source revision `id`.
- Finding `sentence_ref` and `node_ref` target §7's keys. §7 defines them.
- Retention references from snapshots, decisions, deck specifications and
  in-flight requests to these records belong to §9. §9 must cover acquisitions as
  well as analyses, so that a retained reference also keeps its access route.
- `vocabulary_ready` is computed by §10's validation. The adoption transaction,
  the request link on acquisitions and the replacement of the current record
  belong to §11.

## 6. Vocabulary identity and normalization

Verified at commit `553c2bec53f95b7bff8e3fa8c6043a33b9bad35f` (`main`). Every
`path:line` in this section refers to that commit. Toolchain facts were read with
Go 1.27.2 and Python 3.13.5.

### Summary

- A **rich vocabulary identity** is the five-part value
  `(language, canonical_lemma, upos, discriminator_scheme, discriminator_value)`.
  The two discriminator fields are both set or both null. It extends ADR 0005's
  `(language, canonical lemma, POS)` tuple. The discriminator is never encoded in
  the lemma spelling.
- Rich identities are **interned** in one shared, immutable table with a surrogate
  ID. New consumers reference that ID (recommendation OD-21).
- The **citation headword** is separate from the key. It is the decoded citation
  form, with final `ς` kept. It is stored per occurrence (§7) and as one **Analysis
  headword** per (Exact analysis, identity) (OD-22).
- **Source decoding** and **normalization** are separate, separately versioned
  steps. Decoding is an adapter responsibility (for example `example-decode/1`).
  It turns native lemma data into a decoded citation form and a discriminator.
  Normalization is the Mouseion-owned profile `ancient-greek/1`, pinned to
  Unicode 17.0.0. It turns a decoded citation form into a key and never infers a
  discriminator.
- `grc` resolves only to `ancient-greek/1`. Generic casefold for `grc` is an error
  everywhere (OD-02, option 1).
- Existing tuple rows are untouched. A legacy tuple equals a rich identity only
  when the rich discriminator is null and language, lemma and UPOS are equal byte
  for byte. It is never a wildcard. Legacy `grc` tuples match no rich identity at
  all (OD-24).
- Modern languages keep their profiles, keys and outcomes. Their rich identities
  have a null discriminator and are identical to their tuples.

New record names are proposed terms (OD-27). Table names in the illustrative DDL
are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| The identity tuple is `selection.Identity{Language, CanonicalLemma, UPOS}`, mirrored by `domain.SnapshotIdentity`. | `internal/selection/selection.go:27`, `internal/domain/current_reading_finish.go:5-7` | The rich identity is a new type. The tuple types and every tuple-keyed store keep their meaning (B-09, B-11, B-13 to B-16). |
| `Effective` takes the lemma from the learner decision when present, and the UPOS always from the analyzed fact. | `internal/selection/selection.go:264-274` | A Lemma correction today changes the lemma only. It never changes UPOS. This section keeps that. |
| A decision whose lemma equals the analyzer's canonical lemma resolves to `keep` and stores nothing. | `internal/domain/lemma_review.go:101-122` | A learner cannot clear a discriminator by re-entering the same lemma (OD-23). |
| Lemma review normalizes the corrected lemma with `canonicalization.For(p.Language)`, the active profile, and records its name and version on the decision. | `internal/lemmareview/lemmareview.go:466-474`, `internal/lemmareview/lemmareview.go:501` | For a classical analysis, the correction uses the profile named by the analysis's interpretation, looked up by exact version, not the active profile. |
| `For` falls back to `unicode-casefold` 1.2.0 for any unregistered language, and `Lookup` does so for version `1.2.0`. Only `de` and `el` are registered. | `internal/canonicalization/canonicalization.go:76-103`, `internal/canonicalization/canonicalization.go:105-126` | B-18. `grc` must be registered as requiring a dedicated profile, so neither function falls back for it. |
| The casefold profile collapses whitespace and applies full Unicode case folding. `modern-greek` 1 uses the same function. | `internal/canonicalization/unicode.go:28-31`, `internal/canonicalization/unicode.go:42-44` | Full folding expands iota subscript to adscript (`ᾳ` → `αι`). It is unsuitable for `grc` and stays unchanged for `el`. |
| Concordance falls back to the casefold profile when `For` fails. Its form key is NFC, `strings.ToLower` and `ς` → `σ`. | `internal/persistence/concordance.go:22-30` | For `grc`, the lemma key comes from `ancient-greek/1` and the fallback branch never applies. |
| Concordance form matching lowercases surfaces in SQL: `translate(lower(o.surface), 'ς', 'σ')`. | `sqlc/queries/concordance.sql:115` | PostgreSQL `lower()` depends on the database's collation provider and Unicode tables, which are not pinned. OD-26. |
| The dictionary resolves its lookup key through `canonicalization.Normalize`, and returns an empty key on error. | `internal/dictionary/index.go:365-370` | With `grc` registered as dedicated, it uses `ancient-greek/1` or fails closed. It never casefolds `grc`. |
| The NLP producer casefolds every language except `de`, and labels the result `unicode-casefold` 1.2.0. | `nlp/src/mouseion_nlp/producer.py:42-43`, `nlp/src/mouseion_nlp/producer.py:351-355` | NLP produces no `grc` keys (Greek NLP support is out of scope). Shared validation rejects a `grc` candidate labelled with any profile other than `ancient-greek/1`. |
| Known matching treats a Known row with `upos = ''` as matching every UPOS. | `sqlc/queries/core.sql:238-239`, `sqlc/queries/books.sql:180-185` | A legacy UPOS wildcard. For modern languages it stays as it is. It never matches a rich identity. |
| The module requires Go 1.27.2. That toolchain and `golang.org/x/text` v0.43.0 report Unicode 17.0.0. An older local Go 1.26 reports 15.0.0, and x/text then selects its 15.0.0 tables. Python 3.13 reports 15.1.0. | `go.mod:3`; go1.27.2 `$GOROOT/src/unicode/tables.go:6` (`const Version = "17.0.0"`); `golang.org/x/text@v0.43.0/unicode/norm/tables17.0.0.go:11` (built only under `go1.27`); `go.mod:20`; `python3 -c 'import unicodedata; print(unicodedata.unidata_version)'` prints `15.1.0`; `nlp/pyproject.toml:6` | The profile is pinned to 17.0.0, and the implementation checks the version at run time (IDN-11). Python is not a conforming implementation today. |
| No browser code normalizes or compares lemmas. The only first-party script is `my-books.js`, which handles request recovery. | `git grep -nI -e 'normalize(' -e 'toLowerCase' -- 'internal/webapp/*.js' 'internal/webapp/*.templ' ':!internal/webapp/static/vendor'` returns nothing | Browser search sends raw input. The server computes every key. |

### Records

#### Vocabulary identity

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One distinct rich vocabulary identity, interned so that consumers carry one non-null reference instead of five fields with nullable parts. | `id uuid`; `language text` (canonical base language tag from `NormalizeLanguage`, for example `grc`; checked `^[a-z]{2,3}$`); `canonical_lemma text` (non-empty; the profile output, never the headword); `upos text` (one of the 17 UD UPOS tags; never empty); `discriminator_scheme text NULL` (a registered scheme identifier, checked `^[a-z][a-z0-9-]*$`); `discriminator_value text NULL` (non-empty, no leading or trailing whitespace); `created_at` | `id` | `(language, canonical_lemma, upos, discriminator_scheme, discriminator_value)` unique `NULLS NOT DISTINCT`: one row per distinct identity. A check requires `(discriminator_scheme IS NULL) = (discriminator_value IS NULL)`. | Shared. A row carries no owner and grants no access. It is reached only through an owner-scoped reference or a shared Exact analysis row that the owner may read (§5 access rule). | **Immutable.** Insert-if-absent only. No update, no delete path. | Referenced `RESTRICT` by §7 occurrence rows, Analysis headwords, rich Lemma corrections and §8 membership records. |

Text comparison on these columns is byte equality under the database's
deterministic default collation. No query applies `lower()`, `upper()`,
`btrim()`, `normalize()` or `translate()` to them. The profile version is not a
column. It is provenance on the Interpretation configuration (§5) and on each
rich Lemma correction. An identity computed under two profile versions that
yields the same five values is the same row.

This section requires no modern-language row in this table. Whether §8 writes
modern-language rows is §8's decision. Any it writes have null discriminators.

```sql
-- illustrative, not a migration
CREATE TABLE vocabulary_identities (
  id uuid PRIMARY KEY,
  language text NOT NULL CHECK (language ~ '^[a-z]{2,3}$'),
  canonical_lemma text NOT NULL CHECK (canonical_lemma <> ''),
  upos text NOT NULL CHECK (upos IN ('ADJ','ADP','ADV','AUX','CCONJ','DET','INTJ','NOUN',
    'NUM','PART','PRON','PROPN','PUNCT','SCONJ','SYM','VERB','X')),
  discriminator_scheme text NULL CHECK (discriminator_scheme ~ '^[a-z][a-z0-9-]*$'),
  discriminator_value text NULL CHECK (discriminator_value <> '' AND discriminator_value = btrim(discriminator_value)),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((discriminator_scheme IS NULL) = (discriminator_value IS NULL)),
  UNIQUE NULLS NOT DISTINCT (language, canonical_lemma, upos, discriminator_scheme, discriminator_value)
);
-- plus: a BEFORE UPDATE trigger rejecting every update (mirrored, as in §5).
```

**Index and size consequences of interning (OD-21).** The table has one primary
key index and one five-column unique index. Each consumer stores one 16-byte
`uuid` and one single-column index entry per row. A composite value would put
the language, a Greek lemma of typically 10–30 UTF-8 bytes, the UPOS and two
nullable text fields (roughly 40–70 bytes with varlena headers) in every
occurrence, headword, correction, membership and snapshot row. Every key and
index of those consumers would grow to the same width. Each join would also
compare two nullable columns, where a plain `=` treats `NULL = NULL` as unknown
and silently drops the match. With interning, consumers join on one non-null
column, and the nullable comparison happens once, in the unique index. The
costs are one join to display a lemma, and an insert-if-absent step
(`ON CONFLICT DO NOTHING` followed by a select) when an analysis or correction
introduces an identity.

#### Discriminator scheme (compiled registry entry)

Not a table. Schemes are registered in reviewed application code, like
interpretation slot identifiers (§5).

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Names one documented lexical scheme in which discriminator values are interpreted. | `scheme` (for example `proiel`, `morpheus`, `example`); `description`; `value_pattern` (regular expression for valid values); `native_markers` (the native spellings that carry a discriminator inside a lemma, for example a trailing `#<digits>` in PROIEL, used to reject undecoded lemmas) | `scheme` | One entry per scheme. | Release-wide. | Changed only by a reviewed release. An entry is never removed or redefined while any identity uses it. | — |

A scheme names a lexical scheme, not a Book, corpus, package or download URL.
Two corpora that both write `#1` share a scheme only if a reviewed entry says
their lexicons are the same scheme. Values are compared byte for byte. The
profile never normalizes them.

#### Analysis headword

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The learner-facing headword for one identity in one Exact analysis. | `analysis_id uuid`; `identity_id uuid`; `headword text` (an NFC decoded citation form that occurs in this analysis, with final `ς` kept); `variant_count integer` (≥ 1; number of distinct NFC decoded citation forms in the analysis that normalize to this identity) | `(analysis_id, identity_id)` | Key only: one headword per identity per analysis. | Shared, as the Exact analysis. | **Immutable.** Written in the analysis's single transaction (§5 "Exact analysis"). | `analysis_id` → Exact analysis `RESTRICT`; `identity_id` → Vocabulary identity `RESTRICT` |

The per-occurrence decoded citation form is node evidence and belongs to §7.
The Analysis headword is derived from it, deterministically:

1. Group the analysis's occurrences of the identity by NFC decoded citation form.
2. Choose the form with the most occurrences. Break a tie by the earliest
   occurrence in reading order (§7 order).
3. Record the number of distinct forms as `variant_count`. A value above 1 is
   reported in the owner's evidence detail. It writes no Diagnostic finding,
   because it does not change any capability state. The identity is not split or
   changed.

**What is shown.**

- A view scoped to one Book shows the headword of the analysis it reads: the
  Book's current classical analysis, or for a frozen record, the analysis that
  record froze.
- When a Lemma correction gives an effective identity, that Book's views show
  the correction's `corrected_citation_form` for it. If several corrections in
  the Book give the same identity with different forms, the most recently
  confirmed one is shown. A correction never changes the Analysis headword.
- A record that freezes an identity for display (snapshot vocabulary, deck
  specification item, Known acceptance) freezes the headword it showed. §8 places
  the column. Later analyses never change a frozen headword.
- A view that aggregates one identity across Books (the Known list, Concordance
  lemma groups) shows, in order of preference:
  1. the headword frozen with the owner's most recent Known acceptance of that
     identity;
  2. the Analysis headword of the owner's current analysis for the Concordance
     priority Book;
  3. the smallest headword, by code-point order, among the owner's current
     classical analyses that contain the identity.
- The discriminator is shown as separate metadata, the scheme's description and
  the value, next to the headword. It is never appended to the headword text.
- The normalized key is shown only for a legacy row, which has no headword. That
  is today's behaviour.

When two analyses disagree, each Book-scoped view keeps showing its own
analysis's headword. A disagreement never merges, splits or renames an identity.

#### Rich Lemma correction (fields this section requires)

The occurrence key and record placement belong to §7 and §8. This table states
only the identity fields that a correction on a classical Exact analysis carries.

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| A learner-confirmed alternative identity for one exact occurrence. | §7 occurrence key fields; `owner_id`; `book_id`; `corrected_citation_form text` (the learner's input, trimmed and NFC, with final `ς` kept; it is the headword for this occurrence); `identity_id uuid` (the resulting rich identity); `normalization_profile text` (the profile `name/version` used, equal to the analysis's interpretation slot); `confirmed_at` | §7 | At most one per (owner, Book, exact occurrence), as for today's corrections. | Owner. | Mutable only by that owner's confirmed lemma review, as today. Never changed by a new analysis or profile. | `identity_id` → Vocabulary identity `RESTRICT`; occurrence → §7 `RESTRICT` |

### Normalization profile `ancient-greek/1`

**Definition.** For an admitted decoded citation form `s`:

1. `s₁ = NFC(s)`.
2. `s₂` = `s₁` with each code point replaced by its **simple** lowercase mapping
   (`UnicodeData.txt` field 13). No `SpecialCasing.txt` mapping, no `Final_Sigma`
   context and no locale tailoring. This is "Unicode lowercase", not full case
   folding. It equals Go's `strings.ToLower`.
3. `s₃` = `s₂` with every U+03C2 `ς` replaced by U+03C3 `σ`.
4. `key = NFC(s₃)`.

**Unicode pin.** The profile uses Unicode **17.0.0** data for NFC and lowercase
mappings. That is the version that Go 1.27's `unicode` package and
`golang.org/x/text` v0.43.0's `unicode/norm` report (see Current anchors).

**Admitted input.** The profile is defined only for strings that pass the shared
validation interface's admission check (§10). Every caller computes a `grc` key
through that entry point. A string is admitted when all of these hold:

- it is valid UTF-8 and non-empty;
- it has no leading or trailing `White_Space` and no internal `White_Space`
  (an Ancient Greek citation lemma in an admitted scheme is one word; a
  multi-word native lemma is a decoding rejection);
- it has no code point whose General_Category is `Cc`, `Cs`, `Co` or `Cn` **as of
  Unicode 17.0.0**, so a later toolchain cannot silently widen the repertoire;
- it does not match any registered scheme's `native_markers`, so an undecoded
  discriminator never becomes lemma spelling.

A string that fails admission is rejected with `citation_form_invalid` or
`discriminator_in_lemma`. Nothing is repaired.

**What the profile preserves.** Each item is a fixture case (IDN rows):

- Iota subscript is preserved. `ᾼ` lowercases to `ᾳ`, not `αι`. A subscript form
  and its adscript spelling give different keys.
- Accents, breathings and diaeresis stay significant. Acute and grave give
  different keys. Only canonical equivalence unifies, for example oxia U+1F73 and
  tonos U+03AD, which NFC maps to one code point.
- No NFKC or NFKD. Lunate sigma U+03F2 `ϲ` stays `ϲ`.
- No elision, apostrophe, koronis or crasis handling. Combining U+0313 and
  U+2019 stay distinct. Only a documented producer decoding may change them.
- Malformed stacked marks are kept. NFC composes what canonical composition
  composes, and the remaining marks stay in the key. Nothing else is repaired.
- No whitespace collapsing and no trimming. Admission rejects whitespace instead.

The profile is pure and idempotent: `key(key(s)) = key(s)` for every admitted `s`.

**Registration.** `grc` is registered as a **dedicated-profile language**. For it:

- `For("grc")` returns `ancient-greek/1` while that is the active `grc` profile.
  If no `grc` profile were registered, it would return `ErrUnsupportedLanguage`,
  never the casefold fallback.
- `Lookup("grc", v)` returns a registered `grc` profile with version `v`, or
  `ErrProfileNotFound`. `Lookup("grc", "1.2.0")` is `ErrProfileNotFound`.
- Locale forms such as `grc-x-example` resolve through the base tag `grc`, never
  through `el`. `el` never resolves to `ancient-greek/1`.
- Every caller that has a fallback branch (`internal/persistence/concordance.go:24-27`)
  skips the fallback for a dedicated-profile language and reports the language as
  unsupported instead.
- Unregistered languages other than `grc` keep today's casefold fallback, so
  their behaviour does not change.

**Version as provenance.** `ancient-greek/1` is recorded in the Interpretation
configuration's `normalization_profile` slot (§5) and on each rich Lemma
correction. It is not part of the identity. A later `ancient-greek/2` that gives
the same five values for an input resolves to the same Vocabulary identity row,
so Known and Reserved still match. A new version may not make a key that already
denotes one lexical entry denote another. Its reviewed fixture must contain every
`ancient-greek/1` case and state, for each, whether the key changes. Any changed
key that existing learner rows use needs a separately approved plan (§13).

### Source decoding

Source decoding is an adapter responsibility. It is named in the Interpretation
configuration's `source_decoding` slot as its own `name/version`, separate from
lemma mapping and from normalization (OD-13).

- **Input:** one native lemma record as retained evidence: the native lemma string
  and any native lexical-identifier attribute, for example a PROIEL `#1` suffix or
  a UD `LId=1` feature.
- **Output:** either a **decoded citation form** with an optional
  `(discriminator_scheme, discriminator_value)`, or a rejection reason. The
  decoded citation form is the citation spelling, with final `ς`, case and marks
  as the decoding documents.
- Decoding may change elision marks, apostrophes or koronis only where its
  reviewed documentation and its fixture name the exact native convention.
- Decoding never guesses an elision expansion, resolves crasis, inserts an iota or
  repairs marks.
- The discriminator comes only from native data that the decoding's reviewed
  mapping names. Its scheme comes from the reviewed definition, never from a
  Book, package or URL. A native identifier with no reviewed mapping is
  `discriminator_unmapped`, a vocabulary finding for that node, never dropped.
- Native evidence is never edited. The decoded form and the discriminator are
  derived values stored with the node (§7).
- Normalization receives only the decoded citation form. It cannot see native
  attributes, so it can never infer, add or drop a discriminator.

Example (synthetic): under `example-decode/1`, native lemma `λέξις#ex:1`, and
native lemma `λέξις` with native attribute `lexid=ex:1`, both decode to citation
form `λέξις` with scheme `example`, value `ex:1`. Their key is `λέξισ`. Public
anchor: PROIEL `εἰ#1` and `εἰ#2` decode to citation form `εἰ` with scheme
`proiel` and values `1` and `2`. They are two identities that share lemma and
UPOS (ADV).

### Implementations and fixtures

Every place that computes or compares a `grc` key:

| # | place | computes or compares | current code | requirement | fixture format |
| --- | --- | --- | --- | --- | --- |
| I-1 | Go `canonicalization`: the `ancient-greek/1` profile and the admission check | computes | none; `internal/canonicalization/unicode.go:28-31` is the casefold profile that `grc` uses today | Implements the definition above. A test asserts that `unicode.Version` and `norm.Version` equal `17.0.0`. | `ancient-greek-normalization/1` |
| I-2 | Go `canonicalization` registry (`For`, `Lookup`) | resolves the profile | `internal/canonicalization/canonicalization.go:76-103` | Dedicated-profile rule above. | `ancient-greek-normalization/1` rows with `"call"` set (profile resolution) |
| I-3 | Go source-decoding adapters | produce the decoded citation form and discriminator (not the key) | none | One fixture file per decoding version. | `source-decoding/1` |
| I-4 | Go shared canonical validation (§10) | recomputes the key from the decoded citation form and compares | none | Rejects a candidate whose supplied key differs (`canonical_key_mismatch`), whose `grc` profile is not `ancient-greek/1` (`normalization_profile_rejected`), or whose citation form is not admitted. | `ancient-greek-normalization/1` and `vocabulary-identity-comparison/1` |
| I-5 | Go lemma review | computes the corrected key | `internal/lemmareview/lemmareview.go:466-474` | For a classical analysis, uses `Lookup` with the analysis's `normalization_profile` slot, through the admission check. For an analysis whose `grc` profile is not registered (a legacy casefold `grc` analysis), lemma review is unavailable; it never renormalizes. | `ancient-greek-normalization/1` |
| I-6 | Go Concordance term normalization | computes the lemma lookup key; compares stored keys | `internal/persistence/concordance.go:22-30` | Lemma key through `ancient-greek/1`; no fallback for `grc`. Form search for `grc` per OD-26. | `ancient-greek-normalization/1` |
| I-7 | Go dictionary lookup key | computes | `internal/dictionary/index.go:365-370` | Resolves through the registry, so `grc` uses `ancient-greek/1` or fails closed with an empty key. No Greek dictionary is in scope. | `ancient-greek-normalization/1` |
| I-8 | Go identity comparison (rich to rich, and legacy to rich) | compares | `internal/selection/selection.go:27` compares tuples | One shared function implements the comparison rules below. Every Go caller uses it. | `vocabulary-identity-comparison/1` |
| I-9 | PostgreSQL | compares only, byte equality | tuple equality in `sqlc/queries/core.sql:238-242`, `sqlc/queries/books.sql:180-185` | Never computes a `grc` key. Rich joins use `identity_id`. Legacy-to-rich joins require `discriminator_scheme IS NULL`, equal language, lemma and UPOS, and a non-`grc` language (OD-24). A legacy `upos = ''` row never joins to a rich identity. | `vocabulary-identity-comparison/1`, loaded by a seam (d) test |
| I-10 | Python NLP producer | none for `grc` | `nlp/src/mouseion_nlp/producer.py:351-355` | Emits no `grc` key. Python 3.13 has Unicode 15.1.0 tables and full-mapping `str.lower()`, so it cannot conform without a reviewed pin change (OD-25). A `grc` candidate it produced would be rejected by I-4. | none (I-4's rejection row covers it) |
| I-11 | Browser | none | no first-party normalization code (see Current anchors) | Sends raw input. The server applies I-6. No client-side key, filter or sort decides identity. | none (a seam (a) row covers search input) |

**Fixture formats.** All three are UTF-8 JSON Lines. They give every string as an
array of `"U+XXXX"` code points, so editors, Git filters and terminals cannot
normalize them. Public fixtures use synthetic or #2033-public examples only (§14).

`ancient-greek-normalization/1`:

```json
{"format":"ancient-greek-normalization/1","case":"nfd-acute","profile":"ancient-greek/1","unicode":"17.0.0",
 "input":["U+03BB","U+03B5","U+0301","U+03BE","U+03B9","U+03C2"],
 "expect":{"key":["U+03BB","U+03AD","U+03BE","U+03B9","U+03C3"]},
 "equal_to_case":"nfc-acute","note":"NFD and NFC give one key"}
```

`expect` is either `{"key": [...]}` or `{"rejected": "<reason code>"}`. An
optional `equal_to_case` or `distinct_from_case` asserts the relation to another
case's key. Profile-resolution rows carry `"call": {"fn": "For" | "Lookup",
"language": "...", "version": "..."}` and expect `{"profile": "name/version"}` or
`{"rejected": "..."}`.

`vocabulary-identity-comparison/1`:

```json
{"format":"vocabulary-identity-comparison/1","case":"legacy-null-vs-ex1",
 "left":{"kind":"legacy","language":"xx","canonical_lemma":["U+03BB","U+03AD","U+03BE","U+03B9","U+03C3"],"upos":"NOUN"},
 "right":{"kind":"rich","language":"xx","canonical_lemma":["U+03BB","U+03AD","U+03BE","U+03B9","U+03C3"],"upos":"NOUN",
          "discriminator_scheme":"example","discriminator_value":"ex:1"},
 "equal":false}
```

`source-decoding/1`:

```json
{"format":"source-decoding/1","case":"marker-suffix","decoding":"example-decode/1",
 "native":{"lemma":["U+03BB","U+03AD","U+03BE","U+03B9","U+03C2","U+0023","U+0065","U+0078","U+003A","U+0031"],"attributes":{}},
 "expect":{"citation_form":["U+03BB","U+03AD","U+03BE","U+03B9","U+03C2"],"discriminator_scheme":"example","discriminator_value":"ex:1"}}
```

`expect` is either the decoded triple or `{"rejected": "<reason code>"}`.

Proposed locations: `internal/canonicalization/testdata/ancient-greek-normalization-v1.jsonl`,
`internal/canonicalization/testdata/vocabulary-identity-comparison.jsonl`, and
one `source-decoding` file beside each decoding adapter. This ticket creates none
of them.

**Required synthetic cases** (each one is an IDN row):

| case | input | expected key | relation |
| --- | --- | --- | --- |
| NFC versus NFD | `λέξις` (U+03AD) and `λε` + U+0301 + `ξις` | `λέξισ` | equal |
| oxia versus tonos | `λ` + U+1F73 + `ξις` and `λέξις` | `λέξισ` | equal (canonical equivalence) |
| case | `Λέξις`, `ΛΈΞΙΣ` | `λέξισ` | equal to `λέξις` |
| final sigma | `λέξις`, `λέξισ` | `λέξισ` | equal |
| iota subscript versus adscript | `λῄξις` (U+1FC4), `λήιξις` | `λῄξισ`, `λήιξισ` | distinct |
| capital with prosgegrammeni | `ᾼ` (U+1FBC) | `ᾳ` (U+1FB3) | not `αι` |
| acute versus grave | `λέξις`, `λὲξις` (U+1F72) | `λέξισ`, `λὲξισ` | distinct |
| elision marks | `λέξ` + U+0313, `λέξ` + U+2019 | each unchanged | distinct; neither stripped |
| malformed stacked marks | `λε` + U+0301 + U+0301 + `ξις`; U+0313 + U+0301 + `Λέξις` | `λέ` + U+0301 + `ξισ`; U+0313 + U+0301 + `λέξισ` | marks kept, not repaired |
| no compatibility mapping | `λέξι` + U+03F2 | `λέξι` + U+03F2 | distinct from `λέξισ` |
| simple, not full, lowercase | U+0130 | U+0069 | not U+0069 U+0307 |
| discriminators | `λέξις` NOUN `example:ex:1` and `example:ex:2` | `λέξισ` both | distinct identities |
| undecoded marker | `λέξις#ex:1` given to admission | — | rejected `discriminator_in_lemma` |
| whitespace | ` λέξις`, `λέξις λέξις` | — | rejected `citation_form_invalid` |

### Identity comparison

Rich to rich: two rich identities are equal exactly when all five fields are
byte-equal, with a null discriminator equal only to a null discriminator.

Legacy to rich: a legacy tuple `(language, canonical_lemma, upos)` equals a rich
identity exactly when all of these hold:

1. the rich identity's discriminator is null;
2. the languages are byte-equal;
3. the lemmas are byte-equal;
4. the UPOS values are byte-equal and non-empty, so a legacy `upos = ''` row never
   matches a rich identity;
5. the language is not `grc` (OD-24).

A legacy tuple is never a wildcard over discriminated identities. `λέξισ` NOUN
Known as a legacy row does not make `λέξισ` NOUN `example:ex:1` or `example:ex:2`
Known or Reserved. The comparison is never used to transfer Known across a split
(one identity becomes several) or a merge (several become one). Facts stay on the
identity they were recorded for. Any reconciliation needs a separately approved
plan (ADR 0098, handoff Stage 2).

### Legacy compatibility

- Existing tuple rows stay as they are, with no backfill and no rewrite. That
  covers Known (B-11), Reserved snapshot rows (B-13), Generated and deck
  preparation rows (B-14), deck specification items (B-15), Browse counts (B-16),
  Reading history (B-12) and corrections, flags and exclusions (B-06, B-07).
- Modern languages keep their profiles (`german-standard-post-1996` 6,
  `modern-greek` 1, `unicode-casefold` 1.2.0), their keys, their blank-UPOS Known
  wildcard and their selection outcomes. Their rich identity is their tuple with a
  null discriminator. The German parity fixture
  (`internal/canonicalization/german_parity_test.go:12-30`) and every existing
  store-contract scenario pass unchanged.
- `grc` and `el` are different languages with separate identities, Known,
  Reserved, Browse and counts, even for byte-equal lemmas. `NormalizeLanguage`
  keeps `grc` and `el` apart (`internal/canonicalization/canonicalization.go:162-172`).
- **Legacy `grc` rows.** §3 expects none (B-33), but the casefold fallback
  (B-18) could have created some. The §13 preflight counts them per owner and
  table. If it finds any:
  - they stay unchanged and readable in their existing views, under their
    original casefold interpretation;
  - they match no rich `grc` identity (comparison rule 5), so they neither mark
    rich identities Known nor reserve them;
  - lemma review on their legacy analysis is unavailable, because `Lookup("grc",
    "1.2.0")` fails (I-5); they are never renormalized;
  - the preflight report lists them, and no reconciliation happens without a
    separately approved plan. Rollout of the classical path continues (OD-24).

  A casefold key expands iota subscripts. For example, casefold maps `λῄξις` to
  `λήιξισ`, which is byte-equal to the `ancient-greek/1` key of the adscript
  spelling `λήιξις`. Applying the plain legacy rule to `grc` would therefore
  transfer Known across a subscript and adscript distinction. Rule 5 prevents
  that.

### Effective identity and Lemma corrections

- The **effective vocabulary identity** of an occurrence in a classical Exact
  analysis is:
  - none, if an Occurrence exclusion applies;
  - the rich Lemma correction's identity, if one applies;
  - otherwise, the analysis's identity for the occurrence (§7).
- A correction targets one exact occurrence (§7) in one Exact analysis, and
  produces a rich identity. It never edits native evidence, the decoded form,
  the Exact analysis or the Analysis headword.
- The learner enters a citation form. It is trimmed, put in NFC and admitted,
  then normalized with the profile named by the analysis's interpretation, found
  by `Lookup` with the exact version.
- UPOS is always the occurrence's canonical UPOS. Learners do not edit POS.
- **Discriminator** (OD-23):
  - If the resulting key equals the occurrence's key, the decision is `keep` and
    stores nothing, as today. This also holds when the occurrence has a
    discriminator, so a learner cannot set, change or clear one.
  - If the key differs, the corrected identity has a null discriminator.
- Only a reviewed interpretation (source decoding, lemma mapping or repair layer)
  sets a discriminator. That produces a new Interpretation configuration and Exact
  analysis (§5).
- A correction keeps the profile it was confirmed under. It is never
  reinterpreted under a new profile, and never carried to another analysis,
  occurrence, Book or owner.
- A correction's `corrected_citation_form` is the headword for that occurrence's
  effective identity in that Book.

### Invariants

Each invariant is covered by the `IDN` rows in [§15](#15-acceptance-test-matrix).

1. **Normalization.**
   - Canonically equivalent inputs give one key (IDN-01, IDN-02).
   - Case and final sigma do not distinguish keys (IDN-03, IDN-04).
   - Iota subscript and adscript give different keys. A capital with
     prosgegrammeni keeps its subscript (IDN-05).
   - Acute and grave give different keys (IDN-06).
   - Combining U+0313 and U+2019 are preserved and give different keys (IDN-07).
   - Malformed stacked marks are kept in the key, not repaired (IDN-08).
   - No compatibility mapping is applied (IDN-09).
   - Lowercasing uses simple mappings (IDN-10).
   - The profile is idempotent, and every implementation that computes `grc`
     keys (I-1) reports Unicode 17.0.0 (IDN-11).
   - Input that is not admitted is rejected, never repaired (IDN-12).
2. **Profile resolution.**
   - No new `grc` key is computed with casefold. `For`, `Lookup` and validation
     never resolve `grc` to the fallback (IDN-13), and no learner route casefolds
     `grc` input (IDN-14). Existing legacy casefold `grc` rows are only read
     (IDN-27, IDN-33).
   - `grc` and `el` resolve to different profiles and give distinct identities for
     the same spelling (IDN-15, IDN-16).
3. **Identity.**
   - The same lemma and UPOS with discriminators `example:ex:1` and `example:ex:2`
     are distinct identities, and both differ from the null-discriminator identity
     (IDN-17).
   - At most one Vocabulary identity row exists per five-value identity. A half-set
     discriminator, an empty lemma or an empty UPOS is rejected, and rows never
     change (IDN-18, IDN-19).
   - No identity's lemma carries a discriminator marker (IDN-20).
   - Normalization never infers a discriminator. Decoding produces it from
     reviewed native data only (IDN-21).
   - The profile version is provenance: unchanged identities match across
     versions (IDN-22).
4. **Learner state.**
   - Discriminated identities stay distinct in Known, Reserved, Current-reading
     vocabulary snapshots and Reading completion (IDN-23).
   - A legacy null-discriminator Known row does not mark `example:ex:1` or
     `example:ex:2` Known or Reserved (IDN-24).
   - A legacy `upos = ''` Known row never matches a rich identity (IDN-25).
   - Known is never transferred across a split or merge (IDN-26).
   - Legacy `grc` rows match no rich `grc` identity and are not rewritten (IDN-27).
5. **Headword.**
   - A learner sees the decoded citation form, with final `ς`, never the key
     (IDN-28).
   - Within one analysis the headword is deterministic, and variants are reported
     (IDN-29).
   - Two analyses that disagree each show their own headword. Frozen records keep
     theirs (IDN-30).
   - Cross-Book views follow the stated preference order (IDN-37).
   - A corrected identity shows the correction's citation form in its Book
     (IDN-38).
6. **Corrections.**
   - A correction uses the analysis's profile, yields a null discriminator, and
     leaves evidence unchanged (IDN-31).
   - A learner cannot set or clear a discriminator (IDN-32).
   - No lemma review runs on a legacy casefold `grc` analysis (IDN-33).
7. **Search.** A `grc` Concordance lemma lookup in NFD, upper case or with final
   sigma reaches the same results as the NFC citation form, through the server
   (IDN-34). Form search is OD-26.
8. **Modern languages.** Identity, normalization and outcomes for `de`, `el`,
   `it` and unregistered languages are unchanged (IDN-35, IDN-36).

### Conflicts and dependencies noted

- Issue item 7 says Lemma corrections "may change lemma or UPOS". Today a
  correction changes the lemma only (`internal/selection/selection.go:264-274`).
  This section keeps UPOS unchanged and lets only a reviewed interpretation change
  it (OD-23).
- OD-02 is resolved here as option (1), the dedicated-profile registry rule.
- Legacy comparison rule 4 (non-empty UPOS) goes beyond the issue's wording. It
  is needed because Known matching treats `upos = ''` as a wildcard
  (`sqlc/queries/core.sql:238-239`), which would otherwise match every rich UPOS.
  Rule 5 is OD-24.
- §6 names a separate `source_decoding` version, which assumes OD-13 option (1).
  Under option (2), decoding and normalization stay separate steps, but a
  decoding change is versioned through the lemma-mapping identifier.
- The #2033 rule for highlighting (verified target spans, `grc`-scoped fallback
  matching, no erased diacritics or subscript and adscript equivalence) belongs
  to §7 occurrence alignment. Search keys never establish identity here.
- The per-occurrence decoded citation form and discriminator, and the occurrence
  key that corrections target, belong to §7.
- Where rich identities attach to Known, snapshots, completion and deck
  specifications, and where a frozen headword is stored, belong to §8. §8 must
  also keep OD-01: no rich identity reaches the tuple-keyed Browse-selection or
  Custom-deck stores.
- The admission check, reason codes and the `canonical_key_mismatch` rejection
  belong to §10's interface. This section defines what they check.
- The Analysis headword rows are written in the Exact analysis's single
  transaction, which §5 defines.
- The legacy `grc` preflight query and report belong to §13.
- Public fixture content rules belong to §14.

## 7. Canonical analysis and exact occurrences

Verified at commit `2ac8b48cdbf088e4b99e83f202774497199f5eb4` (`main`). Every
`path:line` in this section refers to that commit. No file outside `doc/` changed
between §3's baseline commit and this one, so §3's citations still hold.

### Summary

- A validated Exact analysis (§5) stores its **sentences**, **nodes**, **span
  references**, **container components**, **Annotation occurrences** and the
  optional **morphology**, **basic syntax** and **secondary syntax** layers. All
  of them are shared, immutable and written in the analysis's single
  transaction.
- A node is identified by `(analysis_id, node_id)`. `node_id` is an
  analysis-scoped integer. Native identifiers are kept as evidence and scoped to
  their source revision and document. They never identify a node across
  analyses.
- Spans are a separate relation: a node has 0..N half-open Unicode code-point
  spans against the binding's Extracted text. Several nodes may share a span. One
  node may have several spans. Empty nodes have none. No offset is ever derived
  from lemma or surface length.
- Every node in the bound extent has an **accounting class**: `vocabulary`,
  `non_vocabulary` (with a reason code and a native evidence reference) or
  `unresolved`. A multiword container never counts; its attested components
  may.
- An **Annotation occurrence** is `(analysis_id, node_id)` for a word or
  component node whose class is `vocabulary` or `unresolved`. Offsets are display
  data and never identify an occurrence or transfer a decision.
- Optional layers keep native values. Canonical FEATS, UPOS and relations exist
  only where a reviewed mapping supplies them. Each layer's availability is the
  §5 Capability diagnostic for that capability, narrowed by sentence and node
  findings. Invalid syntax is retained and withheld from consumers.
- Legacy analyses stay in `corpus_tokens` and `corpus_sentences` and are never
  rewritten. New analyses write only the new representation. Every consumer reads
  both through one **analysis read interface** (OD-28, option (i)).

New record names are proposed terms (OD-31). Table names in the illustrative DDL
are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| A token's key is its position `(analysis_run_id, sentence_ordinal, token_ordinal)`. Surface, lemmas, UPOS, offsets, dependency and head are `NOT NULL`, with `dependency <> ''` and `head >= 0` checked. | `migrations/000001_initialize.up.sql:380-404`, `:1648` (B-01, B-02) | Classical analyses cannot be stored there without inventing values. They get their own records. Legacy rows keep their shape. |
| A legacy sentence has one span and a stored `sentence_text`. | `migrations/000001_initialize.up.sql:360-373`; written by `insertCorpusSentences` at `internal/analysis/jobs.go:1759-1787` | A classical sentence has 1..N spans and no stored text. Its text is the slices of the Extracted text. |
| `corpusTokenInserter.add` passes every token through `validateCorpusToken` before batching it. | `internal/analysis/jobs.go:1789-1822`, `:1837-1847` | The legacy write path is unchanged. Classical analyses are written by the §11 adoption path after §10 validation, never through this inserter. |
| `validateCorpusToken` checks only a non-empty dependency, a head within the sentence, and that a `root` token is self-headed. It does not check one root per sentence, cycles or connection. | `internal/analysis/jobs.go:1849-1862` | The legacy check is not the §7 tree rule. The read interface does not re-run the §7 rule on legacy rows (OD-29). |
| The Stanza producer flattens multiword tokens into words. A word without its own character offsets takes the token's offsets, and a missing end becomes `start + len(word.text)`. | `nlp/src/mouseion_nlp/producer.py:255-262` | Legacy components already share their container's span, and the container itself is not stored. The length fallback is a legacy derivation that this section forbids for classical analyses (rule SPAN-4). |
| The producer drops a word with no usable lemma, with only a log line. | `nlp/src/mouseion_nlp/producer.py:266-271` | A classical node with no usable identity is `unresolved` and blocks Vocabulary-ready. It is never dropped (rule ACC-1). |
| Occurrence identity is `(SourceDocumentID, StartOffset, EndOffset)`. | `internal/selection/selection.go:44-51` | Legacy occurrence keys stay offset-keyed. Classical keys are `(analysis_id, node_id)`. |
| Sentence study counts the persisted words a decision's span addresses, and attributes the decision only when the count is one. | `sqlc/queries/concordance.sql:43-68` | Legacy fail-safe behaviour stays. Classical decisions address one node and need no span-uniqueness proof. |
| `concordance_occurrences` joins the head token by `token_ordinal = t.head`. Sentence study shows each token's dependency and head. | `migrations/000018_remove_reading_journey.up.sql:6-34`; `internal/webapp/concordance.templ:186` | With syntax unavailable, there is no head to join and no relation to show. The interface reports "unavailable" with a reason, never an empty relation. |
| `OccurrenceEligible` rejects dependency `compound:prt`. | `internal/selection/selection.go:281-285` | Syntax-based exclusions apply only where the node's basic syntax is usable (OD-03, option (1)). |
| Concordance form search lowercases surfaces in SQL. | `sqlc/queries/concordance.sql:115` | Classical nodes carry a stored `ancient-greek/1` form search key (OD-26, option (1)). |

### Records

All records in this section are **shared** (no owner column) and reached only
through the §5 access rule for their Exact analysis. They are **immutable**:
written only in the Exact analysis's single transaction, with every `UPDATE`
rejected by a mirrored trigger (§5), and no insert accepted for an analysis
that has already committed. Default `ON DELETE` is `RESTRICT`. Offsets are
integers in Unicode code points into the binding's Extracted text (§5), which
holds exactly the Reading unit's extent.

#### Analysis sentence

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One sentence of an Exact analysis, as the annotation segments it within the bound extent. | `analysis_id uuid`; `sentence_ordinal integer` (0-based, dense, in reading order); `native_member_path text` (the annotation revision member the sentence comes from); `native_document_ref text NULL` (native document or subdocument identifier, for example AGDT `document_id`/`subdoc`; NULL when the scheme has none); `native_sentence_ref text` (native sentence identifier within that member and document); `boundary text` ∈ {`complete`, `incomplete`, `unknown`}; `boundary_reason text NULL` (reason code, required unless `complete`, for example `unit_extent_cuts_sentence`, `source_boundary_undeclared`); `reconstruction text NULL` and `reconstructed_text text NULL` (both or neither: a reviewed transformation `name/version` and its output) | `(analysis_id, sentence_ordinal)` | `(analysis_id, native_member_path, native_document_ref, native_sentence_ref)` unique `NULLS NOT DISTINCT`. | Shared. | **Immutable.** | `analysis_id` → Exact analysis `RESTRICT` |

The sentence's **text** is the ordered slices of the Extracted text at its
spans. It is not stored. `reconstructed_text` exists only when a reviewed
transformation (for example spacing or punctuation reconstruction from
annotation-carried text, ADR 0093) produced it. Consumers then show it labelled
with that transformation and still highlight against the spans. No consumer
joins token surfaces into sentence text.

`boundary` is the adapter's declaration, checked by validation (rule SENT-3):

- `complete`: the native sentence lies wholly inside the extent, and the source
  declares its boundaries.
- `incomplete`: the native sentence continues before the extent's start or after
  its end. Only its in-extent part is in the analysis.
- `unknown`: the source does not declare sentence boundaries in a way the
  reviewed adapter can verify.

#### Sentence span

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One half-open span of a sentence. | `analysis_id uuid`; `sentence_ordinal integer`; `span_ordinal integer` (0-based, in text order); `start_cp integer`; `end_cp integer` | `(analysis_id, sentence_ordinal, span_ordinal)` | Key only. | Shared. | **Immutable.** | `(analysis_id, sentence_ordinal)` → Analysis sentence `RESTRICT` |

#### Analysis node

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One node of the annotation: a word, a multiword container, a component, an unprinted (empty) node or punctuation. | `analysis_id uuid`; `node_id integer` (≥ 1; position in the analysis's canonical node order, see below); `sentence_ordinal integer`; `native_node_ref text` (native node identifier, scoped to the sentence's native member, document and sentence; for example an AGDT token `id` or a CoNLL-U `ID` such as `4`, `4-5` or `4.1`); `native_cite text NULL` (native citation such as AGDT `cite`; evidence only); `kind text` ∈ {`word`, `multiword_container`, `component`, `empty`, `punctuation`}; `surface text NULL` (native form, NFC not applied; NULL exactly for `empty`); `raw_lemma text NULL` (native lemma, verbatim); `citation_form text NULL` (decoded citation form, §6); `discriminator_scheme text NULL`, `discriminator_value text NULL` (decoded, §6; both or neither); `identity_id uuid NULL` (rich identity, §6); `native_pos text NULL` (native POS or class, verbatim); `upos text NULL`; `upos_state text` ∈ {`resolved`, `ambiguous`, `unsupported`, `missing`}; `upos_candidates text[]` (non-empty exactly when `ambiguous`; UD tags); `accounting_class text` ∈ {`vocabulary`, `non_vocabulary`, `unresolved`}; `non_vocabulary_reason text NULL`, `native_evidence_ref text NULL` (both required exactly when `non_vocabulary`); `form_search_key text NULL` (OD-26; `ancient-greek/1` key of `NFC(surface)` for `grc`; NULL for `empty`) | `(analysis_id, node_id)` | `(analysis_id, sentence_ordinal, native_node_ref)` unique. | Shared. | **Immutable.** | `(analysis_id, sentence_ordinal)` → Analysis sentence `RESTRICT`; `identity_id` → Vocabulary identity `RESTRICT` |

Checks on a node row:

- `upos` is non-NULL exactly when `upos_state = resolved`.
- `identity_id` non-NULL requires `upos_state = resolved`. The identity's UPOS
  equals `upos`, and its discriminator equals the node's. A trigger checks this.
- `kind = empty` ⇔ `surface IS NULL`.
- `kind ∈ {multiword_container, empty, punctuation}` ⇒
  `accounting_class = non_vocabulary`, with reason `multiword_container`,
  `empty_node` or `punctuation` respectively.
- `accounting_class = vocabulary` ⇒ `kind ∈ {word, component}`, `identity_id`,
  `citation_form` and `upos` non-NULL.

**Canonical node order and `node_id`.** The adapter assigns `node_id`
deterministically from the bound evidence:

1. sentences in `sentence_ordinal` order;
2. within a sentence, native node order;
3. a container immediately before its first component;
4. an empty node immediately after the node it is anchored to (for example
   CoNLL-U `4.1` after `4`).

The same binding, interpretation and adapter therefore always give the same
`node_id` values (§5 determinism). `node_id` is stable for the life of the
analysis because the analysis is immutable. It is not comparable across
analyses.

**Non-vocabulary reason codes** come from a registry in reviewed code that is
versioned with the canonical contract (§5 `contract_version`). The structural
codes are `punctuation`, `empty_node` and `multiword_container`. Every other
code names a source-supported native class that the reviewed interpretation
maps to non-vocabulary, for example `native_conjunction_class`. A reason code is
never a learner decision. A non-vocabulary classification is not an Occurrence
exclusion (ADR 0094 "Usability is capability-specific"). `native_evidence_ref`
names the native attribute that justifies the class, for example
`pos@native_node_ref`, so it can be inspected.

**Example: non-vocabulary conjunction (synthetic, per #2058 item 1).** Scheme
`example`. Node 7 is printed `ξέλ` at span `[40,43)`. Raw lemma `ξέλ`. Native POS
`example:C`, which the reviewed definition documents as the conjunction class.
The interpretation's POS mapping cannot choose between CCONJ and SCONJ from the
native record. The row is:

| field | value |
| --- | --- |
| `kind` | `word` |
| `citation_form` | `ξέλ` |
| `identity_id` | NULL |
| `native_pos` | `example:C` |
| `upos` | NULL |
| `upos_state` | `ambiguous` |
| `upos_candidates` | `{CCONJ,SCONJ}` |
| `accounting_class` | `non_vocabulary` |
| `non_vocabulary_reason` | `native_conjunction_class` |
| `native_evidence_ref` | `pos@7` |

It is accounted for, so it does not block Vocabulary-ready (rule ACC-2). It has
no Annotation occurrence, so it is neither counted nor offered for a learner
decision. A node finding records `ambiguous` UPOS with reason
`upos_mapping_ambiguous`. No `X` is invented, and the node is not omitted.

#### Node span

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One verified half-open span of a node against the Extracted text. | `analysis_id uuid`; `node_id integer`; `span_ordinal integer` (0-based, in text order); `start_cp integer`; `end_cp integer` | `(analysis_id, node_id, span_ordinal)` | Key only. **No** uniqueness on `(analysis_id, start_cp, end_cp)`: several nodes may share a span. | Shared. | **Immutable.** | `(analysis_id, node_id)` → Analysis node `RESTRICT` |

- **Shared span:** nodes 4 and 5 each have one row `[10,16)`.
- **Discontinuous span:** node 9 has rows `[2,5)` (ordinal 0) and `[9,12)`
  (ordinal 1).
- **Empty node:** no rows.

#### Container component

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Links a multiword container to one of its linguistic components. | `analysis_id uuid`; `container_node_id integer`; `component_node_id integer`; `component_order integer` (0-based) | `(analysis_id, component_node_id)` | `(analysis_id, container_node_id, component_order)` unique. A component belongs to exactly one container. | Shared. | **Immutable.** | both node references → Analysis node `RESTRICT` |

A trigger checks that the container's `kind` is `multiword_container`, the
component's is `component`, and both are in the same sentence. Validation checks
the remaining rules (CONT-1 to CONT-3).

**Example (synthetic).** Container node 3 has surface `λέξισῥῆσ` and span
`[10,18)`. Its components are node 4 (`λέξις` NOUN `example:ex:1`) and node 5
(`ῥῆσις` NOUN). The source gives no finer offsets, so nodes 4 and 5 each
reference `[10,18)`. The container is `non_vocabulary(multiword_container)`, and
both components are `vocabulary`. The count is 2, not 3.

#### Annotation occurrence

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The exact occurrence that counts, Lemma corrections, Occurrence exclusions and Lemma review flags address (§8). | `analysis_id uuid`; `node_id integer`; `occurrence_ordinal integer` (0-based, dense, in canonical node order) | `(analysis_id, node_id)` | `(analysis_id, occurrence_ordinal)` unique. | Shared. Decisions on it are owner-scoped (§8). | **Immutable.** | `(analysis_id, node_id)` → Analysis node `RESTRICT`. Target of §8 decision references `RESTRICT`. |

A row exists **exactly** for each node whose `kind ∈ {word, component}` and
`accounting_class ∈ {vocabulary, unresolved}` (rule OCC-ID-2). A trigger rejects
a row for any other node, and validation rejects an analysis that is missing
one. Materializing the row gives §8 decisions a foreign-key target that cannot
name punctuation, a container, an empty node or a non-vocabulary node.
Non-vocabulary word nodes, such as the conjunction example, are accounted for
but are not occurrences (OD-30).

An occurrence **contributes one count** exactly when its node's class is
`vocabulary`. Existing language-specific eligibility (allowed POS, syntax-based
exclusions) then applies on top of that, in the learner core (§8, §10). An
`unresolved` occurrence never counts. It exists so diagnostics and inspection
can name it, and because it blocks Vocabulary-ready (rule ACC-3).

#### Node morphology

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The morphology layer for one node. | `analysis_id uuid`; `node_id integer`; `native_morphology jsonb` (native features or tag exactly as the source gives them, as an object of strings, for example `{"postag":"n-s---fn-"}`); `feats jsonb` (canonical UD FEATS, an object holding only the features a reviewed mapping supplies; may be `{}`) | `(analysis_id, node_id)` | Key only. | Shared. | **Immutable.** | `(analysis_id, node_id)` → Analysis node `RESTRICT` |

No row means the source gives no morphology for the node. A native feature
without a reviewed mapping stays only in `native_morphology` and gets a node
finding (`morphology`, `unsupported` or `ambiguous`). It is never guessed into
`feats`.

#### Basic syntax edge

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One node's basic dependency: its head and relation. | `analysis_id uuid`; `node_id integer`; `head_node_id integer` (equal to `node_id` only for an actual root); `native_relation text NULL`; `relation text NULL` (UD basic relation where a reviewed mapping supplies it; `root` exactly when self-headed) | `(analysis_id, node_id)` | Key only: at most one basic head per node. | Shared. | **Immutable.** | `(analysis_id, node_id)` and `(analysis_id, head_node_id)` → Analysis node `RESTRICT` |

Checks: `head_node_id = node_id` ⇔ `relation = 'root'`. Both nodes are in the
same sentence. Neither is `empty` or `multiword_container`. A row is written
only for a node the source gives a head for. A node with no native head gets no
row, and the adapter never makes it a root.

#### Secondary syntax edge

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One native secondary or enhanced edge, such as a PROIEL `slash` or a CoNLL-U `DEPS` edge, including edges to and from empty nodes. | `analysis_id uuid`; `edge_ordinal integer`; `node_id integer`; `head_node_id integer`; `native_relation text`; `relation text NULL` (mapped enhanced relation where reviewed) | `(analysis_id, edge_ordinal)` | `(analysis_id, node_id, head_node_id, native_relation)` unique. | Shared. | **Immutable.** | both nodes → Analysis node `RESTRICT` |

Secondary edges are never folded into basic syntax, and basic syntax is never
derived from them.

```sql
-- illustrative, not a migration
CREATE TABLE analysis_nodes (
  analysis_id uuid NOT NULL,
  node_id integer NOT NULL CHECK (node_id >= 1),
  sentence_ordinal integer NOT NULL,
  native_node_ref text NOT NULL CHECK (native_node_ref <> ''),
  native_cite text NULL,
  kind text NOT NULL CHECK (kind IN ('word','multiword_container','component','empty','punctuation')),
  surface text NULL,
  raw_lemma text NULL,
  citation_form text NULL,
  discriminator_scheme text NULL,
  discriminator_value text NULL,
  identity_id uuid NULL REFERENCES vocabulary_identities(id) ON DELETE RESTRICT,
  native_pos text NULL,
  upos text NULL,
  upos_state text NOT NULL CHECK (upos_state IN ('resolved','ambiguous','unsupported','missing')),
  upos_candidates text[] NOT NULL DEFAULT '{}',
  accounting_class text NOT NULL CHECK (accounting_class IN ('vocabulary','non_vocabulary','unresolved')),
  non_vocabulary_reason text NULL,
  native_evidence_ref text NULL,
  form_search_key text NULL,
  PRIMARY KEY (analysis_id, node_id),
  UNIQUE (analysis_id, sentence_ordinal, native_node_ref),
  FOREIGN KEY (analysis_id, sentence_ordinal) REFERENCES analysis_sentences(analysis_id, sentence_ordinal) ON DELETE RESTRICT,
  CHECK ((kind = 'empty') = (surface IS NULL)),
  CHECK ((upos_state = 'resolved') = (upos IS NOT NULL)),
  CHECK ((upos_state = 'ambiguous') = (cardinality(upos_candidates) > 0)),
  CHECK ((discriminator_scheme IS NULL) = (discriminator_value IS NULL)),
  CHECK ((accounting_class = 'non_vocabulary') = (non_vocabulary_reason IS NOT NULL)),
  CHECK ((accounting_class = 'non_vocabulary') = (native_evidence_ref IS NOT NULL)),
  CHECK (kind IN ('word','component') OR accounting_class = 'non_vocabulary'),
  CHECK (accounting_class <> 'vocabulary'
         OR (identity_id IS NOT NULL AND citation_form IS NOT NULL AND upos IS NOT NULL))
);

CREATE TABLE analysis_node_spans (
  analysis_id uuid NOT NULL,
  node_id integer NOT NULL,
  span_ordinal integer NOT NULL CHECK (span_ordinal >= 0),
  start_cp integer NOT NULL CHECK (start_cp >= 0),
  end_cp integer NOT NULL CHECK (end_cp > start_cp),
  PRIMARY KEY (analysis_id, node_id, span_ordinal),
  FOREIGN KEY (analysis_id, node_id) REFERENCES analysis_nodes(analysis_id, node_id) ON DELETE RESTRICT
);

CREATE TABLE annotation_occurrences (
  analysis_id uuid NOT NULL,
  node_id integer NOT NULL,
  occurrence_ordinal integer NOT NULL CHECK (occurrence_ordinal >= 0),
  PRIMARY KEY (analysis_id, node_id),
  UNIQUE (analysis_id, occurrence_ordinal),
  FOREIGN KEY (analysis_id, node_id) REFERENCES analysis_nodes(analysis_id, node_id) ON DELETE RESTRICT
);
-- plus: a trigger rejecting an occurrence whose node is not a word or component
-- classed vocabulary or unresolved; mirrored update triggers on every table;
-- a trigger rejecting inserts for an analysis committed in an earlier transaction.

CREATE TABLE analysis_basic_syntax (
  analysis_id uuid NOT NULL,
  node_id integer NOT NULL,
  head_node_id integer NOT NULL,
  native_relation text NULL,
  relation text NULL,
  PRIMARY KEY (analysis_id, node_id),
  FOREIGN KEY (analysis_id, node_id) REFERENCES analysis_nodes(analysis_id, node_id) ON DELETE RESTRICT,
  FOREIGN KEY (analysis_id, head_node_id) REFERENCES analysis_nodes(analysis_id, node_id) ON DELETE RESTRICT,
  CHECK ((head_node_id = node_id) = (relation IS NOT DISTINCT FROM 'root'))
);
```

### Occurrence identity

- An Annotation occurrence is identified by `(analysis_id, node_id)`, and only
  by that pair.
- Offsets are display data. No store method, query or validation step
  identifies an occurrence, attributes a decision or transfers a decision by
  offset equality, span overlap, surface, lemma, `native_node_ref` or
  `native_cite`.
- Two nodes with identical spans are two occurrences with two keys, and can have
  different decisions (§8).
- Equal `native_node_ref` values in two analyses, for example under two
  interpretations of the same binding, give two different occurrences. No
  decision, flag or count moves from one to the other (#2058 qualification,
  ADR 0094 "Adapters and publication compatibility").
- §5 Diagnostic finding references resolve to this section's keys:
  `sentence_ref` is `(analysis_id, sentence_ordinal)` and `node_ref` is
  `(analysis_id, node_id)`.

**Highlighting.** A target is highlighted at its node's spans, sliced from the
Extracted text, which is exactly the text the reader displays. A discontinuous
node highlights each slice. Components that share their container's span
highlight that whole span, and the UI names which component is the target. If a
span's slice cannot be shown, for example because a reconstruction changed the
displayed text, the alignment failure is reported, and no highlight is guessed
from the lemma or surface (#2033 "Exact evidence, headwords and source
matching"). Search keys never place a highlight.

### Optional layers and availability

Each layer's availability is the §5 **Capability diagnostic** for its
capability (`morphology`, `basic_syntax`, `secondary_syntax`), narrowed per
sentence or node by **Diagnostic findings**. No §7 record repeats the state.
Validation (§10) derives the states from the stored rows as follows.

**Morphology.**

- `missing`: no node has a morphology row.
- `unsupported`: rows exist, but the interpretation has no reviewed FEATS
  mapping for the scheme. `native_morphology` is still retained.
- Otherwise `usable`, with a node finding for each node whose native features are
  partly unmapped (`unsupported`) or ambiguous.

**Basic syntax.** A sentence's **tree participants** are its nodes of kind
`word`, `component` and `punctuation`. Empty nodes and containers never
participate. A sentence has a **complete basic tree** exactly when rules TREE-1
to TREE-4 hold. For each sentence:

- no participant has a basic-syntax row: sentence finding `missing`
  (`syntax_absent`);
- rows exist but the tree is not complete: sentence finding `invalid`, with one
  reason per violated rule (`root_missing`, `root_multiple`, `tree_cycle`,
  `tree_disconnected`, `head_unresolved`);
- complete, but some participant's `relation` is NULL: sentence finding
  `unsupported` (`relation_unmapped`);
- otherwise no finding: the sentence's basic syntax is usable.

The release-scope state is `missing` when no sentence has any row,
`unsupported` when the interpretation declares no reviewed relation mapping,
`invalid` when rows exist but no sentence has a complete tree, and otherwise
`usable`, with the findings above for every other sentence.

**Secondary syntax.** `missing` when there are no edges. `unsupported` when edges
exist but no relation is mapped. `usable` otherwise. Secondary syntax is not
tree-validated. Its endpoints must be nodes of the same sentence (rule SYN-2).

**Withholding.** The read interface returns a layer's values for a sentence or
node only when its effective state is `usable` and no Usability restriction
(§5) names the capability. Otherwise it returns the state and reason codes and
no values. Stored rows of an invalid tree stay inspectable in the owner's
evidence detail (§5 access rule). They are never served to Concordance,
sentence selection or eligibility.

**Eligibility with unavailable syntax (OD-03, option (1)).** A syntax-based
exclusion, such as today's `compound:prt` rule
(`internal/selection/selection.go:281-285`), applies to an occurrence only when
the occurrence's basic syntax is usable. When syntax is unavailable, the
occurrence's eligibility is decided by its identity and UPOS alone. Unavailable
syntax is never evidence that a relation is absent (ADR 0094).

### Validation rules

These are the rules the shared canonical-validation interface (§10) applies to a
candidate before any write. §10 references them by ID. "Extent" is the binding's
Extracted text.

**Spans.**

- **SPAN-1.** Every span satisfies `0 ≤ start_cp < end_cp ≤ code_point_length`
  of the Extracted text.
- **SPAN-2.** A node's or sentence's spans are ordered by `start_cp`, pairwise
  disjoint and non-adjacent (`end_cpᵢ < start_cpᵢ₊₁`). Adjacent spans are one
  span.
- **SPAN-3.** Nodes of kind `word`, `multiword_container`, `component` and
  `punctuation` have at least one span. `empty` nodes have none.
- **SPAN-4.** Every span is the adapter's verified mapping of native coordinates
  or native text into the Extracted text. No span is computed from the length of
  a surface, lemma or citation form.
- **SPAN-5.** Each node's spans lie within the union of its sentence's spans.

**Sentences.**

- **SENT-1.** Sentence ordinals are dense from 0, in order of first `start_cp`.
  Sentence spans are pairwise disjoint across the analysis.
- **SENT-2.** A sentence has at least one span and at least one node.
- **SENT-3.** `boundary = complete` only when the native sentence lies wholly in
  the extent and the source declares its boundaries. A native sentence that the
  extent cuts is `incomplete`. A sentence the adapter cannot verify is
  `unknown`. The adapter never upgrades `unknown` to `complete`.
- **SENT-4.** A candidate that supplies sentence text is accepted only if the
  text equals the slices at its spans, or if it names a reviewed reconstruction
  transformation. Otherwise it is rejected with `sentence_text_unbound`.

**Containers.**

- **CONT-1.** A `multiword_container` has at least two components. A `component`
  has exactly one container.
- **CONT-2.** A component's spans lie within its container's spans. A component
  without native finer offsets references exactly its container's spans.
- **CONT-3.** A container contributes no count. Each of its components is
  classified on its own.

**Occurrence identity.**

- **OCC-ID-1.** `node_id` values are dense from 1, in canonical node order.
  `native_node_ref` is unique within its sentence.
- **OCC-ID-2.** An Annotation occurrence exists for exactly the nodes with
  `kind ∈ {word, component}` and `accounting_class ∈ {vocabulary, unresolved}`.

**Accounting.**

- **ACC-1.** Every node in the extent has exactly one accounting class. A node
  that the source attests but the interpretation cannot classify is
  `unresolved`. It is never dropped.
- **ACC-2.** `non_vocabulary` requires a registered reason code and a native
  evidence reference. Structural kinds (container, empty, punctuation) are
  always `non_vocabulary` with their structural reason. A reason other than a
  structural one requires a reviewed native-class mapping in the
  interpretation.
- **ACC-3.** A Vocabulary-ready analysis has zero `unresolved` occurrences.
- **ACC-4.** A Vocabulary-ready analysis covers the full declared unit. Every
  code point of the Extracted text that is not `White_Space` lies in some
  printed node's span (`word`, `multiword_container`, `component` or
  `punctuation`). An uncovered code point is a `coverage_gap` finding and
  blocks Vocabulary-ready. Material that is not part of the text, such as
  apparatus, is removed by the binding's reviewed projection (§5), never by the
  analysis.
- **ACC-5.** A `vocabulary` node has a rich identity, a decoded citation form and
  a resolved UPOS. Its identity's UPOS and discriminator equal the node's.

**Trees.** For one sentence's tree participants:

- **TREE-1.** Every participant has exactly one basic-syntax row, and its head is
  a participant of the same sentence (`head_unresolved` otherwise).
- **TREE-2.** Exactly one participant is self-headed with relation `root`
  (`root_missing`, `root_multiple`). A self-headed row whose relation is not
  `root`, or a `root` that is not self-headed, is rejected outright as malformed.
- **TREE-3.** Following heads from any participant reaches the root without
  revisiting a node (`tree_cycle`).
- **TREE-4.** Every participant reaches the root (`tree_disconnected`).
- **TREE-5.** The adapter never adds a root, head or relation that the source
  does not give, in order to satisfy TREE-1 to TREE-4. A failed tree is stored as
  given and is withheld.

**Syntax endpoints.**

- **SYN-1.** Basic-syntax endpoints are never `empty` or `multiword_container`
  nodes.
- **SYN-2.** Secondary-syntax endpoints are nodes of the same sentence, and may be
  `empty`.

Rules SPAN-\*, SENT-1, SENT-2, SENT-4, CONT-1, CONT-2, OCC-ID-\*, ACC-1, ACC-2,
ACC-5, TREE-2's malformed case, TREE-5 and SYN-\* **reject** a candidate. Rules
ACC-3 and ACC-4 decide `vocabulary_ready`. Rules TREE-1 to TREE-4 produce syntax
findings and never reject a candidate or change vocabulary readiness.

### Legacy coexistence and the analysis read interface

**Decision (OD-28, option (i)).** Analyses that already exist stay in
`analysis_runs`, `corpus_sentences` and `corpus_tokens`. They are never
rewritten, backfilled or copied. New Exact analyses write only the records in
this section. Nothing dual-writes. EPUB analysis keeps writing legacy rows
through `corpusTokenInserter` (`internal/analysis/jobs.go:1789-1847`), unchanged.

Every consumer that reads analysis content goes through one **analysis read
interface**. It has two adapters: an exact adapter over this section's records,
and a legacy adapter over `corpus_*` rows. The consumers are the Reading page,
Book vocabulary and Browse projection, lemma review, sentence study,
Concordance, sentence selection and deck preparation, and Finish. They do not
branch on which adapter answered.

**Contract.** Every method takes the authenticated owner and a Book, and
applies the §5 access rule (exact) or the existing current-analysis predicate
(legacy) in the same query. It returns these shapes:

| shape | fields | exact adapter | legacy adapter |
| --- | --- | --- | --- |
| Analysis ref | kind (`exact` or `legacy`), ID | `exact`, `analysis_id` | `legacy`, `analysis_run_id` + `corpus_id` |
| Sentence | ordinal; spans; text; reconstruction label; boundary and reason | as stored; text sliced from Extracted text | ordinal; one span `[start_offset, end_offset)` in its unit; stored `sentence_text`; boundary `complete` (see below) |
| Node | node key; kind; surface (nullable); raw lemma; citation form; identity; native POS; UPOS and state; accounting class; spans; container or components | as stored; identity is the rich identity | node key `(sentence_ordinal, token_ordinal)`; kind `punctuation` when UPOS is `PUNCT`, otherwise `word`; identity is the legacy tuple, with no discriminator; no citation form; accounting class `not_declared`; one span; no containers |
| Occurrence key | `exact(analysis_id, node_id)` or `legacy(source_document_id, start_offset, end_offset)` | `exact` | `legacy`, exactly today's `selection.OccurrenceIdentity` |
| Layer\<T\> (morphology, basic syntax, secondary syntax) | state; reason codes; value only when usable | derived as above, with restrictions | morphology `usable` (stored map as both native and FEATS); basic syntax `usable` (stored relation and head, unchanged); secondary syntax `missing` (`legacy_basic_only`) |

Legacy rules:

- **Legacy rows are read, never written.** No new code updates or deletes a
  `corpus_tokens`, `corpus_sentences` or `analysis_runs` row, except the
  existing EPUB write and cascade paths.
- **Legacy occurrences keep today's semantics.** They stay offset-keyed, a
  decision still needs a unique span (`internal/selection/selection.go:150-166`),
  and eligibility still runs `OccurrenceEligible` as today.
- **`not_declared`** is the legacy adapter's accounting value. It appears only
  for legacy nodes. The §7 accounting rules apply only to exact analyses.
- **Legacy boundary is `complete`.** An EPUB analysis covers whole source units.
  A legacy sentence is therefore never cut by a Reading-unit edge. Reporting
  `complete` keeps today's consumer behaviour, which reads no boundary
  declaration.
- **Legacy trees are reported as today.** The legacy adapter reports what
  `validateCorpusToken` admitted. It does not re-run TREE-1 to TREE-4 (OD-29).
- **Characterization.** For a legacy analysis, every consumer's output through the
  interface equals its output before the interface existed (OCC-11).

**Unavailable syntax in consumers.** When a Layer is not usable:

- Sentence study shows "Syntax unavailable", with the state and the reason, in
  place of the relation and head. It never shows an empty relation or a head
  token.
- A syntax-dependent Concordance query, such as a role filter or dependents of a
  lemma, returns an **unavailable** result for the analyses that lack usable
  syntax, with the reason. It never returns "no results" for them. When some
  Books have usable syntax and others do not, the result lists the matches from
  the usable Books and names the Books whose syntax is unavailable.
- Lemma and form lookup in Concordance do not depend on syntax and work as usual.
- Eligibility follows OD-03 option (1) above.

### Size and indexes

Public orders of magnitude for one Book-sized unit: about 10⁴ nodes and about
6×10³ eligible occurrences. Sentence counts are on the order of 10³. These are
estimates, not requirements.

| record | rows per unit (order) | note |
| --- | --- | --- |
| Analysis sentence, Sentence span | ~10³ each | about one span per sentence |
| Analysis node | ~10⁴ | words, punctuation, containers, components, empty nodes |
| Node span | ~10⁴ | about one per printed node, more for discontinuous nodes; none for empty nodes |
| Container component | 10¹–10³ | depends on the scheme's multiword tokens |
| Annotation occurrence | < 10⁴ | nodes minus punctuation, containers, empty and non-vocabulary nodes; ~6×10³ of them pass eligibility |
| Node morphology | ≤ 10⁴ | |
| Basic syntax edge | ≤ 10⁴ | |
| Secondary syntax edge | 0–10³ | |
| Diagnostic finding (§5) | ≤ 10⁴ | bounded by nodes and sentences |

That is about 6×10⁴ rows per Exact analysis. At roughly 100–250 bytes per node
row and under 60 bytes per span, occurrence or edge row, the heap is a few MB,
with indexes of the same order. The rows are stored once per Exact analysis and
shared by every owner who adopts it.

Indexes that the named queries need, beyond primary keys and the uniqueness
constraints above:

| query | index |
| --- | --- |
| Reading page and sentence study: nodes of a sentence in order | primary key `(analysis_id, node_id)` serves range scans in node order. `(analysis_id, sentence_ordinal, node_id)` serves sentence lookup. |
| Spans of a sentence's nodes | primary key `(analysis_id, node_id, span_ordinal)` |
| Browse and Book vocabulary: group occurrences by identity in one analysis | `analysis_nodes (analysis_id, identity_id)`, joined to Annotation occurrence on its primary key |
| Concordance lemma lookup across the owner's current classical analyses | `analysis_nodes (identity_id, analysis_id)` |
| Concordance form lookup (OD-26) | `analysis_nodes (form_search_key, analysis_id)` |
| Occurrence review and lemma review: occurrences of an analysis in reading order | unique `(analysis_id, occurrence_ordinal)` |
| Lemma review by exact observed form | `analysis_nodes (analysis_id, form_search_key)` |
| Concordance dependents of a head | `analysis_basic_syntax (analysis_id, head_node_id)` |
| Concordance role filter | `analysis_basic_syntax (analysis_id, relation)` |
| Container of a component | primary key of Container component |
| Findings of a sentence or node (§5) | `(analysis_id, sentence_ref)` and `(analysis_id, node_ref)` on Diagnostic finding |

Owner-scoped counts and decisions are §8 records keyed by owner and Book. They
join these shared rows by `(analysis_id, node_id)` or `identity_id`. None of
the shared indexes needs an owner column.

### Invariants

Each invariant is covered by the `OCC` rows in [§15](#15-acceptance-test-matrix).

1. **Identity.**
   - Occurrence identity is `(analysis, node)`. Two nodes with an identical span
     have two occurrence keys (OCC-01, OCC-12).
   - Equal native IDs in two analyses give two occurrences, and nothing transfers
     between them (OCC-13).
   - Offsets never identify an occurrence or transfer a decision (OCC-01, OCC-13).
2. **Spans.**
   - Shared and discontinuous spans are stored explicitly and round-trip exactly
     (OCC-02).
   - Empty nodes have no span and no count (OCC-03).
   - No span is derived from length. Component spans lie within their
     container's spans (OCC-14).
3. **Containers.** A container plus two counted components counts 2 (OCC-04).
4. **Occurrence membership.** Punctuation, containers, empty nodes and
   non-vocabulary nodes are never Annotation occurrences (OCC-05).
5. **Sentences.**
   - A boundary cut by the extent is persisted as `incomplete` (OCC-06).
   - Sentence text comes only from the bound text, or from a named reviewed
     transformation (OCC-15).
6. **Accounting.**
   - Every node is classified (OCC-16).
   - An unresolved occurrence blocks Vocabulary-ready (OCC-09).
   - A coverage gap blocks Vocabulary-ready (OCC-17).
   - Native-class non-vocabulary accounting with ambiguous UPOS is accepted
     (OCC-10).
7. **Optional layers.**
   - Missing syntax is reported `missing`, and syntax-dependent Concordance shows
     unavailable rather than zero (OCC-07).
   - An invalid tree is withheld as `invalid` while vocabulary stays usable
     (OCC-08, OCC-18).
   - No root is fabricated (OCC-18).
   - Morphology keeps native features and gives FEATS only where mapped
     (OCC-19).
   - Secondary syntax is kept separately, including empty-node edges (OCC-20).
   - Unavailable syntax never makes an occurrence ineligible (OCC-21).
8. **Persistence.** §7 records are immutable, and syntax endpoints stay within
   one analysis and one sentence (OCC-22, OCC-23).
9. **Legacy.**
   - A legacy analysis reads identically through the shared interface (OCC-11).
   - A new analysis writes no legacy rows, and no legacy row is rewritten
     (OCC-24).
10. **Search.** The `grc` form search key matches NFC and NFD, case and final
    sigma, and nothing broader (OCC-25).

### Conflicts and dependencies noted

- §5's Diagnostic finding has `sentence_ref` and `node_ref`. This section defines
  them as `(analysis_id, sentence_ordinal)` and `(analysis_id, node_id)`.
- §6 leaves the per-node decoded citation form, discriminator and occurrence key
  to this section. They are the node fields above. The Analysis headword (§6)
  is derived from the `citation_form` of `vocabulary` occurrences.
- OD-26 is placed here as `form_search_key` on the node, computed by Go
  (`ancient-greek/1`, §6 I-6).
- OD-03 is answered here as option (1). #2071 (§10) owns the eligibility
  interface that applies it.
- The coverage denominator and owner-scoped counts are §8. This section defines
  only which occurrences contribute a count.
- §10 owns the validation interface, its result type and readiness rules. It
  references the rule IDs above. §11 owns the transaction that writes these
  records with the Exact analysis.
- Whether a sentence is suitable as a card target (beyond `boundary`) belongs to
  the #2034 sentence and card rules, not to this section.

## 8. Learner-state attachment and compatibility

Verified at commit `f05c521b38378378bd725d0549c2289c843b5e3e` (`main`). Every
`path:line` in this section refers to that commit.

### Summary

- There is **one** learner lifecycle. Start, Switch, End, Finish, disposition,
  visibility and previously-read import keep their tables, locks, decision
  functions and replay rules. Rich identities add records that hang off the
  existing anchors: the Book, the Current reading row (`primary_goals`), the
  snapshot row (`primary_goal_snapshots`) and the completion row
  (`reading_history`).
- **Exact occurrence decisions** (Lemma correction, Occurrence exclusion) and
  **exact Lemma review flags** are keyed by owner, Book and occurrence
  `(analysis_id, node_id)` (§7). The offset-keyed tables are unchanged.
- **Rich count projection**: per (owner, Book, analysis), effective counts per
  rich identity plus a readiness record with state `pending | ready |
  unavailable`. It parallels `vocabulary_browse_counts` (migration 000031),
  which is unchanged.
- **Rich Known** is a membership record per (owner, identity), written only by
  Finish from a rich snapshot. **Reserved** has no table today: it is the tuple
  rows of the owner's unreleased snapshot (§3 corrections). Its rich membership
  rows are likewise the **Rich snapshot membership** rows of an unreleased rich
  snapshot, which sit beside the tuple rows on the same snapshot anchor.
- A **snapshot representation** is either legacy (tuple rows) or rich (rich
  membership rows), never both. The snapshot row says which (OD-32).
- Every consumer branches on the representation of the record it reads, never
  on the language or the producer.

New record names are proposed terms (OD-36). Table names in the illustrative DDL
are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| The Current reading is one `primary_goals` row per `(owner_id, language)`, pointing at its snapshot. | `migrations/000001_initialize.up.sql:1291-1299`, `:1968`; `migrations/000007_primary_goal_snapshots.up.sql:4-5`, `:53-54` | One current reading per owner and language, whatever the representation. A `grc` reading and an `el` reading coexist and are independent. |
| A snapshot row requires `source_material_id`, `analysis_run_id`, `content_revision_id`, `content_snapshot_id` and `corpus_id` (`NOT NULL`), with foreign keys to `source_materials`, `analysis_runs` and `corpora`. | `migrations/000007_primary_goal_snapshots.up.sql:14-33` (columns `:19-23`, references `:30-32`) | A classical reading has none of these (§5 OD-12). The snapshot row cannot be inserted for it unless the constraint is relaxed additively (OD-32). |
| Snapshot vocabulary is tuple-keyed, `occurrence_count > 0`, and cascades from its snapshot. | `migrations/000007_primary_goal_snapshots.up.sql:35-51` | B-13. Rich membership is a separate record with the same cascade. |
| A Current reading without a snapshot is the legacy no-snapshot shape. Finish accepts it with an empty snapshot ID and accepts nothing. | `internal/persistence/current_reading.go:236-240`, `:282-291`; `internal/domain/current_reading_finish.go:90-107` | A classical reading must never be represented by a NULL `snapshot_id`; it would finish as an empty legacy reading. |
| Reserved vocabulary is read from the unreleased snapshot of the language's current reading. Release sets `released_at`. | `sqlc/queries/deck_preparations.sql:289-296`; `sqlc/queries/current_reading.sql:99-107`, `:157-160` | Rich Reserved is the same predicate over rich membership. No reservation record is added. |
| Start freezes in-transaction: Known and Reserved exclusions, then the 3/15 rule with library totals from ready projections, which fail with pending or unavailable. | `internal/persistence/current_reading_lifecycle.go:46-90`, `:100-115`, `:138-154`, `:178-260` (errors `:253-258`); `internal/selection/eligibility.go:5`, `:9`, `:52-54` | A rich freeze uses the same order and the same `selection` rule. Only the fact loaders differ. |
| Known matching treats a Known `upos = ''` as a wildcard; Reserved matching is exact. | `internal/selection/eligibility.go:63-74`; `internal/domain/current_reading_finish.go:127-133` | Legacy only. Rich matching follows §6 "Identity comparison". |
| Finish writes one `reading_history` row (idempotent per snapshot) and inserts Known with set semantics, then returns the Book to Inbox, releases the snapshot and deletes the current row. A replay still ends an active reading. | `internal/persistence/current_reading.go:234-278`, `:322-343`, `:347-355`; `sqlc/queries/current_reading.sql:190-213`, `:271-333`; `migrations/000010_reading_history_append_only.up.sql:4-11` | Reused unchanged. A rich Finish writes rich Known instead of tuple Known. |
| Legacy Known completion provenance requires source, run, revision, unit snapshot and corpus together. | `migrations/000009_primary_goal_completion_provenance.up.sql:24-78` (check `:67-78`) | Rich Known cannot be stored in `known_vocabulary` without inventing those values. It gets its own record. |
| Lemma decisions take the learner-state lock, the Book row lock and the per-Book advisory lock, reject the current reading's Book, check the preview fingerprint, then update decisions and Browse counts in the same transaction. | `internal/persistence/lemma_review.go:187-214`, `:216-248`, `:256-259`, `:265-274` | Rich decisions take the same locks in the same order and keep counts exact in the same transaction. |
| A decision equal to the analyzer lemma deletes the stored correction (`keep` stores nothing). | `internal/persistence/lemma_review.go:276-286` | Same for rich decisions (§6 OD-23). |
| Flags are inserted with `ON CONFLICT DO NOTHING`, so a resolved flag is never reopened. Unresolved flags on the current analysis block Start and Switch. | `internal/persistence/lemma_review.go:63-81`, `:106-110`; `internal/persistence/current_reading_lifecycle.go:627-632`; `internal/persistence/current_reading.go:146` | Rich flags keep both rules, keyed by exact occurrence. |
| The Browse projection is per (owner, Book) with the analysis run in the readiness row and a builder version. It is built in the publication transaction and fenced by the per-Book lock. | `migrations/000031_vocabulary_browse_counts.up.sql:1-32`; `internal/persistence/vocabulary_browse_counts.go:18`, `:299-351` | B-16. The rich projection is a parallel pair of records with the same build, fence and delta rules (ADR 0087). |
| Coverage uses the analysis's analyzable-token count as denominator, so exclusions never shrink it. | `internal/persistence/projected_vocabulary.go:12-14`; `sqlc/queries/books.sql:161-180` | The rich denominator, EFF-3, is the counterpart of that analyzable-token count: fixed by the analysis, never by decisions. |
| Disposition and visibility writes touch only their own row. | `internal/persistence/book_dispositions.go:54-91`; `internal/persistence/book_visibility.go:36-66` | Reused unchanged. They write nothing rich. |
| A catalogue language change releases and deletes the Book's readings in other languages. | `internal/persistence/books.go:832-847`, `:868-880` | Reused. Releasing the snapshot is enough for rich Reserved. Classical Books do not take this path (OD-10). |
| Store-contract scenarios are fixed to `de` and seed tuples. The fixture adapter stores per-Book frozen vocabulary directly. | `internal/storecontract/harness.go:17-39`, `:99-115`, `:124-132`; `internal/fixtures/storecontract_test.go:42-69` | B-30. See [Store-contract extension](#store-contract-extension). |

### Effective identity

For an Annotation occurrence `(analysis_id, node_id)` of a classical Book `b`
of owner `o`:

- **EFF-1.** At most one Exact occurrence decision exists for `(o, b, analysis_id,
  node_id)`. The effective identity is:
  - none, when the decision is `exclude`;
  - the decision's `identity_id`, when the decision is `correct`;
  - otherwise the node's native `identity_id` (§7).

  A native identity plus at most one confirmed correction is therefore the only
  way an occurrence gets an identity. Corrections never chain or stack.
- **EFF-2.** The occurrence contributes one count to its effective identity
  exactly when its node is `vocabulary` (§7), it has an effective identity, and
  the shared language eligibility (`selection`, with OD-03 option (1) for
  syntax) accepts it. SQL never decides this (ADR 0087).
- **EFF-3.** The **coverage denominator** of an exact analysis, its counterpart
  of the legacy analyzable-token count, is the number of its occurrences that
  contribute under EFF-2 with no decisions applied. It depends
  only on the analysis. An exclusion removes the occurrence's count from its
  identity but keeps it in the denominator, and never makes it Known or
  Reserved (ADR 0081). A correction moves the count between identities and
  leaves the denominator unchanged.
- **EFF-4.** A decision is written only while its analysis is the Book's current
  classical analysis. It stays stored after a later adoption, unchanged and
  inert: it never applies to another analysis, even one whose node IDs, native
  references or spans are equal (§7 "Occurrence identity"). It is readable as
  evidence, and §9 retains what it references.
- **EFF-5.** A decision never moves to another occurrence, Book or owner, and
  is never derived from a span, surface or lemma match.

### Records

#### Exact occurrence decision

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One learner-confirmed Lemma correction or Occurrence exclusion for one exact occurrence of a classical Book. | `owner_id uuid`; `book_id uuid`; `analysis_id uuid`; `node_id integer`; `decision text` ∈ {`correct`, `exclude`}; `identity_id uuid NULL`; `corrected_citation_form text NULL`; `normalization_profile text NULL` (`name/version`, §6); `created_at`, `updated_at timestamptz` | `(owner_id, book_id, analysis_id, node_id)` | Key only: at most one decision per occurrence per owner and Book. Two occurrences on one span have two keys. | Owner. Another owner's decisions are never read. | **Mutable** only by that owner's confirmed lemma review on the Book's current analysis: replaced, or deleted for `keep`. Never changed by adoption, profile changes or other owners. | `(owner_id, book_id)` → §4 Book–Reading unit association `ON DELETE CASCADE`; `(analysis_id, node_id)` → §7 Annotation occurrence `RESTRICT`; `identity_id` → §6 Vocabulary identity `RESTRICT` |

Checks:

- `decision = correct` ⇔ `identity_id`, `corrected_citation_form` and
  `normalization_profile` are all non-NULL; `exclude` ⇔ all three NULL (mirrors
  `migrations/000024_occurrence_vocabulary_exclusions.up.sql:6-7`, `:13-15`).
- A trigger requires: the node's accounting class is `vocabulary`; for
  `correct`, the identity's UPOS equals the node's UPOS, its language equals the
  Book's language, its discriminator is NULL, and it differs from the node's
  identity (§6 OD-23); on insert or update, `analysis_id` is the Book's current
  classical analysis (EFF-4).
- At most one Book per owner can hold decisions on one analysis, because an
  analysis has one Reading unit (§5 binding) and an owner has at most one Book
  per unit (§4).

#### Exact Lemma review flag

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| A non-authoritative review flag on one exact occurrence. | `owner_id`; `book_id`; `analysis_id`; `node_id`; `reason text` (non-empty); `evidence_provenance jsonb` (object); `resolution text NULL` ∈ {`keep`, `correct`, `exclude`}; `flagged_at`; `resolved_at NULL` | `(owner_id, book_id, analysis_id, node_id)` | Key only. | Owner. | Inserted with insert-if-absent, so a resolved flag is never reopened. Only `resolution` and `resolved_at` change, together, when a decision resolves it. | As Exact occurrence decision. |

Checks mirror `migrations/000025_occurrence_lemma_review_flags.up.sql:14-18`.
The German detector writes no `grc` flags, and no `grc` detector is approved, so
no row is written until one is. The Start and Switch gate still reads this
record for classical Books, so a future detector needs no lifecycle change.

#### Rich count readiness

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Whether the Book's rich counts are usable for its current classical analysis. | `owner_id`; `book_id`; `language text`; `analysis_id uuid`; `state text` ∈ {`pending`, `ready`, `unavailable`}; `builder_version integer` (> 0, starting at 1, independent of the legacy builder's 3); `coverage_denominator bigint NULL` (EFF-3; non-NULL ⇔ `ready`); `reason text NULL` (required ⇔ `unavailable`); `updated_at` | `(owner_id, book_id)` | `(owner_id, book_id, analysis_id)` unique, as the count rows' reference target. | Owner. | **Mutable** by adoption (§11), decisions and the rebuild job only, as listed under [Count projection](#count-projection). | `(owner_id, book_id)` → §4 association `ON DELETE CASCADE`; `analysis_id` → Exact analysis `RESTRICT` |

#### Rich vocabulary count

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Effective occurrence count of one rich identity in one Book's analysis. | `owner_id`; `book_id`; `analysis_id`; `identity_id`; `occurrence_count bigint` (> 0); `corrected boolean` (some contributing occurrence is corrected) | `(owner_id, book_id, analysis_id, identity_id)` | Key only. | Owner. | **Mutable** by the same writers as readiness. Derived data: never retained evidence. | `(owner_id, book_id, analysis_id)` → Rich count readiness `ON DELETE CASCADE`; `identity_id` → Vocabulary identity `RESTRICT` |

Index `(owner_id, identity_id, book_id)` serves the library total.

#### Snapshot representation (additive to `primary_goal_snapshots`)

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Lets the existing snapshot row anchor a rich reading. | new column `exact_analysis_id uuid NULL`; the five legacy evidence columns become nullable (OD-32) | existing `(owner_id, id)` | unchanged | unchanged | `exact_analysis_id` is set at insert and never changes. `released_at` keeps its one write. | `exact_analysis_id` → Exact analysis `RESTRICT` |

A check makes the representations exclusive and exhaustive: either all five
legacy evidence columns are non-NULL and `exact_analysis_id` is NULL
(**legacy**), or all five are NULL and `exact_analysis_id` is non-NULL
(**rich**). Every existing row is legacy, because the five columns are `NOT
NULL` today, so no row is rewritten. A trigger on insert requires a rich
snapshot's `exact_analysis_id` to be the Book's current classical analysis.

#### Rich snapshot membership

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One frozen rich identity of a rich Current-reading vocabulary snapshot, with its count context and exact evidence. | `owner_id`; `snapshot_id`; `identity_id`; `language text`; `analysis_id uuid` (equals the snapshot's `exact_analysis_id`); `headword text` (frozen display headword, §6, OD-35); `occurrence_count integer` (> 0; in-Book effective count); `library_total bigint NULL` (the total used; NULL when the in-Book route qualified and no total was read); `route text` ∈ {`in_book`, `library_total`}; `occurrence_node_ids integer[]` (the effective occurrences, ascending; cardinality = `occurrence_count`); `observed_forms jsonb`; `selected_at` | `(owner_id, snapshot_id, identity_id)` | Key only: an identity appears once per snapshot, and `ex:1` and `ex:2` are two rows. | Owner. | **Immutable.** Inserted only in the transaction that inserts its snapshot. | `(owner_id, snapshot_id)` → `primary_goal_snapshots` `ON DELETE CASCADE` (as `migrations/000007_primary_goal_snapshots.up.sql:49`); `identity_id` → Vocabulary identity `RESTRICT`; `analysis_id` → Exact analysis `RESTRICT` |

Checks: `route = in_book` ⇔ `occurrence_count ≥ 3` and `library_total IS NULL`;
`route = library_total` ⇔ `1 ≤ occurrence_count < 3` and `library_total ≥ 15`.
A trigger rejects a row for a legacy snapshot, an `analysis_id` other than the
snapshot's, a `language` other than the snapshot's or the identity's, and an
insert for a snapshot created in an earlier transaction. An additive trigger on
`primary_goal_snapshot_vocabulary` rejects a tuple row for a rich snapshot.

#### Snapshot count contribution

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Which Books' analyses made up the library totals a rich snapshot used. | `owner_id`; `snapshot_id`; `book_id`; `analysis_id`; `builder_version` | `(owner_id, snapshot_id, book_id)` | Key only. | Owner. | **Immutable.** Written in the snapshot's transaction, one row per contributing Book, only when at least one identity needed a library total. | `(owner_id, snapshot_id)` → snapshot `ON DELETE CASCADE`; `(owner_id, book_id)` → `books` `ON DELETE CASCADE`; `analysis_id` → Exact analysis `RESTRICT` |

#### Rich Known vocabulary

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Modeled Known membership of one rich identity, with the completion that accepted it. | `owner_id`; `language text` (equals the identity's language); `identity_id`; `headword text` (copied from the accepted membership row); `completion_id uuid`; `snapshot_id uuid`; `book_id uuid`; `analysis_id uuid`; `accepted_at timestamptz` | `(owner_id, identity_id)` | Key only: set semantics. A second acceptance of the same identity inserts nothing and keeps the first provenance. | Owner, per language through the identity. `grc` and `el` identities are different rows. | **Immutable.** Inserted only by a rich Finish. There is no update or delete path, as for legacy Known. | `owner_id` → `users` `ON DELETE CASCADE`; `completion_id` → `reading_history(completion_id)` `NO ACTION`; `(owner_id, snapshot_id, identity_id)` → Rich snapshot membership `NO ACTION`; `identity_id` → Vocabulary identity `RESTRICT` |

The membership reference makes "Known only from a completed reading's frozen
snapshot" a database fact. `NO ACTION` lets the owner cascade delete both rows
in one statement. Rich Known has no Generated or deck provenance column: Finish
never reads deck state, and Generated provenance for rich identities belongs to
#2063.

```sql
-- illustrative, not a migration
ALTER TABLE primary_goal_snapshots
  ALTER COLUMN source_material_id DROP NOT NULL,
  ALTER COLUMN analysis_run_id DROP NOT NULL,
  ALTER COLUMN content_revision_id DROP NOT NULL,
  ALTER COLUMN content_snapshot_id DROP NOT NULL,
  ALTER COLUMN corpus_id DROP NOT NULL,
  ADD COLUMN exact_analysis_id uuid NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  ADD CONSTRAINT primary_goal_snapshots_one_representation CHECK (
    (exact_analysis_id IS NULL AND source_material_id IS NOT NULL AND analysis_run_id IS NOT NULL
     AND content_revision_id IS NOT NULL AND content_snapshot_id IS NOT NULL AND corpus_id IS NOT NULL)
    OR
    (exact_analysis_id IS NOT NULL AND source_material_id IS NULL AND analysis_run_id IS NULL
     AND content_revision_id IS NULL AND content_snapshot_id IS NULL AND corpus_id IS NULL));

CREATE TABLE exact_occurrence_decisions (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  analysis_id uuid NOT NULL,
  node_id integer NOT NULL,
  decision text NOT NULL CHECK (decision IN ('correct','exclude')),
  identity_id uuid NULL REFERENCES vocabulary_identities(id) ON DELETE RESTRICT,
  corrected_citation_form text NULL,
  normalization_profile text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id, analysis_id, node_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES book_reading_units(owner_id, book_id) ON DELETE CASCADE,
  FOREIGN KEY (analysis_id, node_id) REFERENCES annotation_occurrences(analysis_id, node_id) ON DELETE RESTRICT,
  CHECK ((decision = 'correct') = (identity_id IS NOT NULL)),
  CHECK ((decision = 'correct') = (corrected_citation_form IS NOT NULL)),
  CHECK ((decision = 'correct') = (normalization_profile IS NOT NULL))
);

CREATE TABLE rich_count_readiness (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  language text NOT NULL,
  analysis_id uuid NOT NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  state text NOT NULL CHECK (state IN ('pending','ready','unavailable')),
  builder_version integer NOT NULL CHECK (builder_version > 0),
  coverage_denominator bigint NULL CHECK (coverage_denominator >= 0),
  reason text NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id),
  UNIQUE (owner_id, book_id, analysis_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES book_reading_units(owner_id, book_id) ON DELETE CASCADE,
  CHECK ((state = 'ready') = (coverage_denominator IS NOT NULL)),
  CHECK ((state = 'unavailable') = (reason IS NOT NULL))
);

CREATE TABLE rich_vocabulary_counts (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  analysis_id uuid NOT NULL,
  identity_id uuid NOT NULL REFERENCES vocabulary_identities(id) ON DELETE RESTRICT,
  occurrence_count bigint NOT NULL CHECK (occurrence_count > 0),
  corrected boolean NOT NULL DEFAULT false,
  PRIMARY KEY (owner_id, book_id, analysis_id, identity_id),
  FOREIGN KEY (owner_id, book_id, analysis_id)
    REFERENCES rich_count_readiness(owner_id, book_id, analysis_id) ON DELETE CASCADE
);
CREATE INDEX ON rich_vocabulary_counts (owner_id, identity_id, book_id);

CREATE TABLE rich_snapshot_vocabulary (
  owner_id uuid NOT NULL,
  snapshot_id uuid NOT NULL,
  identity_id uuid NOT NULL REFERENCES vocabulary_identities(id) ON DELETE RESTRICT,
  language text NOT NULL,
  analysis_id uuid NOT NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  headword text NOT NULL CHECK (headword <> ''),
  occurrence_count integer NOT NULL CHECK (occurrence_count > 0),
  library_total bigint NULL,
  route text NOT NULL CHECK (route IN ('in_book','library_total')),
  occurrence_node_ids integer[] NOT NULL CHECK (cardinality(occurrence_node_ids) = occurrence_count),
  observed_forms jsonb NOT NULL,
  selected_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, snapshot_id, identity_id),
  FOREIGN KEY (owner_id, snapshot_id) REFERENCES primary_goal_snapshots(owner_id, id) ON DELETE CASCADE,
  CHECK ((route = 'in_book' AND occurrence_count >= 3 AND library_total IS NULL)
      OR (route = 'library_total' AND occurrence_count BETWEEN 1 AND 2 AND library_total >= 15))
);

CREATE TABLE rich_known_vocabulary (
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  language text NOT NULL,
  identity_id uuid NOT NULL REFERENCES vocabulary_identities(id) ON DELETE RESTRICT,
  headword text NOT NULL,
  completion_id uuid NOT NULL REFERENCES reading_history(completion_id),
  snapshot_id uuid NOT NULL,
  book_id uuid NOT NULL,
  analysis_id uuid NOT NULL,
  accepted_at timestamptz NOT NULL,
  PRIMARY KEY (owner_id, identity_id),
  FOREIGN KEY (owner_id, snapshot_id, identity_id)
    REFERENCES rich_snapshot_vocabulary(owner_id, snapshot_id, identity_id)
);
-- plus: exact Lemma review flags (as 000025, keyed like the decisions); snapshot
-- count contributions; the membership, decision and representation triggers
-- above; mirrored update-rejecting triggers on the immutable records; and an
-- insert trigger on primary_goal_snapshot_vocabulary rejecting rich snapshots.
```

### Representation selection

Code chooses the representation from the record it holds, never from the
language, the producer or a package:

| record | legacy when | rich when |
| --- | --- | --- |
| Book (counts, decisions, flags, Start target) | it has a `book_current_analyses` row | it has a §5 Book current classical analysis row. §5's exclusion trigger forbids both. A Book with neither has no analysis and is not a Start target. |
| Snapshot | `exact_analysis_id IS NULL` | `exact_analysis_id IS NOT NULL` (exclusive by the check above) |
| Current reading | its snapshot is legacy, or `snapshot_id` is NULL (the legacy no-snapshot shape, `internal/persistence/current_reading.go:236-238`) | its snapshot is rich. A rich reading never has a NULL `snapshot_id`. |
| Completion | its snapshot is legacy or absent | its snapshot is rich |
| Known check for a candidate | the candidate is a legacy tuple: only `known_vocabulary`, with today's blank-UPOS wildcard | the candidate is a rich identity: Rich Known, plus `known_vocabulary` rows that equal it under §6 "Identity comparison" (null discriminator, non-blank UPOS, not `grc`) |

Start and Switch choose at freeze time from the target Book's anchor. Every
later read (Reserved, the Finish preview at
`internal/persistence/current_reading_lifecycle.go:418-431`, Finish, snapshot
listing) dispatches on the snapshot's representation. A legacy selection never
reads Rich Known (OD-33), so no legacy outcome can change.

No consumer tests the language or producer to choose a path. The only language
test is inside §6's shared comparison function (rule 5, OD-24), which every
legacy-to-rich comparison calls.

### Count projection

Per (owner, Book, analysis), the rich projection holds one count row per
effective identity (EFF-2) and one readiness row. It parallels migration
000031; nothing in `vocabulary_browse_counts` or its readiness changes, and the
legacy builder never reads classical Books (it reads `current_analysis_identity`,
which a classical Book never appears in).

Writers, mirroring `internal/persistence/vocabulary_browse_counts.go:299-351` and
`internal/persistence/lemma_review.go:216-248`:

| event | writes |
| --- | --- |
| §11 adoption of analysis Y for Book b | In the adoption transaction: delete b's count rows, then set readiness to `(Y, ready, denominator, counts)` when the build fits the transaction, otherwise `(Y, pending)` and enqueue a rebuild job. The build applies b's decisions on Y only (EFF-4), so a fresh adoption has none. |
| Confirmed exact decision on b's current analysis, readiness `ready` | In the decision transaction, under the per-Book lock: recompute only the old and new effective identities of each changed occurrence (ADR 0087 delta), upsert or delete their rows. Readiness stays `ready`; the denominator does not change (EFF-3). |
| Confirmed exact decision, readiness not `ready` | Set readiness to `pending` and enqueue a rebuild. |
| Rebuild job succeeds | Under the per-Book lock, if readiness still names the current analysis and the decision revision is unchanged (as `:334-343`), replace count rows and set `ready`. Otherwise write nothing; a newer event owns readiness. |
| Rebuild job exhausts its attempts | Set `unavailable` with the failure reason, if readiness still names the same analysis (OD-34). |
| Owner deletion | Cascades. |

Readers use count rows only when readiness is `ready`, names the Book's current
classical analysis and has the current builder version. Book-scoped consumers
(Browse, My Books coverage, Reading coverage and chooser bands, lemma review
impact) read the rich projection for classical Books exactly as they read the
legacy one for EPUB Books (ADR 0087 item 2), and show Updating or Unavailable
for any other readiness.

**Contributing Books** for owner `o` and language `L` (ADR 0090, ADR 0095):

- Exactly the Books of `o` with `language_state = chosen`, `language_tag = L`,
  and a Book current classical analysis that is Vocabulary-ready and carries no
  Usability restriction naming the vocabulary capability (§5). Disposition and
  visibility do not matter: Inbox, To Read, Read, Hidden and the current
  reading's own Book all contribute.
- Discovery (offerings without a Book or without a current analysis), whole-work
  bytes with no binding for the unit, other owners' Books, EPUB Books and legacy
  `grc` analysis-run Books contribute nothing to rich totals and do not affect
  their readiness. Legacy `grc` tuples match no rich identity (§6 rule 5).
- Likewise, classical Books never contribute to legacy totals.

**Library total** of rich identity `i` = Σ `occurrence_count` of `i` over the
contributing Books, including the Book being frozen. It is read in one
statement snapshot together with readiness, as at
`internal/persistence/current_reading_lifecycle.go:189-234`. If any contributing
Book's readiness is not `ready` for its current analysis, no total is returned:
the error is unavailable when any such Book is `unavailable`, otherwise pending
(`:253-258`). A pending or unavailable Book is never counted as zero.

### The 3/15 rule on rich identities

A rich identity `i` with in-Book effective count `n` (EFF-2, computed in the
freeze transaction from the analysis and the Book's decisions) enters a new rich
snapshot exactly when:

1. `i` is not Known for the owner (Rich Known, or a legacy row equal under §6),
2. `i` is not Reserved (a member of an unreleased rich snapshot of the owner in
   `L`, or equal under §6 to a tuple of an unreleased legacy snapshot), and
3. `n ≥ 3`, or `n ≥ 1` and the library total of `i` is at least 15.

`n = 0` never qualifies. The library total is read only when some identity
satisfies 1 and 2 with `1 ≤ n < 3`; if none does, other Books' readiness does
not block the freeze (ADR 0085). The rule is `selection`'s
`readingFrequencyEligible` (`internal/selection/eligibility.go:52-54`), applied
to a rich identity key; thresholds are not duplicated.

### Lifecycle transitions

Existing behaviour, restated, and what each transition additionally writes for a
rich representation. No transition, verdict, lock or learner state is added.
Each transition keeps today's locks in today's order. Start, Switch, End, Finish
and lemma decisions take the learner-state lock
(`internal/persistence/lemma_review.go:256-259`); disposition and visibility
take only their row locks.

| transition | existing behaviour (path:line) | reused | additionally for rich |
| --- | --- | --- | --- |
| **Start** | Locks (`internal/persistence/current_reading_lifecycle.go:511-516`); loads facts including unresolved flags (`:606-634`); `DecideStart` (`internal/domain/current_reading_start.go:58-76`); freezes the snapshot with Known, Reserved and 3/15 (`internal/persistence/current_reading_lifecycle.go:46-90`, `:100-154`); fails on pending or unavailable totals (`:253-258`); inserts the current row (`:677-686`). | Locks, `DecideStart`, the current-row insert and the `beforeCommit` hook (`:559-583`). | Flags read from Exact Lemma review flags on the current analysis. The snapshot row is rich. Membership rows and, when totals were read, contribution rows are inserted in the same transaction. No tuple row is written. |
| **Switch** | `internal/persistence/current_reading.go:32-62`; `DecideSwitchCurrentReading` (`internal/domain/current_reading_transition.go:120-143`); releases the old snapshot (`internal/persistence/current_reading.go:93`), freezes the target (`:96`), repoints the current row (`:100`). | All of it, including replay proof by timestamps (`internal/domain/current_reading_transition.go:150-159`). | The target's freeze is rich or legacy by the target Book. Switching between a legacy and a rich Book in one language is allowed and writes one representation per snapshot. |
| **End** | `internal/persistence/current_reading.go:175-213`; `DecideEndCurrentReading` (`internal/domain/current_reading_transition.go:83-97`); releases the snapshot (`internal/persistence/current_reading.go:206`) and deletes the current row (`:209`). Disposition, snapshot and history stay. | All of it. | Nothing. Releasing the snapshot ends its rich reservation. |
| **Finish** | `internal/persistence/current_reading.go:234-278`; `PlanCurrentReadingFinish` (`internal/domain/current_reading_finish.go:86-108`); records the completion and Known (`internal/persistence/current_reading.go:322-343`); returns the Book to Inbox, releases and deletes (`:347-355`); a replay still ends an active reading (`:269-273`). | The plan, the completion row (`reading_history`, idempotent per snapshot), Inbox, release and delete. | Facts: the snapshot's rich identities and the Known that matches them. The plan's counts fill the same `reading_history` columns. Accepted identities are inserted into Rich Known with set semantics, and the inserted count must equal the plan, as at `:333-341`. Deck and Generated state are never read, so Finish completes whatever the card state. |
| **Disposition change** | `internal/persistence/book_dispositions.go:54-91` | All of it. | Nothing. Contribution does not depend on disposition. |
| **Visibility** | `internal/persistence/book_visibility.go:40-66` | All of it. | Nothing. |
| **Previously-read import** | `internal/persistence/current_reading_lifecycle.go:385-400` | All of it. | Nothing: it writes a completion with no snapshot and no Known. |
| **Language change ending a reading** | `internal/persistence/books.go:839-845`, `:868-880` | All of it. | Nothing: release ends the rich reservation. |
| **Lemma decision** | `internal/persistence/lemma_review.go:187-214`: locks, rejects the current reading's Book (`:265-274`), checks the fingerprint, writes decisions, resolves flags (`:250-254`), updates counts (`:241-246`). | Locks, current-reading rejection, fingerprint check. | Writes Exact occurrence decisions and resolves exact flags. The fingerprint covers the exact decisions and the current analysis ID, so a preview made before an adoption is stale. Counts follow [Count projection](#count-projection). |
| **Flag save** | `internal/persistence/lemma_review.go:19-30`, `:63-81` | Insert-if-absent. | Keyed by exact occurrence, on the current analysis only. |
| **Deck preparation** | Admission from the current snapshot (ADR 0085). | Unchanged in §8. | Nothing in §8. Card records are #2063. Preparing a deck never writes Known (ADR 0036). |
| **Evidence adoption** (§11; not a learner transition) | — | — | Count and readiness writes above. It never changes a snapshot, membership, decision, Known or completion. |

### Store-contract extension

The existing scenarios keep `const Language = "de"`
(`internal/storecontract/harness.go:17-18`), their seeds and their bodies.
`Scenarios()` (`:124-132`) still returns exactly them. The rich scenarios are a
second list, `richScenarios(v Variant)`, run by `Run` for `Variant{Language:
"grc"}` after the existing ones. Rich scenarios seed through a new
`RichSeeder` returned by the harness. Its seeds carry rich identities (lemma,
UPOS, discriminator), occurrences with exact keys, spans and node class, per-Book
readiness state, and rich Known. The harness shape is §10's to settle (OD-04).

Named scenarios:

| scenario | covers |
| --- | --- |
| `rich: lifecycle start, switch, end and finish on a classical Book` | Start freezes rich membership; Switch to a second classical Book; End; Finish writes completion and Rich Known. Same verdicts as the `de` lifecycle scenarios (LRN-19). |
| `rich: discriminated identities stay distinct across Known, Reserved, snapshot and finish` | LRN-01 |
| `rich: same-span occurrences hold independent decisions` | LRN-02 |
| `rich: a decision does not transfer to a new analysis` | LRN-03 |
| `rich: an exclusion keeps the coverage denominator` | LRN-04 |
| `rich: pending or unavailable counts block only below-floor selection` | LRN-05 |
| `rich: the 3/15 boundaries` | LRN-07 to LRN-09 |
| `rich: contributing Books` | LRN-21 |
| `rich: grc is isolated from el, and owners are isolated` | LRN-11, LRN-12 |
| `rich: a frozen snapshot is unchanged by later counts, analyses and corrections` | LRN-13 |
| `rich: stale requests change nothing` | LRN-19 |
| `rich: counts follow the current analysis and stay exact after decisions` | LRN-26 |
| `rich: the current reading refuses decisions on its Book` | LRN-20 |
| `rich: exact flags gate Start and are never reopened` | LRN-22 |
| `rich: disposition, visibility and previously-read import write no rich state` | LRN-23 |

**Fixture adapter** (`internal/fixtures`, plain `go test`):

- It stores rich identities as five-field values and compares them with the §6
  comparison function (I-8). A NULL discriminator is a distinct "absent" value,
  never the empty string. It does not intern.
- It stores exact occurrences as `(analysisID, nodeID)` with their spans,
  allowing equal spans, and decisions and flags in maps keyed by owner, Book and
  that pair. It never looks an occurrence up by span.
- It computes in-Book counts, effective identities and the coverage denominator
  by calling the same `selection` projection over its seeded occurrences, and
  the 3/15 decision through the same eligibility function. It does not seed
  frozen vocabulary directly for rich Books (unlike the tuple map at
  `internal/fixtures/storecontract_test.go:62-66`).
- It holds readiness per (owner, Book) as one of `pending`, `ready`,
  `unavailable`, settable by the seeder, and returns the same pending and
  unavailable errors as PostgreSQL.
- It marks each snapshot legacy or rich, keeps Rich Known apart from its legacy
  Known slice, and uses the same `domain` decision functions as today
  (`internal/fixtures/reading_lifecycle.go:297-334`).

The PostgreSQL adapter seeds classical Books through the §11 adoption interface
with synthetic analyses, not by inserting rows behind it.

### Invariants

Each invariant is covered by the `LRN` rows in [§15](#15-acceptance-test-matrix).

1. **Identity.** Discriminated identities stay distinct in counts, Known,
   Reserved, snapshot membership and Finish (LRN-01). `grc` and `el` are
   isolated, and so are owners (LRN-11, LRN-12).
2. **Decisions.**
   - At most one decision per (owner, Book, exact occurrence); a correction's
     identity has the occurrence's UPOS and a NULL discriminator; decisions name
     only `vocabulary` occurrences of a classical Book (LRN-02, LRN-17).
   - Same-span occurrences hold independent decisions and flags (LRN-02,
     LRN-22).
   - A decision never transfers to another analysis, and is written only on the
     current analysis (LRN-03, LRN-18).
   - An exclusion keeps the coverage denominator (LRN-04).
   - The current reading's Book refuses decisions (LRN-20).
3. **Counts.**
   - Pending or unavailable counts block a freeze that needs a library total and
     are never read as zero (LRN-05, LRN-06).
   - Only contributing Books add to totals (LRN-21); disposition and visibility
     do not change contribution (LRN-23).
   - Counts are used only for the Book's current analysis and stay exact after a
     decision (LRN-26).
4. **Selection.** The 3/15 boundaries hold for rich identities, and Known and
   Reserved are excluded (LRN-07, LRN-08, LRN-09, LRN-10).
5. **Snapshots.**
   - A snapshot is exactly one of legacy or rich, and its membership is
     immutable and tied to its analysis (LRN-16).
   - A frozen snapshot is unchanged by later counts, analyses and corrections
     (LRN-13).
6. **Known.** Rich Known is written only by Finish from frozen membership, with
   set semantics, and never by deck preparation (LRN-15, LRN-24).
7. **Lifecycle.** No new state: rich readings follow the same transitions,
   verdicts and stale-request rules (LRN-19); Finish completes without deck
   state (LRN-15).
8. **Legacy.** Legacy `de` snapshots, Known and history behave identically and
   their rows are not rewritten (LRN-14). Rich identities never reach the
   Browse-selection or Custom-deck stores (LRN-25, OD-01).

### Conflicts and dependencies noted

- **OD-32.** The existing snapshot row cannot anchor a classical reading
  without relaxing five `NOT NULL` columns on a shipped table
  (`migrations/000007_primary_goal_snapshots.up.sql:19-23`). §5 avoided the
  same kind of change for `book_current_analyses` (OD-12). Here the alternative
  is a second reading or snapshot anchor, which ADR 0098 forbids. §8 is written
  against OD-32 option (1).
- The decision, readiness and count records reference §4's Book–Reading unit
  association and §5's Exact analysis. The rich-snapshot insert trigger reads
  §5's Book current classical analysis.
- How a Reading gate recognizes a classical Book (B-25, B-26) is OD-05, owned
  by §11. §8 assumes Start's eligibility classifier reports a classical Book
  with a current classical analysis as eligible.
- OD-04 is #2071's. §8 keeps every `de` scenario unchanged and adds a rich list
  beside it. If #2071 parameterizes the existing scenarios instead, the rich
  scenario names above still apply.
- The retention of decisions, snapshot membership, contributions and Rich Known
  on older analyses is §9's.
- Card records for rich identities (Generated vocabulary, deck preparation
  vocabulary and deck specifications, B-14 and B-15) are #2063's. §8 requires
  only that Finish reads none of them and that none writes Known.
- The fingerprint of a rich lemma-review preview includes the current analysis
  ID. The preview interface is §10's.

## 9. Retention references

_Pending: owned by the retention references ticket._

## 10. Module interfaces and validation

_Pending: owned by the module interfaces ticket._

## 11. Durable preparation and atomic adoption

_Pending: owned by the durable preparation and adoption ticket._

## 12. Operator-local entry point

_Pending: owned by the operator-local entry point ticket._

## 13. Migration sequence, preflight and recovery

_Pending: owned by the migration sequence ticket._

## 14. Public and local material

_Pending: owned by the public and local material ticket._

## 15. Acceptance-test matrix

Every invariant in §4–§14 has at least one row here, under its owner's ID
prefix. Rows assert observable behaviour and persisted invariants. They do not
assert proposed table names, call order or private helper layout. Public CI
rows use synthetic fixtures only.

### Columns

| column | meaning |
| --- | --- |
| ID | `<PREFIX>-<nn>`, numbered from `01` within each prefix, for example `IDN-03`. IDs are never reused. |
| invariant | The testable statement, worded as in its owning section. |
| seam | One of (a)–(d) below. |
| setup | The persisted and learner state before the action, using synthetic data. |
| action | The single operation under test, through the seam's public interface. |
| expected | The observable result and the persisted invariant after the action. |
| negative case | The input or state that must be rejected or left unchanged, and how that is observed. |

### Seams

- **(a)** A web-handler integration test with real PostgreSQL and River job execution.
- **(b)** A shared store-contract scenario, run against the fixture and PostgreSQL adapters.
- **(c)** The shared canonical-validation interface.
- **(d)** A PostgreSQL persistence integration test, only for constraints that (a) and (b) cannot reach.

### ID prefixes

| prefix | section | owner ticket |
| --- | --- | --- |
| `BIB` | §4 Bibliographic identity and offerings | #2065 |
| `EVD` | §5 Immutable evidence, bindings and interpretations | #2066 |
| `IDN` | §6 Vocabulary identity and normalization | #2067 |
| `OCC` | §7 Canonical analysis and exact occurrences | #2068 |
| `LRN` | §8 Learner-state attachment and compatibility | #2069 |
| `RET` | §9 Retention references | #2070 |
| `VAL` | §10 Module interfaces and validation | #2071 |
| `ADP` | §11 Durable preparation and atomic adoption | #2072 |
| `LOC` | §12 Operator-local entry point | #2073 |
| `MIG` | §13 Migration sequence, preflight and recovery | #2074 |
| `PUB` | §14 Public and local material | #2075 |

### Rows

| ID | invariant | seam | setup | action | expected | negative case |
| --- | --- | --- | --- | --- | --- | --- |
| BIB-01 | Repeating acquisition for the same Edition and unit returns the same Book. | (b) | Owner A, reviewed definition `example-def` for `example-work / ed-1 / book 1`. A has registered once, then moved the Book to To Read. | Register `example-def` for A again. | The same Book ID is returned. A has one Book for the unit, one association and one Local registration. The disposition is still To Read. | A third registration after a new pin version of `example-def` still returns the same Book and adds no Local registration. |
| BIB-02 | At most one Book per (owner, unit), even under concurrent acquisition. | (d) | Owner A, no Book for the unit. | Two transactions ensure a Book for the same unit concurrently. | Both return the same Book ID. A has exactly one Book for the unit. | See BIB-12 for a direct second Book on the same unit. |
| BIB-03 | Two owners with the same unit get two Books, each invisible to the other. | (a) | Owners A and B. Both register `example-def`. | B requests My Books, then requests A's Book detail by ID. | B's My Books lists only B's Book. A's Book detail returns the existing not-found response. The two Book IDs differ and share only the shared unit. | B's To Read on B's Book leaves A's disposition unchanged. |
| BIB-04 | Two Editions of one Work give two Books with independent state. Work grouping is display only. | (a) | Shared `example-work` with named Editions `ed-1` and `ed-2`, each with `book 1`. Owner A has both Books. | A moves the `ed-1` Book to To Read. | My Books groups both under `example-work`. Only the `ed-1` Book is To Read; the `ed-2` Book stays Inbox. | Hiding the `ed-2` Book does not hide the `ed-1` Book. |
| BIB-05 | Overlapping units give two Books with no shared disposition or completion. | (a) | Edition `ed-1` with units `book 1` and `books 1–2`. Owner A has both Books. The `book 1` Book has a Vocabulary-ready analysis. | A starts and finishes Reading the `book 1` Book. | The `book 1` Book has a Reading completion. The `books 1–2` Book has no history and keeps its disposition. | Ensuring a Book for `books 1–2` returns the existing `books 1–2` Book, not the `book 1` Book. |
| BIB-06 | An unidentified Edition is never merged with a named one by title. | (b) | Shared `example-work` with named Edition `ed-1` `book 1`, and an unidentified Edition of the same Work with `book 1` backed by pinned evidence. Owner A. | Ensure A's Book for each unit. | Two Books. The unidentified one displays as unidentified and never shows `ed-1`'s label. | Giving both Books the same title and author still yields two Books. |
| BIB-07 | Verified reconciliation keeps Book IDs and history. | (b) | Owner A has a To Read Book with history on the unidentified Edition's `book 1`. | Record a reconciliation of the unidentified Edition as `ed-1`. Then ensure A's Book for `ed-1` `book 1`. | The original Book ID is returned. The store still reports its original unit for it. Its disposition and history are unchanged. No Edition or unit ID is changed, and A has no new Book. | A reconciliation from sync input, without a reviewed definition, is rejected and writes nothing. |
| BIB-08 | Reconciliation never merges two existing Books. | (b) | Owner A has a Book on each side of the later reconciliation. | Record the reconciliation, then ensure A's Book for `ed-1` `book 1`. | Both Books remain. The call returns the `ed-1` Book. One `duplicate_after_reconciliation` conflict is recorded. | Repeating the call records no second conflict. |
| BIB-09 | Reading unit bounds never change. | (d) | A shared unit `constituent_book:example-section:1`. | Update its kind, scheme, start or end. | Each update is rejected. The unit row is unchanged. | Inserting a second unit with the same Edition and extent is rejected (see BIB-11). |
| BIB-10 | Annotation coverage and source revisions never change a unit or its Book. | (a) | Owner A has a To Read Book for `book 1`. Synthetic annotation evidence covers only part of `book 1`. | Prepare the Book with the partial evidence, then with a later source revision. | The displayed bounds stay `book 1`. The Book ID and association are unchanged. Preparation reports incomplete coverage (§10, §11). | No smaller unit is created and no other Book is offered from the partial coverage. |
| BIB-11 | At most one Reading unit per (Edition, extent), and every Edition belongs to one Work. | (d) | Edition `ed-1`. | Insert two units with equal `extent_key`. Insert an Edition with no Work. | The second unit and the Work-less Edition are rejected. | Units `constituent_book:example-section:1` and `bounded_passage:example-section:1:2` coexist. |
| BIB-12 | At most one Book per (owner, Reading unit). | (d) | Owner A has a Book for `book 1`. | Insert an association of a second A Book to the same unit. | Rejected. | An association for owner B to the same unit succeeds. |
| BIB-13 | A classical Book references exactly one unit, and the reference never changes. | (d) | Owner A has a Book for `book 1`. | Update the association to `books 1–2`. Insert a second association for the same Book. | Both are rejected. | Deleting owner A removes the Book and its association by cascade. |
| BIB-14 | An alias collision across repositories without verification gives two Books and a recorded conflict. | (b) | Owner A has connections to two synthetic repositories. Each has one entry claiming `cts:urn:example:ed-1` for a unit `book 1`. Neither is verified. | Resolve both offerings. | Two Books, each Inbox. An Identity conflict records the shared claimed alias on both Books. | Repeating the sync records no new conflict and no new Book. |
| BIB-15 | Verified aliases from two repositories resolve to one Book only for the same Edition and unit. | (b) | Reviewed mappings verify `example-repo-1:x` and `example-repo-2:y` as shared `ed-1`. Owner A has both connections. | Resolve offerings for `book 1` from both, and then for `books 1–2` from the second. | Both `book 1` offerings point to one Book. `books 1–2` is a separate Book. | A `book 1` offering whose verified Edition is `ed-2` gives a separate Book. |
| BIB-16 | A verified alias is unique per scheme and value. | (d) | A verified alias `cts:urn:example:ed-1` names `ed-1`. | Insert the same scheme and value naming `ed-2`. | Rejected. | The same value under another scheme is accepted. |
| BIB-17 | Titles, authors and identical bytes never merge Books. | (b) | Owner A has two connections advertising unverified entries with identical title and author. Synthetic pinned evidence for both has an identical digest. | Resolve both offerings. | Two private Editions and two Books. | The second offering is not resolved to the first's Edition or Book. |
| BIB-18 | One entry offering two units gives two Books. | (a) | A synthetic classical connection whose entry declares `book 1` and `books 1–2`. | Run catalogue sync. | Two Inbox Books, listed in My Books with their Edition and bounds. | No Book alias is created for the entry (see BIB-21). |
| BIB-19 | Repeat sync and withdrawal keep Books and dispositions. | (a) | After BIB-18, A has moved `book 1` to To Read. | Sync again with changed advertised titles, then sync without the entry. | No new Book. The disposition and the Books' title, author and language are unchanged; the new titles appear only as advertised offering metadata. The offerings are marked withdrawn. Both Books stay in My Books. | Removing the connection deletes its offerings. Both Books and their associations remain. |
| BIB-20 | An existing EPUB Book behaves identically, and no association rows are created. | (a) | An existing OPDS connection, and an EPUB Book created before classical support. | OPDS sync, To Read acquisition, analysis, then Reading. | Responses match the modern-language baseline. Neither Book has a Reading unit association, and the owner has no Unit offerings or Local registrations. | Re-running OPDS sync does not attach a Reading unit to any Book. |
| BIB-21 | The existing catalogue-entry alias keeps working unchanged for OPDS. | (d) | Owner A has one OPDS Book with a `catalog_entry` alias, and one classical Book from an offering on the same entry identifier. | Insert a second `catalog_entry` alias with the same owner, connection, namespace and value. Look up the alias. | The duplicate is rejected as today. The lookup returns only the OPDS Book. | The classical Book has no `catalog_entry` alias. |
| BIB-22 | The operator-local path creates a Book without a connection. | (b) | Owner A with no catalogue connection. | Register `example-def` for A. | A has one Inbox Book with an association and a Local registration, and no Unit offering. The Work, Edition and unit exist. | An unknown `definition_id` writes no Work, Edition, unit, Book or registration. |
| BIB-23 | A second owner's registration reuses the shared unit and creates that owner's Book. | (b) | After BIB-22, owner B. | Register `example-def` for B. | B has a new Book on the same shared unit. A's rows are unchanged. No second Work, Edition or unit exists. | B's Book ID differs from A's. |
| BIB-24 | A private unit is never associated with another owner's Book. | (d) | Owner A has a private Edition and unit from an unverified offering. | Insert owner B's association to A's private unit. | Rejected. | Owner A's association to the same unit is accepted. |
| BIB-25 | Sync input never changes a shared description or creates a verified alias. | (b) | Shared `ed-1` from a reviewed definition. Owners A and B each have a `book 1` Book. | A's offering for a verified `ed-1` unit advertises a different Edition label, a different Work title and a new claimed CTS identifier. | B sees the reviewed label and title. A's values appear only as A's advertised offering metadata and Claimed alias. No verified alias is added. | Resolving a later offering that claims the same new identifier does not resolve through it. |
| BIB-26 | A Work's identity is its internal ID, not its description. | (b) | Two reviewed definitions describe two Works with identical title, author and language. | Register both for owner A. Then a reviewed definition update changes the first Work's title. | Two Works and two Books exist. After the update, the Work ID, its Editions and A's Book are unchanged. | Neither registration resolves to the other's Work. |
| BIB-27 | Owner deletion removes the owner's private records and leaves shared records and other owners intact. | (d) | Owner A has a Book on a shared unit, a Book on a private unit with an offering and Claimed alias, a Local registration and an Identity conflict. Owner B has a Book on the same shared unit. | Delete user A. | The deletion succeeds. A's Books, associations, offerings, Claimed aliases, conflicts, registrations and private Work, Edition and unit are gone. The shared Work, Edition and unit and all of B's rows remain. | Deleting the shared unit directly while B's Book references it is rejected. |
| BIB-28 | Reconciliations do not self-reference or chain. | (d) | Editions `u-1` (unidentified), `u-2` (unidentified), `ed-1` (named). `u-1` is reconciled to `ed-1`. | Record `u-2` → `u-2`, `u-1` → `ed-1` again, and `u-2` → `u-1`. | All three are rejected, and no row changes. | `u-2` → `ed-1` is accepted. |
| BIB-29 | An operation for one owner never changes or reveals another owner's offering-side rows. | (a) | Owners A and B each have a Book for the same verified unit from their own connections. A's Book has an Identity conflict. | B syncs and withdraws the offering, moves the Book to To Read, and requests My Books and Book detail. | B's responses show no conflict or offering metadata from A. A's offering, conflict, disposition and Book are unchanged. | B's requests carrying A's Book ID return the existing not-found response. |
| BIB-30 | A private Edition or unit is never resolved for another owner. | (b) | Owner A has a private Edition and unit from an unverified offering. | Resolve owner B's offering with an identical advertisement. | B gets B's own private Work, Edition, unit and Book. | B's resolution never returns A's private record IDs. |
| EVD-01 | Package bytes cannot be replaced, or deleted while referenced. | (d) | A synthetic package blob referenced by owner A's acquisition, and an interpretation table blob referenced by an Interpretation configuration. | Update each blob's content, digest or length. Delete each blob. | Every update and both deletes are rejected. Both blobs still verify against their digests. | Deleting an unreferenced synthetic blob is not blocked by any evidence constraint (no purge path is designed; retention is §9). |
| EVD-02 | Identical bytes are stored once, with a distinct acquisition per origin and release. | (b) | Synthetic package `example-pkg` and two synthetic pin sets that accept it, from package origins `example-local` and `example-release-2`. Both pin sets declare the same upstream text and annotation revisions. Owner A. | Import `example-pkg` for A from `example-local`, then from `example-release-2`, then from `example-local` again. | One blob, one Package content and one set of revisions. A has exactly two acquisitions, each reporting its own origin and release. | The third import adds no row. A different byte sequence with equal size is stored as a separate blob. |
| EVD-03 | A package whose inventory differs from the reviewed pins is rejected, and nothing is written. | (a) | Owner A. Synthetic pin set for `example-pkg`. Variants: one unlisted extra member, one missing member, one member with a changed byte, one with the right digest but a pinned size that differs, and one duplicate entry name. | Run the local import for each variant through its handler and River job. | Each request ends failed, with an inventory reason and no automatic retry. No blob, content, member, revision, notice, acquisition, binding or analysis row exists. A has no current classical analysis. | The exact `example-pkg` imports successfully under the same pin set. |
| EVD-04 | The compressed-size and member-count bounds hold before any write. | (a) | Owner A. Synthetic packages: one of 16 MiB + 1 byte, and one with 129 empty members, each pinned in a test pin set. | Run the local import for each. | Each fails with `package_too_large` or `package_too_many_members`. The request records it, and no evidence row is written. | A package of exactly 16 MiB with 128 members, all pinned, passes these two checks. |
| EVD-05 | The expanded-size bound holds against dishonest metadata, and unsafe entries are rejected. | (a) | Owner A. Synthetic packages: one whose central directory understates sizes and that expands past 64 MiB, one with a `../x` entry, one with an absolute path, and one with a symlink entry. | Run the local import for each. | Each fails with `package_expanded_too_large` or `package_entry_unsafe`. The reader stops at the cap, and no evidence row is written. | A pinned package expanding to exactly 64 MiB passes this check. |
| EVD-06 | Transport identity and source-content identity are separate. | (b) | Two synthetic ZIP builds of the same members with different compression. A pin set version lists both transport digests. Owner A. | Import build 1, then build 2. | Two blobs and one Package content. Both acquisitions resolve to the same revisions. Preparing the same unit and interpretation from either gives the same binding and Exact analysis. | A third build that is not listed in the pin set is rejected, even though its members match. |
| EVD-07 | Pins come from reviewed definitions, never from the package. | (a) | Owner A. A synthetic package whose own manifest member lists one extra member that is present in the ZIP, and whose manifest bytes differ from the pinned manifest. | Run the local import. | It fails with an inventory mismatch on both the manifest member and the extra member. Nothing is written. | Changing only the package's manifest to list exactly the pinned members still fails, because the manifest member's digest differs from its pin. |
| EVD-08 | Notices are retained and linked to every component they cover. | (b) | A synthetic pin set declaring a notice member for the text and annotation members, and a Mouseion-issued record notice for the interpretation. Owner A. | Import and prepare. Then request A's evidence detail for the Book. | Each component reports its covering notices, with exact text that can be retrieved. The notice member's bytes verify against their pin. | A pin set that declares a notice member absent from the package is rejected at inventory, and nothing is written. |
| EVD-09 | Extracted text and its spans are tied to an exact digest. | (d) | — | Insert an Extracted text whose digest is not the SHA-256 of its UTF-8 content. Update an existing Extracted text. Insert a binding whose `extracted_text_digest` names no Extracted text. | All are rejected. | Extracted text whose digest matches is accepted, and an insert of equal content reuses the key. |
| EVD-10 | Text and Annotation Source revisions are immutable. | (d) | One synthetic text revision and one annotation revision with their member rows. | Update any field of each revision and member row. | Every update is rejected. | Inserting a second revision with an equal uniqueness tuple is rejected. |
| EVD-11 | Undeclared upstream metadata stays explicitly unknown. | (b) | A synthetic pin set declaring `release_identifier: unknown` and `scheme_version: unknown` for the annotation revision. Owner A. | Import, then read A's evidence detail. | The revision reports both fields as unknown, not an empty string, a default or a value inferred from file names. | A pin set that omits the field rather than declaring it unknown stops import, and nothing is written. |
| EVD-12 | Whole-work bytes create no offering, Book, binding, analysis or count for units the definition does not name. | (a) | A synthetic package with text and annotation members for `example-section` 1–7. Its reviewed definition names only unit `constituent_book:example-section:1`. Owner A. | Register and import, prepare, then request My Books and Vocabulary. | A has one Book, for section 1, with one binding and one analysis over section 1. Counts cover only section 1 occurrences. | No Book, Unit offering, Local registration, binding or analysis exists for sections 2–7. Counts and Browse rows contain no occurrence from them. |
| EVD-13 | A binding is unique on its inputs and immutable. A changed binding is a new row. | (d) | One validated synthetic binding. | Update its unit, bounds, projection or extracted-text digest. Insert a second binding with an equal key. | Every update and the duplicate insert are rejected. | The same revisions with projection `example-projection/2` give a new binding row, and the first is unchanged. |
| EVD-14 | Recomputing a binding is deterministic. | (b) | An existing validated binding. A test adapter that returns a different extracted text for the same inputs. | Prepare the same unit again with that adapter. | The request fails with `binding_nondeterministic`. No new binding, extracted text or analysis is written, and A's current analysis is unchanged. | With the normal adapter, the existing binding is reused and no row is added. |
| EVD-15 | No analysis references a rejected binding. | (d) | A synthetic binding with `validation_status = rejected` and one reason code. | Insert an Exact analysis on it. | Rejected. The binding remains, with its reasons. | An analysis on a validated binding is accepted. |
| EVD-16 | An unknown interpretation, contract or table version stops import. | (c) | A synthetic candidate whose interpretation names `example-lemma/9`, which is absent from the compiled registry. Variants: an unknown `contract_version`, and an unlisted table digest. | Submit each to shared validation. | Each returns a rejection, `interpretation_version_unknown`, identifying the slot, and no validated interpretation. | The same candidate with `example-lemma/1` validates. Persistence after the rejection is EVD-33. |
| EVD-17 | The same version set resolves to the same interpretation identity. | (b) | The synthetic interpretation set `example-decode/1`, `example-lemma/1`, `example-upos/1`, `example-repairs/1`, `example-terminal/1`, `ancient-greek/1` and table `T`. | Prepare two different Books, for owners A and B, with that set. | Both analyses reference one Interpretation configuration. | Changing any one slot, or the table digest, gives a different configuration. A repair layer of none differs from `example-repairs/1`. |
| EVD-18 | A new mapping version produces a new analysis and leaves the old one unchanged. | (b) | Owner A has adopted analysis X under `example-lemma/1`, has an active Reading and snapshot, and has a Lemma correction on X. | Prepare and adopt under `example-lemma/2`. | New analysis Y with a new configuration. X, its diagnostics and occurrences, the snapshot and the correction are unchanged and still readable. | Y carries no correction from X, and X is never updated to point at the new configuration. |
| EVD-19 | At most one Exact analysis per (binding, interpretation, adapter). | (d) | An Exact analysis for (binding B, interpretation I, `example-adapter/1`). | Insert a second analysis with the same triple. Update the first analysis's interpretation. | Both are rejected. | `example-adapter/2` with the same B and I is accepted as a new analysis. |
| EVD-20 | Every analysis persists one diagnostic state per capability, with reasons and scope. | (b) | A synthetic candidate with usable vocabulary and sentence/target, unsupported morphology, basic syntax invalid in one sentence, and missing secondary syntax. One node has an ambiguous POS. | Prepare it, then read A's evidence detail. | Five capability states are reported: `usable`, `usable`, `unsupported`, `usable` with an `invalid` sentence finding, and `missing`. Reason codes are present for every non-usable state and finding. The node finding reports `ambiguous`. | A write of an analysis lacking any one of the five capability rows fails at commit, and no analysis remains. |
| EVD-21 | Diagnostics and findings are immutable. | (d) | An analysis with five Capability diagnostics and one finding. | Update a state, a reason code or a finding. Delete a diagnostic. | All are rejected. | Inserting a further finding for the committed analysis is also rejected, because an analysis is written in one transaction. |
| EVD-22 | A confirmed defect is appended as a restriction, and the analysis is not edited. | (b) | Owner A has adopted analysis X with usable basic syntax. | Record a `confirmed_defect` restriction on X for `basic_syntax`. | A's syntax-dependent features report it unavailable, with the explanation. Vocabulary, Reading and the Capability diagnostic row are unchanged. X's fields are byte-identical. | Recording a second restriction adds a row and changes none. No learner route can record one. |
| EVD-23 | A restriction applies transitively, and only to the capabilities it names. | (b) | Analyses X and Y both use annotation revision R. Analysis Z uses another revision. | Record an `integrity_failure` restriction on R for `morphology`. | X and Y report morphology as restricted. Z is unaffected, and vocabulary stays usable for X and Y. New adoption of X or Y for a morphology-dependent request is refused. | Z's morphology stays usable, and the restriction changes no diagnostic row of X, Y or Z. |
| EVD-24 | Provenance claims are kept per layer and scope, including the repair layer. | (b) | A synthetic pin set: lemma `automatic` at release scope, basic syntax `human_reviewed_automatic` for a sentence subset, the repair layer `automatic` on the interpretation, and morphology `unknown`. | Import, prepare, then read evidence detail. | Each layer reports its own claim and scope, attributed to its source statement. | Basic syntax review is not reported as lemma or morphology review. A pin set lacking a release claim for a used layer stops import. |
| EVD-25 | Identical bytes never give another owner access. | (a) | Owner A has imported and adopted `example-pkg`. Owner B has none. | B requests evidence detail with A's Book ID, A's acquisition ID, and the shared revision, binding and analysis IDs. Then B imports identical `example-pkg`. | Before the import, every request returns the existing not-found response. After it, B has B's own acquisition and can read the shared revisions through it. B still gets not-found for A's Book, acquisition and current record. | A's acquisition, current record and request are unchanged by B's import. |
| EVD-26 | The database enforces the package bounds as ceilings. | (d) | — | Insert a blob over 16,777,216 bytes. Insert a Package content with 129 members, or with expanded bytes over 67,108,864. Commit a content whose member rows disagree with its count or sum. | Each is rejected. | A content whose count and sum equal its rows commits. |
| EVD-27 | At most one current published analysis per (owner, Book), across both anchors. | (d) | Owner A has a classical Book C and an EPUB Book E. | Insert an existing-anchor (EPUB) current analysis for C. Insert a classical current row for E. Insert a second classical current row for C. Insert a classical current row whose acquisitions do not carry the analysis's revisions. | All are rejected. | A classical current row for C with matching acquisitions is accepted. |
| EVD-28 | EPUB analysis promotion is unchanged. | (a) | The existing EPUB analysis and promotion integration scenarios. | Run them with the classical schema present. | Same responses and persisted current analyses as the baseline. No classical row is created, and the exclusion trigger never fires. | No EPUB Book gains a Reading unit association or a classical current row. |
| EVD-29 | Owner deletion removes that owner's access rows and leaves shared evidence and other owners intact. | (d) | Owners A and B each hold an acquisition of `example-pkg` and a current classical analysis on the shared analysis. | Delete user A. | A's acquisitions and current rows are gone. Blob, content, revisions, binding, interpretation, analysis, diagnostics, B's acquisition and B's current row remain. | B's evidence detail and Reading are unchanged after the deletion. |
| EVD-30 | Backup and restore reproduce exact evidence. | (d) | A synthetic installation with two owners, two builds, notices, a restriction and claims. | Take a logical dump, then restore into an empty database. | Every evidence row is equal. Every blob, extracted-text and member digest re-verifies, and each owner's evidence detail is unchanged. | A restore missing one blob fails digest verification for each record that references it, rather than reporting the evidence usable. |
| EVD-31 | A selection-policy change creates no new interpretation or analysis. | (b) | Owner A has adopted analysis X. The reviewed definition's selection policy changes from `example-select/1` to `example-select/2`. | Prepare A's Book again under the new definition version. | No new Interpretation configuration or Exact analysis. X stays current. The new policy is recorded only for newly prepared specifications (§8). | Changing any analysis-affecting slot in the same definition does create a new analysis (see EVD-18). |
| EVD-32 | A binding references only a shared Reading unit. | (d) | A private §4 unit owned by A. | Insert a binding on it. | Rejected. | A binding on a shared unit is accepted. |
| EVD-33 | An import naming an unknown interpretation version persists nothing for it. | (a) | Owner A. A synthetic package whose reviewed test definition names `example-lemma/9`, which is absent from the compiled registry. | Run the local import and preparation through the handler and River job. | The request ends failed with `interpretation_version_unknown`, without automatic retry. No Interpretation configuration, binding, analysis, diagnostic or current classical analysis is written for it. | The same package with `example-lemma/1` prepares successfully. |
| IDN-01 | Canonically equivalent NFC and NFD inputs give one key. | (c) | `ancient-greek-normalization/1` cases `λέξις` (U+03AD) and `λε` + U+0301 + `ξις`. | Normalize each under `ancient-greek/1`. | Both keys are U+03BB U+03AD U+03BE U+03B9 U+03C3 (`λέξισ`). | `λὲξις` (grave) does not give this key (see IDN-06). |
| IDN-02 | Oxia and tonos, which are canonically equivalent, give one key. | (c) | Cases `λ` + U+1F73 + `ξις` and `λέξις` (U+03AD). | Normalize each. | Both keys are `λέξισ` with U+03AD. | U+1F72 (varia) does not give this key. |
| IDN-03 | Case does not distinguish keys. | (c) | Cases `Λέξις` and `ΛΈΞΙΣ`. | Normalize each. | Both keys equal the key of `λέξις`. | Normalizing the key again changes nothing (see IDN-11). |
| IDN-04 | Final and medial sigma give one key. | (c) | Cases `λέξις` and `λέξισ`. | Normalize each. | Both keys end in U+03C3. No key contains U+03C2. | Lunate U+03F2 is not unified with either (see IDN-09). |
| IDN-05 | Iota subscript and adscript stay distinct, and a capital keeps its subscript. | (c) | Cases `λῄξις` (U+1FC4), `λήιξις`, and `ᾼ` (U+1FBC). | Normalize each, and compare the first two. | `λῄξισ` keeps U+1FC4. `λήιξισ` keeps U+03AE U+03B9. The two keys differ. `ᾼ` gives U+1FB3. | Neither the casefold profile's output `αι` for `ᾼ` nor `λήιξισ` for `λῄξις` is produced. |
| IDN-06 | Acute and grave stay distinct. | (c) | Cases `λέξις` and `λὲξις` (U+1F72). | Normalize and compare. | `λέξισ` and `λὲξισ`. The keys differ. | No accent is stripped or converted. |
| IDN-07 | Combining U+0313 and U+2019 elision marks are preserved and stay distinct. | (c) | Cases `λέξ` + U+0313 and `λέξ` + U+2019. | Normalize and compare. | Each key equals its input. The two keys differ. | Neither mark is removed, replaced or mapped to the other, or to U+1FBD or U+02BC. |
| IDN-08 | Malformed stacked marks are kept, not repaired. | (c) | Cases `λε` + U+0301 + U+0301 + `ξις`, and U+0313 + U+0301 + `Λέξις`. | Normalize each. | Keys `λ` U+03AD U+0301 `ξισ`, and U+0313 U+0301 `λέξισ`. The second U+0301 and the leading marks remain. | The key never equals `λέξισ`. |
| IDN-09 | No compatibility mapping is applied. | (c) | Case `λέξι` + U+03F2. | Normalize. | The key ends in U+03F2. | It differs from `λέξισ`. An NFKC implementation would fail this row. |
| IDN-10 | Lowercasing uses simple mappings. | (c) | Case U+0130. | Normalize. | Key U+0069. | A full-mapping implementation, which gives U+0069 U+0307, fails this row. |
| IDN-11 | The profile is idempotent and pinned to Unicode 17.0.0. | (c) | Every key in the `ancient-greek-normalization/1` fixture. | Normalize each key again, and read the implementation's reported Unicode versions. | Each key is unchanged. The Go implementation reports `17.0.0` for both case mapping and normalization. | A build whose reported version differs fails this row before any fixture runs. |
| IDN-12 | Input that is not admitted is rejected, never repaired. | (c) | Cases: empty; ` λέξις`; `λέξις λέξις`; `λέξις` + U+0007; `λέξις` + a code point unassigned in Unicode 17.0.0. | Submit each to the admission check. | Each is rejected with `citation_form_invalid`. No key is returned. | `λέξις` is admitted. |
| IDN-13 | `grc` never uses the casefold fallback, in the registry or in validation. | (c) | The default registry. A synthetic `grc` candidate labelled `unicode-casefold` 1.2.0, as an NLP-shaped producer would label it. | Call `For("grc")`, `Lookup("grc", "1")`, `Lookup("grc", "1.2.0")` and `For("grc-x-example")`. Submit the candidate to shared validation. | `For` gives `ancient-greek/1` for both tags. `Lookup("grc", "1")` gives `ancient-greek/1`. `Lookup("grc", "1.2.0")` gives `ErrProfileNotFound`. Validation rejects the candidate with `normalization_profile_rejected`. | A test registry without a `grc` profile returns `ErrUnsupportedLanguage` from `For("grc")`, not the casefold profile. `For("xx")` still returns the casefold profile. |
| IDN-14 | No learner route normalizes `grc` with casefold. | (a) | Owner A has a classical Book with an adopted synthetic analysis under `ancient-greek/1`. | Confirm a Lemma correction to `ᾼλέξις` through lemma review. Run a Concordance lemma lookup for `ᾼλέξις`. | The stored corrected key and the Concordance lookup key both contain U+1FB3, not `αι`. The correction records `ancient-greek/1`. | With the `grc` profile removed from a test registry, both routes report the language as unsupported and write nothing. |
| IDN-15 | `grc` and `el` resolve to different profiles and give distinct identities for one spelling. | (c) | `vocabulary-identity-comparison/1` case: `grc` `λέξισ` NOUN null, and `el` `λέξισ` NOUN null. | Resolve each language's profile. Compare the two identities. | `grc` gives `ancient-greek/1`. `el` gives `modern-greek` 1. The identities are not equal. | `For("el")` never returns `ancient-greek/1`, and `For("grc")` never returns `modern-greek`. |
| IDN-16 | `grc` and `el` learner state stays separate. | (b) | Owner A has `el` `λέξισ` NOUN Known. A `grc` analysis contains `λέξις` NOUN with a null discriminator. | Read the `grc` Book's vocabulary, Known status and counts. | The `grc` identity is not Known and counts as unknown. The `el` Known row is unchanged. | Marking the `grc` identity Known leaves the `el` row and `el` counts unchanged. |
| IDN-17 | Same lemma and UPOS with different discriminators are distinct identities. | (c) | Comparison cases: `λέξισ` NOUN `example:ex:1`, `example:ex:2`, and null. | Compare every pair. | No two of the three are equal. | Equal scheme and value, compared byte for byte, are equal. |
| IDN-18 | At most one identity row per five-value identity, even under concurrency. | (d) | No identity `grc` `λέξισ` NOUN `example:ex:1`. | Two transactions insert-if-absent that identity concurrently. Then insert `grc` `λέξισ` NOUN null twice. | Each pair resolves to one row and one ID. | `example:ex:2` gets a second, distinct row. |
| IDN-19 | Identity rows are well formed and immutable. | (d) | One identity row. | Insert a row with only `discriminator_scheme` set, with only `discriminator_value` set, with an empty lemma, and with an empty or unknown UPOS. Update any field of the existing row. | Every insert and the update are rejected. | A row with both discriminator fields null, or both set, is accepted. |
| IDN-20 | No lemma carries a discriminator marker. | (c) | Scheme `example` registered with native marker `#ex:<n>`. Scheme `proiel` with marker `#<digits>`. | Submit `λέξις#ex:1` and the public anchor `εἰ#1` to admission as citation forms. | Both are rejected with `discriminator_in_lemma`. | `λέξις` and `εἰ` are admitted. |
| IDN-21 | Decoding produces the discriminator, and normalization never infers one. | (c) | `source-decoding/1` cases for `example-decode/1`: native `λέξις#ex:1`; native `λέξις` with attribute `lexid=ex:1`; native `λέξις` with attribute `lexid=zz:9`, which has no reviewed mapping. | Decode each, then normalize the decoded form. | The first two give citation form `λέξις`, `example:ex:1`, and key `λέξισ`. The third is rejected with `discriminator_unmapped`. Normalizing `λέξις` alone gives a null discriminator. | The unmapped identifier is neither dropped nor turned into a null-discriminator identity. |
| IDN-22 | The profile version is provenance, so unchanged identities match across versions. | (b) | Owner A has `λέξισ` NOUN `example:ex:1` Known, from an analysis under `ancient-greek/1`. A test-only `ancient-greek/2`, identical for this input, is registered. | Adopt a new analysis of another Book under `ancient-greek/2` that contains the same identity. | The identity reuses the same row and is Known in the new Book. | A synthetic input whose key changes under `ancient-greek/2` gives a different identity, which is not Known. |
| IDN-23 | Discriminated identities stay distinct in Known, Reserved, snapshots and completion. | (b) | A classical Book with occurrences of `λέξις` NOUN `example:ex:1` and `example:ex:2`. Owner A has neither Known. | Start a Reading, freeze the snapshot, then finish and accept it. | The snapshot holds two identities. Both are Reserved while the Reading is active. After acceptance, A has two distinct Known identities, and the completion counts two. | In a second Book where every `example:ex:2` occurrence has an Occurrence exclusion, the snapshot, Reserved set and accepted Known hold only `example:ex:1`. `example:ex:2` stays not Known. |
| IDN-24 | A legacy null-discriminator Known row is not a wildcard. | (b) | Owner A has a legacy tuple Known row, `xx` `λέξισ` NOUN. `xx` is a test-only language, not `grc`, so that comparison rule 5 does not decide the result. A rich analysis in `xx` contains `λέξισ` NOUN `example:ex:1` and `example:ex:2`. | Read Known status and counts for both. | Neither discriminated identity is Known. The legacy row is unchanged. | A rich `xx` `λέξισ` NOUN identity with a null discriminator is Known. |
| IDN-25 | A legacy blank-UPOS Known row never matches a rich identity. | (b) | Owner A has a legacy `xx` `λέξισ` Known row with `upos = ''`, and a modern `de` Known row with `upos = ''`. | Read the Known status of rich `xx` `λέξισ` NOUN with a null discriminator, and of a `de` tuple occurrence with that lemma. | The rich identity is not Known. The `de` occurrence is Known, as today. | Neither legacy row is changed, and no rich identity gains Known from either. |
| IDN-26 | Known never transfers across a split or a merge. | (b) | Split: owner A has `λέξισ` NOUN with a null discriminator Known. A's Book adopts analysis Y, whose new interpretation gives `example:ex:1` and `example:ex:2` instead. Merge: owner B has `λέξισ` NOUN `example:ex:1` Known and no null identity Known. B's Book adopts analysis Z, whose interpretation gives only the null identity. | Adopt Y for A and Z for B, then read Known status. | Neither descendant is Known for A. The null identity is not Known for B. Both original Known rows remain on their original identities. | Neither adoption creates, deletes or rewrites a Known row. |
| IDN-27 | Legacy `grc` tuple rows match no rich `grc` identity and are not rewritten. | (b) | Owner A has a legacy `grc` Known tuple `λήιξισ` NOUN, created by the casefold fallback. A classical analysis contains `λήιξις` NOUN with a null discriminator, whose key is `λήιξισ`. | Read Known status and counts for the classical Book. List A's Known vocabulary. | The rich identity is not Known. The legacy row is listed unchanged, with its key. | No Known row is added, removed or changed. A legacy `grc` Known tuple `λέξισ` NOUN marks none of the rich `grc` identities `λέξισ` NOUN null, `example:ex:1` or `example:ex:2` Known or Reserved. |
| IDN-28 | Learners see the decoded citation form, never the key, and the discriminator separately. | (a) | Owner A has an adopted classical analysis containing `λέξις` NOUN `example:ex:1`. | Request the Book's vocabulary, the Reading snapshot review and Known. | Each shows the headword `λέξις` with final `ς`, and the discriminator as separate metadata. | No response shows `λέξισ` or `λέξις#ex:1` as a headword. |
| IDN-29 | The Analysis headword is deterministic, and variants are reported. | (b) | A synthetic analysis with three occurrences decoded as `Λέξις` and two as `λέξις`, one identity. | Prepare it twice, then read evidence detail. | Both preparations give headword `Λέξις` and `variant_count` 2. Evidence detail reports the variants. | With a 2–2 tie, the form of the earliest occurrence in reading order wins. |
| IDN-30 | Disagreeing analyses each show their own headword, and frozen records keep theirs. | (b) | Two Books with analyses whose headwords for one identity are `Λέξις` and `λέξις`. Owner A has an active snapshot frozen on the first. | Read each Book's vocabulary. Adopt a new analysis for the first Book with headword `λέξις`. Read the snapshot. | Each Book shows its own analysis's headword. The snapshot still shows `Λέξις`. The identity row is unchanged. | No headword change creates or merges an identity. |
| IDN-31 | A correction uses the analysis's profile, yields a null discriminator, and leaves evidence unchanged. | (b) | Owner A's classical analysis X under `ancient-greek/1` has an occurrence of `λέξις` NOUN `example:ex:1`. | Confirm a correction to `λῆξις`. | The effective identity is `λῆξισ` NOUN with a null discriminator. The correction records `ancient-greek/1` and headword `λῆξις`. X's occurrence, decoded form, Analysis headword and native evidence are unchanged. | The correction does not apply in any other analysis, Book or owner. |
| IDN-32 | A learner cannot set or clear a discriminator. | (a) | As IDN-31. | Submit a correction request carrying a discriminator parameter. Submit a correction to `λέξις`. | The parameter is ignored and no discriminator is stored. The second request resolves to `keep` and stores nothing. The effective identity keeps `example:ex:1`. | No route accepts a discriminator from learner input. |
| IDN-33 | No lemma review runs on a legacy casefold `grc` analysis. | (b) | Owner A has a legacy analysis-run-backed `grc` Book, whose profile is `unicode-casefold` 1.2.0. | Request lemma review and confirm a correction. | Lemma review reports the language unsupported for that analysis. Nothing is written. | Lemma review on a `de` Book works as today. |
| IDN-34 | `grc` search input reaches the same lemma results whatever its NFC form, case or final sigma. | (a) | Owner A has an adopted classical analysis containing `λέξις` NOUN. | Run Concordance lemma lookups for `λέξις`, its NFD form, `ΛΈΞΙΣ` and `λέξισ`. | All four return the same occurrences. | `λὲξις` and `λήιξις` return none of them. |
| IDN-35 | Modern-language normalization is unchanged. | (c) | The German parity fixture, and characterization cases for `modern-greek` 1 and `unicode-casefold` 1.2.0 recorded at this commit. | Normalize every case. | Every key equals its recorded value. | `For("el")`, `For("de")` and `For("xx")` return the same profile name and version as at this commit. |
| IDN-36 | Modern-language identities and learner outcomes are unchanged. | (b) | Every existing store-contract scenario (language `de`). | Run them with rich identities and the classical schema present. Run selection over synthetic `de` and `el` corpora. | Every scenario passes unchanged. Known, Reserved, snapshot, completion and count results equal the baseline, including the blank-UPOS Known match. Selection gives the same candidate identities, counts, observed forms and sentence references as the baseline recorded at this commit. | No modern-language Known, snapshot or count row gains a discriminator. |
| IDN-37 | Cross-Book views choose an identity's headword by the stated preference order. | (b) | Owner A has two classical Books whose current analyses give one identity the headwords `Λέξις` and `λέξις`. The second Book is the Concordance priority Book. | Read the Concordance lemma group. Then accept a completed Reading of the first Book that makes the identity Known, and read the Known list and the Concordance group. | Before acceptance, the group shows `λέξις`, from the priority Book. After it, both views show `Λέξις`, the headword frozen with the Known acceptance. | Without a priority Book or Known acceptance, the view shows `Λέξις`, the smaller by code-point order. It never shows the key. |
| IDN-38 | A corrected identity shows the correction's citation form in its Book. | (b) | Owner A's Book has two occurrences of `λέξις` NOUN `example:ex:1`. | Confirm a correction of the first to `Λῆξις`, then of the second to `λῆξις`. Read the Book's vocabulary. | The effective identity `λῆξισ` NOUN with a null discriminator shows `λῆξις`, from the latest correction. The Analysis headword for `example:ex:1` is unchanged. | Another Book with the same identity from its analysis shows its own headword, not the correction's. |
| OCC-01 | Two nodes with an identical span are two occurrences with two keys. | (c) | A synthetic candidate. Nodes 4 (`λέξις` NOUN `example:ex:1`) and 5 (`λέξις` NOUN `example:ex:2`) are both `word`, `vocabulary`, with span `[10,16)`. | Validate the candidate. | Accepted. It yields occurrences `(A,4)` and `(A,5)`, both counted. Each key resolves to its own node and identity. | A candidate that gives node 5 the same `native_node_ref` as node 4 in that sentence is rejected (`OCC-ID-1`). A lookup by span `[10,16)` is not an occurrence key and is not offered by the interface. |
| OCC-02 | A discontinuous span round-trips exactly. | (d) | A validated synthetic analysis where node 9 has spans `[2,5)` and `[9,12)`, and the Extracted text has known slices at both. | Persist it, then read node 9 through the analysis read interface. | Two spans in order, `[2,5)` then `[9,12)`. The text slices equal the Extracted text at those spans. No span `[2,12)` exists. | Validating spans `[2,5)` and `[4,8)` (overlapping), `[2,5)` and `[5,8)` (adjacent), or `[9,12)` before `[2,5)` is rejected (`SPAN-2`), seam (c). |
| OCC-03 | An empty node has no span and no count. | (c) | A candidate sentence with words 1–3 and an empty node `2.1` that has a lemma and secondary-syntax edges. | Validate and read counts. | The empty node is `empty`, `non_vocabulary(empty_node)`, with a NULL surface, no span and no occurrence. The count is that of words 1–3 only. | A candidate giving the empty node a span, a surface or class `vocabulary` is rejected (`SPAN-3`, `ACC-2`). |
| OCC-04 | A container with two counted components counts 2, not 3. | (c) | Container node 3, surface `λέξισῥῆσ`, span `[10,18)`, with components 4 (`λέξις` NOUN `example:ex:1`) and 5 (`ῥῆσις` NOUN), both referencing `[10,18)`. | Validate and count. | Occurrences `(A,4)` and `(A,5)`. The container has no occurrence. The count is 2. | A candidate classing the container `vocabulary`, giving a component a span `[10,14)` computed from `len("λέξις")` without native offsets, or a span outside `[10,18)`, is rejected (`ACC-2`, `SPAN-4`, `CONT-2`). |
| OCC-05 | Punctuation is not an occurrence. | (c) | A candidate sentence ending in a punctuation node with span `[30,31)`. | Validate. | The node is `punctuation`, `non_vocabulary(punctuation)`, with a span and no occurrence. It counts nothing. | A candidate classing it `vocabulary` or `unresolved` is rejected. At seam (d), inserting an occurrence row for that node is rejected by the database. |
| OCC-06 | A sentence cut by the extent is persisted as `incomplete`. | (a) | Owner A. A synthetic package whose unit `book 1` starts inside a native sentence. | Import and prepare the Book, then open its first sentence in sentence study. | The sentence is reported `incomplete` with reason `unit_extent_cuts_sentence`. Its in-extent nodes are present and counted. Its text is only the in-extent slice. It is not offered as a complete sentence target. | The adapter declaring that sentence `complete` is rejected by validation (`SENT-3`), and nothing is written. |
| OCC-07 | Missing syntax gives `basic_syntax` `missing`, and Concordance shows unavailable, not zero. | (a) | Owner A. A Vocabulary-ready synthetic analysis without basic syntax, adopted for Book G. Owner A also has an EPUB Book with legacy syntax. | Run a syntax-dependent Concordance query (role filter) for a lemma present in G. Then open sentence study for a G sentence. | Capability `basic_syntax` is `missing` with reason `syntax_absent`. The query names G as "syntax unavailable" with the reason, and lists only the EPUB Book's matches. Sentence study shows "Syntax unavailable" and no relation or head. Lemma lookup still finds G's occurrences. | The response never reports G as having zero matches for the role, and never shows an empty relation. |
| OCC-08 | A cyclic tree is withheld as `invalid`, and vocabulary stays usable. | (c) | A candidate where sentence 2's basic heads form a cycle (node 5 → 6 → 5) and every other sentence has a complete tree. | Validate, then read sentence 2's basic syntax through the interface. | Accepted, and `vocabulary_ready` is true. Sentence 2 has a `basic_syntax` finding `invalid` with reason `tree_cycle`. The interface returns state `invalid` and no edges for sentence 2, and edges for the other sentences. The stored edges stay inspectable in evidence detail. | No root is added and no edge is dropped to repair the cycle (`TREE-5`). |
| OCC-09 | An unresolved lexical occurrence blocks Vocabulary-ready. | (c) | A candidate in which one `word` node has a native lemma that the interpretation cannot map, and is classed `unresolved`. | Validate. | Accepted for inspection with `vocabulary_ready` false. The node has an occurrence that does not count, and a vocabulary finding names it. | The same candidate with that native node dropped from the node set is rejected (`ACC-1`), and nothing is written. |
| OCC-10 | Non-vocabulary conjunction accounting is accepted with ambiguous UPOS. | (c) | The synthetic conjunction example: node 7 `ξέλ`, native POS `example:C`, `upos_state` `ambiguous` with candidates CCONJ and SCONJ, `non_vocabulary(native_conjunction_class)`, evidence `pos@7`. All other nodes resolved. | Validate. | Accepted with `vocabulary_ready` true. Node 7 has no occurrence and no count, and a node finding `ambiguous` (`upos_mapping_ambiguous`). It is not shown as an Occurrence exclusion. | Node 7 with UPOS `X`, with no `native_evidence_ref`, or with reason `native_conjunction_class` under an interpretation that has no reviewed conjunction mapping, is rejected (`ACC-2`). |
| OCC-11 | A legacy `corpus_tokens` analysis reads identically through the shared interface. | (a) | Owner A. A German EPUB Book with a published legacy analysis that includes a multiword token, a shared span, a correction and an exclusion. The responses of the Reading page, Book vocabulary, Browse, lemma review, sentence study, Concordance and deck preparation are recorded before the interface. | Request the same pages after the interface is in place. | Every response is byte-identical to the recording, apart from CSRF tokens and timestamps. Occurrence keys stay offset-keyed. A decision on the shared span still fails as today. | A checksum of the analysis's `corpus_tokens` and `corpus_sentences` rows is unchanged after every request. |
| OCC-12 | Occurrence rows are unique per (analysis, node), and the key names no offset. | (d) | A persisted analysis with occurrences `(A,4)` and `(A,5)` that share span `[10,16)`. | Insert a second occurrence row for `(A,4)`. Insert a decision for `(A,4)` and another for `(A,5)`. | The duplicate occurrence is rejected. Both decisions are stored and each references one occurrence. | A decision referencing `(A,99)` (no such occurrence) is rejected by the foreign key. |
| OCC-13 | Equal native IDs in two analyses give two occurrences, and nothing transfers. | (c) | Analyses A and B of the same binding under two interpretations. Both contain native node `s1:4` with span `[10,16)`. Owner O has a Lemma correction on `(A,n)`. | Read O's effective identities for B. | `(A,n)` and `(B,m)` are different keys, even if `n = m`. B shows the analysis identity for `(B,m)` and no correction. | Matching `native_node_ref`, span or surface never attributes O's decision to `(B,m)`. |
| OCC-14 | No span is derived from length. Component spans lie within the container's spans. | (c) | A container `[10,18)` whose source gives no component offsets. | Validate it with components referencing `[10,18)`, and again with component spans `[10,15)` and `[15,18)` that the adapter computed from citation-form lengths. | The first is accepted as a shared span. The second is rejected (`SPAN-4`), because its spans have no native coordinate mapping. | Components with native-attested sub-spans `[10,15)` and `[15,18)` are accepted. |
| OCC-15 | Sentence text comes only from the bound text. | (c) | A candidate sentence whose spans slice to `λέξις, ῥῆσις` in the Extracted text. | Validate it with supplied text `λέξις ῥῆσις` and no reconstruction. | Rejected with `sentence_text_unbound`. | The same text with a reviewed reconstruction `example-spacing/1` is accepted and stored as `reconstructed_text`. Without supplied text, the sentence text is the slices. |
| OCC-16 | Every node in the extent has exactly one accounting class. | (c) | A candidate with a word node whose accounting class is absent. | Validate. | Rejected (`ACC-1`), and nothing is written. | A candidate with every node classed is accepted. |
| OCC-17 | A coverage gap blocks Vocabulary-ready. | (c) | A candidate whose Extracted text has a non-whitespace code point at `[50,51)` that no printed node covers. All other nodes are resolved. | Validate. | Accepted for inspection with `vocabulary_ready` false and a `coverage_gap` finding for `[50,51)`. | Uncovered whitespace alone leaves `vocabulary_ready` true. |
| OCC-18 | A tree without a root is withheld, and no root is fabricated. | (c) | Sentence 3 of a candidate gives heads for every node, and none of them is a root. Sentence 4 has two roots. Sentence 5 has a node whose head is in sentence 4. | Validate. | Accepted. Sentences 3, 4 and 5 get `invalid` findings `root_missing`, `root_multiple` and `head_unresolved`, and their edges are withheld. Vocabulary readiness is unchanged. | A self-headed row with relation `nsubj`, or a `root` row whose head is another node, is rejected as malformed (`TREE-2`). |
| OCC-19 | Morphology keeps native features and gives FEATS only where mapped. | (c) | A node whose native tag has one feature with a reviewed mapping and one without. | Validate and read the node's morphology. | `native_morphology` holds the full native tag. `feats` holds only the mapped feature. A node finding marks the unmapped feature `unsupported`. | A candidate putting a guessed value for the unmapped feature into `feats` is rejected (`canonical_value_unmapped`). |
| OCC-20 | Secondary syntax is kept separately, including empty-node edges. | (c) | A candidate with complete basic trees and secondary edges to and from an empty node. | Validate, then read both layers. | Basic syntax `usable`. Secondary syntax `usable`, with the empty-node edges. No secondary edge appears in basic syntax. | A candidate without secondary edges has `secondary_syntax` `missing` and leaves basic syntax unchanged. A basic edge whose endpoint is the empty node is rejected (`SYN-1`). |
| OCC-21 | Unavailable syntax never makes an occurrence ineligible. | (b) | Owner A's Book has a Vocabulary-ready analysis with `basic_syntax` `missing`, and a NOUN occurrence that would be eligible. | Read A's Book vocabulary and counts. | The occurrence is eligible and counted. | With usable syntax in which that node's relation is a syntax-excluded relation, the occurrence is not counted. |
| OCC-22 | §7 records are immutable once the analysis commits. | (d) | A committed analysis with sentences, nodes, spans, a container, occurrences, morphology and both syntax layers. | Update any field of each record. Insert a further node, span or occurrence for that analysis. Delete a node. | Every operation is rejected, and the rows are unchanged. | Inserting the same records for a new analysis in its own transaction succeeds. |
| OCC-23 | Syntax endpoints stay within one analysis and one sentence. | (d) | Committed analyses A and B. | Insert, in A's transaction, a basic edge whose head is a node of B, and one whose head is in another sentence of A. | Both are rejected. | An edge between two nodes of the same sentence of A is accepted. |
| OCC-24 | New analyses write no legacy rows, and legacy rows are never rewritten. | (a) | Owner A has an EPUB Book with a legacy analysis. The legacy table row counts and checksums are recorded. | Import and adopt a synthetic classical Book for A, read it through every consumer, and record a decision on it. | `analysis_runs`, `corpora`, `corpus_sentences` and `corpus_tokens` have the same counts and checksums as before. | Re-analysing the EPUB Book still writes legacy rows through the existing path, as today. |
| OCC-25 | The `grc` form search key matches NFC and NFD, case and final sigma, and nothing broader. | (a) | Owner A has a classical Book with a node whose surface is `Λέξις` in NFD. | Run Concordance form searches for `λέξις` (NFC), `ΛΈΞΙΣ` and `λέξισ`. Then search for `λεξις`. | The first three find the node, and the highlight is its span. | `λεξις` (no accent) finds nothing. A search never changes any identity or Known state. |
| LRN-01 | The same lemma and UPOS with `ex:1` and `ex:2` stays distinct in counts, Known, Reserved, the snapshot and Finish. | (b) | `grc`. Owner A's To Read classical Book G1 has an analysis with `λέξις` NOUN `example:ex:1` ×3 and `example:ex:2` ×3, ready counts, nothing Known. | Read G1's counts. Start G1, read Reserved, then Finish. | Counts list two identities with 3 each. The snapshot has two members. Both are Reserved while reading. Finish records snapshot 2, eligible 2, newly Known 2, already Known 0, and A has two Rich Known identities. | With `example:ex:1` already Rich Known before Start, the snapshot holds only `example:ex:2`, Finish accepts only it, and the `ex:1` Known row keeps its original completion. A null-discriminator `λέξισ` NOUN is never Known or Reserved by either. |
| LRN-02 | Same-span occurrences hold independent decisions. | (b) | G1's analysis X has nodes 4 (`example:ex:1`) and 5 (`example:ex:2`), both at span `[10,16)`, each eligible. | Confirm an exclusion of `(X,4)`. Then confirm a correction of `(X,5)` to `ῥῆσις`. | `(X,4)` is excluded and `(X,5)` is corrected; reading each key returns only its own decision. Counts: `ex:1` and `ex:2` each fall by 1, and `ῥῆσισ` NOUN (null discriminator) rises by 1. | A decision request that names the span `[10,16)` and no node is rejected as not found and writes nothing. The exclusion never changes `(X,5)`. |
| LRN-03 | A decision never transfers to another analysis. | (b) | G1 has a correction on `(X,4)`. §11 adopts analysis Y for G1, in which node 4 has the same native reference, span and identity. | Read G1's effective identities, counts and decisions. | `(Y,4)` has its native identity, and Y's counts ignore the `(X,4)` correction. The `(X,4)` decision is still stored, unchanged. | After Y is adopted, a confirm naming `(X,4)` is rejected and writes nothing. No decision row names `(Y,4)`. |
| LRN-04 | An exclusion keeps the coverage denominator. | (b) | G1's analysis has 10 counted occurrences, 3 of them `example:ex:1`, which is Rich Known. Ready counts. | Exclude one `ex:1` occurrence. | `ex:1` counts 2. The coverage denominator stays 10, and covered occurrences are 2 of 10. | The denominator never becomes 9. The excluded occurrence is not Known, not Reserved, and not in any later snapshot's occurrence list. |
| LRN-05 | A pending projection blocks cross-Book selection, never counted as zero. | (b) | G1 has `example:ex:1` ×2 and nothing Known. Classical Book G2 in `grc` has readiness `pending`. | Start G1. | Rejected with the pending (Updating) error. No current reading or snapshot exists. | When every G1 candidate has 3 or more in-Book occurrences or is Known, the same Start succeeds while G2 is pending, and the snapshot records no contributing Books. |
| LRN-06 | An unavailable projection reports unavailable, never zero. | (a) | As LRN-05, but G2's rebuild job exhausts its attempts in River. | Request Start for G1 through the Reading handler. | The response shows the unavailable recovery state. Nothing is written. | After a successful rebuild of G2, the same Start succeeds and G2's counts are in the library total. |
| LRN-07 | 3/15: two in the Book with 15 in total qualifies; 14 does not. | (b) | G1 has `example:ex:1` ×2. G2 has it ×13. Both ready. | Start G1. | The snapshot includes `ex:1` with in-Book 2, library total 15, route `library_total`. The snapshot records G1 and G2, with their analysis IDs, as the contributing Books. | With G2 ×12 (total 14), `ex:1` is not in the snapshot. |
| LRN-08 | 3/15: three in the Book qualifies at any total; zero never does. | (b) | G1 has `example:ex:1` ×3 and no other Book contains it. G2 has `example:ex:2` ×100 and G1 has none. | Start G1. | `ex:1` is in the snapshot with route `in_book` and no library total. | `ex:2` is not in the snapshot. |
| LRN-09 | 3/15: one in the Book with 15 in total qualifies; 14 does not. | (b) | G1 has `example:ex:1` ×1. G2 has it ×14. Both ready. | Start G1. | `ex:1` is in the snapshot with in-Book 1 and library total 15. | With G2 ×13, `ex:1` is not in the snapshot. |
| LRN-10 | Known and Reserved identities are excluded from 3/15. | (b) | `example:ex:1` ×5 is Rich Known. G1 is the current reading and holds `example:ex:2`. G2 has both ×5. | Preview a correction on G2 through lemma review impact. End G1, then Start G2. | The impact preview reports `ex:1` as Known and `ex:2` as Reserved, not as new candidates. After End, G2's snapshot includes `ex:2` and still excludes `ex:1`. | A legacy `xx` `λέξισ` NOUN Known row does not exclude either (§6 rule 1). |
| LRN-11 | `grc` and `el` are isolated. | (b) | Owner A has an `el` Book E with `λέξισ` NOUN ×5 and an `el` current reading, and `el` Known `λέξισ` NOUN. G1 in `grc` has `λέξις` NOUN (null discriminator) ×3. | Start G1 in `grc`, then Finish it. | Both readings are current at once. G1's snapshot includes the `grc` identity. Finish writes only `grc` Rich Known. | The `el` reading, snapshot, Known and counts are unchanged. The `grc` library total excludes E, and the `el` totals exclude G1. |
| LRN-12 | Two owners are isolated. | (b) | Owners A and B each have a Book on the same shared analysis X. B has `example:ex:1` Known and an exclusion on `(X,4)`. | A reads counts, Starts and Finishes. | A's counts include `(X,4)`, A's snapshot includes `ex:1`, and A gets Rich Known for it. | B's decision, Known and counts are unchanged. A's request naming B's Book is not found. |
| LRN-13 | A frozen snapshot is unchanged by new counts, a new analysis or a correction. | (b) | A Started G1: snapshot S holds `example:ex:1` (in-Book 3) and `example:ex:2` (2, total 15). | (1) Adopt a new analysis for G2 that changes its counts. (2) Adopt analysis Y for G1. (3) Request a correction on G1. (4) End, correct `(X,4)`, Start again. | S's members, counts, totals, headwords, occurrence lists, analysis ID and contributions are unchanged after every step. Step 4 creates a new snapshot from Y. | Step 3 is rejected and writes nothing. |
| LRN-14 | Legacy `de` snapshots, Known and history behave identically (characterization). | (b) | Every existing `de` store-contract scenario. A `de` Book with a legacy snapshot, Known and history recorded before the change. | Run the existing scenarios. Run LRN-01 for `grc` in the same store. | The `de` scenarios pass unchanged. The recorded `de` snapshot rows, Known rows and history rows are unchanged. | No rich row is written for a `de` Start or Finish. No tuple Known or tuple snapshot row is written by the `grc` run. |
| LRN-15 | Finish with the card state unavailable still completes, and cards never make anything Known. | (a) | A's current reading is classical G1. A deck preparation from its snapshot is queued and its River job is discarded. | Finish through the Reading handler. | The completion is recorded, Rich Known equals the snapshot minus what was already Known, G1 returns to Inbox and the reading ends. | Before Finish, the queued and failed preparation added no Rich Known. The preparation's state is not changed by Finish. |
| LRN-16 | A snapshot is exactly one of legacy or rich, and its membership is immutable and tied to its analysis. | (d) | A legacy snapshot L and a rich snapshot R for owner A. | Insert a snapshot with both representations, and with neither. Insert a rich member into L, a tuple row into R, a member of R naming another analysis, and a member of R in a later transaction. Update a member of R. | Every write is rejected and R is unchanged. | Inserting a legacy snapshot with its five evidence columns and its tuple rows is accepted exactly as today. |
| LRN-17 | Decision integrity. | (d) | A's classical Book G1 with analysis X, a punctuation node 6, and an EPUB Book E. | Insert two decisions for `(A, G1, X, 4)`; a `correct` without an identity; an `exclude` with one; a decision on node 6; a correction whose identity has another UPOS or a discriminator; a decision on E. | Every insert is rejected. | One `correct` decision with a matching null-discriminator identity on `(A, G1, X, 4)` is accepted. |
| LRN-18 | A decision is written only on the Book's current analysis; a stale confirm writes nothing. | (a) | A previews a correction on `(X,4)` through the lemma review page. §11 then adopts Y for G1. | Confirm the preview. | The stale response is shown. No decision row and no count change exists. | The same confirm before the adoption succeeds. |
| LRN-19 | No second state machine: rich readings follow the same transitions and stale-request rules. | (b) | Classical G1 and G2, To Read. | Start G1; Switch to G2; End; Start G1; Finish. Repeat each with a stale expected Book or snapshot, and replay each accepted request. | The accepted sequence gives the same verdicts and replays as the `de` lifecycle scenarios, and each snapshot is rich. | Each stale request is rejected and changes no row. A replayed Finish returns the recorded completion and adds no Known. |
| LRN-20 | The current reading refuses decisions on its Book. | (b) | G1 is the current reading. | Confirm an exclusion on `(X,4)`. | Rejected with the current-reading error. | No decision, flag resolution or count change is written. |
| LRN-21 | Only contributing Books add to the library total. | (b) | `grc`, owner A: G1 current; G2 Hidden Inbox; G3 Read; G4 with an offering only; package bytes for a unit with no binding; G6 whose analysis carries a vocabulary Usability restriction; a legacy `grc` EPUB Book L with `λέξισ` NOUN tuples. Owner B's G5. Each has `example:ex:1` or `λέξισ` occurrences. G1 has `ex:1` ×1. | Start G1. | The library total is the sum over G1, G2 and G3, and the snapshot records exactly those as contributing Books. | G4, the unbound bytes, G5, G6 and L add nothing and, while pending or failing, do not block the Start. |
| LRN-22 | Exact flags gate Start and are independent per occurrence. | (b) | An unresolved flag on `(X,4)` and none on `(X,5)`, which share a span. | Start G1. Confirm `keep` on `(X,4)`. Save the same flag again. Start G1. | The first Start is rejected for unresolved flags. `keep` resolves the flag and stores no decision. The second Start succeeds. | Saving the flag again does not reopen it. `(X,5)` has no flag throughout. |
| LRN-23 | Disposition, visibility and previously-read import write no rich learner state. | (b) | G2 is a ready contributing Book. G3 is an Inbox classical Book with no history. | Hide G2 and move it to Inbox. Import G3 as previously read. | The library total and G2's readiness are unchanged. G3 gets one completion with no snapshot. | No decision, count, snapshot or Known state changes. Stale revisions are rejected as today. |
| LRN-24 | Rich Known is immutable, set-valued and backed by frozen membership. | (d) | A has Rich Known `example:ex:1` from completion C. | Update any column. Insert `ex:1` again for A. Insert a row with no matching snapshot member. Insert a row whose language differs from its identity's. | Each is rejected or inserts nothing; the row is unchanged. | Inserting `ex:1` for owner B from B's own completion is accepted. |
| LRN-25 | Rich identities never reach the Browse-selection or Custom-deck stores (OD-01). | (a) | Owner A with classical Books. | Run Start, decisions, Finish and Browse for `grc` through the handlers. | The tuple-keyed Browse-selection and Custom-deck stores have no `grc` row for A. | The existing `de` Browse-selection characterization still reads its rows. |
| LRN-26 | Counts follow the current analysis and stay exact after decisions. | (b) | G1 is ready under X with `example:ex:1` ×3. | Confirm a correction of one `ex:1` occurrence to `ῥῆσις`. Then adopt Y for G1 without building counts. Start G2, which needs G1's total. | After the correction, readiness is still ready, `ex:1` is 2 and `ῥῆσισ` NOUN is 1. After the adoption, G1 is pending and G2's Start waits. | X's counts are never reported for G1 once Y is current. |

## 16. Requirement traceability

_Pending: owned by the public and local material ticket._

## 17. Open decisions

Any ticket may append rows. Owner tickets resolve their rows. Settled
decisions (see §1) are not reopened here.

| ID | question | options | recommendation | consequence | raised by | status |
| --- | --- | --- | --- | --- | --- | --- |
| OD-01 | Must rich identities reach the tuple-keyed Browse selection and Custom deck stores (B-17), whose UI is being retired? | (1) Greek identities are never written there; a test asserts it. (2) Add a discriminator to those stores. | (1), decided by #2069. §8 is written against it, and LRN-25 asserts it. | (1) adds no structure, but a Greek identity cannot be selected for a Custom deck. (2) changes stores outside the Reading path for a retiring UI. | #2064 | open |
| OD-02 | Where is the generic casefold fallback rejected for `grc` (B-18, B-19)? | (1) The registry refuses `grc` unless a dedicated profile is registered, for both `For` and `Lookup`. (2) Only the shared validation interface rejects casefold-normalized `grc` identities. | (1), decided by #2067. §6 is written against it: `grc` is a dedicated-profile language, and every other unregistered language keeps the fallback. | (1) fails closed for every caller, including lemma review, but changes registry behaviour for an unregistered language. (2) leaves other callers able to normalize `grc` with casefold. | #2064 | open |
| OD-03 | How does Reading eligibility treat an occurrence whose optional syntax is unavailable, given that `OccurrenceEligible` reads the dependency (B-10)? | (1) Unavailable syntax never makes an occurrence ineligible; syntax-based exclusions apply only when syntax is present. (2) Language-specific eligibility rules come from the canonical interpretation. | (1). §7 is written against it: a syntax-based exclusion applies only where the occurrence's basic syntax is usable. #2071 (§10) owns the eligibility interface that applies it. | (1) keeps one rule but needs explicit "unavailable" semantics. (2) moves eligibility into producer data. | #2064 | open |
| OD-04 | How do store-contract scenarios cover Greek when the harness is fixed to `de` and seeds only tuples (B-30)? | (1) Parameterize the scenario language and extend the seeds with discriminators, span references and optional syntax. (2) Add a separate Greek scenario set beside the existing one. | (1), decided by #2071. | (1) runs every existing scenario against rich identities, but changes the harness interface for both adapters. (2) duplicates scenarios and lets them drift. | #2064 | open |
| OD-05 | Does Evidence adoption publish through the existing analysis-run anchor that every Reading gate checks (B-23 to B-26), or through a new anchor? | (1) Adoption produces an exact analysis that the existing classification recognizes, so the six call sites stay unchanged. (2) A new adopted-analysis anchor, with every gate changed together. | Decided by #2068 and #2072, with §8 compatibility. | (1) reuses existing anchors, as the handoff requires, but `book_current_analyses` references a source material and run. (2) needs an atomic change across all gates. | #2064 | open |
| OD-06 | Are Work, Edition and Reading unit records global or owner-scoped (§4)? | (1) All shared. (2) All owner-scoped. (3) Shared when created from a reviewed identity definition; private to one owner when derived only from that owner's unverified catalogue advertisement. | (3). §4 is written against it. | (1) puts one learner's connection-supplied descriptions in rows other owners can resolve to. Sync then needs a write ban that the schema cannot express. (2) gives the simplest authorization (composite owner keys everywhere), but every owner duplicates descriptions, and §5 evidence cannot cite a shared unit. (3) keeps reviewed identity and its evidence shareable, and lets no learner input reach another owner. Its cost is a nullable `owner_id` and a trigger checking that associated units are shared or same-owner. It narrows issue item 7's "shared descriptions" for advertised rows. | #2065 | open |
| OD-07 | Does a Reading unit need its own stable external citation scheme field? | (1) The unit stores an internal reviewed `citation_scheme` name with its bounds. External citation identifiers, such as a CTS passage URN, are verified aliases only. (2) Add a stable external citation identifier column to the unit. (3) No scheme field; bounds are opaque text. | (1). | (1) makes `extent_key` comparable within a scheme without trusting external identifiers. (2) makes an external alias a de facto key, against ADR 0093. (3) cannot tell `1` in one scheme from `1` in another, so uniqueness on (Edition, extent) is unsafe. | #2065 | open |
| OD-08 | How does the operator-local path fit `CONTEXT.md:355-357` ("Books … enter the library only through a connected catalog, never by manual entry") and the closed provenance values at `internal/domain/books.go:14-19` and `:252-256`? | (1) A reviewed, compiled definition is an offering origin. Add a provenance value such as `operator_local`, and amend the `CONTEXT.md` wording in a separately reviewed change. (2) Require a synthetic local connection row, so that every Book still has a connection. | (1). Final interface decided by #2073. | (1) keeps the path free of connections, as #2065 requires, but needs a glossary change outside this ticket. (2) keeps the wording but invents a connection that has no feed, credentials or sync, and would be visible in Catalogs. | #2065 | open |
| OD-09 | Where do classical catalogue connections live, given that `opds_connections.owner_id` is nullable and has no unique `(owner_id, id)` (`migrations/000001_initialize.up.sql:1267-1277`, `:1960`)? | (1) Generalize `opds_connections` with a connection kind, and add a unique `(owner_id, id)`. (2) A separate classical connection record, owner-scoped with `(owner_id, id)` unique. | (2), decided in the spec for handoff stage 3 ([catalog release and durable acquisition](annotation-backed-ancient-greek.md#3-authorize-catalog-release-and-implement-durable-acquisitionadoption)). The local route needs neither. | (1) shares one Catalogs list but carries the OPDS single-`language` and `url` columns, which ADR 0096 says classical data must not be forced through. (2) lets a Unit offering use a composite owner and connection key, but adds a second connection kind to Catalogs. | #2065 | open |
| OD-10 | Does later sync update a classical Book's `title`, `author` and `language`? | (1) They are set once at creation from the Work. Advertised metadata is stored on the Unit offering, and display reads Work, Edition and unit descriptions. (2) Follow OPDS: sync overwrites them, and a language change ends an incompatible Current reading (`internal/persistence/books.go:832-847`). | (1). | (1) means sync can never end a reading or rename a Book shared by several offerings. A changed description reaches the Book only through a reviewed definition. (2) lets two offerings of one Book overwrite each other. | #2065 | open |
| OD-11 | New terms used in §4. | Add to `CONTEXT.md`: **Unit offering**, **Local registration**, **Verified identity alias**, **Claimed alias**, **Identity conflict**, **Edition reconciliation**, **Reconciliation class**, **Book–Reading unit association**, **Named Edition** / **Unidentified Edition**, and **classical Book** (a Book with a Book–Reading unit association). Or keep them as design-local names. | Add them in a separately reviewed `CONTEXT.md` change. **Unit offering** is defined as one declared Reading unit that a Catalog entry offers. | Adding them gives implementation and UI one vocabulary. Keeping them local leaves "offering" ambiguous: `CONTEXT.md:273-274` already calls a Catalog entry "an offering", and "alias" is ambiguous against the OPDS `catalog_entry` alias. | #2065 | open |
| OD-12 | How does an Exact analysis relate to `analysis_runs` and `book_current_analyses` (§5)? | (1) Extend additively: make `book_current_analyses.source_material_id` and `analysis_run_id` nullable, and add a nullable classical-analysis column with a check that exactly one side is set. (2) A parallel owner-scoped "Book current classical analysis" record keyed `(owner_id, book_id)` and referencing the §4 association, plus an additive trigger on `book_current_analyses` that rejects classical Books. | (2). §5 is written against it. | (1) keeps one physical anchor, but it relaxes `NOT NULL` columns and foreign keys on a shipped table (`migrations/000001_initialize.up.sql:239-245`, `:2613`, `:2629`). It also changes the meaning of `current_analysis_identity` (`:433-444`), which code and queries reference in 61 places, and of the upsert at `internal/analysis/jobs.go:1736-1739`. (2) leaves every EPUB structure, view and query unchanged. Its cost is that the single domain classification must read two anchors, which is OD-05's gate question, and a cross-table exclusion trigger. | #2066 | open |
| OD-13 | Which identifier fills the source-decoding slot for the retained candidate? The handoff names lemma, POS, repair, terminal and normalization identifiers, but no decoding identifier. | (1) Source decoding is a separate slot, and the Stage 1 reviewed definition names its own `name/version` identifier. (2) Treat the decoding as part of `oga-greta-lemma/2` and fill the slot with that identifier. | (1). | (1) keeps decoding revisions visibly separate from mapping and normalization, as the #2033 resolution requires, but needs one more reviewed public identifier at Stage 1. (2) adds no identifier, but a decoding-only change then cannot be versioned without also versioning the lemma mapping. | #2066 | open |
| OD-14 | Is the selection policy (`grc-earliest-capped/1`) part of the analysis-affecting interpretation identity? Issue #2066 item 6 lists it there. The handoff says a card-selection policy change applies to a newly prepared specification, not to an analysis rewrite. | (1) Pin it in the same reviewed definition, but keep it out of the Interpretation configuration. It is recorded with the specifications it governs (§8). (2) Include it as a slot, so a policy change creates a new interpretation and analysis. | (1). §5 is written against it. | (1) matches the handoff, and a policy change needs no re-adoption or new counts. (2) forces a new analysis, adoption and count projection for a change that alters no occurrence. That contradicts the handoff. | #2066 | open |
| OD-15 | Where does the private retained pin set live, including transport, content and member digests and the exception-table digest? | (1) Compiled into reviewed application code, which publishes the private digests in the public repository. (2) A reviewed operator-local pin file, loaded through the same pin-set interface. Its identifier and version are recorded on every acquisition. (3) Synthetic pin sets in code for CI, and the retained pin set operator-local. | (3), decided by #2075 (§14) with #2073 (§12). | (1) gives one review path, but exposes values this design keeps private. (2) keeps them local, but the file's review happens outside the repository. (3) keeps CI public and synthetic and the retained values local, at the cost of two pin sources behind one interface. All three keep the rule that pins never come from the package. | #2066 | open |
| OD-16 | Are the package bounds (16 MiB, 128 members, 64 MiB) configurable above the handoff values? | (1) The application enforces configured values that default to the handoff values and can only be lowered. The database checks the handoff values as ceilings, and raising one needs a reviewed change. (2) Fully configurable at runtime, with no database ceiling. (3) Fixed constants. | (1). | (1) follows "Deliberately review any required limit change" (handoff Stage 1) and keeps a backstop against application bugs. (2) lets an operator setting silently admit larger packages, with no backstop. (3) contradicts "configurable engineering defaults". | #2066 | open |
| OD-17 | Can a Text–annotation binding reference a private (advertised-only) Reading unit? | (1) No. Bindings reference shared units only, and the Stage 3 spec decides private-unit evidence. (2) Yes. A binding inherits the unit's `owner_id`, and private bindings and analyses cascade on owner deletion. | (1). | (1) keeps every evidence row shared and immutable, and is enough for the local path, which uses reviewed definitions and so shared units (§4 OD-06). It defers evidence for unverified advertised units. (2) supports that case now, but adds owner-scoped cascades to otherwise shared immutable evidence. | #2066 | open |
| OD-18 | New terms used in §5. | Add to `CONTEXT.md`: **Package acquisition** (an owner's verified acquisition of exact package bytes; their provenance and evidence access), **Capability diagnostic** and **Usability restriction**. Keep Package blob, Package content (source-content identity, as against a blob's transport identity), Package member, pin set, Evidence notice, Notice coverage, Extracted text, Interpretation configuration, Exact analysis, Diagnostic finding, Provenance claim and Book current classical analysis as design-local names. "Owner evidence access" is the role that Package acquisition plays, not a separate record. | Add the three in a separately reviewed `CONTEXT.md` change. | The three appear in learner-facing explanations and in other sections. The rest are storage detail. "Exact analysis" stays design-local, so that it does not compete with the existing "analysis". | #2066 | open |
| OD-19 | Can a Usability restriction be lifted? | (1) Append-only lift records are allowed only for `integrity_failure`, after the exact bytes are restored and re-verified. A `confirmed_defect` is never lifted; recovery is a new validated analysis. (2) No lift. Every recovery is a new analysis and adoption. | (1), specified with §9 and §11. | (1) lets an exact restore return evidence to use without rewriting history, as ADR 0097 "Durable bounded work and recovery" requires. (2) makes a restored, byte-identical package permanently unusable. | #2066 | open |
| OD-20 | Is a Usability restriction installation-wide, or per owner? | (1) Installation-wide: a restriction on shared evidence applies to every owner whose Book uses it, and each affected owner is notified. (2) Per owner: the operator records it for chosen owners. | (1). §5 is written against it. | (1) follows ADR 0097: "an integrity failure or confirmed binding/interpretation defect records an explained usability restriction … notify affected owners". A defect in shared bytes is a property of the bytes, not of one owner's choice. It changes no owner's adoption, which ADR 0097 "Alice's refresh cannot switch Bob" protects. (2) lets known-defective evidence stay usable for owners who were not named. | #2066 | open |
| OD-21 | Is the rich vocabulary identity an interned record with a surrogate ID, or a composite value carried in each consumer (§6)? | (1) One shared, immutable, interned Vocabulary identity table, unique `NULLS NOT DISTINCT` on the five values. Consumers carry its `uuid`. (2) The same, but owner-scoped: one interned row per owner. (3) A composite value: each consumer carries language, lemma, UPOS, scheme and value. | (1). §6 is written against it. | (1) gives each consumer one non-null 16-byte reference and single-column indexes. The nullable comparison happens once, in one unique index. Its costs are a join to display a lemma and an insert-if-absent step. A row interned by one learner's Lemma correction holds only a normalized lemma and UPOS, has no owner link, and stays after that owner is deleted. (2) removes that residue, but shared Exact analysis rows (§5) would then need a second, shared identity reference. (3) needs no join, but widens every consumer key and index by roughly 40–70 bytes. Every join must then compare nullable columns, where `NULL = NULL` silently drops matches. That is the merge or wildcard defect this design forbids. | #2067 | open |
| OD-22 | Where does the citation headword live, and what is shown when analyses disagree (§6)? | (1) The decoded citation form per occurrence (§7), plus one Analysis headword per (Exact analysis, identity), chosen deterministically. Frozen records freeze the headword they showed. (2) One headword per (identity, Interpretation configuration). (3) A headword column on the identity row. | (1). | (1) keeps every Book-scoped view exact to its own analysis, and keeps frozen records stable. It needs a cross-Book display rule, which §6 gives. (2) is smaller, but two Books under one interpretation whose texts spell the citation form differently would share one headword. (3) makes the first analysis's spelling permanent for every Book and owner, so a later analysis could never show its own. | #2067 | open |
| OD-23 | Who may set a discriminator, or change UPOS, in a Lemma correction (§6)? Issue #2067 item 7 says corrections "may change lemma or UPOS". Today they change the lemma only (`internal/selection/selection.go:264-274`). | (1) Learners never set, change or clear a discriminator, and never change UPOS. A correction to a different key gives a null discriminator. Re-entering the same key is `keep`. Only a reviewed interpretation sets discriminators or changes UPOS, as a new Exact analysis. (2) Learners may choose among `(scheme, value)` pairs already attested for the same key and UPOS in the same Exact analysis. (3) Learners may type a discriminator. | (1). | (1) matches today's lemma-only corrections and "no learner POS editing". A learner who thinks the analysis chose the wrong homograph can exclude the occurrence or correct it to a different lemma, but cannot pick the other homograph. (2) allows that choice, but the reading increment shows no glosses that tell `ex:1` from `ex:2`, so the choice would be uninformed. It also needs a new correction UI. (3) lets free text create identities in reviewed schemes, against the #2033 rule that discriminators come from documented schemes. | #2067 | open |
| OD-24 | What happens if the §13 preflight finds legacy `grc` tuple rows (§6)? | (1) Isolate and continue. The rows stay unchanged and readable, match no rich `grc` identity (comparison rule 5), are reported, and are never reconciled without a separately approved plan. Lemma review on their legacy analysis is unavailable. (2) Apply the plain legacy rule (equal lemma, UPOS and null discriminator). (3) Block classical adoption for affected owners until a reconciliation plan is approved. | (1). | (1) never transfers Known across a distinction. The cost is that an affected owner's legacy `grc` Known words show as unknown in classical Books. (2) is unsafe: a casefold key expands iota subscripts, so a legacy subscript row byte-equals the `ancient-greek/1` key of the adscript spelling, and Known would transfer across that distinction. (3) is safe, but it adds a learner-data gate to §11 adoption for a case that §3 expects never to occur. | #2067 | open |
| OD-25 | How does `ancient-greek/1` survive a toolchain whose Unicode version is not 17.0.0 (Go, `golang.org/x/text`, or a future Python implementation, where Python 3.13 has 15.1.0)? | (1) The implementation asserts 17.0.0. A toolchain change fails that test, and a reviewed change may keep `ancient-greek/1` only if the full fixture plus an exhaustive per-code-point lowercase and NFC table for every code point assigned in 17.0.0 are unchanged. Admission keeps rejecting code points unassigned in 17.0.0, through a frozen table. Otherwise the change needs `ancient-greek/2`. (2) Ship frozen 17.0.0 normalization and case tables with the profile, independent of the toolchain. (3) Follow the toolchain's Unicode version silently. | (1). | (1) keeps one definition and catches drift in review, at the cost of a generated check per toolchain upgrade. (2) is fully reproducible, but carries and maintains copied Unicode tables. (3) can change keys, and therefore Known matches, without review, against the #2033 rule that Unicode changes that can alter identity be reviewable. | #2067 | open |
| OD-26 | How is the `grc` Concordance **form** search key computed (§6 I-6)? Today SQL computes it as `translate(lower(o.surface), 'ς', 'σ')` (`sqlc/queries/concordance.sql:115`), with PostgreSQL's unpinned `lower()` and no NFC on stored surfaces. | (1) Go computes an `ancient-greek/1` search key for each occurrence surface, which §7 stores with the occurrence, and SQL compares bytes. The key is for search only, never identity. (2) Keep the SQL expression for `grc`. | (1), with §7 placing the stored key. | (1) gives exactly the accommodation #2033 allows for search (NFC and NFD, case, sigma), from the pinned implementation. Its cost is one stored column per occurrence. (2) depends on the database's collation provider and Unicode tables, and an NFD surface never matches an NFC query. | #2067 | open |
| OD-27 | New terms used in §6. | Add to `CONTEXT.md`: **Rich vocabulary identity** (language, canonical lemma, UPOS and an optional scheme-qualified lexical discriminator), **Lexical homograph discriminator** (the #2033 term), **Citation headword** (the decoded citation form shown to learners, distinct from the canonical lemma key) and **Source decoding** (the versioned adapter step from native lemma data to a decoded citation form and discriminator). Keep Vocabulary identity (the interned record), Analysis headword, Discriminator scheme and dedicated-profile language as design-local names. Extend the existing **Effective vocabulary identity** entry to name the rich identity. | Add them in a separately reviewed `CONTEXT.md` change. | The four appear in learner-facing explanations, corrections and other sections. Without them, "lemma" stays ambiguous between key, headword and native value. The rest are storage detail. | #2067 | open |
| OD-28 | How do new Exact analyses coexist with legacy `corpus_tokens` and `corpus_sentences` analyses (§7)? | (i) New analyses write only the §7 records. Every consumer reads both through one analysis read interface, with an exact adapter and a legacy adapter. The legacy adapter maps `corpus_tokens` rows into the same shape, with offset-keyed occurrences and no discriminator. (ii) New analyses dual-write: the §7 records plus `corpus_tokens` and `corpus_sentences` rows. | (i), with no dual-write. §7 is written against it. | (i) leaves every legacy row and the EPUB write path untouched, and keeps one source of truth per analysis. Its cost is changing every content consumer to read through the interface at once, guarded by a characterization test (OCC-11). (ii) keeps legacy consumers working unchanged, but `corpus_tokens` cannot hold unprinted nodes, shared-span occurrences, discontinuous spans or absent syntax without invented values (B-01, B-02, B-03). It would also store two copies that could disagree, and those rows are owner-scoped while Exact analyses are shared (§5). | #2068 | open |
| OD-29 | Does the legacy adapter re-validate legacy dependency trees with the §7 tree rules (TREE-1 to TREE-4)? `validateCorpusToken` never checked one root per sentence, cycles or connection (`internal/analysis/jobs.go:1849-1862`). | (1) No. The legacy adapter reports legacy basic syntax as `usable`, exactly as consumers read it today. (2) Yes. Legacy sentences that fail are reported `invalid` and withheld. | (1). | (1) keeps the legacy characterization exact (OCC-11). A legacy tree that would fail the §7 rules stays visible as it is today. (2) applies one standard everywhere, but changes current EPUB Concordance and sentence-study output without a reviewed migration, against "legacy analyses remain readable without mass conversion" (handoff Stage 2). | #2068 | open |
| OD-30 | Is a `non_vocabulary` word or component, such as a native-class conjunction, an Annotation occurrence? | (1) No. Occurrences are word or component nodes classed `vocabulary` or `unresolved`. Non-vocabulary nodes are accounted for but take no count and no learner decision. (2) Yes. Every text-attested word or component is an occurrence, and the accounting class decides whether it counts. | (1). §7 is written against it. | (1) makes the occurrence foreign key unable to name anything a learner cannot sensibly decide. A learner cannot correct a native-class conjunction into vocabulary; only a new reviewed interpretation can. (2) lets a learner correct or exclude such a node, but an exclusion of something that never counts has no effect, and a correction would override a reviewed classification, against #2058 item 1 ("not a learner Occurrence exclusion"). | #2068 | open |
| OD-31 | New terms used in §7. | Add to `CONTEXT.md`: **Non-vocabulary classification** (a source-supported, reason-coded accounting class that is not an Occurrence exclusion), **Multiword container** (a printed token whose linguistic components are counted instead of it) and **Unprinted node** (an annotation node with no text span, such as an empty node). Extend the existing **Annotation occurrence** entry to say it is identified by (Exact analysis, node), never by offsets. Keep Analysis sentence, Analysis node, Node span, Sentence span, Container component, accounting class, analysis read interface and the rule IDs as design-local names. | Add them in a separately reviewed `CONTEXT.md` change. | The three appear in learner-facing explanations of counts and coverage. Without them, "excluded" is ambiguous between a learner's Occurrence exclusion and a source-supported classification. The rest are storage detail. | #2068 | open |
| OD-32 | How does the existing snapshot row anchor a classical reading? `primary_goal_snapshots` requires five legacy evidence columns and references `analysis_runs` and `corpora` (`migrations/000007_primary_goal_snapshots.up.sql:19-23`, `:30-32`), which a classical Book does not have (§5 OD-12). | (1) A new migration drops `NOT NULL` from the five columns and adds a nullable `exact_analysis_id` with a check that exactly one representation is set. (2) The same relaxation, plus a separate 1:1 classical snapshot record and a deferred trigger requiring exactly one side. (3) A parallel classical snapshot and current-reading anchor. (4) Leave `primary_goals.snapshot_id` NULL for classical readings and point at a classical snapshot elsewhere. | (1). §8 is written against it. | (1) keeps one snapshot anchor, so `primary_goals`, `reading_history` and `deck_preparations` keep their snapshot references, and a row check states the representation. It relaxes constraints on a shipped table, the kind of change OD-12 avoided. No existing row changes, because all of them satisfy the legacy branch. Queries that read the five columns directly (`sqlc/queries/current_reading.sql:109-119`, `:271-333`) must run only for legacy snapshots; `GetCurrentReading` already coalesces them (`:12-17`). (2) gives the same result with a cross-table trigger instead of a row check. (3) is a second learner state machine, which ADR 0098 forbids. (4) makes a classical reading look like the legacy no-snapshot shape, which Finish accepts with nothing to accept (`internal/persistence/current_reading.go:236-240`, `internal/domain/current_reading_finish.go:90-107`). | #2069 | open |
| OD-33 | Do Rich Known rows affect legacy (tuple) selection? §6 defines equality between a legacy tuple and a null-discriminator, non-`grc` rich identity. | (1) No. Legacy selection reads only `known_vocabulary`. Rich selection reads Rich Known plus the legacy rows that equal its candidates under §6. (2) Both directions apply the §6 equality. | (1). | (1) guarantees that no legacy outcome changes (LRN-14). It is unobservable today, because the only rich language is `grc`, and §6 rule 5 keeps `grc` legacy and rich apart. A future non-`grc` classical language needs this revisited before its Known crosses representations. (2) is symmetric but makes legacy selection read a new record for no current case. | #2069 | open |
| OD-34 | Is rich count readiness stored as an explicit state, or derived from River job rows as the legacy library-total query does (`internal/persistence/current_reading_lifecycle.go:197-219`)? | (1) Explicit `pending`, `ready` and `unavailable`, written by adoption, decisions and the rebuild job; the job writes `unavailable` when it exhausts its attempts. (2) Readiness row presence plus River job state, as today. | (1). §8 is written against it. | (1) makes the readiness state a persisted fact the fixture adapter can represent directly, and drops the JSON match on job arguments. It needs the job's last attempt to write the state, and a discarded job that never reaches that path leaves `pending`. (2) reuses a proven query, but couples the learner core to River's table layout. | #2069 | open |
| OD-35 | Is the frozen snapshot headword stored per membership row, or derived when read? §6 asks §8 to place it. | (1) Stored on Rich snapshot membership and copied to Rich Known at acceptance. (2) Derived from the snapshot's analysis and the Book's decisions at read time. | (1). §8 is written against it. | (1) keeps the headword frozen even if a decision on the old analysis is later read differently, and lets the Known list show the accepted headword without a join through analyses. It costs one text column per row. (2) stores nothing, but makes a frozen display depend on mutable decisions. | #2069 | open |
| OD-36 | New terms used in §8. | Add to `CONTEXT.md`: **Snapshot representation** (legacy tuple or rich, fixed when a snapshot is frozen) and **Library total** (an identity's effective count over the owner's contributing Books in one language, the 15 of the 3/15 rule). Extend **Known vocabulary** and **Current-reading vocabulary snapshot** to say they hold rich identities for classical Books. Keep Exact occurrence decision, Exact Lemma review flag, Rich count readiness, Rich vocabulary count, Rich snapshot membership, Snapshot count contribution and Rich Known vocabulary as design-local names. | Add them in a separately reviewed `CONTEXT.md` change. | The two terms appear in learner-facing explanations of why a word was selected and why Start is waiting. The rest are storage detail. | #2069 | open |

## 18. Approval record

_Pending: owned by the approval ticket._

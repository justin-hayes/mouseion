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

_Pending: owned by the immutable evidence ticket._

## 6. Vocabulary identity and normalization

_Pending: owned by the vocabulary identity ticket._

## 7. Canonical analysis and exact occurrences

_Pending: owned by the canonical analysis ticket._

## 8. Learner-state attachment and compatibility

_Pending: owned by the learner-state attachment ticket._

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

## 16. Requirement traceability

_Pending: owned by the public and local material ticket._

## 17. Open decisions

Any ticket may append rows. Owner tickets resolve their rows. Settled
decisions (see §1) are not reopened here.

| ID | question | options | recommendation | consequence | raised by | status |
| --- | --- | --- | --- | --- | --- | --- |
| OD-01 | Must rich identities reach the tuple-keyed Browse selection and Custom deck stores (B-17), whose UI is being retired? | (1) Greek identities are never written there; a test asserts it. (2) Add a discriminator to those stores. | (1), decided by #2069. | (1) adds no structure, but a Greek identity cannot be selected for a Custom deck. (2) changes stores outside the Reading path for a retiring UI. | #2064 | open |
| OD-02 | Where is the generic casefold fallback rejected for `grc` (B-18, B-19)? | (1) The registry refuses `grc` unless a dedicated profile is registered, for both `For` and `Lookup`. (2) Only the shared validation interface rejects casefold-normalized `grc` identities. | (1), decided by #2067. | (1) fails closed for every caller, including lemma review, but changes registry behaviour for an unregistered language. (2) leaves other callers able to normalize `grc` with casefold. | #2064 | open |
| OD-03 | How does Reading eligibility treat an occurrence whose optional syntax is unavailable, given that `OccurrenceEligible` reads the dependency (B-10)? | (1) Unavailable syntax never makes an occurrence ineligible; syntax-based exclusions apply only when syntax is present. (2) Language-specific eligibility rules come from the canonical interpretation. | Decided by #2068 together with #2071. | (1) keeps one rule but needs explicit "unavailable" semantics. (2) moves eligibility into producer data. | #2064 | open |
| OD-04 | How do store-contract scenarios cover Greek when the harness is fixed to `de` and seeds only tuples (B-30)? | (1) Parameterize the scenario language and extend the seeds with discriminators, span references and optional syntax. (2) Add a separate Greek scenario set beside the existing one. | (1), decided by #2071. | (1) runs every existing scenario against rich identities, but changes the harness interface for both adapters. (2) duplicates scenarios and lets them drift. | #2064 | open |
| OD-05 | Does Evidence adoption publish through the existing analysis-run anchor that every Reading gate checks (B-23 to B-26), or through a new anchor? | (1) Adoption produces an exact analysis that the existing classification recognizes, so the six call sites stay unchanged. (2) A new adopted-analysis anchor, with every gate changed together. | Decided by #2068 and #2072, with §8 compatibility. | (1) reuses existing anchors, as the handoff requires, but `book_current_analyses` references a source material and run. (2) needs an atomic change across all gates. | #2064 | open |
| OD-06 | Are Work, Edition and Reading unit records global or owner-scoped (§4)? | (1) All shared. (2) All owner-scoped. (3) Shared when created from a reviewed identity definition; private to one owner when derived only from that owner's unverified catalogue advertisement. | (3). §4 is written against it. | (1) puts one learner's connection-supplied descriptions in rows other owners can resolve to. Sync then needs a write ban that the schema cannot express. (2) gives the simplest authorization (composite owner keys everywhere), but every owner duplicates descriptions, and §5 evidence cannot cite a shared unit. (3) keeps reviewed identity and its evidence shareable, and lets no learner input reach another owner. Its cost is a nullable `owner_id` and a trigger checking that associated units are shared or same-owner. It narrows issue item 7's "shared descriptions" for advertised rows. | #2065 | open |
| OD-07 | Does a Reading unit need its own stable external citation scheme field? | (1) The unit stores an internal reviewed `citation_scheme` name with its bounds. External citation identifiers, such as a CTS passage URN, are verified aliases only. (2) Add a stable external citation identifier column to the unit. (3) No scheme field; bounds are opaque text. | (1). | (1) makes `extent_key` comparable within a scheme without trusting external identifiers. (2) makes an external alias a de facto key, against ADR 0093. (3) cannot tell `1` in one scheme from `1` in another, so uniqueness on (Edition, extent) is unsafe. | #2065 | open |
| OD-08 | How does the operator-local path fit `CONTEXT.md:355-357` ("Books … enter the library only through a connected catalog, never by manual entry") and the closed provenance values at `internal/domain/books.go:14-19` and `:252-256`? | (1) A reviewed, compiled definition is an offering origin. Add a provenance value such as `operator_local`, and amend the `CONTEXT.md` wording in a separately reviewed change. (2) Require a synthetic local connection row, so that every Book still has a connection. | (1). Final interface decided by #2073. | (1) keeps the path free of connections, as #2065 requires, but needs a glossary change outside this ticket. (2) keeps the wording but invents a connection that has no feed, credentials or sync, and would be visible in Catalogs. | #2065 | open |
| OD-09 | Where do classical catalogue connections live, given that `opds_connections.owner_id` is nullable and has no unique `(owner_id, id)` (`migrations/000001_initialize.up.sql:1267-1277`, `:1960`)? | (1) Generalize `opds_connections` with a connection kind, and add a unique `(owner_id, id)`. (2) A separate classical connection record, owner-scoped with `(owner_id, id)` unique. | (2), decided in the spec for handoff stage 3 ([catalog release and durable acquisition](annotation-backed-ancient-greek.md#3-authorize-catalog-release-and-implement-durable-acquisitionadoption)). The local route needs neither. | (1) shares one Catalogs list but carries the OPDS single-`language` and `url` columns, which ADR 0096 says classical data must not be forced through. (2) lets a Unit offering use a composite owner and connection key, but adds a second connection kind to Catalogs. | #2065 | open |
| OD-10 | Does later sync update a classical Book's `title`, `author` and `language`? | (1) They are set once at creation from the Work. Advertised metadata is stored on the Unit offering, and display reads Work, Edition and unit descriptions. (2) Follow OPDS: sync overwrites them, and a language change ends an incompatible Current reading (`internal/persistence/books.go:832-847`). | (1). | (1) means sync can never end a reading or rename a Book shared by several offerings. A changed description reaches the Book only through a reviewed definition. (2) lets two offerings of one Book overwrite each other. | #2065 | open |
| OD-11 | New terms used in §4. | Add to `CONTEXT.md`: **Unit offering**, **Local registration**, **Verified identity alias**, **Claimed alias**, **Identity conflict**, **Edition reconciliation**, **Reconciliation class**, **Book–Reading unit association**, **Named Edition** / **Unidentified Edition**, and **classical Book** (a Book with a Book–Reading unit association). Or keep them as design-local names. | Add them in a separately reviewed `CONTEXT.md` change. **Unit offering** is defined as one declared Reading unit that a Catalog entry offers. | Adding them gives implementation and UI one vocabulary. Keeping them local leaves "offering" ambiguous: `CONTEXT.md:273-274` already calls a Catalog entry "an offering", and "alias" is ambiguous against the OPDS `catalog_entry` alias. | #2065 | open |

## 18. Approval record

_Pending: owned by the approval ticket._

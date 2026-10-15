# Ancient Greek additive analysis: schema and module-interface design

Status: **Changes requested (§18) — not approved. No SQL, migration or application implementation is authorized.** · Date: 2026-10-15

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

Status: **Changes requested (§18) — not approved. No SQL, migration or application implementation is authorized.** · Date: 2026-10-15

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
  `completion_id`, unique per `(owner_id, language, goal_snapshot_id)`, and it
  carries only per-completion counts.
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
| B-09 | Vocabulary identity is `selection.Identity{Language, CanonicalLemma, UPOS}`. The domain mirror is `domain.SnapshotIdentity`. | `internal/selection/selection.go:27`, `internal/domain/current_reading_finish.go:5-7` | **Blocks.** There is no lexical discriminator, so homographs such as the synthetic `λέξις` `example:ex:1` and `example:ex:2` merge. | §6 |
| B-10 | `selection.Effective` builds the identity from the trimmed lemma and upper-cased UPOS. `OccurrenceEligible` also rejects dependency `compound:prt`. | `internal/selection/selection.go:264-285` | **Constrains.** Eligibility reads syntax. When syntax is unavailable, the result depends on an empty dependency, which today cannot occur (B-03). | §6, §7 |
| B-11 | `known_vocabulary` is unique on `(owner_id, language, canonical_lemma, upos)`. | `migrations/000001_initialize.up.sql:1936` | **Blocks.** Known cannot distinguish discriminated identities. A legacy tuple row would match both descendants if read as a wildcard. | §8 |
| B-12 | `reading_history` primary key was `(owner_id, language, book_id)`. Per-completion counts and `goal_snapshot_id` were added later, and the primary key is now `completion_id`, with `(owner_id, language, goal_snapshot_id)` unique. | `migrations/000004_reading_history.up.sql:9`, `migrations/000009_primary_goal_completion_provenance.up.sql:6-22`, `migrations/000010_reading_history_append_only.up.sql:7-11` | **Does not block identity.** Reading completion is per snapshot. Its counts must be computed over rich identities. | §8 |
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
| One Reading unit that a connection-scoped Catalog entry offers, resolved to the owner's Book. | `id uuid`; `owner_id uuid`; `connection_id uuid`; `entry_namespace text`; `entry_value text`; `offering_key text` (adapter-declared stable unit designation within the entry); `declared_edition_key text` (adapter-declared within the entry); `reading_unit_id uuid`; `book_id uuid`; `advertised_title`, `advertised_author`, `advertised_bounds_label text`; `advertised_availability` (ADR 0096 metadata summary); `first_seen_at`, `last_seen_at`; `withdrawn_at NULL` | `id` | `(owner_id, id)` unique, as the Claimed alias reference target. `(owner_id, connection_id, entry_namespace, entry_value, offering_key)` unique. Several offerings, from one entry or several, may name the same unit and Book. | Owner. | Identity fields (`owner_id` through `book_id`) immutable. Advertised fields, `last_seen_at` and `withdrawn_at` are mutable by that owner's sync only. | `(owner_id, reading_unit_id, book_id)` → association `NO ACTION`; `connection_id` → the owner's catalogue connection `ON DELETE CASCADE` (OD-09); `owner_id` → `users(id)` `ON DELETE CASCADE` |

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
- An **Interpretation configuration** pins the analysis-affecting named versions,
  including the POS and morphology mappings, and the frozen exception table
  digest. The same set always resolves to the same row.
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
| The pinned set of analysis-affecting named versions that turns bound evidence into a Canonical interpretation. | `id uuid`; `source_decoding text`; `lemma_mapping text`; `pos_mapping text`; `morphology_mapping text NULL` (NULL = the definition declares no morphology mapping, not "unknown"); `repair_layer text NULL` (NULL = the definition declares no repair layer, not "unknown"); `reading_terminal text`; `normalization_profile text`; `exception_table_digest text NULL` (blob with `blob_kind = interpretation_table`; NULL only when the reviewed interpretation definition declares that it has no exception table; the retained candidate's definition requires one, #2058 item 3); `contract_version text` (canonical-contract version); `configuration_digest text` (SHA-256 of the canonical serialization `interpretation/1` of the seven slots, `exception_table_digest` and `contract_version`, with an explicit null marker for each NULL slot; never `id` or `created_at`); `created_at` | `id` | `configuration_digest` unique, and the tuple of every slot plus `exception_table_digest` and `contract_version` unique `NULLS NOT DISTINCT`. The same set therefore always gives the same row. | Shared. | **Immutable.** | `exception_table_digest` → Package blob `RESTRICT` |

Each slot holds a public identifier of the form `name/version`, checked against
`^[a-z0-9][a-z0-9-]*/[1-9][0-9]*$`. For the retained candidate, the handoff names
`oga-greta-lemma/2` (lemma mapping), `oga-agdt-upos/4` (POS mapping, including its
εἰμί rule), `oga-agdt-feats/1` (morphology mapping), `oga-anabasis1-research-repairs/2`
(repair layer), `oga-reading-terminal/2` (reading-terminal policy) and
`ancient-greek/1` (normalization profile). The handoff names no separate
source-decoding identifier (OD-13). The selection policy `grc-earliest-capped/1`
is pinned by the same reviewed definition but is **not** a slot here, because a
selection-policy change must not create a new analysis (OD-14).

Every mapping slot follows the handoff's "No treebank data" rule, and its
reviewed definition records its lineage. For the retained candidate:

- `oga-agdt-upos/4` is written independently from the AGDT tagset documentation
  and the UPOS definitions, and replaces the superseded `oga-agdt-upos/3`. Its
  εἰμί rule is OD-77.
- `oga-agdt-feats/1` maps the AGDT `postag` positions (person, number, tense,
  mood, voice, gender, case, degree) to UD FEATS, from the same documentation
  and the FEATS definitions. A native value that the documentation does not map
  stays only in `native_morphology` (§7). Stage 1 confirms that the core
  package's annotation members carry the tags. Only if they do not does the
  retained candidate declare no morphology mapping, with morphology
  `unsupported`. Morphology never affects Vocabulary-ready.

The **frozen exception-value/precondition table** (#2058 item 3) is referenced
only by `exception_table_digest`. Its bytes come from local material and are
verified against the digest pinned in the reviewed interpretation definition
before they are stored. Its values never appear in public documents, logs or
fixtures (§14).

**Exception-table changes.** The six POS repairs corrected the output of
`oga-agdt-upos/3`'s εἰμί rule, so they are dropped with it. The table keeps at
most 28 records: the 20 confirmed labels, the compound record and the seven
citation repairs. That change alone gives `oga-anabasis1-research-repairs/2` and
a new table digest. Before the table is frozen, Stage 1 audits the lineage of
each remaining record and records the documentation or retained source evidence
that supports it. Its per-record results are local material, and its public
report gives only counts per lineage class and outcome (§14). A value that
depends on a treebank witness is replaced by a value with permitted lineage or
dropped, through a further repair-layer version. Nothing is edited in place, and
nothing adds a repair. Errors of the new εἰμί rule are disclosed as a quality
limit, not repaired. A dropped value that leaves an occurrence unresolved is
reported by ACC-3 and returned as a Stage 1 obstacle (handoff Stage 1 "Stop"),
never filled by guessing.

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
  morphology_mapping text NULL,
  repair_layer text NULL, reading_terminal text NOT NULL, normalization_profile text NOT NULL,
  exception_table_digest text NULL REFERENCES evidence_blobs(digest) ON DELETE RESTRICT,
  contract_version text NOT NULL,
  configuration_digest text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (source_decoding, lemma_mapping, pos_mapping, morphology_mapping,
    repair_layer, reading_terminal, normalization_profile, exception_table_digest, contract_version)
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
   - The morphology mapping is part of that set: changing it, or declaring none,
     gives a different configuration (EVD-34).
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
- **No treebank data (#2089).** The handoff records the rule and the earlier
  lines it overrides. This section applies it: `oga-agdt-upos/4`, the
  `morphology_mapping` slot with `oga-agdt-feats/1`, and the
  exception-table changes.
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
| Names one documented lexical scheme in which discriminator values are interpreted. | `scheme` (for example `morpheus`, `example`); `description`; `value_pattern` (regular expression for valid values); `native_markers` (the native spellings that carry a discriminator inside a lemma, for example a trailing `#<digits>`, used to reject undecoded lemmas) | `scheme` | One entry per scheme. | Release-wide. | Changed only by a reviewed release. An entry is never removed or redefined while any identity uses it. | — |

**Initial registry.** The first release registers `morpheus`, the scheme the
retained candidate's discriminators use (#2058), and the synthetic `example`
and `example-*` schemes that fixtures use.

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
| A learner-confirmed alternative identity for one exact occurrence. | §7 occurrence key fields; `owner_id`; `book_id`; `corrected_citation_form text` (the learner's input, trimmed and NFC, with final `ς` kept; it is the headword for this occurrence); `identity_id uuid` (the resulting rich identity); `normalization_profile text` (the profile `name/version` used, equal to the analysis's interpretation slot); `updated_at` (§8; the time of the latest confirmation) | §7 | At most one per (owner, Book, exact occurrence), as for today's corrections. | Owner. | Mutable only by that owner's confirmed lemma review, as today. Never changed by a new analysis or profile. | `identity_id` → Vocabulary identity `RESTRICT`; occurrence → §7 `RESTRICT` |

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
  and any native lexical-identifier attribute, for example a `#ex:1` suffix or
  a `lexid=ex:1` attribute.
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
form `λέξις` with scheme `example`, value `ex:1`. Their key is `λέξισ`. Native
lemma `λέξις#ex:2` decodes to the same citation form with value `ex:2`. They
are two identities that share lemma and UPOS.

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
| One native secondary or enhanced edge, such as a CoNLL-U `DEPS` edge, including edges to and from empty nodes. | `analysis_id uuid`; `edge_ordinal integer`; `node_id integer`; `head_node_id integer`; `native_relation text`; `relation text NULL` (mapped enhanced relation where reviewed) | `(analysis_id, edge_ordinal)` | `(analysis_id, node_id, head_node_id, native_relation)` unique. | Shared. | **Immutable.** | both nodes → Analysis node `RESTRICT` |

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

Morphology is `usable` only through the Interpretation configuration's
`morphology_mapping` slot (§5). `feats` holds only what that slot's reviewed
mapping supplies, and nothing else may fill it: not the native tag read
directly, and not a capability claim in the candidate.

- `missing`: no node has a morphology row.
- `unsupported`: rows exist, but `morphology_mapping` is NULL.
  `native_morphology` is still retained, and every `feats` is `{}`.
- Otherwise `usable`, with a node finding for each node whose native features are
  partly unmapped (`unsupported`) or ambiguous.

The legacy adapter below is unchanged: legacy morphology was stored as UD FEATS
by the producer and is read as it is.

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
   - Morphology is `usable` only through the interpretation's
     `morphology_mapping` slot (OCC-26).
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
- "Reviewed FEATS mapping" now means §5's `morphology_mapping` slot (#2089).
  Before it, the configuration had no place to name one.

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

Verified at commit `f84003451246cef53c52fe246284dd4a801ea145` (`main`). Every
`path:line` in this section refers to that commit. No file outside `doc/` changed
between §3's baseline commit and this one, so §3's citations still hold.

### Summary

- A **retention root** is a retained record that depends on exact evidence:
  adopted Book evidence, a preparation request, an exact occurrence decision or
  flag, a snapshot and its membership, a completion and Rich Known, a deck
  specification or artifact, a Usability restriction, and an unidentified
  Edition's evidence pin. Each root protects the full **evidence closure** of
  what it names, down to the bytes.
- Mechanism **(i)** (OD-37): direct foreign keys along the closure, `RESTRICT`
  into shared evidence and `NO ACTION` between owner-scoped rows, plus **delete
  guards** that reject every `DELETE` and `TRUNCATE` of shared evidence today. No
  retention-reference table keyed by referencer kind is added.
- One new owner-scoped record, the **Adopted evidence record**, keeps every
  (owner, Book, analysis) that was ever adopted, with the acquisitions that gave
  access. Owner-scoped roots reference it, so a root also protects the blob and
  keeps its owner's access route after a later adoption replaces the current
  analysis.
- The protection is always part of the referencing record's own transaction
  (rule RET-TX).
- Hiding, leaving To Read, removing a connection, upstream withdrawal and
  cancellation delete nothing that a root needs. Account deletion keeps the
  existing owner-cascade boundary and removes only that owner's rows. This
  design adds no erasure workflow (ADR 0035).
- Unreferenced evidence is kept. A read-only report identifies it. No job, TTL
  or purge exists.
- After a restore, an operator-run **evidence integrity check** re-hashes every
  stored byte and resolves every reference. A failure appends an
  `integrity_failure` Usability restriction (§5). Affected Books show
  unavailable with the reason. Nothing frozen changes, and nothing is
  substituted.

New record and rule names are proposed terms (OD-40). Table names in the
illustrative DDL are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| The baseline's immutability triggers are `BEFORE UPDATE` only. No migration has a `BEFORE DELETE` or `TRUNCATE` trigger. | `migrations/000001_initialize.up.sql:116-121`, `:2504`, `:2511`, `:2518`, `:2525` | A row with no inbound foreign key is deletable today. §9 adds delete guards to the new shared evidence records. The baseline triggers are not changed. |
| Legacy decks protect their analysis run with a `NO ACTION` foreign key. The same deck row cascades when its source material is deleted. | `migrations/000001_initialize.up.sql:2821`, `:2853` | The direct-foreign-key precedent exists. Classical evidence has no cascade from an owner-scoped parent, so it cannot be lost through one. |
| Legacy snapshots cascade from their Book, source material, run and corpus. Completions and deck preparations reference the snapshot with `NO ACTION`. | `migrations/000007_primary_goal_snapshots.up.sql:29-32`, `:57`; `migrations/000009_primary_goal_completion_provenance.up.sql:12-14` | A completion already protects its snapshot. A rich snapshot (§8) adds a `RESTRICT` reference to its Exact analysis and no cascade from evidence. |
| A legacy deck preparation is retired by setting `retired_at`. It is not deleted. | `sqlc/queries/deck_preparations.sql:149-158`; `migrations/000001_initialize.up.sql:2343` | Deck history is retained by convention today. Classical deck records are roots (#2063). |
| No application code deletes a user or a Book. The only `DELETE FROM users` is in a test. Books cascade from `users`. | `git grep -nIi "delete from users\|delete from books" -- . ':!doc'` returns only `internal/epub/ingest_integration_test.go:91`; `migrations/000001_initialize.up.sql:2645` | Account deletion exists only as a database-level owner cascade. Book deletion does not exist. |
| Removing a connection deletes the connection row. Book aliases and sync status cascade from it. | `sqlc/queries/opds.sql:29`, called from `internal/webapp/catalog_handlers.go:126`; `migrations/000001_initialize.up.sql:2597`, `:2669` | §4 offerings cascade from a connection. §5 acquisitions name their origin as text, not as a foreign key, so no evidence or root is reached. |
| End and Finish delete only the current-reading row. Disposition and visibility write only their own rows. | `sqlc/queries/current_reading.sql:174-176`; `internal/persistence/book_dispositions.go:54-91`; `internal/persistence/book_visibility.go:36-66` | No learner transition deletes a root or evidence. |

### Referencers

Each row is a retention root. "Names" lists the closure entry points that the
root references itself. The full closure of each entry point is in
[Dependency closure](#dependency-closure). Owner-scoped rows reach shared
evidence through the owner's Adopted evidence record or acquisitions, which
also protect the bytes and the access route (§5 "Owner evidence access").

| root | record (owning section) | owner scope | names | reference and on-delete |
| --- | --- | --- | --- | --- |
| R1. Adopted current Book evidence | Book current classical analysis (§5) and Adopted evidence record (§9) | Owner | Exact analysis; text and annotation acquisitions | Current row → `(owner_id, book_id, analysis_id)` Adopted evidence record `NO ACTION` (added constraint, no new column). Adopted evidence record → analysis `RESTRICT`, → acquisitions `NO ACTION`. |
| R2. In-flight preparation request | Preparation request (§11) | Owner | Acquisitions; candidate Exact analysis once written; Interpretation configuration once resolved | Request → acquisitions `NO ACTION`; → candidate analysis `RESTRICT`; → Interpretation configuration `RESTRICT`. §11 owns the request's shape and states. |
| R3. Exact occurrence decisions and flags | Exact occurrence decision, Exact Lemma review flag (§8) | Owner | Annotation occurrence; corrected identity; the analysis through its Adopted evidence record | As §8 (occurrence and identity `RESTRICT`), plus `(owner_id, book_id, analysis_id)` → Adopted evidence record `NO ACTION` (added constraint, no new column). |
| R4. Active and ended snapshots | Rich snapshot row, Rich snapshot membership, Snapshot count contribution (§8) | Owner | Exact analysis; identities; contributing Books' analyses | As §8 (analysis and identity `RESTRICT`). Rich snapshot row `(owner_id, book_id, exact_analysis_id)` and contribution `(owner_id, book_id, analysis_id)` → Adopted evidence record `NO ACTION`. The composite key is unchecked for a legacy snapshot because `exact_analysis_id` is NULL. Release sets `released_at` and deletes nothing. |
| R5. Reading completions and history | `reading_history` row; Rich Known vocabulary (§8) | Owner | The completed snapshot; the accepted membership; the analysis | Completion → snapshot `NO ACTION` (existing, `migrations/000009_primary_goal_completion_provenance.up.sql:12-14`). Rich Known → membership `NO ACTION` (§8), plus `(owner_id, book_id, analysis_id)` → Adopted evidence record `NO ACTION`. |
| R6. Deck specifications and artifacts, Reading-owned and custom | Classical deck records (#2063) | Owner | Every (Book, analysis) whose occurrences, sentences or headwords the specification or artifact uses; identities | Required of #2063: one `(owner_id, book_id, analysis_id)` → Adopted evidence record `NO ACTION` per (Book, analysis) used, and identities `RESTRICT`. The artifact resolves its notices through these references and never through the Book's current analysis. |
| R7. Capability restrictions | Usability restriction (§5) | Shared | Its subject | Subject `RESTRICT` (§5). A restriction is itself guarded, so its explanation stays resolvable. |
| R8. Unidentified Edition's pinned text revision | Edition (§4) | Shared, or private to an owner | Text Source revision | `RESTRICT` (§4, §5). |

Derived data is not a root. Rich count readiness and Rich vocabulary counts
(§8) reference the analysis with `RESTRICT` only to keep their rows consistent.
They are rebuilt or deleted by their writers, and their deletion releases
nothing, because the Adopted evidence record still holds the analysis.

### Dependency closure

The **evidence closure** of an Exact analysis X is every row below. Most rows
are reached from X through `RESTRICT` foreign keys between shared records.
Notices, provenance claims and restrictions point at the closure instead, and
no key reaches them. Every row carries the delete guard. The closure of an acquisition is its blob and
that blob's Package content. An owner-scoped root holds the union of the
closures of everything it names.

| dependency | records (section) | reached from X by | protected by |
| --- | --- | --- | --- |
| Analysis | Exact analysis; its five Capability diagnostics and Diagnostic findings (§5) | X itself; diagnostics and findings → X | Inbound `RESTRICT` from roots; delete guard on every row |
| Analysis content | Analysis sentences, Sentence spans, Analysis nodes, Node spans, Container components, Annotation occurrences, Node morphology, Basic and Secondary syntax edges (§7); Analysis headwords (§6) | each → X, directly or through its node or sentence | `RESTRICT` to X; delete guard. The node IDs in a membership's `occurrence_node_ids` array have no foreign key. Their rows are protected because nothing under X can be deleted. |
| Vocabulary identity | Vocabulary identity (§6) | node → identity | `RESTRICT` from nodes, decisions, membership, Known and counts; delete guard |
| Binding | Text–annotation binding (§5) | X → binding | `RESTRICT`; delete guard |
| Text revision and annotation revision | Text Source revision, Annotation Source revision, and their revision members (§5) | binding → revisions; members → revision | `RESTRICT`; delete guard |
| Unit | Reading unit (§4) | binding → unit | `RESTRICT` (§5); §4's update-rejecting trigger; the delete guard on shared units, and on their shared Edition and Work |
| Extracted text | Extracted text (§5) | binding → Extracted text | `RESTRICT`; delete guard |
| Interpretation version and frozen table | Interpretation configuration; exception-table Package blob (§5) | X → interpretation → table blob | `RESTRICT`; delete guard |
| Manifest and members | Package content; Package members, including the `manifest` member (§5) | revision members → Package member → Package content | `RESTRICT`; delete guard |
| Blob | Package blob of the build that the owner acquired (§5) | Root → Adopted evidence record or request → acquisition → blob | Acquisition → blob `RESTRICT` (§5). Root → acquisition `NO ACTION`. The acquisition also has the owner-scoped guard below. A shared root (R7, R8) does not name a build. It protects the content, not a particular blob. |
| Notices | Evidence notices and Notice coverage whose subject is a revision, a member of either revision, or the interpretation (§5) | coverage → subject; coverage → notice → notice member | `RESTRICT` from coverage; delete guard on coverage and notices. Coverage has no inbound key, so the guard is its only protection. |
| Provenance | Provenance claims on the annotation revision or the interpretation (§5) | claim → subject | Delete guard. A claim has no inbound key. |
| Restrictions | Usability restrictions naming any row above (§5) | restriction → subject | Delete guard |
| Rights records | None is modelled. §5 keeps operation-specific rights with the external gates [#2054](https://github.com/justin-hayes/mouseion/issues/2054), [#2055](https://github.com/justin-hayes/mouseion/issues/2055) and [#2056](https://github.com/justin-hayes/mouseion/issues/2056). | — | A rights record that a gate later adds for a component in this closure joins it with a `RESTRICT` reference to that component and the delete guard. |

### Records

#### Adopted evidence record

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| Keeps every (owner, Book, Exact analysis) that was ever adopted, with the acquisitions that gave the owner access when it was first adopted. It is the owner-scoped anchor that R1 and R3–R6 reference. | `owner_id uuid`; `book_id uuid`; `analysis_id uuid`; `text_acquisition_id uuid`; `annotation_acquisition_id uuid` (may equal the text acquisition); `first_adopted_at timestamptz` | `(owner_id, book_id, analysis_id)` | Key only. Adopting the same analysis again for the same Book reuses the row, and its first acquisitions are kept. | Owner. | **Immutable** and append-only. Inserted only by §11 adoption, insert-if-absent, in the transaction that writes the Book current classical analysis row. Deleted only by the owner cascade (owner guard below). | `(owner_id, book_id)` → §4 Book–Reading unit association `ON DELETE CASCADE`, with the owner guard; `analysis_id` → Exact analysis `RESTRICT`; `(owner_id, text_acquisition_id)` and `(owner_id, annotation_acquisition_id)` → Package acquisition `NO ACTION` |

At insert, a trigger applies the same checks as §5's trigger on the current
row. The analysis's binding names the Book's Reading unit. The text
acquisition's blob carries the text revision's Package content, and the
annotation acquisition's blob carries the annotation revision's. Every Adopted
evidence record therefore satisfies §5's access predicate for its owner. That is
why a retained reference stays readable as that owner's evidence after a
replacement (EFF-4).

```sql
-- illustrative, not a migration
CREATE TABLE adopted_evidence_records (
  owner_id uuid NOT NULL,
  book_id uuid NOT NULL,
  analysis_id uuid NOT NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  text_acquisition_id uuid NOT NULL,
  annotation_acquisition_id uuid NOT NULL,
  first_adopted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, book_id, analysis_id),
  FOREIGN KEY (owner_id, book_id) REFERENCES book_reading_units(owner_id, book_id) ON DELETE CASCADE,
  FOREIGN KEY (owner_id, text_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id) ON DELETE NO ACTION,
  FOREIGN KEY (owner_id, annotation_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id) ON DELETE NO ACTION
);

-- Added constraints on §5 and §8 records. They add no column.
ALTER TABLE book_current_classical_analyses
  ADD FOREIGN KEY (owner_id, book_id, analysis_id)
    REFERENCES adopted_evidence_records(owner_id, book_id, analysis_id) ON DELETE NO ACTION;
ALTER TABLE exact_occurrence_decisions
  ADD FOREIGN KEY (owner_id, book_id, analysis_id)
    REFERENCES adopted_evidence_records(owner_id, book_id, analysis_id) ON DELETE NO ACTION;
ALTER TABLE primary_goal_snapshots
  ADD FOREIGN KEY (owner_id, book_id, exact_analysis_id)
    REFERENCES adopted_evidence_records(owner_id, book_id, analysis_id) ON DELETE NO ACTION;
-- likewise: exact Lemma review flags, snapshot count contributions and Rich Known.

-- Delete guard on shared evidence (one function, one trigger pair per table).
CREATE FUNCTION reject_classical_evidence_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'classical evidence is retained and cannot be deleted';
END;
$$;
CREATE TRIGGER evidence_blobs_retained BEFORE DELETE ON evidence_blobs
  FOR EACH ROW EXECUTE FUNCTION reject_classical_evidence_deletion();
CREATE TRIGGER evidence_blobs_not_truncated BEFORE TRUNCATE ON evidence_blobs
  FOR EACH STATEMENT EXECUTE FUNCTION reject_classical_evidence_deletion();

-- Owner guard on owner-scoped retention rows: deletion only inside the owner cascade.
CREATE FUNCTION reject_retention_root_deletion() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE id = OLD.owner_id) THEN
    RAISE EXCEPTION 'retained evidence reference can be removed only with its owner';
  END IF;
  RETURN OLD;
END;
$$;
CREATE TRIGGER adopted_evidence_records_retained BEFORE DELETE ON adopted_evidence_records
  FOR EACH ROW EXECUTE FUNCTION reject_retention_root_deletion();
```

### Mechanism

**Decision (OD-37, option (i)).** Protection is a foreign key on the
referencing row itself. There is no separate reference table. The rules:

- **RET-FK-1.** Every reference into a shared evidence record is `ON DELETE
  RESTRICT`. This covers both shared-to-shared references and owner-to-shared
  references. Owner deletion never deletes a shared row, so `RESTRICT` never
  blocks it.
- **RET-FK-2.** Every retention reference between owner-scoped rows that the
  same owner cascade removes is `NO ACTION`, as in §4 and §5. PostgreSQL checks
  it at the end of the statement, so cascade order does not matter. The only
  exception is a record's anchor to its own Book. The Adopted evidence record,
  like §8's records, keeps `ON DELETE CASCADE` from the §4 association, and the
  owner guard (RET-GUARD-2) decides whether that cascade may proceed.
- **RET-FK-3.** Every owner-scoped root that names an Exact analysis for a Book
  also references the Adopted evidence record `(owner_id, book_id,
  analysis_id)`. A root that names a request instead, before adoption,
  references that request's acquisitions (R2).
- **RET-GUARD-1. Delete guard.** Every shared record in the closure table above
  has a `BEFORE DELETE` row trigger and a `BEFORE TRUNCATE` statement trigger.
  Both reject the operation. These records are the §5 shared records, the §6
  identity and headword records, and the §7 records. On §4's Work, Edition and
  Reading unit, the row trigger rejects only shared rows (`owner_id IS NULL`), so
  the owner cascade still removes private ones (§4 "Owner deletion"). §4's
  private rows are never in a closure, because bindings name shared units only
  (OD-17). The guard is what makes "retain by default" a database fact. It
  also covers leaf rows that no foreign key reaches, such as Notice coverage,
  Provenance claims, findings, spans and restrictions. `RESTRICT` still records
  the closure, which a future reclamation must respect after it replaces the
  guard. Together with §5's mirrored update triggers, a referenced row can be
  neither deleted nor replaced.
- **RET-GUARD-2. Owner guard.** The Adopted evidence record, the Package
  acquisition (§5) and the preparation request (§11) reject `DELETE` while
  their owner's `users` row exists. Inside the owner cascade the parent row is
  already gone, so the delete proceeds. A direct delete of a classical Book, or
  of its association, cascades to its Adopted evidence records and fails there,
  rolling back the whole statement.
- **RET-TX. Same transaction.** A retention reference is part of its
  referencing record. It is either a foreign key on that record's own row, or
  an Adopted evidence record or request link inserted earlier in the same
  transaction. No referencing record commits without its protection. No job,
  later transaction or deferred step adds protection afterwards. In particular:
  - §11 adoption inserts the Adopted evidence record, insert-if-absent, in the
    transaction that writes or replaces the Book current classical analysis
    row.
  - §11 links a request to its acquisitions and candidate analysis in the
    transaction that writes those evidence rows (§5 "Limits").
  - §8's writers (Start, Switch, Finish, lemma decisions) and #2063's deck
    writers insert their rows with the foreign keys above, in their existing
    transactions.

  §11 relies on this rule. A failed or rolled-back transaction leaves neither
  the referencing record nor its protection.
- **Concurrency.** Shared evidence is never deleted, so nothing races a
  reference today. If a later reclamation replaces the guard, each foreign-key
  check takes `FOR KEY SHARE` on the referenced row. A concurrent delete and a
  new reference therefore serialize, and a committed reference never points at
  a deleted row.
- **Indexes.** The owner cascade deletes Adopted evidence records and
  acquisitions, so every `NO ACTION` key that references them needs an index
  led by its referencing columns. The primary keys of decisions, flags and the
  current row already serve this. The rich snapshot row needs `(owner_id,
  book_id, exact_analysis_id)`. Contributions and Rich Known need `(owner_id,
  book_id, analysis_id)`. Adopted evidence records need `(owner_id,
  text_acquisition_id)` and `(owner_id, annotation_acquisition_id)`. `RESTRICT`
  keys into shared evidence get no extra index, because no shared row is ever
  deleted. A reclamation change adds those, and the report below tolerates
  sequential scans.

**Rules for records not yet designed.** §11's preparation request and #2063's
deck specifications and artifacts must follow RET-FK-1 to RET-FK-3, RET-GUARD-2
(for the request) and RET-TX. A classical deck record never resolves notices,
sentences or headwords through the Book's current analysis. It always resolves
them through its own Adopted evidence record references.

### Deletion behaviour

| event | rows it writes or deletes today or in this design | evidence and roots released |
| --- | --- | --- |
| Hide a Book | Visibility row only (`internal/persistence/book_visibility.go:36-66`) | None |
| Leave To Read, or any disposition change | Disposition row only (`internal/persistence/book_dispositions.go:54-91`). Leaving To Read also cancels the owner's in-flight request (§11), which changes the request's state and deletes nothing. | None |
| End or Finish a Reading | Deletes the current-reading row (`sqlc/queries/current_reading.sql:174-176`), releases the snapshot and, for Finish, writes the completion and Rich Known (§8) | None. The snapshot, membership and completion stay roots. |
| Remove a catalogue connection | Deletes the connection, its offerings and claimed aliases (§4) | None. Acquisitions name their origin as text (§5), and Books, associations and roots stay. |
| Upstream withdrawal | Sets `withdrawn_at` on the offering (§4) | None. Discovery reports the withdrawal separately (ADR 0097). |
| Owner cancels a request | Request state only (§11) | None. The cancelled request's references stay physical rows (OD-39). Other owners' requests, adoptions and roots are not read or written. |
| Replacement adoption (§11) | Replaces the Book current classical analysis row | None. The old analysis keeps its Adopted evidence record. Decisions, snapshots, completions and decks on it stay resolvable (EFF-4). |
| `keep` on an exact decision (§8) | Deletes that decision row | Only that decision's reference. The Adopted evidence record still holds the analysis. |
| Delete a classical Book directly | **No path exists today.** A direct `DELETE` is refused by RET-GUARD-2 when the Book has an Adopted evidence record. | — |
| Delete an account | **No application path exists today.** Only the database-level owner cascade exists (`migrations/000001_initialize.up.sql:2645`), exercised only by a test (`internal/epub/ingest_integration_test.go:91`). The cascade removes the owner's Books, associations, offerings, acquisitions, requests, Adopted evidence records, current rows, decisions, flags, snapshots, membership, completions, Rich Known and deck records. | That owner's roots only. Shared evidence, other owners' rows and their roots are untouched. Evidence that only this owner referenced becomes unreferenced and is retained. |

This design adds **no erasure workflow**. ADR 0035 "Independent removal and
retention" and ADR 0097 "Retention follows every retained reference" keep
whole-account deletion at the existing owner-cascade boundary. Any legal or
erasure workflow that must remove referenced evidence needs a separate
decision.

### Cross-owner rule

- Shared evidence rows have no owner and no owner foreign key. No owner-scoped
  delete, cascade or state change can reach them (RET-FK-1, RET-GUARD-1).
- One owner's root protects the shared rows for every owner. Owner A releasing
  interest, by cancelling, hiding, leaving To Read, removing a connection or
  deleting the account, never removes a row that owner B's root reaches.
- A root grants **no access**. Access stays §5's predicate: the owner holds an
  acquisition whose blob carries the evidence's Package content. B's roots are
  checked against B's acquisitions at insert (RET-TX and the Adopted evidence
  record trigger). A's roots never satisfy B's predicate, and the presence of
  identical bytes never does (physical deduplication is not access). An ID that
  only A's roots reach returns not-found to B.

### Unreferenced evidence

Unreferenced evidence is retained by default (ADR 0097, ADR 0098). Evidence is
**unreferenced** when it is outside the closure of every root R1–R8 whose root
is live. Every root is live except a request in a terminal state (OD-39).

An operator-run **unreferenced-evidence report** identifies it:

- It runs in one read-only `REPEATABLE READ` transaction, writes nothing, and
  takes no lock beyond that snapshot.
- It is started only by an operator command. It is not a River job, it has no
  schedule, and nothing calls it automatically.
- Per record kind (blob, Package content, revision, binding, interpretation,
  analysis, identity), it reports the IDs or digests, the byte or row totals,
  and `held_by`. `held_by` lists any non-root holders: an acquisition with no
  live root, a terminal request, or another unreferenced evidence row.
- It deletes nothing, and it marks nothing for deletion. A future reclamation
  is a separately approved change that defines its own selection, guard
  replacement and recovery.

```sql
-- illustrative, not a migration
-- Analyses outside every live root (one of the per-kind queries).
WITH live_roots(analysis_id) AS (
  SELECT analysis_id FROM adopted_evidence_records
  UNION SELECT candidate_analysis_id FROM preparation_requests
    WHERE candidate_analysis_id IS NOT NULL AND state NOT IN ('adopted','cancelled','failed','superseded')
  UNION SELECT exact_analysis_id FROM primary_goal_snapshots WHERE exact_analysis_id IS NOT NULL
  UNION SELECT analysis_id FROM exact_occurrence_decisions
  UNION SELECT analysis_id FROM rich_snapshot_vocabulary
  UNION SELECT analysis_id FROM snapshot_count_contributions
  UNION SELECT analysis_id FROM rich_known_vocabulary
  UNION SELECT analysis_id FROM classical_deck_evidence_references     -- #2063
  UNION SELECT analysis_id FROM usability_restrictions WHERE analysis_id IS NOT NULL
)
SELECT a.id
FROM exact_analyses a
WHERE NOT EXISTS (SELECT 1 FROM live_roots r WHERE r.analysis_id = a.id);
-- Blobs, contents, revisions, bindings and interpretations are reported by
-- walking the closure downward from the referenced analyses, plus the R2, R7
-- and R8 entry points.
```

### Restore expectations and integrity check

**Backup.** All classical bytes and records are in PostgreSQL (§5, ADR 0098). A
valid backup is one consistent logical dump of the whole database, taken in
one snapshot, so records and blobs are always captured together. A dump that
excludes evidence table data, a data-only dump of part of the schema, or bytes
copied separately from records is not a valid backup of classical evidence.
The operator owns capacity and backups (ADR 0097 "Capacity and availability
guarantees").

**Restore.** A restore recreates the schema with every constraint and trigger,
in one transaction that stops at the first error (for `pg_restore`,
`--single-transaction --exit-on-error`). A restore that loses a row that a key
needs, or that violates a check, therefore fails as a whole, and the
installation is not started on it. Loading data with triggers or constraints
disabled is not a valid restore. The integrity check below still catches bytes
altered at the storage level, and most rows lost by such a load (IC-6, IC-7).
It cannot detect a lost Usability restriction, or a lost provenance claim that
no pin set declares. That is why the load rule exists.

**Evidence integrity check.** This is an operator command. It runs after every
restore, before the web server serves requests, and on demand. It is not a job
and has no schedule. It reads everything in one snapshot and writes only the
restrictions listed below. It checks:

| check | verifies | failure reason code |
| --- | --- | --- |
| IC-1 | Every Package blob: SHA-256 of `content` equals `digest`, and `octet_length(content)` equals `byte_length`. | `blob_digest_mismatch` |
| IC-2 | Every `package` blob: the safe reader (§5 "Limits") re-reads it, and every member's path, size and digest, and the recomputed Package content digest, equal the stored inventory. | `package_inventory_mismatch` |
| IC-3 | Every `interpretation_table` blob digest. | `interpretation_table_digest_mismatch` |
| IC-4 | Every Extracted text: digest of the UTF-8 content, and code-point length. | `extracted_text_digest_mismatch` |
| IC-5 | Every record-kind Evidence notice: `notice_digest`. | `notice_digest_mismatch` |
| IC-6 | Every closure edge resolves: an anti-join for each foreign key in the closure table and the Referencers table, including the added Adopted evidence record keys. Every Adopted evidence record and current row satisfies §5's access predicate. Every Package content's member count and byte sum equals its member rows. Every Exact analysis has its five diagnostics and a `validated` binding. | `evidence_reference_missing` |
| IC-7 | Rows that no key reaches are complete. Every Exact analysis's persisted rows pass the §10 shared validation rules again; for example, every printed node has a span, and every qualifying node has an Annotation occurrence. Every Notice coverage and Provenance claim that the acquisition's recorded pin set declares exists. | `evidence_reference_missing` |

**On failure.** For each failure, the check appends one Usability restriction
(§5) with basis `integrity_failure`, all five capabilities, the reason code
above and an explanation naming the failed row. The subject is:

- for IC-1 and IC-2, the blob's Package content (OD-38);
- for IC-3, every Interpretation configuration that names the table;
- for IC-4, every binding that names the Extracted text;
- for IC-6 and IC-7, the nearest existing row of the closure above the missing
  one.

Under §5's transitive rule, every analysis that depends on the subject becomes
restricted. An IC-5 failure restricts no capability. It is reported, and every
operation that would reproduce that notice, such as a deck artifact or an export,
fails closed until an exact restore. If an IC-6 failure has no existing row to
attach a restriction to, for example because a root names a missing analysis,
the check reports the restore as failed. The operator restores again.

**What a Book shows.** A Book whose current classical analysis, or whose
retained reference used by an operation, is restricted shows **"Evidence
unavailable"** with the reason, in Book detail, the Reading page and the
chooser. The following happen, as ADR 0097 "Durable bounded work and recovery"
and §5 require:

- Start, Switch to the Book, lemma decisions, new deck preparation and new
  adoption of the restricted analysis are refused with the reason.
- There is **no automatic End or Finish**. A current reading on the Book stays
  current. Explicit End stays available. Explicit Finish stays available
  because it reads only the frozen snapshot (§8), which the check does not
  change.
- There is **no substitution**. No other analysis, revision, Edition, or intact
  build of the same content is adopted or read in its place.
- **Nothing frozen changes.** Snapshots, membership, decisions, flags,
  completions, Known, Adopted evidence records and the Book current classical
  analysis row are byte-identical before and after the check.
- Recovery is an exact restore that passes the check, followed by OD-19's lift.
  An explicitly chosen, validated replacement through §11 is the alternative.

### Invariants

Each invariant is covered by the `RET` rows in [§15](#15-acceptance-test-matrix).

1. **Referenced evidence cannot be deleted or replaced.** While any root
   reaches a blob, revision, binding, interpretation, analysis or any other
   closure row, every `DELETE`, `TRUNCATE` and `UPDATE` of it is refused
   (RET-01, RET-02, RET-03).
2. **Every root protects its full closure**, including notices, provenance and
   the acquired blob, and keeps its owner's access route after a replacement
   adoption (RET-04, RET-05, RET-06).
3. **Same transaction.** No referencing record commits without its protection
   (RET-07).
4. **Release events release nothing.** Hiding, leaving To Read, End, Finish,
   connection removal, upstream withdrawal and cancellation delete no evidence
   and no root (RET-08, RET-09).
5. **Cross-owner.**
   - One owner's release never removes evidence another owner's root reaches
     (RET-10).
   - Owner deletion removes only that owner's rows (RET-11).
   - A root grants no access (RET-12).
6. **Deletion boundary.** A classical Book with adopted evidence cannot be
   deleted except through its owner. The design adds no erasure path (RET-13).
7. **Unreferenced evidence** is reported and never deleted (RET-14).
8. **Restore.**
   - The integrity check detects altered bytes and unresolved references
     (RET-15, RET-16).
   - It marks affected Books unavailable without changing frozen records,
     without automatic End or Finish, and without substitution (RET-15,
     RET-17).
   - A clean restore passes with no restriction (RET-15).

### Conflicts and dependencies noted

- **§5 additions, not contradictions.** §9 adds the delete guard to every §5
  shared record, and the owner guard to Package acquisition. §5 already says no
  delete path is designed. §9 also adds the composite foreign key from the Book
  current classical analysis row to the Adopted evidence record. None of these
  changes a §5 field.
- **§8 additions.** §8 assigned retention on older analyses to §9. §9 adds the
  composite foreign key `(owner_id, book_id, analysis_id)` → Adopted evidence
  record to the decision, flag, rich snapshot, contribution and Rich Known
  records. No §8 column changes.
- **§11** owns the request record. It must carry the R2 references, the owner
  guard and its terminal states (OD-39), and must insert the Adopted evidence
  record under RET-TX.
- **#2063** owns classical deck records. It must carry the R6 references and
  resolve notices only through them.
- **Usability restriction subject.** §5's restriction has no blob subject, so a
  corrupted build can only be restricted through its Package content (OD-38).
- **Ticket wording versus ADR 0097.** Issue #2070 says a failed Book shows "no
  End or Finish". ADR 0097 is binding and allows explicit End and Finish when
  frozen records support them, forbidding only automatic ones. §9 follows the
  ADR (OD-42).
- **Seams.** RET-14 and RET-16 test operator commands on seam (d) (OD-41).
- **Operator commands.** The integrity check and the unreferenced report are
  operator commands. Their command-line surface is §12's to place beside the
  local entry point.

## 10. Module interfaces and validation

Verified at commit `f84003451246cef53c52fe246284dd4a801ea145` (`main`). Every
`path:line` in this section refers to that commit.

### Summary

- ADR 0098 names four modules, and this section gives each one an interface:
  - A **source interpretation adapter** turns verified package contents into a
    **canonical-analysis candidate**.
  - **Shared canonical validation** turns a candidate into either a **validated
    analysis**, with one state per capability, or a rejection.
  - **Shared preparation and adoption** (§11) persists and adopts only validated
    analyses.
  - The **shared learner core** reads only validated analyses, through the §7
    analysis read interface.
- Validation is one pure Go function in one package. Classical adapters and the
  Stanza compatibility adapter feed the same function. A producer's capability
  claim never raises a state.
- Only validation can construct a validated analysis, and adoption accepts
  nothing else.
- Each rule is **structural** (it rejects the candidate), **required** (it
  denies Vocabulary-ready) or **optional** (it withholds one capability at one
  scope).
- Diagnostics carry codes, rule IDs, integer references and counts. They never
  carry source text, learner input or digests, so they are safe to log.
- Every validation failure is permanent. Optional-layer findings are not
  failures. Only infrastructure failures are transient.
- The Stanza compatibility adapter observes parity after today's publication
  commit. It writes nothing, changes nothing and switches nothing.
- Annotation-backed adapters, validation, adoption and Reading never need the
  NLP service. NLP readiness gates producer-backed analysis only.

New names are proposed terms (OD-46). Package and type names are proposals, not
prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| `analyzer.Result` carries the language, the normalization profile and analyzer provenance. Its sentences carry text and a location. Its tokens carry surface, raw and canonical lemma, UPOS, dependency, head, morphology and location. | `internal/analyzer/analyzer.go:35-98` | The Stanza compatibility adapter maps exactly these fields. Stanza output has no container, empty node, discriminator or accounting class to map. |
| The analysis job checks NLP dependency-parsing readiness before it calls the analyzer, then calls the analyzer once per chunk. | `internal/analysis/jobs.go:1256`, `:1909-1921`, `:1345` | B-05. These are the only NLP gates on analysis. They stay, and apply to producer-backed analysis only. |
| One transaction persists the corpus and its tokens. Selection runs after that commit. | `internal/analysis/jobs.go:1467-1500`, `:1362-1394` | The parity observation runs after `persistSnapshotWork` commits, outside every transaction. |
| `validateCorpusToken` is the only structural check on Stanza output today. | `internal/analysis/jobs.go:1849-1862`, called at `:1800` | Unchanged (B-03). The compatibility adapter's validation never replaces it. |
| Today's counts come from `selection.AnalyzableStatistics` and `selection.Project`. A candidate's sentence evidence is `SentenceReference{SentenceIndex, Text, Location}`. | `internal/analysis/jobs.go:1319`; `internal/selection/selection.go:28-43`, `:89-106`, `:198-227` | Parity compares against these functions' output, field by field. |
| `OccurrenceEligible` reads the dependency. | `internal/selection/selection.go:281-285` | The eligibility interface below applies OD-03. |
| Catalogue sync records the NLP-ready languages and keeps only the OPDS language feeds that NLP reports ready. | `internal/cataloguesync/sync.go:1018-1034`, `:897-910`, `:494-498`, `:553-557`; `internal/analyzer/capabilities.go:50-58` | B-27, B-28. OPDS only. Classical paths never call them. |
| The server creates its NLP client lazily and caches capabilities. Startup never probes NLP. | `cmd/server/main.go:94`, `:171`; `internal/analyzer/grpc.go:23-38` (`grpc.NewClient` does not connect) | With NLP stopped, the server still starts. That must stay true. |
| `supported_languages` supplies only display names to the My Books and Known language lists, and the code is the fallback. | `sqlc/queries/book_aliases.sql:5-15`, `sqlc/queries/core.sql:229-236` | Not a gate. A `grc` Book is listed under `grc` unless a display name comes from elsewhere (§12). |
| depguard enforces ADR 0091 with one rule per file glob. | `.golangci.yml:86-192` | The import rules below become depguard rules (OD-43). |
| The store-contract package imports only the domain and fixes `Language = "de"`. | `internal/storecontract/harness.go:7-18`, `:44-49`, `:99-143` | OD-04, resolved below. |
| Stanza characterization fixtures: one German protobuf corpus, Italian and Greek regression JSON, and inline Italian and Greek corpora. | `internal/analyzer/testdata/stanza_python_corpus.pb.b64` and `nlp/tests/testdata/{italian,greek}_stanza_expected.json`, loaded at `internal/analyzer/python_fixture_test.go:16-71`; `internal/selection/selection_test.go:194-270` | These are the parity fixtures. The JSON fixtures carry no document text. Their inputs exist only in the Python tests, for example `nlp/tests/test_producer.py:952-954`. |

### Package placement

Layering, with arrows meaning "may import":

```text
cmd/server, §12 local entry command        (composition only)
  │  registers adapters, wires the parity observer
  ├─► internal/interpretation/<adapter>     source interpretation adapters
  │     e.g. internal/interpretation/oga, internal/interpretation/stanzacompat
  │     └─► internal/canonical ─► internal/domain ─► internal/canonicalization
  ├─► internal/parity                       Stanza parity observation
  │     └─► internal/interpretation/stanzacompat, internal/selection, internal/canonical
  └─► internal/adoption                     preparation and adoption job package (§11)
        └─► internal/canonical, internal/persistence

learner core: internal/webapp, internal/lemmareview, internal/prepareddeck,
  internal/analysisinsights, internal/selection, internal/persistence,
  internal/fixtures, internal/storecontract
  └─► internal/canonical ─► internal/domain ─► internal/canonicalization
```

| package | role | may import (internal) | must not import |
| --- | --- | --- | --- |
| `internal/canonical` (new) | Candidate and validated-analysis types, `Validate`, the compiled registries (contract versions, adapters, interpretation slots, reason codes, discriminator schemes), the diagnostic type, the adapter interface, and the §7 read-interface shapes. | `internal/domain`, `internal/canonicalization` | Every other internal package, pgx, River, gRPC, generated code, `net/...`, `os/exec`, `plugin`. It is a pure leaf above the domain. |
| `internal/interpretation/oga` (new) | The classical adapter for the retained candidate's evidence family. | `internal/canonical`, `internal/domain`, `internal/canonicalization` | persistence, sqlc, pgx, River, `internal/analysis`, `internal/adoption`, learner-core packages, `net/...`, `os`, `os/exec`, `plugin`, `syscall`, `unsafe`, `archive/zip` (OD-43). |
| `internal/interpretation/stanzacompat` (new) | The Stanza compatibility adapter. Its input is its own plain value type mirroring the `analyzer.Result` fields it maps. | as `oga` | as `oga`, and also `internal/analyzer`, which transitively links gRPC and `net` (`internal/analyzer/grpc.go:23-38`) |
| `internal/parity` (new) | Converts an `analyzer.Result` into the compatibility adapter's input, compares the Stanza path with the canonical path, and reports mismatches. | `internal/interpretation/stanzacompat`, `internal/canonical`, `internal/selection`, `internal/analyzer`, `internal/domain` | persistence, sqlc, pgx, River |
| `internal/adoption` (new, §11) | Durable preparation and atomic adoption. It holds adapters through the registry interface in `internal/canonical`, never by import. | `internal/canonical`, `internal/domain`, `internal/persistence`, `internal/txcleanup`, pgx and River (an ADR 0091 §3 amendment, OD-43) | `internal/interpretation/...`, `internal/analyzer`, `internal/analysis` |
| learner core (existing) | Consumers listed under [Shared learner core](#shared-learner-core). | `internal/canonical` for read shapes and capability states, plus their existing imports | `internal/interpretation/...`, `internal/parity` |
| `internal/domain` (existing) | Gains the shared value types: rich identity, capability, capability state, analysis reference (OD-44). | unchanged (ADR 0091 §1) | `internal/canonical`. The domain stays below it. |

Only `cmd/server`, the §12 local entry command, `internal/parity`, and test
files may import `internal/interpretation/...`. A depguard rule enforces this,
with a message that cites the ADR 0091 amendment (VAL-13, VAL-14). The rule does
not name individual learner-core packages, so a new consumer is covered
automatically.

depguard checks direct imports only. A dependency test therefore walks the
transitive graph (`go list -deps -test`) and fails when:

- any package outside the allowed importers depends on
  `internal/interpretation/...`;
- any `internal/interpretation/...` package depends on `internal/persistence`,
  `gen/sqlc`, pgx, River, gRPC, `net`, `os/exec` or `plugin`.

Verified package members reach adapters through a read-only interface. The
acquisition stage (§11, §12) implements it after the §5 inventory and limit
checks. Adapters never open the package archive themselves.

### Source interpretation adapter

| aspect | specification |
| --- | --- |
| Responsibilities | Read the pinned members. Apply the reviewed projection and correspondence check to produce the Extracted text and verify `source_bounds` against the unit (§5 binding). Decode native lemma data (§6 source decoding). Apply the pinned lemma, POS, morphology, repair and reading-terminal mappings and the frozen exception table. Assign `node_id` in canonical node order. Map native spans into the Extracted text (SPAN-4). Classify accounting (§7). Emit the native lineage and adapter diagnostics. |
| Inputs | The verified members (read-only, already checked against the pins); the binding declaration from the reviewed definition (Reading unit, `source_bounds`, projection and correspondence identifiers); the pinned Interpretation configuration (§5); and the frozen exception table's bytes, verified against the pinned digest before the call. Nothing else: no clock, environment, network, database or learner state. |
| Outputs | Either a **candidate** (the §7 shape: sentences, nodes, spans, containers, occurrences, morphology, basic and secondary syntax) with its Extracted text, native lineage (native member, document, sentence and node references, and the native attributes that justify each derived value) and adapter diagnostics, or an adapter rejection with diagnostics. |
| Must not | Publish or persist anything. Change learner state. Invent defaults: no `X` for an unknown POS, no root for a headless tree, no offset from a length, no guessed metadata. Approve its own output: it never sets `vocabulary_ready` or a capability state. Execute package-supplied code: no plugins, no subprocesses, no evaluation of expressions, regular expressions, templates, stylesheets or schemas taken from package bytes; XML is parsed with DTDs and external entities disabled and with bounded depth and token counts. Fetch network resources. Read the wall clock, environment or randomness. |
| Invariants | Deterministic: the same inputs give a byte-identical candidate under the canonical serialization (EVD-14 relies on it). Every node the source attests in the extent appears in the candidate (ACC-1). Native evidence is copied, never edited. |
| Version support | Each adapter declares one adapter identifier (`name/version`, for example `example-adapter/1`) and the exact set of slot identifiers it implements. Both are compiled into the reviewed release. A configuration naming any other version is refused before any member is read, with `adapter_version_unsupported` (permanent). A new mapping version needs a reviewed release. |
| Diagnostics | Adapter-scope codes: `native_parse_failed`, `projection_failed`, `correspondence_failed` (each rejects). Node-scope mapping findings: `mapping_unsupported`, `upos_mapping_ambiguous`, `lemma_absent`, `discriminator_unmapped`, `native_class_unresolved` (they leave the node `unresolved` or `non_vocabulary` with an `ambiguous` UPOS, as §7 allows). All use the shape under [Diagnostic shape](#diagnostic-shape). |
| Failure classes | Every adapter failure is permanent. The adapter performs no I/O that can fail transiently, because its members are already in memory. |
| Ownership | One package per adapter under `internal/interpretation/`. It owns its decoding fixtures (`source-decoding/1`, §6) and its mapping fixtures. It owns no records. Changes ship only in reviewed releases, never in packages (ADR 0098). |

Illustrative signature:

```go
// illustrative signature — not an implementation
package canonical

type SourceInterpretationAdapter interface {
	ID() string                                         // "example-adapter/1"
	Supports(cfg InterpretationConfig) error            // ErrAdapterVersionUnsupported
	Interpret(ctx context.Context, in AdapterInput) (AdapterOutput, error)
}

type AdapterInput struct {
	Members        VerifiedMembers       // read-only; bytes already verified against pins
	Binding        BindingDeclaration    // unit, source_bounds, projection, correspondence
	Interpretation InterpretationConfig  // the pinned slots, contract version, table digest
	ExceptionTable []byte                // verified against Interpretation's table digest
}

type VerifiedMembers interface {
	Member(path string) ([]byte, error)  // only pinned paths; bounded by §5 limits
}

type AdapterOutput struct {
	ExtractedText string                 // the binding's text (§5)
	Candidate     Candidate              // §7 shape, unvalidated
	Lineage       NativeLineage
	Diagnostics   []Diagnostic
}
```

### Shared canonical validation

| aspect | specification |
| --- | --- |
| Responsibilities | Decide whether a candidate is admissible, and derive each capability's state from the candidate's content alone, by the rules below. |
| Inputs | The candidate; the declared Reading unit and binding (unit ID, language, binding ID and status, Extracted text and its digest and code-point length); the Interpretation configuration; and the adapter ID. |
| Outputs | Either **Rejected**, with diagnostics and no analysis, or **Validated**: a validated analysis carrying one state and reason-code set per capability (§5's five), sentence- and node-scoped findings, and `VocabularyReady`. |
| Must not | Write anything. Read learner state, restrictions, the database or the network. Repair, reorder or fill in candidate content. Read producer claims as proof. Branch on adapter, producer or language, except where a rule names one: the dedicated-profile check (§6) and a registry lookup keyed by adapter ID. |
| Invariants | Pure and deterministic: equal inputs give equal outputs, including diagnostic order (sorted by rule, scope, sentence, node). Only `Validate` can construct a validated analysis, because its fields are unexported. A producer claim never raises a state: a claim with no support produces only a `claim_not_supported` notice. Optional failures never change `VocabularyReady`. |
| Version support | The compiled registries say what this release supports: contract versions (initially `canonical/1`), adapter identifiers, slot identifiers and table digests, normalization profiles (through `canonicalization.Lookup`), discriminator schemes, and reason codes per contract version. Anything else fails VER-1 to VER-4. |
| Diagnostics | As under [Diagnostic shape](#diagnostic-shape). |
| Failure classes | All validation outcomes are permanent, because validation is deterministic and a retry would give the same result. |
| Ownership | `internal/canonical`. It owns the rule catalogue, the capability-derivation rules and the registries. It owns no records: §11 writes the §5 diagnostics and §7 rows from a validated analysis. |

Illustrative signature:

```go
// illustrative signature — not an implementation
package canonical

func Validate(in ValidationInput) Result

type ValidationInput struct {
	Candidate      Candidate
	Declared       DeclaredScope         // unit, language, binding (or legacy scope), Extracted text
	Interpretation InterpretationConfig
	AdapterID      string
}

type Result struct {
	Outcome     Outcome                  // Rejected | Validated
	Analysis    *ValidatedAnalysis       // nil exactly when Rejected
	Diagnostics []Diagnostic
}

// ValidatedAnalysis has only unexported fields; Validate is its only constructor.
func (v *ValidatedAnalysis) VocabularyReady() bool
func (v *ValidatedAnalysis) Capability(c domain.Capability) CapabilityResult // state + reason codes
func (v *ValidatedAnalysis) Findings() []Diagnostic
```

#### Rule catalogue

Rules owned by this section have new IDs: `VER-*`, `SCOPE-*`, `COR-*`, `IDV-*`
and `BND-1`. Every other ID is §7's, used unchanged.

| rule | checks | source | class | codes |
| --- | --- | --- | --- | --- |
| VER-1 | `contract_version` is registered. | §5 | structural | `contract_version_unknown` |
| VER-2 | Every interpretation slot, including `morphology_mapping` when it is set, and the table digest are registered. Superseded identifiers are not registered. | §5 "Unknown versions stop import" | structural | `interpretation_version_unknown` (names the slot, never its value) |
| VER-3 | The adapter ID is registered and supports the configuration. | this section | structural | `adapter_version_unsupported` |
| VER-4 | Every reason code in the candidate (non-vocabulary reasons, adapter findings) is registered for the contract version. | §7 ACC-2 | structural | `reason_code_unknown` |
| SCOPE-1 | The candidate names the declared binding, Reading unit, Extracted text digest and code-point length. | §5 binding | structural | `scope_mismatch` |
| SCOPE-2 | The declared binding's status is `validated`. | §5 (EVD-15) | structural | `binding_rejected` |
| SCOPE-3 | The candidate's language equals the declared language, and every identity's language equals it. | §4, §6 | structural | `language_mismatch` |
| ACC-4 | **Full declared scope:** every non-`White_Space` code point of the extent lies in a printed node's span. | §7 | **required** | `coverage_gap` (count = uncovered code points) |
| COR-1 | The supplied Extracted text hashes to the declared digest, and its length equals the declared code-point length. | §5 | structural | `extracted_text_mismatch` |
| COR-2 | **Exact text correspondence:** when the adapter declares verbatim surfaces, each printed node's surface equals the concatenation of its Extracted-text slices. A component that references exactly its container's spans (CONT-2) is exempt. | §7 SPAN-4 | structural | `surface_span_mismatch` |
| SPAN-1, -2, -3, -5 | Spans lie within bounds, are ordered, disjoint and non-adjacent, are present exactly for printed kinds, and lie within their sentence. | §7 | structural | `span_out_of_bounds`, `span_order`, `span_presence`, `span_outside_sentence` |
| SENT-1, -2, -4 | Sentences are dense, ordered and disjoint, and not empty. Supplied sentence text is bound to the spans. | §7 | structural | `sentence_order`, `sentence_empty`, `sentence_text_unbound` |
| BND-1 | **Sentence boundaries:** only the first and the last sentence of the extent may be `incomplete`. | §7 SENT-3 | structural | `boundary_incomplete_interior` |
| SENT-3 | `complete`, `incomplete` or `unknown`, as declared. | §7 | optional (`sentence_target`) | `unit_extent_cuts_sentence`, `source_boundary_undeclared` |
| CONT-1, CONT-2 | **Container well-formedness.** | §7 | structural | `container_malformed`, `component_outside_container` |
| OCC-ID-1, OCC-ID-2 | **Node well-formedness and occurrence accounting:** dense canonical node order, and exactly one occurrence per word or component classed `vocabulary` or `unresolved`. | §7 | structural | `node_order`, `occurrence_set_mismatch` |
| ACC-1, ACC-2 | Every node has one accounting class. A `non_vocabulary` node has a registered reason and native evidence, and a non-structural reason has a reviewed mapping. | §7 | structural | `accounting_missing`, `non_vocabulary_unsupported` |
| ACC-3 | **Occurrence accounting:** zero `unresolved` occurrences. | §7 | **required** | `occurrence_unresolved` (count = occurrences) |
| ACC-5, IDV-1 | **Identity validity:** a `vocabulary` node has an identity, a citation form and a resolved UPOS. The identity's UPOS and discriminator equal the node's. UPOS is one of the 17 UD tags. | §6, §7 | structural | `identity_upos_mismatch`, `upos_invalid` |
| IDV-2 | **Profile applied:** the interpretation's normalization profile resolves through `canonicalization.Lookup` for the candidate's language. A dedicated-profile language (`grc`) requires `ancient-greek/1`. | §6 I-2, I-4 | structural | `normalization_profile_rejected` |
| IDV-3 | Every citation form passes the §6 admission check. | §6 | structural | `citation_form_invalid`, `discriminator_in_lemma` |
| IDV-4 | Every identity's canonical lemma equals the profile key recomputed from the citation form. | §6 I-4 | structural | `canonical_key_mismatch` |
| IDV-5 | **Discriminator pairs:** scheme and value are both set or both null. The scheme is registered, and the value matches the scheme's pattern. | §6 | structural | `discriminator_half_set`, `discriminator_scheme_unknown`, `discriminator_value_invalid` |
| TREE-2 (malformed case), SYN-1, SYN-2 | A self-headed edge that is not `root`, or a `root` that is not self-headed, and invalid syntax endpoints. | §7 | structural | `root_malformed`, `syntax_endpoint_invalid` |
| TREE-1 to TREE-4 | **Tree validity per optional layer**, per sentence. | §7 | optional (`basic_syntax`) | `head_unresolved`, `root_missing`, `root_multiple`, `tree_cycle`, `tree_disconnected`, `syntax_absent`, `relation_unmapped` |
| morphology, secondary syntax | As §7 "Optional layers and availability". | §7 | optional | `morphology_unmapped`, `morphology_ambiguous`, `secondary_unmapped` |
| SPAN-4, TREE-5 | No span from a length, and no fabricated root, head or relation. | §7 | adapter obligation | — |

Rules marked **required** are the ones that deny Vocabulary-ready. A
structural failure denies it too, because nothing is validated. Optional rules
withhold only their capability, only at the scope of the finding.

SPAN-4 and TREE-5 constrain how an adapter produced its output. A candidate
alone cannot show that they were broken: a fabricated root looks like a real
one. They are enforced by each adapter's reviewed fixtures and by COR-2, not by
`Validate` (see Conflicts).

#### Capability derivation and readiness

For each capability, `Validate` derives the release-scope state and its
findings:

| capability | release-scope state | sentence or node findings |
| --- | --- | --- |
| `vocabulary` | `usable` when ACC-3 and ACC-4 hold. `missing` (`lemma_layer_absent`) when no node carries native lemma data. Otherwise `invalid`, with the failing codes among `occurrence_unresolved` and `coverage_gap`. | One node finding per unresolved occurrence, with its cause (`mapping_unsupported` → `unsupported`, `upos_mapping_ambiguous` → `ambiguous`, `lemma_absent` → `missing`, `discriminator_unmapped` → `unsupported`). |
| `sentence_target` | `usable` when at least one sentence is `complete`. `unsupported` when every sentence is `unknown`. Otherwise `missing`. | `incomplete` gives a `missing` finding (`unit_extent_cuts_sentence`). `unknown` gives an `unsupported` finding (`source_boundary_undeclared`). Whether a sentence is a good card target belongs to the #2034 rules. |
| `morphology`, `basic_syntax`, `secondary_syntax` | As §7 "Optional layers and availability". | As §7. |

Readiness:

- A **Vocabulary-ready analysis** is a validated analysis whose `vocabulary`
  state is `usable`. §5's `vocabulary_ready` stores exactly this.
- An analysis is **adoptable** (§11) when it is Vocabulary-ready and no
  Usability restriction (§5) names `vocabulary` on it or on any transitive
  input. Restrictions are applied at adoption and at read time, never by
  `Validate`. Validation never sees them, so its result is fixed.
- A classical Book is **Reading-ready** when its current classical analysis is
  adoptable at read time. A restriction appended after adoption makes the Book
  not Reading-ready for a new Start (VAL-21). Start also needs the §8 count
  readiness when the 3/15 rule reads library totals.
- The **effective state** a consumer sees for a capability at a scope is the
  narrowest validated finding, or the release-scope state if no finding
  applies, forced to not usable by any restriction that names the capability.
  The read interface computes it. Consumers never do.

#### Diagnostic shape

| field | type | meaning |
| --- | --- | --- |
| `code` | text, registered | Reason code, `^[a-z][a-z0-9_]*$`, from the registry for the contract version. |
| `rule` | text, nullable | Rule ID from the catalogue above, for example `TREE-3`. Null for adapter and version codes. |
| `severity` | `reject`, `blocks_vocabulary`, `withholds`, `notice` | `reject` makes the outcome Rejected. `blocks_vocabulary` denies Vocabulary-ready. `withholds` withholds one capability at one scope. `notice` changes nothing. |
| `capability` | one of the five, or `none` | The capability affected. `none` for structural, version and scope codes. |
| `scope` | `request`, `binding`, `analysis`, `sentence`, `node` | The narrowest scope the fact holds for. |
| `sentence_ordinal` | integer, nullable | §7 key, set for `sentence` and `node` scope. |
| `node_id` | integer, nullable | §7 key, set for `node` scope. |
| `count` | integer ≥ 1 | How many items the diagnostic aggregates, for example uncovered code points or unresolved occurrences. |
| `message` | text | The registry's fixed English text for `code` in this contract version. It is never built from candidate, package, request or learner strings. |

**Safe to log.** The Go type has no string field whose value comes from input.
`code`, `rule` and `message` come from the compiled registry, and every other
field is an enum or an integer. A diagnostic never contains a surface, lemma,
citation form, discriminator value, native attribute value, native ID or
citation, sentence text, member path, digest, owner ID or Book title. A slot is
named by its slot name (`lemma_mapping`), never by the rejected identifier.
Logs carry the request ID and a summary of counts per code. The diagnostics
themselves are readable only through the owner's evidence detail (§5 access
rule), where the UI may resolve integer references to the owner's evidence.

**Where diagnostics go.** `withholds` and `blocks_vocabulary` diagnostics with
sentence or node scope become §5 Diagnostic findings. Analysis-scope ones become
the §5 Capability diagnostic's `reason_codes`. `reject` diagnostics and notices
are recorded only on the §11 request, because no analysis exists.

### Shared preparation and adoption

Interface name only. The transaction, durable stages, expected-state checks and
recovery belong to §11.

| aspect | specification |
| --- | --- |
| Responsibilities | ADR 0098's row: authorize and durably pin intent, coordinate bounded work, retain evidence, prepare exact count projections, and adopt atomically against the expected owner, Book and request state. |
| Inputs | An owner, a Book, a reviewed definition (§4, §12) and expected state, then a validated analysis produced from them. |
| Outputs | An adopted Book current classical analysis (§5) with count readiness (§8), or a request outcome with diagnostics. |
| Must not | Accept anything but a `*canonical.ValidatedAnalysis` from `Validate`. Adopt an analysis that is not adoptable. Import an adapter package. Call NLP. |
| Failure classes | As in [Failure classes](#failure-classes). |
| Ownership | `internal/adoption` (§11). It writes the §5 shared rows, the §7 rows and the owner's current record. |

```go
// illustrative signature — not an implementation; §11 owns the semantics
package adoption

type Service interface {
	Prepare(ctx context.Context, owner, bookID string, req PreparationRequest) (RequestID, error)
	Adopt(ctx context.Context, owner, bookID string, expected ExpectedState, analysis *canonical.ValidatedAnalysis) (Outcome, error)
}
```

### Shared learner core

The learner core reads only validated analyses, through the §7 analysis read
interface (`canonical.AnalysisReader`, implemented by the exact and legacy
adapters in persistence and by the fixtures store). It decides from two things
only:

- the **representation**: the analysis reference kind (`exact` or `legacy`)
  and the record representations in §8 "Representation selection";
- the **effective capability state** of what it reads.

Read shapes expose no adapter, producer or analyzer name. That provenance is
available only to the owner's evidence-detail view, which displays it and
decides nothing (VAL-20).

| consumer | capability it requires | when that capability is not usable |
| --- | --- | --- |
| Browse and Book vocabulary | `vocabulary` (Book Reading-ready), plus §8 count readiness `ready` for the current analysis | Updating or Unavailable, as §8. Never zero counts. |
| Counts: My Books coverage, Reading chooser bands, library totals | `vocabulary`. The contributing-Book rule is §8's. | Pending or unavailable, never zero (§8). |
| Concordance lemma and form lookup | `vocabulary` | The Book is left out, and its name and reason are listed. |
| Concordance sentence context and highlighting | sentence spans (always present, SENT-2); `sentence_target` only for the boundary label | — |
| Concordance role filter, dependents, sentence-study relations | `basic_syntax`, per sentence | Unavailable with the reason (§7). Never an empty result. |
| Morphology display | `morphology`, per node | "Morphology unavailable" with the reason. |
| Lemma review: preview, confirm and flags | `vocabulary` on the Book's current analysis | Refused with the existing not-ready response. The rich preview fingerprint covers the current analysis ID and the Book's exact decisions and flags, so an adoption makes a preview stale (§8, LRN-18). |
| Reading Start and Switch | Reading-ready target Book, and count readiness when 3/15 needs library totals | The existing ineligible verdict (`internal/domain/current_reading_start.go:58-76`). |
| Reading End | none | — |
| Reading Finish | none at Finish time. It reads the frozen Rich snapshot membership (§8). | A restriction appended after Start does not block an explicit Finish from intact frozen records (ADR 0097 "Durable bounded work and recovery"; §9 OD-42, RET-17). |
| Deck admission | none from the analysis. It reads the frozen snapshot (ADR 0085). Card-sentence choice reads `sentence_target` per sentence (#2034, #2063). | Card rules are #2063's. |
| Eligibility (`selection`) | `basic_syntax` per occurrence, only for syntax-based exclusions | The exclusion is not applied (OD-03 (1)). |

**Eligibility interface (OD-03).** §7 assigns it here. `selection` gains one
entry point that takes the occurrence's identity, UPOS and basic relation as a
layer value:

```go
// illustrative signature — not an implementation
package selection

type OccurrenceView struct {
	Lemma, UPOS string
	Relation    canonical.Layer[string] // State + Value; Value only when usable
}

// EligibleOccurrence applies the language rules. A syntax-based exclusion such as
// compound:prt applies only when Relation.State is usable.
func EligibleOccurrence(cfg SelectionConfig, o OccurrenceView) bool
```

The §7 legacy adapter reports legacy basic syntax as `usable` with the stored
relation (OD-29). So for every legacy occurrence, `EligibleOccurrence` returns
exactly what `OccurrenceEligible(cfg, lemma, upos, dependency)` returns today
(`internal/selection/selection.go:281-285`), and the characterization OCC-11
holds. `OccurrenceEligible` becomes a wrapper over the new function. No consumer
calls it with an exact occurrence.

### Failure classes

| family | example codes | class | raised by | consequence |
| --- | --- | --- | --- | --- |
| Package integrity and limits | `package_too_large`, `package_too_many_members`, `package_expanded_too_large`, `package_inventory_mismatch`, `package_entry_unsafe` | permanent | acquisition (§5, §11, §12) | The request fails and no evidence row is written. |
| Hash and inventory | transport-digest or content-digest mismatch against the pins | permanent | acquisition | As above. |
| Rights | closed operation gate (#2054–#2056) | permanent | §11 | The request fails before acquisition. |
| Binding and scope | `binding_rejected`, `binding_nondeterministic`, `scope_mismatch`, `extracted_text_mismatch`, `language_mismatch`, `correspondence_failed`, `projection_failed` | permanent | adapter, validation, §5 | No analysis. |
| Required scope | `coverage_gap`, `occurrence_unresolved` | permanent for that analysis | validation | Validated but not Vocabulary-ready. Never adopted. A new interpretation or evidence is a new request. |
| Unsupported mapping or version | `contract_version_unknown`, `interpretation_version_unknown`, `adapter_version_unsupported`, `normalization_profile_rejected`, `discriminator_scheme_unknown`, `reason_code_unknown` | permanent | adapter, validation | Nothing is written (EVD-16, EVD-33). |
| Structure and identity | `span_*`, `sentence_*`, `boundary_incomplete_interior`, `container_malformed`, `component_outside_container`, `node_order`, `occurrence_set_mismatch`, `accounting_missing`, `non_vocabulary_unsupported`, `identity_upos_mismatch`, `upos_invalid`, `citation_form_invalid`, `discriminator_in_lemma`, `discriminator_half_set`, `discriminator_value_invalid`, `canonical_key_mismatch`, `surface_span_mismatch`, `root_malformed`, `syntax_endpoint_invalid`, `native_parse_failed` | permanent | adapter, validation | Rejected. No analysis. |
| Node mapping findings | `mapping_unsupported`, `upos_mapping_ambiguous`, `lemma_absent`, `discriminator_unmapped`, `native_class_unresolved` | no failure in itself | adapter | They lead to `occurrence_unresolved` (permanent) only when they leave an occurrence unresolved. |
| Optional layers | `syntax_absent`, `root_missing`, `root_multiple`, `tree_cycle`, `tree_disconnected`, `head_unresolved`, `relation_unmapped`, `morphology_unmapped`, `morphology_ambiguous`, `secondary_unmapped`, `unit_extent_cuts_sentence`, `source_boundary_undeclared`, `claim_not_supported` | no failure | validation | The capability is withheld at that scope. Vocabulary-ready is unaffected. |
| Infrastructure | `storage_unavailable` (connection loss, serialization failure, lock timeout), `worker_interrupted` (shutdown, job timeout) | **transient** | §11 worker | Retried within the §11 budget. When the budget is exhausted, the request fails with `retry_budget_exhausted`, which is permanent for that request only. |
| Stale request | `expected_state_changed` | no failure | §11 | A stale outcome. Nothing changes (§11). |

A permanent failure stops at once and consumes no transient retry (handoff
"Initial operational envelope"). `Validate` and adapters never return a
transient class. The Stanza path keeps its own failures unchanged, including
`ErrDependencyParsingUnavailable` (`internal/analysis/jobs.go:42`, `:1902-1907`).

### Stanza compatibility adapter

**Purpose.** Run Stanza output through the same validation interface as an
**observation only**, so that parity between the two paths can be measured. It
is not a publication path.

**Mapping** (`internal/interpretation/stanzacompat`, adapter ID
`stanza-compat/1`). It produces one candidate per source document of the run,
because `SourceLocation` offsets are per document
(`internal/analyzer/analyzer.go:90-98`). The extent is the document text the
job sent to the analyzer (`internal/analysis/jobs.go:1176-1180`, offsets made
document-relative at `:1345-1350`).

| `analyzer.Result` | candidate |
| --- | --- |
| `Language` | language. The declared scope is `legacy(run, source document)`, not a binding. `internal/parity` builds it from the run and from the document text the job analyzed, computing that text's digest and code-point length. SCOPE-1 compares the candidate with this declaration, and COR-1 checks the digest. SCOPE-2 does not apply, because there is no binding. |
| `NormalizationProfile` | The normalization slot of a fixed compiled compatibility interpretation. That interpretation is never persisted as an Interpretation configuration. IDV-2 resolves it with `Lookup`. Its `morphology_mapping` slot names a compiled compatibility mapping: the reviewed identity mapping over the UD FEATS that the producer emits. So the `Morphology` row below goes through the slot, as §7 requires. |
| `Analysis` (analyzer name and version) | Lineage only. It never reaches consumers. |
| `Sentence` | An Analysis sentence with one span from its `Location`, boundary `complete` (as §7's legacy adapter), and `Text` checked by SENT-4. |
| `Token` | A node of kind `punctuation` when `UPOS = PUNCT`, otherwise `word`. Raw lemma and citation form are `RawLemma`. The identity is `(Language, CanonicalLemma, UPOS)` with no discriminator. Class `vocabulary` for words, `non_vocabulary(punctuation)` for punctuation. One span from `Location`. Surfaces are declared **not verbatim**, because the producer strips punctuation (`internal/analyzer/analyzer.go:51-53`), so COR-2 does not apply. |
| `Dependency`, `Head` | One basic syntax edge per token. The relation is `Dependency`, and the head is the token at `Head` in the same sentence. A token whose `Head` is its own ordinal and whose `Dependency` is `root` maps to a self-headed `root` edge. The observation runs only after `persistSnapshotWork` has committed, so every token has passed `validateCorpusToken`: a non-empty dependency and an in-range head (`internal/analysis/jobs.go:1851-1862`). No token without a dependency reaches the adapter. If one did, it would get no edge, never a default. |
| `Morphology` | Native morphology, and FEATS as given (as §7's legacy adapter). |
| Multiword tokens | None. The producer flattens them into words that share the token's span (`nlp/src/mouseion_nlp/producer.py:255-262`). They become `word` nodes with a shared span and no container. |

**Hook.** `internal/analysis` defines a `PublicationObserver` interface that
takes the merged `analyzer.Result`, the run reference and the document extents.
`cmd/server` wires `internal/parity` into it. `internal/parity` converts the
`Result` into the compatibility adapter's own input type, so the adapter never
imports `internal/analyzer`. The worker calls it once, after
`persistSnapshotWork` has committed and before `selectSnapshotRun`
(`internal/analysis/jobs.go:1491-1498`):

- on a context detached from cancellation, with its own time budget;
- with every error and panic recovered and reported, never returned;
- only when observation is enabled (OD-45).

The job's return value, retries, run state, transactions and every row it
writes are identical whether observation is on, off, failing or timing out
(VAL-09).

**What stays unchanged.**

- `persistSnapshotWork`, `corpusTokenInserter`, `validateCorpusToken`,
  `publishEligibleAnalysis` and the Browse projection build are unchanged
  (`internal/analysis/jobs.go:1362-1394`, `:1713-1743`, `:1789-1862`).
- Every `corpus_tokens`, `corpus_sentences`, `analysis_runs`, `corpora` and
  `book_current_analyses` row is written exactly as today.
- **No stored analysis is rewritten.** The observer and the parity test write
  no row anywhere: no Exact analysis, no §7 row, no diagnostic, no restriction.
- **No cutover happens in this design.** No Book changes representation, and
  no setting routes Stanza output into Exact analyses or changes which adapter
  the learner core reads for a modern-language Book. Any cutover needs its own
  approved design.

The §7 legacy *read* adapter and this compatibility adapter are different
things. The legacy read adapter serves stored `corpus_*` rows to consumers. The
compatibility adapter maps a fresh `analyzer.Result` into a candidate for
observation. Neither writes.

**Parity comparison.** Both sides start from the same `analyzer.Result` and the
same `selection.DefaultConfig`, with no learner decisions.

| # | compares | legacy side | canonical side | equal when |
| --- | --- | --- | --- | --- |
| P-1 | Vocabulary identities | identities of `selection.Project(result, cfg, nil)` | identities of the validated analysis's contributing occurrences (EFF-2) | the sets are equal under §6's legacy-to-rich comparison (null discriminator, byte-equal fields) |
| P-2 | Eligible count per identity | `Candidate.OccurrenceCount` | occurrences counted through `selection.EligibleOccurrence` with the read interface's relation layer | equal for every identity |
| P-3 | Coverage denominator | `AnalyzableStatistics(...).AnalyzableTokenCount` | EFF-3 denominator | equal |
| P-4 | Sentence evidence per identity | `SentenceReferences`: index, document, sentence span, sentence text | the occurrences' sentences: ordinal, document, span, text sliced from the extent | equal as ordered lists, field by field |

Parity compares Go values field by field, never serialized JSON. Observed forms,
morphology and diagnostics are not parity fields.

**Mismatch categories.** These are reported, never acted on:
`candidate_rejected` (with the rejection codes), `vocabulary_not_ready` (with
codes), `identity_only_legacy`, `identity_only_canonical`, `count_differs`,
`denominator_differs`, `sentence_differs`, and `syntax_withheld_changed_eligibility`.
The last one counts identities whose P-2 difference comes only from a sentence
whose basic syntax validation withheld, where OD-03 then skipped a syntax-based
exclusion.

**Reporting.** A mismatch is reported as:

- one structured log event per observed run: the run ID, the language, the
  outcome, and a count per category and per code;
- a counter metric per category.

No lemma, surface, sentence text, document ID or title appears in either. The
detailed list (which identities differ) exists only in memory and in test
failure messages. The observer writes no row, raises no error, changes no
status and shows nothing to learners. A mismatch therefore cannot change
publication, counts, selection or any learner outcome.

**Parity test** (`internal/parity`, plain `go test`, no database or NLP). For
each characterization fixture (Current anchors), with the exact document text
the producer received:

- The German protobuf corpus, the Italian and Greek regression JSON, and the
  inline Italian and Greek corpora in `internal/selection/selection_test.go`.
- Fixtures that carry no document text get a sibling text file holding the
  producer test's exact input, for example `first + " " + second` at
  `nlp/tests/test_producer.py:952-954`. A fixture whose input cannot be
  recovered exactly is left out and listed in the test with the reason. Text is
  never reconstructed from token surfaces.

The test asserts that P-1 to P-4 are equal and that the outcome is Validated and
Vocabulary-ready, with no mismatch category reported (VAL-08). A fixture that
legitimately diverges, for example because its tree fails TREE-3, does not get
an exception. Its expectation names the category and count, and changing it
needs review. The test also asserts that the legacy-side values equal the
fixture's existing expectations, so parity can never pass by changing the
legacy path.

### NLP independence

These are the §3 entry points that gate on NLP, or are reachable on the Reading
path:

| # | entry point | evidence (path:line) | gate today | required change |
| --- | --- | --- | --- | --- |
| N-1 | Analysis worker readiness check | `internal/analysis/jobs.go:1256`, `:1909-1921` | NLP depparse capability for the language | None. Only `analyze_corpus` jobs reach it, and only EPUB source materials enqueue them (`:158`, `:422`). Adoption (§11) never enqueues one, and never constructs an `analyzer.AnalyzeRequest`. The gate applies only to producer-backed analysis. |
| N-2 | Analyzer call (B-05) | `internal/analysis/jobs.go:1345` | NLP service answering | None. Producer-backed only. The parity observer runs after it and never calls NLP. |
| N-3 | Catalogue sync language scope and supported languages (B-27, B-28) | `internal/cataloguesync/sync.go:1018-1034`, `:897-910`, `:494-498`, `:553-557` | NLP capability readiness | None for OPDS. Classical discovery (handoff Stage 3, OD-09) and the §12 local entry point never call `GetCapabilities`, `ReadySupportedLanguages` or `eligibleLanguages`. A classical Book's availability is its reviewed definition and its validated analysis. |
| N-4 | Reading gates (B-25, B-26) | `internal/webapp/analysis_evidence.go:58-60` and the call sites in B-26 | A published analysis run and corpus (§3 correction: not an NLP check, but there is no annotation-backed path) | Legacy-representation Books: unchanged. Classical Books gate on validated capability: the Book is Reading-ready (above), decided by the single domain classification (ADR 0052) from the §5 current record. No gate reads NLP capabilities. How the call sites reach that classification is OD-05 (§11). |
| N-5 | Server start-up | `cmd/server/main.go:94`, `:171`; `internal/analyzer/grpc.go:33` | None: the client connects lazily | Must stay that way. No readiness probe of NLP may block serving or block the River workers that adoption uses. |
| N-6 | Lemma review language support | `internal/lemmareview/lemmareview.go:466-469` | The canonicalization registry, not NLP | None for NLP. §6 I-5 governs the profile. |

An annotation-backed Book is therefore acquired, validated, adopted, read,
started, ended and finished with the NLP service stopped (VAL-11). A
modern-language Book still needs NLP readiness to be analyzed, exactly as today
(VAL-12).

### Store-contract harness (OD-04)

OD-04 is resolved as option (1) for the **harness**, combined with §8's
separate **rich scenario list**. As with every recommendation in §17, its
status stays open until §18 records approval.

- `storecontract.Run` takes the harness factory as today and runs `Scenarios()`
  (unchanged, `Language = "de"`, tuple seeds). It then runs §8's
  `richScenarios(v)` once for each variant in `RichVariants()`. Those are
  `Variant{Language: "grc"}`, and a synthetic `Variant{Language: "xx"}` that
  uses the casefold fallback, so the same verdicts are shown not to depend on
  the language (VAL-20).
- `Harness` gains `RichSeeds() RichSeeder`. A rich Book is seeded from a
  synthetic `canonical.Candidate` built by a test-only builder in
  `internal/storecontract`. Each adapter passes it through `canonical.Validate`,
  and seeding fails the test unless the result is Validated. The PostgreSQL
  adapter then adopts it through the §11 interface. The fixtures adapter stores
  the validated analysis in memory. So both adapters hold only validated
  analyses, as production does.
- `internal/storecontract` may import `internal/canonical`. It may not import
  `internal/interpretation/...`: seeds never run a real adapter.
- The existing `de` scenarios do not run against `grc`. Their seeds are tuples,
  and running them under a rich variant would test the tuple path twice. The
  rich list covers the same verdicts (LRN-19).

### Invariants

Each invariant is covered by the `VAL` rows in [§15](#15-acceptance-test-matrix).

1. **One validator.** Every candidate, imported or model-produced, is decided by
   the same `Validate`. Equal defects give equal diagnostics (VAL-01). Validation
   is deterministic (VAL-15).
2. **Claims are not proof.** A producer claim never raises a capability state
   (VAL-02, VAL-03). Only a validated, Vocabulary-ready, unrestricted analysis is
   adopted (VAL-22).
3. **Optional failures are local.** A failed optional rule withholds only its
   capability at its scope. Vocabulary readiness is unchanged (VAL-02).
4. **Scope.** A candidate that does not cover its declared unit is not
   Vocabulary-ready, with a `coverage_gap` diagnostic. A candidate for another
   unit, binding or language is rejected (VAL-04, VAL-05).
5. **Rules.** Identity, boundary and correspondence rules reject as catalogued
   (VAL-16, VAL-17, VAL-18).
6. **Versions.** An unknown contract, interpretation or adapter version is a
   permanent failure that writes nothing and is not retried (VAL-06).
7. **Failure classes.** Only infrastructure failures are retried, within budget
   (VAL-19).
8. **Diagnostics.** No diagnostic contains source text or any other input
   string (VAL-07).
9. **Stanza compatibility.** Parity holds on the characterization fixtures
   (VAL-08). Observation never changes behaviour (VAL-09), rewrites no stored
   analysis, and switches nothing (VAL-10).
10. **NLP independence.** An annotation-backed Book becomes Reading-ready and
    completes a reading with NLP stopped (VAL-11). A modern-language Book still
    requires NLP readiness (VAL-12).
11. **Layering.** No learner-core or adoption package imports an adapter
    package, and adapters cannot reach the database, network, subprocesses or
    plugins (VAL-13, VAL-14).
12. **Consumers.** Consumers decide from representation and effective capability
    state only. Producer and language never change a verdict (VAL-20), and a
    restriction makes a capability unavailable at read time (VAL-21).
13. **Adapters.** An adapter is deterministic, keeps every attested node, and
    never invents a default (VAL-23).

### Conflicts and dependencies noted

- **SPAN-4 and TREE-5.** §7 lists TREE-5 among the rules that reject a
  candidate. A candidate alone cannot show that a span came from a length or
  that a root was fabricated, so `Validate` cannot enforce either rule. This
  section treats both as adapter obligations, enforced by each adapter's
  reviewed fixtures and, for spans, partly by COR-2. §7 is not rewritten.
- **"Missing scope gives a rejection."** Issue #2071 asks for a rejection. §7
  makes a coverage gap (ACC-4) deny Vocabulary-ready instead of rejecting the
  candidate. This section follows §7: the analysis is validated but not
  Vocabulary-ready, carries `coverage_gap`, and is never adopted. A candidate
  declaring a different unit or binding is rejected outright (SCOPE-1). VAL-04
  and VAL-05 assert both. The choice is recorded as OD-47.
- **New rule IDs.** VER-1 to VER-4, SCOPE-1 to SCOPE-3, COR-1, COR-2, IDV-1 to
  IDV-5 and BND-1 are added here. They do not change any §7 rule.
- **B-25 and B-26 are not NLP gates** (§3 correction). They are still listed
  under NLP independence, because they are the gates an annotation-backed Book
  must pass with NLP stopped. Their mechanism is OD-05 (§11).
- **VAL-13 and VAL-14 use a lint seam.** The ticket requires a dependency test
  or lint rule for the layering. That is not one of seams (a) to (d), so those
  two rows name `make lint-go` (depguard) and the transitive dependency test
  (see Package placement) as their seam. The deviation is OD-48.
- **ADR 0091.** New package groups, the adapter allow-list and a new pgx
  importer (`internal/adoption`) need an ADR 0091 amendment (OD-43). That
  amendment needs human review under CODEOWNERS.
- §8's "the preview interface is §10's" is answered in the learner-core table:
  the rich preview fingerprint covers the current analysis ID and the exact
  decisions and flags.
- §5 `vocabulary_ready` equals this section's Vocabulary-ready definition.
  Restrictions are §5's and are applied at adoption and read time.
- The rich store-contract scenario names in §8 apply unchanged under the OD-04
  resolution above.

## 11. Durable preparation and atomic adoption

Verified at commit `669cf4b5165212c4422b3eac57b5851d37399e32` (`main`). Every
`path:line` in this section refers to that commit. River is v0.49.0
(`go.mod:11`); River citations name files in that module version.

### Summary

- A **Preparation request** is the durable, owner-scoped intent to prepare one
  pinned choice for one classical Book. It records the pinned choice, its kind
  (`initial` or `replacement`), the evidence that was current when it was made,
  an owner approval for a replacement, its state, its deadline and one attempt
  counter per stage.
- **Admission** creates the request and inserts its first River job in one
  transaction, under the installation **capacity gate**. Over capacity, the
  request is still written, in state `failed` with an explained reason, and
  nothing is dropped.
- Work runs in five **stages**: verify package → interpret → validate →
  prepare projections → adopt. Each stage attempt is one River job that names
  its attempt number. The attempt counters live on the request row, and a job
  runs only if it names the next attempt, so duplicate delivery and restarts
  cannot reset or skip them. A periodic **reconciler** resumes interrupted
  attempts and applies the deadline.
- Stage work runs outside the HTTP request and outside every transaction.
  Each stage opens and closes with a short transaction.
- **Adoption** is one transaction. It locks the Book, re-checks authorization,
  the request and the expected evidence, then writes the Adopted evidence
  record (§9), the Book current classical analysis (§5) and the ready count
  projection (§8) together. If any check fails, nothing is written and the old
  evidence stays current.
- Leaving To Read cancels that owner's outstanding requests for that Book.
  Choosing a new candidate supersedes the running one. Neither deletes
  evidence.
- Adoption writes only request, current-evidence, projection and retention
  rows. It cannot change snapshots, reservations, Known, history, deck records
  or decisions.

New record and rule names are proposed terms (OD-55). Table names in the
illustrative DDL are not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| Analysis submission takes a per-owner advisory lock, locks the To Read Book, and creates the run, its River job (`InsertTx`), the display handle and the first attempt row in one transaction. | `internal/analysis/jobs.go:238-270` (lock `:247`, To Read check `:250`), `:416-431` (`InsertTx` `:422`) | **Reused pattern.** Admission writes the request and inserts its River job with `InsertTx` in the same transaction (ADR 0097 "Durable bounded work"). |
| River retries are disabled for analysis jobs (`MaxAttempts: 1`). Each retry increments `analysis_runs.attempt_count` and inserts a new job and attempt row in one transaction. | `internal/analysis/jobs.go:130-166` (counter `:152-155`, insert `:158`, attempt row `:162`) | **Reused pattern.** The attempt budget is a database fact on the request. River's own attempt number is never the budget. |
| `admitToReadBookAnalysis` locks the Book row and refuses a Book that is the current reading or is not To Read. | `internal/analysis/jobs.go:277-306` | Admission and adoption apply the same two checks under the same Book row lock. |
| `publishCompletedRun` takes the owner lock, locks run and attempt, skips a cancelled attempt, locks the Book, and publishes only through `publishEligibleAnalysis`. | `internal/analysis/jobs.go:1644-1680`, `:1684-1709` | The adoption transaction follows the same shape: lock, re-check the state that cancellation writes, then publish. |
| `publishEligibleAnalysis` checks the run is still current and that `noNewerAnalysisRun` holds, builds the Browse projection, and upserts `book_current_analyses`, all in one transaction. | `internal/analysis/jobs.go:1713-1743`, `:168-176` | **Precedent, extended.** Supersession here is decided by request state and an explicit expected-evidence check, not by display number or creation time. |
| Cancellation commits the durable state first, then cancels River jobs best effort. The cancelled state fences a racing worker. | `internal/analysis/jobs.go:788-797`, `:803` | Cancellation here works the same way. |
| Failure recording uses `context.WithoutCancel`, so a cancelled job context still records its failure. | `internal/analysis/jobs.go:1217-1218` | Stage failure and interruption recording do the same. |
| The River client has one worker each for the analysis, catalogue sync and prepared-deck queues, and a client-wide job timeout that defaults to 30 minutes. `RescueStuckJobsAfter` is not set. | `internal/analysis/jobs.go:2017`, `:36`, `:2020-2030` | A new queue, `classical_preparation`, with `MaxWorkers: 1`. That bounds one process only. The installation bound is the capacity gate. River's stuck-job rescuer waits 1 hour by default (`client.go:391-397`), and with `MaxAttempts: 1` a rescued job is not retried, so §11 recovers interrupted attempts with its own reconciler. |
| `river.JobSnooze` reschedules a job without incrementing its attempt. | River `error.go:31-43` | Waiting for capacity snoozes, keeps the job's attempt number and consumes no budget. |
| Lemma decisions and Start take the learner-state lock (salt 194), then the Book row lock, then the per-Book fence (salt 193). The Browse publication takes the same fence. | `internal/persistence/lemma_review.go:192-205`, `:256-259`; `internal/persistence/current_reading_lifecycle.go:511-516`; `internal/persistence/vocabulary_browse_counts.go:330-336` | Adoption takes the Book row lock and then the per-Book fence, a subsequence of that order. It takes no learner-state lock, because it writes no learner state. |
| A fixed-key transaction advisory lock already serializes first-user creation. | `internal/persistence/persistence.go:346` | Precedent for the capacity gate (OD-49). |
| The To Read handler commits the disposition change, then submits EPUB analysis in a separate step. | `internal/webapp/library_handlers.go:289-307` | For classical Books, admission runs inside the disposition transaction, so To Read intent and its request commit together. EPUB behaviour is unchanged. |
| Disposition changes lock the Book row and the disposition state. Finish returns the Book to Inbox. | `internal/persistence/book_dispositions.go:54-91`; `internal/persistence/current_reading.go:345-349` | Both paths leave To Read, so both cancel the Book's outstanding requests in their own transaction. |

### Records

#### Preparation request

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One owner's durable intent to prepare one pinned choice for one classical Book, with its bounded progress and outcome. | **Identity:** `id uuid`; `owner_id uuid`; `book_id uuid`; `sequence bigint` (1, 2, … per (owner, Book), assigned under the Book row lock); `kind text` ∈ {`initial`, `replacement`}; `retry_of uuid NULL`. **Pinned choice:** `pin_set text` (pin-set identifier and version, §5 "Pins"); `origin_kind text`, `origin_ref text`, `package_release text NULL` (as §5 Package acquisition); `package_digest text` (transport identity); `package_content_digest text` (manifest identity); `reading_unit_id uuid`; `binding_declaration_digest text` (SHA-256 of the canonical serialization of `source_bounds`, `projection` and `correspondence` from the pin set); `interpretation_digest text` (the §5 `configuration_digest`); `adapter text` (`name/version`). **Expected evidence:** `expected_analysis_id uuid NULL`; `expected_adoption_request_id uuid NULL`. **Approval:** `approved_at timestamptz NULL`. **Progress:** `state text` ∈ {`queued`, `running`, `blocked`, `failed`, `cancelled`, `superseded`, `adopted`}; `stage text` ∈ {`verify`, `interpret`, `validate`, `prepare_projections`, `adopt`}; `attempts_verify`, `attempts_interpret`, `attempts_validate`, `attempts_prepare`, `attempts_adopt smallint` (each 0–3); `lease_expires_at timestamptz NULL` (set while a stage attempt is in its work step); `next_attempt_at timestamptz NULL`. **Resolved inputs** (write-once, set by stages): `text_acquisition_id`, `annotation_acquisition_id`, `binding_id`, `interpretation_id`, `candidate_analysis_id uuid NULL`; `staged_builder_version integer NULL`; `staged_coverage_denominator bigint NULL`. **Outcome:** `reason text NULL` (registered code); `diagnostics jsonb` (array in the §10 diagnostic shape, default `[]`); `superseded_by uuid NULL`. **Times:** `created_at`; `deadline_at` (= `created_at` + 24 h); `updated_at`; `finished_at NULL`; `adopted_at NULL` | `id` | `(owner_id, id)` unique, as a composite-reference target. `(owner_id, book_id, sequence)` unique. **At most one non-terminal request per (owner, Book):** partial unique index on `(owner_id, book_id)` where `state IN ('queued','running')`. | Owner. Another owner's requests are never read, written, counted per owner or cancelled. | **Mutable** in its progress and outcome fields only. Identity, pinned choice, expected evidence, `approved_at`, `created_at` and `deadline_at` are immutable after insert. Each resolved input is write-once. Attempt counters only increase. A terminal state never changes. A trigger enforces all of these and the transition table below. Deleted only by the owner cascade (RET-GUARD-2). | `owner_id` → `users(id)` `ON DELETE CASCADE`; `(owner_id, book_id)` → §4 Book–Reading unit association `ON DELETE CASCADE`, under the owner guard; `(owner_id, retry_of)`, `(owner_id, expected_adoption_request_id)` and `(owner_id, superseded_by)` → Preparation request `NO ACTION`; `(owner_id, text_acquisition_id)` and `(owner_id, annotation_acquisition_id)` → §5 Package acquisition `NO ACTION`; `binding_id` → §5 binding `RESTRICT`; `interpretation_id` → §5 Interpretation configuration `RESTRICT`; `candidate_analysis_id` → §5 Exact analysis `RESTRICT` (§9 R2) |

Checks:

- `kind = initial` ⇔ `expected_analysis_id` and `expected_adoption_request_id`
  are both NULL. A replacement names both.
- `kind = replacement` ⇒ `approved_at IS NOT NULL`. A replacement row without an
  owner approval cannot exist.
- `state = adopted` ⇔ `adopted_at IS NOT NULL`.
- Terminal state ⇔ `finished_at IS NOT NULL`. `blocked`, `failed`, `cancelled`
  and `superseded` require `reason`.
- `state = superseded` with `reason = superseded_by_request` ⇔ `superseded_by IS NOT NULL`.
- `lease_expires_at` is NULL in every state except `running`.
- At most one request in the installation is `running` (a partial unique
  index on a constant). This is the database backstop for the active bound.
- `staged_builder_version` and `staged_coverage_denominator` are both set or
  both NULL.
- `deadline_at = created_at + interval '24 hours'`.

`reason` and `diagnostics` hold only registered codes and the §10 diagnostic
shape. They are safe to log and carry no source text, digests or learner input.

#### Preparation attempt

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The history of one stage attempt, for diagnostics. | `owner_id`; `request_id`; `stage text`; `attempt_number smallint` (1–3); `river_job_id bigint`; `outcome text NULL` ∈ {`succeeded`, `transient`, `permanent`, `interrupted`, `discarded`}; `code text NULL`; `started_at`; `finished_at NULL` | `(request_id, stage, attempt_number)` | Key only. A fourth attempt for one stage cannot be inserted. | Owner. | **Append-only.** Only `outcome`, `code` and `finished_at` change, once, together. | `(owner_id, request_id)` → Preparation request `ON DELETE CASCADE` |

A trigger requires `attempt_number` to equal the request's counter for that
stage at insert. The counter on the request is the budget. The attempt row
explains it.

#### Staged count row

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One identity's effective count for the candidate analysis, computed before adoption. | `owner_id`; `request_id`; `identity_id`; `occurrence_count bigint` (> 0); `corrected boolean` | `(request_id, identity_id)` | Key only. | Owner. | **Mutable** by the prepare stage only, which replaces the whole set. Deleted when the request becomes terminal. Derived data: never retained evidence. | `(owner_id, request_id)` → Preparation request `ON DELETE CASCADE`; `identity_id` → §6 Vocabulary identity `RESTRICT` |

```sql
-- illustrative, not a migration
CREATE TABLE preparation_requests (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  book_id uuid NOT NULL,
  sequence bigint NOT NULL CHECK (sequence > 0),
  kind text NOT NULL CHECK (kind IN ('initial','replacement')),
  retry_of uuid NULL,
  pin_set text NOT NULL,
  origin_kind text NOT NULL, origin_ref text NOT NULL, package_release text NULL,
  package_digest text NOT NULL, package_content_digest text NOT NULL,
  reading_unit_id uuid NOT NULL,
  binding_declaration_digest text NOT NULL,
  interpretation_digest text NOT NULL,
  adapter text NOT NULL,
  expected_analysis_id uuid NULL,
  expected_adoption_request_id uuid NULL,
  approved_at timestamptz NULL,
  state text NOT NULL CHECK (state IN ('queued','running','blocked','failed','cancelled','superseded','adopted')),
  stage text NOT NULL CHECK (stage IN ('verify','interpret','validate','prepare_projections','adopt')),
  attempts_verify smallint NOT NULL DEFAULT 0 CHECK (attempts_verify BETWEEN 0 AND 3),
  attempts_interpret smallint NOT NULL DEFAULT 0 CHECK (attempts_interpret BETWEEN 0 AND 3),
  attempts_validate smallint NOT NULL DEFAULT 0 CHECK (attempts_validate BETWEEN 0 AND 3),
  attempts_prepare smallint NOT NULL DEFAULT 0 CHECK (attempts_prepare BETWEEN 0 AND 3),
  attempts_adopt smallint NOT NULL DEFAULT 0 CHECK (attempts_adopt BETWEEN 0 AND 3),
  lease_expires_at timestamptz NULL, next_attempt_at timestamptz NULL,
  text_acquisition_id uuid NULL, annotation_acquisition_id uuid NULL,
  binding_id uuid NULL REFERENCES text_annotation_bindings(id) ON DELETE RESTRICT,
  interpretation_id uuid NULL REFERENCES interpretation_configurations(id) ON DELETE RESTRICT,
  candidate_analysis_id uuid NULL REFERENCES exact_analyses(id) ON DELETE RESTRICT,
  staged_builder_version integer NULL, staged_coverage_denominator bigint NULL,
  reason text NULL, diagnostics jsonb NOT NULL DEFAULT '[]', superseded_by uuid NULL,
  created_at timestamptz NOT NULL, deadline_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL, finished_at timestamptz NULL, adopted_at timestamptz NULL,
  UNIQUE (owner_id, id),
  UNIQUE (owner_id, book_id, sequence),
  FOREIGN KEY (owner_id, book_id) REFERENCES book_reading_units(owner_id, book_id) ON DELETE CASCADE,
  FOREIGN KEY (owner_id, text_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id),
  FOREIGN KEY (owner_id, annotation_acquisition_id) REFERENCES evidence_package_acquisitions(owner_id, id),
  FOREIGN KEY (owner_id, retry_of) REFERENCES preparation_requests(owner_id, id),
  FOREIGN KEY (owner_id, expected_adoption_request_id) REFERENCES preparation_requests(owner_id, id),
  FOREIGN KEY (owner_id, superseded_by) REFERENCES preparation_requests(owner_id, id),
  CHECK ((kind = 'initial') = (expected_analysis_id IS NULL)),
  CHECK ((expected_analysis_id IS NULL) = (expected_adoption_request_id IS NULL)),
  CHECK (kind = 'initial' OR approved_at IS NOT NULL),
  CHECK ((state = 'adopted') = (adopted_at IS NOT NULL)),
  CHECK ((state IN ('queued','running')) = (finished_at IS NULL)),
  CHECK (state IN ('queued','running','adopted') OR reason IS NOT NULL),
  CHECK (state = 'running' OR lease_expires_at IS NULL),
  CHECK ((staged_builder_version IS NULL) = (staged_coverage_denominator IS NULL)),
  CHECK (deadline_at = created_at + interval '24 hours')
);
CREATE UNIQUE INDEX preparation_requests_one_open_per_book
  ON preparation_requests (owner_id, book_id) WHERE state IN ('queued','running');
CREATE UNIQUE INDEX preparation_requests_one_running
  ON preparation_requests ((true)) WHERE state = 'running';
CREATE INDEX ON preparation_requests (state, deadline_at) WHERE state IN ('queued','running');
-- plus: the transition and write-once trigger; the §9 owner guard
-- (reject_retention_root_deletion); preparation_attempts and staged_count_rows
-- as tabled above; indexes led by (owner_id, text_acquisition_id) and
-- (owner_id, annotation_acquisition_id) for the owner cascade (§9 "Indexes").
```

### State transitions

Only these transitions are allowed. The trigger rejects every other one.
`blocked`, `failed`, `cancelled`, `superseded` and `adopted` are terminal.
Every transition into a terminal state clears `lease_expires_at`, sets
`finished_at` and deletes the request's Staged count rows.

| from | to | when | written by |
| --- | --- | --- | --- |
| (insert) | `queued` | Admission within capacity. | Admission |
| (insert) | `failed` | Admission over capacity, with `installation_queue_full` (OD-51). | Admission |
| `queued` | `running` | A stage attempt begins and no other request is `running`. | Stage Begin |
| `running` | `queued` | A transient failure or an interruption leaves budget and deadline, so the next attempt is scheduled after backoff. The active slot is released. | Stage Finish, reconciler |
| `running` | `running` | A stage succeeds and the next stage's job is inserted. The request keeps the active slot between stages. | Stage Finish |
| `running` | `blocked` | A permanent failure that needs intervention or another choice (see [Outcomes](#outcomes-and-retry)). | Stage Finish, adoption |
| `queued`, `running` | `failed` | The stage budget or the deadline is exhausted, or adoption finds the Book is the current reading (OD-50). | Stage Begin, stage Finish, adoption, reconciler |
| `queued`, `running` | `cancelled` | The Book leaves To Read. | Disposition change, Finish of a Reading |
| `queued`, `running` | `superseded` | A newer request for the same Book is admitted, or adoption finds stale request or evidence state. | Admission, adoption |
| `running` | `adopted` | The adoption transaction commits. | Adoption |

### Stages

Every stage is one or more **stage attempts**. Each attempt is one River job
whose arguments name the request, the stage and the **attempt number**. A job
is **current** only while the request is open, its `stage` equals the job's
stage, and the job's attempt number is exactly one more than the stage
counter (in Begin) or equal to it with that attempt still open (in Finish).
Every other delivery is stale: it finishes and changes nothing.

Each attempt has three parts:

1. **Begin** (short transaction). Lock the request `FOR UPDATE`.
   - If the job is not current, finish the job and change nothing. A duplicate
     delivery, a second copy of the same job, or a job left over from an
     earlier attempt ends here.
   - If the deadline has passed, set `failed` with `deadline_exceeded`.
   - If the request is `queued`, take the capacity gate (see
     [Capacity](#capacity)). If another request is `running`, snooze, and
     change nothing. Otherwise set `running`.
   - Increment the stage counter, insert the Preparation attempt row, and set
     `lease_expires_at` to now + stage timeout + 1 minute. Commit.
2. **Work** (no transaction). Run the stage under a context with the 5-minute
   stage timeout. The work reads only persisted rows and pins, and holds no
   database lock.
3. **Finish** (short transaction, with `context.WithoutCancel`, as at
   `internal/analysis/jobs.go:1218`). Lock the request `FOR UPDATE`.
   - If the job is no longer current (the request was cancelled or
     superseded, or the reconciler closed this attempt), write no stage
     output. Close the attempt as `discarded` if it is still open.
   - **Success:** write the stage's outputs, close the attempt as `succeeded`,
     clear the lease, advance `stage`, and insert the next stage's job with
     attempt number 1 using `InsertTx`. The request stays `running`.
   - **Transient** (`storage_unavailable`, or `worker_interrupted` from the
     stage timeout or a shutdown): close the attempt as `transient` or
     `interrupted`, and clear the lease. If the counter is 3, set `failed` with
     `retry_budget_exhausted`. If the backoff delay would end after the
     deadline, set `failed` with `deadline_exceeded`. Otherwise set `queued`
     with `next_attempt_at`, and insert the job for the next attempt number,
     scheduled at `next_attempt_at`, using `InsertTx`.
   - **Permanent:** close the attempt as `permanent`, and set `blocked` or
     `failed` with the diagnostics.
   - Commit.

The counter is incremented and committed in Begin, before any work, and only a
job naming the next attempt number can increment it. A crash during Work
therefore leaves the attempt counted. A duplicate delivery, a restart or a
re-inserted job can neither give a stage a fourth attempt (the counter's
database ceiling is 3) nor reset or skip one.

| stage | inputs | outputs (written in Finish) | idempotency and resume rule |
| --- | --- | --- | --- |
| `verify` | The pinned package source: the §12 local path, or a later catalogue transfer. The pins. The rights gate for the operation (#2054–#2056). | The §5 evidence rows of a verified package (Package content, members, blob, notices, revisions, provenance claims) and the owner's Package acquisition. Links `text_acquisition_id` and `annotation_acquisition_id`. | The rights gate is checked first. A closed gate gives `blocked` with `rights_gate_closed`, before any byte is read. Bytes are checked against the pins and the §5 limits in the work step, outside every transaction (§5 "Limits"). Every row is insert-if-absent on its digest key. The acquisition is insert-if-absent on its §5 unique tuple. A re-run after a committed Finish is a stale delivery. Later stages read the stored blob and never re-acquire it. |
| `interpret` | The stored blob, through the safe reader. The pins. | Interpretation configuration, insert-if-absent on `configuration_digest`. Extracted text and Text–annotation binding (§5), insert-if-absent. Links `interpretation_id` and `binding_id`. | The resolved configuration's digest must equal `interpretation_digest`, otherwise `blocked` with `interpretation_version_unknown`. A recomputed binding that differs from a stored one gives `blocked` with `binding_nondeterministic` (EVD-14). A `rejected` binding is stored for inspection and gives `blocked` with its reason codes. |
| `validate` | The stored blob, binding and interpretation. | When Validated: the Exact analysis with its five Capability diagnostics, Diagnostic findings and §7 rows, in one transaction (§5). Links `candidate_analysis_id`. | The adapter is run again on the stored members. It is deterministic (§10), so the re-run is the stage's resume. `canonical.Validate` decides the outcome. The analysis is insert-if-absent on (binding, interpretation, adapter). An existing row is reused only if its `vocabulary_ready` and diagnostics are equal, otherwise `blocked` with `analysis_nondeterministic`. Rejected: `blocked` with the diagnostics, and no analysis. Validated but not Vocabulary-ready: the analysis is stored for inspection, and the request is `blocked` with the denying codes (OD-54). Not adoptable because of a `vocabulary` Usability restriction: `blocked` with `evidence_restricted`. |
| `prepare_projections` | The candidate analysis, the Book's decisions on it (none for a never-adopted analysis, EFF-4) and the current rich builder version. | The whole Staged count row set, `staged_builder_version` and `staged_coverage_denominator` (EFF-2, EFF-3). | The set is replaced as a whole, so a re-run gives the same rows. Decisions on the candidate cannot change meanwhile, because a decision is written only on the current analysis (§8 EFF-4). |
| `adopt` | Everything above. | The adoption transaction below. | The stage uses the shared Begin, so its attempts are counted and leased like every other stage. Its Work is empty, and the adoption transaction is its Finish. If the adoption commits, the request is `adopted`. A re-run finds a terminal request and changes nothing. |

Rules shared by all stages:

- The River job carries only `owner_id`, `request_id`, `stage` and
  `attempt_number`. The pinned choice and the budget are read from the
  request, never from job arguments or River metadata.
- River jobs are inserted on queue `classical_preparation` with
  `MaxAttempts: 1`, as the analysis jobs are
  (`internal/analysis/jobs.go:158`, `:422`). River never retries a stage. Every
  retry is a new job inserted in the same transaction that records the failed
  attempt.
- A shutdown cancels the work context. Finish then records the interruption
  under `context.WithoutCancel`, as a transient failure, and schedules the
  next attempt.
- The worker sets its River timeout to the stage timeout plus 1 minute.
- All deadline and lease comparisons use one application clock value, passed
  into SQL as a parameter. Tests inject the clock and the jitter source.

### Reconciler

A crash or a killed process can stop a job between Begin and Finish, and River
then discards it (`MaxAttempts: 1`). The **preparation reconciler** recovers
such requests. It is a River periodic job on its own queue,
`classical_preparation_reconcile`, with `MaxWorkers: 1`, run every minute. In
one short transaction per request, with the request locked `FOR UPDATE`, it:

1. Sets every open request whose deadline has passed to `failed` with
   `deadline_exceeded`, closing any open attempt as `interrupted`.
2. For a `running` request whose `lease_expires_at` has passed, closes the open
   attempt as `interrupted`. That attempt stays counted. It then applies
   Finish's transient rule: `failed`, or `queued` with the next attempt's job.
3. For an open request with no open attempt and no live River job for its
   current stage and next attempt number, inserts that job. This recovers a
   job lost outside the request's own transactions. It consumes no budget,
   because the counter changes only in Begin.

The reconciler never cancels or supersedes a request, never changes a pinned
choice, and never raises a capacity count. It only applies each request's own
deadline and attempt rules, so touching another owner's request is not
cross-owner authority. A zombie worker that finishes after step 2 finds its
attempt closed and writes nothing.

### Budgets

These are the handoff's "Initial operational envelope" values. They are
binding defaults. The application reads configured values that can only be
lower. The database checks the attempt ceiling and the deadline arithmetic.

| scope | bound | enforcement |
| --- | --- | --- |
| Each stage | 3 total attempts, including the first, for transient failures | Per-stage counter on the request, `CHECK (… BETWEEN 0 AND 3)`, incremented in Begin. The third transient failure gives `failed` with `retry_budget_exhausted`. |
| Each request | 24-hour deadline from `created_at`, including queue, snooze, backoff and recovery time | `deadline_at`. Checked in Begin, before scheduling a retry, in adoption step 3, and by the reconciler, which fails an expired open request within a minute. A request past its deadline is never counted as pending. |
| Each local import stage, and each package transfer | 5-minute timeout | Work-step context deadline. Expiry is transient `worker_interrupted`. A process that dies instead is recovered by the reconciler once the lease (timeout + 1 minute) has passed. |
| Each metadata request | 30-second timeout | Not used by these stages. Catalogue discovery owns it (handoff Stage 3). |
| Automatic retry delay | Jittered exponential backoff starting at 10 s, capped at 5 min | After the *n*-th transient failure of a stage, base = min(10 s × 2^(*n*−1), 5 min). The delay is uniform in [base/2, base]. A longer upstream retry instruction is honoured only if the retry still starts before the deadline. If a retry cannot start before the deadline, the request becomes `failed` with `deadline_exceeded` immediately. |
| Waiting for the active pipeline | No budget used | `river.JobSnooze` with a delay uniform in [10 s, 20 s], and never past the deadline. Waiting increments no counter. |
| Permanent failures | No retry | `blocked` or `failed` at once, with diagnostics. No counter is consumed beyond the attempt that found the failure. |

Failure classes are §10's. Only `storage_unavailable` and `worker_interrupted`
are transient. Every other code is permanent.

### Outcomes and Retry

| outcome | state | reason codes (examples) | Retry offered |
| --- | --- | --- | --- |
| Transient budget exhausted | `failed` | `retry_budget_exhausted` | Yes |
| Deadline passed | `failed` | `deadline_exceeded` | Yes |
| Admission over capacity | `failed` | `installation_queue_full` | Yes |
| The Book became the current reading | `failed` | `book_is_current_reading` (OD-50) | Yes, after End or Finish |
| Supplied bytes do not match the pins, or break a limit | `blocked` | `package_too_large`, `package_inventory_mismatch`, transport or content digest mismatch | Yes, after the operator supplies the exact bytes (§12) |
| Rights gate closed | `blocked` | `rights_gate_closed` | Yes, after the gate opens |
| The pinned choice is deterministically unusable | `blocked` | `interpretation_version_unknown`, `binding_rejected`, `binding_nondeterministic`, structural codes, `coverage_gap`, `occurrence_unresolved`, `evidence_restricted` | No. The same pin gives the same result. Another choice is a new replacement request. |
| Leaving To Read | `cancelled` | `left_to_read` | No. Re-entering To Read follows the initial-request rule. |
| Newer choice or stale state | `superseded` | `superseded_by_request`, `expected_state_changed`, `already_current` | No |

**Retry** is an explicit owner action on the Book's latest request. It is
offered only when that request is `failed` or `blocked` with a code marked as
retryable above. It runs Admission for a **new** request:

- with the same pinned choice (every pinned-choice field copied);
- with the same kind, and `retry_of` naming the earlier request;
- with expected evidence equal to the Book's current evidence at the time of
  Retry;
- for a replacement, only through a confirmation form that names the
  candidate and the current evidence it would replace, with the same
  explanation as Refresh. `approved_at` records that confirmation. A Retry
  POST without it is refused, like a Refresh without approval.

Retry never picks a newer candidate, revision, Edition or unit. If the earlier
request is `initial` and the Book has current evidence by now, Retry is not
offered.

### Capacity

The two installation bounds are enforced at admission and at the queued-to-
running step, in the same transaction as the write they guard.

- **Pending.** At most 32 requests are **pending** across all owners. Pending
  means `state IN ('queued','running')` and `deadline_at` later than now.
- **Active.** At most 1 request is **active**. Active means
  `state = 'running'`. A request holds the slot from its first Begin until it
  leaves `running`, including between stages. It releases the slot when a
  transient failure returns it to `queued` and when it becomes terminal.

**Mechanism (OD-49).** A transaction-scoped advisory lock with one fixed
installation key, the **capacity gate**, as at
`internal/persistence/persistence.go:346`. Under it:

- Admission counts pending requests, leaving out the Book's own open request
  (the one it would supersede). If the count is 32 or more, it inserts the new
  request as `failed` with `installation_queue_full`, inserts no job, and leaves
  the Book's open request unchanged. Otherwise it supersedes the Book's open
  request, if any, and inserts the new request as `queued` with its job. Either
  way the pending count after commit is at most 32.
- Begin, for a `queued` request, checks for a `running` request. If one
  exists, it snoozes. Otherwise it sets `running`.

**Why this is race-free.**

- Only two writes can raise a count: an admission (pending) and a
  queued-to-running step (active). Both happen only while holding the gate.
- Each transaction counts in a statement that runs after it acquired the gate.
  Under `READ COMMITTED`, that statement's snapshot includes every row
  committed by an earlier holder, and an earlier holder commits before its
  transaction-scoped lock is released. So two concurrent admissions at 31 are
  serialized, and the second sees 32.
- Every other transition lowers a count, and runs without the gate. A
  concurrent decrease can only make an admission refuse conservatively. It can
  never let a count pass its bound.
- The deadline only lowers the pending count, and never raises the active
  count.
- The partial unique index on `running` backs the active bound in the
  database. A bug that bypassed the gate would fail on the index, not run a
  second pipeline.

**Lock order.** Every path takes locks in this order and skips the ones it
does not need: Book row → Book current classical analysis row → request rows,
in ascending `id` → capacity gate → per-Book fence (salt 193). Stage Begin takes
request → gate. Stage Finish and the reconciler take only the request.
Adoption takes no gate. The cycle-free
order prevents deadlock with Start, lemma decisions and dispositions, which take
the Book row before the per-Book fence (`internal/persistence/lemma_review.go:192-205`).

`MaxWorkers: 1` on the `classical_preparation` queue keeps one process from
running a second pipeline. The gate is what bounds the installation when
several processes run.

**Explained, not dropped.** A refused or waiting request is always a
persisted row with a state and a reason. Book detail shows "Waiting for another
preparation to finish" for a `queued` request whose job is snoozed. It shows
"Not started: the installation's preparation queue is full" with Retry for
`installation_queue_full`. No request is ever deleted, and no job is ever
inserted without its request.

### Admission

Admission is one transaction that creates a request and inserts its first
`verify` job with `InsertTx`, as the existing analysis submission does
(`internal/analysis/jobs.go:238-270`, `:416-431`). It runs from three entry
points:

| entry point | kind | transaction it joins | pinned choice |
| --- | --- | --- | --- |
| To Read on a classical Book with no current classical analysis and no open request | `initial` | The disposition transaction (`internal/persistence/book_dispositions.go:54-91`), so To Read and its intent commit together | The pinned choice of the Book's most recent `initial` request, if one exists. Otherwise the candidate that the Book's Unit offering or Local registration (§4) selects at this moment. |
| **Refresh**: the owner approves a named replacement candidate on a confirmation form (OD-55) | `replacement` | Its own transaction | The candidate the owner approved. `approved_at` is set. Expected evidence is the Book's current evidence. |
| Retry | as the earlier request | Its own transaction | Copied from the earlier request. |

Steps, in order:

1. Lock the Book row `FOR UPDATE`. A Book that is not the owner's, or has no §4
   association, returns the existing not-found response.
2. Require To Read, and refuse the owner's current reading's Book, as
   `admitToReadBookAnalysis` does (`internal/analysis/jobs.go:277-306`).
3. Lock the Book's open request, if any. For To Read, an open request makes
   admission a no-op, so repeated To Read never adds or re-pins a request.
4. Take the capacity gate and count, as under [Capacity](#capacity). Over
   capacity, insert the new request as `failed` with `installation_queue_full`
   and stop, leaving the open request unchanged.
5. Within capacity, for Refresh and Retry, set the open request to
   `superseded` with `superseded_by_request` naming the new request. Insert the
   new request as `queued`. Its `sequence` is the Book's highest plus one.
6. Insert the `verify` job, attempt 1, with `InsertTx`.
7. Commit.

A To Read on a Book that already has current classical evidence creates no
request. To Read never creates a replacement. The HTTP handler returns as soon
as admission commits. No byte is read and no package is parsed in the request.

### Adoption transaction

Adoption is the `adopt` stage's Finish. One transaction runs these steps in
order. Any failed check rolls the transaction back.

1. **Lock.** Lock the Book row `FOR UPDATE`, then the Book's current classical
   analysis row `FOR UPDATE` if it exists. The Book row is locked first because
   an initial adoption has no current row yet, and because Start, dispositions
   and lemma decisions lock the same row.
2. **Authorize.** The Book is the owner's and still has the association to the
   request's `reading_unit_id`. The owner still holds both linked acquisitions,
   and they satisfy §5's access predicate for the candidate. The operation's
   rights gate is still open. The candidate is adoptable: Vocabulary-ready, with
   no `vocabulary` Usability restriction on it or its inputs (§10). A
   replacement has `approved_at`. The Book is still To Read and is not the
   current reading.
3. **Request.** Lock the request `FOR UPDATE`. This job is current (see
   [Stages](#stages)), so the request is `running`, its stage is `adopt`, and
   this job's attempt is the open one. Its deadline has not passed. It has the
   highest `sequence` among the Book's requests that are not `cancelled`.
4. **Expected evidence.** The current row's `analysis_id` is not distinct from
   `expected_analysis_id`, and the Book's most recently adopted request (highest
   `sequence` in state `adopted`) is not distinct from
   `expected_adoption_request_id`. The second comparison also catches X → Y → X
   re-adoption, which an analysis comparison alone would miss. If the candidate
   equals the current analysis, the outcome is `superseded` with
   `already_current`.
5. **Projections ready.** `staged_builder_version` equals the current rich
   builder version, and the staged rows exist for this request. If not, the
   adopt attempt is closed as `transient` with `projection_stale`, `stage`
   returns to `prepare_projections`, and that stage's next attempt is scheduled
   by Finish's transient rule. Neither counter is reset, so the regression is
   bounded by both stage budgets and the deadline.
6. **Retention references.** Insert the Adopted evidence record
   `(owner, Book, candidate, text acquisition, annotation acquisition)`,
   insert-if-absent (§9 RET-TX). The request's own R2 references already exist.
7. **Switch.** Take the per-Book fence (salt 193). Upsert the Book current
   classical analysis row with the candidate, both acquisitions and
   `adopted_at`, all together. The analysis fixes its binding, and the binding
   fixes its text and annotation revisions, so text, binding and analysis
   switch as one value. Replace the Book's Rich vocabulary count rows with the
   staged rows, and set Rich count readiness to (candidate, `ready`, builder
   version, staged denominator) (§8 "Count projection").
8. **Close.** Set the request `adopted` with `adopted_at` and `finished_at`.
   Set any other open request for the Book to `superseded` with
   `superseded_by_request`. Admission leaves none, so this step is defensive.
   Delete the request's staged rows. Close the attempt as `succeeded`. Clear
   `lease_expires_at`. Commit.

**On failure.** After the rollback, one short transaction locks the request and
records the outcome, unless the request is already terminal:

| failed step | outcome |
| --- | --- |
| 2: rights gate closed, or candidate restricted | `blocked` with `rights_gate_closed` or `evidence_restricted` |
| 2: the Book is the current reading | `failed` with `book_is_current_reading` (OD-50) |
| 2: any other authorization check | `failed` with `authorization_lost` |
| 3: this job is not current | No outcome. The attempt is closed as `discarded` if still open, as in every stage's Finish. |
| 3: the deadline has passed | `failed` with `deadline_exceeded` |
| 3: a later non-cancelled request exists, or 4 | `superseded` with `expected_state_changed` or `already_current` |
| 5 | transient `projection_stale`, as above |
| A database error at any step, including commit | transient `storage_unavailable`, under the stage budget |

In every case, the old evidence stays current, and no Adopted evidence record,
count row, readiness change or current-row change from this attempt exists.
They were all written in the rolled-back transaction.

### Initial and replacement

- **Initial.** An `initial` request adopts its pinned choice automatically when
  it passes the adoption checks. It needs no approval, because To Read on an
  offered unit is the owner's selection of that candidate (ADR 0097).
- **Replacement.** A `replacement` request needs the owner's explicit approval
  of the named candidate, recorded as `approved_at` when the request is
  created. Before approval, the page explains that replacement can change
  personal counts and does not transfer corrections or exclusions (ADR 0097).
  A replacement without approval cannot be created (database check), and
  adoption checks approval again (step 2).
- **Repeated To Read never re-pins.** To Read creates only `initial` requests,
  only when the Book has no current classical analysis, and reuses the pinned
  choice of the Book's most recent `initial` request. A newer catalogue
  candidate reaches the Book only through Refresh.

### Cancellation and supersession

- **Leaving To Read** cancels that owner's open request for that Book only. The
  cancel is written in the same transaction as the disposition change:
  `TransitionBookDisposition` away from To Read
  (`internal/persistence/book_dispositions.go:54-91`), and Finish's return to
  Inbox (`internal/persistence/current_reading.go:345-349`). The statement is
  scoped by `(owner_id, book_id)`. It sets `cancelled` with `left_to_read`,
  clears `lease_expires_at` and deletes the staged rows. After commit, River jobs are
  cancelled best effort (`internal/analysis/jobs.go:788-797`). A worker that
  races the cancel writes nothing, because Finish and adoption both require
  `running`.
- **Hiding a Book** is not leaving To Read, and cancels nothing (§9
  "Deletion behaviour").
- **Choosing C while B runs.** Admitting C supersedes B in C's transaction,
  under the Book row lock. When B finishes later, its Finish sees `superseded`
  and writes nothing. B's adoption fails step 3. B cannot win by finishing last.
- **Re-entering To Read** never revives a cancelled request. A cancelled
  request is terminal. To Read never creates a replacement, so a cancelled
  replacement comes back only if the owner approves it again through Refresh.
  A Book without current evidence gets a new `initial` request with its
  original pin.
- **Nothing is deleted.** Cancellation and supersession change request state
  only. Package acquisitions, shared evidence, Adopted evidence records and
  every other §9 root stay. A request's references stay as physical rows
  (OD-39).
- **Other owners.** No cancellation or supersession reads or writes another
  owner's request. The capacity gate reads only installation-wide counts, and
  the reconciler applies only each request's own deadline and attempt rules. Owner A's cancel leaves owner B's request on
  the same shared evidence running. Shared rows that A's `verify` stage wrote
  are reused by B's stages.

### Frozen state

Adoption writes exactly these records: Preparation request, Preparation
attempt, Staged count row (deleted), Adopted evidence record (§9), Book current
classical analysis (§5), Rich count readiness and Rich vocabulary count (§8).
The earlier stages also write the shared §5, §6 and §7 evidence rows and the
owner's Package acquisition.

No stage and no adoption step writes `primary_goals`, `primary_goal_snapshots`,
snapshot vocabulary or Rich snapshot membership, snapshot count contributions,
Known (legacy or rich), `reading_history`, Exact occurrence decisions or flags,
deck preparations or specifications, dispositions or visibility. This is a
structural property: the `internal/adoption` package issues no statement on
those records, and adoption's checks read only the Book, current-evidence and
request rows. The §8 snapshot trigger and §9 references keep a frozen record on
its own analysis, so a later adoption cannot reach it. ADP-21 asserts the
result.

### OD-05 resolution

OD-05 is answered as option (2), consistent with §5's OD-12 option (2). The
adopted anchor is the parallel Book current classical analysis record. The
single domain classification (ADR 0052) reads it and reports a classical Book
as Reading-ready (§10). Every B-26 call site moves to that classification in
one change. EPUB Books keep reading `book_current_analyses` through the same
classification, so their verdicts do not change.

### Invariants

Each invariant is covered by the `ADP` rows in [§15](#15-acceptance-test-matrix).

1. **Intent.** A request and its first job commit together, or neither does.
   A classical To Read and its initial request commit together (ADP-01).
   Parsing never runs in an HTTP request or inside a transaction (ADP-01).
2. **One open request.** At most one open request exists per (owner, Book)
   (ADP-16, ADP-27). Repeated To Read never adds a request or re-pins (ADP-17).
3. **Budgets.**
   - No stage exceeds 3 attempts, and no delivery or restart pattern resets
     or skips an attempt or its backoff (ADP-02, ADP-03, ADP-04, ADP-27,
     ADP-28).
   - An interrupted or lost job is recovered without consuming budget beyond
     the interrupted attempt (ADP-03, ADP-29).
   - The deadline counts from creation, including queue and backoff time
     (ADP-05, ADP-29).
   - Permanent failures stop at once and consume no retry (ADP-06, ADP-18).
   - Retry creates a new request with the same pin (ADP-04).
   - Retry delays follow the binding backoff, and waiting consumes no attempt
     (ADP-08, ADP-26).
4. **Capacity.** At most 32 pending and 1 active request per installation,
   under concurrency, including between stages. A refused request is
   explained, not dropped, and changes no other request (ADP-07, ADP-08,
   ADP-19, ADP-27).
5. **Adoption.**
   - Adoption happens only in one transaction that writes current evidence,
     counts and retention references together (ADP-12, ADP-20).
   - It requires the expected evidence and the latest running request,
     including after re-adoption of an earlier analysis (ADP-09, ADP-13,
     ADP-25).
   - It never switches the evidence of the current reading's Book (ADP-24).
   - A failure leaves the old evidence current and no partial row (ADP-12).
   - A replacement, including its Retry, requires owner approval (ADP-14,
     ADP-27). An initial request adopts automatically (ADP-15).
6. **Cancellation.**
   - A cancelled or superseded request never adopts (ADP-09, ADP-10).
   - Cancellation is scoped to one owner and Book, and deletes no evidence
     (ADP-10, ADP-11).
   - Re-entering To Read revives nothing (ADP-22).
7. **Frozen state.** Adoption changes no snapshot, reservation, Known, history,
   deck record or decision (ADP-21).
8. **Isolation.** An owner never sees or affects another owner's requests
   (ADP-11, ADP-23).

### Conflicts and dependencies noted

- **§8 "Count projection".** §8 lets adoption set readiness `pending` and
  enqueue a rebuild when the build does not fit the transaction. ADR 0097
  requires ready projections before adoption, and this ticket requires the
  adoption commit to write them. §11 always builds counts in the
  `prepare_projections` stage and adopts only with them ready, so the `pending`
  path is never taken at adoption. §8 is not rewritten (OD-52).
- **§9 terminal states.** §9's illustrative unreferenced-evidence query treats
  `adopted`, `cancelled`, `failed` and `superseded` as terminal. §11 also makes
  `blocked` terminal. Under OD-39 option (1), a `blocked` request is a non-root
  holder too (OD-53). §9 is not rewritten.
- **§10 `adoption.Service`.** §10's illustrative `Adopt(…, analysis
  *canonical.ValidatedAnalysis)` is called only by the `adopt` stage, with the
  analysis the `validate` stage produced. The public entry points are
  admission (To Read, Refresh, Retry) and cancellation. This refines §10, and
  does not change it.
- **OD-47.** §11 stores a validated analysis that is not Vocabulary-ready, for
  owner inspection, and never adopts it (OD-54).
- **Current reading.** The ticket does not say what happens when a Book
  becomes the current reading while a replacement runs. §11 refuses the
  adoption (OD-50).
- **Package transfer.** The `verify` stage's catalogue transfer, its 5-minute
  timeout and upstream rate limits belong to handoff Stage 3. §12 supplies the
  local source. Both feed the same stage.
- **§12** places the operator commands, and calls Admission for the local
  path. §11 does not choose the local command surface.
- **§10 failure-class wording.** §10's table says rights and package failures
  make "the request fail", and that `expected_state_changed` changes nothing.
  §11 records the first as state `blocked`, and the second as state
  `superseded` with no other write. Both are §10's classes, named as §11
  states. §10 is not rewritten.
- **New request-scope codes.** §11 registers these codes in §10's registry,
  for request outcomes only: `installation_queue_full`,
  `retry_budget_exhausted`, `deadline_exceeded`, `left_to_read`,
  `superseded_by_request`, `expected_state_changed`, `already_current`,
  `book_is_current_reading`, `authorization_lost`, `rights_gate_closed`,
  `evidence_restricted`, `analysis_nondeterministic` and `projection_stale`.
  Only `projection_stale`, with §10's `storage_unavailable` and
  `worker_interrupted`, is transient.
- **Ticket wording on check failures.** #2072 says a failed adoption check
  marks the request `superseded` or `failed`. §11 also uses `blocked` (rights
  or restriction), and treats stale projections (step 5) as a transient retry,
  because the request is still current and only its derived rows need
  rebuilding. Every outcome still leaves the old evidence current.


## 12. Operator-local entry point

Verified at commit `bf961751318a8216c8f6d76a0479ede5ae903673` (`main`). Every
`path:line` in this section refers to that commit.

### Summary

- The entry point is a **hybrid** (OD-56 option (iii)). An operator command,
  `cmd/localregistration`, admits one local package and registers it for one
  owner. The learner then moves the resulting Inbox Book to To Read through the
  existing My Books action. That To Read starts §11's initial request.
- The command reads files. The web process never does. The command writes the
  verified §5 evidence, the §4 Book and Local registration, the owner's
  Package acquisition and a **Local selection** in one short transaction. It
  creates no Preparation request, no River job and no disposition change.
- Every value that decides acceptance comes from reviewed code, or from an
  operator-local pin file whose digest is compiled into reviewed code
  (OD-15, OD-57). The operator supplies only the owner, the names of a
  compiled pin set and interpretation, and the local files. The package
  supplies bytes only.
- Admission runs ten ordered checks on the package bytes in memory, outside
  every transaction, before any interpretation. Each check has a permanent
  diagnostic code. A failure writes nothing.
- The command never contacts NLP, never opens a network connection other than
  to PostgreSQL, never downloads anything and never falls back to another
  file, pin set, Edition or source.
- Output and logs carry codes, rule IDs, counts and record IDs only.

New names are proposed terms (OD-62). The command, package and flag names are
proposals, not prescriptions.

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| Operator commands read `MOUSEION_DATABASE_URL`, open the store with `persistence.Open`, run one operation, print one `key=value` summary line to stdout, and exit non-zero through `log.Fatal` on any error. `cataloguebackfill` also validates `MOUSEION_SECRET`, because it builds the catalogue service. | `cmd/vocabularybackfill/main.go:17-35`, `:42-53`; `cmd/cataloguebackfill/main.go:18-47` | `cmd/localregistration` follows the same shape: database URL from the environment, one summary line, explicit exit codes. It builds no catalogue service and needs no secret. |
| The server reads one operator-local file, the dictionary index named by `MOUSEION_DICTIONARY_INDEX`. A missing file is a supported absent state, and an unusable file is a startup error. | `cmd/server/main.go:377-401` | Precedent for an operator-supplied local file. This section keeps package files out of the server entirely: only the command reads them. |
| The NLP client reads `MOUSEION_NLP_ADDR` and connects lazily. | `internal/analyzer/grpc.go:22-25`; §10 N-5 | The command never imports `internal/analyzer` and never reads `MOUSEION_NLP_ADDR`. |
| `users.username` is `NOT NULL` and unique. There is no admin role in use (ADR 0017/0024). | `migrations/000001_initialize.up.sql:1433-1440`, `:2152` | The operator names the owner by username. |
| Book metadata provenance accepts only `catalog_sync`, `acquisition` and `source_materials_backfill`. | `internal/domain/books.go:14-19`, `:248-256` | A Book created here carries a new value, `operator_local` (OD-08). |
| Study-language display names come from `supported_languages`, with the code as fallback. Catalogue sync writes it. | `migrations/000001_initialize.up.sql:1422-1426`; `sqlc/queries/core.sql:60-67`; `sqlc/queries/book_aliases.sql:4-14` | Registration inserts the Work language's reviewed display name if absent, so `grc` is not shown as a bare code. It never overwrites a row. |
| The To Read handler checks the owner's Book, applies `TransitionBookDisposition` with the expected revision, then submits EPUB analysis. | `internal/webapp/library_handlers.go:270-310`; route `internal/webapp/webapp.go:238` | The learner's existing To Read action is the trigger. §11 admission joins its disposition transaction for classical Books. |
| EPUB reading opens ZIP bytes already in memory with `zip.NewReader` and decodes XML with `encoding/xml`. OPDS bounds XML input with `io.LimitReader`. | `internal/epub/epub.go:131-136`, `:484-499`; `internal/opds/client.go:452` | Admission likewise works on in-memory bytes. It does not reuse the EPUB reader, which has no inventory, link or expansion checks. |
| Design patterns for this flow: `AnnotatedBookRow` (margin note with an adjacent recovery action), `Confirmation`, `AsyncStatus`, `Feedback`, `MetadataList`; "Errors explain what happened and the next available action." | `doc/design/components.md:141-170`, `:448-458`, `:485-488`, `:496-506`, `:508-531`; `doc/design/design-system.md:429` | The learner-visible flow uses only these. |

### Interface choice

| option | how it works | assessment |
| --- | --- | --- |
| (i) CLI registers the package, creates the Book **and** the initial request | The operator command also sets To Read and admits a §11 request. | The operator changes a learner's disposition and makes the learner's reading choice. ADR 0096 requires an explicit informed choice for automatic-provenance annotation, and §11 requires To Read and its request to commit together in the disposition transaction. Running admission from a command also puts a second admission caller outside the web flow. Rejected. |
| (ii) Operator-configured local directory, plus a learner-visible "Add local edition" action | The server scans a configured directory. A learner picks a package and the web process imports it. | The web process gains package file access and parsing on a request path. Any learner could import any file in the directory, so the directory needs its own owner scoping. "Add local edition" is a new UI pattern, and it contradicts the Book rule "never by manual entry" (`CONTEXT.md:355-357`) more strongly than (i) or (iii). Rejected. |
| **(iii) Hybrid** | The operator command registers the package and the owner's Inbox Book. The learner moves it to To Read through the existing My Books action, which starts §11's initial request. | File access stays in an operator process. Learner intent and the informed choice stay with the learner and the existing disposition flow. Admission keeps one caller. **Recommended (OD-56).** |

The rest of this section specifies (iii).

### Command

```text
MOUSEION_DATABASE_URL=postgres://… \
  go run ./cmd/localregistration \
    -owner <username> \
    -pin-set <identifier>/<version> \
    -interpretation <identifier>/<version> \
    -package <path> \
    [-pin-file <path>] \
    [-interpretation-table <path>] \
    [-verify-only]
```

- Every flag except the two optional files and `-verify-only` is required.
  There is no default pin set, no "latest" version and no default
  interpretation. A flag given twice is a usage error.
- `-pin-file` is required exactly when the compiled pin set has
  operator-local fields (OD-15 (3)). `-interpretation-table` is required
  exactly when the interpretation definition declares an exception table. An
  unneeded file flag is a usage error, so a stray file is never silently
  ignored.
- `-verify-only` runs every check up to and including admission, opens no
  database connection, writes nothing, and reports `outcome=verified`.
- The command reads no other environment variable. It ignores
  `MOUSEION_NLP_ADDR`, `MOUSEION_SECRET` and every provider setting.

### Inputs

| value | supplied by | checked against | never from |
| --- | --- | --- | --- |
| Owner | Operator: `-owner` username | An existing `users` row, inside the registration transaction | The package, the pin file |
| Pin set identifier and version | Operator: `-pin-set` names one entry | The compiled pin-set registry. The entry must be present and not retired (OD-58). | The package. No default. |
| Interpretation definition identifier and version | Operator: `-interpretation` names one entry | The compiled interpretation registry (§10 VER-1, VER-2). The pin set must list it among the interpretations it supports, and its adapter must support it (§10 VER-3). | The package. No default. |
| Package bytes | Operator: `-package` file | Admission ADM-1 to ADM-10, against the pins | — |
| Pin-file bytes (retained pin sets only) | Operator: `-pin-file` | SHA-256 of the exact file bytes equals the commitment compiled into the pin-set entry (OD-57). Parsed only after that match. | The package |
| Exception-table bytes | Operator: `-interpretation-table` | SHA-256 equals the table digest the pin set declares for that interpretation, and length ≤ 16 MiB | The package |
| `definition_id` | Reviewed code: the pin set's identifier | — | Operator, package |
| Work, Edition, Reading unit, verified aliases, title, author, language and language display name | Reviewed code: the definition (§4 "Local registration") | — | Operator, package |
| `source_bounds`, projection, correspondence, adapter, provenance claims, notice coverage, revision origins and release metadata, `package_release`, the list of supported interpretations | Reviewed code, or the authenticated pin file where §14 marks a field private | — | Operator, package |
| Accepted transport digests, Package content digest, every member's path, role, size and digest, the table digest | The authenticated pin file (retained pin sets), or reviewed code (synthetic CI pin sets) | — | Operator flags, package |
| `origin_kind = operator_local`, `origin_ref = definition_id` | Fixed by the command | — | — |
| Database location | Environment: `MOUSEION_DATABASE_URL` | Required unless `-verify-only` | — |

A field present in both reviewed code and the pin file makes the pin file
invalid (`pin_file_invalid`). The pin file can add the private values that
the compiled entry marks as local, and nothing else. Its identifier and
version must equal `-pin-set`. The package's own manifest member is an
ordinary pinned member: its bytes are checked, and its content is never read
to decide anything (§5 "Pins").

### Pin sets the server can read

The server needs the pins again in §11's `verify` and `interpret` stages, and
must not read files. The command therefore stores the authenticated pin file:

#### Installed pin set

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| The exact bytes of one operator-local pin file, whose digest reviewed code has compiled as that pin set's commitment. The server reads private pins from here, never from a file. | `pin_set text` (`identifier/version`); `pin_file_format text` (`pin-file/1`); `content bytea` (≤ 1 MiB); `digest text` (`sha256:` of `content`, checked); `installed_at timestamptz` | `pin_set` | `digest` unique | Shared. | **Immutable.** Every `UPDATE` is rejected by a mirrored trigger, as for §5 records. No delete path. | None. Package acquisitions and Preparation requests name it by `pin_set` text. |

The one pin-set interface (§5 "Pins") resolves a pin set as follows:

1. Look up the compiled entry. If it is absent, the pin set is unknown.
2. If the entry has no local fields, return it.
3. Otherwise read the Installed pin set row and check that its digest equals
   the compiled commitment, on every load. If the row is missing or differs,
   the pin set is unavailable, with `pin_file_missing` or `pin_file_mismatch`.

So a row edited directly in the database is detected, not trusted. The
private values live in the installation's database, which is local, and never
in the public repository (§14).

```sql
-- illustrative, not a migration
CREATE TABLE installed_pin_sets (
  pin_set text PRIMARY KEY CHECK (pin_set ~ '^[a-z0-9][a-z0-9-]*/[1-9][0-9]*$'),
  pin_file_format text NOT NULL CHECK (pin_file_format = 'pin-file/1'),
  content bytea NOT NULL CHECK (octet_length(content) <= 1048576),
  digest text NOT NULL UNIQUE CHECK (digest = 'sha256:' || encode(digest(content, 'sha256'), 'hex')),
  installed_at timestamptz NOT NULL DEFAULT now()
);
-- plus: BEFORE UPDATE trigger mirroring reject_source_content_revision_mutation().
```

### Local selection

The operator chooses both the package and the frozen interpretation. §4's
Local registration records neither, by design, so the choice gets its own
record. It is the candidate that §11 admission takes "from the Book's Local
registration" when it builds an `initial` request.

#### Local selection

| purpose | fields (type sketch) | key | uniqueness | owner scope | mutability | references |
| --- | --- | --- | --- | --- | --- | --- |
| One operator selection of a package acquisition and a frozen interpretation for one owner's classical Book. | `id uuid`; `owner_id uuid`; `book_id uuid`; `definition_id text`; `pin_set text`; `acquisition_id uuid`; `interpretation text` (interpretation definition `name/version`); `interpretation_digest text` (§5 `configuration_digest` of that definition with the pinned table digest); `adapter text`; `selected_at timestamptz` | `id` | `(owner_id, id)` unique. No uniqueness on the choice: re-selecting an earlier choice appends a new row. | Owner. | **Append-only.** Each row is immutable. | `(owner_id, book_id)` → §4 Book–Reading unit association `ON DELETE CASCADE`; `(owner_id, acquisition_id)` → §5 Package acquisition `NO ACTION` (owner deletion cascades both); `owner_id` → `users(id)` `ON DELETE CASCADE` |

Rules:

- The Book's **current Local selection** is its row with the greatest
  `(selected_at, id)`.
- Registration appends a row only when the current Local selection differs
  from the new one in `pin_set`, `acquisition_id` or `interpretation`. A
  repeat registration appends nothing.
- A Local selection names an acquisition whose `origin_kind` is
  `operator_local` and whose `origin_ref` is the same `definition_id`. A
  trigger enforces this.
- §11's pinned choice for an initial request is built from the current Local
  selection: `pin_set`; `origin_kind`, `origin_ref` and `package_release` from
  its acquisition; `package_digest` from the acquisition and
  `package_content_digest` from its blob; `reading_unit_id` from the Book's
  association; `binding_declaration_digest` from the pin set;
  `interpretation_digest` and `adapter` from the selection. Text and
  annotation acquisitions are the same acquisition, because one package
  carries both.
- A Local selection never changes a running request, current evidence or a
  disposition. When it differs from the Book's current evidence, Book
  surfaces offer it as a Refresh candidate through §11's confirmation form.

### Order of operations

Every step before the registration transaction runs without a database
connection. The command stops at the first failure.

| step | check | failure code | exit |
| --- | --- | --- | --- |
| P-1 | Flags parse. Every required flag is present once. File flags match what the pin set and interpretation need. | `usage_invalid` | 2 |
| P-2 | `MOUSEION_DATABASE_URL` is set, unless `-verify-only`. | `database_url_missing` | 2 |
| P-3 | The pin set is compiled and not retired. | `pin_set_unknown`, `pin_set_retired` | 4 |
| P-4 | The interpretation definition is compiled, its slots, contract version and adapter are registered, and the pin set supports it. | `interpretation_version_unknown`, `contract_version_unknown`, `adapter_version_unsupported`, `interpretation_not_pinned` | 4 |
| P-5 | The compiled rights-gate state permits the operator-local import operation (#2054). | `rights_gate_closed` | 4 |
| P-6 | Pin file: opened, a regular file, at most 1 MiB + 1 bytes read, digest equal to the commitment, then parsed strictly (`pin-file/1`: UTF-8 JSON, unknown and duplicate keys rejected, identifier and version equal `-pin-set`, only local fields). | `local_file_missing`, `local_file_unreadable`, `pin_file_mismatch`, `pin_file_invalid` | 3, 3, 4, 4 |
| P-7 | Exception table: opened, a regular file, at most 16 MiB + 1 bytes read, digest equal to the pinned table digest. | `local_file_missing`, `local_file_unreadable`, `interpretation_table_mismatch` | 3, 3, 4 |
| P-8 | Package: opened, a regular file. At most 16 MiB + 1 bytes are read into memory. | `local_file_missing`, `local_file_unreadable` | 3 |
| ADM-1 to ADM-10 | [Admission checks](#admission-checks), in order. | as listed there | 5 |
| T | [Registration transaction](#registration-transaction). | `owner_not_found`; `storage_unavailable` | 6; 1 |

No package byte is read before P-3 to P-5 pass. So an unknown version or a
closed gate is reported even when the package file is absent or hostile.

### Admission checks

Admission is a pure function over the package bytes and one resolved pin set.
It lives in a new package, `internal/evidencepackage`. That package may
import `archive/zip`, `compress/flate`, `crypto/sha256`, `encoding/xml`
through the shared safe XML reader (OD-61), and `golang.org/x/text/unicode/norm`.
It must not import `os`, `net/...`, `os/exec`, `plugin`, persistence or River.
The command calls it, and §11's `verify` stage calls the same function on the
stored blob.

It runs on in-memory bytes, outside every transaction, under the 5-minute
local import stage timeout (handoff "Initial operational envelope"). Every
check is permanent: a retry gives the same result. A failure stops admission
at that check and writes nothing.

| rule | check | codes (§10 family) |
| --- | --- | --- |
| ADM-1 | **Size.** The file is at most 16,777,216 bytes. P-8 read at most one byte more, so a larger file is detected without reading it all. The configured limit can only be lower (OD-16). | `package_too_large` |
| ADM-2 | **Transport pin.** The SHA-256 of the whole file is one of the pin set's accepted transport digests. Nothing is parsed before this. | `package_transport_unpinned` (`package_inventory_mismatch`) |
| ADM-3 | **Central-directory sanity.** Exactly one end-of-central-directory record, at the end of the file, with no trailing bytes. No ZIP64 records, no multi-disk fields, no archive comment. The declared entry count equals the entries actually parsed. The central directory and every local header lie inside the file, start where declared, and do not overlap. Nothing precedes the first local header. Each local header's name, method, flags and sizes equal its central entry's. | `package_archive_malformed` (`package_entry_unsafe`) |
| ADM-4 | **Member count.** The reader counts central-directory entries and stops at entry 129. | `package_too_many_members` |
| ADM-5 | **Exact member set.** No two entries have the same path, byte for byte or after NFC normalization. Every entry path is a pinned path, and every pinned path is present. | `package_member_duplicate`, `package_member_unlisted`, `package_member_missing` (`package_inventory_mismatch`) |
| ADM-6 | **Safe paths and entry types.** No absolute path, drive letter, backslash, `.` or `..` segment, empty segment, control character or non-NFC path. Every entry is a regular file: no directory entries, no symbolic link (Unix mode `S_IFLNK` in the external attributes), no hard link, device or other special type, and no extra field outside a compiled allowlist (extended timestamp only), so link and owner extensions are refused. No encrypted entry. Compression is stored or deflate only. | `package_path_unsafe`, `package_entry_link`, `package_entry_unsafe` (`package_entry_unsafe`) |
| ADM-7 | **No executable payload.** No entry has an execute bit in its Unix mode. Every path's extension is in the compiled allowlist for its pinned role. The first 16 decompressed bytes of every member carry no executable or archive signature: ELF, PE (`MZ`), Mach-O, WebAssembly, a `#!` interpreter line, or a nested ZIP. This check decompresses at most 16 bytes per member, so it runs before the full pass. | `package_member_executable` (`package_entry_unsafe`) |
| ADM-8 | **Streamed expansion bound.** One pass decompresses the members in central-directory order, with a running total across all members. Declared sizes are never trusted and never used to size a buffer. The pass stops when the total would exceed 67,108,864 bytes, and also when one member's output exceeds its pinned size by one byte. | `package_expanded_too_large` |
| ADM-9 | **Exact pins.** In the same pass, each member's actual size and SHA-256 equal its pin. After the pass, the `inventory/1` serialization of the actual members (§5 Package content) hashes to the pinned Package content digest. The manifest member is checked here like any other member. | `package_member_size_mismatch`, `package_member_digest_mismatch`, `package_content_digest_mismatch` (`package_inventory_mismatch`) |
| ADM-10 | **Safe XML.** Every member whose pinned format is XML is scanned to the end with the shared safe XML reader. It accepts only UTF-8, runs in strict mode, and knows only the five predefined entities and numeric character references. It refuses any document type declaration with an internal subset, and any `ENTITY` declaration, with `xml_dtd_forbidden`. A document type declaration with only an external identifier is a token: the identifier is never resolved and no DTD or schema is loaded. The reader has no resolver that can open a file or a network connection, and bounds element depth at 256. Processing instructions are tokens, never acted on. | `xml_malformed`, `xml_encoding_unsupported`, `xml_dtd_forbidden`, `xml_entity_reference`, `xml_depth_exceeded` (`package_entry_unsafe`) |

ADM-2 hashes the transport bytes before any parsing. A matching digest proves
the bytes are the reviewed build, so ADM-3 to ADM-10 then cannot fail for a
correct pin set. They still run in full on every admission. They protect
against a wrong or hostile pin set, and they are what §11's `verify` stage
re-runs on stored bytes. Each test for ADM-3 to ADM-10 uses a synthetic
test-only pin set that pins the hostile build, so that every check is
reachable (LOC rows).

Each admission diagnostic uses §10's shape with `scope = request`, the rule ID
above, and `count` set to the 1-based central-directory entry ordinal where one
applies (or to the member count for ADM-4). It never names a member path,
digest or byte value. The operator can find the entry from the ordinal in the
file they hold.

Errors from `archive/zip`, `compress/flate` or `encoding/xml` are mapped to
the codes above and never printed, because their text can contain entry
names or content. A panic inside admission is recovered and reported as
`package_archive_malformed`.

### Registration transaction

One short PostgreSQL transaction, after admission has passed. It contains no
file, network or NLP I/O. Every insert is insert-if-absent on the key named in
§4, §5 or this section, so a repeat or a concurrent run converges.

1. Read the owner by username `FOR SHARE`. If there is none, roll back with
   `owner_not_found` (exit 6). The shared lock fences a concurrent owner
   deletion.
2. Insert the Installed pin set, if this pin set has local fields.
3. Insert the shared §5 evidence for the verified package: Package content and
   members, the Package blob, notices and Notice coverage, Text and Annotation
   Source revisions with their members, and provenance claims. Insert the
   exception table as a Package blob of kind `interpretation_table`. The
   deferred §5 count and sum triggers check the inventory at commit.
4. Run §4 **Local registration**: ensure the shared Work, Edition, unit and
   verified aliases; **Ensure Book for Reading unit** with origin
   `operator_local` (a new Book is Inbox, with active membership, Work title,
   author and language, and `metadata_provenance = operator_local`); insert
   the Local registration if absent.
5. Insert the owner's Package acquisition: `origin_kind = operator_local`,
   `origin_ref = definition_id`, `package_release` and `pin_set` from the pin
   set. Its §5 unique tuple makes a repeat a no-op.
6. Lock the Book row `FOR UPDATE`, then append a Local selection if the
   current one differs.
7. Insert `supported_languages(language, display_name)` from reviewed code if
   no row exists for the Work language. An existing row is never changed.
8. Commit.

It never writes a Preparation request, a River job, a disposition, a
visibility change, a Reading, a snapshot, Known, history, a decision, a deck
record or an Interpretation configuration. The `interpret` stage writes the
configuration (§11).

Lock order: user row → §4 identity lock (`owner:root`) → Book row. §11's lock
order starts at the Book row, so the two orders do not conflict.

Any database error, including at commit, rolls everything back and exits 1
with `storage_unavailable`. A re-run is safe.

### Output and exit codes

| exit | outcome | stdout | stderr |
| --- | --- | --- | --- |
| 0 | `created`: the Book was created | One line: `local registration complete: outcome=created book=<book id> members=<n> package_bytes=<n> expanded_bytes=<n>` | — |
| 0 | `added`: the Book existed, and a new acquisition or Local selection was added | Same line with `outcome=added` | — |
| 0 | `unchanged`: every row already existed | Same line with `outcome=unchanged` | — |
| 0 | `verified` (`-verify-only`) | `local registration verified: members=<n> package_bytes=<n> expanded_bytes=<n>` | — |
| 1 | Infrastructure failure. Nothing committed. Safe to re-run. | — | `localregistration: failed: storage_unavailable: <message>` |
| 2 | Usage or configuration | — | `localregistration: usage: <code>: <message>`, followed by the flag summary |
| 3 | A local file is missing or unreadable | — | `localregistration: refused: <code>: <message>`. The message names the flag (`-package`, `-pin-file` or `-interpretation-table`), never the path. |
| 4 | Refused by reviewed configuration: unknown or retired pin set, unknown or unsupported interpretation, closed rights gate, pin file or table not matching | — | `localregistration: refused: <code>: <message>` |
| 5 | Package rejected by admission | — | `localregistration: rejected: <code>: rule=<ADM-n> entry=<ordinal>: <message>` |
| 6 | Owner not found | — | `localregistration: refused: owner_not_found: <message>` |

`<message>` is the registry's fixed English text for the code. Examples:

- `package_too_many_members`: "The package has more than 128 members. Nothing
  was registered. Supply the exact reviewed package."
- `local_file_missing`: "The file named by -package does not exist. Nothing was
  registered."
- `interpretation_version_unknown`: "This release does not include the named
  interpretation. Nothing was registered. Name an interpretation listed for
  this pin set."
- `pin_file_mismatch`: "The pin file is not the reviewed pin file for this pin
  set. Nothing was registered."

Every message says that nothing was registered, as the `Feedback` pattern
requires ("what remains unchanged").

The only values the command prints are the outcome, the Book ID (an owner
record ID, which is not source content), counts, codes, rule IDs and entry
ordinals. It prints no digest, no member path, no file path and no package
content. The operator already holds every value it reports.

### Safe failure

- **Nothing partial.** Every refusal before T writes nothing, because no
  database connection is open yet. A refusal or error in T rolls back the
  whole transaction. No path leaves a blob without its Package content, a
  Book without its Local registration, or an acquisition without its
  selection.
- **No fallback.** On any failure the command exits. It never tries another
  pin-set version, interpretation, file, Edition, unit or source, never
  downloads, and never contacts a catalogue connection. A failed admission
  never changes the owner's existing Book, selection, request or evidence.
- **Missing files** exit 3 before any write (P-6 to P-8).
- **Unknown or retired versions** exit 4 before any package byte is read
  (P-3, P-4).
- **Pin mismatches** (transport, member, content, pin file, table) exit 4 or
  5 before any write.
- **Capacity.**
  - Package bounds (ADM-1, ADM-4, ADM-8) exit 5 before any write.
  - Installation queue capacity is not checked by the command, because it
    admits no request. It applies when the learner moves the Book to To Read:
    §11 records `failed` with `installation_queue_full`, and the Book row
    shows "Not started: the installation's preparation queue is full" with
    Retry. The registration is unaffected.
  - Database storage exhaustion is a database error: exit 1, rollback.
- **Server side.** For `origin_kind = operator_local`, §11's `verify` stage
  reads the stored blob through the owner's acquisition, never a file. It
  re-runs ADM-3 to ADM-10 on the stored bytes against the pin set, then links
  the acquisition. A mismatch there is an integrity failure: the request is
  `blocked` with the ADM code, and recovery is §9's exact restore. If the
  owner holds no acquisition for the pinned digest, the request is `blocked`
  with `local_package_not_registered`. Retry is offered once the operator has
  registered it. A pin set that the interface reports unavailable (Installed
  pin set missing or altered) gives `blocked` with `pin_file_missing` or
  `pin_file_mismatch`.

### Idempotency

- **Same package, same owner, again.** The second run finds every shared row,
  the Book, the Local registration, the acquisition and an equal current Local
  selection. It writes nothing and reports `outcome=unchanged`. The command
  never writes a Preparation request, so it can never add a second open
  request, whatever the Book's state. A To Read on a Book with an open request
  is already a no-op (§11 Admission step 3).
- **Same package, second owner.** The shared Package blob, content, members,
  revisions and claims are found, not inserted again. The second owner gets
  their own Book, association, Local registration, acquisition and Local
  selection (§4, §5 EVD-25). The first owner's rows are unchanged.
- **Same owner, new pin-set version or interpretation.** The Book and Local
  registration are reused (§4). A new acquisition (for a new transport build)
  or only a new Local selection is added, and the outcome is `added`. A running
  request, the current evidence and the disposition are unchanged. If the Book
  has current evidence, the new selection is a Refresh candidate. If it has
  none and an earlier initial request is `blocked`, see OD-59.
- **Concurrent runs.** Two runs for the same owner and definition converge:
  §4's identity lock serializes Ensure Book, the acquisition's unique tuple
  absorbs the second insert, and the Book row lock serializes the Local
  selection check.
- **Hidden Book.** Registration never unhides a Book. The learner unhides it
  through the existing More actions disclosure.

### Learner-visible flow after registration

The flow uses only existing destinations and patterns. "Book detail" in §11
means these existing places: the My Books `AnnotatedBookRow`, Reading's To Read
candidate row, and the `/jobs` `AsyncStatus` entry.

1. **The Book appears in My Books.** Its language is the Work's (`grc`), shown
   with the reviewed display name in the active-language switcher (ADR 0050).
   The Book is an Inbox `AnnotatedBookRow`:
   - Title and author come from the Work.
   - Beneath them, in the same text column, the row adds the Edition label (or
     "Unidentified edition", §4) and the unit's bounds label. Long text wraps
     and is never truncated.
   - One plain line states the annotation provenance at release scope, for
     example "Automatic annotation, with reviewed repairs". It is taken from
     the §5 provenance claims and the repair layer, and is never a badge or a
     score.
   - The margin carries the **Inbox** `StatusBadge`. There is no evidence note
     yet, rather than `0%`.
2. **To Read starts or continues the §11 request.** The primary action stays
   **Move to To Read**.
   - When any release-scope claim for a layer the analysis uses is
     `automatic` or `unknown`, the action is a `Confirmation` disclosure. Its
     summary is "Move to To Read". Its body uses a `MetadataList` to name the
     Edition, unit, source release, provenance by layer and the repair layer.
     It says that preparation runs in the background and does not need the
     language service, and that Reading still starts only when the learner
     chooses it. The final button reads "Move to To Read and prepare this
     edition". This is ADR 0096's informed choice (OD-60). A POST without the
     confirmation field changes nothing and returns the row with the
     disclosure open.
   - The POST is the existing To Read handler. For a classical Book, §11
     Admission joins its disposition transaction and builds the pinned choice
     from the current Local selection. If an open request exists, nothing new
     is admitted.
3. **Progress and failure use existing job and recovery states.**

   | §11 request state | margin note on the row | adjacent recovery action |
   | --- | --- | --- |
   | `queued` | "Preparation queued" | — |
   | `queued`, job snoozed for capacity | "Waiting for another preparation to finish" | — |
   | `running` | "Preparing edition" | — |
   | `failed` with a retryable code | "Preparation failed: " + the code's fixed explanation | **Retry** (submit button), as §11 "Outcomes and Retry" |
   | `failed` with `installation_queue_full` | "Not started: the installation's preparation queue is full" | **Retry** |
   | `blocked` with a retryable code | "Edition unavailable: " + explanation | **Retry**, only when §11 marks the code retryable |
   | `blocked` otherwise | "Edition unavailable: " + explanation, plus "This release cannot prepare this edition." | None. No dead route. |
   | `cancelled` | No note (the Book has left To Read) | — |
   | `adopted` | The existing current-evidence note (Known coverage) | — |

   The `/jobs` page shows the request with `AsyncStatus`: Book title, Edition
   and unit lead; stage and attempt are supporting detail. Failure uses
   `Feedback` inside the status region with the same recovery action.
   Messages come from the code registry and never include source text.
4. **Current reading is separate.** Adoption never starts a Reading. The
   adopted Book appears among Reading's To Read candidates. The learner
   starts it through the existing `ReadingChooser` confirmation.

### NLP and network independence

- The command's transitive dependencies exclude `internal/analyzer`, gRPC,
  `net/http` and every provider package. A dependency test (`go list -deps`)
  asserts this, alongside §10's layering test (OD-48).
- The only connection the command opens is to PostgreSQL. The admission
  package imports no `net` or `os` package. The safe XML reader has no
  resolver.
- The server needs no NLP for any step of this flow (§10 N-3, N-4).
  `register → To Read → adopted` works with NLP stopped (LOC-20).

### Logging and privacy

- The command, the server and River job metadata never contain package bytes,
  member content, member paths, Greek text, lemma or table values, digests,
  file paths or pin-file content.
- They contain only: outcome, codes, rule IDs, counts, entry ordinals, the
  Book ID, the request ID and the owner's username (operator-supplied, on the
  command's own output only).
- §11's request `diagnostics` hold §10-shaped diagnostics, which carry no
  input strings. River job arguments carry only owner, request, stage and
  attempt (§11).
- Library error text is never logged (see Admission checks). A test plants
  unique sentinel strings in every synthetic member, path, lemma, table value
  and pin field, and fails if any appears in command output, server logs, job
  rows or request rows (LOC-21).

### Invariants

Each invariant is covered by the `LOC` rows in [§15](#15-acceptance-test-matrix).

1. **Pins come from reviewed code.** Acceptance is decided only by the
   compiled pin set and interpretation, and by a pin file whose digest equals
   the compiled commitment. A package, a pin file that does not match, or an
   operator flag never adds or changes a pin (LOC-13, LOC-18).
2. **Admission order and completeness.** Every package passes ADM-1 to ADM-10,
   in order, before any write and before any interpretation. A package that
   fails any check writes no row in any table (LOC-01 to LOC-13).
3. **Hostile archives.** Path traversal, links, duplicate and unlisted
   members, more than 128 members, more than 16 MiB, expansion past 64 MiB
   with lying headers, executable payloads, malformed central directories and
   XML entity payloads are each refused with their code. No file outside the
   process's inputs is read, and no network connection is attempted (LOC-01 to
   LOC-12).
4. **Versions first.** An unknown or retired pin set, an unknown or unsupported
   interpretation, or a closed rights gate is refused before any package byte
   is read (LOC-17).
5. **Idempotent registration.** Registering the same package for the same
   owner twice gives the same blob, Book, Local registration, acquisition and
   selection, and writes nothing the second time, including under concurrency.
   Registration never creates a Preparation request (LOC-14, LOC-24, LOC-28).
6. **Shared bytes, separate owners.** A second owner gets their own Book,
   acquisition and selection, and no new shared bytes. Neither owner can reach
   the other's records (LOC-15).
7. **Safe failure.** Missing files, unknown versions, pin mismatches and
   capacity failures each give their documented exit code and message, write
   nothing, and fall back to nothing (LOC-16, LOC-17, LOC-18, LOC-19, LOC-22,
   LOC-23, LOC-29).
8. **Learner intent.** Registration changes no disposition, Reading or learner
   state. Preparation starts only from the learner's To Read, after the
   informed choice when provenance is automatic or unknown. Reading starts only
   from the learner's explicit Start (LOC-20, LOC-26).
9. **Selection without disruption.** A new Local selection never changes a
   running request or current evidence (LOC-25).
10. **No file access in the server.** The server prepares and adopts a locally
    registered package without the original file (LOC-27).
11. **NLP and network independence.** Register, To Read, preparation and
    adoption complete with NLP stopped and no outbound network. The command
    cannot link NLP or network clients (LOC-20, LOC-30).
12. **Privacy.** Output, logs, job metadata and request diagnostics contain no
    package content, Greek text, table values, digests or paths (LOC-21).
13. **Records.** A Local selection names only an acquisition of its own
    definition. Local selections and Installed pin sets are immutable, and an
    Installed pin set's digest matches its content (LOC-31).

### Conflicts and dependencies noted

- **§11 `verify` stage input.** §11 lists "the §12 local path" as the `verify`
  stage's source. §12 never gives the server a path. For `operator_local`
  origins the source is the stored blob that the command wrote, reached
  through the owner's acquisition, and the stage re-runs admission on it.
  §11 is not rewritten.
- **§11 initial requests and approval.** §11 says an initial request needs no
  approval. ADR 0096 requires an explicit informed choice for automatic or
  unknown-provenance annotation. §12 makes To Read's confirmation that choice,
  and records it as `approved_at` on the initial request, which §11's checks
  permit (OD-60).
- **§11 re-pinning.** §11's To Read reuses the most recent initial request's
  pinned choice, and Refresh requires current evidence. A Book whose initial
  choice is deterministically `blocked` therefore cannot reach a newer Local
  selection (OD-59).
- **§4 Local registration** is used unchanged. The pinned versions it leaves to
  §11 are carried by the new Local selection.
- **§5 "Pins" names one Interpretation configuration per pin set.** §12 lets a
  pin-set version list one or more interpretation definitions, and the
  operator names one with `-interpretation`. With a single listed
  interpretation the two readings agree. §5 is not rewritten.
- **§5 pin source (OD-15).** §12 is written against option (3), and adds the
  compiled commitment and the Installed pin set (OD-57).
- **§5 limit codes.** §5 names five families of package codes. §12's finer
  codes each map to one of them, shown in the Admission checks table. §11 and
  §10 continue to name the families.
- **§10 registry.** §12 registers these codes in §10's registry, all
  permanent: the ADM codes in the Admission checks table; for request outcomes,
  `local_package_not_registered`, `pin_file_missing` and `pin_file_mismatch`;
  and, for command output only, `usage_invalid`, `database_url_missing`,
  `pin_set_unknown`, `pin_set_retired`, `interpretation_not_pinned`,
  `pin_file_invalid`, `interpretation_table_mismatch`, `local_file_missing`,
  `local_file_unreadable` and `owner_not_found`. §10's failure-class table is
  not rewritten.
- **Admission order.** The ticket lists nine checks, with central-directory
  sanity second. §12 keeps that order and inserts one check, ADM-2 (the
  whole-file transport digest), before it. ADM-2 parses nothing, and refuses
  any unpinned build before a ZIP parser sees it. Every listed check still
  runs, so each LOC row reaches its check with a test-only pin set that pins
  the hostile build.
- **§11 Admission's pinned choice.** §11 takes the initial candidate from
  "the Book's Unit offering or Local registration (§4)". §4's Local
  registration records no candidate. For local Books, §12's current Local
  selection is that candidate. §11 is not rewritten.
- **§11 "Outcomes and Retry" for byte mismatches.** §11 offers Retry "after the
  operator supplies the exact bytes (§12)". On the local path, mismatched bytes
  are refused by the command before any request exists. The only retryable
  local case is `local_package_not_registered`, after the operator registers
  the exact package. A mismatch found on bytes already stored is an integrity
  failure, recovered by §9's exact restore, not by Retry.
- **§5 "Limits".** §5 says the request records an admission failure. On the
  local path no request exists yet, so the command's exit code and message
  record it. A `verify` stage failure on stored bytes is recorded on the
  request as §5 says.
- **§10 placement.** The command may import adapters for `Supports` checks, as
  §10 allows for "the §12 local entry command". `internal/evidencepackage` and
  the shared safe XML reader are new packages, which need the ADR 0091
  amendment (OD-43, OD-61).
- **§9 retention.** The Installed pin set and Local selection are new
  records. §9's delete guard on shared evidence must cover the Installed pin
  set, and §9's referencer list must include Local selection, which references
  an acquisition. §9 is not rewritten.
- **`CONTEXT.md:355-357`.** The Book entry says Books enter "only through a
  connected catalog, never by manual entry". OD-08 records the wording change.
  A Book entered by an operator's reviewed registration is not manual entry by
  a learner.
- **Seams.** The command's rows use a command integration test against real
  PostgreSQL, the operator-command form of seam (d) under OD-41 (1), combined
  with seam (a) for what the learner sees, as the ticket asks ("seam (a), plus
  a CLI test"). They are marked `(a) + CLI`. LOC-28 uses seam (d) for
  concurrent transactions, as BIB-02 does. LOC-30 is a layering row and uses
  the lint seam under OD-48 (1).

## 13. Migration sequence, preflight and recovery

Verified at commit `bf961751318a8216c8f6d76a0479ede5ae903673` (`main`). Every
`path:line` in this section refers to that commit. golang-migrate is v4.20.1
(`go.mod:8`); golang-migrate citations name files in that module version.

### Summary

- Nine **logical steps**, S1 to S9, implement §4 to §11. Each step is one
  migration pair, named here and never numbered. No step rewrites, deletes or
  backfills an existing row.
- Three existing tables gain objects: `book_current_analyses` (one trigger),
  `primary_goal_snapshots` (§8's representation column and constraints) and
  `primary_goal_snapshot_vocabulary` (one trigger). Every other record is a new
  table.
- The **legacy-`grc` preflight** is a read-only operator command. It reports
  counts and IDs per owner, checks the assumptions that the new constraints rely
  on, and reports rollout status. Any `grc` row stops Greek import for the whole
  installation until a separately approved reconciliation plan exists (OD-63).
  Rows are never converted.
- No data-only work is required to add Greek.
- Recovery is roll-forward by default. Backup and restore is the evidence-loss
  path, verified by §9's integrity check. A half-applied rollout is detected by
  golang-migrate's dirty flag and the preflight's rollout checks.

New rule and command names are proposed terms (OD-67).

### Current anchors

| fact | evidence (path:line) | consequence here |
| --- | --- | --- |
| The server applies migrations with `Up` before it opens the store, then applies River's migrations. No production path runs `Down`. | `cmd/server/main.go:146-153`; `internal/persistence/persistence.go:79-108` (`Up` at `:92`); `doc/adr/0070-migration-and-documentation-reboot.md:52-54` | Every step runs at server start, before any request or job. River's schema is separate, and no step touches it. |
| golang-migrate marks the target version dirty before running a file and clean after it. It runs the file as one `Exec` without arguments, which lib/pq sends as one simple query, so PostgreSQL runs it as one implicit transaction. `Up` refuses to run on a dirty database. | `migrate.go:738`, `:750`; `database/postgres/postgres.go:285-296`; `migrate.go:215-223` | A failed step leaves no partial object, leaves its version dirty and stops server start. The dirty flag is the half-applied signal. |
| Migrations are embedded with `//go:embed *.sql`. | `migrations/embed.go:6-9` | A new step needs no change outside `migrations/`. |
| sqlc reads `migrations/*.up.sql`. Non-nullable `uuid` columns map to `string`, and nullable ones stay `pgtype`. `make sqlc` runs the pinned sqlc, and CI asserts `git diff --exit-code`. | `sqlc.yaml:12-13`, `:22-27`; `Makefile:192-197` | Every step regenerates `gen/sqlc`. S7 changes generated Go types (see [Generated artifacts](#generated-artifacts-and-review)). |
| The baseline already carries a `NOT VALID` constraint. | `migrations/000001_initialize.up.sql:1520` | `NOT VALID` followed by validation is an established form. |
| `pgcrypto` is installed by the baseline. | `migrations/000001_initialize.up.sql:29` | §5's digest checks need no new extension and no `go.mod` change. |
| `primary_goal_snapshots` has five `NOT NULL` legacy evidence columns. `primary_goal_snapshot_vocabulary` cascades from it. | `migrations/000007_primary_goal_snapshots.up.sql:14-33`, `:35-51` | S7 relaxes these columns under §8's representation check (OD-32). |
| `book_current_analyses` has primary key `(owner_id, book_id)`. | `migrations/000001_initialize.up.sql:239-245`, `:1544` | S6's trigger looks up one association per write by this key. |
| An operator command reads `MOUSEION_DATABASE_URL`, opens the store without migrating, prints a report and fails while conflicts remain. | `cmd/vocabularybackfill/main.go:1-3`, `:23-38`, `:40-42` | The preflight follows this precedent. |
| `NormalizeLanguage` trims, lower-cases, maps `_` to `-` and keeps the base subtag. | `internal/canonicalization/canonicalization.go:162-172` | The preflight's language predicate mirrors it. |
| The integration-test template database name hashes the embedded migration files. | `internal/testutil/postgres_integration.go:167-180` | Each step's PR rebuilds the template automatically. |

### Ordered steps

Rules for every step:

- **STEP-1. Names, not numbers.** Each step is one migration pair named after
  the step. Its number is the next free number in `migrations/` when its PR
  merges. `000001_initialize` and every shipped migration stay untouched. An
  unmerged step may be renumbered. A merged step never is.
- **STEP-2. Order.** Steps merge in order S1 to S9. A step does not merge before
  every step it depends on has merged.
- **STEP-3. One transaction.** A step contains no statement that cannot run in
  a transaction block (`CREATE INDEX CONCURRENTLY`, `VACUUM`, explicit
  `BEGIN`/`COMMIT`). A failed step therefore leaves no object behind.
- **STEP-4. Self-contained records.** Each new table ships in one step, together
  with its keys, checks, indexes, mirrored update trigger (§5) and its §9 delete
  guard or owner guard. No later step alters a table that an earlier step of this
  sequence created.
- **STEP-5. Green alone.** A step's PR contains its regenerated `gen/sqlc` and
  any code change that compilation needs. It passes `make lint`, `make test` and
  `make test-integration`. Store code that writes the new records may land with
  the step or with a later PR of the same spec.
- **STEP-6. No effect on existing rows.** A step leaves every pre-existing row
  byte-identical (MIG-01, MIG-03).
- **STEP-7. Bounded lock waits.** Every step locks at least one existing table:
  S6 to S8 change one, and the others reference `users`, `books`,
  `primary_goal_snapshots` or `reading_history`. Each step bounds its own lock
  waits to 5 seconds, for its own transaction only (for example `SET LOCAL
  lock_timeout`). If an unexpected session holds a conflicting lock, the step
  fails and rolls back. It never queues a lock request that would block every
  later reader or writer (MIG-04).
- **STEP-8. No classical writer before S9.** No code that inserts a §4
  association, Book current classical analysis or §8 rich row is reachable
  before S9 has shipped. Their only entry point in scope,
  `cmd/localregistration` (§12), needs S9's Local registration and Local
  selection tables. So when S6 and S7 run, no classical Book exists, and A-4
  confirms it.

| step | name | records added (section) | depends on | purely additive | existing tables touched | effect on existing rows | downstream spec |
| --- | --- | --- | --- | --- | --- | --- | --- |
| S1 | `classical_evidence_storage` | The mirrored update-rejecting function, `reject_classical_evidence_deletion` and `reject_retention_root_deletion` (§9). Package content, Package member, Package blob, Package acquisition, Evidence notice, Text Source revision and its members, Annotation Source revision and its members, Extracted text (§5). | — | Yes | `users` (referenced only) | None | #2061 |
| S2 | `classical_bibliographic_identity` | Work, Edition, Reading unit, Edition reconciliation, Verified identity alias, Book–Reading unit association, Identity conflict (§4). | S1 (Edition `evidence_pin` → Text Source revision; guard functions) | Yes | `users`, `books` (referenced only) | None | #2061 |
| S3 | `vocabulary_identities` | Vocabulary identity (§6). | S1 (guard functions) | Yes | — | None | #2061 |
| S4 | `classical_interpretations_and_bindings` | Interpretation configuration, Notice coverage, Provenance claim, Text–annotation binding (§5). | S1, S2 | Yes | — | None | #2061 |
| S5 | `exact_analyses` | Exact analysis, Capability diagnostic, Diagnostic finding, Usability restriction (§5); Analysis headword (§6); Analysis sentence, Sentence span, Analysis node, Node span, Container component, Annotation occurrence, Node morphology, Basic syntax edge, Secondary syntax edge (§7). | S3, S4 | Yes | — | None | #2061 |
| S6 | `classical_adoption_anchor` | Adopted evidence record (§9); Book current classical analysis (§5), including its foreign key to the Adopted evidence record; the exclusion trigger on `book_current_analyses` (§5). | S1, S2, S5 | New tables plus one new trigger on an existing table | `book_current_analyses` (trigger) | None | #2061 (storage); #2059 writes it through adoption |
| S7 | `rich_learner_attachments` | Exact occurrence decision, Exact Lemma review flag, Rich count readiness, Rich vocabulary count, Rich snapshot membership, Snapshot count contribution, Rich Known vocabulary (§8), each with its §9 foreign key to the Adopted evidence record; snapshot representation on `primary_goal_snapshots` (§8, §9); the tuple-row trigger on `primary_goal_snapshot_vocabulary` (§8). | S3, S5, S6 | No: relaxes five `NOT NULL` columns, guarded by a new check (see [Changes to existing tables](#changes-to-existing-tables)) | `primary_goal_snapshots`, `primary_goal_snapshot_vocabulary`; `users`, `books`, `reading_history` (referenced only) | None | #2061 (storage); #2059 writes counts at adoption; #2060 uses it in learner workflows |
| S8 | `validate_snapshot_representation` | No record. Validates the constraints that S7 added `NOT VALID`. | S7 | Yes: reads rows, changes none | `primary_goal_snapshots` | None | #2061 |
| S9 | `classical_preparation` | Local registration (§4); Installed pin set, Local selection (§12); Preparation request, Preparation attempt, Staged count row (§11). | S1, S2, S3, S4, S5 | Yes | `users` (referenced only) | None | #2059 |

#2060 adds no step. If it needs structure, that structure gets a new step after
S9, under these rules. §4's Unit offering and Claimed alias get no step here,
because they depend on OD-09's connection record and on the Stage 3 discovery
spec (OD-66).

**Why the order differs from the ticket's example.** Three foreign-key
dependencies force it:

- Edition `evidence_pin` references a Text Source revision (§4, §5), so evidence
  storage comes before bibliographic records.
- An Analysis node references its Vocabulary identity (§7), so identities come
  before nodes.
- Under RET-TX and STEP-4, the §5 current row and the §8 records carry their
  foreign key to the Adopted evidence record in their own definitions. So the
  retention anchor ships before learner attachments, not after them, and no new
  table is altered later.

### Changes to existing tables

Written against OD-12 option (2) and OD-32 option (1), which §5 and §8 use. If
either resolves differently, this table is revised before any SQL is written.
S7 alters an existing table instead of adding one only because §8 anchors rich
readings on the existing snapshot row. OD-32 records why a parallel anchor would
be the second learner-state machine that ADR 0098 rules out
(`doc/adr/0098-shared-core-classical-acquisition.md:118-119`). Every other
record is a new table.

Lock names are PostgreSQL table-lock modes. Every step runs in the startup
migration path, before the server accepts requests (`cmd/server/main.go:146-153`),
so the locks contend only with operator commands or a server instance still
running from before the deployment.

| # | table | change | step | lock and runtime on a populated table | `NOT VALID` + validate | why it is non-breaking |
| --- | --- | --- | --- | --- | --- | --- |
| X-1 | `book_current_analyses` | New `BEFORE INSERT OR UPDATE` row trigger that rejects a row whose Book has a §4 association (§5). | S6 | `SHARE ROW EXCLUSIVE` until S6 commits: writes wait, reads continue. No scan. Afterwards, each EPUB publication adds one primary-key lookup on the association. | Not applicable: a trigger checks only new writes, and no association exists when S6 runs (STEP-8, A-4). | No Book has an association when S6 runs. §4 inserts an association only in the transaction that creates its Book, so no EPUB Book ever gains one. `publishEligibleAnalysis` (`internal/analysis/jobs.go:1711-1743`) is unchanged (EVD-28). |
| X-2 | `primary_goal_snapshots` | Add `exact_analysis_id uuid NULL`, with no default (§8). | S7 | `ACCESS EXCLUSIVE` for a catalog-only change. A nullable column without a default rewrites no row. | Not applicable | Every existing row reads `NULL`, which is the legacy representation. |
| X-3 | `primary_goal_snapshots` | Drop `NOT NULL` from `source_material_id`, `analysis_run_id`, `content_revision_id`, `content_snapshot_id` and `corpus_id` (OD-32). | S7 | `ACCESS EXCLUSIVE`, catalog-only, no scan. | Not applicable | X-4 still requires all five for a legacy row, so no legacy row can lose one. Their existing foreign keys are unchanged. |
| X-4 | `primary_goal_snapshots` | Check that exactly one representation is set (§8). | S7 adds it `NOT VALID`; S8 validates it | S7: `ACCESS EXCLUSIVE`, catalog-only, no scan. The check applies to every row inserted or updated after S7 commits. S8: `SHARE UPDATE EXCLUSIVE`, so reads and writes continue during one sequential scan (one row per snapshot ever frozen). | Yes | Every existing row satisfies the legacy branch. The five columns are `NOT NULL` until S7, and `exact_analysis_id` is new. |
| X-5 | `primary_goal_snapshots` | Foreign key `exact_analysis_id` → Exact analysis `RESTRICT` (§8), added as a separate constraint after X-2, not inline in the column definition. | S7 adds it `NOT VALID`; S8 validates it | S7: `SHARE ROW EXCLUSIVE` on both tables, catalog-only. S8: `SHARE UPDATE EXCLUSIVE` on snapshots and `ROW SHARE` on the referenced table, with a scan that finds only `NULL`. | Yes | Every existing value is `NULL`. |
| X-6 | `primary_goal_snapshots` | Foreign key `(owner_id, book_id, exact_analysis_id)` → Adopted evidence record `NO ACTION` (§9). | S7 adds it `NOT VALID`; S8 validates it | As X-5. | Yes | `MATCH SIMPLE` skips a row whose `exact_analysis_id` is `NULL`, so every legacy row passes. |
| X-7 | `primary_goal_snapshots` | Index `(owner_id, book_id, exact_analysis_id) WHERE exact_analysis_id IS NOT NULL`. This serves the owner-cascade check that §9 "Indexes" requires. | S7 | `SHARE` while the index builds: writes wait, reads continue. The build scans the heap but stores no entry for an existing row. | Not applicable | The cascade's equality on `exact_analysis_id` implies `IS NOT NULL`, so the partial index serves it. No existing query names the column, so no existing plan changes. |
| X-8 | `primary_goal_snapshots` | New `BEFORE INSERT` row trigger requiring a rich snapshot's `exact_analysis_id` to be the Book's current classical analysis (§8). | S7 | `SHARE ROW EXCLUSIVE`, no scan. | Not applicable | The trigger returns at once when `exact_analysis_id` is `NULL`, which is every legacy insert. |
| X-9 | `primary_goal_snapshot_vocabulary` | New `BEFORE INSERT` row trigger that rejects a tuple row for a rich snapshot (§8). | S7 | `SHARE ROW EXCLUSIVE`, no scan. Afterwards, each inserted tuple row adds one primary-key lookup on its snapshot. | Not applicable | Every existing snapshot is legacy, so Start keeps writing tuple rows as it does today (MIG-06). |

**References from new tables into existing ones.** These change no definition
of an existing table. The new tables reference `users(id)`,
`books(owner_id, id)`, `primary_goal_snapshots(owner_id, id)` and
`reading_history(completion_id)`. Creating each foreign key takes `SHARE ROW
EXCLUSIVE` on the referenced table until the step commits. No scan is needed,
because the new table is empty. After rollout, a delete from a referenced table,
such as the owner cascade, checks every new table that references it. So each of
these foreign keys gets an index led by its referencing columns, in the step
that creates the table (STEP-4). A primary key or §9 "Indexes" covers most of
them. These need their own index: Identity conflict `(owner_id, book_id)` and
`(owner_id, other_book_id)`; Snapshot count contribution `(owner_id, book_id)`;
and Rich Known `(completion_id)`.

**Unchanged.** `books`, `known_vocabulary`, `reading_history`, `analysis_runs`,
`corpora`, `corpus_sentences`, `corpus_tokens`, `vocabulary_browse_counts`, the
occurrence decision tables, every deck table, the `current_analysis_identity`
view and every baseline function and trigger.

### Legacy-`grc` preflight

**Delivery: an operator command.** The proposed name is
`cmd/legacygrcpreflight`, one command per operation, as §12 does with
`cmd/localregistration`. It follows `cmd/vocabularybackfill`: it reads
`MOUSEION_DATABASE_URL`, opens the store without migrating, and prints a report.
Unlike §12's one-line summary, the report is a JSON document, because it lists
IDs.

- It runs every check in one `REPEATABLE READ READ ONLY` transaction. It writes
  nothing and takes only the locks that ordinary reads take.
- It is not a River job and has no schedule.
- A command, not a SQL script, because reviewed code then enforces the
  content-free output and seam (d) can test it (MIG-07).
  `cmd/localregistration` calls the same function before registering (STOP-1).
- The command merges before S1, with an empty step manifest, so that run 1
  below is possible. Each step's PR adds that step's manifest entry (R-2).

It runs against both the pre-rollout schema and any later step. Checks on new
records run only when those records exist.

**When it runs:**

1. after the pre-rollout backup, before the binary that carries S1 is deployed;
2. after each step is deployed;
3. inside every `cmd/localregistration` run that writes (STOP-1);
4. after every restore, following §9's integrity check.

**Language classification.** A language value **is `grc`** exactly when
`canonicalization.NormalizeLanguage` returns `grc` for it. The command does
not reimplement that rule in SQL. For each table below, it reads the distinct
values of the language column, classifies each value in Go, and then counts and
lists the rows that hold a `grc` value, by byte equality on that value. So
`GRC`, `grc-x-test`, `grc_Grek` and a value with surrounding whitespace are all
`grc`, exactly as the application would treat them.

**Legacy rows only.** The classical path itself writes `grc` values into
existing tables: a classical Book's `books.language_tag`, its `primary_goals`
row, its rich snapshot, its `reading_history` row, and §12's
`supported_languages` row. A row is **legacy** unless it belongs to a classical
Book (one with a §4 association) or to a rich snapshot (`exact_analysis_id IS
NOT NULL`). The G checks count legacy rows only, so a later run after a
successful import still reports `clear` (MIG-14). Before S2 and S7 exist, every
row is legacy.

**Legacy checks.** Each check counts and lists, per owner, the rows that match.
A row whose key contains vocabulary text is reported only by the UUID columns
of its key, with a count. "Is `grc`" in this table means the classification
above.

| check | category | table (path:line) | a row matches when | reported |
| --- | --- | --- | --- | --- |
| G-1 | Books | `books` (`migrations/000001_initialize.up.sql:268-280`) | `language_tag` is `grc` and the Book has no §4 association | Book IDs |
| G-2 | Analyses | `source_materials` (`:412-426`), `analysis_runs` (`:193-215`), `book_current_analyses` (`:239-245`), `corpus_tokens` (`:380-405`) | the source material's `language` is `grc`, or its Book matches G-1; a run of such a source material; a current-analysis row of such a Book; a token whose `language` is `grc` | source material IDs, run IDs, Book IDs; token counts per run ID |
| G-3 | Current readings | `primary_goals` (`:1291-1299`) | `language` is `grc` and the Book is not classical | Book and snapshot IDs |
| G-4 | Known vocabulary | `known_vocabulary` (`:1104-1111`) | `language` is `grc` | row IDs |
| G-5 | Reservations | `primary_goal_snapshot_vocabulary` (`migrations/000007_primary_goal_snapshots.up.sql:35-51`) | a row of an unreleased snapshot named by a `primary_goals` row, where the row or its snapshot is `grc` | snapshot IDs, row counts |
| G-6 | Snapshots and history | `primary_goal_snapshots` (`:14-33`), `primary_goal_snapshot_vocabulary`, `reading_history` (`migrations/000004_reading_history.up.sql:3-12`) | `language` is `grc`, the snapshot is not rich, and the Book is not classical | snapshot IDs; row counts per snapshot ID; completion IDs |
| G-7 | Occurrence decisions | `occurrence_lemma_corrections` (`migrations/000023_occurrence_lemma_corrections.up.sql:1-25`, with exclusions from `migrations/000024_occurrence_vocabulary_exclusions.up.sql`), `occurrence_lemma_review_flags` (`migrations/000025_occurrence_lemma_review_flags.up.sql:1-27`) | the Book matches G-1, or the run matches G-2 | counts of corrections, exclusions and flags per (Book ID, run ID) |
| G-8 | Browse counts | `vocabulary_browse_counts` (`migrations/000031_vocabulary_browse_counts.up.sql:1-15`), `vocabulary_browse_count_readiness` (`:20-32`) | `language` is `grc` | counts per (Book ID, run ID) |
| G-9 | Deck specifications | `deck_preparation_manifest_items` (`migrations/000001_initialize.up.sql:676-711`), `deck_preparation_vocabulary` (`:876-884`), `generated_vocabulary` (`:1089-1097`), `decks` (`:935-941`), `vocabulary_browse_selections`, `custom_vocabulary_decks`, `custom_vocabulary_deck_identities` (`migrations/000026_custom_vocabulary_selections.up.sql:1-12`, `:14-27`, `:29-42`), `custom_vocabulary_deck_preparations`, `custom_vocabulary_deck_preparation_identities` (`migrations/000027_custom_deck_preparations.up.sql:1-36`, `:45-59`) | `language` is `grc` | the UUID key columns of each row, or of its containing deck or preparation, with counts |
| G-10 | Other learner and analysis rows | Owner-scoped: `vocabulary_states` (`migrations/000001_initialize.up.sql:1447-1456`), `selection_candidates` (`:1335-1356`), `curated_sentences` (`:541-550`), `example_sentences` (`:1031-1047`). Installation-wide: `shared_lemmas` (`:1377-1386`), `enrichment_cache` (`:948-968`), `normalized_corpus_artifacts` (`:1251-1260`), `frequency_datasets` (`:1054-1064`), `frequency_entries` (`:1071-1082`) | `language` is `grc` | counts per owner; owner `null` means installation-wide |
| INF-1 | Preferences and display metadata (informational, never blocks) | `users.active_study_language` (`migrations/000001_initialize.up.sql:1433-1441`), `supported_languages` (`:1422-1426`), `opds_connections` (`:1267-1277`) | the value is `grc` | counts per owner. These hold no vocabulary or learner fact, and §12 itself writes `supported_languages`, so they are reported but never block. |

```sql
-- illustrative, not a migration
-- 1. Distinct values, classified in Go with NormalizeLanguage.
SELECT DISTINCT language FROM known_vocabulary;
-- 2. Rows holding the values classified as grc, by byte equality.
SELECT owner_id, count(*) AS n, array_agg(id ORDER BY id) AS ids
FROM known_vocabulary
WHERE language = ANY($1::text[])
GROUP BY owner_id
ORDER BY owner_id;
```

**Assumption checks.** These cover what the new constraints and triggers rely
on that existing constraints do not guarantee, or that a faulty restore could
lose.

| check | assumption | relied on by | failure code |
| --- | --- | --- | --- |
| A-1 | These unique keys exist with their expected columns and are validated: `users_pkey` (`migrations/000001_initialize.up.sql:2144`), `books_owner_id_id_key` (`:1560`), `book_current_analyses_pkey` (`:1544`), `primary_goals_pkey` (`:1968`), `known_vocabulary_owner_id_language_canonical_lemma_upos_key` (`:1936`), `primary_goal_snapshots_pkey` (`migrations/000007_primary_goal_snapshots.up.sql:26`), `reading_history_pkey` (`migrations/000010_reading_history_append_only.up.sql:9`). | Every new composite foreign key into `books`, snapshots and history; the X-1 lookup; §6's rule that at most one legacy Known row equals a rich identity | `referenced_key_missing` (constraint name) |
| A-2 | Every `primary_goal_snapshots` row has all five legacy columns set, and a `NULL` `exact_analysis_id` once that column exists. | X-4's validation in S8 | `snapshot_not_legacy_complete` (snapshot IDs) |
| A-3 | Each `primary_goals` row's snapshot has the goal's language, and each `primary_goal_snapshot_vocabulary` row has its snapshot's language. The composite keys enforce only the owner (`migrations/000007_primary_goal_snapshots.up.sql:49`, `:53-54`). | Reserved, which filters both languages (`sqlc/queries/deck_preparations.sql:289-296`); §8's membership language trigger | `snapshot_language_mismatch` (goal and snapshot IDs) |
| A-4 | Once S6 exists, no (owner, Book) has both a `book_current_analyses` row and a §4 association. | X-1; §8 "Representation selection" | `anchor_conflict` (Book IDs) |

**Rollout checks.** These are reported in every run.

| check | verifies | failure code |
| --- | --- | --- |
| R-1 | `schema_migrations` is not dirty. The report includes its version. | `migration_dirty` |
| R-2 | For each of S1 to S9, every object of the step exists (from a manifest that each step's PR adds to the command), or none does. The complete steps form a prefix: S1 to Sk complete, the rest absent. | `rollout_partial` (step names) |
| R-3 | When S8 is complete, every constraint that S7 added is validated. | `constraint_not_validated` (constraint names) |

**Result and stop rule.**

- **Result.** The result is `clear`, with exit code 0, exactly when every G
  count is 0 and every A and R check passes. INF-1 never affects the result. Otherwise the result is `blocked`,
  with exit code 2. If the command cannot finish (connection, permission or
  timeout), it exits 1, and that run is never treated as clear.
- **STOP-1.** `cmd/localregistration` runs the preflight after its configuration
  checks (§12 P-1 to P-5) and before it reads any local file. If the result is
  not `clear`, it refuses with `legacy_grc_preflight_blocked`, exits 4 ("refused
  by reviewed configuration") and writes nothing. `-verify-only` opens no
  database and writes nothing, so it skips the preflight. In this design's
  scope, §12 is the only path that creates a classical Book (catalogue
  offerings are Stage 3, OD-66), so no Greek import can start while the result
  is blocked. A later import path must apply the same gate.
- **STOP-2.** When a G check finds rows, Greek import stays stopped for the
  whole installation until a separately approved reconciliation plan exists
  (OD-63). The plan states how its outcome shows in the preflight, for example
  through a reviewed list of reconciled row IDs, so that a `clear` result
  stays the only condition for import.
- **STOP-3.** Rows are never converted. No step, command, job, adoption or
  learner transition updates, deletes or copies a legacy `grc` row, or derives a
  rich identity, Known, membership, decision or count from one (MIG-10). The
  preflight writes nothing.
- **STOP-4.** A failed A or R check also blocks. Its remedy is a restore or a
  corrective roll-forward, not a reconciliation plan.
- Structural steps may still ship while the result is blocked. They change no
  row (STEP-6), and Greek import stays disabled.

**Output.** The report is one JSON document on stdout, in format
`legacy-grc-preflight/1`. It contains only check IDs, schema identifiers (table,
constraint and step names), owner and row UUIDs, integers, booleans and the
result. It never contains a lemma, surface, form, title, author, text, JSON
payload, file name, email address or display name. Log lines follow the same
rule. Synthetic example:

```json
{"format":"legacy-grc-preflight/1","result":"blocked",
 "schema":{"version":"<current version>","dirty":false,
           "steps":[{"step":"S1","state":"complete"},{"step":"S2","state":"absent"}]},
 "findings":[{"check":"G-4","table":"known_vocabulary",
              "owner_id":"00000000-0000-4000-8000-00000000000a","count":2,
              "ids":["00000000-0000-4000-8000-0000000000b1","00000000-0000-4000-8000-0000000000b2"]}],
 "assumptions":[{"check":"A-3","result":"pass","count":0,"ids":[]}],
 "rollout":[{"check":"R-1","result":"pass"}]}
```

### Data-only work

- **None is required to add Greek.** Every new record starts empty. Only
  application operations write to it: Local registration, preparation and
  adoption (§11), and the learner transitions (§8). S8's validation changes no
  row. No EPUB Work or Edition, correction, identity, snapshot, Known or Browse
  backfill is required or authorized (handoff Stage 2 "Migration/backfill
  separation").
- Rich count projections start per Book in §11's `prepare_projections` stage and
  the adoption commit, not in a migration.
- Reconciling legacy `grc` rows (STOP-2) is outside this design and needs its
  own approved plan. So does any changed key in a later normalization profile
  (§6 "Version as provenance").
- If a later additive projection initialization becomes necessary, it is never
  part of a structural step. It needs its own approved plan covering owner scope,
  exact input versions, checkpoints and resumability, idempotency and expected
  counts, plus ADR 0038's backfill contract (who runs it, transaction and
  locking, failure detection, and forward fix). It never alters immutable facts.
  None is needed now, so this section adds no Open decision for it.

### Recovery

- **REC-1. `down` is not a rollback.** Each step ships a `down` file, as every
  migration does, for development and tests only. Production applies `Up` only
  (`internal/persistence/persistence.go:92`; ADR 0070). Once classical rows
  exist, a `down` file destroys data or fails. `DROP TABLE` on S1–S6 or S9
  bypasses the row-level delete guards and deletes evidence, and S7's `down`
  cannot restore `NOT NULL` once a rich snapshot exists. Running a `down` file
  against an installation that holds classical rows is forbidden.
- **REC-2. Roll forward by default.** A defect in a shipped step, such as a wrong
  constraint, a missing index or a faulty trigger, is corrected by a new
  migration with the next free number. It is reviewed like any step and applies
  to a populated database without changing existing rows, unless it carries its
  own approved data plan. Shipped files are never edited. Running a binary older
  than the database schema is not supported. Roll forward instead.
- **REC-3. Backup.** Before deploying each step, the operator takes a full,
  consistent logical dump, as §9 "Restore expectations and integrity check"
  defines: one snapshot of the whole database, including `schema_migrations`
  and River's tables. The compose stack runs PostgreSQL 18 (`compose.yaml:13`).
  `pg_dump` and `pg_restore` must be of the server's major version or newer.
- **REC-4. Restore is the evidence-loss path.** Use it when evidence is lost or
  corrupted (an integrity-check failure), or when a step must be abandoned
  (REC-5). The procedure:
  1. Stop the server.
  2. Create an empty database.
  3. Restore with `pg_restore --single-transaction --exit-on-error` (§9).
  4. Run §9's evidence integrity check.
  5. Run the preflight. R-1 to R-3 must pass.
  6. Start the server. It applies any newer steps at startup.

  Restoring a pre-step dump discards every write made after it. The operator
  decides this explicitly. Nothing restores automatically.
- **REC-5. Detecting a half-applied rollout.**
  - **Within a step.** A partial step cannot exist (STEP-3). A failed step
    leaves its version dirty, and the server refuses to start
    (`migrate.go:215-223`, `cmd/server/main.go:149`). R-1 and the startup error
    both report it.
  - **Recovery from an environmental failure, such as a lock timeout.** Confirm
    with R-2 that none of the failed step's objects exist. Reset
    `schema_migrations` to the previous version with `dirty = false`
    (golang-migrate's force semantics). Remove the cause and restart.
  - **A step whose own SQL fails on a real installation** means MIG-01 or MIG-03
    missed a case. Recover by restoring the pre-step backup (REC-4), then take a
    reviewed decision.
  - **Across steps.** The complete steps must be a prefix (R-2). A gap or a
    partial step can only come from manual DDL or a restore of a partial dump.
    It is reported as `rollout_partial` and recovered by restore.
- **REC-6. Restore test.** MIG-11 and MIG-12 specify it:
  1. Build classical evidence for two owners through the §11 adoption interface,
     with synthetic packages, including an older analysis held by its Adopted
     evidence record, an exact decision, a rich snapshot, a completion and Rich
     Known.
  2. Dump the database with `pg_dump -Fc`, run inside the database container so
     that the client matches the server.
  3. Restore into a fresh database, and pass §9's integrity check with no
     restriction.

  With the `MOUSEION_TEST_DATABASE_URL` fallback, the test runs `pg_dump` and
  `pg_restore` from `PATH`. If they are missing, the test fails rather than
  skips.

### Generated artifacts and review

- **sqlc.** The schema input is `migrations/*.up.sql` and the queries are in
  `sqlc/queries` (`sqlc.yaml:12-13`).
  - Every step's PR runs `make sqlc`, which uses the pinned v1.31.1, and commits
    `gen/sqlc`. CI asserts that nothing drifts (MIG-13).
  - A step that adds tables adds model structs even when it adds no query.
  - In S7, the five relaxed columns become nullable. Their generated type changes
    from `string` to `pgtype.UUID` wherever a query selects them directly
    (`sqlc.yaml:22-27`). The S7 PR adapts those callers so behaviour does not
    change. Queries that read the five columns run only for legacy snapshots, as
    OD-32 states, and MIG-02 verifies the result. `exact_analysis_id` appears in
    the snapshot model.
  - If sqlc cannot parse a construct that a step needs, such as `UNIQUE NULLS
    NOT DISTINCT`, a deferred constraint trigger or a PL/pgSQL body, the PR stops
    and returns to design review. DDL is never hidden from sqlc, and generated
    code is never hand-edited.
- **Other artifacts.** `migrations/embed.go` needs no change. templ and protobuf
  output are unaffected. No `go.mod` change is expected.
- **Review.** Every step adds files under `migrations/`, which CODEOWNERS
  protects (`.github/CODEOWNERS:12`). So every migration PR, S1 to S9, needs
  human review and is never auto-merged.
  - A PR that also changes `doc/adr/` or `doc/product.md` (`:15-16`), or
    `go.mod` and `go.sum` (`:24-25`), needs the same review.
  - The ADR changes this design implies, such as OD-43's ADR 0091 amendment, a
    new ADR and its `doc/product.md` index entry, are separate reviewed changes.
  - Each step's PR names its step, its dependencies, the MIG rows it satisfies
    and its rows in the [Changes to existing tables](#changes-to-existing-tables)
    table.
- **Approval.** No step is written until §18 records approval (§1).

### Invariants

Each invariant is covered by the `MIG` rows in [§15](#15-acceptance-test-matrix).

1. **Order.** Each step applies alone on top of the steps it depends on, and the
   complete steps always form a prefix (MIG-03, MIG-04).
2. **No effect on existing rows.** Applying S1 to S9 leaves every existing row
   and every characterization outcome unchanged (MIG-01, MIG-02).
3. **Existing tables.** The only changes to existing tables are X-1 to X-9.
   Legacy writes keep working, and S8 validates every `NOT VALID` constraint
   (MIG-05, MIG-06).
4. **Atomic steps.** A failed step leaves no partial object, is reported as
   dirty, and stops server start (MIG-04).
5. **Preflight.** It is read-only and content-free, and reports counts and IDs
   per owner. Any `grc` row blocks Greek import for the whole installation.
   Legacy rows are never converted, and rows written by the classical path never count as legacy (MIG-07, MIG-08, MIG-10, MIG-14).
6. **Assumptions.** A violated assumption that a new constraint relies on is
   reported and blocks (MIG-09).
7. **Restore.** A restore of blobs plus records passes the integrity check. A
   restore that is missing a blob fails it, and the affected Book shows
   unavailable (MIG-11, MIG-12).
8. **Generated code.** Regenerated sqlc output matches the committed code at
   every step (MIG-13).

### Conflicts and dependencies noted

- **§6 "Legacy compatibility" and OD-24.** They recommend isolating legacy `grc`
  rows and continuing rollout. This ticket requires rollout of Greek import to
  stop while any `grc` row exists. §13 follows the ticket (STOP-2). §6 is not
  rewritten (OD-63).
- **§9 IC-6.** IC-6 does not name the restriction subject for a missing blob
  that only acquisitions reference (OD-64). MIG-12 asserts only that the Book
  shows unavailable.
- **§15 seams.** The seams include no generated-artifact check, which MIG-13
  needs (OD-65).
- **§4 Unit offering and Claimed alias** depend on OD-09's connection record and
  on Stage 3. No step includes them (OD-66).
- **§9 "Indexes".** X-7 makes §9's snapshot index partial. This refines §9 and
  does not change it.
- **§12 order of operations and exit codes.** §12 does not run the preflight.
  STOP-1 inserts it after §12's P-5, refusing with exit 4 and
  `legacy_grc_preflight_blocked`, which joins §10's registry as a
  command-output code. §12's exit-code table and step list are not rewritten.
  §12's step IDs P-1 to P-8 are why the legacy checks here are named G-1 to
  G-10.
- **§12 records.** §12's Installed pin set and Local selection ship in S9 with
  Local registration. §12's registration transaction writes S1, S2 and S4
  records, so the command needs every step up to S9 (STEP-8).
- **§12 `supported_languages` write.** §12 inserts a `grc` display-name row.
  That is why INF-1 is informational, and why the G checks exclude classical
  Books and rich snapshots.
- **OD-12 and OD-32.** X-1 to X-9 assume OD-12 option (2) and OD-32 option (1).
- **OD-41.** MIG rows that run operator commands use seam (d), widened as for
  RET-14 and RET-16.
- **§8's illustrative DDL** adds the `exact_analysis_id` foreign key inline in
  `ADD COLUMN`, which validates it at once. §13 adds it as a separate `NOT
  VALID` constraint that S8 validates (X-5). This refines §8 and does not
  change it.

## 14. Public and local material

Verified at commit `00a63be6b38f9b54d359b6e68ed014af30ab7b25` (`main`). Every
`path:line` in this section refers to that commit.

### Summary

- All material is either **public** or **local only**. Public material can
  appear in the repository, issues, pull requests, public CI and messages to
  external services. Local-only material stays in the operator's installation,
  its backups and the operator's own files. The class says where material may
  appear. It authorizes no operation.
- Public CI runs only on synthetic fixtures and the existing public
  characterization fixtures. Building, testing or releasing the code never needs
  the retained package, its pin file or the frozen table.
- Every log, diagnostic and command output is content-free by construction
  (§10, §12, §13). Content-free output from a run on retained material is still
  local-only.
- Releasing source code and publishing private data are separate operations.
  This design authorizes no data publication and no provider transmission.
- The open public-release questions in #2054 and #2056 block only the
  operations they name. They do not block any generic interface, synthetic test
  or the operator-local route.

### Classification

| material | class | examples, and where it lives | owning section |
| --- | --- | --- | --- |
| This design, and the ADRs and handoff it cites | public | `doc/` | — |
| Synthetic fixtures | public | `example-*` pin sets, packages, tables and definitions; the `ancient-greek-normalization/1`, `vocabulary-identity-comparison/1` and `source-decoding/1` cases; synthetic candidates and store-contract seeds | §6, §10, §12, §15 |
| Structural examples | public | Invented forms such as `λέξις` with `example:ex:1` and `example:ex:2`. | §6, §7 |
| Existing characterization fixtures | public | The Stanza fixtures listed in §10 "Current anchors" | §10 |
| Public interpretation version identifiers | public | Slot identifiers such as `oga-greta-lemma/2`, `oga-agdt-upos/4`, `oga-agdt-feats/1`, `oga-anabasis1-research-repairs/2`, `oga-reading-terminal/2` and `ancient-greek/1`; the selection policy `grc-earliest-capped/1`; adapter, projection, correspondence, contract and pin-set identifiers | §5, §10, §12 |
| Code | public | Adapters, validation, registries, reason codes and their fixed messages, synthetic pin sets, the public fields of retained pin-set entries, and pin-file commitments (see below) | §10, §12 |
| The retained package | local only | The package file the operator holds, and its stored Package blob | §5, §12 |
| Its hashes and member names | local only | Transport digest, Package content digest, and every member's path, size and digest | §5, §12 |
| The retained pin file | local only | The operator's `pin-file/1` file, and its Installed pin set row | §12 |
| The frozen exception-value/precondition table | local only | The table values, the operator's table file and its `interpretation_table` blob. The table digest is local too (OD-70). | §5, §12 |
| Exception-table lineage audit | local only, apart from its summary | Per-record lineage and outcome are local. The public summary gives counts per lineage class (documentation, retained source, Morpheus, treebank) and per outcome (kept, replaced, dropped), and any further repair-layer identifier. | §5 |
| Interpretation lineage statements | public | Each reviewed slot definition's statement of the documentation it is written from, for example the AGDT tagset documentation and the UD UPOS and FEATS definitions for `oga-agdt-upos/4` and `oga-agdt-feats/1` | §5 |
| Book 1 text and derived diagnostics | local only | Extracted text, §7 rows, Analysis headwords, Capability diagnostics and Diagnostic findings of an analysis of retained evidence; request `reason` and `diagnostics`; counts, snapshots, Known and history built from it | §5–§8, §11 |
| Real-fixture parity output | local only | Every result of a [local-only acceptance step](#local-only-acceptance-steps), apart from whether it passed | this section |
| Installation data | local only | The installation database, and every dump or backup of it (§9, §13 REC-3) | §9, §13 |

### Retained pin-set fields

§12 reads a retained pin set from two places: reviewed code, and the
authenticated pin file. It leaves this section to say which fields are private.
A field is **local** exactly when its value is, or names, a package or member
digest, a member path, or a frozen table value or digest. Every other field is
public and lives in reviewed code.

| pin-set field (§5 "Pins", §12 "Inputs") | class | lives in |
| --- | --- | --- |
| Identifier and version | public | reviewed code |
| Pin-file commitment | public | reviewed code |
| Accepted transport digests, and the Package content digest | local | pin file |
| Each member's path, role, size and digest | local | pin file |
| Revision members | local, because they name member paths | pin file |
| Notice coverage of package members | local, because it names member paths | pin file |
| Table digest of each listed interpretation | local (OD-70) | pin file |
| Reading unit, `source_bounds`, projection, correspondence, adapter, and the list of supported interpretations | public | reviewed code |
| Revision origins and release metadata, `package_release`, provenance claims, and record notices that Mouseion issues | public | reviewed code |

**The commitment is public (OD-57).** The commitment is the SHA-256 of the exact
pin-file bytes. Publishing it reveals no package, member or table digest and no
member name. Because SHA-256 is preimage resistant, the commitment only lets
someone who already holds the exact pin file confirm that they hold it. This
confirms what OD-57 assumes. A synthetic pin set keeps every field in reviewed
code, because its values are synthetic.

### Destinations

| destination | may hold public material | may hold local-only material |
| --- | --- | --- |
| Public repository: code, `testdata`, `doc/`, commit messages | yes | never |
| Issues, pull requests and review comments | yes | never |
| Public CI inputs, logs and uploaded artifacts. Today these are the browser fixture directory and the Playwright failure reports (`.github/workflows/ci.yml:321-327`, `:406-414`), and the SonarQube Cloud scan (`.github/workflows/sonarqube.yml:100-102`). | yes | never |
| External model and provider calls, including AI agent prompts, tool output and replies | yes | never. Card-provider transmission is #2063's own separately approved operation. Approving it does not approve agent use, and agent use does not approve it (#2063). |
| The installation database, its dumps and backups | yes | yes |
| The operator's files: the retained package, pin file and table | — | yes |
| Local acceptance runs and their outputs | yes | yes, and they stay local |

### Rules

- **PUB-R1. Public CI is synthetic.** Every test that public CI runs uses
  synthetic fixtures or the existing public characterization fixtures. No test
  reads a file outside the repository. A test that needs retained material is a
  local-only acceptance step (below), never a §15 row (PUB-01).
- **PUB-R2. Output is content-free.** No log, diagnostic, River job argument or
  metadata, request row, command output, report or CI artifact contains package
  bytes, member content, text, lemma or table values, member paths or file
  paths. The mechanisms are §10 "Diagnostic shape", §12 "Logging and privacy"
  and §13 "Output" (PUB-02). Digests are local-only material, but they are not
  content. §12 prints none, and §9's unreferenced-evidence report lists them
  (OD-72).
- **PUB-R3. Local material never leaves the installation.** Local-only material
  never goes into an issue, pull request, review comment, CI run, or a message to
  an external model or provider, including an AI agent conversation. An agent
  whose next step would need local material stops and asks for separately scoped
  authorization (#2059 "Implementation Decisions"). The classical flow opens no
  network connection other than PostgreSQL (LOC-20, LOC-30, VAL-11). Review
  checks the human-channel part of this rule (OD-75).
- **PUB-R4. Code release and data publication are separate.** Releasing code
  ships synthetic fixtures, public identifiers and commitments only. It never
  makes the retained pin set usable without the operator's local files
  (PUB-01). Publishing a package, pin file, table, text or derived output is a
  separate operation with its own operation-specific disposition: #2054 for the
  interpretation and affected source-derived values, and #2056 for catalog
  publication. This design authorizes none of them.
- **PUB-R5. Unknown public-release applicability does not block generic work.**
  #2054 and #2056 stay open. They gate only the public operations they name.
  They do not gate §4–§13's interfaces, any §15 row, or the operator-local route,
  whose local-operation assessment #2059 owns. The rights gate that §11 and §12
  check is per operation (PUB-03, OD-69).

### Local-only acceptance steps

Every §15 row runs on public, synthetic material. The steps below need retained
material. They run only on the operator's installation, with NLP stopped and no
outbound network, after the authorization that each spec names. A step's public
report says only that it ran and whether it passed. Time and memory measurements
(L-5) carry no content and may also be reported.

| spec | step | synthetic counterpart in §15 |
| --- | --- | --- |
| #2061 | None. All of #2061's acceptance is synthetic. Real-source parity belongs to #2059 (#2061 "Testing Decisions"). | — |
| #2059 | **L-1.** Register the retained package with `cmd/localregistration`, naming the retained pin set and interpretation and supplying the pin file and the frozen table (§12). | LOC-14, LOC-18, LOC-19 |
| handoff Stage 1 | **L-0.** Before the retained table is frozen: the exception-table lineage audit (§5), the confirmation that the core annotation members carry the morphology tags that `oga-agdt-feats/1` maps, and the re-run of complete-unit accounting and selection under the new interpretation. The re-run's numbers are accepted as they come out and become the expected values for L-2. No mapping or table value is tuned to reproduce 438/434. | OCC-09, OCC-10, OCC-17 |
| #2059 | **L-2.** Real-fixture comparison. Prepare and adopt the retained Book 1, then compare complete-unit accounting, identities, selected identities, sentence inputs and omissions with the L-0 result, and report the explicit differences from the earlier 438/434 baseline (#2059 "Testing Decisions", to be restated, §16). | VAL-08, OCC-09, OCC-10, OCC-17, ADP-15 |
| #2059 | **L-3.** Compare every learner-relevant identity, POS and accounting decision, every exact complete sentence span and text, and every selected target span and surface. | OCC-01 to OCC-06, IDN-28 |
| #2059 | **L-4.** Reproduce the retained negative-test classes and precondition-mutation rejections at the production interface, by mutating the retained package or the frozen table. | LOC-01 to LOC-13, LOC-19, VAL-16 |
| #2059 | **L-5.** Measure production parsing, storage, memory and time against the operational envelope. | EVD-04, EVD-05, LOC-05, LOC-07, LOC-08 |
| #2059 | **L-6.** Exact backup, restore and integrity check of the real evidence, with no upstream access. | EVD-30, MIG-11, MIG-12, RET-15 |
| #2059 | **L-7.** The local integration operation's inventory, notice handling, and supported, restricted or unresolved assessment (#2059 "Implementation Decisions"). | EVD-08, EVD-24 |
| #2060 | **L-8.** The separately authorized private Book 1 usability acceptance: My Books → To Read → Start → Browse, coverage and review → End, Switch and Finish, with NLP stopped (#2060 "Testing Decisions"). | LOC-20, VAL-11, LRN-19 |
| #2063 | Card-specific steps are outside this design: the review of the exact provider payload before any transmission, the real provider sample review, and the personal-export and Anki client tests. #2063 owns their authorization. | — |

The public record of the retained baseline is the #2054 canonical result. No
local-only step adds values to it. The L-0 re-run is a new result under a new
interpretation. It does not replace that record. Its public report follows the
rule above, plus the lineage-audit summary that the classification table makes
public.

### Invariants

Each invariant is covered by the `PUB` rows in [§15](#15-acceptance-test-matrix).

1. **Code ships no retained value.** A retained pin set cannot be resolved from
   reviewed code alone. Every synthetic pin set works with nothing outside the
   repository (PUB-01).
2. **Output is content-free.** No output channel of the classical flow or of the
   operator commands carries local material (PUB-02).
3. **Gates are per operation.** A closed public-release or catalog-publication
   gate never blocks the operator-local route (PUB-03).

### Conflicts and dependencies noted

- **§12 delegation.** §12 "Inputs" leaves to this section which retained
  pin-set fields are private. The table above answers it, and OD-57's public
  commitment is confirmed.
- **Table digest.** §5 and §10 VER-2 put the table digest in reviewed code. §12
  puts the retained table digest in the pin file, and this section classes it
  local (OD-70).
- **Rights gates.** §5 and §9 name #2054 to #2056 as the external rights gates.
  §10 and §11 name them, and §12 names #2054, as the gate that the local import
  checks. #2062 and #2059 say #2054 and #2056 hold public-release questions
  only, and #2055 is now closed into #2063 (OD-69).
- **Report digests.** §9's unreferenced-evidence report prints digests (OD-72).
- **Human channels.** No seam can test what people paste into issues, pull
  requests or agent conversations (OD-75).

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

Some rows name a widened seam. Each widening waits on its open decision:

- `(d) (OD-41)`: an operator command run against a real PostgreSQL database.
- `(a) + CLI`: an operator command under OD-41, combined with seam (a) for what
  the learner sees (§12 "Conflicts and dependencies noted").
- `lint (OD-48)`: `make lint-go` plus the transitive dependency test.
- `CI (OD-65)`: a repository check in CI.

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
| EVD-16 | An unknown interpretation, contract or table version stops import. | (c) | A synthetic candidate whose interpretation names `example-lemma/9`, which is absent from the compiled registry. Variants: an unknown `contract_version`, an unlisted table digest, and `morphology_mapping` `example-feats/9`. | Submit each to shared validation. | Each returns a rejection, `interpretation_version_unknown`, identifying the slot, and no validated interpretation. | The same candidate with `example-lemma/1` validates. Persistence after the rejection is EVD-33. |
| EVD-17 | The same version set resolves to the same interpretation identity. | (b) | The synthetic interpretation set `example-decode/1`, `example-lemma/1`, `example-upos/1`, `example-feats/1`, `example-repairs/1`, `example-terminal/1`, `ancient-greek/1` and table `T`. | Prepare two different Books, for owners A and B, with that set. | Both analyses reference one Interpretation configuration. | Changing any one slot, or the table digest, gives a different configuration. A repair layer of none differs from `example-repairs/1`. |
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
| EVD-34 | The morphology mapping is part of the interpretation identity. | (b) | The EVD-17 set, and two variants: one with `morphology_mapping` `example-feats/2`, one declaring no morphology mapping. | Prepare owner A's Book under each of the three sets. | Three Interpretation configurations with three different `configuration_digest` values, and three Exact analyses. The analysis declaring no morphology mapping reports `morphology` `unsupported`. | Preparing again under the first set reuses its configuration and analysis. A no-mapping set never equals a set naming `example-feats/1`. |
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
| IDN-20 | No lemma carries a discriminator marker. | (c) | Scheme `example` registered with native marker `#ex:<n>`. Synthetic scheme `example-num` with marker `#<digits>`. | Submit `λέξις#ex:1` and `ξέλ#2` to admission as citation forms. | Both are rejected with `discriminator_in_lemma`. | `λέξις` and `ξέλ` are admitted. |
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
| OCC-26 | Morphology is `usable` only through the interpretation's morphology-mapping slot. | (c) | A candidate whose nodes all have native morphology rows that `example-feats/1` maps completely. Variant: the same candidate under an interpretation with no `morphology_mapping`. | Validate each, and read morphology through the read interface. | Under `example-feats/1`, `morphology` is `usable` and each node's `feats` holds the mapped features. Under no mapping, `morphology` is `unsupported`, every `feats` is `{}`, `native_morphology` is unchanged, and the read interface returns no FEATS. | The no-mapping variant with any non-empty `feats`, including a value copied from the native tag, is rejected (`canonical_value_unmapped`). |
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
| RET-01 | Deleting or replacing a blob, revision, binding or interpretation referenced by a snapshot is refused. | (d) | Owner A adopted synthetic analysis X of `example-pkg` for Book G1, Started G1 (rich snapshot S on X), then Ended it. | Separately: delete X's package blob, text revision, annotation revision, binding, Interpretation configuration, exception-table blob and X itself; update each of them; insert a blob with X's blob digest and different bytes. | Every statement fails. Every row and its digest is unchanged, S still resolves to X, and X's sentences still slice the same Extracted text. | The same deletes with no root at all (an unadopted, unrequested synthetic analysis) are also refused by the delete guard. Nothing is ever deleted. |
| RET-02 | Leaf evidence that no foreign key reaches cannot be deleted, and evidence tables cannot be truncated. | (d) | X as in RET-01, with Notice coverage, a Provenance claim, a Diagnostic finding, a Node span and a Usability restriction. | Delete one row of each kind. Truncate each table, with and without `CASCADE`. | Every statement fails. Row counts are unchanged. | Deleting A's own exact decision through `keep` (§8) succeeds, and X is still protected by A's Adopted evidence record. |
| RET-03 | An in-flight request protects its acquisitions and candidate analysis before adoption. | (d) | Owner A has a preparation request whose candidate analysis Y and acquisitions are written. Y is not adopted. | Delete Y, its binding and A's acquisition. | Refused. The request still resolves Y and its acquisitions. | A request that has not yet written any evidence references nothing, and no evidence row exists for it. |
| RET-04 | A deck artifact still resolves its notices after a new analysis is adopted. | (b) | Owner A's Book G1 adopted X. A synthetic pin set declares a member notice for the text and annotation members and a record notice on X's interpretation. A Reading-owned deck artifact was prepared from G1's snapshot on X. | Adopt analysis Y (new interpretation, with its own record notice) for G1, then read the artifact's notices. | The artifact lists exactly X's three notices, with their text. It lists none of Y's. | A custom deck artifact drawn from X behaves the same. An artifact never lists notices through the Book's current analysis. |
| RET-05 | A retained reference keeps its owner's access route after a replacement adoption. | (b) | G1 adopted X. A has an exclusion on `(X,4)`, an ended snapshot on X and a completion on X. | Adopt Y for G1. Then read, as A, the decision's evidence, the snapshot's sentences and the completion's headwords. | All three reads return X's rows and X's Extracted text slices. G1's current analysis is Y. | Reading X's evidence through G1's current analysis returns Y's rows, never X's. |
| RET-06 | A root protects the blob of the build the owner acquired. | (d) | G1 adopted X through acquisition Q of blob P. | Delete Q while A exists. Delete P. | Both refused. | Deleting user A succeeds and removes Q (see RET-11). |
| RET-07 | Protection is written in the same transaction as the referencing record. | (d) | G1 is current on X. An injected fault makes an adoption of Y for G1 fail before commit. Separately, Book G2 has never adopted X. | Run the failing adoption. Then insert a decision for G2 that names `(X,4)`. | After the failure, G1's current analysis is still X, and G1 has no Adopted evidence record for Y. The G2 decision is rejected because G2 has no Adopted evidence record for X. | A successful adoption commits the current row and the Adopted evidence record together. |
| RET-08 | Hiding a Book or leaving To Read keeps the evidence. | (a) | Owner A's G1 has an in-flight request, adopted X, an ended snapshot and a completion. | Through the handlers: hide G1, then move it out of To Read. | Every evidence row, acquisition, Adopted evidence record and root of G1 is unchanged, except that the request is now cancelled. Unhiding G1 shows X's evidence again. | The unreferenced-evidence report lists no row of X's closure. |
| RET-09 | Connection removal and upstream withdrawal keep the evidence. | (a) | Owner A acquired `example-pkg` through a synthetic catalogue connection and adopted X for G1. | Sync without the entry (withdrawal), then remove the connection through the handler. | The offering is withdrawn and then deleted with the connection. G1, its acquisition, X and every root remain. G1 still opens for Reading. | No new acquisition, request or adoption is created. |
| RET-10 | Owner A's cancellation keeps owner B's referenced evidence. | (b) | Owners A and B each acquired `example-pkg`. B adopted the shared analysis X and Started a reading on it. A's request for the same unit is in flight on X. | A cancels the request (leaves To Read). | B's current analysis, snapshot, Adopted evidence record and access to X are unchanged. Every shared row of X's closure exists. | A's cancellation writes nothing in B's scope. B cancelling B's own request later does not change A's rows. |
| RET-11 | Owner deletion removes only that owner's rows, and evidence only that owner used is retained. | (d) | Owners A and B share analysis X. A alone uses analysis Z. Both have roots on their analyses. | Delete user A. | All of A's Books, acquisitions, requests, Adopted evidence records, snapshots, decisions, completions and Rich Known are gone. X, Z and their closures are unchanged. B's rows are unchanged. The report lists Z's closure as unreferenced. | Z is not deleted. |
| RET-12 | A retention reference grants no access. | (a) | Owner A adopted X and has an ended snapshot on it. Owner B has no acquisition of `example-pkg`. | B requests evidence detail, sentence study and the snapshot route using A's Book, snapshot and analysis IDs. | Every response is the existing not-found response, exactly as for an absent ID. | After B acquires the identical bytes, B can read X only through B's own Book. A's Book and snapshot still return not-found to B. |
| RET-13 | A classical Book with adopted evidence cannot be deleted except through its owner. | (d) | A's G1 adopted X and has an ended snapshot and a completion. | Delete G1's `books` row. Delete G1's Reading-unit association. | Both deletes fail, and every row of G1, its roots and X's closure is unchanged. | Deleting user A removes G1 (RET-11). |
| RET-14 | Unreferenced evidence is reported but not deleted. | (d) (OD-41) | Synthetic analysis Z whose only root was a cancelled request. Analysis X is adopted. | Run the unreferenced-evidence report twice. | Both runs list Z's analysis, binding, revisions, content and blob with `held_by` = terminal request, and do not list X. Both runs write nothing. Row counts and digests are unchanged. | No River job, schedule or background task runs the report or deletes anything. |
| RET-15 | The post-restore integrity check detects an altered blob and marks affected Books unavailable, without changing frozen records. | (a) | A dump of an installation where A's To Read Book G1 adopted X, with an ended snapshot, decisions and a completion. It is restored into a test database. One package blob's bytes are then altered at the storage level (the test drops the digest check for that one write). | Run the integrity check. Then, through the handlers, request G1's Book detail and Reading page, Start G1, and confirm a lemma decision on G1. | One `integrity_failure` restriction naming `blob_digest_mismatch` is appended on the blob's Package content. G1 shows "Evidence unavailable" with that reason. Start and the lemma decision are refused with the reason. The snapshot, membership, decisions, completion, Known, Adopted evidence records and current rows are byte-identical to before the check. | A clean restore of the same dump passes every check and appends no restriction. |
| RET-16 | The integrity check detects an unresolved reference. | (d) (OD-41) | A restored database where data was loaded with triggers and constraints disabled, and one Node span and one Extracted text row of X are missing. | Run the integrity check. | The check reports `evidence_reference_missing` for both (IC-7 for the span, IC-6 for the Extracted text), and appends restrictions on X and on X's binding. No frozen record changes. | When a root names an analysis that does not exist, the check reports the restore as failed and appends nothing. |
| RET-17 | A restricted Book is never ended, finished or substituted automatically. | (b) | G1 is the current reading on X. A second, intact build P2 of the same Package content exists, from another acquisition of A. An `integrity_failure` restriction is on that content. | Read the current reading. Then End it. Then, in a second run, Finish it. | Before any action, G1 is still the current reading. Explicit End succeeds. Explicit Finish succeeds from the frozen snapshot. No analysis, acquisition or blob is adopted or read for G1 in place of X, including through P2. | Start of G1 is refused with the restriction reason. |
| VAL-01 | Imported and Stanza candidates both go through one validator, and equal defects give equal diagnostics. | (c) | Two synthetic candidates over one 40-code-point extent: one from a test adapter declaring `example-adapter/1`, and one produced by the Stanza compatibility adapter from a synthetic `analyzer.Result`. Each has one node span `[38,41)`. | Validate each through the shared interface. | Both are Rejected with one `reject` diagnostic, code `span_out_of_bounds`, rule `SPAN-1`, scope `node`, and the same `node_id`. Neither yields a validated analysis. | With the span corrected to `[37,40)`, both are Validated and Vocabulary-ready. |
| VAL-02 | A producer claiming syntax gets `invalid` for a cyclic tree, while vocabulary stays usable. | (c) | A candidate whose producer claim marks `basic_syntax` usable. In every sentence, the basic heads form a cycle (for example node 2 → 3 → 2), and no other rule fails. | Validate, then read basic syntax for each sentence through the read interface. | Validated. `basic_syntax` is `invalid` with `tree_cycle`. `vocabulary` is `usable`, and `VocabularyReady` is true. A `claim_not_supported` notice is returned. The read interface returns no edges. The validated analysis holds exactly the candidate's edges: none added, none dropped. | The same candidate with a complete tree in one sentence gives release-scope `usable` and an `invalid` finding for each other sentence. The claim never makes `basic_syntax` `usable` on its own. |
| VAL-03 | A capability claim is not proof. | (c) | A candidate whose claim marks `morphology` and `secondary_syntax` usable, but which has no morphology rows and no secondary edges. | Validate. | `morphology` is `missing` and `secondary_syntax` is `missing`, each with a `claim_not_supported` notice. `VocabularyReady` is unchanged. | Adding morphology rows with no reviewed FEATS mapping gives `unsupported`, never `usable`. |
| VAL-04 | A candidate that does not cover its declared unit is denied Vocabulary-ready and is never adopted, with a diagnostic (OD-47). | (c) | A declared unit whose Extracted text is 200 code points. The candidate's printed nodes cover only `[0,120)`. Everything else is well formed. | Validate. | Validated, with `vocabulary` `invalid`, code `coverage_gap`, rule `ACC-4`, severity `blocks_vocabulary`, and a count equal to the uncovered non-`White_Space` code points. `VocabularyReady` is false. The analysis is not adoptable. | A candidate covering the full extent is Vocabulary-ready. A coverage gap never shows as a smaller unit or as a ready analysis. |
| VAL-05 | A candidate for another unit, binding or language is rejected. | (c) | Declared binding B1 for unit `book 1`, language `grc`. Candidates naming (i) binding B2, (ii) the right binding with a different Extracted text digest, and (iii) the right binding with language `el`. | Validate each. | Each is Rejected: (i) `scope_mismatch` (`SCOPE-1`), (ii) `extracted_text_mismatch` (`COR-1`), (iii) `language_mismatch` (`SCOPE-3`). | The candidate naming B1, the right digest and `grc` validates. |
| VAL-06 | An unknown version is a permanent failure that writes nothing and is never retried. | (a) | Owner A has a classical Book. A synthetic reviewed definition names `example-lemma/9`, which is absent from the compiled registry. A second definition names adapter `example-adapter/2`, which no registered adapter declares. | Submit a preparation request for each through the web handler, and run the River jobs to completion. | Each request is failed with a permanent class, code `interpretation_version_unknown` or `adapter_version_unsupported` respectively, after exactly one attempt. No Exact analysis, Interpretation configuration, diagnostic or current record exists for either. The Book's state is unchanged. | The same definitions with `example-lemma/1` and `example-adapter/1` prepare successfully. A transient fault (VAL-19) is the only thing that retries. |
| VAL-07 | Diagnostics contain no source text or other input string. | (c) | Synthetic candidates in which every input string (surface, raw lemma, citation form, discriminator value, native references, sentence text, member path, request and Book labels) contains the sentinel `QQSENTINELQQ`. Together they trigger at least one diagnostic of every registered code. | Validate each, and serialize every diagnostic and the log summary. | No serialized diagnostic or log line contains `QQSENTINELQQ`. Every `code`, `rule` and `message` equals a registry entry. Every other field is an enum or an integer. | A rejected slot is named by its slot name only, never by the unknown identifier. |
| VAL-08 | Stanza parity: equal identities, counts and sentences on the characterization fixtures. | (c) | Each characterization fixture (§10 Current anchors) with its exact producer input text. | Run the legacy projection (`selection.Project`, `AnalyzableStatistics`) and the compatibility adapter plus shared validation, then compare P-1 to P-4. | Each fixture is Validated and Vocabulary-ready. Identity sets, per-identity eligible counts, the denominator and per-identity sentence evidence are equal, compared field by field and never as JSON. The legacy values equal the fixtures' existing expectations. | A test copy with one token's canonical lemma changed reports exactly one `identity_only_legacy`, one `identity_only_canonical` and the affected `count_differs`, and leaves the legacy projection's output unchanged. |
| VAL-09 | Parity observation never changes behaviour. | (a) | Two cloned databases with the same EPUB `de` Book and a fake analyzer whose result fails `canonical_key_mismatch` under validation. Observation is on in one and off in the other. In a third run, the observer is forced to panic and then to exceed its time budget. | Run the analysis job to completion in each. | The jobs return the same result. `analysis_runs`, `corpora`, `corpus_tokens`, `corpus_sentences`, `book_current_analyses` and the Browse counts are row-for-row equal across all runs, apart from generated IDs and timestamps. With observation on, one parity log event with `candidate_rejected` and the code is emitted. | The panic and timeout runs publish the analysis exactly as the off run does and record no failure. |
| VAL-10 | Compatibility observation rewrites no stored analysis and switches nothing. | (a) | An installation with a published legacy `de` analysis and an `it` analysis created before observation existed. | Enable observation, analyze a new `de` Book, and read every Book's vocabulary, Concordance and Reading pages. | Every pre-existing legacy row is unchanged. No Exact analysis, §7 row, Capability diagnostic or Book current classical analysis row exists for any EPUB Book. Every EPUB Book still reads through the legacy representation, with OCC-11's characterized output. | Turning observation off and on again changes no row and no page output. |
| VAL-11 | An annotation-backed Book is Reading-ready with NLP stopped. | (a) | The server is composed with an NLP address that refuses connections and a capability provider that errors. Owner A has a classical Book prepared from a synthetic package and definition. | Run preparation and adoption, then request the Reading chooser, Start a reading, open the Reading page, End, Start again and Finish. | Every step succeeds. The Book is Reading-ready and its snapshot is rich. Finish writes a completion and Rich Known. No `analysis_runs` row and no `analyze_corpus` job exists for the Book. The erroring capability provider and the analyzer receive zero requests during the flow. | The same flow with NLP running gives identical results. |
| VAL-12 | A modern-language Book still requires NLP readiness. | (a) | The same NLP-stopped server. Owner A also has an EPUB `de` Book with acquired content and no analysis. | Request its analysis, run the job, then request Start. | The run fails with the existing dependency-parsing message (`ErrDependencyParsingUnavailable`). The Book is not Reading-ready, and Start returns the existing ineligible verdict. | The classical Book from VAL-11 in the same installation stays Reading-ready. |
| VAL-13 | No learner-core or adoption package imports an adapter package. | lint: `make lint-go` (depguard), plus the transitive dependency test in §10 "Package placement" (OD-48) | A scratch file in `internal/selection`, one in `internal/webapp` and one in `internal/adoption`, each importing `internal/interpretation/stanzacompat`. | Run `make lint-go`, and the dependency test that walks `go list -deps -test` for every learner-core and adoption package. | Lint fails on each file with the ADR 0091 amendment message. The dependency test lists the violating package. | The same import from `cmd/server` or `internal/parity` passes. |
| VAL-14 | Adapters cannot reach the database, network, subprocesses or plugins, directly or transitively. | lint: `make lint-go` (depguard), plus the transitive dependency test (OD-48) | Scratch files in `internal/interpretation/oga` and `internal/interpretation/stanzacompat` importing `internal/persistence`, pgx, `net/http`, `os/exec`, `plugin` and `internal/analyzer` in turn. | Run `make lint-go` and the dependency test. | Each direct import fails with the adapter rule's message. The `internal/analyzer` import also fails the dependency test, because it links gRPC and `net`. | Importing `internal/canonical`, `internal/domain` or `internal/canonicalization` passes both. |
| VAL-15 | Validation is deterministic. | (c) | One synthetic candidate with findings in several capabilities and sentences. | Validate it 100 times, in separate goroutines and with different `GOMAXPROCS`. | Every result is equal, including the order of diagnostics and findings. | A candidate whose node order differs is Rejected with `node_order`, never silently re-sorted. |
| VAL-16 | Identity validity is enforced. | (c) | `grc` candidates that differ from a valid one in one respect each: (i) normalization slot `unicode-casefold/1.2.0`; (ii) one identity key `λέξις` where the recomputed key is `λέξισ`; (iii) scheme `example` with a null value; (iv) scheme `unregistered`; (v) citation form `λέξις#ex:1`; (vi) an identity whose UPOS differs from its node's. | Validate each. | Each is Rejected: (i) `normalization_profile_rejected`, (ii) `canonical_key_mismatch`, (iii) `discriminator_half_set`, (iv) `discriminator_scheme_unknown`, (v) `discriminator_in_lemma`, (vi) `identity_upos_mismatch`. | The valid candidate, with `λέξις` NOUN `example:ex:1` and `example:ex:2`, validates with two distinct identities. |
| VAL-17 | Sentence boundaries are checked, and boundary state affects only the sentence-target capability. | (c) | A three-sentence candidate. Variants: (i) the middle sentence is `incomplete`; (ii) the first is `incomplete` and the rest `complete`; (iii) all are `unknown`. | Validate each. | (i) Rejected, `boundary_incomplete_interior` (`BND-1`). (ii) Validated, `sentence_target` `usable` with a `missing` finding on sentence 0. (iii) Validated, `sentence_target` `unsupported`. In (ii) and (iii), `vocabulary` and `VocabularyReady` equal those of the all-`complete` variant. | An adapter never upgrades `unknown` to `complete`: the all-`unknown` variant never yields `usable`. |
| VAL-18 | Exact text correspondence holds for verbatim surfaces. | (c) | A candidate declaring verbatim surfaces, in which one word's surface is `λέξις` while its span slices `λεξις` (no accent) from the Extracted text. A multiword container's components reference exactly the container's span. | Validate. | Rejected, `surface_span_mismatch` (`COR-2`), naming only the word's `node_id`. The components raise no correspondence diagnostic. | With the surface matching the slice, the candidate validates. |
| VAL-19 | Only infrastructure failures are transient, and they retry within budget. | (a) | A preparation request whose persistence is wrapped to fail the first adoption commit with a serialization failure. A second request whose candidate is rejected `coverage_gap`. | Run both River jobs to completion. | The first retries, succeeds once, and leaves exactly one current record and one Exact analysis. The second fails permanently after one attempt, with `coverage_gap`. | A fault that persists past the §11 budget ends with `retry_budget_exhausted` and leaves the Book's previous current analysis unchanged. |
| VAL-20 | Consumers never branch on producer or language. | (b) | The rich scenarios run under `Variant{grc}` and `Variant{xx}`. In each, the same synthetic analysis content is seeded twice: once labelled `example-adapter/1` and once labelled `stanza-compat/1`. | Run Start, counts, lemma review preview and Finish for each Book. | Verdicts, counts, snapshot membership and Known are equal across both adapters and both variants, up to the language field. | Changing the adapter label alone never changes a verdict or a count. |
| VAL-21 | A restriction makes a capability unavailable to consumers at read time. | (b) | A Reading-ready classical Book with no current reading. Then an operator Usability restriction naming `vocabulary` is appended to its analysis's binding. | Request Start, Browse counts and Concordance lemma lookup. | Start returns the ineligible verdict. Browse reports Unavailable, and Concordance lists the Book as unavailable with the restriction's reason. The validated states and diagnostics stored for the analysis are unchanged. | A restriction naming only `basic_syntax` leaves Start, counts and lemma lookup unchanged, and withholds only syntax. |
| VAL-22 | Only a validated, Vocabulary-ready, unrestricted analysis is adopted. | (a) | Owner A's classical Book with current analysis X. A new preparation produces a validated analysis Y with `VocabularyReady` false (`occurrence_unresolved`). | Run preparation through adoption. | The request ends with `occurrence_unresolved`. X remains current, with its counts, decisions and snapshots unchanged. | A Vocabulary-ready Y is adopted. A Y with a `vocabulary` restriction is not. |
| VAL-23 | An adapter is deterministic and invents no defaults. | (c) | A synthetic package for `example-adapter/1`, in which one native sentence has no heads, one native token has no POS, and one native token has a POS with no reviewed mapping. | Run the adapter twice on the same inputs, then validate its candidate. | The two candidates are equal under the canonical serialization. The headless sentence has no basic-syntax edge and gets a `basic_syntax` `missing` finding (`syntax_absent`), not a fabricated root. The token without a POS has `upos_state` `missing`, and the unmapped POS gives `mapping_unsupported`. Neither has UPOS `X`. Every native node in the extent appears in the candidate (ACC-1). | A test adapter that adds a root to the headless sentence is caught by the adapter's own reviewed fixture, not by `Validate` (SPAN-4 and TREE-5 are adapter obligations). |
| ADP-01 | A request and its first job commit together. A classical To Read and its initial request commit together. Parsing runs outside the HTTP request and every transaction. | (a) | Owner A, a classical Book in Inbox, registered from the synthetic `example-def`, with its Local selection (§12). The test's River client is not yet started. A second run injects a failure into the job insert. | POST To Read, then start River. | The response returns before any package byte is read. Book detail shows the Book To Read with a `queued` preparation. One request and one `verify` job exist. After River runs, the Book has current evidence. | With the job insert failing, the POST shows the error, the Book is not To Read, and neither a request nor a job exists. |
| ADP-02 | Duplicate job delivery gives one adoption and does not reset attempts. | (a) | A's initial request in `queued`. The test inserts a second River job with the same arguments for each stage, and delivers each stage's job twice concurrently. | Run River to completion. | Exactly one current evidence for the Book, one Adopted evidence record and one ready count projection exist. Book detail's preparation history shows one successful attempt per stage. The duplicate deliveries wrote nothing. | A duplicate delivered after the request is `adopted` changes nothing, and the history is unchanged. |
| ADP-03 | A restart mid-stage resumes without a budget reset. | (a) | A's request in its `validate` stage, attempt 1, with work in progress. The test kills the worker process during the work step, then starts a new process. | Advance the injected clock past the attempt's lease, and run the reconciler and River. | The history shows `validate` attempt 1 as interrupted and attempt 2 as succeeded, and one attempt for each earlier stage. The request is adopted. | A clean shutdown during work records the interruption at once and schedules attempt 2 after backoff. The interrupted attempt is still counted. |
| ADP-04 | The third transient failure gives `failed` with Retry offered. Retry creates a new request with the same pin. | (a) | A's request whose `interpret` stage fails transiently with `storage_unavailable` every time (persistence wrapper). | Run River to completion, then POST Retry from Book detail with the wrapper removed, and run River. | The first request is `failed` with `retry_budget_exhausted`, and its history shows three `interpret` attempts. Book detail offers Retry. The new request is shown as a retry of the first, with the same kind and the same pinned choice, and a fresh history. It is adopted. | No fourth `interpret` attempt of the first request ever runs. A Retry POST for a request that is not the Book's latest is refused and creates nothing. |
| ADP-05 | The deadline counts from creation, including queue and backoff time, and stops the request. | (a) | A's request created at T, held in `queued` by another active pipeline. The injected clock moves to T + 24 h + 1 s. | Let the snoozed job run. | The request is `failed` with `deadline_exceeded`, with no new attempt. Book detail offers Retry. Pending and active counts exclude it. | At T + 23 h 59 m, a transient failure whose backoff would end after T + 24 h fails at once with `deadline_exceeded`, and schedules no job. |
| ADP-06 | A permanent hash mismatch is `blocked` immediately, with no retries. | (a) | A's request whose local package source has one member byte changed from its pin. | Run River to completion. | The request is `blocked` with `package_inventory_mismatch` after exactly one `verify` attempt. No evidence row and no acquisition was written. Book detail explains it, and offers Retry once exact bytes are supplied. | A transient failure in the same stage would retry, which shows that the permanent class is what stopped it. |
| ADP-07 | A 33rd pending request is refused, with an explained state, and the refusal changes no other request. | (a) | 32 open requests across owners A, B and C, on 32 Books, including A's running replacement B1 on Book G. One more request whose deadline has passed. | Owner D POSTs To Read on a classical Book. Then A approves a different replacement for G. | D's Book is To Read. D's request is `failed` with `installation_queue_full`, and no job exists for it. Book detail explains it and offers Retry. A's Refresh is admitted, because it supersedes B1 and the pending count stays 32. | The expired request is not counted. With 31 open, D's request is `queued`. If A's Refresh were for a Book with no open request at 32 pending, it would be refused, and no other request would change. |
| ADP-08 | A second active pipeline waits, including while the first is between stages. | (a) | Owners A and B each have a `queued` request. A test hook pauses A after each stage's Finish, before its next stage begins. | Run River with two worker processes, then release A stage by stage. | While A is in any stage or between stages, B's job snoozes and B's history shows no attempt. Book detail shows B waiting. At no observed instant are two requests `running`. After A adopts, B runs and adopts. | B's first stage history starts at attempt 1. |
| ADP-09 | Choosing C while B runs supersedes B, and B cannot adopt. | (a) | A's Book with current analysis X. Replacement B is in `prepare_projections`, held by a test hook. | A approves replacement C. Then release B's hook, and run River. | B is `superseded`, naming C. B left no staged counts. C is adopted and is current. X keeps its Adopted evidence record. | Delivering B's adopt job directly after the supersession is refused, and the Book's current analysis is never B's candidate. |
| ADP-10 | Leaving To Read cancels intent and keeps evidence. | (a) | A's initial request in `interpret`, with acquisitions and package rows already written. | A moves the Book to Inbox, then River runs the remaining jobs. | The request is `cancelled` with `left_to_read`, and no later stage output exists. The blob, Package content, acquisitions and every evidence row are still present. The Book has no current evidence. | A Finish that returns a Book to Inbox cancels that Book's open request in the same way. Hiding the Book cancels nothing. |
| ADP-11 | Owner A's cancel does not affect owner B. | (a) | A and B each have an open request for the same package and unit, on their own Books. A's `verify` wrote the shared rows. | A leaves To Read. Run River. | A's request is `cancelled`. B's request reuses the shared rows, reaches `adopted`, and B's Book has current evidence. No row of B's changed during A's cancellation. | A request by A for A's Book can never change state because of B's disposition change. |
| ADP-12 | A transaction failure at the adopt step keeps the old evidence current, with no partial rows. | (a) | A's Book with current analysis X and ready counts. Replacement Y's adoption fails through an injected fault at several points inside the adoption, including the commit itself, one run per point. | Run the adopt job. | X is current. Book detail, Reading and Browse show X's evidence and counts. No Adopted evidence record for Y, no count row for Y and no readiness change exist. The attempt is transient, and the next attempt adopts Y. | A fault that persists through three attempts ends with `retry_budget_exhausted`, and X stays current. |
| ADP-13 | Stale expected evidence leads to `superseded`. | (a) | Replacement request R for A's Book, expecting current analysis X, held before its adopt step. A test hook then commits an adoption of analysis Z for the Book that does not pass through admission, so R is still the latest `running` request. | Release R and run its adopt job. | R is `superseded` with `expected_state_changed`. Z is current, and no Adopted evidence record, count row or readiness change for R's candidate exists. | Without the hook, R adopts. |
| ADP-14 | A replacement without approval is refused. | (a) | A's Book with current analysis X, and a newer candidate offered. | POST Refresh without the approval confirmation, then POST Retry on a failed replacement without the confirmation. | Both responses refuse and explain. No request and no job is created. | The same POSTs with the confirmation each create a `queued` replacement. |
| ADP-15 | An initial request adopts its pinned choice automatically. | (a) | A's classical Book in Inbox, with no current evidence. | POST To Read and run River. | The request is `adopted` with no approval step. The Book is Reading-ready, and Start is offered. | An initial request whose candidate is not Vocabulary-ready is `blocked`, and the Book stays without current evidence. |
| ADP-16 | At most one open request per (owner, Book). | (a) | A's Book with current analysis X and no open request. | Send two concurrent Refresh approvals for candidates C1 and C2. | Exactly one request is open: the one admitted second. The one admitted first is `superseded`, naming the second. | A To Read POST while a request is open creates nothing. |
| ADP-17 | Repeated To Read never re-pins. | (a) | A's initial request I1 for candidate P is `cancelled`, after A left To Read. The Book's current Local selection (§12) now names a newer candidate Q. | A POSTs To Read again, then POSTs To Read on the already-To-Read Book. | One new `initial` request I2 exists, with every pinned-choice field equal to I1's (P). The second To Read creates nothing. Q is offered only through Refresh. | A To Read on a Book that has current evidence creates no request. |
| ADP-18 | A deterministically unusable pin is `blocked` with no Retry offered. | (a) | A's request whose validated candidate is not Vocabulary-ready (`occurrence_unresolved`). | Run River to completion. | The request is `blocked` with `occurrence_unresolved` after one `validate` attempt. The analysis is stored and inspectable, and no Book current classical analysis names it. Book detail offers no Retry. | A `vocabulary` restriction on an otherwise ready candidate gives `blocked` with `evidence_restricted`. |
| ADP-19 | The capacity gate is race-free. | (a) | 31 open requests. | Two owners POST To Read on two classical Books concurrently, 50 times on fresh databases. | In every run, exactly one new request is `queued` and the other is `failed` with `installation_queue_full`. Pending never exceeds 32. | With 30 open requests, both are `queued` in every run. |
| ADP-20 | Adoption writes current evidence, ready counts and retention references in one commit. | (a) | A's request at the adopt step. A test observer reads the Book's current evidence, counts and Adopted evidence records in one `REPEATABLE READ` transaction per sample, in a loop on a separate connection. | Run the adopt job. | Each sample shows either no current evidence and no counts, or the new analysis with `ready` counts and its Adopted evidence record, never a mix. After adoption, Browse counts equal the counts computed from the analysis. | With the adoption rolled back by a fault, every sample shows the earlier state. |
| ADP-21 | Adoption changes no frozen state. | (a) | A's classical Book G1 is the current reading, with an active rich snapshot whose library totals used G2's counts. A's classical Book G2 has an ended rich snapshot, a completion and Rich Known from an earlier reading on analysis X, and decisions on X. A deck preparation exists for G1. | Approve and run a replacement that adopts Y for G2. | G2's current analysis is Y, with ready counts. Both snapshots, their membership and contributions, reservations, Known, history, the deck preparation and the decisions on X are byte-identical before and after. | A later lemma decision preview on G2 made before the adoption is stale, and writes nothing. |
| ADP-22 | Re-entering To Read does not revive a cancelled replacement. | (a) | A's Book with current analysis X. Replacement R is `cancelled` after A left To Read. | A POSTs To Read again, and River runs. | No request is created, R stays `cancelled`, and X is current. | A Refresh approval for R's candidate creates a new request. R itself stays `cancelled`. |
| ADP-23 | An owner never sees or affects another owner's requests. | (a) | Owner A has an open request on A's Book. | B GETs A's Book detail and POSTs Retry and To Read with A's Book ID and request ID. | Each returns the existing not-found response. A's request is unchanged. | The same POSTs by A succeed or are refused only by A's own state. |
| ADP-24 | A Book that became the current reading is not switched under it. | (a) | A's Book with analysis X. Replacement R is held before adopt. A Starts a Reading on the Book. | Release R. | R is `failed` with `book_is_current_reading`. X is current, and the Reading is unchanged. Book detail offers Retry after End or Finish. | After End, Retry admits a new request that adopts. |
| ADP-25 | Expected evidence catches re-adoption of an earlier analysis. | (a) | A's Book: X adopted by R1, then Y by R2, then X again by R3. A request R4 that expected (X, R1) is still `running` (test-injected, bypassing admission's supersession). | Run R4's adopt job. | R4 is `superseded` with `expected_state_changed`, although the current analysis is X again. | A request created after R3, expecting (X, R3), adopts. |
| ADP-26 | Retry delays follow the binding backoff. | (a) | A's request whose `verify` fails transiently twice, then succeeds. The injected clock and random source are recorded. | Run River. | The second attempt is scheduled 5–10 s after the first failure, and the third 10–20 s after the second. No delay exceeds 5 minutes. Waiting for the active pipeline snoozes 10–20 s and increments no counter. | An upstream retry instruction longer than the remaining deadline gives `failed` with `deadline_exceeded`, with no scheduled job. |
| ADP-27 | The database backs the request invariants. | (d) | A's classical Book with an open request, and a second Book with a `running` request. | Insert directly: a second open request for the first Book; a `replacement` without approval; a second `running` request; a fourth attempt for one stage; a change to a pinned-choice field; a transition out of a terminal state. | Each statement is rejected. | Valid inserts and the listed transitions succeed. |
| ADP-28 | A job left over from an earlier attempt never skips backoff or consumes budget. | (a) | A's request whose `verify` attempt 1 failed transiently, so it is `queued` with attempt 2 scheduled after its backoff. The test re-delivers attempt 1's job at once. | Run River without advancing the clock, then advance it past the backoff. | The re-delivered job writes nothing. Attempt 2 runs only after the backoff. The history shows exactly two `verify` attempts. | A job naming attempt 3 delivered before attempt 2 also writes nothing. |
| ADP-29 | A lost job is recovered without consuming budget. | (a) | A's request between stages, `running`, whose next stage's River job the test deletes. | Run the reconciler, then River. | The reconciler inserts the missing job. The next stage runs as attempt 1, and the request is adopted. | A request whose deadline has passed is set `failed` with `deadline_exceeded` by the reconciler, and no job is inserted. |
| ADP-30 | One owner's Refresh never switches another owner's Book on the same unit (ADR 0097 "Stress cases"). | (a) | Owners A and B each have a Book on the same shared unit. A's current analysis is X, from `example-adapter/1`. B's is Z, from `example-adapter/2`, with ready counts. | A approves a Refresh to analysis Y, and River runs. | A's current analysis is Y. B's current analysis, counts and Adopted evidence records are unchanged. B's Book detail shows nothing of A's request or acquisition. | A Refresh POST by A that names B's Book returns the existing not-found response and creates no request. |
| LOC-01 | Admission refuses path traversal, and the refused package writes nothing. | (a) + CLI | Owner A (username `owner-a`). Synthetic test-only pin set `example-hostile/1` pins the transport digest of a synthetic ZIP with one entry named `../escape.xml`. Row counts of every §4, §5 and §12 record, of Books and of Preparation requests are recorded. | Run `localregistration -owner owner-a -pin-set example-hostile/1 -interpretation example-interpretation/1 -package <zip>`. | Exit 5 with `package_path_unsafe`, `rule=ADM-6`, `entry=1`. Every recorded row count is unchanged. No file named `escape.xml` exists outside the test's input directory. | The same with an absolute path (`/escape.xml`), a backslash path and a non-NFC path each gives `package_path_unsafe`. A safe path in the same position passes ADM-6. |
| LOC-02 | Admission refuses links. | (a) + CLI | As LOC-01, with a pinned synthetic ZIP whose entry has Unix mode `S_IFLNK` and target `/etc/passwd`. | Run the command. | Exit 5 with `package_entry_link`, `rule=ADM-6`. No rows written. The link target is never opened. | An entry carrying a link or owner extra field outside the allowlist gives `package_entry_unsafe`. A directory entry gives `package_entry_unsafe`. |
| LOC-03 | Admission refuses duplicate members. | (a) + CLI | As LOC-01, with a pinned synthetic ZIP holding two entries with the same path. | Run the command. | Exit 5 with `package_member_duplicate`, `rule=ADM-5`, `entry=2`. No rows written. | Two paths equal only after NFC normalization also give `package_member_duplicate`. |
| LOC-04 | Admission refuses unlisted members and missing members. | (a) + CLI | As LOC-01, with a pinned synthetic ZIP holding every pinned member plus `extra.xml`, and a synthetic manifest member that lists `extra.xml`. | Run the command. | Exit 5 with `package_member_unlisted`, `rule=ADM-5`. No rows written. The manifest's listing has no effect. | A ZIP that lacks one pinned member gives `package_member_missing`. |
| LOC-05 | The expansion bound holds against lying headers. | (a) + CLI | As LOC-01, with a pinned synthetic ZIP whose one entry declares 1 KiB compressed and uncompressed in both headers but deflates to 1 GiB. The test wraps the decompressor to count output bytes. | Run the command. | Exit 5 with `package_expanded_too_large`, `rule=ADM-8`. At most 64 MiB plus one read buffer was decompressed. No buffer was sized from a header. No rows written. | An honest synthetic package expanding to exactly 67,108,864 bytes passes ADM-8. |
| LOC-06 | Safe XML never resolves entities or fetches anything. | (a) + CLI | Synthetic pin set `example-xxe/1` pins a synthetic ZIP whose annotation member declares `<!DOCTYPE a [<!ENTITY x SYSTEM "file://<sentinel file>">]>` and an entity pointing at a local test listener, and references both. The sentinel file and listener record any access. | Run the command. | Exit 5 with `xml_dtd_forbidden`, `rule=ADM-10`. The sentinel file is not opened and the listener receives no connection. No rows written. | A billion-laughs payload, and an internal subset that declares no entity, each give `xml_dtd_forbidden`. A document type declaration with only a `SYSTEM` identifier naming the listener passes ADM-10, and the listener still receives no connection. An undeclared reference `&x;` with no DTD gives `xml_entity_reference`. A member in UTF-16 gives `xml_encoding_unsupported`. Element depth 257 gives `xml_depth_exceeded`. |
| LOC-07 | At most 128 members. | (a) + CLI | As LOC-01, with a pinned synthetic ZIP of 129 small entries, all pinned. | Run the command. | Exit 5 with `package_too_many_members`, `rule=ADM-4`. No rows written. | The same package with 128 entries passes ADM-4 and registers. |
| LOC-08 | At most 16 MiB, detected without reading the whole file. | (a) + CLI | A synthetic file of 16,777,217 bytes. The test counts bytes read from it. | Run the command. | Exit 5 with `package_too_large`, `rule=ADM-1`. At most 16,777,217 bytes were read. No rows written. | A pinned synthetic package of exactly 16,777,216 bytes passes ADM-1. |
| LOC-09 | Malformed central directories are refused. | (a) + CLI | Pinned synthetic ZIPs: one with bytes prepended before the first local header; one with trailing bytes after the end record; one whose local and central names differ; one with a ZIP64 record. | Run the command for each. | Each exits 5 with `package_archive_malformed`, `rule=ADM-3`. No rows written. | The unmodified valid package passes ADM-3. |
| LOC-10 | Executable payloads are refused. | (a) + CLI | Pinned synthetic ZIPs: one whose `text` member begins with the ELF signature; one with an entry whose Unix mode has an execute bit; one with an entry whose extension is not allowed for its role; one with a nested ZIP member. | Run the command for each. | Each exits 5 with `package_member_executable`, `rule=ADM-7`. No rows written. | The valid synthetic package, whose members are XML and plain text with mode 0644, passes ADM-7. |
| LOC-11 | Encrypted entries and unsupported compression are refused. | (a) + CLI | Pinned synthetic ZIPs: one with an encrypted entry, one using a compression method other than stored or deflate. | Run the command for each. | Each exits 5 with `package_entry_unsafe`, `rule=ADM-6`. No rows written. | Stored and deflate entries both pass. |
| LOC-12 | The transport pin is checked before any parsing. | (a) + CLI | A synthetic malformed ZIP whose transport digest no pin set lists. | Run the command with `example-def/1`. | Exit 5 with `package_transport_unpinned`, `rule=ADM-2`, not `package_archive_malformed`. No rows written. | The same bytes, pinned in `example-hostile/1`, reach ADM-3 and give `package_archive_malformed`. |
| LOC-13 | Exact member and content pins are enforced, and pins never come from the package. | (a) + CLI | Synthetic pin sets whose transport digest matches but which pin one member with a different size, one with a different digest, and one with a different content digest. A fourth package's manifest member claims different digests. | Run the command for each. | Exits 5 with `package_member_size_mismatch`, `package_member_digest_mismatch` and `package_content_digest_mismatch` (`rule=ADM-9`). The fourth is decided by the pins only. No rows written for the failures. | A repacked build of the same members, listed as a second transport digest in `example-def/2`, registers and resolves to the same Package content as the first build. |
| LOC-14 | A valid package registers once, and a repeat writes nothing. | (a) + CLI | Owner A, no classical Book. Valid synthetic package for `example-def/1` (unit `example-work / ed-1 / book 1`) and synthetic exception table. | Run the command twice with the same flags. | First run: exit 0, `outcome=created`. A has one Inbox Book with metadata provenance `operator_local`, one association, one Local registration, one acquisition and one Local selection. One blob of each kind exists. No Preparation request or River job exists. Second run: exit 0, `outcome=unchanged`, the same Book ID, and every row count and row is unchanged. My Books for A lists the Book with its Edition, unit and provenance line. | A Book that A moved to To Read between the runs keeps its disposition and request after the second run. |
| LOC-15 | A second owner gets a separate Book with shared bytes. | (a) + CLI | Owner A registered as in LOC-14. Owner B exists. | Run the command for `owner-b` with the same files. | Exit 0, `outcome=created`. B has a new Book, association, Local registration, acquisition and Local selection. The blob, Package content, members and revisions have one row each. A's rows are unchanged. | B's GET of A's Book anchor and B's To Read POST with A's Book ID return the existing not-found response. A's My Books does not list B's Book. |
| LOC-16 | A missing file is an explained failure that writes nothing. | (a) + CLI | Owner A. Row counts recorded as in LOC-01. | Run the command with `-package` naming a path that does not exist. Repeat with a directory, and with an unreadable file. | Exit 3 with `local_file_missing`, then `local_file_unreadable` twice. stderr names `-package`, never the path. Every row count is unchanged. | The same for `-pin-file` and `-interpretation-table` names those flags. |
| LOC-17 | Unknown versions and closed gates are refused before the package is read. | (a) + CLI | Owner A. `-package` names a path that does not exist. A test rights-gate state can be closed. | Run the command with `-pin-set example-def/9`; with `-interpretation example-interpretation/9`; with an interpretation that `example-def/1` does not list; with a retired pin set; with the gate closed. | Exits 4 with `pin_set_unknown`, `interpretation_version_unknown`, `interpretation_not_pinned`, `pin_set_retired` and `rights_gate_closed`. None reports `local_file_missing`, which shows the package was not opened. No rows written. | The same flags with a compiled, listed, unretired version and an open gate proceed to P-6. |
| LOC-18 | A pin file is accepted only when its digest equals the compiled commitment. | (a) + CLI | Synthetic test pin set `example-local/1` with local fields and a compiled commitment. A synthetic pin file with that digest. | Run the command with the pin file changed by one byte; with a pin file that repeats a compiled field; with no `-pin-file`. | Exit 4 `pin_file_mismatch`; exit 4 `pin_file_invalid`; exit 2 `usage_invalid`. No rows written. | The exact pin file registers, and its Installed pin set row has the compiled digest. A later run with the same file writes no second row. |
| LOC-19 | The exception table must match its pinned digest. | (a) + CLI | Owner A. Valid package for `example-def/1`. | Run the command without `-interpretation-table`; with a table changed by one byte; with a table of 16 MiB + 1 bytes. | Exit 2 `usage_invalid`; exit 4 `interpretation_table_mismatch` twice. No rows written. | The exact table registers and is stored once as an `interpretation_table` blob. |
| LOC-20 | The full flow works with NLP stopped and no outbound network. | (a) + CLI | NLP service not running; `MOUSEION_NLP_ADDR` points at a test listener that records connections. The command and the server run with a dialer guard that fails the test on any connection other than PostgreSQL. Owner A. | Register `example-def/1` for A; A POSTs To Read with the informed-choice confirmation; run River to completion; A Starts the Book from the Reading chooser. | The request is `adopted`. The Book is Reading-ready, appears among Reading's To Read candidates, and the Start creates a Reading. The NLP listener and the dialer guard record no connection. | Before Start, no Reading exists: adoption alone never starts one. An EPUB Book's To Read in the same run still reports that analysis needs the language service. |
| LOC-21 | Output, logs and job metadata contain no content. | (a) + CLI | Synthetic package, pin file and table whose every member, path, lemma, table value and digest field contains a unique sentinel string. Server and command output captured. | Run LOC-14, LOC-20 and each failure in LOC-01 to LOC-13 and LOC-16 to LOC-19. | No sentinel appears in command stdout or stderr, server logs, River job arguments or metadata, or Preparation request `reason` and `diagnostics`. | A test-only diagnostic that embeds a member path fails this row, which shows the check is effective. |
| LOC-22 | Queue capacity is explained at To Read, and the registration is unaffected. | (a) + CLI | 32 pending requests from other owners. Owner A registered as in LOC-14. | A POSTs To Read with the confirmation. | A's Book is To Read. Its request is `failed` with `installation_queue_full`, with no job. The row shows "Not started: the installation's preparation queue is full" with Retry. A's registration rows are unchanged. | After one other request finishes, Retry admits a `queued` request with the same pinned choice. |
| LOC-23 | A failure falls back to nothing. | (a) + CLI | Owner A with a registered Book on `example-def/1` and current evidence. `example-def/2` is compiled. A test hook fails the registration transaction at commit. | Run the command with a hostile package for `example-def/2`; then with the valid `example-def/2` package and the commit hook set. | First: exit 5, and A has no acquisition or Local selection for any pin-set version other than `example-def/1`. Second: exit 1 `storage_unavailable`, and no row from the run exists. A's current evidence, selection and requests are unchanged after both. | Re-running the second command without the hook exits 0 with `outcome=added`. |
| LOC-24 | Re-registering during an open request adds no request. | (a) + CLI | Owner A's Book in To Read with a `running` request, held before `adopt`. | Run the command again with the same flags, then POST To Read again. | `outcome=unchanged`. The Book has exactly one open request, with an unchanged pinned choice and attempt counters. | Releasing the request adopts it normally. |
| LOC-25 | A new Local selection never changes running work or current evidence. | (a) + CLI | Owner A's Book with current evidence from `example-def/1` and `example-interpretation/1`. `example-interpretation/2` is listed by `example-def/2`. | Register `example-def/2` with `example-interpretation/2`. | `outcome=added`. A new acquisition and Local selection exist. The current analysis and every request are unchanged. The row offers the new selection as a Refresh candidate. | Approving the Refresh confirmation admits a `replacement` request that adopts the new analysis. Registering the old selection again appends a selection row and changes nothing else. |
| LOC-26 | Registration leaves learner state alone, and To Read needs the informed choice. | (a) + CLI | Owner A with a current Reading on another Book. The synthetic provenance claims are `automatic`. | Register `example-def/1` for A. POST To Read without the confirmation field, then with it. | After registration, the new Book is Inbox and A's Reading, dispositions, Known and history are unchanged. The first POST changes nothing and returns the row with the confirmation open. The second moves the Book to To Read and admits an `initial` request with `approved_at` set. | With all release-scope claims `human`, To Read needs no confirmation. |
| LOC-27 | The server never reads the package file. | (a) + CLI | Owner A registered as in LOC-14. The test deletes every input file after registration. | A POSTs To Read with the confirmation, and River runs. | The request is `adopted`. | Changing one byte of the stored Installed pin set row (test-only, with its update trigger disabled) makes the `verify` stage `blocked` with `pin_file_mismatch`, and nothing is adopted. |
| LOC-28 | Concurrent registrations converge. | (d) | Owner A, no classical Book. | Two registration transactions for the same owner, pin set and package run concurrently. | Both commit. A has one Book, one Local registration, one acquisition and one Local selection. | Two concurrent runs for owners A and B give two Books and one blob. |
| LOC-29 | An unknown owner writes nothing, and verify-only writes nothing. | (a) + CLI | Row counts recorded as in LOC-01. | Run the command with `-owner nobody`. Then run it with `-verify-only` and no `MOUSEION_DATABASE_URL`. | Exit 6 `owner_not_found`, with every row count unchanged. Verify-only exits 0 with `outcome=verified` and opens no database connection. | Verify-only on a hostile package gives the same exit 5 code as a full run. |
| LOC-30 | The command and admission package cannot reach NLP or the network. | lint (OD-48) | The module at the change under test. | `make lint-go` and the transitive dependency test. | `cmd/localregistration` does not depend on `internal/analyzer`, gRPC, `net/http` or a provider package. `internal/evidencepackage` and the safe XML reader depend on no `net`, `os`, `os/exec`, `plugin`, persistence or River package. | Adding such an import to either fails the test. |
| LOC-31 | The database backs the §12 records. | (d) | Owner A with a Local registration on `example-def`, and an acquisition on another definition. | Insert directly: a Local selection naming the other definition's acquisition; an update to a Local selection; an update to an Installed pin set; an Installed pin set whose digest does not match its content. | Each statement is rejected. | Valid inserts succeed. |
| MIG-01 | Applying S1 to S9 to a populated modern-language database leaves every existing row unchanged. | (d) | A database at the last pre-rollout migration, populated through existing application interfaces for owners A and B in `de`, `it` and `el`. It holds EPUB Books with published analyses and Browse counts, Lemma corrections, Occurrence exclusions and Lemma review flags, a current Reading with its snapshot, a finished Reading with history and Known, a released snapshot, prepared decks and custom decks. A digest of every row of every pre-existing table, ordered by primary key, is recorded. | Apply S1 to S9 through the server's migration path. | Every digest equals the recorded one. `schema_migrations` is at S9's version and clean. Every new table has zero rows. | After migration, a `de` snapshot insert with a NULL `corpus_id` is rejected by the representation check, and nothing is written. |
| MIG-02 | Every characterization outcome is unchanged after rollout. | (b) | The PostgreSQL adapter's template includes S1 to S9. The fixture adapter is unchanged. | Run the existing `de` scenarios and the OCC-11 legacy read characterization. | Every scenario passes with its existing expectations: the same verdicts, frozen vocabulary, counts, Known sets and history. | Seeding Rich Known and a classical Book for the same owner changes no legacy `de` selection outcome (OD-33). |
| MIG-03 | Each step applies alone on top of the steps it depends on, and is all-or-nothing. | (d) | For each step Sk: a database populated as in MIG-01, migrated to the pre-rollout version plus S1 to S(k−1). | Apply Sk alone, as one migration step. | Sk succeeds, and the version is Sk's and clean. Every Sk object exists, and no object of a later step does. MIG-01's digests are unchanged. | Running Sk's `up` file against a database that lacks one of Sk's dependencies fails, and afterwards no Sk object exists. |
| MIG-04 | A failed step is reported as half-applied, stops server start, and leaves no partial object. | (d) | A database at S6 with MIG-01's data. A second session holds a lock on `primary_goal_snapshots` that conflicts with S7. | Run the server's migration path. | S7 fails within its 5-second lock wait. `schema_migrations` is at S7's version and dirty. No S7 object exists, and the server does not start. The preflight reports `migration_dirty` and S7 `absent`, with result `blocked`. After the lock is released and the version is reset to S6 (REC-5), the migration path applies S7. | On a cleanly migrated database, the preflight reports every step `complete`. A database with S9's objects but without S8's validation reports `rollout_partial`. |
| MIG-05 | The representation check and S7's foreign keys hold from S7's commit, and S8 validates them. | (d) | A database at S7 with MIG-01's data. | Insert a snapshot with all five legacy columns and an `exact_analysis_id`. Then apply S8. | The insert is rejected. After S8, every constraint that S7 added to `primary_goal_snapshots` is validated, and no row has changed. | A legacy snapshot insert and its release succeed between S7 and S8, and again after S8. |
| MIG-06 | The changes to existing tables leave the legacy write paths unchanged. | (a) | Server at S9. Owner A has an EPUB `de` Book. | A moves the Book to To Read. The analysis job publishes. A starts, ends, starts again and finishes the Reading. | Publication writes the Book's current analysis. Start writes a legacy snapshot with no `exact_analysis_id`, and its tuple vocabulary. Finish writes history and Known. Every response matches the existing handler tests. | A tuple vocabulary row for a rich snapshot, and a current EPUB analysis for a classical Book, are both rejected (EVD-27, LRN-16). |
| MIG-07 | The preflight reports seeded legacy `grc` rows per owner, in counts and IDs only, and blocks rollout. | (d) (OD-41) | MIG-01's database, plus rows for owners A and B in every G-1 to G-10 table whose language is `grc`, `GRC`, `grc-x-test` or `grc_Grek`. Every seeded lemma, title and form contains a marker string. | Run the preflight. | The result is `blocked`, with exit code 2. Each G check reports exactly the seeded counts and IDs, per owner and table. The output contains no marker string, lemma, title, text or form. Every row digest is unchanged. | Owner C has no `grc` row, yet `cmd/localregistration` for C exits 4 with `legacy_grc_preflight_blocked`, reads no package file and writes nothing. The stop covers the whole installation. |
| MIG-08 | The preflight passes on a clean database. | (d) (OD-41) | MIG-01's database at S9, plus `el` rows with the invented lemma `λέξις`, and a Book whose title contains `grc`. | Run the preflight. | The result is `clear`, with exit code 0. Every count is 0, and every A and R check passes. | Adding one `grc` Known row for owner B makes the result `blocked`, listing exactly that row's ID. |
| MIG-09 | The preflight checks the uniqueness and owner-isolation assumptions that the new constraints rely on. | (d) (OD-41) | MIG-01's database. Through direct SQL, one current reading points at a snapshot in another language, and one snapshot vocabulary row's language differs from its snapshot's. | Run the preflight. | The result is `blocked`. A-3 reports `snapshot_language_mismatch` with those goal and snapshot IDs. Every G count is 0. | In a scratch database with `books_owner_id_id_key` dropped, A-1 reports `referenced_key_missing` and names that constraint. |
| MIG-10 | Legacy `grc` rows are never converted. | (d) (OD-41) | MIG-07's seeded database at the pre-rollout version. | Apply S1 to S9, run the preflight twice, then run `cmd/localregistration` for owner A. | Every seeded row is byte-identical. No Vocabulary identity, Rich Known, membership, decision, count or Book–Reading unit association exists for any of them. Both preflight reports are identical. | Registration is refused, and every new table still has zero rows. |
| MIG-11 | A restore of blobs plus records passes the integrity check. | (d) (OD-41) | At S9, owners A and B each adopt a synthetic classical Book through §11. A adopts twice, so an older analysis keeps its Adopted evidence record. A also has an exact decision, a rich snapshot, a completion and Rich Known. | Dump the whole database with `pg_dump -Fc` inside the database container. Restore it into a fresh database with `pg_restore --single-transaction --exit-on-error`. Run the integrity check, then the preflight. | The restore succeeds. The integrity check passes with no restriction, and R-1 to R-3 pass. Every table digest equals the source's. Both Books report current evidence available, and A's older analysis is still readable as A's evidence. | A dump that excludes the evidence tables' data fails `pg_restore` as a whole, and the fresh database stays empty. |
| MIG-12 | A restore that is missing a blob fails the integrity check and shows the unavailable state. | (d) (OD-41) | MIG-11's dump, loaded into a fresh database with triggers disabled (`session_replication_role = replica`) and without the Package blob of A's current build. §9 calls this a non-conforming restore. | Run the integrity check. Then read A's Book evidence and A's Reading chooser through the store. | The check fails with `evidence_reference_missing` and appends an `integrity_failure` restriction (OD-64). A's Book shows Evidence unavailable with the reason, and Start is refused. Frozen records are byte-identical, and no Reading ends automatically. | B's Book, whose evidence does not depend on that blob, stays available. A conforming restore of the same reduced data fails as a whole. |
| MIG-13 | Regenerated sqlc output matches the committed code at every step. | CI: `make sqlc` plus `git diff --exit-code` (OD-65) | Each step's PR. | Run `make sqlc`, then `git diff --exit-code gen/sqlc`. | There is no diff, and the build and tests pass. | A step file changed without regeneration fails the check. |
| MIG-14 | Rows that the classical path writes are not legacy rows. | (d) (OD-41) | A clean database at S9. Owner A registers a synthetic classical Book with `cmd/localregistration`, adopts it, sets their study language to `grc`, then starts and finishes a rich Reading. | Run the preflight. | The result is `clear`. Every G count is 0. INF-1 reports the `grc` study language and display-name row, and the result does not change. | A legacy `grc` Known row added for owner A makes the result `blocked`, listing only that row. |
| MIG-15 | Shipped migrations are never edited, renumbered or removed (STEP-1). | CI (OD-65) | Each step's PR. | Compare every migration file on `main` at the PR's merge base, including `000001_initialize`, with the PR's tree. | No shipped file differs, and none is missing or renamed. | A PR that changes one byte of `000001_initialize.up.sql`, or renumbers a merged step, fails the check. |
| PUB-01 | Code ships no retained value, and public CI needs nothing outside the repository. | (a) + CLI | A public CI checkout at S9, with no file outside the repository and no Installed pin set row. The compiled registry holds the retained pin-set entry, the synthetic `example-def/1`, and the synthetic `example-local/1`, which has local fields as in LOC-18. | Run the command naming the retained pin set and interpretation, with a synthetic package and no `-pin-file` or `-interpretation-table`. Run it again with a synthetic pin file and table. Then register `example-def/1` for owner A and run LOC-14's flow to adoption. | The first run exits 2 `usage_invalid` and the second exits 4 `pin_file_mismatch`. Neither writes a row. `example-def/1` registers and is adopted with no file outside the repository. | After `example-local/1` is registered and its Installed pin set row is removed (test-only, with its guards disabled), its Book's `verify` stage is `blocked` with `pin_file_missing`. A compiled entry with local fields never resolves from code alone. |
| PUB-02 | No output channel carries local material. | (a) + CLI | LOC-21's sentinel package, pin file and table, registered and adopted for owner A. | Run LOC-21's flows, including its failures, the legacy-`grc` preflight (§13), the evidence integrity check and the unreferenced-evidence report (§9). Capture all stdout and stderr, server log lines, River job arguments and metadata, and request `reason` and `diagnostics`. | No sentinel appears in any captured output. | A test-only diagnostic or report line that embeds a member path or a lemma fails this row, which shows the check is effective. |
| PUB-03 | Gates are per operation: a closed public-release or catalog-publication gate never blocks the operator-local route. | (a) + CLI | A test rights-gate state with the operator-local import gate open and every other operation's gate closed. Owner A. | Register `example-def/1`, POST To Read with the informed-choice confirmation, and run River. | Registration exits 0 and the request is `adopted`. No response, output or diagnostic names a public-release or catalog gate. | With the operator-local import gate closed, the command exits 4 `rights_gate_closed` before it reads the package (LOC-17), and the `verify` stage of an already registered Book is `blocked` with `rights_gate_closed`. |

## 16. Requirement traceability

Traced at commit `00a63be6b38f9b54d359b6e68ed014af30ab7b25` (`main`), against
[#2062](https://github.com/justin-hayes/mouseion/issues/2062), ADR 0097
"Stress cases" and the handoff's Stage 2 **Stop** line.

Matrix IDs are §15 rows. A requirement with no row is a **gap**. Each gap is
recorded in §17, or closed by a row that follows directly from a stated rule.
"L-n" names a §14 local-only step.

### (a) User stories

| # | story (#2062) | sections | matrix IDs |
| --- | --- | --- | --- |
| 1 | Ancient Greek in my existing application | §2, §4, §8, §10 | BIB-22, LOC-14, LRN-19, VAL-11, VAL-20 |
| 2 | My Book is an explicit Edition and Reading unit, and annotation availability cannot change it | §4, §5 "Bytes present versus unit offered" | BIB-06, BIB-09, BIB-10, BIB-13, EVD-12, LOC-14 |
| 3 | Usable citation headwords and distinct lexical homographs | §6, §7, §8 | IDN-17, IDN-28, IDN-29, IDN-30, IDN-37, OCC-01, LRN-01 |
| 4 | Exact occurrence decisions when spans coincide | §7 "Occurrence identity", §8 "Effective identity" | OCC-01, OCC-12, OCC-13, LRN-02, LRN-22 |
| 5 | Unsupported optional syntax stays explicitly unavailable | §7 "Optional layers and availability", §10 "Capability derivation and readiness" | OCC-07, OCC-08, OCC-18, OCC-21, OCC-26, VAL-02, VAL-03, EVD-20 |
| 6 | Existing Known, reservations, snapshots and history preserved | §6 "Legacy compatibility", §8, §11 "Frozen state", §13 | IDN-24, IDN-25, IDN-26, IDN-27, LRN-13, LRN-14, ADP-21, MIG-01, MIG-02, MIG-10 |
| 7 | Source refresh separated from adoption | §11 "Initial and replacement", §12 "Local selection" | ADP-14, ADP-15, ADP-17, ADP-22, ADP-30, LOC-25, EVD-18 |
| 8 | Annotation-backed Reading works with NLP stopped | §10 "NLP independence", §12 | VAL-11, VAL-12, LOC-20, LOC-30 |
| 9 | Private source material stays local | §14, §10 "Diagnostic shape", §12 "Logging and privacy" | PUB-01, PUB-02, LOC-20, LOC-21, VAL-07. Human channels: gap, OD-75. |
| 10 | Immutable evidence and notices retained with protected references; exact backup and recovery | §5, §9, §13 "Recovery" | EVD-01, EVD-08, EVD-10, EVD-30, RET-01 to RET-07, RET-15, MIG-11 |
| 11 | A preflight for legacy Greek records | §13 "Legacy-`grc` preflight", §6 "Legacy compatibility" | MIG-07, MIG-08, MIG-09, MIG-10, MIG-14, IDN-27 |
| 12 | Unchanged modern-language outcomes | §6, §7 "Legacy coexistence", §10 "Stanza compatibility adapter", §13 | BIB-20, EVD-28, IDN-35, IDN-36, OCC-11, OCC-24, LRN-14, VAL-09, VAL-10, MIG-01, MIG-02, MIG-06 |
| 13 | Clear module interfaces and transaction invariants | §10, §11 | VAL-01, VAL-13, VAL-14, VAL-22, ADP-12, ADP-20 |
| 14 | An explicit design-approval checkpoint | §1, §18 | None. A review rule: gap, OD-75. |
| 15 | Local operation distinguished from public release | §12, §14 | PUB-01, PUB-03, LOC-17, BIB-22 |

### (b) Implementation and Testing Decisions

| bullet (#2062) | summary | sections | matrix IDs |
| --- | --- | --- | --- |
| Impl 1 | Design only. Follow ADRs 0093–0098 and #2058. Do not reopen source selection. | §1, §2 | None. A review rule: gap, OD-75. |
| Impl 2 | Records, uniqueness and owner authorization for Work, Edition, Reading unit, Book, evidence, bindings, interpretations, diagnostics and retention. Entry-to-unit offering without a public catalog. | §4, §5, §9 | BIB-03, BIB-11, BIB-12, BIB-22, EVD-13, EVD-17, EVD-19, EVD-20, EVD-25, RET-01, RET-12. Offering rows BIB-14, BIB-15, BIB-17 to BIB-19, BIB-21, BIB-25, BIB-29 and BIB-30 wait for Stage 3 (OD-66). |
| Impl 3 | Rich identity with an optional scheme-qualified discriminator. Decoding separate from normalization. No discriminator in lemma spelling. | §6 | IDN-17 to IDN-21, VAL-16 |
| Impl 4 | Exact-analysis occurrence identity, explicit spans, unprinted nodes and containers. Offsets never identify or transfer. | §7 | OCC-01 to OCC-05, OCC-12, OCC-13, OCC-14 |
| Impl 5 | Additive membership on existing anchors. Legacy tuples are not wildcards. No second Greek state machine, no backfill. | §6, §8, §13 "Data-only work" | LRN-01, LRN-16, LRN-19, IDN-24, IDN-26, MIG-01 |
| Impl 6 | Adapter → validation → preparation and adoption → learner core, with versions, diagnostics, failures and ownership. Model candidates through the same validator. No rewrite, no global switch. | §10 | VAL-01, VAL-06, VAL-07, VAL-08, VAL-09, VAL-10, VAL-13, VAL-19, VAL-22 |
| Impl 7 | Bounded immutable bytes in PostgreSQL. Packages are data, never plugins. The research fixture is not the importer. | §5, §10 "Source interpretation adapter", §12 | EVD-01, EVD-04, EVD-05, EVD-26, LOC-10, VAL-14, LOC-30. The research fixture is compared only in L-2. |
| Impl 8 | Atomic adoption against expected owner, Book, evidence and request state, with counts and protected references. Freezes never change, and decisions never transfer. | §8, §9, §11 | ADP-09, ADP-12, ADP-13, ADP-20, ADP-21, ADP-25, RET-07, LRN-03, LRN-13 |
| Impl 9 | Structural migration sequence, legacy preflight, concurrency rules, and tested restore or roll-forward. No numbers, no baseline edit, separate backfills. | §13 | MIG-01 to MIG-15, ADP-19, BIB-02 |
| Impl 10 | Operator-local entry point without hosting, downloads or NLP, with safe failure. | §12 | LOC-14 to LOC-20, LOC-23, LOC-29 |
| Impl 11 | Preserve the frozen minimum: unchanged source, the frozen table, non-vocabulary conjunctions with unresolved CCONJ/SCONJ, and required headwords and discriminators. Preserve provenance. Morpheus is corroboration, not a runtime dependency. | §5, §6, §7, §10, §12 | OCC-10, IDN-17, IDN-28, LOC-19, EVD-24, VAL-14, LOC-30. The real values are checked only in L-0 and L-2 to L-4. Under #2089, "the frozen table" is the table that §5's exception-table changes and the L-0 audit leave, and the POS and morphology mappings are the independent `oga-agdt-upos/4` and `oga-agdt-feats/1` (§5). |
| Impl 12 | Public and local material. Code release and data publication are separate. Unknown public-release applicability does not block. | §14 | PUB-01, PUB-02, PUB-03, LOC-21, VAL-07. Human channels: gap, OD-75. |
| Impl 13 | Completion needs a reviewed design, a matrix and recorded human approval. SQL is not authorized before then. | §1, §15, §18 | None. A review rule: gap, OD-75. |
| Test 1 | Observable behaviour, not table names. Each invariant is exercised and rejected. | §15 | See [Consistency audit](#consistency-audit): every invariant has a row, and no row asserts a proposed table name. |
| Test 2 | Primary seam (a). Reuse the store-contract scenarios for lifecycle, Known, deck admission and occurrence review. | §8 "Store-contract extension", §10 "Store-contract harness", §15 | LRN-01, LRN-02, LRN-19, LRN-22. Rich deck admission: gap, OD-76. PostgreSQL seeding of rich scenarios: OD-68. |
| Test 3 | Seam (c) for parity, full-scope rejection, identities, overlapping spans, normalization and unavailable optional evidence. | §6, §7, §10 | VAL-08, VAL-04, VAL-05, VAL-16, OCC-01, OCC-02, IDN-01 to IDN-12, VAL-02, VAL-03 |
| Test 4 | Duplicate delivery, late finishers, stale intent, transaction failure, two-owner isolation and exact restoration, with modern characterization retained. | §9, §11, §13 | Duplicates ADP-02, ADP-28; late finishers ADP-09; stale intent ADP-13, ADP-25, LRN-18; transaction failure ADP-12, RET-07; two owners ADP-11, ADP-23, ADP-30, EVD-25, LRN-12; restoration EVD-30, MIG-11, RET-15; characterization OCC-11, IDN-36, LRN-14, MIG-02 |
| Test 5 | Public CI is synthetic. The real Book 1 fixture is a local oracle. No private payload in logs or artifacts. | §14 | PUB-01, PUB-02, LOC-21, VAL-07; L-1 to L-8 |
| Test 6 | No migrations, implementation, provider calls or fixture execution are needed to complete this design. | — | None. A review rule: gap, OD-75. |

### (c) Stress cases and Stop conditions

Each row names the matrix IDs that prove the case is handled as accepted, or
that the stop condition does not happen.

| case | accepted result, or the condition that must not happen | matrix IDs |
| --- | --- | --- |
| SC-1. Alice and Bob adopt different producers | Independent current evidence. Alice's refresh cannot switch Bob or reveal his restricted acquisition. | ADP-30, ADP-11, ADP-23, EVD-25, LRN-12, RET-12 |
| SC-2. A replacement fails validation or publication | Old valid evidence and To Read remain. No mixed revisions or correction transfer. | VAL-22, ADP-12, ADP-18, ADP-20, LRN-03 |
| SC-3. Upstream withdraws exact text | The retained copy stays usable. Discovery reports the withdrawal separately. | RET-09, BIB-19. Both use the Stage 3 catalogue path; the local path has no upstream. |
| SC-4. The exact local copy is lost too | Restore verified exact evidence or show unavailable. Never choose another Edition or revision. | RET-15, RET-16, RET-17, MIG-12, EVD-30 |
| SC-5. B finishes after the owner chooses C or cancels | B cannot adopt. No cancellation of another owner's request. | ADP-09, ADP-10, ADP-11, RET-10 |
| SC-6. Published evidence has a confirmed defect | Restrict the affected usability and explain it. Keep provenance and frozen learner state. | EVD-22, EVD-23, VAL-21, RET-17 |
| SC-7. Storage pressure or connection removal | Stop new work as needed. Protected references and evidence are never evicted. | Connection removal RET-09, BIB-19; no eviction RET-01, RET-02, RET-14; queue pressure ADP-07, LOC-22. Storage pressure: gap, OD-74. |
| ST-1. Silent homograph collapse | must not happen | IDN-17, IDN-23, LRN-01, OCC-01, VAL-16 |
| ST-2. Offset-only decision transfer | must not happen | OCC-01, OCC-12, OCC-13, LRN-02, LRN-03 |
| ST-3. Changed legacy counts or identities | must not happen | BIB-20, EVD-28, IDN-35, IDN-36, OCC-11, OCC-24, LRN-14, VAL-09, VAL-10, MIG-01, MIG-02, MIG-06 |
| ST-4. Destructive baseline edits | must not happen | MIG-01, MIG-03, MIG-15 |
| ST-5. A design that needs producer branches throughout Reading, Known and snapshots | must not happen | VAL-20, VAL-13, LRN-19 |

### Gaps and added rows

Rows added by #2075, each following directly from a rule already stated:

- **ADP-30** (SC-1): §11 scopes every request and adoption by (owner, Book), and
  §5's access rule hides another owner's acquisitions. No row had A refresh while
  B holds a different analysis of the same unit.
- **MIG-15** (ST-4): §13 STEP-1 forbids editing a shipped migration. No row
  asserted it.
- **PUB-01 to PUB-03**: §14's invariants.

Rows added by #2089 (no treebank data), for the morphology-mapping slot:

- **EVD-34**: §5 adds `morphology_mapping` to the configuration identity.
- **OCC-26**: §7 makes morphology `usable` only through that slot.

The exception-table lineage audit, the morphology-tag confirmation and the re-run of the baseline need retained material, so they are the local-only step L-0
(§14), not rows. The εἰμί rule is OD-77.

Gaps recorded in §17: OD-68 (rich store-contract seeding), OD-74 (storage
pressure), OD-75 (review-only requirements) and OD-76 (rich deck admission).
SC-3 and the catalogue-offering rows have rows, but only the Stage 3 spec runs
them. That is noted on OD-66.

### Downstream inheritance

**Rule.** A row is listed under the first spec that delivers every interface
its setup and action use. Every later spec keeps all earlier rows passing.
Rows marked † also observe a Browse, Reading or Start surface that #2060
delivers: the listed spec makes their persisted assertions pass, and #2060
completes the observation. Rows marked ‡ need a deck preparation on a rich
snapshot in their setup, which #2063 delivers (OD-76).

**Not in any of the four specs.** These rows need §4's Unit offering and Claimed
alias, which ship with the Stage 3 discovery spec (OD-66): BIB-14, BIB-15,
BIB-17, BIB-18, BIB-19, BIB-21, BIB-25, BIB-29, BIB-30, RET-09, and the
offering and Claimed-alias parts of BIB-27.

#### #2061 canonical foundations

| item | inheritance |
| --- | --- |
| Sections | §4 records, "Ensure Book for Reading unit" and Edition reconciliation (not Unit offering or Claimed alias, and not Local registration, which ships in S9). §5 records, access rule and database ceilings. §6 in full. §7 in full, including the analysis read interface and its legacy adapter. §8 records, effective identity and the count-projection builder. §9 mechanism, guards and Adopted evidence record storage. §10's interfaces: shared validation, registries, diagnostics, the read shapes, the eligibility function, the Stanza compatibility adapter and parity observation, the store-contract harness, and the layering rules. #2059 adds the `oga` adapter and `internal/adoption`, and #2060 moves the learner-core consumers onto these interfaces. §13 S1 to S8, the legacy-`grc` preflight command, and the recovery rules. |
| Matrix IDs | BIB-02, BIB-06, BIB-07, BIB-08, BIB-09, BIB-11, BIB-12, BIB-13, BIB-16, BIB-20, BIB-24, BIB-27 (except its offering and Claimed-alias parts), BIB-28; EVD-01, EVD-09, EVD-10, EVD-13, EVD-15, EVD-16, EVD-19, EVD-21, EVD-26 to EVD-30, EVD-32; IDN-01 to IDN-13, IDN-15, IDN-17 to IDN-21, IDN-33, IDN-35, IDN-36; OCC-01 to OCC-05, OCC-08 to OCC-20, OCC-22, OCC-23, OCC-26; LRN-16, LRN-17, LRN-24; RET-01, RET-02, RET-06, RET-13; VAL-01 to VAL-05, VAL-07 to VAL-10, VAL-13 to VAL-18, VAL-23; MIG-01 to MIG-06, MIG-08, MIG-09, MIG-13, MIG-15. #2061 runs MIG-01 to MIG-06 at S8; #2059 runs them again at S9. |
| Local-only steps | None (§14). |
| Blocking decisions | Every §17 row whose **blocks** column names #2061: OD-02, OD-03, OD-04, OD-06, OD-07, OD-12, OD-13, OD-14, OD-16, OD-17, OD-20, OD-21, OD-22, OD-23, OD-25, OD-26, OD-28, OD-29, OD-30, OD-32, OD-34, OD-35, OD-37, OD-41, OD-43, OD-44, OD-45, OD-47, OD-48, OD-63, OD-65, OD-68, OD-70. Most are under [§17 "Blocks #2061"](#blocks-2061-canonical-foundations). |

#### #2059 local import and adoption

| item | inheritance |
| --- | --- |
| Sections | §4 Local registration. §5 pins, application limits and acquisition. §9 evidence integrity check, unreferenced-evidence report and restore procedure. §10 the `oga` adapter and `internal/adoption`. §11 in full, including the OD-05 gate classification. §12 in full. §13 S9 and STOP-1. §14 L-1 to L-7. |
| Matrix IDs | BIB-01, BIB-03, BIB-10, BIB-22, BIB-23, BIB-26; EVD-02 to EVD-08, EVD-11, EVD-12†, EVD-14, EVD-17, EVD-20, EVD-23, EVD-24, EVD-25, EVD-31, EVD-33, EVD-34; IDN-29; RET-03, RET-07, RET-11, RET-14, RET-16; VAL-06, VAL-19, VAL-22; ADP-01 to ADP-11, ADP-12†, ADP-13, ADP-14, ADP-15†, ADP-16 to ADP-19, ADP-20†, ADP-22, ADP-23, ADP-25 to ADP-30; LOC-01 to LOC-19, LOC-20†, LOC-21 to LOC-31; MIG-07, MIG-10; PUB-01, PUB-02, PUB-03. EVD-03, EVD-04, EVD-05, EVD-07, EVD-33, VAL-06 and ADP-06 need restating first (OD-71). |
| Local-only steps | L-1 to L-7 (§14). L-2 compares against the result of §14's L-0, which runs in handoff Stage 1, so L-0 runs first. |
| Blocking decisions | Every §17 row whose **blocks** column names #2059: OD-05, OD-08, OD-12, OD-13, OD-15, OD-16, OD-19, OD-38, OD-39, OD-41, OD-43, OD-47, OD-49, OD-50, OD-51, OD-52, OD-54, OD-56 to OD-61, OD-63, OD-64, OD-69 to OD-74, OD-77. Most are under [§17 "Blocks #2059"](#blocks-2059-local-import-and-adoption). |

#### #2060 Reading and vocabulary

| item | inheritance |
| --- | --- |
| Sections | §4 display of Edition and unit. §6 headword display, lemma review on classical analyses (I-5) and Concordance (I-6). §7 consumers through the analysis read interface. §8 lifecycle transitions, representation selection, the 3/15 rule, contributing Books and the rich store-contract scenarios. §9 deletion behaviour and the restricted-Book behaviour. §10 "Shared learner core": the consumers that read through #2061's interfaces, and the NLP independence of the Reading entry points. |
| Matrix IDs | BIB-04, BIB-05; EVD-18, EVD-22; IDN-14, IDN-16, IDN-22 to IDN-28, IDN-30, IDN-31, IDN-32, IDN-34, IDN-37, IDN-38; OCC-06, OCC-07, OCC-21, OCC-24, OCC-25; LRN-01 to LRN-14, LRN-15‡, LRN-18 to LRN-23, LRN-25, LRN-26; RET-05, RET-08, RET-10, RET-12, RET-15, RET-17; VAL-11, VAL-12, VAL-20, VAL-21; ADP-21‡, ADP-24; MIG-11, MIG-12, MIG-14; and the † observations of EVD-12, ADP-12, ADP-15, ADP-20 and LOC-20. |
| Local-only steps | L-8 (§14) |
| Blocking decisions | Every §17 row whose **blocks** column names #2060: OD-01, OD-03, OD-04, OD-05, OD-22, OD-23, OD-26, OD-33, OD-34, OD-35, OD-42. Those that block no earlier spec are under [§17 "Blocks #2060"](#blocks-2060-reading-and-vocabulary). |

#### #2063 cards

| item | inheritance |
| --- | --- |
| Sections | #2063 mostly consumes §8 and §9: frozen Rich snapshot membership and headwords, Rich Known, and §9's R6 rules (one Adopted evidence record reference per (Book, analysis) used, and notices resolved only through them). It also reads §7's `sentence_target` capability, sentence boundaries and highlighting rule, and §6's headword and discriminator display. **Card-specific rules are outside this design:** rich deck records (B-14, B-15), sentence and target selection, the length caps, Glosses and translations, provider payloads, note and APKG packaging, client acceptance and the personal-export assessment. |
| Matrix IDs | RET-04, plus LRN-15‡ and ADP-21‡ with real deck records. #2063 adds its own card rows (OD-76). |
| Local-only steps | Card-specific, and outside this design (§14). |
| Blocking decisions | OD-14 and OD-76. OD-76 is under [§17 "Blocks #2063"](#blocks-2063-cards). |

#### Spec statements updated for #2089

The #2059 and #2063 bodies were restated for #2089 on 2026-10-15: the reduced
exception table, `oga-agdt-upos/4` and `oga-agdt-feats/1` as the only new
mappings, and comparison with the Stage 1 re-run instead of 438/434.

### Consistency audit

| check | result | action |
| --- | --- | --- |
| Field and record names agree across §4–§13 | Two mechanical mismatches. §6's Rich Lemma correction named `confirmed_at`, which §8's Exact occurrence decision calls `updated_at`. ADP-01 and ADP-17 used "offering" for the in-scope local path. | Fixed in §6, ADP-01 and ADP-17. |
| Every FK target exists | One missing key. §4's Claimed alias references `(owner_id, offering_id)`, but Unit offering declared no `(owner_id, id)` uniqueness. §4's Unit offering → "the owner's catalogue connection" names a record that OD-09 leaves to Stage 3. Every other composite reference, including those into `books`, `primary_goal_snapshots` and `reading_history`, has a matching key at this commit. | Added `(owner_id, id)` to Unit offering. This is mechanical: `id` is already the key, so the composite adds no restriction and only makes the declared reference valid, as for Work and Edition. The connection target stays with OD-09 and OD-66. |
| Current-state facts | §3 B-12 and its correction said `reading_history` is keyed `(owner_id, language, book_id)`. Migration 000010 replaced that key with `completion_id` (`migrations/000010_reading_history_append_only.up.sql:7-11`). §8 and §13 already use the current key. | Fixed in §3. |
| Every invariant has a matrix row | Every numbered invariant in §4–§14 cites at least one existing row, and every cited ID exists. §4's "Implementation rules" are review rules exercised by the BIB rows above them, as §4 says. | — |
| Every row's seam is (a)–(d) | **Not clean until OD-41, OD-48 and OD-65 are decided.** Every row uses (a)–(d) except the widened ones: the `(a) + CLI` rows, the `(d) (OD-41)` rows, the `lint (OD-48)` rows and the `CI (OD-65)` rows, each listed by its label in §15. RET-14 and RET-16 ran operator commands under a plain `(d)` label. | Relabelled RET-14 and RET-16 `(d) (OD-41)`, as §9 already says. Listed the widenings under §15 "Seams". OD-41's question now names the `(a) + CLI` rows, and OD-65's names MIG-15. |
| No table-name assertion in "expected" | No row's expected column, setup, action or negative case names a proposed table. OCC-24, VAL-09, VAL-11 and MIG-05 name existing shipped tables, and MIG-01 and MIG-04 name `schema_migrations`, as the unchanged characterization baseline. The ground rules forbid proposed names only. | — |
| No private values | Every Greek form is an invented example. No 64-hex digest, frozen table value, Book 1 text or private count appears. The only hashes are commit SHAs. | — |
| Rows that contradict later sections | EVD-03, EVD-04, EVD-05, EVD-07, EVD-33, VAL-06 and ADP-06 expect admission failures through a handler and request state `failed`. §12 refuses them in the command, and §11 records stored-byte failures as `blocked`. | Raised as OD-71. |
| Cross-section gaps | §12 asks §9 to guard the Installed pin set and to list the Local selection as a referencer, with no open decision. §5, §9, §10, §11 and §12 cite #2054 to #2056 (and the closed #2055) as rights gates for local operations. §5 and §10 compile the table digest, while §12 pins it locally. §9's report prints digests. OD-19's lift record is specified nowhere. | Raised as OD-73, OD-69, OD-70 and OD-72. The lift is noted on OD-19. |
| One interpretation per pin set | §5 "Pins" names one Interpretation configuration per pin set. §12 lets a pin set list several and names one with `-interpretation`. With one listed interpretation they agree, as §12 says. | No decision needed. |

## 17. Open decisions

Any ticket may append rows. Owner tickets resolve their rows. Settled
decisions (see §1) are not reopened here.

Rows are grouped by the first downstream spec they block. The **blocks**
column lists every spec a row blocks. §18 records an answer to every row before
any spec starts (§1). IDs are never reused: OD-24 and OD-53 are merged into
OD-63 and OD-39, and keep their IDs so that references still resolve. OD-68 to
OD-76 come from the §16 traceability and consistency audit. OD-77 comes from
the #2089 rework (no treebank data).

### Blocks #2061 canonical foundations

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-02 | Where is the generic casefold fallback rejected for `grc` (B-18, B-19)? | (1) The registry refuses `grc` unless a dedicated profile is registered, for both `For` and `Lookup`. (2) Only the shared validation interface rejects casefold-normalized `grc` identities. | (1), decided by #2067. §6 is written against it: `grc` is a dedicated-profile language, and every other unregistered language keeps the fallback. | (1) fails closed for every caller, including lemma review, but changes registry behaviour for an unregistered language. (2) leaves other callers able to normalize `grc` with casefold. | #2064 | #2061 | open |
| OD-03 | How does Reading eligibility treat an occurrence whose optional syntax is unavailable, given that `OccurrenceEligible` reads the dependency (B-10)? | (1) Unavailable syntax never makes an occurrence ineligible; syntax-based exclusions apply only when syntax is present. (2) Language-specific eligibility rules come from the canonical interpretation. | (1). §7 is written against it: a syntax-based exclusion applies only where the occurrence's basic syntax is usable. #2071 (§10) owns the eligibility interface that applies it. | (1) keeps one rule but needs explicit "unavailable" semantics. (2) moves eligibility into producer data. | #2064 | #2061, #2060 | open |
| OD-04 | How do store-contract scenarios cover Greek when the harness is fixed to `de` and seeds only tuples (B-30)? | (1) Parameterize the scenario language and extend the seeds with discriminators, span references and optional syntax. (2) Add a separate Greek scenario set beside the existing one. | (1) for the harness, decided by #2071 (§10 "Store-contract harness"). The harness takes a language variant and gains a rich seeder that seeds only validated synthetic candidates. The rich scenarios are §8's separate list, run for `grc` and a synthetic casefold language `xx`. The existing `de` scenarios and their tuple seeds are unchanged. | (1) runs every existing scenario against rich identities, but changes the harness interface for both adapters. (2) duplicates scenarios and lets them drift. The chosen form changes the harness once for both adapters, and does not re-run tuple scenarios under a rich variant, where they would test the tuple path twice. Rich coverage is §8's named list. | #2064 | #2061, #2060 | open |
| OD-06 | Are Work, Edition and Reading unit records global or owner-scoped (§4)? | (1) All shared. (2) All owner-scoped. (3) Shared when created from a reviewed identity definition; private to one owner when derived only from that owner's unverified catalogue advertisement. | (3). §4 is written against it. | (1) puts one learner's connection-supplied descriptions in rows other owners can resolve to. Sync then needs a write ban that the schema cannot express. (2) gives the simplest authorization (composite owner keys everywhere), but every owner duplicates descriptions, and §5 evidence cannot cite a shared unit. (3) keeps reviewed identity and its evidence shareable, and lets no learner input reach another owner. Its cost is a nullable `owner_id` and a trigger checking that associated units are shared or same-owner. It narrows issue item 7's "shared descriptions" for advertised rows. | #2065 | #2061 | open |
| OD-07 | Does a Reading unit need its own stable external citation scheme field? | (1) The unit stores an internal reviewed `citation_scheme` name with its bounds. External citation identifiers, such as a CTS passage URN, are verified aliases only. (2) Add a stable external citation identifier column to the unit. (3) No scheme field; bounds are opaque text. | (1). | (1) makes `extent_key` comparable within a scheme without trusting external identifiers. (2) makes an external alias a de facto key, against ADR 0093. (3) cannot tell `1` in one scheme from `1` in another, so uniqueness on (Edition, extent) is unsafe. | #2065 | #2061 | open |
| OD-12 | How does an Exact analysis relate to `analysis_runs` and `book_current_analyses` (§5)? | (1) Extend additively: make `book_current_analyses.source_material_id` and `analysis_run_id` nullable, and add a nullable classical-analysis column with a check that exactly one side is set. (2) A parallel owner-scoped "Book current classical analysis" record keyed `(owner_id, book_id)` and referencing the §4 association, plus an additive trigger on `book_current_analyses` that rejects classical Books. | (2). §5 is written against it. | (1) keeps one physical anchor, but it relaxes `NOT NULL` columns and foreign keys on a shipped table (`migrations/000001_initialize.up.sql:239-245`, `:2613`, `:2629`). It also changes the meaning of `current_analysis_identity` (`:433-444`), which code and queries reference in 61 places, and of the upsert at `internal/analysis/jobs.go:1736-1739`. (2) leaves every EPUB structure, view and query unchanged. Its cost is that the single domain classification must read two anchors, which is OD-05's gate question, and a cross-table exclusion trigger. | #2066 | #2061, #2059 | open |
| OD-13 | Which identifier fills the source-decoding slot for the retained candidate? The handoff names lemma, POS, repair, terminal and normalization identifiers, but no decoding identifier. | (1) Source decoding is a separate slot, and the Stage 1 reviewed definition names its own `name/version` identifier. (2) Treat the decoding as part of `oga-greta-lemma/2` and fill the slot with that identifier. | (1). | (1) keeps decoding revisions visibly separate from mapping and normalization, as the #2033 resolution requires, but needs one more reviewed public identifier at Stage 1. (2) adds no identifier, but a decoding-only change then cannot be versioned without also versioning the lemma mapping. | #2066 | #2061, #2059 | open |
| OD-14 | Is the selection policy (`grc-earliest-capped/1`) part of the analysis-affecting interpretation identity? Issue #2066 item 6 lists it there. The handoff says a card-selection policy change applies to a newly prepared specification, not to an analysis rewrite. | (1) Pin it in the same reviewed definition, but keep it out of the Interpretation configuration. It is recorded with the specifications it governs (§8). (2) Include it as a slot, so a policy change creates a new interpretation and analysis. | (1). §5 is written against it. | (1) matches the handoff, and a policy change needs no re-adoption or new counts. (2) forces a new analysis, adoption and count projection for a change that alters no occurrence. That contradicts the handoff. | #2066 | #2061, #2063 | open |
| OD-16 | Are the package bounds (16 MiB, 128 members, 64 MiB) configurable above the handoff values? | (1) The application enforces configured values that default to the handoff values and can only be lowered. The database checks the handoff values as ceilings, and raising one needs a reviewed change. (2) Fully configurable at runtime, with no database ceiling. (3) Fixed constants. | (1). | (1) follows "Deliberately review any required limit change" (handoff Stage 1) and keeps a backstop against application bugs. (2) lets an operator setting silently admit larger packages, with no backstop. (3) contradicts "configurable engineering defaults". | #2066 | #2061, #2059 | open |
| OD-17 | Can a Text–annotation binding reference a private (advertised-only) Reading unit? | (1) No. Bindings reference shared units only, and the Stage 3 spec decides private-unit evidence. (2) Yes. A binding inherits the unit's `owner_id`, and private bindings and analyses cascade on owner deletion. | (1). | (1) keeps every evidence row shared and immutable, and is enough for the local path, which uses reviewed definitions and so shared units (§4 OD-06). It defers evidence for unverified advertised units. (2) supports that case now, but adds owner-scoped cascades to otherwise shared immutable evidence. | #2066 | #2061 | open |
| OD-20 | Is a Usability restriction installation-wide, or per owner? | (1) Installation-wide: a restriction on shared evidence applies to every owner whose Book uses it, and each affected owner is notified. (2) Per owner: the operator records it for chosen owners. | (1). §5 is written against it. | (1) follows ADR 0097: "an integrity failure or confirmed binding/interpretation defect records an explained usability restriction … notify affected owners". A defect in shared bytes is a property of the bytes, not of one owner's choice. It changes no owner's adoption, which ADR 0097 "Alice's refresh cannot switch Bob" protects. (2) lets known-defective evidence stay usable for owners who were not named. | #2066 | #2061 | open |
| OD-21 | Is the rich vocabulary identity an interned record with a surrogate ID, or a composite value carried in each consumer (§6)? | (1) One shared, immutable, interned Vocabulary identity table, unique `NULLS NOT DISTINCT` on the five values. Consumers carry its `uuid`. (2) The same, but owner-scoped: one interned row per owner. (3) A composite value: each consumer carries language, lemma, UPOS, scheme and value. | (1). §6 is written against it. | (1) gives each consumer one non-null 16-byte reference and single-column indexes. The nullable comparison happens once, in one unique index. Its costs are a join to display a lemma and an insert-if-absent step. A row interned by one learner's Lemma correction holds only a normalized lemma and UPOS, has no owner link, and stays after that owner is deleted. (2) removes that residue, but shared Exact analysis rows (§5) would then need a second, shared identity reference. (3) needs no join, but widens every consumer key and index by roughly 40–70 bytes. Every join must then compare nullable columns, where `NULL = NULL` silently drops matches. That is the merge or wildcard defect this design forbids. | #2067 | #2061 | open |
| OD-22 | Where does the citation headword live, and what is shown when analyses disagree (§6)? | (1) The decoded citation form per occurrence (§7), plus one Analysis headword per (Exact analysis, identity), chosen deterministically. Frozen records freeze the headword they showed. (2) One headword per (identity, Interpretation configuration). (3) A headword column on the identity row. | (1). | (1) keeps every Book-scoped view exact to its own analysis, and keeps frozen records stable. It needs a cross-Book display rule, which §6 gives. (2) is smaller, but two Books under one interpretation whose texts spell the citation form differently would share one headword. (3) makes the first analysis's spelling permanent for every Book and owner, so a later analysis could never show its own. | #2067 | #2061, #2060 | open |
| OD-23 | Who may set a discriminator, or change UPOS, in a Lemma correction (§6)? Issue #2067 item 7 says corrections "may change lemma or UPOS". Today they change the lemma only (`internal/selection/selection.go:264-274`). | (1) Learners never set, change or clear a discriminator, and never change UPOS. A correction to a different key gives a null discriminator. Re-entering the same key is `keep`. Only a reviewed interpretation sets discriminators or changes UPOS, as a new Exact analysis. (2) Learners may choose among `(scheme, value)` pairs already attested for the same key and UPOS in the same Exact analysis. (3) Learners may type a discriminator. | (1). | (1) matches today's lemma-only corrections and "no learner POS editing". A learner who thinks the analysis chose the wrong homograph can exclude the occurrence or correct it to a different lemma, but cannot pick the other homograph. (2) allows that choice, but the reading increment shows no glosses that tell `ex:1` from `ex:2`, so the choice would be uninformed. It also needs a new correction UI. (3) lets free text create identities in reviewed schemes, against the #2033 rule that discriminators come from documented schemes. | #2067 | #2061, #2060 | open |
| OD-25 | How does `ancient-greek/1` survive a toolchain whose Unicode version is not 17.0.0 (Go, `golang.org/x/text`, or a future Python implementation, where Python 3.13 has 15.1.0)? | (1) The implementation asserts 17.0.0. A toolchain change fails that test, and a reviewed change may keep `ancient-greek/1` only if the full fixture plus an exhaustive per-code-point lowercase and NFC table for every code point assigned in 17.0.0 are unchanged. Admission keeps rejecting code points unassigned in 17.0.0, through a frozen table. Otherwise the change needs `ancient-greek/2`. (2) Ship frozen 17.0.0 normalization and case tables with the profile, independent of the toolchain. (3) Follow the toolchain's Unicode version silently. | (1). | (1) keeps one definition and catches drift in review, at the cost of a generated check per toolchain upgrade. (2) is fully reproducible, but carries and maintains copied Unicode tables. (3) can change keys, and therefore Known matches, without review, against the #2033 rule that Unicode changes that can alter identity be reviewable. | #2067 | #2061 | open |
| OD-26 | How is the `grc` Concordance **form** search key computed (§6 I-6)? Today SQL computes it as `translate(lower(o.surface), 'ς', 'σ')` (`sqlc/queries/concordance.sql:115`), with PostgreSQL's unpinned `lower()` and no NFC on stored surfaces. | (1) Go computes an `ancient-greek/1` search key for each occurrence surface, which §7 stores with the occurrence, and SQL compares bytes. The key is for search only, never identity. (2) Keep the SQL expression for `grc`. | (1), with §7 placing the stored key. | (1) gives exactly the accommodation #2033 allows for search (NFC and NFD, case, sigma), from the pinned implementation. Its cost is one stored column per occurrence. (2) depends on the database's collation provider and Unicode tables, and an NFD surface never matches an NFC query. | #2067 | #2061, #2060 | open |
| OD-28 | How do new Exact analyses coexist with legacy `corpus_tokens` and `corpus_sentences` analyses (§7)? | (i) New analyses write only the §7 records. Every consumer reads both through one analysis read interface, with an exact adapter and a legacy adapter. The legacy adapter maps `corpus_tokens` rows into the same shape, with offset-keyed occurrences and no discriminator. (ii) New analyses dual-write: the §7 records plus `corpus_tokens` and `corpus_sentences` rows. | (i), with no dual-write. §7 is written against it. | (i) leaves every legacy row and the EPUB write path untouched, and keeps one source of truth per analysis. Its cost is changing every content consumer to read through the interface at once, guarded by a characterization test (OCC-11). (ii) keeps legacy consumers working unchanged, but `corpus_tokens` cannot hold unprinted nodes, shared-span occurrences, discontinuous spans or absent syntax without invented values (B-01, B-02, B-03). It would also store two copies that could disagree, and those rows are owner-scoped while Exact analyses are shared (§5). | #2068 | #2061 | open |
| OD-29 | Does the legacy adapter re-validate legacy dependency trees with the §7 tree rules (TREE-1 to TREE-4)? `validateCorpusToken` never checked one root per sentence, cycles or connection (`internal/analysis/jobs.go:1849-1862`). | (1) No. The legacy adapter reports legacy basic syntax as `usable`, exactly as consumers read it today. (2) Yes. Legacy sentences that fail are reported `invalid` and withheld. | (1). | (1) keeps the legacy characterization exact (OCC-11). A legacy tree that would fail the §7 rules stays visible as it is today. (2) applies one standard everywhere, but changes current EPUB Concordance and sentence-study output without a reviewed migration, against "legacy analyses remain readable without mass conversion" (handoff Stage 2). | #2068 | #2061 | open |
| OD-30 | Is a `non_vocabulary` word or component, such as a native-class conjunction, an Annotation occurrence? | (1) No. Occurrences are word or component nodes classed `vocabulary` or `unresolved`. Non-vocabulary nodes are accounted for but take no count and no learner decision. (2) Yes. Every text-attested word or component is an occurrence, and the accounting class decides whether it counts. | (1). §7 is written against it. | (1) makes the occurrence foreign key unable to name anything a learner cannot sensibly decide. A learner cannot correct a native-class conjunction into vocabulary; only a new reviewed interpretation can. (2) lets a learner correct or exclude such a node, but an exclusion of something that never counts has no effect, and a correction would override a reviewed classification, against #2058 item 1 ("not a learner Occurrence exclusion"). | #2068 | #2061 | open |
| OD-32 | How does the existing snapshot row anchor a classical reading? `primary_goal_snapshots` requires five legacy evidence columns and references `analysis_runs` and `corpora` (`migrations/000007_primary_goal_snapshots.up.sql:19-23`, `:30-32`), which a classical Book does not have (§5 OD-12). | (1) A new migration drops `NOT NULL` from the five columns and adds a nullable `exact_analysis_id` with a check that exactly one representation is set. (2) The same relaxation, plus a separate 1:1 classical snapshot record and a deferred trigger requiring exactly one side. (3) A parallel classical snapshot and current-reading anchor. (4) Leave `primary_goals.snapshot_id` NULL for classical readings and point at a classical snapshot elsewhere. | (1). §8 is written against it. | (1) keeps one snapshot anchor, so `primary_goals`, `reading_history` and `deck_preparations` keep their snapshot references, and a row check states the representation. It relaxes constraints on a shipped table, the kind of change OD-12 avoided. No existing row changes, because all of them satisfy the legacy branch. Queries that read the five columns directly (`sqlc/queries/current_reading.sql:109-119`, `:271-333`) must run only for legacy snapshots; `GetCurrentReading` already coalesces them (`:12-17`). (2) gives the same result with a cross-table trigger instead of a row check. (3) is a second learner state machine, which ADR 0098 forbids. (4) makes a classical reading look like the legacy no-snapshot shape, which Finish accepts with nothing to accept (`internal/persistence/current_reading.go:236-240`, `internal/domain/current_reading_finish.go:90-107`). | #2069 | #2061 | open |
| OD-34 | Is rich count readiness stored as an explicit state, or derived from River job rows as the legacy library-total query does (`internal/persistence/current_reading_lifecycle.go:197-219`)? | (1) Explicit `pending`, `ready` and `unavailable`, written by adoption, decisions and the rebuild job; the job writes `unavailable` when it exhausts its attempts. (2) Readiness row presence plus River job state, as today. | (1). §8 is written against it. | (1) makes the readiness state a persisted fact the fixture adapter can represent directly, and drops the JSON match on job arguments. It needs the job's last attempt to write the state, and a discarded job that never reaches that path leaves `pending`. (2) reuses a proven query, but couples the learner core to River's table layout. | #2069 | #2061, #2060 | open |
| OD-35 | Is the frozen snapshot headword stored per membership row, or derived when read? §6 asks §8 to place it. | (1) Stored on Rich snapshot membership and copied to Rich Known at acceptance. (2) Derived from the snapshot's analysis and the Book's decisions at read time. | (1). §8 is written against it. | (1) keeps the headword frozen even if a decision on the old analysis is later read differently, and lets the Known list show the accepted headword without a join through analyses. It costs one text column per row. (2) stores nothing, but makes a frozen display depend on mutable decisions. | #2069 | #2061, #2060 | open |
| OD-37 | How do retained records protect the evidence they depend on (§9)? | (i) Direct foreign keys along the closure: `RESTRICT` into shared evidence and `NO ACTION` between owner-scoped rows. One owner-scoped Adopted evidence record per (owner, Book, analysis) carries the acquisitions. Delete and truncate guards on shared evidence, and an owner guard on owner-scoped retention rows. (ii) A retention-reference record (referencer kind and ID → analysis and acquisitions), written beside each referencing record, plus `RESTRICT` foreign keys from the analysis downward. | (i). §9 is written against it. | **Query cost.** (i) adds a few index lookups per referencing insert and no extra row. The unreferenced report must union about nine referencing kinds, which is acceptable for a rare operator report. (ii) adds one row and one write per referencing record, and a trigger per referencer kind to remove the row when the referencer goes. Its report is a single anti-join. **Backup and restore.** Both live in one dump. In (i), under §9's single-transaction restore rule, a restore that lost a referenced row fails constraint validation, so a broken reference cannot be restored silently. In (ii), the reference row has no foreign key to its polymorphic referencer, so a restore can bring back reference rows that disagree with their referencers, and only the integrity check can notice. **Future reclamation.** In (i), any delete of a reachable row is refused by the database itself, so after replacing the guard, reclamation can only remove rows that nothing reaches. A new referencer kind must add its foreign key, which review and the `RET` rows check. In (ii), reclamation reads one table, but a referencer whose reference row was never written, or was removed early, is unprotected. Nothing in the database detects that, which is the failure ADR 0097 forbids. | #2070 | #2061 | open |
| OD-41 | Which §15 seam covers the operator commands in §9 (the evidence integrity check and the unreferenced-evidence report)? Neither has a web handler or a store-contract operation, and §15's seam (d) is for constraints. §12's `(a) + CLI` rows, §13's `(d) (OD-41)` rows and PUB-01 to PUB-03 use the same widening. | (1) Use seam (d), widened to cover operator commands run against a real, restored PostgreSQL database, as RET-14 and RET-16 do. RET-15 still observes the Book through seam (a). (2) Add a seam (e) for operator-command integration tests. | (1). | (1) keeps four seams and reuses `testutil.Postgres` (B-32), but widens (d)'s wording. (2) names the seam exactly, but changes §15's shared seam list for every ticket. | #2070 | #2061, #2059 | open |
| OD-43 | How does ADR 0091 take in the new packages (§10 "Package placement")? `internal/adoption` needs pgx and River, and ADR 0091 §3 lists pgx importers exhaustively. The web app's allow list (§6) does not name `internal/canonical`, and no rule isolates adapters. | (1) Amend ADR 0091: add `internal/adoption` to the §3 pgx list as a job package; add `internal/canonical` to the web-app allow list; add rules that keep `internal/canonical` a leaf above the domain, allow `internal/interpretation/...` only in `cmd/server`, the §12 command, `internal/parity` and test files, and deny adapters persistence, pgx, sqlc, River, `net/...`, `os`, `os/exec`, `plugin`, `syscall`, `unsafe` and `archive/zip`. Express each as depguard rules. (2) Put adoption inside `internal/analysis`, which already holds pgx, and enforce adapter isolation only with a `go list -deps` test. | (1). | (1) needs a reviewed ADR change (CODEOWNERS), but keeps every rule in the place ADR 0091 already uses, with lint messages that cite it. ADR 0091 names a new job package as a valid reason to amend §3. (2) needs no ADR change, but ties annotation-backed adoption to the NLP analysis package that NLP independence separates it from, and leaves adapter isolation outside `make lint-go`. | #2071 | #2061, #2059 | open |
| OD-44 | Where do the shared value types (rich identity, capability, capability state, analysis reference) live, given that ADR 0091 §1 lets the domain import only the standard library and `internal/canonicalization`? | (1) In `internal/domain`, as plain value types. `internal/canonical` imports the domain, and the domain never imports `internal/canonical`. (2) In `internal/canonical`, as a leaf that the domain may import, which amends ADR 0091 §1. | (1). | (1) keeps the domain's ADR 0091 rule unchanged, and lets domain decisions (Start, Finish, evidence classification) name capability states without seeing candidates or validation. (2) puts candidate and validator types within the domain's reach, and widens the domain's allow list for types it does not need. | #2071 | #2061 | open |
| OD-45 | Is Stanza parity observation on by default in production (§10 "Stanza compatibility adapter")? | (1) Off by default. Tests always run it, and an operator setting turns it on. (2) Always on. (3) Tests only, with no production hook. | (1). | (1) keeps today's analysis cost and logs unchanged unless the operator asks, and still allows real-data evidence before any future cutover. (2) adds validation time and a log event to every EPUB analysis, which ADR 0098 does not require. (3) gives no evidence beyond the fixtures. In every option, observation writes nothing and changes nothing. | #2071 | #2061 | open |
| OD-47 | Issue #2071 says "missing scope gives a rejection with a diagnostic". §7 makes a coverage gap (ACC-4) deny Vocabulary-ready instead of rejecting the candidate. Which applies? | (1) Follow §7: the candidate is Validated, `vocabulary` is `invalid` with `coverage_gap`, `vocabulary_ready` is false, and §11 never adopts it. A candidate declaring a different unit, binding or Extracted text is Rejected outright (SCOPE-1, COR-1). (2) Make ACC-4 structural, so a coverage gap rejects the candidate and nothing is stored. | (1). §10 is written against it. | (1) keeps §7 unchanged and lets §11 retain a partial analysis for owner inspection, if §11 chooses to store it. It never reaches learners, so the learner-visible outcome is the rejection the issue asks for. (2) matches the issue's wording literally, but rewrites a §7 rule, and an owner then cannot inspect which part of the unit was uncovered. | #2071 | #2061, #2059 | open |
| OD-48 | The layering rows VAL-13 and VAL-14 need a lint rule or dependency test, which the ground rules' seams (a)–(d) do not include. | (1) Allow a lint seam for layering rows only: `make lint-go` (depguard) plus a transitive `go list -deps -test` test. (2) Express layering through seam (c) by asserting only behaviour, with no import check. | (1). | (1) is the only way to test a package-import invariant, and it matches how ADR 0091 is already enforced. It adds a fifth seam kind, limited to two rows. (2) cannot observe an import, so the invariant would go untested. | #2071 | #2061 | open |
| OD-63 | What does a legacy `grc` row found by the §13 preflight stop? §6 "Legacy compatibility" and OD-24 recommend isolating the rows and continuing rollout. Issue #2074 requires rollout of Greek import to stop until a separately approved reconciliation plan exists. ADR 0098 requires an audit before rollout, and stops any reconciliation until a human-approved plan exists. OD-24 is merged here. | (1) Stop Greek import for the whole installation while any `grc` row exists (§13 STOP-1, STOP-2). (2) Block classical adoption only for affected owners (OD-24 option (3)). (3) Isolate and continue (OD-24 option (1)). (4) Apply the plain legacy rule to `grc` (OD-24 option (2)). | (1). §13 is written against it, as #2074 requires. | (1) is the strictest choice. It matches "audit before rollout", and §3 (B-33) expects no such rows, so in practice it should cost nothing. Its cost is that one owner's legacy rows stop import for every owner. (2) needs an owner-scoped gate inside §11 adoption. (3) relies on §6's comparison rule 5 alone, and lets import start before anyone has reviewed the legacy rows. If (1) is chosen, OD-24's isolation rules still describe how the rows behave after an approved plan lets import resume. (4) is unsafe: a casefold key expands iota subscripts, so a legacy subscript row byte-equals the `ancient-greek/1` key of the adscript spelling, and Known would transfer across that distinction (§6). | #2074 | #2061, #2059 | open |
| OD-24 | What happens if the §13 preflight finds legacy `grc` tuple rows (§6)? | Merged into OD-63, which carries this row's options. | See OD-63. | §6 "Legacy compatibility" still describes how legacy `grc` rows behave once an approved plan lets Greek import resume. | #2067 | #2061, #2059 | merged into OD-63 |
| OD-65 | Which seam covers MIG-13, the generated sqlc drift check? It is not a web, store-contract, validation or persistence test. MIG-15, added by #2075, uses the same seam for the shipped-migration check. | (1) Allow a CI generated-artifact seam for generated-code and shipped-migration rows only: `make sqlc` followed by `git diff --exit-code`. (2) Drop the row, and rely on repository CI without a matrix row. | (1). | (1) keeps the acceptance criterion visible in the matrix, as OD-48 does for layering. (2) leaves the criterion implicit. | #2074 | #2061 | open |
| OD-68 | Which spec makes the rich store-contract rows pass on PostgreSQL? §8 and §10 seed classical Books on the PostgreSQL adapter only through §11's adoption interface, and §13 gives adoption's writes to #2059 (S6, S9). #2061 "Testing Decisions" extends the store-contract scenarios for the transitions it exposes. | (1) The rows belong to the first spec that has §11 adoption: #2059 for adoption rows, #2060 for learner rows. #2061 proves identity, occurrence and storage invariants at seams (c) and (d), and lands the harness (OD-04) with fixture-adapter seeding. (2) #2061 ships §11's adoption transaction without the durable stages, so that its PostgreSQL adapter can seed through it. #2059 adds admission, stages and the reconciler. (3) A test-only PostgreSQL seeder writes validated analyses directly. | (1). §16 "Downstream inheritance" is written against it. | (1) changes no design, but #2061's acceptance includes no PostgreSQL run of the rich scenarios: they first pass under #2059 and #2060. (2) gives #2061 full store-contract coverage, but splits §11 across two specs. (3) contradicts §8, which seeds "through the §11 adoption interface with synthetic analyses, not by inserting rows behind it". | #2075 | #2061 | open |
| OD-70 | Is the frozen exception table's digest public? §5 pins it in the reviewed interpretation definition, and §10 VER-2 checks it against the compiled registry. §12 puts the retained table digest in the authenticated pin file, and §14 classes it local. | (1) Local. A retained interpretation's table digest comes from the authenticated pin set, and VER-2 checks it against that value instead of a compiled one. Synthetic tables keep compiled digests. (2) Public. The digest is compiled, as §5 and §10 say, and §14 moves it to the public column. | (1). §14 is written against it. | (1) keeps every hash of local material except the commitment out of the repository, as #2062's ground rules ask. `Validate` stays pure, because its caller passes the resolved digest in. (2) needs no change to §5 or §10. It reveals no table value, but publishes a hash of local material that the commitment already authenticates, so it adds no protection. | #2075 | #2061, #2059 | open |

### Blocks #2059 local import and adoption

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-05 | Does Evidence adoption publish through the existing analysis-run anchor that every Reading gate checks (B-23 to B-26), or through a new anchor? | (1) Adoption produces an exact analysis that the existing classification recognizes, so the six call sites stay unchanged. (2) A new adopted-analysis anchor, with every gate changed together. | (2), decided by #2072 (§11 "OD-05 resolution") for #2068 and #2072, consistent with §5's OD-12 (2) and §8. The adopted anchor is the parallel Book current classical analysis record. The single domain classification (ADR 0052) reads it, and every B-26 call site moves to that classification in one change. EPUB verdicts are unchanged. | (1) reuses existing anchors, as the handoff requires, but `book_current_analyses` references a source material and run. (2) needs an atomic change across all gates. | #2064 | #2059, #2060 | open |
| OD-08 | How does the operator-local path fit `CONTEXT.md:355-357` ("Books … enter the library only through a connected catalog, never by manual entry") and the closed provenance values at `internal/domain/books.go:14-19` and `:252-256`? | (1) A reviewed, compiled definition is an offering origin. Add a provenance value such as `operator_local`, and amend the `CONTEXT.md` wording in a separately reviewed change. (2) Require a synthetic local connection row, so that every Book still has a connection. | (1). Final interface decided by #2073 (§12): the operator command creates Books with provenance `operator_local`, and no connection row is invented. | (1) keeps the path free of connections, as #2065 requires, but needs a glossary change outside this ticket. (2) keeps the wording but invents a connection that has no feed, credentials or sync, and would be visible in Catalogs. | #2065 | #2059 | open |
| OD-15 | Where does the private retained pin set live, including transport, content and member digests and the exception-table digest? | (1) Compiled into reviewed application code, which publishes the private digests in the public repository. (2) A reviewed operator-local pin file, loaded through the same pin-set interface. Its identifier and version are recorded on every acquisition. (3) Synthetic pin sets in code for CI, and the retained pin set operator-local. | (3), decided by #2075 (§14) with #2073 (§12). §12 is written against (3), and authenticates the operator-local pin file with a compiled commitment (OD-57). | (1) gives one review path, but exposes values this design keeps private. (2) keeps them local, but the file's review happens outside the repository. (3) keeps CI public and synthetic and the retained values local, at the cost of two pin sources behind one interface. All three keep the rule that pins never come from the package. | #2066 | #2059 | open |
| OD-19 | Can a Usability restriction be lifted? | (1) Append-only lift records are allowed only for `integrity_failure`, after the exact bytes are restored and re-verified. A `confirmed_defect` is never lifted; recovery is a new validated analysis. (2) No lift. Every recovery is a new analysis and adoption. | (1), specified with §9 and §11. | (1) lets an exact restore return evidence to use without rewriting history, as ADR 0097 "Durable bounded work and recovery" requires. (2) makes a restored, byte-identical package permanently unusable. Neither §9 nor §11 specifies the lift record, and no §15 row covers it (#2075 audit). Until one is specified, recovery is §13 REC-4's restore into a fresh database, which holds no restriction appended after its dump. | #2066 | #2059 | open |
| OD-38 | Usability restriction (§5) has no blob subject. Which subject does an `integrity_failure` on one package build name (§9 IC-1, IC-2)? | (1) The blob's Package content, so every build of that content is restricted. (2) Add a blob subject to Usability restriction, and restrict only analyses reached through acquisitions of that blob. | (1). | (1) needs no change to §5. An owner whose acquisition is a different, intact build of the same content also sees unavailability until an exact restore and lift (OD-19). That is conservative, and rare, because a second build needs its own pin-set version (§5 "Pins"). (2) is precise, but adds a subject kind to §5 and makes restriction scope depend on owner acquisitions, against OD-20's installation-wide reading. | #2070 | #2059 | open |
| OD-39 | Is a preparation request a retention root in every state, or only while in flight? This includes whether a `blocked` request is terminal (OD-53, merged here). | (1) Live only while not terminal. A terminal request (every §11 state other than `queued` and `running`, including `blocked`) keeps its foreign keys as physical rows, but the unreferenced report treats it as a non-root holder (`held_by`). (2) Live in every state. | (1). The state names belong to §11. | (1) follows ADR 0097, which lists "in-flight work" as a reference. It lets the report show evidence that only an abandoned request touched, while the foreign keys still block any delete until a separately approved reclamation decides about request history. (2) never reports such evidence, so a cancelled import holds its bytes forever, even in the report. Under (1), §9's illustrative query must add `blocked` to its terminal states. | #2070 | #2059 | open |
| OD-53 | Is a `blocked` request terminal, and a non-root holder under OD-39? §9's illustrative query lists only `adopted`, `cancelled`, `failed` and `superseded` as terminal. | Merged into OD-39. | See OD-39. | OD-39 (1) treats `blocked` as terminal, like every state other than `queued` and `running`. | #2072 | #2059 | merged into OD-39 |
| OD-49 | Which mechanism enforces the installation capacity bounds transactionally (§11 "Capacity")? | (1) A transaction-scoped advisory lock with one fixed key (the capacity gate), taken by admission and by the queued-to-running step, with counts read after it. (2) A singleton lock row, seeded by a migration and locked `FOR UPDATE`. (3) A database constraint alone. | (1). | (1) follows the existing fixed-key precedent (`internal/persistence/persistence.go:346`), needs no seeded row, and is released automatically at commit or rollback. Its cost is a key that must not collide with other advisory keys, so the key is a named constant. (2) is equally race-free and visible in `pg_locks` as a row lock, but it fails open if the seed row is missing, and needs a migration-owned row. (3) is impossible: "active" depends on lease expiry and "pending" on the deadline, which a constraint cannot read. A partial unique index cannot express a bound of 32. | #2072 | #2059 | open |
| OD-50 | What happens when a Book becomes the current reading while a replacement request runs? ADR 0097 says frozen Reading keeps its exact evidence, and active-reading vocabulary changes only through End, review and Start. | (1) Adoption refuses. The request becomes `failed` with `book_is_current_reading`, and Retry is offered after End or Finish. (2) Adoption proceeds. The frozen snapshot is unchanged, but the Reading page's current evidence changes under it. (3) Start cancels the Book's open replacement. | (1). §11 is written against it. | (1) never shows a Reading on one revision while its Book's evidence is another, and matches `admitToReadBookAnalysis`, which refuses the current Book (`internal/analysis/jobs.go:277-306`). The owner repeats the request later. (2) adopts sooner, but exposes mixed revisions during a Reading, which ADR 0097 forbids. (3) makes Start cancel work, which is a new Start side effect. | #2072 | #2059 | open |
| OD-51 | How is a request refused for capacity represented? | (1) A persisted request in `failed` with `installation_queue_full`, no job, and Retry offered. (2) No row. The handler shows a message, and the Book stays To Read with no request. (3) A new state `refused`. | (1). | (1) keeps the refusal visible after the response, and keeps the ticket's state set. The refused row is terminal and never counted. (2) leaves no durable trace, so the owner cannot later tell why nothing happened ("silently dropped"). (3) adds a state the ticket does not list. | #2072 | #2059 | open |
| OD-52 | §8 lets adoption set count readiness `pending` and enqueue a rebuild when the build does not fit the transaction. ADR 0097 requires ready projections before adoption. Which applies? | (1) Counts are built in a `prepare_projections` stage before adoption, and adoption copies them and sets `ready` in its own commit. The `pending` path is never taken at adoption. (2) Follow §8: adoption may publish with `pending` counts. | (1). §11 is written against it. | (1) satisfies ADR 0097 "Required vocabulary count projections must be ready", and the adoption commit writes the projections, as #2072 requires. The cost is a staged row set per request. (2) can publish a Book whose counts are not usable yet, so Start and Browse wait after adoption. §8's `pending` path still serves decisions and rebuilds. | #2072 | #2059 | open |
| OD-54 | Does §11 store a validated analysis that is not Vocabulary-ready (OD-47)? | (1) Store it, with its diagnostics and §7 rows, for owner inspection. Never adopt it. The request is `blocked`. (2) Store nothing. The request carries the diagnostics only. | (1). | (1) lets the owner see which occurrences or code points failed, through §5's access rule. The cost is retained rows that no root keeps live, which the unreferenced report then lists. (2) stores less, but the owner can see only counts per code. | #2072 | #2059 | open |
| OD-56 | Which operator-local entry point selects the retained package and frozen interpretation (§12)? | (i) An operator command registers the package, creates the Book, sets To Read and admits the initial request. (ii) An operator-configured local directory, plus a learner-visible "Add local edition" action in the web app. (iii) Hybrid: the operator command registers the package and the owner's Inbox Book; the learner moves it to To Read through the existing My Books action, which admits the request. | (iii). §12 is written against it. | (i) needs no learner step, but the operator changes a learner's disposition and makes the informed choice ADR 0096 reserves for the learner, and admission gains a second caller outside the disposition transaction §11 requires. (ii) needs no operator step per owner, but the web process reads and parses package files on a request path, the directory needs its own owner scoping, and "Add local edition" is a new UI pattern. (iii) keeps file access in an operator process and learner intent in the existing flow. Its cost is one command run per owner. | #2073 | #2059 | open |
| OD-57 | How does the application authenticate the operator-local pin file (OD-15 (3)), and how does the server read private pins without file access? | (1) Reviewed code compiles a commitment, the SHA-256 of the exact pin-file bytes, into each retained pin-set entry. The command accepts the file only when its digest matches, and stores it as an immutable Installed pin set. The server reads pins only from that row, re-checking the commitment on every load. (2) Trust the pin file as reviewed, and let the server load it at start-up from an environment path, as it loads `MOUSEION_DICTIONARY_INDEX` (`cmd/server/main.go:377-401`). (3) Only the command reads the pin file; the server trusts the persisted §5 inventory rows instead of pins. | (1). | (1) makes reviewed code the authority for every pin while keeping private digests out of the public repository; a changed file or row is detected. Its cost is one shared record, and a release for every new retained pin-set version. It publishes the commitment, which reveals no package or member hash; §14 (#2075) confirms that. (2) needs no new record, but the pin file's review happens outside the repository, and the server reads an operator file on every start. (3) has no second pin source in the server, but `verify` and `interpret` would check stored bytes against rows written by the same path, not against pins. | #2073 | #2059 | open |
| OD-58 | Can a release remove a pin-set or interpretation version that existing acquisitions, selections or requests name? | (1) The compiled registries are append-only. A superseded version is marked retired: the command refuses it for new registrations (`pin_set_retired`), and §11 and §9 can still resolve and verify it. (2) Versions may be removed. Requests that name one become `blocked` with `pin_set_unknown`, and retained evidence can no longer be re-verified. | (1). | (1) keeps every retained acquisition verifiable and every request resolvable, as ADR 0097's retention requires. The registry grows by one entry per release. (2) keeps the registry small, but strands retained evidence and makes a restore unverifiable. | #2073 | #2059 | open |
| OD-59 | How does a Book with no current evidence reach a newer Local selection, when its initial request is `blocked` for a deterministic reason? §11 makes To Read reuse the most recent initial request's pinned choice, and Refresh requires current evidence. | (1) Allow Refresh on a Book with no current evidence. It creates an `initial` request for the named candidate with `approved_at` set and no expected evidence, which §11's checks permit. (2) To Read re-selects from the current Local selection when the most recent initial request is `blocked` with a non-retryable code. (3) No path. The Book stays unavailable. | (1). | (1) keeps "To Read never re-pins" and makes every new choice an explicit owner approval. It needs §11 to allow Refresh without current evidence. (2) needs no new action, but To Read then selects a new candidate implicitly, which the handoff forbids ("repeated To Read never select a new revision implicitly"). (3) leaves a Book permanently unusable after a reviewed fix. | #2073 | #2059 | open |
| OD-60 | How is ADR 0096's "explicit informed choice" for automatic or unknown-provenance annotation made on the local path? §11 says an initial request needs no approval. | (1) To Read on a classical Book whose release-scope claims include `automatic` or `unknown` is a `Confirmation` disclosure that states provenance by layer. Confirming records `approved_at` on the initial request. (2) The operator's registration is the informed choice. (3) Show provenance on the row only, with an ordinary To Read. | (1). §12 is written against it. | (1) puts the choice with the owner, on an existing pattern, and leaves a durable record. Its cost is one extra disclosure step for these Books. (2) needs no learner step, but the operator chooses for the learner. (3) is informative, but nothing shows the learner made the choice. | #2073 | #2059 | open |
| OD-61 | Where does the safe XML reader live, so that admission (ADM-10) and adapters parse XML under the same rules? §10 lets adapters import only `internal/canonical`, `internal/domain` and `internal/canonicalization`. | (1) A new leaf package, `internal/safexml`, importing only the standard library. Adapters and `internal/evidencepackage` may import it, through the ADR 0091 amendment (OD-43). (2) Put the reader in `internal/canonical`. (3) Each user configures `encoding/xml` itself, checked by tests. | (1). | (1) gives one implementation and one test suite for entity, DTD, encoding and depth rules, at the cost of one more allow-list entry. (2) needs no new allow-list entry, but puts parsing into the validation package, which §10 keeps pure of input formats. (3) lets the two copies drift, so admission could accept what an adapter would parse differently. | #2073 | #2059 | open |
| OD-64 | Which restriction subject does §9's integrity check use when a Package blob is missing and only acquisitions reference it (IC-6)? The blob carries the link to its Package content, so that link is lost with the blob. | (1) The Package content of each text and annotation revision behind every Adopted evidence record or current row whose acquisition names the missing blob. (2) Report the restore as failed, with no restriction. | (1). | (1) lets the affected Books show unavailable through §5's transitive rule, and leaves Books that do not depend on the blob available, as MIG-12 asserts. Like OD-38, it restricts every build of that content. (2) is simpler, but leaves the installation unusable until an exact restore succeeds. | #2074 | #2059 | open |
| OD-69 | Which recorded decision opens the rights gate that the operator-local import checks? §12 P-5 names #2054. §10 "Failure classes" and §11's `verify` stage name #2054–#2056, and §5 and §9 also list #2055 as an external gate. #2062, #2059, #2054 and #2056 say that #2054 and #2056 hold only public interpretation release and catalog publication, and that #2059 owns the local integration operation's assessment. #2055 is closed, consolidated into #2063. | (1) The operator-local import gate records #2059's local-operation disposition. #2054 and #2056 gate only public release and catalog publication, and #2063 owns the provider and export gates. The citations in §5, §9, §10, §11 and §12 are corrected once this is approved. (2) Keep #2054 as the local import's gate, as §12 says. | (1). §14 PUB-R5 and PUB-03 are written against it. | (1) matches every controlling issue. The gate stays per operation, so no mechanism changes. Until it is approved, #2059 cannot say which recorded decision opens its gate. (2) lets an open public-release question block the local route, which #2062 "Implementation Decisions" and #2059 rule out. | #2075 | #2059 | open |
| OD-71 | EVD-03, EVD-04, EVD-05, EVD-07 and EVD-33 run "the local import ... through its handler and River job" and expect the request to end `failed`. VAL-06 does the same for unknown versions, and ADP-06 changes a byte of a local package and expects a `blocked` request. Under §12, these local failures are refused by the command before any request exists (P-3, P-4, ADM-1 to ADM-10). §11 records failures found on stored bytes as `blocked`, not `failed`. | (1) Restate the rows' actions as `cmd/localregistration` runs at seam (a) + CLI, with §12's exit codes. Keep §11 request-state assertions only for failures that the `verify` and `interpret` stages find on stored bytes, as `blocked`. (2) Keep the rows as written, as tests of the Stage 3 catalogue path, and move them to that spec. | (1). | (1) makes every row reachable in this design's scope. Some rows then overlap LOC-01 to LOC-17, which is acceptable, because they assert §5's no-write invariants. (2) leaves #2059 with rows it cannot run, and leaves §5's invariants without an in-scope row. | #2075 | #2059 | open |
| OD-72 | May §9's unreferenced-evidence report print blob and Package content digests? §12 and §13 print no digest, and §14 classes package hashes as local-only. | (1) Yes. The report runs only on the installation, and its output is local-only material that is never posted (§14 PUB-R3). (2) The report names blobs and contents by record IDs and a per-run ordinal, and prints digests only with an explicit operator flag. | (1). | (1) needs no change to §9, and the operator can match a blob to the file they hold. A pasted report would expose hashes, which only the review rule prevents. (2) makes a pasted report safe, but blobs and contents are keyed by digest, so the report needs a second handle. | #2075 | #2059 | open |
| OD-73 | §12 says that §9's delete guard must cover the Installed pin set, and that §9's referencer list must include the Local selection, which references an acquisition. §9 lists neither, and no row covers them. | (1) The Installed pin set gets the shared-evidence delete guard (RET-GUARD-1). The Local selection is an owner-scoped retention root: its acquisition reference stays `NO ACTION` (RET-FK-2), it gets the owner guard (RET-GUARD-2), and the unreferenced report treats a Book's current Local selection as live. (2) Neither is protected. Pins are recoverable from the operator's file, and an acquisition that only a Local selection names is reported unreferenced. | (1). | (1) keeps every retained pin set resolvable, which OD-58 needs for re-verification, and keeps a registered package from being reported unreferenced before the learner moves its Book to To Read. It adds two guards and one referencer to §9. (2) adds nothing, but a lost Installed pin set row blocks `verify` with `pin_file_missing` until the operator registers again. | #2075 | #2059 | open |
| OD-74 | How is new work paused under storage pressure? ADR 0097 says "monitor storage and queue pressure; pause/reject new acquisition before exhausting capacity", and the handoff says "configure operator capacity checks/alerts and pause new work before exhaustion". §11 bounds queue pressure. No section pauses new work on storage pressure, and no row covers it. | (1) Leave storage monitoring to the operator ("the operator owns capacity", ADR 0097). A full database stays a failure that writes nothing (§12 exit 1, §11 transient `storage_unavailable`), and nothing is evicted (RET-01, RET-14). (2) Add an operator-configured storage threshold that admission and the command check before new work, and that refuses with a permanent code. #2059 specifies it before implementation. | (2). | (1) adds nothing, but new work is refused only once the database is already full, which is later than ADR 0097 asks. (2) meets ADR 0097, at the cost of a configuration value, a code and a design addition that needs review. | #2075 | #2059 | open |
| OD-77 | Which εἰμί rule does `oga-agdt-upos/4` apply, AUX or VERB? #2089 supersedes `oga-agdt-upos/3`, its εἰμί rule and the six POS repairs that corrected that rule's output. AUX is non-vocabulary and VERB is vocabulary (#2058), so the rule changes the εἰμί VERB count. The rule may rest only on the UPOS definitions and the source's own annotation, never on a treebank. OGA's own annotation is the retained source (#2030), read as evidence, not a treebank witness. | (1) AUX where the source's own native annotation marks εἰμί as a copula, and VERB otherwise (existential and possessive uses), following the UPOS definitions of AUX and VERB. (2) AUX for every εἰμί. (3) VERB for every εἰμί. | (1). A human decides and records the rule in Stage 1, before the L-0 re-run. | (1) follows the UPOS definitions and reads only the source's own annotation, but it inherits the automatic annotation's errors, which are disclosed and not repaired (§5). (2) needs no native syntax, but it removes every existential εἰμί from vocabulary. (3) needs no native syntax either, but it counts copular εἰμί as vocabulary, against the AUX definition. | #2089 | #2059 | open |

### Blocks #2060 Reading and vocabulary

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-01 | Must rich identities reach the tuple-keyed Browse selection and Custom deck stores (B-17), whose UI is being retired? | (1) Greek identities are never written there; a test asserts it. (2) Add a discriminator to those stores. | (1), decided by #2069. §8 is written against it, and LRN-25 asserts it. | (1) adds no structure, but a Greek identity cannot be selected for a Custom deck. (2) changes stores outside the Reading path for a retiring UI. | #2064 | #2060 | open |
| OD-33 | Do Rich Known rows affect legacy (tuple) selection? §6 defines equality between a legacy tuple and a null-discriminator, non-`grc` rich identity. | (1) No. Legacy selection reads only `known_vocabulary`. Rich selection reads Rich Known plus the legacy rows that equal its candidates under §6. (2) Both directions apply the §6 equality. | (1). | (1) guarantees that no legacy outcome changes (LRN-14). It is unobservable today, because the only rich language is `grc`, and §6 rule 5 keeps `grc` legacy and rich apart. A future non-`grc` classical language needs this revisited before its Known crosses representations. (2) is symmetric but makes legacy selection read a new record for no current case. | #2069 | #2060 | open |
| OD-42 | Issue #2070 says a Book that fails the integrity check shows "unavailable with a reason, with no End or Finish". ADR 0097 "Durable bounded work and recovery" says "do not automatically End or Finish a Reading. Operations wholly supported by intact frozen records may continue, including explicit End/Finish". | (1) Follow the ADR: no automatic End or Finish, and explicit End and Finish stay available. (2) Follow the ticket wording: refuse explicit End and Finish too. | (1). The ADR is binding (§1), and §9 and RET-17 are written against it. | (1) lets a learner close a reading whose frozen snapshot is intact, without touching evidence. (2) traps the learner in a current reading until an operator restores the evidence, which contradicts a binding ADR. | #2070 | #2060 | open |

### Blocks #2063 cards

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-76 | #2062 "Testing Decisions" bullet 2 reuses the store-contract scenarios for deck admission. §8 leaves rich deck records to #2063, and no row covers deck admission from a rich snapshot. LRN-15 and ADP-21 need a deck preparation on a rich snapshot in their setup. | (1) #2063 adds its deck-admission rows, with §9's R6 references, when its card design is reviewed. LRN-15 and ADP-21 pass their other assertions under #2060, and gain their deck setup in #2063. (2) Add rich deck-admission rows to this design now. | (1). | (1) keeps card design out of this document, as #2062 scopes it. (2) designs card records here, which §8 assigns to #2063. | #2075 | #2063 | open |

### Blocks only the Stage 3 discovery spec

These rows concern catalogue offerings, which ship with the Stage 3 discovery spec (OD-66). They block none of the four specs.

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-09 | Where do classical catalogue connections live, given that `opds_connections.owner_id` is nullable and has no unique `(owner_id, id)` (`migrations/000001_initialize.up.sql:1267-1277`, `:1960`)? | (1) Generalize `opds_connections` with a connection kind, and add a unique `(owner_id, id)`. (2) A separate classical connection record, owner-scoped with `(owner_id, id)` unique. | (2), decided in the spec for handoff stage 3 ([catalog release and durable acquisition](annotation-backed-ancient-greek.md#3-authorize-catalog-release-and-implement-durable-acquisitionadoption)). The local route needs neither. | (1) shares one Catalogs list but carries the OPDS single-`language` and `url` columns, which ADR 0096 says classical data must not be forced through. (2) lets a Unit offering use a composite owner and connection key, but adds a second connection kind to Catalogs. | #2065 | Stage 3 | open |
| OD-10 | Does later sync update a classical Book's `title`, `author` and `language`? | (1) They are set once at creation from the Work. Advertised metadata is stored on the Unit offering, and display reads Work, Edition and unit descriptions. (2) Follow OPDS: sync overwrites them, and a language change ends an incompatible Current reading (`internal/persistence/books.go:832-847`). | (1). | (1) means sync can never end a reading or rename a Book shared by several offerings. A changed description reaches the Book only through a reviewed definition. (2) lets two offerings of one Book overwrite each other. | #2065 | Stage 3 | open |
| OD-66 | In which step do §4's Unit offering and Claimed alias ship? Unit offering references OD-09's classical connection record, which is Stage 3 work, and none of #2061, #2059 or #2060 implements catalogue offerings. | (1) No §13 step. The Stage 3 discovery spec adds them, with the connection record, as a later step under §13's rules. (2) Add them, with the connection record, to S2. | (1). | (1) ships no table without a writer, as ADR 0038 asks, and §12 Local registration covers this design's scope. Identity conflict kinds about claimed aliases stay unused until Stage 3. Under (1), SC-3 (upstream withdrawal) and the offering rows that §16 "Downstream inheritance" lists are proven only by the Stage 3 spec, because the local path has no upstream (#2075 audit). (2) settles Stage 3's connection shape early. | #2074 | Stage 3 | open |

### Blocks no spec

Vocabulary rows propose `CONTEXT.md` changes, which are separately reviewed. Until one lands, implementation uses the design-local names. OD-75 is a review rule.

| ID | question | options | recommendation | consequence | raised by | blocks | status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OD-11 | New terms used in §4. | (1) Add to `CONTEXT.md`: **Unit offering**, **Local registration**, **Verified identity alias**, **Claimed alias**, **Identity conflict**, **Edition reconciliation**, **Reconciliation class**, **Book–Reading unit association**, **Named Edition** / **Unidentified Edition**, and **classical Book** (a Book with a Book–Reading unit association). (2) Keep them as design-local names. | Add them in a separately reviewed `CONTEXT.md` change. **Unit offering** is defined as one declared Reading unit that a Catalog entry offers. | Adding them gives implementation and UI one vocabulary. Keeping them local leaves "offering" ambiguous: `CONTEXT.md:273-274` already calls a Catalog entry "an offering", and "alias" is ambiguous against the OPDS `catalog_entry` alias. | #2065 | none | open |
| OD-18 | New terms used in §5. | (1) Add to `CONTEXT.md`: **Package acquisition** (an owner's verified acquisition of exact package bytes; their provenance and evidence access), **Capability diagnostic** and **Usability restriction**. Keep Package blob, Package content (source-content identity, as against a blob's transport identity), Package member, pin set, Evidence notice, Notice coverage, Extracted text, Interpretation configuration, Exact analysis, Diagnostic finding, Provenance claim and Book current classical analysis as design-local names. "Owner evidence access" is the role that Package acquisition plays, not a separate record. (2) Keep every term design-local. | Add the three in a separately reviewed `CONTEXT.md` change. | The three appear in learner-facing explanations and in other sections. The rest are storage detail. "Exact analysis" stays design-local, so that it does not compete with the existing "analysis". | #2066 | none | open |
| OD-27 | New terms used in §6. | (1) Add to `CONTEXT.md`: **Rich vocabulary identity** (language, canonical lemma, UPOS and an optional scheme-qualified lexical discriminator), **Lexical homograph discriminator** (the #2033 term), **Citation headword** (the decoded citation form shown to learners, distinct from the canonical lemma key) and **Source decoding** (the versioned adapter step from native lemma data to a decoded citation form and discriminator). Keep Vocabulary identity (the interned record), Analysis headword, Discriminator scheme and dedicated-profile language as design-local names. Extend the existing **Effective vocabulary identity** entry to name the rich identity. (2) Keep every term design-local. | Add them in a separately reviewed `CONTEXT.md` change. | The four appear in learner-facing explanations, corrections and other sections. Without them, "lemma" stays ambiguous between key, headword and native value. The rest are storage detail. | #2067 | none | open |
| OD-31 | New terms used in §7. | (1) Add to `CONTEXT.md`: **Non-vocabulary classification** (a source-supported, reason-coded accounting class that is not an Occurrence exclusion), **Multiword container** (a printed token whose linguistic components are counted instead of it) and **Unprinted node** (an annotation node with no text span, such as an empty node). Extend the existing **Annotation occurrence** entry to say it is identified by (Exact analysis, node), never by offsets. Keep Analysis sentence, Analysis node, Node span, Sentence span, Container component, accounting class, analysis read interface and the rule IDs as design-local names. (2) Keep every term design-local. | Add them in a separately reviewed `CONTEXT.md` change. | The three appear in learner-facing explanations of counts and coverage. Without them, "excluded" is ambiguous between a learner's Occurrence exclusion and a source-supported classification. The rest are storage detail. | #2068 | none | open |
| OD-36 | New terms used in §8. | (1) Add to `CONTEXT.md`: **Snapshot representation** (legacy tuple or rich, fixed when a snapshot is frozen) and **Library total** (an identity's effective count over the owner's contributing Books in one language, the 15 of the 3/15 rule). Extend **Known vocabulary** and **Current-reading vocabulary snapshot** to say they hold rich identities for classical Books. Keep Exact occurrence decision, Exact Lemma review flag, Rich count readiness, Rich vocabulary count, Rich snapshot membership, Snapshot count contribution and Rich Known vocabulary as design-local names. (2) Keep every term design-local. | Add them in a separately reviewed `CONTEXT.md` change. | The two terms appear in learner-facing explanations of why a word was selected and why Start is waiting. The rest are storage detail. | #2069 | none | open |
| OD-40 | New terms used in §9. | (1) Add **Retention reference** to `CONTEXT.md`: the reference by which a retained learner, request, deck or restriction record protects the exact evidence it depends on, transitively, without granting access. A record that holds one is a retention root. Keep retention root, evidence closure, Adopted evidence record, delete guard, owner guard, unreferenced-evidence report and evidence integrity check as design-local names. (2) Keep every term design-local. | Add **Retention reference** in a separately reviewed `CONTEXT.md` change, and keep the rest design-local. | Adding it gives implementation, ADR 0097 and UI copy one word for "why this cannot be deleted". Keeping everything design-local avoids a glossary change, but leaves ADR 0097's "retained reference" undefined in `CONTEXT.md`. | #2070 | none | open |
| OD-46 | New terms used in §10. | (1) Add to `CONTEXT.md`: **Canonical-analysis candidate** (an adapter's unvalidated output) and **Validated analysis** (shared validation's verdict, the only thing adoption and the learner core accept). Keep source interpretation adapter, shared canonical validation, learner core, compatibility adapter, parity observation, failure class, effective capability state, adoptable (analysis), Reading-ready (Book) and the rule IDs as design-local names. (2) Keep every term design-local. | Add the two in a separately reviewed `CONTEXT.md` change. | Both appear in other sections and in diagnostics shown to owners. Without them, "analysis" is ambiguous between a candidate, a validated analysis and a stored Exact analysis. The rest are module or storage detail. | #2071 | none | open |
| OD-55 | New terms used in §11. | (1) Add to `CONTEXT.md`: **Preparation request** (an owner's durable, bounded intent to prepare one pinned choice for one Book) **Refresh** (an owner's explicit approval of a named replacement candidate for a Book) and **Retry** (an explicit owner action that creates a new bounded request for the same pinned choice). Keep stage, stage attempt, attempt number, Preparation attempt, Staged count row, admission, capacity gate, reconciler, expected evidence, pending, active and lease as design-local names. (2) Keep every term design-local. | Add the three in a separately reviewed `CONTEXT.md` change. | All three appear in learner-facing Book detail states and in ADR 0097's wording ("refresh request", "explicit Retry"). The rest are worker detail. | #2072 | none | open |
| OD-62 | New terms used in §12. | (1) Keep **Local selection**, **Installed pin set**, pin file, pin-set commitment, admission check and interpretation definition (a compiled, named Interpretation configuration) as design-local names. Amend the `CONTEXT.md` **Book** entry as OD-08 says. (2) Add them to `CONTEXT.md`. | Keep them design-local. | None of them appears to learners: learners see the Edition, unit, provenance and request states. Adding them would grow the glossary with operator detail. | #2073 | none | open |
| OD-67 | New terms used in §13. | (1) Add to `CONTEXT.md`: **Legacy-`grc` preflight** (the read-only report that must be clear before Greek import starts). Keep logical step, the STEP, STOP and REC rules, the P, A and R checks, and rollout check as design-local names. (2) Keep every term design-local. | Add the one term in a separately reviewed `CONTEXT.md` change. | The preflight's result decides whether an operator can import Greek, so operators need the name. The rest is implementation detail. | #2074 | none | open |
| OD-75 | Some #2062 requirements cannot be tested by any matrix row: story 14, "Implementation Decisions" bullets 1 and 13 (design only, and explicit approval before SQL), "Testing Decisions" bullet 6 (no migrations, provider calls or fixture execution for this design), and the human-channel part of §14 PUB-R3 (no local material in issues, pull requests, review comments or agent and provider messages). How are they checked? | (1) As review rules. The reviewer checks them on each pull request, §18 records approval, and no row is added. (2) Add a CI scan that fails on 64-hex digests and on Greek text outside an allowlist of invented and #2033 forms, in `doc/`, `testdata` and pull-request bodies. | (1). | (1) adds nothing, and relies on review. (2) catches some leaks automatically, but cannot recognize a private value it has never seen, and needs its allowlist updated for every new invented example. | #2075 | none | open |

## 18. Approval record

Status: **changes requested.** §1's status line stays "not approved" until this
record is filled in with an approval.

The repository owner fills in this record under
[#2076](https://github.com/justin-hayes/mouseion/issues/2076). An agent may
apply recorded answers, but never records approval on the owner's behalf. A
merged pull request is not approval (§1).

| field | value |
| --- | --- |
| Outcome | Changes requested |
| Approver | justin-hayes (review outcome only; no approval is recorded) |
| Date | 2026-10-15 |
| Approved document revision | none; reviewed at `1cc4e7f7` |
| Approved sections | none yet |
| Sections with changes requested | §5, §6, §7, §10, §13, §14, §15 and §16, and the handoff, through [#2089](https://github.com/justin-hayes/mouseion/issues/2089) (no treebank data: independent `oga-agdt-upos/4`, a morphology-mapping slot, an exception-table lineage audit, a restated Stage 1 check), [#2090](https://github.com/justin-hayes/mouseion/issues/2090) (defer syntax storage for Exact analyses) and [#2091](https://github.com/justin-hayes/mouseion/issues/2091) (place Greek dictionary lookup in the learner flow) |
| Conditions | **No treebank data.** No treebank (AGDT, PROIEL, Gorman, UD conversions) is a runtime input, mapping basis, repair witness or test oracle, and no source with a non-commercial licence is used. This deliberately overrides the handoff's Stage 1 "do not … change mappings merely to evade the rights gate" and #2058's "do not claim independence" for the UPOS table. The source decision (#2030) is unchanged. |
| Approval comment on #2062 | none: #2062 stays open until approval |

Answer every §17 row: accept the recommendation, choose another option, or send
it back for narrow rework.

| decision | answer | notes |
| --- | --- | --- |
| OD-01 | _pending_ | |
| OD-02 | _pending_ | |
| OD-03 | _pending_ | |
| OD-04 | _pending_ | |
| OD-05 | _pending_ | |
| OD-06 | _pending_ | |
| OD-07 | _pending_ | |
| OD-08 | _pending_ | |
| OD-09 | _pending_ | |
| OD-10 | _pending_ | |
| OD-11 | _pending_ | |
| OD-12 | _pending_ | |
| OD-13 | _pending_ | |
| OD-14 | _pending_ | |
| OD-15 | _pending_ | |
| OD-16 | _pending_ | |
| OD-17 | _pending_ | |
| OD-18 | _pending_ | |
| OD-19 | _pending_ | |
| OD-20 | _pending_ | |
| OD-21 | _pending_ | |
| OD-22 | _pending_ | |
| OD-23 | Accept (1) | Revisit option (2) once dictionary glosses exist (#2091). |
| OD-24 | — | Merged into OD-63. Record no separate answer. |
| OD-25 | _pending_ | |
| OD-26 | _pending_ | |
| OD-27 | _pending_ | |
| OD-28 | _pending_ | |
| OD-29 | _pending_ | |
| OD-30 | _pending_ | |
| OD-31 | _pending_ | |
| OD-32 | _pending_ | |
| OD-33 | _pending_ | |
| OD-34 | _pending_ | |
| OD-35 | _pending_ | |
| OD-36 | _pending_ | |
| OD-37 | _pending_ | |
| OD-38 | _pending_ | |
| OD-39 | _pending_ | |
| OD-40 | _pending_ | |
| OD-41 | _pending_ | |
| OD-42 | _pending_ | |
| OD-43 | _pending_ | |
| OD-44 | _pending_ | |
| OD-45 | Option (3) | Parity runs in tests only; no production hook. |
| OD-46 | _pending_ | |
| OD-47 | _pending_ | |
| OD-48 | _pending_ | |
| OD-49 | _pending_ | |
| OD-50 | _pending_ | |
| OD-51 | _pending_ | |
| OD-52 | _pending_ | |
| OD-53 | — | Merged into OD-39. Record no separate answer. |
| OD-54 | _pending_ | |
| OD-55 | _pending_ | |
| OD-56 | _pending_ | |
| OD-57 | _pending_ | |
| OD-58 | _pending_ | |
| OD-59 | _pending_ | |
| OD-60 | _pending_ | |
| OD-61 | _pending_ | |
| OD-62 | _pending_ | |
| OD-63 | _pending_ | |
| OD-64 | _pending_ | |
| OD-65 | _pending_ | |
| OD-66 | _pending_ | |
| OD-67 | _pending_ | |
| OD-68 | _pending_ | |
| OD-69 | Accept (1) | |
| OD-70 | _pending_ | |
| OD-71 | _pending_ | |
| OD-72 | _pending_ | |
| OD-73 | _pending_ | |
| OD-74 | Option (1) | Single-user installation; the operator owns storage capacity. |
| OD-75 | _pending_ | |
| OD-76 | _pending_ | |
| OD-77 | Accept (1) | AUX where the native annotation marks εἰμί as a copula, VERB otherwise. Recorded from the owner's answer on #2093. |

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

_Pending: owned by the bibliographic identity ticket._

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

## 18. Approval record

_Pending: owned by the approval ticket._

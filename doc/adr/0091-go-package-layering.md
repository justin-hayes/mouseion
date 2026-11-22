# ADR 0091: Go package layering

Status: **Accepted** · Date: 2026-10-12 · Author: Justin + opencode

Amended 2026-10-13 by [ADR 0092](0092-retire-known-vocabulary-import.md): `internal/knownvocab` was retired, so it no longer appears in the pgx owner list or the web-app composition list ([#2010](https://github.com/justin-hayes/mouseion/issues/2010)).

Amended 2026-11-09 by [ADR 0116](0116-classical-catalog.md): the web app may read the built-in Classical catalog from `internal/classical`, a leaf package of reviewed manifest artifacts.

Amended 2026-11-19 by [issue #2458](https://github.com/justin-hayes/mouseion/issues/2458): the web app composes `internal/readingintent`, which owns a Book's reading intent — the To Read transition, the route between Classical preparation and EPUB acquisition and analysis, and the operator log line for a failed submission. It is added to §6's allow list.

Amended 2026-11-21 by [ADR 0119](0119-retire-german-vocabulary-backfill.md): `cmd/vocabularybackfill` was retired, so the pgx-owner explanation now names only `cmd/cataloguebackfill`.

Amended 2026-11-21 by [ADR 0120](0120-retire-catalogue-alias-backfill.md): `cmd/cataloguebackfill` was retired, so no operator backfill command remains; operator commands reach the database through persistence instead.

Amended 2026-11-22 by [issue #2504](https://github.com/justin-hayes/mouseion/issues/2504): the web app and the fixture server no longer import `internal/persistence`. The domain owns the remaining sentinels and value types, and depguard denies the import in their non-test files.

Tracked by [issue #1973](https://github.com/justin-hayes/mouseion/issues/1973), under parent [#1811](https://github.com/justin-hayes/mouseion/issues/1811). Enforcement with golangci-lint's `depguard` follows in [#1974](https://github.com/justin-hayes/mouseion/issues/1974) (test infrastructure and fixtures) and [#1975](https://github.com/justin-hayes/mouseion/issues/1975) (persistence layering).

## Context

The Go code has a layering that earlier decisions rely on, but no gate checks it. [ADR 0052](0052-domain-owns-evidence-classification.md) makes the domain the single owner of evidence classification. [ADR 0087](0087-vocabulary-counts-come-only-from-selection.md) keeps the vocabulary eligibility rule only in Go `selection`, so SQL never re-encodes it. [ADR 0088](0088-fixture-adapter-transition-store-contracts.md) keeps an in-memory fixtures adapter for the browser harness and holds its transitions to shared `internal/storecontract` scenarios. Each of these depends on imports flowing one way. An agent can still import the PostgreSQL store into the domain package, or the fixtures store into production code, and every current gate passes.

`go list -tags=integration` at `21e0a090`, test imports included, shows the current graph:

- **Domain.** `internal/domain` imports the standard library and `internal/canonicalization`. Its tests add testify.
- **Generated sqlc code.** `gen/sqlc` is imported only by `internal/persistence`. At `21e0a090` two files in `internal/prepareddeck` (`current_reading_admission.go`, `jobs.go`) also imported it and called three generated queries, `GetBookForUpdate`, `GetSourceMaterialBookForUpdate` and `RetireDeckPreparationsForBook`; [#1976](https://github.com/justin-hayes/mouseion/issues/1976) moved those calls behind transaction-scoped persistence methods. Persistence calls all three for the same purposes.
- **pgx.** Non-test files in `internal/persistence`, `internal/txcleanup`, `cmd/server`, the job packages `internal/analysis`, `internal/cataloguesync` and `internal/prepareddeck`, generated `gen/sqlc`, and `internal/testutil` import `github.com/jackc/pgx/v5`. The only other importers are three `internal/webapp` integration tests, which build River clients and inspect database state.
- **Fixtures store.** `internal/fixtures` is imported by `cmd/fixtureserver` and by `internal/webapp` tests. `cmd/fixtureserver` also imports `internal/persistence`, for the `persistence.MyBooksBrowseResult` type in a store wrapper. `internal/fixtures` itself imports neither persistence, pgx nor sqlc.
- **Test infrastructure.** `internal/storecontract`, `internal/testutil`, `internal/testwrite` and `internal/analyzer/analyzertest` are imported only by test files. `testwrite` is used by tests in `cataloguesync`, `enrichment`, `opds` and `webapp`.
- **Web app.** Non-test `internal/webapp` files import the domain, canonicalization, service and job packages, `auth`/`webauth`, and `internal/persistence`, but not pgx, sqlc, fixtures or test infrastructure. Their persistence uses are sentinel errors (`ErrNotFound`, `ErrCurrentReadingStale`, `ErrCurrentReadingExists` and seven others) and value types in store interface signatures (`MyBooksBrowseResult`, `StartResult`, `LanguageCount`, `CurrentReadingIneligibleError`). No web app file names `persistence.PostgresStore`, and none calls a persistence function: [#1977](https://github.com/justin-hayes/mouseion/issues/1977) moved the `MOUSEION_SECRET` check to the `internal/deploysecret` leaf package.

All of the above are direct imports. Transitively, both `internal/fixtures` and `internal/webapp` link pgx and `gen/sqlc` through the job packages, and the rules below do not try to prevent that.

The observed graph is evidence, not the decision. The rules below keep what the earlier ADRs rely on and decide the cases the graph leaves open.

## Decision

Each rule names a package group, says what it may and may not import, and gives the reason. The rules constrain direct imports only, which is what depguard checks. "Test files" means `_test.go` files, including integration-tagged ones. A rule that names a package covers its non-test files unless it says otherwise. Packages that no rule names are not constrained by this ADR. "Fixtures store" means `internal/fixtures`, and "fixture server" means `cmd/fixtureserver`.

### 1. Domain: the standard library and canonicalization only

- **Allow:** the standard library and `internal/canonicalization`. Domain test files may also import `github.com/stretchr/testify`.
- **Deny:** everything else, including persistence, pgx, sqlc, River, gRPC and generated protobuf code, the web app, and every other internal package.

**Why.** The domain holds the decisions that must not be restated elsewhere: evidence classification (ADR 0052), admission order, and the types every adapter shares. If it imported persistence or a transport, those decisions would bend toward one adapter's types. Both the PostgreSQL store and the fixtures store could then no longer implement them as equals, which ADR 0088's shared contracts depend on. Canonicalization is allowed because vocabulary identity is a domain concept that the domain needs to compute. Canonicalization itself stays a leaf (standard library and `golang.org/x/text`), so the domain keeps no transitive internal dependencies.

### 2. Generated sqlc code: persistence only

- **Allow:** `internal/persistence`, in non-test and test files.
- **Deny:** every other package, in non-test and test files.

**Why.** `sqlc.yaml` already describes `gen/sqlc` as the persistence query layer. Its row and parameter types (`pgtype.UUID`, nullable wrappers, per-query structs) mirror the schema. Persistence exists to translate them into domain types, so an importer outside persistence couples itself to column shapes and has to change with every migration.

Prepared-deck's direct use is **not intended**. Its three queries duplicate persistence calls that make the same Book-row lock and preparation retirement. With the queries in two packages, the Book-before-preparation lock order is defined twice and can drift. Prepared-deck already reaches persistence through transaction-scoped methods (`LoadPreparedDeckInputFactsTx`, `FreezePreparedDeckRunTx`), so persistence now exposes these three operations in the same way ([#1976](https://github.com/justin-hayes/mouseion/issues/1976)).

### 3. pgx: named owners of connections and transactions

Non-test files may import `github.com/jackc/pgx/v5` and its subpackages only in these packages:

| Package | Why it needs pgx |
| --- | --- |
| `internal/persistence` | The PostgreSQL store: owns the pool, queries and transactions. |
| `gen/sqlc` | Generated; its queries take a pgx connection or transaction. |
| `internal/txcleanup` | Shared rollback policy over `pgx.Tx`. |
| `cmd/server` | Server wiring: opens the pool and builds the River client over it. |
| `internal/analysis` | Job package: enqueues River jobs in the same transaction as the run state it writes ([ADR 0010](0010-river-job-queue.md)). |
| `internal/cataloguesync` | Job package: same reason as `analysis`. |
| `internal/prepareddeck` | Job package: same reason as `analysis` ([ADR 0030](0030-durable-prepared-deck-translation.md)). |
| `internal/testutil` | Test infrastructure: provisions the per-test PostgreSQL database. |

Test files in any package may import pgx.

**Why.** pgx is the database boundary. Code that holds a `pgx.Tx` can read and write any table, so the importer list is the list of code that can bypass the store. The job packages are on the list because River's transactional enqueue takes a `pgx.Tx`: a job and the state change that schedules it must commit together (ADR 0010), and these packages own the durable job-state tables they lock. Other code reaches the database through persistence or through a job package's service. The backfill commands (`cmd/cataloguebackfill`, `cmd/vocabularybackfill`) already do. Tests may use pgx because they don't ship, and integration tests need it to build River clients and to inspect the state a transition left.

Adding a package to this list requires amending this ADR. A new job package is a reason to amend it. Convenience is not.

### 4. Fixtures store: the fixture server and tests only

- **`internal/fixtures` may be imported by:** `cmd/fixtureserver` and test files.
- **`internal/fixtures` may not import:** `internal/persistence`, `gen/sqlc` or pgx.
- **`cmd/fixtureserver` may not import** `internal/persistence`, `gen/sqlc` or pgx. It names only domain errors and value types, so it cannot construct or open a PostgreSQL store.

**Why.** Production code wired to the in-memory store would lose learner data silently, so the fixtures package must never be reachable from `cmd/server` or the packages it composes. The fixtures store is a second adapter behind the same interfaces as the PostgreSQL store (ADRs [0033](0033-browser-acceptance-harness.md) and 0088). It may import the domain and the service and job packages whose types appear in those interfaces, but not the PostgreSQL adapter, because ADR 0033's fixture server binds no PostgreSQL.

The fixture server's persistence import is **denied**. The web app's store interfaces name only domain types, so a fixture wrapper that satisfies them names the domain too. Lint enforces the ban, which also keeps the fixture server unable to construct a PostgreSQL store. The server stays free of PostgreSQL, as ADR 0033 requires.

### 5. Test infrastructure: test files only

The test-infrastructure packages are `internal/storecontract`, `internal/testutil`, `internal/testwrite` and `internal/analyzer/analyzertest`.

- **Allow:** test files in any package.
- **Deny:** non-test files in every package, except files inside these packages themselves.

**Why.** These packages exist to make tests short and shared. `testutil` starts containers and clones databases, `storecontract` encodes ADR 0088's cross-adapter scenarios with testify assertions, `testwrite` fails a test on a write error, and `analyzertest` supplies a fake analyzer, fake capabilities and the analyzer contract. None of that belongs in a shipped binary, and production code that called it would fail at runtime or behave like a test double. `analyzertest` is not in the parent issue's list, but it has the same role and importers, so the rule covers it. Adding a package to this group amends this ADR.

### 6. Web app: domain, services and store interfaces

- **Allow:** the standard library, the third-party libraries it already uses (templ, `golang.org/x/text` for localized formatting, River's `rivertype` for job identifiers), `internal/domain`, `internal/canonicalization`, `internal/classical`, `internal/deploysecret`, the service and job packages it composes (`analysis`, `analysisinsights`, `analyzer`, `cataloguesync`, `enrichment`, `epub`, `lemmadisplay`, `lemmareview`, `opds`, `prepareddeck`, `readingintent`), `internal/auth` and `internal/webauth`. It may not import `internal/persistence`.
- **Enforcement:** the allow list states intent. Lint enforces the deny entries below. A new internal dependency outside the allow list needs this ADR amended, and review checks for one.
- **Deny (non-test files):** pgx, `gen/sqlc`, `internal/persistence`, `internal/fixtures` and every test-infrastructure package.
- **Deny (test files):** `gen/sqlc`. Web app tests may import fixtures, test infrastructure and pgx.

**Why.** Handlers render state and route intent. Decisions belong to the domain (ADR 0052), to `selection` for vocabulary counts (ADR 0087), and to the services. Data access goes through store interfaces that the web app defines and both adapters satisfy (ADR 0088). A handler holding pgx or sqlc could query around the store and fork a rule the domain owns. A handler holding `*persistence.PostgresStore` would tie the web app to one adapter, and the fixture server could no longer serve it. The store interfaces name only domain errors and value types ([#2504](https://github.com/justin-hayes/mouseion/issues/2504)), so lint denies the persistence import outright rather than asking review to tell a type reference from a store reference. `internal/classical` is a leaf package of reviewed manifest artifacts with no database, transport or job dependency; the web app reads its built-in catalog view directly (ADR 0116).

### 7. Suppressions cite this ADR

A `//nolint:depguard` directive must give a reason that names ADR 0091 and the follow-up issue that will remove it, for example `//nolint:depguard // ADR 0091 temporary exception, removed by #1976`. Prefer an exception in the depguard configuration, scoped to the named files, over an inline directive. A suppression without such a reason is a review failure even where lint accepts it. The `nolintlint` settings stay as they are: they already require a named linter and an explanation, and this rule only constrains what the explanation says.

**Why.** A layering exception is a debt with an owner. Naming the ADR and the issue keeps each exception discoverable from both ends and stops suppression from becoming the easy fix for a design problem.

## Temporary exceptions

None. [#1976](https://github.com/justin-hayes/mouseion/issues/1976) moved prepared-deck's `gen/sqlc` calls behind persistence, and [#1977](https://github.com/justin-hayes/mouseion/issues/1977) moved `MOUSEION_SECRET` validation out of persistence.

No other import or reference at `21e0a090` violates these rules.

## Alternatives considered

- **Keep prepared-deck's sqlc use as intended.** Rejected. The three queries duplicate persistence's own calls and define the Book lock order a second time. Once persistence owns them, the move costs three methods.
- **Forbid persistence in the web app now.** Rejected for this ADR. The web app's store interfaces carry persistence types, so the ban would force a domain-type migration before enforcement could start. The narrower "errors and value types only" rule, checked in review, keeps handlers off the store implementation now. Superseded by the 2026-11-22 amendment, which completed that migration and enforces the ban.
- **Forbid persistence in the fixture server.** Rejected for the same reason: the fixture store has to satisfy the web app interfaces as they are. Superseded by the same amendment.
- **Allow pgx only in persistence.** Rejected. River's transactional enqueue needs a `pgx.Tx` in the job packages. Routing every enqueue through persistence would move job semantics into the store.
- **Keep the rules in review only.** Rejected. Review is how the rules eroded until now, and agents follow lint failures more reliably than prose.

## Consequences

- [#1974](https://github.com/justin-hayes/mouseion/issues/1974) and [#1975](https://github.com/justin-hayes/mouseion/issues/1975) express §§1–6 as depguard rule sets keyed by file globs. Every `deny` message cites ADR 0091, and the temporary exception appears in config with its follow-up issue.
- A forbidden import fails `make lint-go` with a message that names this ADR, so the fix is a design change or an amendment, not a suppression.
- Adding a pgx importer, a sqlc importer or a test-infrastructure package means amending this ADR first.
- Lint denies `internal/persistence` in non-test web app files and in `cmd/fixtureserver`, so neither can name an adapter type or construct a PostgreSQL store.
- Rules for Python code, and further restructuring beyond #1976, are out of scope.

## Related

- [ADR 0010: River job queue](0010-river-job-queue.md) — transactional enqueue, the reason the job packages hold pgx.
- [ADR 0030: Durable prepared-deck translation runs](0030-durable-prepared-deck-translation.md) — prepared-deck's durable job state.
- [ADR 0033: Browser acceptance harness](0033-browser-acceptance-harness.md) — the fixture server binds no PostgreSQL.
- [ADR 0052: The domain owns evidence classification](0052-domain-owns-evidence-classification.md) — why the domain stays free of adapters.
- [ADR 0087: Vocabulary counts come only from selection](0087-vocabulary-counts-come-only-from-selection.md) — why handlers and SQL don't restate selection rules.
- [ADR 0088: Fixture adapter transitions are held to store contracts](0088-fixture-adapter-transition-store-contracts.md) — the two adapters and the shared store contracts.

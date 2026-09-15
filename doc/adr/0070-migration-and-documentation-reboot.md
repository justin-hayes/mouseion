# ADR 0070: Reboot migration history and consolidate superseded documentation

Status: **Proposed** · Date: 2026-09-15 · Author: Justin + opencode

## Context

Discovery-driven development produced 75 paired migrations (`000001`–`000075`)
and 69 ADRs in about a month. Much of the migration history is short-lived
reversal rather than current shape: `000029_long_context` was removed by
`000032_remove_long_context`; `000021_learning_campaigns` was dropped by
`000061_drop_learning_campaigns`; the `000022`–`000027` EPUB scope and
classifier schema was dropped by `000043_drop_classifier_tables` and
`000051_remove_retired_analysis_schema`; `000046_drop_language_profiles`
removed the language-profile schema; `000066`–`000068` rewrote the My Books
and source-material evidence read models; and `000073`–`000075` reworked the
enrichment cache. The churn is active, not historical — migrations 000073–000075
landed while this decision was being written.

The documentation carries the same archaeology. The `product.md` ADR index
records long supersession chains (`0005`→`0017`→`0048`, `0008`→`0018`,
`0019`→`0027`→`0036`→`0053`, `0042`→`0057`), and the feature index is partly a
catalogue of retired surfaces. A reader must chain several ADRs to learn what
the application currently does.

[ADR 0038](0038-schema-change-governance.md) declares every shipped migration
immutable deployment history. That premise is reproducibility and auditability
of deployed state. It is not load-bearing here: Mouseion is a single-user
home-lab stack (`compose.yaml`), `redeploy-mouseion.sh` removes the `db-data`
volume on every deploy, there are no releases or tags, and no external consumer
depends on the in-tree migration sequence. The cost of preserving the churn —
every reader and every tool walking 75 files to find the current schema — is now
paid on every schema change.

## Decision

### One current-state baseline

Consolidate every migration shipped before the baseline into a single baseline
pair `000001_initialize.{up,down}.sql` that reproduces the current schema
exactly. The baseline is not a redesign and does not prune dormant structures;
it is the schema at the pre-baseline version. Acceptance proves it faithful:
`make sqlc` regenerated from the baseline is byte-identical to the pre-baseline
output, and a `pg_dump --schema-only` of a database created from the baseline
equals one created from the retired history. Future migrations resume from
`000002`.

The baseline PR pins an explicit commit SHA and a migration freeze is announced
on the tracking issue. Any migration landing after the pin becomes the
baseline's successor rather than being absorbed into it.

The baseline includes a full teardown down migration (drop and recreate the
application schema), documented as a development convenience. Production
applies `Up` only (`internal/persistence/persistence.go`), and down migrations
remain not assumed safe for production rollback.

### Preserve the retired history out of the active path

Tag the pre-reboot commit `migrations/pre-baseline`, and note the tag in the
baseline header and in `doc/archive/migrations/README.md`. Git is the archive;
the retired SQL is not copied into the documentation tree. The active
`migrations/` directory contains only the baseline and future work.

### Cut over by recreating databases

Old databases are at the pre-baseline version; the baseline is version 1 with
no upgrade path between them. Every database — home-lab `db-data`, dev, and the
integration-test instance — is recreated from the baseline. This is break-glass,
not a supported upgrade. The cutover task records an optional manual
`pg_dump` of the learner database as insurance; it is not automated, and the
deploy already recreates the volume.

### Rebuild migration tests

Tests that assert a historical migration's own transition are retired, because
those migrations no longer exist. Tests that used old migrations only to build
legacy state for an assertion about current behavior are rebuilt against the
baseline. No code asserts migration count, ordering, or the `schema_migrations`
version, so none is affected. Baseline correctness rests on the existing sqlc
drift gate (`make sqlc` plus `git diff --exit-code`) and on every integration
test replaying the full migration set; no golden schema dump is added.

### Reconcile documentation to the present state

`product.md` and `CONTEXT.md` are the present-state reading path. The feature
and ADR sections of `product.md` are rewritten — not the whole document — so
they describe current behavior only. Fully retired feature documents move to
`doc/archive/features/` with a historical banner; live-but-unindexed documents
rejoin the index; stale-but-current documents are corrected.

ADRs stay in `doc/adr/` and remain append-only: decision content is immutable
and only status lines change. **Relative links, however, may be maintained when
a referenced file moves** — a link fix is not a decision change. The ADR index
separates current decisions from superseded ones so no chain has to be walked.

Two glossary descriptions in `CONTEXT.md` are corrected to current behavior:
the introduction's retired "confirmed scopes" wording and the Concordance
entry's conditional "where dependency analysis has been persisted", which
understates always-on dependency parsing. Process vocabulary ("reboot",
"baseline") does not enter the glossary.

### Restate migration immutability from the new baseline

ADR 0038's "Shipped migration history is immutable" decision and the matching
non-goal are superseded once, for this reboot. Its other clauses — settle
consequential shape before SQL; separate structural DDL from data-only
backfills; stage when compatibility or volume requires it; review gates —
remain in force. Migration immutability restarts at the new baseline, and
`AGENTS.md`'s immutability rule is reworded in the same change that squashes
history (the baseline PR), not in this record's PR.

## Consequences

### Positive

- In-tree migrations and the primary docs describe the current application;
  schema and prose read as one state instead of a sequence of reversals.
- sqlc and the schema stay verified in sync through the reboot's acceptance
  checks.
- Future migrations and decisions accumulate from a clean baseline.

### Costs

- In-tree migration archaeology is given up; it survives only in git history
  (tag) and the archive. The one-time cutover destroys the existing home-lab
  database, which the current deploy already does.
- Transition tests are rewritten or retired, a one-time cost that must land
  with the baseline so CI stays green.
- ADR 0038's immutability guarantee is interrupted; the reboot itself is a
  governance decision requiring CODEOWNER review.

## Non-goals

- No product behavior or schema change: the baseline reproduces the current
  schema, it does not simplify it.
- No deletion of ADRs or rewriting of their recorded decisions; only status
  lines and relative links change.
- Not a standing licence to edit migrations after the reboot.

## Related

- [ADR 0038: Schema-change governance and migration review policy](0038-schema-change-governance.md) — partially superseded by this ADR.
- [ADR 0003: PostgreSQL as the initial persistence backend](0003-postgresql-persistence.md)
- [Documentation Governance](../documentation-governance.md)

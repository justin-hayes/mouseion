# ADR 0039: Drop retired EPUB classifier schema

Status: **Accepted** · Date: 2026-09-02 · Author: Justin + Codex

## Context

The classifier-led EPUB scope workflow was retired by issues #508–#511.
Classifier execution, display, and Store APIs are no longer part of the
application, but migrations 000023–000027 left classifier tables, scope
metadata, and a confirmation-key index in the database. Those structures have
no active consumer and no longer provide useful compatibility.

## Decision

Migration 000043 drops the two classifier tables and the classifier,
selection-mode, and confirmation-key columns from `epub_reviewed_scopes`.
Reviewed scopes remain immutable and owner-scoped, identified by their scope
UUID and their persisted source/snapshot relationships. Confirmation inserts
do not deduplicate equivalent selections through a separate confirmation key;
a new scope UUID creates a new immutable scope revision.

The down migration restores the retired table and column structures only. It
uses neutral structural values for existing scope rows and cannot restore
classifier or confirmation data removed by the up migration.

## Consequences

- The active schema and application no longer retain dormant classifier state.
- Existing scope selections and analysis provenance remain readable without
  classifier metadata.
- Rolling back migration 000043 restores compatibility structure, but not the
  deleted classifier records or their original values.
- The destructive migration requires the repository's human schema review.

## Related decisions and specifications

- [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](0028-explicit-scoped-analysis-lifecycle.md)
- [EPUB analysis scope review](../features/epub-analysis-scope-review.md)

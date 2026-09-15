# ADR 0035: Separate My Books membership from acquired source provenance

Status: **Accepted** · Date: 2026-08-31 · Author: Justin + Codex

## Context

The canonical learner-facing architecture defines My Books as the broad set of
works a learner wants Mouseion to remember. A book can be desired, unavailable,
or known only through bibliographic metadata. It does not need an EPUB, an
analysis, a prepared deck, Reading Journey membership, or Primary Goal status.

The shipped acquisition model has a narrower and intentionally stronger
contract. A `source_materials` row represents acquired evidence, not a wish or a
bibliographic placeholder. Its source identifier, language, media type, content
hash, EPUB bytes, and extracted full text are required. Later migrations add
immutable source-content revisions and extracted-unit snapshots, and confirmed
scopes, analysis runs, corpora, and deck preparations bind to that acquired
evidence through owner-scoped identities.

Representing an unacquired book by making those source fields nullable would
make every downstream consumer distinguish real evidence from a partial row. It
would also turn absence into an overloaded acquisition state and weaken the
database constraints that currently prevent analysis without validated source
content. My Books needs a broader membership model without changing what an
acquired source proves.

## Decision

### Separate bibliographic identity, membership, and acquired evidence

Introduce a durable, owner-scoped **Book** as the learner's bibliographic
identity. Represent My Books membership as a durable owner-scoped record keyed
by Book ID, with explicit `active` or `removed` state and transition timestamps;
membership is never inferred from whether source content exists. A Book may have
zero or more immutable acquired source-content revisions, and each acquired
revision belongs to exactly one owner-scoped Book.

The conceptual relationship is:

```text
owner
  -> Book (bibliographic identity)
       -> My Books membership
       -> zero or more acquired source-content revisions
            -> extracted-unit snapshot
                 -> reviewed scope -> analysis -> prepared artifact/history
```

Book metadata and membership may exist before acquisition. `source_materials`
and its revision/snapshot descendants remain acquired-only. Metadata-only Books
must not be represented by placeholder source identifiers, empty hashes, empty
EPUB bytes, empty extracted text, sentinel languages, or partial source rows.

My Books membership alone grants no analysis capability. Scope review requires
a complete current source-content revision and extracted-unit snapshot; analysis
requires a confirmed scope; deck preparation requires the exact completed
analysis. A Book does not need to pass through any of those later states, join
Reading Journey, or become a Primary Goal.

### Book metadata and language state

Every Book requires:

- an owner ID and stable owner-scoped Book ID;
- a non-empty display title;
- metadata provenance identifying manual entry or the catalog/origin from which
  the metadata was recorded;
- an explicit language state of either `unknown` or `chosen(language-tag)`; and
- creation and last-metadata-update timestamps.

Author/contributor, edition, publication date, publisher, external
bibliographic identifiers, catalog links, and cover metadata are optional
because catalogs and manual entries do not reliably supply them. Metadata
provenance is retained when fields are refreshed so a mutable display value is
not mistaken for acquired-content identity.

`unknown` contains no language tag. `chosen(language-tag)` contains a non-empty,
normalized tag selected explicitly by the learner in a workflow that names the
choice. Catalog metadata may be displayed and retained as provenance, but it
does not silently change `unknown` to `chosen`. Mouseion performs no language
inference under this decision. A chosen language may be temporarily unsupported;
current NLP capability remains a separate readiness gate. Acquisition and every
new extraction or analysis request require an explicitly chosen language, and
the language actually used is frozen into the resulting provenance. Later Book
metadata edits do not rewrite existing snapshots or analyses.

### Canonical identity and duplicate handling

The Book ID, scoped by owner, is canonical. Titles and authors are mutable
display metadata and are never identity keys. Cross-owner matching or source
reuse is forbidden even when two owners use the same catalog identifier or
acquire identical bytes.

A Book can carry owner-scoped identity aliases:

- a catalog-entry alias is the catalog connection plus that catalog's stable
  entry/source identifier;
- a strong bibliographic alias is a namespace and normalized value whose
  semantics identify the relevant work or edition, such as an edition-specific
  ISBN; and
- manual or weak metadata has no automatic identity power.

Within one owner, an alias belongs to at most one Book. Re-adding a known alias
resolves to that Book and restores My Books membership if it was removed.
Normalized title/author similarity may offer a duplicate warning, but never
auto-merges records. Conflicting strong aliases stop the operation for explicit
reconciliation; they do not cause last-write-wins reassignment. An explicit
future merge operation may consolidate duplicate Books only if it retains all
aliases and rewrites owner-scoped links without altering immutable evidence.

Acquisition resolves bibliographic identity and content identity separately:

1. Resolve or create the owner-scoped Book from its Book ID or strong aliases.
2. Download and validate the complete EPUB, then compute the versioned digest of
   the actual container bytes before publishing acquired state.
3. Resolve the owner-scoped catalog source identifier and content digest. If
   both identify the same Book and revision, acquisition is idempotent. A new
   source identifier with identical bytes reuses the existing content revision
   and records the alias. The same source identifier with different bytes creates
   a new immutable revision and extracted-unit snapshot for the same Book; it
   never replaces prior bytes in place. Different identifiers and different
   bytes create a new revision for the resolved Book.
4. If the source identifier and digest resolve to different Books, stop and
   require explicit duplicate reconciliation. Never attach one owner's evidence
   across Books or merge Books merely because an acquisition request collided.

A different content revision invalidates no historical scope or artifact. It
becomes the candidate current revision and requires new scope review before
analysis, as required by ADR 0028.

### Acquired-content invariants

The separate Book model must not relax the acquired path. These constraints are
part of the implementation contract and must have migration and integration
coverage:

- The shipped `source_materials` compatibility row remains acquired-only. Its
  `owner_id`, `language`, `source_identifier`, `title`, `media_type`,
  `content_hash`, `content`, and `full_text` are `NOT NULL`.
  `(owner_id, source_identifier)` and `(owner_id, id)` remain unique. The
  baseline migration also made `(owner_id, content_hash)` unique; migration
  `000026` deliberately moved authoritative content identity into immutable
  revisions and removed that compatibility-table constraint. Metadata-only
  membership is not a reason to weaken any remaining constraint.
- An authoritative `source_content_revisions` row requires non-null owner,
  source-material, digest version, content digest, EPUB bytes, and full text.
  For digest version 1, the stored digest must equal SHA-256 of the stored bytes.
  `(owner_id, source_material_id, content_digest)` is unique, owner/source links
  are enforced by composite foreign keys, and updates are rejected. Future
  acquisition logic must resolve equal versioned byte digests idempotently
  within one owner even if the physical constraint changes during rollout.
- An extracted-unit snapshot has a non-null owner, source-material identity,
  content-revision identity, schema version, and snapshot identity. Its units
  have non-null identity, order, offsets, source paths, media metadata, and text;
  unit identity and order are unique within the owner/source/snapshot. Snapshot
  and unit updates are rejected.
- Reviewed scopes bind the same owner, Book source, content revision, and unit
  snapshot through composite foreign keys. Analysis runs additionally bind the
  confirmed scope and analyzer/configuration identity. Corpora, insights, deck
  preparations, and their artifacts continue to consume those acquired
  identities, never a bare Book or My Books membership.
- Every lookup and mutation reloads and validates the complete owner chain.
  Browser-supplied Book, source, scope, analysis, or artifact IDs are references
  only. No alias, digest match, or shared normalized corpus authorizes access to
  another owner's EPUB bytes, extracted text, sentences, history, or artifacts.

Consequently, tests must prove that metadata-only Books cannot create scopes,
analyses, or deck preparations; incomplete acquisitions publish no acquired
state; duplicate and re-acquisition cases follow the rules above; content and
unit mutation remains rejected; and cross-owner IDs and aliases cannot bridge
the provenance chain.

### Independent removal and retention

Removal is three separate decisions, never one cascading "delete book" action:

1. **Remove from My Books** changes only active membership. The Book identity,
   aliases, acquired source evidence, and immutable history remain. Re-adding
   the Book restores membership on the same Book ID. This action does not define
   a future Reading Journey or Primary Goal transition; those contracts must
   decide their own response without using membership removal to delete evidence.
2. **Remove acquired availability** detaches the Book's current acquired-source
   choice, leaving it without a currently usable source regardless of membership.
   It does not mutate or delete a source revision. A separately confirmed
   destructive purge may delete EPUB bytes, extracted text, and units only when
   the complete snapshot is unreferenced by scopes, analyses, decks, campaigns,
   or other immutable history; it removes the evidence as a unit rather than
   nulling fields. If any immutable history references it, ordinary purge is
   rejected.
3. **Immutable history** is retained and remains addressable even when membership
   is removed or a later acquisition becomes current. There is no item-level
   history deletion in this decision. Whole-account deletion retains the
   existing owner-cascade boundary. Any future legal/erasure workflow that must
   remove referenced evidence or history requires a separate decision defining
   tombstones, cascade order, and what provenance can still be claimed.

Each action must state its scope and consequences before confirmation. Removing
a catalog connection remains independent of all three.

### Staged rollout

Implementation uses an expand/backfill/switch/contract rollout and does not edit,
renumber, or squash shipped migrations:

1. **Expand.** Add the owner-scoped Book, membership, alias, language-state, and
   source-link shape while leaving the existing acquired tables and application
   reads intact. New schema work must first satisfy the decision/review policy
   being established by issue #449; structural changes and any data backfill are
   reviewed separately when practical.
2. **Backfill.** Create one active My Books Book for every existing owned
   `source_materials` identity and link all existing revisions and descendants
   without changing their IDs, bytes, hashes, extracted units, scopes, analyses,
   or artifacts. Copy the existing required acquisition language into
   `chosen(language-tag)`; this preserves an explicit shipped input and is not
   language inference. Preserve source identifiers as aliases. Ambiguous
   collisions fail the backfill for investigation rather than merging evidence.
3. **Dual compatibility.** Write and read the Book link for new acquisitions
   while `/library`, shipped **My Library** copy, and historical links continue
   to resolve. Acquisition remains all-or-nothing and still publishes a Book's
   acquired state only after EPUB validation and snapshot persistence succeed.
4. **Switch.** Make My Books the collection read model and enable metadata-only
   additions. In the EPUB catalog workflow, **Add to My Books** may create or
   restore membership and acquire the EPUB in one operation; if membership
   already exists without content, the action attaches the validated acquired
   revision to that Book. Analysis gates remain source-based.
5. **Contract.** Remove obsolete compatibility assumptions only after backfill
   verification, owner/duplicate audits, rollback or forward-fix planning, and
   all consumers use Book plus explicit acquired-source identities. Do not make
   source content nullable as a cleanup step.

The rollout must be independently deployable at each compatibility stage and
must document retry safety, transaction boundaries, locking, failure recovery,
and irreversible operations in the implementation issue and migration review.

## Alternatives considered

- **Make `source_materials` content, hashes, extracted text, and language
  nullable, with an acquired-state discriminator and gates in every consumer.**
  Rejected. This converts a proven acquired-evidence entity into a sum type,
  weakens useful `NOT NULL` and integrity constraints, makes accidental analysis
  of placeholders possible, and requires every downstream query and foreign key
  path to reproduce the same state check. An explicit discriminator does not
  recover the structural guarantee supplied by separate entities.
- **Create a separate owner-scoped bibliographic/aspirational Book and link
  immutable `source_materials` evidence after acquisition.** Chosen. It models
  the learner's intent independently while preserving the existing acquired
  source boundary and makes zero-source versus one-or-more-source lifecycle
  states explicit.
- **Treat a catalog entry or ISBN as the Book primary key.** Rejected because
  catalogs change identifiers, ISBNs are absent or edition-specific, manual
  entries may have neither, and conflicting metadata requires an owner-visible
  reconciliation path rather than global identity claims.
- **Use normalized title and author as an automatic deduplication key.** Rejected
  because translations, editions, pseudonyms, punctuation, and incomplete
  metadata create both false matches and false splits.
- **Delete acquired evidence and history whenever a Book leaves My Books.**
  Rejected because collection intent is reversible and cannot safely control
  immutable analysis, deck, campaign, or vocabulary provenance.

## Consequences

- My Books can represent desired and unavailable works without pretending an
  EPUB was acquired or a language was inferred.
- The existing acquisition, scope, analysis, and preparation validation chain
  stays strict and testable; metadata-only additions cannot enter it.
- Book identity, content identity, and membership identity become separate
  concepts that UI, persistence, import, and authorization code must name
  explicitly.
- Duplicate handling is conservative. Some ambiguous records require learner or
  operator reconciliation instead of unsafe automatic merging.
- Re-acquisition preserves all prior bytes and derived history and requires
  scope review for changed content.
- Removing a book from the visible collection does not reclaim source storage.
  Referenced evidence and immutable history can therefore continue to consume
  storage until a separately designed retention or whole-account deletion path
  applies.
- The staged rollout requires additive schema, a deterministic backfill, and a
  compatibility period. Those changes are future implementation work subject to
  schema and human review; this ADR introduces no migration or application code.

## Related

- [Information architecture](../design/information-architecture.md)
- [Acquisition to My Books workflow](../design/workflows/acquisition-to-library.md)
- [Explicit scoped-analysis workflow](../archive/features/explicit-scoped-analysis-workflow.md)
- [ADR 0027: Single-active learning campaigns and vocabulary graduation](0027-learning-campaigns.md)
- [ADR 0028: Explicit scoped-analysis lifecycle and immutable artifacts](0028-explicit-scoped-analysis-lifecycle.md)
- Issues #449 and #460.

# ADR 0044: Catalogue-entry alias identity retains the catalogue connection

Status: **Accepted** · Date: 2026-09-05 · Author: Justin + opencode

## Context

ADR 0035 defines a catalogue-entry alias as *catalogue connection plus stable
entry/source identifier*. The shipped implementation records only
`(owner, namespace, value)` on `book_aliases`, dropping the connection before
persistence. `ReconcileCatalogueEntry` therefore reconciles by source identifier
alone, and `RefreshEntry` and `FindAcquisitionTarget` compensate by scanning
every owner connection to re-locate an entry. Two learner-owned catalogues that
use the same local entry ID for unrelated books can merge into one Book or let
one sync overwrite the other's title; refresh or acquisition may select the
first matching entry from the wrong connection. Connection provenance leaks away
at the persistence seam even though ADR 0035 makes it part of identity.

## Decision

A Book's catalogue-entry alias identity is **owner + connection + entry**. The
catalogue connection becomes part of the identity at the seam, not an
afterthought:

- `book_aliases` gains a nullable `connection_id`. Catalogue-entry aliases carry
  a real connection and are unique on `(owner_id, connection_id, namespace,
  value)` via a partial index. Strong-bibliographic aliases (for example an
  edition-specific ISBN) remain connection-independent and keep their existing
  `(owner_id, namespace, value)` uniqueness via a `WHERE connection_id IS NULL`
  partial index.
- The reconcile seam accepts the connection as a parameter, and refresh and
  acquisition resolve the alias, read the connection back, and walk only that
  connection's feed instead of scanning every connection.
- Existing catalogue-entry aliases are backfilled deterministically from the
  connection whose feed offered the matching entry. The backfill is idempotent
  and re-runnable, and it fails loudly rather than guessing when an entry ID is
  absent from every feed or present in more than one.

The rollout follows ADR 0038's expand → backfill → switch discipline: add the
nullable column, backfill, then tighten uniqueness and make it non-null. The
shipped migration `000036` remains immutable.

### Single-catalogue posture

Mouseion currently supports a single catalogue per learner, so the multi-
connection collision cannot occur today. The connection is nonetheless threaded
through the identity now: it removes the leaky connection-scanning lookup
immediately and means the future multi-catalogue decision is only about relaxing
the count and adding conflict rules, not about re-shaping identity. The
multi-connection conflict-resolution rules are explicitly deferred.

### ISBN is not a cross-source identity

An ISBN (strong-bibliographic alias) is not a stable Book identity key. ADR 0035
already rejected treating a catalog entry or ISBN as the Book primary key:
catalogs change identifiers, ISBNs are absent or edition-specific, manual
entries may have neither, and conflicting metadata needs an owner-visible
reconciliation path. The ISBN remains an optional strong-bibliographic alias and
is the natural cross-source **merge** key when multiple catalogues arrive:
identical ISBNs link catalogue-entry aliases from different connections to the
same Book, while each catalogue-entry alias keeps its connection-scoped
provenance.

## Alternatives considered

- **Treat the ISBN as the stable identity across all sources.** Rejected. It is
  absent or edition-specific for many entries and manual entries, and identical
  ISBNs could surface in different sources without meaning the learner wants them
  merged. It cannot be a required identity key; it is retained only as the
  cross-source merge bridge.
- **Defer entirely under a hard single-catalogue constraint.** Rejected. The
  connection-scanning leak exists with one catalogue today; deferring leaves the
  leak and bakes in a constraint that would need lifting later. A soft posture
  threads the connection now and defers only the multi-connection conflict rules.
- **Treat the connection as provenance, not identity.** Rejected. This retains
  the owner-wide lookup and cannot distinguish two connections sharing an entry
  ID.
- **Dedicated `catalogue_links` table.** Rejected as redundant with the existing
  `book_aliases` catalogue-entry identity.

## Consequences

- Refresh and acquisition resolve a Book's catalogue identity in one lookup and
  walk only the owning connection's feed; they stop scanning every connection.
- A single-connection collision surface disappears: two connections sharing an
  entry ID can no longer merge unrelated Books or overwrite each other's titles.
- The fixtureserver adapter gains a connection-scoped alias surface for the
  refresh and acquisition flows it exercises; its unused reconcile path stays a
  stub.
- Adding the column and tightening uniqueness is a schema change under ADR 0038
  and crosses the repository's human-review boundary; the deterministic backfill
  needs its ownership, idempotency, and rollback documented in the migration
  review.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0038: Schema-change governance](0038-schema-change-governance.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)

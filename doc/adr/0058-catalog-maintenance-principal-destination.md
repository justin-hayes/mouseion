# ADR 0058: Catalog maintenance is a principal destination

Status: **Accepted** · Date: 2026-09-11 · Author: Justin + opencode

## Context

Catalog maintenance currently lives at `/connections`, but that route is not a
navigation destination. It is reachable only from My Books empty states and from
a `book_id` deep link. Once a learner's My Books collection is non-empty, there
is no persistent way to reach the connection to edit credentials, inspect sync
status, or remove it. ADR 0041, the Catalogue Sync feature, the canonical
information architecture, and the screen inventory all deliberately keep catalog
maintenance off primary navigation: sync "does not create a learner destination",
and adding "a fourth destination" is an explicit non-goal.

That posture under-serves the actual lifecycle. Catalog maintenance is recurring
work — a failed sync, a rotated credential, an upstream URL change, or removal —
not a one-time setup step. The catalog connection is a genuine learner-owned
resource under ADR 0024, and hiding its only home behind an empty-state call to
action is a discoverability gap rather than a design virtue.

The repository also uses two spellings for the same concept: design prose and
`CONTEXT.md` mostly say *catalogue*, while the canonical term table and much of
the code say *catalog*. A learner-facing destination forces the question.

## Decision

The authenticated shell gains a fourth principal destination, **Catalogs**,
placed last after Vocabulary. It is a peer of My Books, Reading Journey, and
Vocabulary, not an account-area or secondary link.

- **Route.** `GET /catalogs` is canonical. `GET /connections` becomes a
  permanent redirect to `/catalogs`, preserving query parameters (`book_id`,
  `message`, `error`) and connection mutation paths so existing deep links and
  the My Books "Add books" flow keep working.
- **Scope.** The Catalogs screen owns connection create, read, edit, and delete,
  per-connection status (**Never synced**, **Syncing**, **Last synced**, **Sync
  failed**), and **Sync now**. "Resync" is the existing idempotent Sync now
  operation, not a distinct destructive re-download.
- **Boundaries.** Per-book metadata refresh remains a My Books row action
  because it is contextual to the Book; operational attempts, retry,
  cancellation, and errors remain on `/jobs`, linked from Catalogs. Catalogs does
  not become a second browse surface for synced metadata.
- **My Books.** My Books remains the sole browse surface for synced metadata and
  remains scoped to the active study language. No catalog facet, source filter,
  or per-catalog grouping is added.
- **Multiplicity.** The plural name is honest: the surface renders a list of N
  connections and no cap is enforced. The single-catalog posture of ADR 0044
  remains a soft expectation for deployment, not an invariant; the
  multi-connection conflict rules stay deferred there.
- **Terminology.** **Add books** is retired as a canonical term. The destination
  and screen are **Catalogs**; the My Books empty state keeps its onboarding
  copy but its primary action points at the Catalogs destination.
- **Spelling.** **catalog** (American) is canonical for learner-facing copy and
  the glossary. Go identifiers, package names, command names, and immutable
  shipped migrations are out of scope; migrations retain *catalogue* as
  historical filenames under ADR 0038. A mechanical identifier rename, if wanted,
  is separate work.

## Alternatives considered

- **Keep catalog maintenance supporting-only, adding a persistent secondary
  link.** Rejected: the maintenance is recurring and learner-owned, so it
  deserves a real home; a persistent link in the account cluster would be less
  discoverable than a destination without avoiding the fourth-nav-item cost.
- **Keep `/connections` as the route and point the "Catalogs" label at it.**
  Rejected: a first-class destination with a mismatched URL is a permanent
  papercut; the redirect preserves compatibility at low cost.
- **Enforce a single connection and name the destination "Catalog".** Rejected:
  there is no product need for a hard cap, the code already lists N, and a soft
  posture keeps the future multi-catalog decision about conflict rules rather
  than about re-shaping the surface.
- **Add a catalog facet to My Books.** Rejected: it cuts against ADR 0050's
  single-axis active-language scoping and the bibliographic-identity-first row
  hierarchy. Catalog remains provenance, not a browse axis.

## Consequences

- The "no fourth destination" non-goals in the information architecture, the
  screen inventory, and the Catalogue Sync feature are superseded by this
  decision and must be reconciled rather than left as live contracts.
- The authenticated shell renders four destinations; the webapp navigation
  context gains a Catalogs value in place of the unused acquisition context.
- `/connections` redirect behavior, including query-parameter preservation,
  needs test coverage; existing tests and docs that name `/connections` should
  move to `/catalogs`.
- `doc/product.md`, `doc/design/information-architecture.md`,
  `doc/design/screen-inventory.md`, `doc/design/terminology.md`,
  `doc/features/catalog-sync.md`, `doc/features/collection-browsing.md`, and
  `CONTEXT.md` are reconciled with this decision.
- Historical ADRs and feature documents keep their original *catalogue* spelling
  as records; this decision does not rewrite history.

## Related

- [ADR 0024: Learner-owned catalogs and removal of the admin role](0024-learner-owned-catalogs-no-admin.md)
- [ADR 0041: Catalogue sync is metadata-first and non-destructive](0041-catalog-sync-metadata-first.md)
- [ADR 0044: Catalogue-entry alias identity retains the catalogue connection](0044-catalogue-entry-connection-scoped-identity.md)
- [ADR 0050: The app works in one active study language at a time](0050-active-study-language.md)
- [Catalogue Sync feature](../features/catalog-sync.md)
- [Information architecture](../design/information-architecture.md)
- [Screen inventory](../design/screen-inventory.md)

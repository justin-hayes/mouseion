# ADR 0045: Book detail is addressed by owner-scoped Book ID, with source IDs resolving in place

Status: **Accepted** · Date: 2026-09-05 · Author: Justin + opencode

## Context

ADR 0035 declares the owner-scoped Book ID canonical, yet the shipped web route
`GET /books/{id}` runs a dual identity regime. The handler first asks
`IsMetadataOnlyMyBook` and treats the id as a **Book ID** for the metadata-only
page; otherwise it scans the owner's source materials and treats the id as a
**source-material ID**. The identity therefore changes with evidence state.

Two producers emit links that break under this regime:

- the **language view** per-book rows link `/books/{BookID}` for analyzed books
  (ADR 0042's per-book spread), but those books are acquired, so the route
  falls into the source-ID scan and returns 404;
- the **acquisition return path** redirects to `/books/{BookID}` after the
  acquired book stops being metadata-only, so the post-acquisition page 404s.
  The integration test asserts the `Location` header but never follows it.

The fixtureserver masks both defects: it stores `Book.ID == Source.ID`, so the
mismatch is invisible to the browser smoke tests, and the two adapters
(PostgreSQL, fixtureserver) disagree about identity rather than substitute.

## Decision

**Book detail is addressed by owner-scoped Book ID.** `/books/{BookID}` is the
one stable URL for a Book, whatever its evidence state; the Book's current
acquired source and analysis evidence are resolved underneath it. Historical
source-material-ID links continue to land on the Book.

- A single owner-scoped read, `GetBookDetail(owner, id)`, lives at the
  persistence seam and returns the complete My Book (Book + current acquired
  evidence + evidence state). It accepts a Book ID or a source-material ID and
  resolves either to the same Book; cross-owner and unknown IDs fail as
  not-found at this seam.
- Legacy source-ID URLs are **resolved in place** — the route renders the Book
  at the requested URL rather than issuing a canonical redirect.
- The route's POST actions (`analyze`, `deck/preparations`, `refresh`, scope
  review) resolve `{id}` through the same resolution module, so a Book-ID URL's
  actions work exactly like a source-ID URL's.
- The metadata-only and acquired pages remain two templates, now both driven by
  the single `GetBookDetail` result and branched on `EvidenceState`; the
  identity decision disappears from the handler.
- `IsMetadataOnlyMyBook`, `loadBook`, and `loadBookID` are deleted. The webapp
  `Store` interface narrows; Book-detail reads go through one module.
- Learner-facing surfaces (My Book rows, language view, journey,
  analysis-result redirect) emit canonical Book ID. Operational surfaces (jobs,
  job detail, prepared-deck breadcrumbs and status) keep their
  source-material-keyed links, which resolve in place.
- The fixtureserver models distinct Book and source identities so the browser
  smoke tests exercise the real dual-resolution.

## Alternatives considered

- **Canonical redirect** from `/books/{sourceID}` to `/books/{bookID}`.
  Rejected: adds a hop on every legacy link, forces URL mutation, and makes the
  route a rewriter rather than a pure reader. ADR 0040's compatibility redirect
  applies only to the run-specific page, not to Book detail.
- **Keep the dual regime and patch the two broken producers.** Rejected: the
  shallow handler branching stays, and the next producer to forget the rule
  reintroduces the defect.
- **Source-material ID as the route identity.** Rejected: metadata-only Books
  have no source to address, so this cannot be canonical.
- **Resolution in webapp composition** rather than at the persistence seam.
  Rejected: evidence state is already computed in the persistence SQL; keeping
  the read at that seam makes one interface the test surface, and the two
  adapters make the seam real.

## Consequences

- One module owns "given this owner and id, show me the Book"; the O(n) source
  scan per Book request disappears.
- Cross-owner Book IDs, source IDs, and aliases fail at the single seam, so
  ownership validation concentrates there.
- Two adapters (PostgreSQL, fixtureserver) now exercise the same identity
  behavior; the browser smoke tests cover the dual-resolution path.
- Learner bookmarks and shared links are stable across acquisition, analysis,
  and content revisions.

## Related

- [ADR 0035: Separate My Books membership from acquired source provenance](0035-my-books-membership-and-source-provenance.md)
- [ADR 0040: One current analysis per book](0040-one-current-analysis-per-book.md)
- [ADR 0042: Derive a per-language corpus view without a persisted corpus object](0042-derived-language-corpus-view.md)
# ADR 0055: Retire the standalone Book detail route

Status: Accepted · Date: 2026-09-10 · Supersedes: the Book-detail route portions of ADR 0045 and ADR 0054

## Context

Reading Journey is the learner-facing boundary for acquired and analyzed Books.
Keeping `/books/{id}` available after moving the analyzed surface to the Journey
entry would preserve a redundant destination and would make old bookmarks
ambiguous across membership states.

## Decision

- `GET /books/{id}` is retired and returns 404 for every Book state.
- The exact-analysis compatibility route redirects with 303 to `/journey/{bookID}`
  only for an owner-scoped Journey member with a current completed analysis; all
  other references return 404.
- My Books metadata refresh lives at `POST /library/books/{id}/refresh`.
- Journey-entry deck preparation lives at
  `POST /journey/books/{id}/deck/preparations`; the old Book-detail mutation
  routes are retired.

## Consequences

Learner-facing navigation has one analyzed-Book destination, while operational
jobs and exact-analysis references remain available without restoring a second
Book detail page. My Books retains metadata maintenance, and Journey membership
continues to govern access to the entry and its deck-preparation action.

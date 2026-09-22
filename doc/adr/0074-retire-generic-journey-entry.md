# ADR 0074: Retire the generic Journey entry surface

Status: Accepted · Date: 2026-09-22 · Supersedes: the learner-facing Journey-entry and deck-surface portions of ADRs 0040, 0055, and 0072

## Context

The former `/journey/{bookID}` page combined Book identity, Journey
membership, current analysis evidence, vocabulary projections, and deck work.
Reading Journey now renders the Book identity, relationship, current evidence,
forecast, and evidence recovery in one ordered surface. Focused preparation has
its own task and status workflow. Retaining the generic page would create a
competing Book-detail or analysis-result destination and would make old
bookmarks ambiguous.

## Decision

- Reading Journey owns the learner-facing Book identity, Journey and Goal
  relationship, current evidence, forecast, and evidence recovery actions.
- `GET /journey/{bookID}` remains a compatibility bookmark. It enforces the
  existing owner, Book-language, Journey-membership, current completed-analysis,
  and evidence-reachability checks, then returns `303 See Other` to
  `/journey#journey-book-{bookID}`. Invalid, unauthorized, non-member, and
  unreachable Books return 404 and do not render a generic page.
- Exact-analysis compatibility URLs, completed operational analysis links, and
  other retained Book references resolve to the same Reading Journey anchor.
- Deck preparation is owned by the focused preparation task and its status,
  retry, download, and return links. It remains bound to the exact current
  analysis or Goal snapshot.
- Vocabulary investment and highest-impact unknown vocabulary are no longer
  learner-facing Journey presentation. Thresholds, top-unknown data, corpus
  statistics, and other analysis read models remain available to internal
  services and operational consumers.

## Consequences

There is one learner-facing Book destination in Reading Journey and no generic
Book-detail, Journey-item-detail, or analysis-result page. Existing links retain
their reachability and owner isolation guarantees while landing on the canonical
anchor. The focused preparation task remains the only learner-facing deck-work
surface outside the Reading Journey Book context.

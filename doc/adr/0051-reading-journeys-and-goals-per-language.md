# ADR 0051: Reading journeys and primary goals are one per language

Status: **Accepted; forecast and reservation semantics amended by ADR 0072** · Date: 2026-09-07 · Author: Justin + opencode

## Context

Today `reading_journeys` and `primary_goals` are each one row per owner, with a
single shared optimistic-concurrency `revision`. Journey membership mixes Books
across languages, and the advisory route comparison (ADR 0037) responds by
deriving an implicit language from the first comparable member and marking
everything else "different study language" — the model already assumes a
journey is single-language without being able to say so. Under ADR 0050 the app
is language-partitioned; a Reading Journey is a provisional pool of Books the
learner imagines reading *in one language*, and a Primary Goal is the commitment
to finish one of them. A single mixed pool contradicts that.

## Decision

- **Reading Journey identity becomes (owner, study language).** Each language's
  Journey owns its order and its own `revision`; reordering or membership
  changes in German no longer affect the Italian Journey's revision. Membership
  references the language Journey. A Book with a chosen language joins its
  language's Journey; an unknown-language Book joins none.
- **Primary Goal identity becomes (owner, study language).** One Goal per
  language, each promoted from that language's Journey member with a successfully
  completed current analysis (ADR 0049's promotion rule, evaluated per language),
  cleared when that member leaves its Journey. How many Goals are active across
  languages is the learner's own discipline, not an enforced invariant.
- **Journey creation is lazy.** A language's Journey row materializes on first
  membership and disappears when its last member leaves and the language ceases
  to be derived.
- **The route comparison (ADR 0037) becomes language-correct by construction.**
  The projection runs against the Journey's own language; the "different study
  language" incomparable reason disappears.
- **Migration splits the legacy journey.** The single existing Journey is split
  into per-language Journeys with positions recomputed per language; existing
  Goals adopt their Book's language. Structural DDL and the data backfill are
  separate migrations under ADR 0038, documented, idempotent, and retry-safe.

## Alternatives considered

- **Keep one Journey row, filter membership by Book language for display.**
  Rejected: the shared `revision` and global positions make languages leak into
  each other — reordering one language changes another's revision, and position
  arithmetic spans the pool.
- **Keep one global Primary Goal across languages.** Rejected: reintroduces the
  cross-language commitment the partition removes and contradicts
  per-language Journeys.

## Consequences

- Schema change to `reading_journeys` and `primary_goals` primary keys, plus a
  separate data-backfill migration under ADR 0038.
- Journey membership and reorder operations become language-scoped; revision
  conflicts are per language.
- ADR 0034 (journey identity and ordering) and ADR 0036 (goal and graduation)
  are amended; ADR 0037's comparison is simplified.
- The fixture server and Playwright smoke tests must model per-language
  Journeys and Goals.

## Related

- [ADR 0034: One implicit Reading Journey with learner-canonical ordering and campaign-queue migration](0034-reading-journey-identity-ordering.md)
- [ADR 0036: Deck-independent Primary Goal and single justified vocabulary-graduation transition](0036-primary-goal-justified-graduation.md)
- [ADR 0037: Cross-book vocabulary projection and advisory Journey ordering](0037-cross-book-projection-advisory-ordering.md)
- [ADR 0049: Reading intent triggers analysis](0049-reading-intent-triggers-analysis.md)
- [ADR 0050: The app works in one active study language at a time](0050-active-study-language.md)
- [Feature: Language Mode](../features/language-mode.md)

## Open questions

- **Campaign reservation stays owner-wide for now.** ADR 0036's internal
  `learning_campaigns` constraint remains `one_active_per_owner`
  (`migrations/000021_learning_campaigns.up.sql`): at most one Campaign reserves
  vocabulary for a learner at a time, even though Goals may now be active in
  parallel languages. This keeps deck-based vocabulary work a single stream
  across languages — a de facto discipline consistent with "how many parallel
  streams you can handle." Whether a per-language campaign
  (`one_active_per_language`) is ever warranted remains open; overlapping
  reservation is already impossible across languages because vocabulary is
  language-keyed, so the ADR 0036 determinism rationale would survive such a
  change.

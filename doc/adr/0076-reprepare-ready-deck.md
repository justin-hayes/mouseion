# ADR 0076: Roll forward when a ready deck requires re-preparation

Status: **Accepted** · Date: 2026-09-22 · Author: Justin + opencode

Amends the source-analysis deduplication consequence of [ADR 0071](0071-decouple-deck-data-from-presentation.md).

## Context

ADR 0071 correctly keeps a ready artifact and its frozen specification
immutable when a presentation-only rerender is possible. When a required render
input is absent from both the specification and the immutable corpus, however,
the existing ready preparation cannot produce a usable replacement. Returning
that same preparation for a re-preparation request leaves the learner with a
ready-looking artifact that cannot be downloaded.

## Decision

Re-preparation is a roll-forward exception to same-analysis deduplication:

- A re-preparation request is valid only for the owner-scoped preparation and
  its exact source material, completed analysis run, and Goal snapshot (when
  present).
- A learner may explicitly request re-preparation of any current Ready
  preparation, including to apply newer Meaning evidence and contextual-Gloss
  rules. This creates a new generation; it does not refresh the old one.
- The current preparation is retired as a lifecycle record; its specification,
  artifact, provenance, and download remain unchanged.
- A new preparation row is created with the same source-analysis identity and,
  for a Goal, the same immutable snapshot identity. The unique analysis
  identity applies only to the unretired current row.
- Repeated requests against the superseded row resolve to the current
  preparation and live job rather than creating another generation.
- The focused task and Reading Journey call this state **Re-preparation
  required** and offer recovery. They never change reading state, Known
  vocabulary, Reserved vocabulary, or Goal identity.

Presentation-only rerender remains deduplicated on the existing preparation as
defined by ADR 0071; this decision applies only to the explicit
re-preparation-required sentinel or a learner's explicit request to prepare a
new generation. A rerender never performs lexical lookup or translation.

## Consequences

Historical preparations can share one exact analysis run, so the analysis
identity index is partial on `retired_at IS NULL`. Old APKG bytes remain
downloadable by their owner-scoped preparation ID, while current Journey and
Goal lookups select only the unretired generation. The replacement uses the
same analysis and snapshot provenance and does not re-run analysis or weaken
owner isolation.

## Alternatives considered

- **Return the ready preparation again.** Rejected: it exposes a truthful error
  but offers no path to a usable replacement.
- **Overwrite the ready row.** Rejected: it destroys historical artifact and
  provenance identity and makes concurrent or bookmarked status views unsafe.
- **Re-run analysis.** Rejected: the current completed analysis is the exact
  source of the replacement and remains the provenance boundary.

## Related

- [ADR 0071: Decouple prepared-deck data from presentation](0071-decouple-deck-data-from-presentation.md)
- [Prepared-deck rendering](../features/prepared-deck-rendering.md)

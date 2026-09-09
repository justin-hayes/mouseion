# Retire the standalone analysis action

Status: Accepted · Date: 2026-09-09 · Supersedes: learner-facing action portions of ADR 0049

## Context

The metadata-only Book detail page duplicated the acquisition and analysis intent
already expressed by Reading Journey membership. It also made a Book appear to
have a learner-facing detail surface before current completed evidence existed.
This split encouraged analysis as an isolated operation even though analysis is
durable Book evidence and Reading Journey is the learner's reading-intent
boundary.

## Decision

- Metadata-only, queued, running, failed, cancelled, stale, and otherwise
  unassessed Books have no learner-facing Book detail page.
- Adding a Book to Reading Journey remains the initial acquisition-and-analysis
  trigger. My Books rows retain metadata refresh, add/remove membership, and
  explanatory state for Books without current evidence.
- A Book detail page is available only for a current completed analysis. Stale
  evidence is re-analyzed from the Journey entry; there is no standalone
  **Start analysis** action.
- The exact-analysis compatibility route redirects Journey members to their
  Journey entry and other valid completed Books to Book detail. Unknown or
  unauthorized references remain 404.

## Consequences

Learners encounter one clear acquisition-and-analysis decision at the point of
reading intent. My Books remains useful for catalogue maintenance without
inventing a detail page for incomplete evidence. Existing durable analyses and
operational job history are preserved, while stale analysis recovery is anchored
in Reading Journey.

# ADR 0083: Keep Concordance server-rendered and consolidate on HTMX 4

Status: **Accepted; server-rendered Concordance slice implemented, HTMX 4 migration pending** · Date: 2026-10-03 · Author: Justin + OpenCode

## Context

Concordance already renders complete, usable KWIC results in Templ. Its former
Lit island serialized the same occurrences into JSON and rendered a second list.
Lookup requests, history, pending state, and recovery are handled by separate
handwritten JavaScript. Maintaining two result renderers, their styling, and a
bundle pipeline is disproportionate to the client-side state needed. Mouseion
currently serves HTMX 2.0.7 for enhancement elsewhere.

## Decision

Remove the Lit Concordance island and its duplicate JSON/bundle path. Render
one result list in Templ, with native GET forms, links, 25-result paging, and
`<details>`/`<summary>` KWIC disclosures that remain usable without JavaScript.
The row summary expands its context on click and supports native Enter/Space
activation; keep the separate study link tabbable. Use a shared disclosure
`name` for single-open rows where supported, including without JavaScript;
multiple open rows in other browsers are acceptable. Remove custom Up/Down and
Escape row-key shortcuts, not native summary keyboard access.

Prefer an application-wide upgrade to pinned, locally served HTMX 4.0.0 to
enhance Concordance lookups: use HTMX for requests, swaps, URL history,
synchronization, and pending indication. Use status-specific `hx-status` swaps
for real HTTP errors (including changed evidence and timeouts) so old results
retain their applied label while only a recovery region changes; do not push
the failed query. A narrowly scoped `hx-on` handler may show network-failure
recovery, since no server response exists to swap. Do not add Alpine or a
second client-side results list. Keep distinct recovery for changed evidence,
server timeout, and network failure. On sentence-study return, pass a
server-readable occurrence target as well as the fragment: focus that
occurrence when present, otherwise the Current results summary.

## Consequences and delivery

Keeping Lit preserves duplicate rendering and custom lookup code; HTMX 2 plus
Alpine avoids an app-wide upgrade but adds a runtime for exceptional failure
and focus handling. HTMX 4's status routing favors one enhancement library,
but changes error swapping, attribute inheritance, event names, and history
restoration across the app. Audit all existing HTMX uses rather than adding
global HTMX 2 compatibility settings. Ship the upgrade and Lit removal in one
PR with separable commits and full-app browser regression coverage. If this
needs extensive compatibility code, stop and reconsider HTMX 2 with narrowly
scoped Alpine rather than forcing the upgrade.

The first implementation slice removes the Lit island, duplicate JSON payload,
bundle/build pipeline, and custom row-key controller while keeping the existing
request/history implementation temporarily. The [Vocabulary feature contract](../features/vocabulary-browse-concordance-and-custom-decks.md)
defines the learner-facing behavior. This ADR's HTMX 4 request migration and
combined-delivery requirement remain pending.

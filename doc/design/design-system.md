# Design system

Mouseion does not yet have a mature component library or complete token system.
The current frontend is server-rendered with Templ, enhanced with HTMX, based on
Pico CSS, and supplemented by a small application stylesheet. This document
records the present baseline and the rules that apply until reusable foundations
and components are specified incrementally.

## Current baseline

- `internal/webapp/views.templ` owns the application shell, pages, fragments,
  and inline enhancement scripts.
- `internal/webapp/static/app.css` contains application-specific layout and
  status styles.
- Pico supplies base typography, forms, spacing, cards, tables, and color
  variables.
- HTMX enhances catalog browsing, acquisition, language selection, and known-
  vocabulary status interactions.
- Native HTML controls and landmarks provide the default keyboard and
  accessibility behavior.

Pico is a foundation dependency, not the Mouseion design system. A Pico class or
variable is not automatically a durable product pattern.

## Established experience rules

Until component documentation is expanded, new frontend work must follow these
rules:

1. Reuse native elements and existing Templ patterns before creating a new UI
   primitive.
2. Keep book identity, learner-facing state, and the next action above
   operational identifiers and provenance.
3. Present loading, empty, error, disabled, success, degraded, historical, and
   asynchronous states explicitly.
4. Use the canonical labels in [`terminology.md`](terminology.md).
5. Preserve correct server-rendered content before JavaScript enhancement.
6. Use text or an accessible name in addition to color for every state.
7. Prefer restrained borders, spacing, and typographic hierarchy over decorative
   cards, gradients, shadows, or oversized headings.
8. Keep reading samples and book content visually stronger than application
   chrome when both appear.
9. Avoid page-specific variants of an established status, alert, action group,
   summary metric, or empty state.
10. Document a reusable visual or interaction decision here when implementation
    establishes it.

## Existing proto-patterns

The current implementation repeats several patterns that should be treated as
candidates for formalization rather than as fully specified components:

- application shell and primary navigation;
- page heading with supporting copy and an optional action;
- back link or breadcrumb;
- status label;
- alert, notice, and live status message;
- empty state;
- action group;
- resource list/card;
- definition-list metadata;
- summary statistic and threshold group;
- asynchronous operation status, retry, cancel, and download;
- responsive data table;
- consequential action confirmation.

Before extracting or restyling one, inspect all existing occurrences and define
its purpose, content rules, variants, states, keyboard behavior, and responsive
behavior.

## Phase 1 interaction decisions

The information architecture establishes several interaction contracts before
component extraction begins:

- My Library, Learning, and Settings are destinations; Add books is a persistent
  global action and should not look like an unselected fourth destination.
- Operational analysis status and the exact completed analysis result are
  separate surfaces. A completed status links to the result; it does not place
  deck preparation ahead of insights.
- The action that completes a learning campaign is a consequential confirmation
  with an outcome-based label. Abandonment uses a separate confirmation with
  different consequences.
- Settings is the canonical surface for study languages and known vocabulary;
  retained compatibility routes do not establish separate visual patterns.
- Server-rendered initial state and error recovery remain coherent before HTMX
  or custom JavaScript runs.

Future component work should derive navigation, result summary, confirmation,
and asynchronous-status patterns from these contracts rather than treating each
workflow as a visual exception. See
[`information-architecture.md`](information-architecture.md) and the workflow
documents indexed in [`README.md`](README.md).

## Foundation work not yet established

Future design-system phases should define:

- a pinned or locally served dependency strategy for Pico and HTMX;
- Mouseion-owned semantic color, surface, border, spacing, type, and readable-
  width tokens mapped onto the base framework;
- application, bibliographic, reading-text, metadata, and numerical type roles;
- responsive transformations for the shell, headings, cards, tables, progress,
  and deeply nested scope content;
- shared Templ components for the proto-patterns above;
- interaction-state and feedback conventions;
- automated accessibility and visual/regression fixtures appropriate to a
  server-rendered application.

Do not introduce speculative tokens or a broad component library before a real
workflow requires them. Establish patterns through the core book-analysis-deck
workflow, document them, and then reuse them in acquisition, learning, and
settings.

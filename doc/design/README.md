# Mouseion product design

This directory is the durable source of truth for Mouseion's cross-feature user
experience and reusable interface rules. It describes how accepted product
behavior is organized and presented to learners; it does not replace feature
specifications or architecture decision records.

## Document responsibilities

Use the narrowest durable artifact that owns the decision:

- [`../product.md`](../product.md) summarizes the present product and pipeline.
- [`../features/`](../features/) defines feature behavior, motivation, scope, and
  acceptance requirements.
- [`../adr/`](../adr/) records significant product-boundary and architecture
  decisions and their rationale.
- This directory defines current information architecture, workflow continuity,
  terminology, visual and interaction principles, and reusable UI rules.
- GitHub issues define bounded implementation work. Planning alternatives and
  unaccepted exploration remain outside the repository, as described by
  [`../documentation-governance.md`](../documentation-governance.md).

When documents disagree, follow the precedence in [`../../AGENTS.md`](../../AGENTS.md)
and surface a material conflict rather than silently choosing one.

## Index

### Direction

- [`principles.md`](principles.md) — project design priorities.
- [`experience-direction.md`](experience-direction.md) — desired product
  character: a digital scholarly reading desk.

### Product structure

- [`information-architecture.md`](information-architecture.md) — shipped
  **My Books / Reading** architecture and navigation, plus the explicitly
  marked accepted Vocabulary target and its peer views.
- [`terminology.md`](terminology.md) — canonical learner-facing language.
- [`screen-inventory.md`](screen-inventory.md) — canonical screen goals,
  transitions, states, and current-route compatibility notes.
- [`roadmap.md`](roadmap.md) — historical record of the completed first design
  rollout; it is not the plan for implementing the frozen architecture.

### Workflows

- [`workflows/acquisition-to-library.md`](workflows/acquisition-to-library.md) —
  catalog setup, local My Books browsing, and intent-driven acquisition and
  analysis.
- [`workflows/book-analysis-and-deck.md`](workflows/book-analysis-and-deck.md) —
  the core current-analysis-to-deck lifecycle, including explicit refresh and
  To Read's ensure-once analysis trigger.
- [`../features/reading-workflow.md`](../features/reading-workflow.md) — shipped
  Inbox, To Read, current-reading, completion-history, and rereading workflow.
- [`../features/vocabulary-browse-concordance-and-custom-decks.md`](../features/vocabulary-browse-concordance-and-custom-decks.md)
  — accepted (not yet shipped) Browse, Concordance, and Custom deck contract.
- [`workflows/learning-campaign.md`](workflows/learning-campaign.md) — historical
  record of the retired Reading Journey / Primary Goal model; not current product
  behavior.
- [`workflows/study-languages-and-known-vocabulary.md`](workflows/study-languages-and-known-vocabulary.md)
  — study-language ownership, capability degradation, and vocabulary import.

Additional workflow documents should be added only when a cross-screen journey
has durable rules that cannot be understood from the screen inventory and its
feature specification.

### Historical design evidence

- [`corpus-campaign-horizon-discovery.md`](corpus-campaign-horizon-discovery.md)
  preserves the discovery reasoning that preceded the frozen architecture. Its
  Corpus / Campaign / Reading Horizon learner model is superseded.
- [`../../prototypes/stitch/reading-horizon/`](../../prototypes/stitch/reading-horizon/)
  preserves prompts, reassessments, and view synthesis from visual exploration.
  These artifacts explain why decisions were made; they are not specifications.

Historical evidence is not rewritten to resemble the accepted answer. Add a
short status note and a link to the canonical document when an assumption is
superseded.

### System

- [`design-system.md`](design-system.md) — pinned frontend dependencies,
  semantic tokens, typography, spacing, responsive behavior, and accessibility
  foundations.
- [`components.md`](components.md) — reusable Templ component purposes, content
  rules, variants, states, focus, keyboard, announcement, and responsive
  contracts.

## Required workflow questions

Before changing a workflow, document or confirm:

1. What is the learner trying to accomplish?
2. What is the starting state and desired outcome?
3. What is the primary path?
4. What questions must each screen answer?
5. Which existing pattern should be reused?
6. What are the loading, empty, error, disabled, success, historical, and
   degraded states?
7. How does the workflow behave with a keyboard, assistive technology, a narrow
   viewport, delayed JavaScript, or failed enhancement?
8. Which feature document or ADR owns the underlying product behavior?

## Maintenance rules

Update the relevant design document when a frontend change:

- adds, removes, renames, or reorders a primary destination;
- changes an end-to-end workflow or the next action after a state transition;
- introduces or changes learner-facing terminology;
- introduces a reusable visual or interaction pattern;
- changes the meaning or presentation of status, progress, errors, empty states,
  or consequential actions;
- changes responsive or accessibility behavior shared by more than one screen.

Do not copy complete feature requirements into design documents. Link to the
owning feature or ADR and document only the cross-feature experience rule. When
an accepted design direction conflicts with a current contract, record the
mismatch explicitly and require planner/ADR reconciliation; do not silently
invent domain behavior or disguise the mismatch as copy. Keep shipped-state
claims synchronized with implementation, and label target design direction as
such until it ships. Do not use this directory for speculative mockups or
session notes.

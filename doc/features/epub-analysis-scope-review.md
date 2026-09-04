# EPUB analysis scope review

Status: RETIRED / superseded by issue #551 · Updated: 2026-09-04

This document is retained as historical implementation context. The learner-facing scope review was removed by issue #551; analysis now always processes the entire acquired EPUB. Immutable reviewed-scope persistence remains internal provenance. The former
classifier-led review described in [the historical classification record](epub-analysis-classification.md),
[the historical scope-workflow record](epub-analysis-scope-workflows.md), and
[the historical recommendation record](epub-analysis-recommendation-corrections.md)
is superseded. New workflow behavior does not classify units, produce
recommendations, compare prior scopes, or present classifier evidence.

## Purpose

Let a learner choose the readable parts of an acquired EPUB before analysis.
The review is a calm, native checklist. It presents the book's reliable
top-level EPUB 3 table of contents when that structure can be mapped safely to
the persisted extracted units; otherwise it presents those units directly in
flat spine order.

Scope confirmation remains separate from starting analysis. Confirmation saves
one immutable ordered unit selection. A later explicit analysis submission
reloads that selection and sends exactly those persisted units to NLP.

## Workflow

```text
EPUB acquired and extracted
  → reliable top-level EPUB 3 TOC projected, or flat readable-unit fallback
  → every rendered choice starts checked
  → learner checks or unchecks choices
  → learner confirms an ordered unit set
  → immutable reviewed-scope revision is persisted
  → learner explicitly starts analysis for that revision
  → analysis processes the selected persisted units in spine order
```

## Current review behavior

### TOC checklist

When a reliable projection is available, render one checkbox for each ordered
top-level entry in the EPUB 3 navigation document's `nav` whose `epub:type`
contains `toc`. The checkbox label is the entry's non-empty navigation label.
Nested section targets are covered by their top-level parent and never become
separate selectable rows.

Each top-level entry expands to the existing persisted extracted-unit IDs it
covers. The expansion follows spine order. A TOC entry is only a view-level
grouping; it does not create a second durable unit identity or replace the
stable extracted-unit ID, text, offsets, title, or provenance contract in
[EPUB analysis scope](epub-analysis-scope.md).

Every rendered checkbox is checked on initial load. **Check all** checks every
rendered choice and **Uncheck all** clears every rendered choice. The learner
must leave at least one readable persisted unit selected to confirm.

### Reliable top-level TOC projection

The projection uses the EPUB 3 navigation document already parsed under the
existing EPUB resource-resolution and security rules:

1. Find the navigation document's `nav` element whose `epub:type` token list
   contains `toc`.
2. Take its ordered top-level list entries and their labels. A top-level entry
   is one list item at that level; nested list items belong to that entry.
3. For each top-level entry, use its direct link as the entry start. If it has
   no usable direct link, use the first resolvable descendant link in document
   order. Strip the fragment from the href, then resolve the path with the
   existing package-directory-relative EPUB resource rules.
4. Require a non-empty label for every entry. Resolve every start to a readable
   persisted unit in the current immutable snapshot. Starts must be unique and
   strictly increasing in spine order.
5. Require the first start to be the first readable unit. Each entry spans from
   its start through the persisted unit immediately before the next top-level
   start; the final entry spans through the final readable unit.

The result is reliable only when these ranges form a complete, non-overlapping
partition of every readable persisted unit in the snapshot. Missing EPUB 2
navigation support, a malformed navigation document, an absent or unusable
TOC, an empty label, an unresolved link, a start outside the readable snapshot,
duplicate or out-of-order starts, a first start that does not cover the first
readable unit, or any gap/overlap selects the fallback. Nested links may help
locate their parent's start, but nested entries never become controls.

### Flat fallback

If a reliable top-level projection cannot be produced, render one checkbox per
readable persisted unit in flat spine order. Use the current extracted-unit
title contract: the first non-blank normalized `h1` or `h2`, otherwise the
manifest ID. The fallback also uses the existing persisted unit ID and
provenance; it does not invent a navigation identity. Every fallback checkbox
starts checked, and the same **Check all**, **Uncheck all**, and non-empty
selection rules apply.

The fallback is a deliberate readable-unit presentation, not a classifier,
recommendation, hierarchy, or degraded selection policy. A single neutral
notice may explain that the table of contents could not be projected.

## Scope semantics and confirmation

A reviewed scope is an immutable, owner-scoped selection snapshot associated
with one source-content revision and one extracted-unit snapshot. Its canonical
ordered selected-unit references are the readable persisted unit IDs in spine
order. TOC rows are expanded to these references before validation and
persistence.

The browser submits selection references and the snapshot identity, never unit
text. The server reloads the owner-scoped source, snapshot, and units; rejects
unknown, duplicate, foreign, stale, or incorrectly ordered references; and
requires at least one readable unit. The selected unit text is then reloaded
from persistence for analysis in deterministic spine order.

Confirmation creates a new immutable reviewed-scope revision but does not
queue analysis. A separate explicit submission creates the analysis run bound
to that revision. Each confirmation has its own scope identity, including when
the selected units are equivalent to an earlier confirmation. Existing scopes,
analyses, corpora, and selected-unit provenance remain immutable and readable
to backend and operational audit paths. Under
[ADR 0040](../adr/0040-one-current-analysis-per-book.md), completing analysis
for a newer confirmed scope replaces the book's current learner-facing
analysis rather than creating parallel learner result history.

Confirmation retains the existing guarantees:

- authenticated owner scoping for every source, snapshot, unit, scope, and
  analysis lookup;
- source-content revision and extracted-unit snapshot validation;
- canonical ordered-unit serialization and immutable scope history;
- CSRF protection on confirmation;
- stale-snapshot rejection before persistence or analysis submission; and
- separation between confirming a scope and starting analysis.

## Compatibility and history

Existing reviewed scopes and analyses remain readable to backend and
operational audit paths, including their selected-unit provenance. They are not
listed as learner-facing analysis history. Migration 000043 removes the dormant
classifier tables and legacy scope columns; classifier-era metadata is
intentionally not restorable. Legacy/full-text analyses remain explicitly
identifiable to operations, but do not claim the new reviewed-unit provenance.

The stable extracted-unit identity, text, Unicode offsets, title fallback,
source hrefs, resolved paths, and source-location provenance remain defined by
[EPUB analysis scope](epub-analysis-scope.md). This review contract changes how
choices are presented and persisted, not what an extracted unit is.

## Non-goals

- no classifier, recommendation, confidence, or evidence dashboard;
- no recommendation, main-matter, prior-scope, or hierarchy-group presets;
- no nested TOC editor;
- no automatic analysis, reanalysis, or mastery change;
- no EPUB editing or cross-book scope composition; and
- no change to coverage math, the NLP contract, card schema, or deck
  preparation.

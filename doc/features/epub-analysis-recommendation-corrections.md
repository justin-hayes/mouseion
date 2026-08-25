# Phase 5: Correct EPUB analysis recommendations

Status: Implemented · Date: 2026-08-25

## Problem

Phase 4 gives learners structural groups, deterministic classifications, and reusable scope workflows. A real German book exposed a serious mismatch between classification and recommendation:

- nine substantive chapters were classified as `main_matter` at 70% confidence but recommended for exclusion;
- `VORWORT`, `Register`, `Hinweise zu Quellen und Literatur`, and `copyright` were classified as `unknown` and automatically included by the whole-book fallback;
- `Bildnachweis` was classified as `main_matter`;
- generic sentence-density evidence was treated as contradictory to explicit structural labels;
- the page did not explain why a positive main-matter classification produced an exclusion recommendation.

The current behavior can select approximately 47,000 characters of metadata/reference material while excluding approximately 528,000 characters of substantive chapter content.

## Goal

Make automatic recommendations safe, structurally coherent, and explainable while preserving deterministic classifier output and learner override control.

## Non-goals

- No machine-learning classifier.
- No external corpus or language-model lookup.
- No silent modification of immutable classifications, reviewed scopes, or corpus history.
- No automatic deletion of extracted units or historical analysis results.
- No change to the NLP service contract.
- No assumption that every appendix or preface should always be excluded; recommendations remain reviewable.

## Design principles

### Category and recommendation are distinct

`Category` and `Confidence` describe structural evidence. `RecommendedInclusion` is policy. The UI and documentation must show both without implying that category confidence is inclusion confidence.

### Explicit structure outranks generic text shape

Signal precedence should be approximately: EPUB landmarks and `epub:type`; explicit title/navigation markers; repeated heading and path structure; reference/citation density; spine position; generic sentence density.

A preface can contain prose. An index can contain sentence-like entries. Sustained prose must not by itself overturn an explicit structural marker.

### Uncertainty must not become unsafe inclusion

Unknown units should be review candidates, not automatic inclusion by default. Reference-like unknown units should be excluded or prominently flagged. Medium-confidence main matter should remain eligible for inclusion, with review guidance.

## Recommendation policy

The exact thresholds must be encoded in the versioned policy and tested, but intended behavior is:

- high-confidence main matter: recommend include;
- medium-confidence main matter with sustained prose: recommend include or include-with-review;
- explicit front/back/reference material: recommend exclude;
- unknown with reference-density signals: recommend exclude or require review;
- unknown without exclusion evidence: require review, not silent inclusion;
- non-linear/navigation-only resources: exclude;
- whole-book fallback must never automatically include obvious bibliography, index, copyright, or citation-heavy reference units.

If no high-confidence main matter exists, broaden review of plausible main matter rather than promoting every unknown unit.

## Required structural recognition

Recognize, at minimum:

- Roman-numeral headings such as `I. Einleitung` and `II. Die Welt der Paläste`;
- repeated chapter/part heading patterns;
- German image-credit markers: `Bildnachweis`, `Abbildungsnachweis`, `Bildquellen`;
- German reference markers: `Quellen und Literatur`, `Hinweise zu Quellen und Literatur`;
- German copyright markers;
- existing German, Italian, and language-neutral bibliography/index/notes/appendix markers.

## UI requirements

The review page must distinguish classification evidence, recommendation policy, review status, and book-level fallback/degraded-mode state. For example:

```text
Category: main matter
Confidence: 70%
Recommendation: Include — review suggested
Reason: substantive prose in a central chapter-like unit
```

If a fallback is used, show one book-level notice and a unit-specific explanation. Do not repeat a fallback reason as though it were evidence that each unit belongs in the analysis.

Group summaries must distinguish all-group totals from selected totals:

```text
All units: 596,187 characters
Selected: 47,013 characters
```

## Reproducibility

The classifier/policy version must be bumped. Existing classifications and reviewed scopes remain immutable. New imports/reclassifications use the new version; prior analysis history remains inspectable. Existing reviewed scopes must not be rewritten silently.

## Acceptance outcome

For the reported book shape, substantive Roman-numeral chapters are recommended for inclusion; `INHALT`, `ANHANG`, `Anmerkungen`, and `Bildnachweis` are excluded or clearly flagged; `Register`, `copyright`, and `Hinweise zu Quellen und Literatur` are not automatically included merely because no high-confidence main matter exists; `VORWORT` is classified coherently; recommendation explanations make the policy decision understandable; full-book regression tests prevent recurrence.

## Final validation and limitations

The issue 287 regression fixture records the complete observed 21-unit reading
order, including fallback-titled front matter, `VORWORT`, `INHALT`, nine
Roman-numeral chapters, and the complete appendix/reference tail. Under
classifier/policy version `1.4.0`, the nine substantive chapters are the only
automatic selections. They account for 1,333 of the fixture's 1,997
characters, so the recommended scope is dominated by chapter prose rather than
front matter or reference material. Snapshot and page tests pin category,
confidence, recommendation, ordered reasons, group expansion, all-versus-
selected totals, and spine order. The no-high-confidence-main regression also
pins degraded mode: plausible medium-confidence main matter may be included
with review suggested, while unknown units are not promoted by a whole-book
fallback.

Category and confidence remain descriptions of structural evidence, not a
probability that a unit should be analyzed. Recommendation is a separate,
versioned policy decision. Thus a confidently identified preface, appendix, or
index is confidently classified but still excluded by default; a plausible
medium-confidence chapter can be included with a review warning. The scope
page presents both concepts and keeps every checkbox and group control under
learner control.

The rules intentionally cover a conservative set of German, Italian, and
language-neutral structural markers. Unusual headings, missing navigation,
OCR-like text, or publisher-specific package paths can remain unknown or
ambiguous. Learners can include or exclude individual units and evidence-backed
groups, but an override creates a new immutable reviewed scope; it does not
rewrite the stored classification, an earlier reviewed scope, or a corpus that
was produced from that scope. A later classifier version likewise creates new
versioned classifications and cannot silently change historical scope or
corpus provenance.

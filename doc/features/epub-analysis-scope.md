# Phase 1: Preserve EPUB analysis structure

Status: Proposed · Date: 2026-08-25

## Problem

Mouseion currently extracts linear EPUB XHTML resources into one concatenated text stream. It preserves a coarse chapter location, but it does not persist enough structure to let the application explain or control which parts of a book are sent to NLP. As a result, table-of-contents pages, bibliographies, indexes, notes, and other editorial material can enter analysis and vocabulary/deck generation.

## Phase 1 goal

Preserve the EPUB reading-order units and their provenance before analysis. This phase does not yet classify units, offer selection UI, or change which text is sent to NLP. It creates the stable foundation for those later phases.

## Scope

Each extracted unit should preserve:

- stable unit ID derived from the EPUB manifest/spine identity;
- spine order;
- source href or equivalent provenance;
- title/heading when available;
- extracted text;
- character offsets within the assembled book text, if the existing offset model remains appropriate;
- EPUB metadata useful for later classification (`linear`, `properties`, navigation/landmark information when available);
- deterministic serialization suitable for persistence and reanalysis.

The extracted book should retain the ordered unit list while continuing to provide the existing full-text output during the compatibility transition.

## Non-goals

- automatic front/main/back-matter classification;
- user-facing unit-selection controls;
- changing the NLP request payload or analysis scope;
- changing coverage, deck selection, or sentence scoring;
- adding an ML classifier;
- rewriting existing analyzed corpora;
- discarding existing source text or locations.

## Design requirements

1. Preserve current EPUB validation and security behavior.
2. Continue skipping non-linear spine items and navigation XHTML as direct content unless the implementation explicitly records them as metadata-only units.
3. Keep unit ordering deterministic and aligned with EPUB spine order.
4. Preserve enough source provenance to map a selected future unit back to the original EPUB resource.
5. Treat missing headings/title metadata as valid; use deterministic fallback IDs/titles.
6. Handle duplicate or missing manifest metadata without silently merging unrelated units.
7. Keep offsets Unicode-consistent with the existing source-location contract.
8. Make the representation versionable so later classifier/selection decisions can be reproduced.

## Acceptance criteria

- Existing EPUB fixtures still extract the same full text and chapter ordering.
- Extraction returns a non-empty ordered unit list for every readable XHTML spine resource.
- Each unit has a stable ID, order, title/fallback title, text, and source provenance.
- `linear="no"` behavior remains covered by tests.
- EPUB navigation resources are not accidentally analyzed as ordinary reading units.
- Malformed/missing manifest and spine references still fail safely as before.
- Unit offsets and existing sentence/token source locations remain valid.
- The serialized representation has explicit schema/version semantics.
- Tests cover ordinary books, missing titles, duplicate-looking titles, nested EPUB paths, and mixed linear/non-linear spine entries.

## Later phases enabled

- Phase 2: deterministic automatic unit classification;
- Phase 3: user review and unit-selection UI;
- Phase 4: selected-unit NLP analysis and scope-aware coverage/decks.

## Open questions

- Whether to persist extracted units in `source_materials` or a related table;
- whether full text remains canonical or becomes a derived compatibility field;
- how EPUB 3 landmarks and nav labels should be represented without making them required;
- whether unit IDs should be content-addressed in addition to source-addressed.

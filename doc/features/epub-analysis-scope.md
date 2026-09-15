# Phase 1: Preserve EPUB analysis structure

Status: Implemented · Date: 2026-08-25 · Updated: 2026-09-14

> Analysis uses declared-structure main-text selection when the EPUB identifies a
> single safe body-matter run, and otherwise analyzes the complete snapshot. The
> selection is defined by [ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md)
> and the [main text selection feature](main-text-selection.md). The former
> learner-facing scope-review workflow is archived.

## Problem

Mouseion extracts linear EPUB XHTML resources into one concatenated text
stream. It must also preserve enough ordered structure and navigation
provenance for analysis-time main-text selection, while retaining the complete
snapshot when that structure is not safe to use.

## Phase 1 goal

Preserve the EPUB reading-order units, their provenance, and the optional
navigation data used by analysis-time structure selection. The former
scope-review contract is preserved in the [archived scope-review record](../archive/features/epub-analysis-scope-review.md).
This phase does not classify units or change the stable extracted-unit
identity, text, offsets, or provenance. It creates the foundation for
analysis-time main-text selection.

## Scope

Each extracted unit should preserve:

- stable unit ID derived from the EPUB manifest/spine identity;
- spine order;
- source href or equivalent provenance;
- title/heading when available;
- extracted text;
- character offsets within the assembled book text, if the existing offset model remains appropriate;
- EPUB metadata useful for structure selection (`linear`, `properties`, and
  navigation/landmark information when available);
- deterministic serialization suitable for persistence and reanalysis.

The extracted book should retain the ordered unit list while continuing to provide the existing full-text output during the compatibility transition.

## Non-goals

- automatic front/main/back-matter *classification* or *recommendation*
  (declared-structure main-text selection is defined by
  [ADR 0066](../adr/0066-main-text-selection-from-epub-structure.md));
- learner-facing scope-review controls (the former workflow is archived);
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
8. Make the representation versionable so later selection decisions can be
   reproduced.

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

## Current analysis behavior

- extraction keeps the complete ordered unit snapshot; and
- analysis over the declared main-text run, with a whole-snapshot fallback when
  structure is absent or ambiguous.

## Extracted-unit contract (version 1)

The Go representation of this contract lives in `internal/epub/contract.go`.
This section is normative for extraction; persistence layout remains a separate
decision for Phase 1 issue #243.

### Book envelope and versioning

An extracted-unit document is an object with these fields:

- `schema_version`: the integer `1`;
- `units`: an array in ascending spine order.

The schema version describes the meaning and serialization of the whole unit
document, not the EPUB version and not an individual unit. Writers must emit
exactly the current version. Readers must reject an unsupported positive
version rather than guessing its meaning. A missing, zero, `null`, or empty
unit document on a source imported before this contract means **legacy data is
unavailable**; it does not mean that the EPUB has no readable units. Callers
that need units must re-extract the retained source EPUB. This PR does not add
persistence or backfill legacy sources.

JSON field names are the names specified below. Arrays are emitted in their
defined order, and absent optional string arrays are emitted as empty arrays,
not `null`. Unknown fields may be ignored by a v1 reader, but missing required
fields, an unsupported `schema_version`, duplicate unit IDs, invalid order, or
invalid offsets make the document invalid. Serialization does not use map
iteration and therefore is deterministic for the same extracted input.

### Unit identity and order

Each readable content unit contains:

- `id`: `epub-unit-v1:<zero-based spine index>:<manifest idref>`;
- `order`: the zero-based index in the emitted `units` array;
- `spine_index`: the zero-based position of the `itemref` in the package
  document, including positions occupied by skipped entries;
- `title` and `title_source`;
- `text`, `start_offset`, and `end_offset`;
- the source and EPUB metadata described below.

The spine index is formatted as an unpadded base-10 integer. The remainder
after the second colon is the manifest `idref` verbatim. Thus a repeated spine
reference still produces distinct IDs, while a changed display title never
changes identity. Identity is stable for repeated extraction of the same EPUB
rendition; editing its manifest or spine creates a different rendition and is
not required to preserve IDs. IDs are source-addressed, not content-addressed:
two units with identical titles or text remain distinct.

Only readable `application/xhtml+xml` spine resources with `linear` absent,
blank, or other than ASCII-case-insensitive `no` are emitted. Manifest items
whose whitespace-separated `properties` contain `nav` are not emitted as
content units. All other media types are skipped as they are today. `order` is
contiguous (`0..len(units)-1`); `spine_index` preserves gaps caused by skipped
non-linear, navigation, non-XHTML, or empty resources. No file-system or
manifest-map ordering participates.

An XHTML resource whose normalized extracted text is empty does not produce a
unit. If no unit remains, extraction fails with the existing invalid-EPUB
behavior.

### Titles

`title` preserves the current chapter-title behavior:

1. the first non-blank normalized `h1` or `h2` text in the resource;
2. otherwise the manifest `idref` verbatim.

`title_source` is respectively `heading` or `manifest_id`. Blank or malformed
heading content is treated as missing. Titles are display metadata, need not be
unique, and never participate in identity or ordering. Navigation labels are
provenance only and do not alter this v1 fallback, preserving existing
`Chapter` and source-location values.

### Source and EPUB metadata provenance

Each unit records:

- `package_path`: the cleaned ZIP path of the selected OPF package document;
- `manifest_id`: the spine `idref` and matching manifest item ID;
- `source_href`: the manifest item's href exactly as decoded from the OPF;
- `resolved_href`: the cleaned, package-directory-relative ZIP path used to
  read the resource;
- `media_type`: the manifest media type;
- `properties`: whitespace-separated manifest property tokens in source order;
- `linear`: `true` for every emitted v1 content unit;
- `navigation_labels`: labels associated with the resource by a successfully
  parsed EPUB navigation document, in navigation-document order;
- `landmark_types`: landmark `epub:type` tokens associated with the resource,
  in navigation-document order and token order.

`source_href` may be nested (for example `Text/part/chapter.xhtml`), while
`resolved_href` is resolved relative to `package_path`, never the ZIP root by
assumption. Navigation href fragments are removed only for resource matching;
the manifest href fields above are not rewritten. Navigation labels and
landmarks are optional evidence: missing navigation, no match, or malformed
optional navigation metadata yields empty arrays and must not change text,
identity, title, or order. A manifest item marked `nav` remains excluded even
when its navigation metadata is malformed.

Required package metadata keeps the existing fail-safe behavior. A missing or
blank package identifier, missing spine `idref`, missing/blank/duplicate
manifest ID, a spine reference with no unique manifest match, unsafe or
missing resolved resource, or malformed required OPF/XHTML fails extraction as
an invalid EPUB. Implementations must not overwrite duplicate manifest IDs in
a map or silently merge resources. Blank optional title, property, navigation,
and landmark metadata is tolerated as described above.

### Text and offsets

`text` is exactly the normalized XHTML text contributed by that unit to
`FullText`. `start_offset` and `end_offset` form a half-open span in Unicode
code points (Go runes), not UTF-8 bytes or UTF-16 code units, into the complete
`FullText`. Therefore:

```text
[]rune(FullText)[start_offset:end_offset] == []rune(unit.text)
```

Offsets include all preceding unit text and the existing two-newline joiner.
The joiner lies between unit spans and belongs to neither unit. The first unit
starts at zero; subsequent units start at the prior `end_offset + 2`; the last
unit ends at the Unicode length of `FullText`. Text normalization and joining
remain unchanged in v1.

### Compatibility fields

During the transition, `ExtractedBook.FullText` remains the canonical text sent
to NLP and must be byte-for-byte identical to current extraction. The new unit
list does not change analysis scope.

`ExtractedBook.Chapters` also remains available. For every unit, in order, its
compatibility chapter is projected as follows:

- `Chapter.ID = unit.manifest_id` (not the new unit ID);
- `Chapter.Title = unit.title`;
- `Location.SourceDocumentID = ExtractedBook.SourceIdentifier`;
- `Location.Chapter = unit.title` and `Location.Section = unit.title`;
- location offsets equal the unit offsets.

This preserves existing `Chapter` and downstream `SourceLocation` values,
including duplicate display titles. New code should use unit IDs for identity;
legacy chapter titles remain display/location labels only.

## Persistence layout (Phase 1)

Fresh EPUB imports store a normalized, owner-scoped snapshot linked to
`source_materials`. The snapshot records the envelope schema version and its
rows record every v1 unit field and retain historical selection metadata for
provenance. Composite foreign keys include
both owner and source-material identity, and unit identity and order are unique
within a snapshot. Reimport replaces the snapshot transactionally while
leaving existing corpora untouched. Sources imported before this persistence
was introduced have no snapshot and therefore report extracted units as
unavailable. Their retained `full_text` remains compatible with current NLP.
Analysis uses the persisted snapshot when available and never changes the
identity of its units.

## Phase 1 fixture and compatibility guarantees

The readable source fixtures under `internal/epub/testfixtures` are packaged
into deterministic EPUB byte streams by the tests. They lock down these cases:

- `epub2-clean` proves EPUB 2 package metadata, NCX/non-XHTML skipping, spine
  order, stable IDs, and the existing `FullText` and `Chapters` projections;
- `epub3-edge-cases` uses a nested package and resource tree to prove skipped
  `linear="no"` front matter, excluded `nav` content, preserved manifest
  properties, navigation labels and landmarks, missing-heading fallback,
  duplicate display titles, bibliography-like content, unmatched navigation
  targets, and rune-based offsets across German, Japanese, and emoji text;
- `epub3-main-text` proves that linear front matter, a declared `bodymatter`
  start, and a terminal `bibliography` landmark survive extraction and import
  before analysis selects only the declared main-text units;
- `invalid-missing-spine-reference` proves a missing manifest target remains an
  invalid EPUB rather than being silently omitted; table-driven extraction
  tests retain coverage for malformed XML, blank and duplicate manifest IDs,
  blank references, missing resources, and unsafe resource paths;
- `epub3-reimport` shares the first EPUB 3 fixture's source identifier and
  proves that reimport transactionally replaces, rather than appends to, the
  owner-scoped extracted-unit snapshot.

The extraction assertions compare every v1 field, not only unit count or text.
Persistence integration compares the complete round-tripped snapshot with that
extraction result and verifies replacement, owner isolation, and cascade
deletion. A source written through the legacy import path still returns
`ErrExtractedUnitsUnavailable` while retaining its `FullText`; fresh imports
continue to store the source EPUB and use the unchanged full text as the NLP
compatibility input. Bibliography-, index-, navigation-, or landmark-like
metadata is preserved as provenance for structure selection. Main-text analysis
consumes the declared landmarks when declared structure identifies a main text,
while the stored snapshot and compatibility full text remain complete.

## Deferred decisions

- A future schema version may make full text derived, but v1 keeps it canonical.
- A future version may add content hashes as secondary identity; v1 IDs remain
  source-addressed.

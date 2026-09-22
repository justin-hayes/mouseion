# Prepared-deck rendering and presentation versioning

Status: Proposed · Date: 2026-09-15

## Motivation

Prepared decks already separate data from presentation in practice: a durable
manifest freezes the selection and render inputs, optional translation lives in
an immutable shared cache, and a finalizer renders the APKG from that data
(see [durable prepared-deck translation](durable-prepared-deck-translation.md)).
But the boundary is implicit and unversioned, so a presentation defect cannot
reach an existing deck:

- render logic, note field order, the Anki model, and the template/styles are
  hard-coded with no version, so a deck cannot be recognized as stale;
- `render_payload` is outside the manifest digest, so adding a render input
  does not change the manifest schema version and an incomplete specification
  is indistinguishable from a complete one — the separable-verb bolding defect
  was exactly a render input that looked frozen and was not;
- a `ready` artifact is byte-immutable and reused, and re-preparing the same
  book returns the existing preparation, so there is no way to regenerate an
  existing deck at all.

The goal is to make "re-render the deck from its existing data" a supported,
cheap, offline operation, so presentation fixes apply without re-analysis,
re-selection, dictionary re-resolution, or provider translation.

## Goal

Make rendering a pure function of a frozen deck specification, an exact
enrichment overlay, and a versioned presentation contract; and let a stale deck
be regenerated from its specification by a background job, replacing its
artifact in place without changing Goal-owned vocabulary or historical
provenance state.

## The seam

**Deck specification.** The durable manifest is the presentation-independent
data for one deck: selected vocabulary identity, representative sentence and
target, syntax (dependency parse), morphology, dictionary forms, source
document, and the exact enrichment cache identity.

**Enrichment overlay.** The effective meaning and translation fields are the
exact immutable `enrichment_cache` rows addressed by the specification's cache
identity. They are replayed at render and are not part of the specification.

**Render inputs.** Rendering consumes an explicit frozen boundary — the
specification plus the enrichment overlay — and never reads live NLP output, the
dictionary index, or a provider. The boundary is a type, so a renderer cannot
reference a field that was not frozen.

## Versions

Two independent versions are recorded on the durable run and carried to the
artifact:

- **`RenderInputVersion`** — the set of inputs frozen into the specification.
  It changes whenever that set changes.
- **`PresentationVersion`** — the rendering rules: bolding and headword logic,
  note field order, the Anki model definition, and the embedded template and
  stylesheet.

A deck is **presentation-stale** when its stored `PresentationVersion` is behind
the current one, and **input-stale** when its stored `RenderInputVersion` is
behind.

## Regeneration

When the current `PresentationVersion` advances, unretired `ready` decks are
marked stale and re-rendered by a background job. Re-rendering:

- reads only the specification and the exact enrichment overlay;
- performs no analysis, selection, dictionary lookup, or provider call;
- is idempotent on `(run_id, presentation_version)`;
- supersedes the artifact in place and advances the preparation's monotonic
  deck revision.

An input-stale deck is re-renderable only when its missing inputs can be read
from the immutable per-analysis corpus; otherwise it is reported as requiring
re-preparation. Re-preparation is an explicit roll-forward exception: the old
owner-scoped artifact remains historical and downloadable, while one new
current preparation is created for the same source and analysis run (or Goal
snapshot). Repeated recovery resolves to that current generation.

## Recovering inputs from the corpus

A manifest item records the corpus identity and sentence ordinal of its
representative sentence. A re-render may read the immutable normalized corpus
for an input the specification did not itself freeze. This is reading persisted
analysis, not re-running NLP. Manifests created before corpus coordinates exist
fall back to re-preparation when an input is missing.

## Learner experience

A deck whose artifact has been superseded is presented as having an updated
version available; the learner re-downloads and re-imports it into Anki. The
Anki note type, model id, and note GUID are unchanged, so re-import updates the
existing notes in place rather than forking a parallel deck. Goal vocabulary
state is untouched: current reservation and graduation reference vocabulary
identity and the Goal snapshot, not card bytes. Historical prepared deck
timestamps remain available for provenance.

## Scope and projections

- All unretired `ready` preparations are eligible for regeneration.
- A re-render regenerates the `cards` projection so it does not drift.
- The TSV remains unpersisted and unserved; the download contract stays
  APKG-only.

## Non-goals

- Re-running analysis, selection, dictionary resolution, or provider
  translation during a re-render.
- Changing a deck's vocabulary identity, representative sentence, ordering, or
  Goal-owned vocabulary state.
- Persisting or serving the TSV.
- Backfilling corpus coordinates for manifests created before this feature.

## Validation

- A specification round-trips through durable storage and re-renders to an
  artifact that bolds every component of a separable verb.
- A presentation-version bump marks a `ready` deck stale, re-renders it without
  any provider call, and advances the deck revision.
- Re-render is idempotent on `(run_id, presentation_version)`: a repeated run
  does not change the artifact.
- An input-stale specification with corpus coordinates recovers the missing
  input; one without them is reported as requiring re-preparation.
- Re-importing a re-rendered package updates the same Anki notes and does not
  change Goal snapshot or Reserved-vocabulary state.
- A deck reported as requiring re-preparation keeps its old artifact and
  provenance, and recovery creates a new current preparation without re-running
  analysis.

## References

- [ADR 0071: Decouple prepared-deck data from presentation](../adr/0071-decouple-deck-data-from-presentation.md)
- [ADR 0030: Durable prepared-deck translation runs](../adr/0030-durable-prepared-deck-translation.md)
- [ADR 0059: Persisted normalized corpus for future concordance](../adr/0059-persisted-normalized-corpus-for-concordance.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](../adr/0060-persist-dependency-parses.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](../adr/0067-recognition-card-morphology-presentation.md)
- [ADR 0068: Recognition-card meaning and form presentation](../adr/0068-recognition-card-meaning-and-form-presentation.md)
- [Durable prepared-deck translation](durable-prepared-deck-translation.md)
- [Recognition-card sentence presentation](recognition-card-sentence-presentation.md)

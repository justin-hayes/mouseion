# ADR 0071: Decouple prepared-deck data from presentation

Status: **Accepted** · Date: 2026-09-15 · Author: Justin + opencode

Amends the byte-immutable ready artifact of **ADR 0030** and the
roll-forward-only delivery posture of **ADR 0067** and **ADR 0068**.

## Context

A prepared deck is already built from a durable, frozen plan: selection and
render inputs are persisted on the manifest, optional translation lives in the
immutable enrichment cache, and the APKG is produced by a finalizer that reads
only that data (ADR 0030). In other words, **rendering is already deferred and
already a function of persisted data** — the seam between the deck's data and
its presentation exists, it is just implicit.

What is missing is the seam as a contract:

- **Presentation is unversioned.** Target bolding, the headword line, note field
  order, the Anki model, and the embedded template and styling are hard-coded in
  the renderer with no version. Nothing records which presentation produced a
  given artifact, so nothing can recognize a deck as stale.
- **Render inputs are not provably complete.** `render_payload` is not part of
  the manifest digest, so adding a render input (the dependency parse, in the
  separable-verb bolding fix) did not bump `ManifestSchemaVersion`. The input
  looked frozen and was not: the deck was prepared, the parse was absent, and
  the card silently degraded to boldening only the observed form. The schema
  version cannot answer "does this specification have what the renderer needs?"
- **A ready artifact cannot be superseded.** Byte-equality idempotency, a
  re-freeze guard, a coordinator that never replans, a finalizer that refuses
  completed runs, and a manifest-mutation trigger all fence the finished deck.
  Re-submitting the same book reuses the existing preparation, so even
  "re-prepare the deck" cannot reach an already-ready deck.
- **The corpus is frozen but unused.** The per-analysis normalized corpus
  (ADR 0059/0060) is immutable and holds the syntax a renderer may need, but the
  manifest does not record where in it a card's sentence lives.

The result is a class of presentation defects that can be fixed in code and
still never reach a learner's existing deck.

## Decision

### The manifest is the deck specification

The durable manifest — selection identity, representative sentence and target,
syntax, morphology, dictionary forms, and the exact cache identity — is the
**deck specification**: the presentation-independent data from which a deck is
rendered. Rendering is a pure function of the deck specification, its exact
enrichment overlay, and the presentation version. It never re-runs selection,
NLP analysis, dictionary resolution, or provider translation.

### Render inputs are explicit and complete

Rendering consumes a frozen input boundary (the specification plus the exact
enrichment overlay) rather than the mutable assembly type used to build a
specification. A renderer cannot reach a field that was not frozen, because the
boundary does not expose one. Adding a render input is a deliberate extension of
the frozen input set, not an incidental field addition.

### Two orthogonal versions

- **`RenderInputVersion`** identifies the set of inputs frozen into a
  specification. It changes whenever the frozen input set changes.
- **`PresentationVersion`** identifies the rendering rules: bolding and
  headword logic, note field order, the Anki model definition, and the embedded
  template and stylesheet.

Both are recorded on the durable run and carried to the artifact, so a deck's
presentation and its input completeness are separately comparable to the
running code.

### Automatic regeneration from existing data

A `ready` deck whose stored `PresentationVersion` is behind the current one is
stale and is re-rendered by a background job from its existing specification —
no new analysis, selection, dictionary lookup, or provider call. A stale deck
whose stored `RenderInputVersion` is behind is re-renderable if the missing
inputs can be read from the immutable per-analysis corpus; otherwise it is
reported as requiring re-preparation rather than silently rendered incomplete.

### Recover from the immutable corpus

A manifest item records the corpus identity and sentence ordinal of its
representative sentence. A re-render may read the immutable normalized corpus
for an input the specification did not itself freeze. This is reading persisted
analysis, not re-running NLP; a missing input the corpus cannot supply means
re-preparation.

### Supersede the artifact, keep the specification

Manifests stay immutable. The **artifact may be superseded in place** by a
re-render, and idempotency is keyed on `(run_id, presentation_version)` rather
than artifact bytes. A monotonic **deck revision** is tracked per preparation so
a learner can be told a newer presentation exists.

### Stable Anki identity

The Anki note type, model id, and note GUID stay keyed to the note type and card
identity, not the presentation version. Re-importing a re-rendered package
updates the learner's existing notes in place instead of forking a parallel note
type.

### Study state is untouched

Graduation and reserved vocabulary reference vocabulary identity and the
deck-snapshot, never card bytes, so a re-render cannot change vocabulary-study
state. Re-render proceeds regardless of an active study; the learner re-imports
deliberately.

### Scope and projections

All unretired `ready` preparations are eligible for re-rendering. A re-render
regenerates the `cards` projection so it does not drift; the TSV remains
unpersisted and unserved (the download contract stays APKG-only).

## Consequences

- A presentation defect becomes fixable on every existing deck by shipping a
  `PresentationVersion` bump; a forgotten render input is diagnosable via
  `RenderInputVersion` instead of shipping as a silently partial card.
- The ready artifact is no longer byte-immutable. Byte-equality idempotency is
  replaced by `(run_id, presentation_version)` idempotency, and the
  manifest-mutation trigger remains in force.
- The schema gains version columns and a corpus-coordinate column on manifest
  items, crossing the migration review boundary.
- A re-render that needs an input absent from both the specification and the
  immutable corpus still requires re-preparation, and re-preparation remains
  deduplicated by source analysis.
- Legacy manifests created before corpus coordinates cannot use corpus
  recovery; they are re-rendered only when their frozen inputs suffice.

## Alternatives considered

- **Keep roll-forward only and force re-preparation.** Rejected: preparation is
  deduplicated by source and analysis run and returns the existing preparation,
  so "re-prepare" does not actually create a new specification and a fix is
  unreachable.
- **One combined version.** Rejected: it cannot distinguish a presentation
  change from a change to the frozen input set, so a missing input would either
  be rendered incomplete or force unnecessary re-preparation.
- **Re-render lazily on download.** Rejected: ADR 0030 makes download a pure
  read that cannot render or mutate state.
- **Fold the presentation version into the Anki model id.** Rejected: it forks
  the learner's note type and orphans their existing cards.
- **Re-resolve dictionary or NLP data at re-render.** Rejected: re-rendering
  must stay deterministic and offline. Dictionary or analysis improvements are
  data changes and belong to a new specification.

## Related

- [ADR 0022: Asynchronous deck preparation and durable APKG artifacts](0022-prepared-decks.md)
- [ADR 0030: Durable prepared-deck translation runs](0030-durable-prepared-deck-translation.md) — its byte-immutable ready artifact is amended here.
- [ADR 0059: Persisted normalized corpus for future concordance](0059-persisted-normalized-corpus-for-concordance.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](0060-persist-dependency-parses.md)
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md) — its roll-forward-only posture is amended here.
- [ADR 0068: Recognition-card meaning and form presentation](0068-recognition-card-meaning-and-form-presentation.md) — its roll-forward-only posture is amended here.
- [Prepared-deck rendering](../features/prepared-deck-rendering.md)

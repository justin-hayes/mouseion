# ADR 0123: Constructions are a derived, versioned reading of a frozen parse

Status: **Accepted — implementation pending** · Date: 2026-11-24 · Author: Justin + OpenCode

Narrows how [ADR 0103](0103-source-structure-and-poetry-evidence.md),
[ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md) and
[ADR 0116](0116-classical-catalog.md) apply to construction results. Decided by
Justin in
[Grilling: persistence and versioning of syntax and constructions](https://github.com/justin-hayes/mouseion/issues/2606#issuecomment-7098921699)
and [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608#issuecomment-7106323004),
under [Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597).
The inventory, definitions and surfaces are in
[Ancient Greek syntax and constructions](../features/ancient-greek-syntax-and-constructions.md).

## Context

[ADR 0122](0122-ancient-greek-native-agdt-syntax.md) stores AGDT relations and
heads verbatim and normalises them at read time. Construction detectors run over
that parse. Their precision is interim and will improve: the first two
measurements changed three of six definitions, and the shipping gate itself
switches constructions and subtypes on and off.

ADRs 0103, 0107 and 0116 say new evidence is a new version, adopted only by
learner confirmation, and that frozen evidence is never rewritten. Applied
literally to constructions, every precision fix would be an Analysis update,
and each update would cost the learner a confirmation.

Three shapes were considered:

- **(A) Detector version in the run identity.** Every detector tweak becomes an
  Analysis update.
- **(B) A derived, rebuildable table** keyed by run and detector version, like
  `vocabulary_browse_counts` and its `vocabulary_browse_count_readiness`
  builder version.
- **(C) Pure query-time detection**, like `sentencequality`. It cannot serve
  Concordance parallels across a language without scanning every parse, and it
  would leave two code paths.

Frozen artifacts pin a run (for example
`primary_goal_snapshots.analysis_run_id` and
`classical_prepared_definitions.analysis_run_id`), and Sentence study is
addressed by an explicit run. Prepared decks carry no constructions.

## Decision

### The parse is evidence; constructions are a reading of it

The frozen evidence is a run's **parse**: tokens, lemmas, morphology, relations,
heads and unprinted syntax nodes. It is never rewritten. Construction
occurrences are a **derived reading** of that parse, like Browse counts and
concept notes. No card, Known fact, Reserved fact, count, coverage or completion
depends on them.

*A frozen Reading's parse never changes; what Mouseion identifies in it may
improve.*

### A derived table (option B)

- Construction occurrences are stored per run, with a per-run marker holding the
  construction detector version the rows were built under and their readiness.
- The same Go detectors run at preparation time and in a rebuild worker.
- A new `native_agdt` run's rows are built inside the preparation transaction,
  before it publishes.
- Runs whose `syntax_state` is not `native_agdt` get no rows; the builder skips
  them. Their Readings show no constructions, which is silent abstention.

### One construction detector version per syntax scheme

- The **AGDT construction detector version** is one integer. It covers the AGDT
  adapter's normalisation (comma roots, COORD resolution, stray labels), the six
  detectors, the authored verb lists and the **shipping gate**, which says which
  constructions and Subtype labels are on.
- It is bumped by hand and enforced by a synthetic golden-output test that fails
  when emitted occurrences change without a bump.
- A UD scheme (Latin) gets its own version later. There are no per-construction
  versions.

### Rebuild without confirmation

- A detector-version change rebuilds every `native_agdt` run's rows, including
  runs that current-reading snapshots and Classical prepared definitions pin. No
  learner confirmation is asked.
- Only the current version's rows are kept.
- Until a run's marker reaches the current version, Sentence study and
  Concordance show no constructions for it: no stale fallback and no "updating"
  notice. Each run's rows are replaced in one transaction (delete and insert
  under the marker).
- Rebuild jobs run on the `classical_preparation` queue.

### Inputs are the source analysis only

Detectors read the run's source lemma, morphology, relations and heads. Lemma
corrections and Occurrence exclusions never change construction occurrences,
so a correction never triggers a rebuild.

### Presentation is not stored

- Rows hold ids only: no text, name, definition or citation. Concept notes are
  looked up at render ([ADR 0124](0124-grammar-references-and-concept-notes.md)).
- The per-word relation and head disclosure is read through the AGDT adapter at
  render. It is neither stored nor versioned.

### Shipping gate and abstention

- **Bar.** A construction ships when its precision on at least 120 random corpus
  hits is ≥ 0.90 (point estimate) with a 95% Wilson lower bound ≥ 0.84. A
  Subtype label is gated by the same bar on at least 60 hits; a failed subtype's
  occurrences still show, without the label. Recall is reported, not gated.
- **Per construction only**, with no per-occurrence confidence layer.
- **Fail closed and silent.** A detector whose required input is missing, or
  reached only through an unprinted node where its definition asks for a printed
  token, produces no hit. Nothing tells the learner it abstained.
- **First slice.** All six constructions ship. Subtype labels: natural result,
  infinitive indirect statement, and dative and genitive articular infinitive.
  Conditional ships only after the first release-check run confirms it clears
  the bar without the dropped "whether" exclusion.

### Release check

- A committed **reviewed judgment file** holds, by ID only, every item reviewed
  in the two measurements: work, sentence id, OGA anchor id, construction,
  subtype and judgment. No Greek text, relations or heads.
- A **`make` target** recomputes per-construction and per-subtype precision
  against the pinned OGA files (fetched, never committed). Every
  detector-version bump runs it and includes its output in the PR. It is not a
  default CI step. A bump fails if a shipped unit falls below the bar or a
  reviewed item flips without a new judgment.
- A bump that changes more than 10% of a construction's corpus hits needs a
  fresh random sample of 120 hits for that construction.

### Schema brief (ADR 0038)

The exact DDL is settled in the implementation issue's migration, under
CODEOWNER review.

- **Construction occurrences:** owner, run, sentence, `language` (denormalised
  for Concordance, a low-risk additive field with one consumer), construction
  id, optional subtype id (present only when the gate labels it), anchor token
  ordinal, and the extent as a set of token ordinals. Indexed on (owner,
  language, construction id, subtype) for
  [ADR 0125](0125-construction-parallels-in-concordance.md).
- **Per-run marker:** owner, run, detector version, readiness, following
  `vocabulary_browse_count_readiness`.
- Both tables are additive and derived. They can be emptied and rebuilt from the
  parse at any time, so they need no data backfill.

## Alternatives considered

- **(A) Detector version in the run identity:** rejected. Every precision fix
  would be an Analysis update, and before
  [ADR 0126](0126-analysis-updates-carry-decisions-onto-identical-tokens.md)
  each one would have dropped Lemma corrections.
- **(C) Query-time detection only:** rejected. It cannot serve
  language-wide parallels and would leave two code paths.
- **Pinning a detector version per Reading:** rejected. Multi-version storage,
  garbage collection, and no clear version for Concordance, which spans Books.
- **A stale fallback during rebuilds:** rejected. Showing an older version's
  constructions next to newer ones would mix two readings on one page.
- **Applying Lemma corrections to detectors:** rejected. A correction would
  trigger a rebuild, and detection would depend on learner decisions.

## Consequences

- Precision fixes, new constructions and gate changes reach every Book
  without learner action and without touching frozen evidence.
- A learner may see an active Reading gain or lose a construction after a
  release. This is intended and is not a frozen-evidence change.
- The golden test and the release check make an unbumped or regressing detector
  change fail loudly.
- Concordance parallels can query one indexed table.

**What would reopen this:** constructions feeding a frozen learner artifact
(cards, counts, difficulty, deck selection). They would then become evidence
and need per-Reading pinning.

## Related

- [ADR 0122](0122-ancient-greek-native-agdt-syntax.md): the stored parse.
- [ADR 0124](0124-grammar-references-and-concept-notes.md): concept notes
  looked up at render.
- [ADR 0125](0125-construction-parallels-in-concordance.md): the Concordance
  query over these rows.
- [ADR 0087](0087-vocabulary-counts-come-only-from-selection.md): the Browse
  projection whose readiness pattern this follows.
- [ADR 0062](0062-derived-sentence-quality-scoring.md): a derived, unstored
  scoring, the option (C) precedent.

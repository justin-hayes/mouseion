# ADR 0122: Ancient Greek syntax is stored as native AGDT behind a construction interface

Status: **Accepted — implementation pending** · Date: 2026-11-24 · Author: Justin + OpenCode

Amends [ADR 0099](0099-ancient-greek-imported-analyses.md)'s decision that
"syntax is recorded as absent, not stored as UD". Decided by Justin in
[Grilling: choose the Ancient Greek syntax source and representation](https://github.com/justin-hayes/mouseion/issues/2603#issuecomment-7098557008),
under [Wayfinder: Ancient Greek syntax and construction identification](https://github.com/justin-hayes/mouseion/issues/2597).
The feature is specified in
[Ancient Greek syntax and constructions](../features/ancient-greek-syntax-and-constructions.md).

## Context

[ADR 0110](0110-philological-reading-competence-boundary.md) wants classical
syntax and constructions in Sentence study and Concordance. Mouseion stores none
for Ancient Greek. ADR 0099 recorded every imported run as
`syntax_state = 'absent'` with a `dependency = 'dep'`, `head = 0` skeleton,
because the `corpus_tokens` fields are read as UD and a native AGDT head could
not be stored in a 0-based printed-token index once ellipsis nodes are dropped.
ADR 0099 reserved `native_agdt` for a later feature.

The pinned OGA v0.2.0 files carry AGDT syntax. A profile of all 7 Works
(14,978 sentences, 251,406 tokens) found clean trees: every head resolves, with
no cycles. It also found that OGA has no `_CO`/`_AP` suffixes, that 12.7% of
sentences have a comma root, that 23 tokens carry non-guideline labels (4 with a
Greek-capital-Zeta `AuxZ`), and that ellipsis nodes are 2.6% of nodes and head
7% of printed tokens. Dropping them leaves 18.6% of sentences with more than one
root ([profile](https://github.com/justin-hayes/mouseion/issues/2598#issuecomment-7098250031)).

Three alternatives were measured:

- **Convert AGDT to UD at import.** No public converter fits OGA. Celano's is
  unpublished; Treex/HamleDT (GPL v2) needs the `_CO`/`_AP` suffixes and
  ellipsis markup OGA dropped, and UD Perseus simply omits every elliptical
  sentence. Conversion merges five distinctions the construction detectors need
  (OCOMP, ATV/AtvV, PNOM, AuxY/AuxZ, non-first conjunct functions), and its
  conjunct guess is exact for only 82.6% of coordinations on gold
  ([conversion research](https://github.com/justin-hayes/mouseion/issues/2599#issuecomment-7098298122)).
- **Parse with Stanza `grc`.** Discards OGA's gold-trained trees, brings the
  NLP service into OGA imports, and is non-commercial on every package.
- **Store both.** The conversion's cost and risks without added fidelity.

Storing UD as `syntax_state = 'ud'` would also switch on the generic UD
relation display in Sentence study for Greek by accident.

## Decision

**Native AGDT, read through a source-neutral construction interface.**

1. **Syntax state.** A Greek run from the new OGA interpretation version records
   `syntax_state = 'native_agdt'`. Syntax needs a new interpretation version
   whatever its source. It is adopted through the learner-confirmed Classical
   Analysis update of [ADR 0116](0116-classical-catalog.md); no new mechanism is
   needed. The interpretation version is already the run's `analyzer_version`.
2. **The scheme marker is per run.** `syntax_state` names the scheme of every
   stored relation and head in a run. There is no per-token scheme column. The
   proto and domain comments that call `dependency` a UD relation are corrected
   to say it is read according to the run's syntax state.
3. **Relations are stored verbatim.** Relations and heads are stored as the
   source has them, except plain encoding defects corrected at import (the
   Greek-Zeta `AuxZ` becomes the Latin-script `AuxZ`). Comma roots, COORD
   resolution and stray labels are normalised at read time, in the AGDT
   adapter.
4. **Unprinted syntax nodes.** Ellipsis nodes are stored as unprinted syntax
   nodes: relation, head and tree position, keyed by their stable source id
   within the sentence. They have no printed form, lemma, morphology,
   vocabulary identity or Annotation occurrence, consistent with that term in
   `CONTEXT.md`. A printed token's head may be an unprinted node.
5. **Printed spans are unchanged.** Sentence and token ordinals and offsets
   still come from `reconstruct`, which skips unprinted nodes. The new
   interpretation version must not change `reconstruct` or the
   sentence-inclusion rules, so every printed token keeps its source span.
   [ADR 0126](0126-analysis-updates-carry-decisions-onto-identical-tokens.md)'s
   carry-over depends on this.
6. **Morphology remap.** The same interpretation version maps the VerbForm,
   Aspect, perfect-tense and common-gender values that `oga-agdt-upos/5`
   drops, following UD Perseus r2.19.
7. **UD consumers stay off for Greek.** The gates in Sentence study and
   Concordance keep treating `native_agdt` as "no UD syntax". No generic UD code
   path reads AGDT labels. AGDT relations reach learners only through the AGDT
   adapter: the construction layer and the plain-English Word relations
   disclosure. Any other reader of `dependency` (today every Greek token carries
   `dep`) must produce the same Greek output as before.
8. **A source-neutral construction interface.** Construction detectors read a
   small interface over nodes, heads, normalised relation categories,
   morphology and source lemma. Each syntax scheme has one adapter. The AGDT
   adapter is first; a UD adapter can be added for Latin or a future Greek
   parser without changing the interface ([ADR 0110](0110-philological-reading-competence-boundary.md)).

### Schema brief (ADR 0038)

The exact DDL is settled in the implementation issue's migration, under
CODEOWNER review. The shape:

- **`analysis_runs`:** unchanged. The `native_agdt` value is already allowed by
  `analysis_runs_syntax_state_check`.
- **`corpus_tokens`:** `dependency` holds the AGDT relation. A token headed by a
  printed token records that token's ordinal in `head`. A token headed by an
  unprinted node references that node (for example through a nullable column
  beside `head`), and its `head` value is never read as UD because the run
  state gates it.
- **Unprinted syntax nodes:** a new table, one row per node: owner, run,
  sentence, the stable source id, relation, and a head that is a printed token,
  another unprinted node, or the root. Additive; no backfill. Existing runs keep
  `absent` and have no rows.

## Alternatives considered

- **AGDT → UD at import:** rejected on fidelity, ellipsis and licence (see
  Context).
- **Stanza `grc`:** rejected on fidelity and licence; out of scope while the
  graded-reader path is paused.
- **Dual store:** rejected; no fidelity gain over native AGDT.
- **Drop ellipsis nodes and re-attach their dependents:** rejected. 18.6% of
  sentences would become multi-root, and constructions over an elided
  predicate would be misread.
- **Normalise relations at import:** rejected. Stored evidence would encode one
  adapter version; read-time normalisation keeps the frozen parse faithful and
  lets [ADR 0123](0123-constructions-are-a-derived-reading.md) version the
  reading of it.
- **A per-token scheme column:** rejected; a run has one scheme.

## Consequences

- ADR 0099's "Native AGDT syntax is not stored and not shown" is superseded for
  runs of the new interpretation version. Its `absent` runs, and its reasoning
  for them, are unchanged.
- The `CONTEXT.md` **Imported analysis** entry no longer says every consumer
  shows "Syntax unavailable" for `native_agdt`.
- Each of the 22 Classical Reading-unit definitions gets the new
  `interpretationVersion` and a revision bump, which offers an Analysis update to
  existing Greek Books. Old runs show no constructions until the learner
  confirms it.
- The licence posture is unchanged: relations, heads and constructions are
  Analysis-resource output under [ADR 0111](0111-third-party-licence-posture.md),
  already named in the register and `layers.json`
  ([licence research](https://github.com/justin-hayes/mouseion/issues/2602#issuecomment-7098154703)).
  Test fixtures stay synthetic and should use the real AGDT label inventory.
- Latin is not blocked: it can arrive as UD (Stanza `la`, UD treebanks) or
  AGDT-style (LDT) and adds an adapter, not a redesign.
- Migrations and this ADR are human-reviewed under CODEOWNERS.

**What would reopen this:** a public converter licensed CC BY-SA or more
permissively that keeps ellipsis and coordination; a fixed requirement that
Latin and Greek share one stored relation scheme; or a preference for generic UD
display over construction precision.

## Related

- [ADR 0099](0099-ancient-greek-imported-analyses.md): the import seam and the
  `absent` decision this amends.
- [ADR 0116](0116-classical-catalog.md): the Classical Analysis update that
  adopts the new interpretation version.
- [ADR 0123](0123-constructions-are-a-derived-reading.md): how constructions are
  read from the stored parse.
- [ADR 0038](0038-schema-change-governance.md) and
  [ADR 0070](0070-migration-and-documentation-reboot.md): schema governance.

# ADR 0062: Sentence-quality scoring derived from the persisted corpus

Status: **Accepted** · Date: 2026-09-12 · Author: Justin + opencode

Amends **ADR 0029** (the representative-sentence selection mechanism).

## Context

ADR 0020 deferred sentence-quality scoring to a follow-up; the export gate in
card export scores a candidate sentence from its text, target surface, and
source location only. It cannot tell a genuine clause from a well-formed
fragment, so a fragment can be exported when it is the only candidate for a
lemma. ADR 0029 retained deterministic representative-sentence selection with
the complete source sentence preserved verbatim.

The normalized corpus and per-token dependency parses are now persisted at
analysis time (ADR 0059, ADR 0060). Sentence quality can therefore be derived
from the persisted data without re-running NLP, as the dependency-parse
foundation's GDEX-readiness criterion anticipated. The GDEX (Good Dictionary
Examples) rubric provides a deterministic, rule-based scorer to adapt.

## Decision

- **Sentence quality is a derived view computed at export time over the
  persisted corpus**, not stored at analysis time. Rubric changes apply
  retroactively to existing analyses on re-export, and `selection_candidates`
  stays schema-stable. Card export gains a batch read path from candidate
  sentence ordinals into the persisted sentences and tokens.
- **The export gate gains a finite-verb-and-subject knock-out** (the sentence
  root is a finite verb with a subject dependent), preserving the GDEX knock-out
  semantics. The existing length, boundary, target-presence, location, and
  structural-noise criteria are unchanged.
- **The GDEX gradual score ranks accepted candidates**: target-in-subordinate-clause
  penalty, deixis penalty, named-entity-density penalty, and an optimal-length
  window, with source order breaking ties so exports stay stable.
- **German only, first pass**; Italian follows later with its own deixis term
  list.
- **The winning sentence's score and reasons are recorded on the prepared-deck
  manifest** per card for diagnostics.

## Consequences

- Card export reads the persisted corpus (a new read path into
  `corpus_sentences`/`corpus_tokens`), computing the rubric as a pure,
  deterministic function over tokens, dependencies, target, and German language
  resources.
- Rubric improvements are free: re-exporting an existing book re-scores without
  re-analysis.
- Sentences lacking a finite verb and subject are rejected outright; a lemma
  whose candidates all fail is omitted and remains eligible under the existing
  omission policy.
- First-encounter card ordering is unchanged; the chosen sentence is still the
  complete source sentence, never truncated or paraphrased (ADR 0029).
- The prepared-deck manifest contract gains a per-card quality score and reasons
  field.

## Alternatives considered

- **Analysis-time scoring persisted into the candidate's sentence references.**
  Rejected: rubric changes would require re-analysis, and it would churn the
  `selection_candidates` reference schema; the persisted corpus is the single
  source of truth the derived view should read.
- **GDEX as ranking-only, gate unchanged.** Rejected: a well-formed fragment
  could still be exported as the only candidate; the syntax knock-out is exactly
  what distinguishes a real clause.
- **Replacing the gate with a single 0–1 rubric.** Rejected: the existing gate's
  tuned criteria and the omission policy are tested and preserved.
- **Including the VulGer blacklist or Italian in the first pass.** Deferred:
  German-only data asset and a language-specific term list respectively.

## References

- [Sentence-quality scoring feature](../features/sentence-quality-scoring.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](0060-persist-dependency-parses.md)
- [ADR 0029: Recognition-card sentence presentation](0029-recognition-card-sentence-presentation.md)
- [ADR 0020: Anki package output](0020-anki-package-output.md)
- [Dependency Parse Foundation feature](../features/dependency-parse-foundation.md)
- [GDEX – Good Dictionary Examples](https://github.com/zentrum-lexikographie/gdex)
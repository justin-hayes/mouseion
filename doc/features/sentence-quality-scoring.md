# Sentence-quality scoring (GDEX-informed)

Status: Implemented · Date: 2026-09-12 · Updated: 2026-09-15

## Motivation

The recognition-card export gate (`ScoreSentenceQuality` in card export) scores
a candidate sentence from its text, target surface, source location, and, for
German candidates, persisted dependency data. It can reject fragments without a
finite verb and subject and rank accepted examples by a deterministic GDEX-style
score. ADR 0020 deferred sentence-quality scoring; this feature implements it.

The GDEX (Good Dictionary Examples) rubric
([zentrum-lexikographie/gdex](https://github.com/zentrum-lexikographie/gdex))
addresses exactly this problem with a deterministic, rule-based scorer: knock-out
criteria that gate a sentence outright, plus gradual criteria that score it.
With the normalized corpus and dependency parses now persisted at analysis time
([ADR 0059](../adr/0059-persisted-normalized-corpus-for-concordance.md),
[ADR 0060](../adr/0060-persist-dependency-parses.md)), the two syntax-based checks
are computable without re-running NLP — the GDEX-readiness criterion from the
dependency-parse foundation.

## Goal

A deterministic, explainable sentence-quality rubric informed by GDEX, computed
at export time over the persisted corpus, that (i) adds finite-verb-and-subject
as a knock-out in the export gate and (ii) ranks accepted candidate sentences by
a gradual GDEX-style score (target-in-subordinate-clause, deixis, named-entity
density, optimal length). German-specific GDEX resources are first pass; Greek
and Italian candidates use the generic deterministic rubric.

## Scope

- **Derivation at export time**: card export reads the candidate sentences'
  tokens and dependency structure from the persisted corpus (sentence ordinal
  ↔ `sentence_ordinal`), batches them, and scores on the fly. Rubric changes
  apply retroactively to existing analyses on re-export.
- **Gate**: `ScoreSentenceQuality` gains finite-verb-and-subject as a rejection
  reason; the existing length, boundary, target-presence, location, and
  structural-noise criteria are unchanged.
- **Ranking**: the GDEX gradual score ranks accepted candidates; source order
  breaks ties, so exports remain stable.
- **Manifest**: the prepared-deck manifest records the winning sentence's score
  and reasons per card.

## Non-goals

- Language-specific GDEX resources for Italian (`it`) or Greek (`el`) — the
  generic rubric is available now, while language-specific deixis and other
  resources remain deferred.
- The VulGer obscenity blacklist — a German-only data asset (CC-BY-SA); deferred.
- The DWDS frequency whitelist — ADR 0018 removed global-frequency data.
- Changing card ordering (first-encounter) or the omission policy.
- Truncating or paraphrasing the chosen sentence — ADR 0029 keeps the complete
  source sentence.
- Surfacing quality to the learner; the score is an internal selection signal.

## Requirements

### Rubric (German, first pass)

**Knock-outs (rejection reasons).** The existing gate reasons (too short or
fragmented, too long, target not present as a word, invalid source location,
incomplete sentence boundaries, structural noise or boilerplate) plus:

- **No finite verb and subject**: the sentence root is not a finite verb (UPOS
  `AUX`/`VERB` with finite morphology) that has a subject dependent (`nsubj`,
  `nsubj:pass`, or `csubj`). Computed from the persisted morphology and
  dependency/head data.

**Gradual score (ranking among accepted candidates, 0–1):**

- **Target-in-subordinate-clause**: if every occurrence of the target sits in a
  subordinate-clause subtree (`acl`, `advcl`, `ccomp`, `csubj`), the sentence is
  penalized; a target in the main clause is preferred.
- **Deixis**: presence of German deictic terms (`hier`, `dort`, `jetzt`,
  `gestern`, `heute`, `morgen`, …) and personal pronouns is penalized — deictic
  sentences read poorly out of context.
- **Named-entity density**: proper nouns (`PROPN`) are penalized — examples full
  of names and places are poor general examples.
- **Optimal length**: a token-count window scores best, extending the existing
  "useful context window" notion.
- **Rare characters**: characters outside a normal typographic set are
  penalized; control characters remain a knock-out.

**Determinism.** The rubric is a pure function over (sentence tokens,
dependencies, target, language resources); it is stable for a fixed analysis and
stable across repeated exports.

### Export integration

- The winning sentence is chosen per lemma as before (`BestSentenceEvidence`),
  with the GDEX gradual score as the primary ranking among accepted candidates,
  then source order. First-encounter card ordering and the omission policy are
  unchanged.

### Manifest

- Each card's manifest entry records the winning sentence's quality score and
  reasons, so "why this sentence?" is diagnosable and omitted/degraded cards are
  explainable.

## Acceptance criteria

- [x] A sentence without a finite verb and subject is rejected by the export gate
- [x] Among accepted candidates, main-clause targets rank above
      subordinate-clause targets; non-deictic, entity-light, well-length
      sentences rank above their opposites
- [x] The ranking is deterministic and stable across repeated exports
- [x] First-encounter card ordering is unchanged
- [x] A lemma with no acceptable sentence is omitted and remains eligible
- [x] The manifest records the winning sentence's score and reasons

## References

- [ADR 0062: Sentence-quality scoring derived from the persisted corpus](../adr/0062-derived-sentence-quality-scoring.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](../adr/0060-persist-dependency-parses.md)
- [ADR 0029: Recognition-card sentence presentation](../adr/0029-recognition-card-sentence-presentation.md)
- [ADR 0020: Anki package output](../adr/0020-anki-package-output.md)
- [Dependency Parse Foundation](dependency-parse-foundation.md)
- [Recognition-card sentence presentation](recognition-card-sentence-presentation.md)
- [GDEX – Good Dictionary Examples](https://github.com/zentrum-lexikographie/gdex) —
  the rule-based sentence scorer this rubric adapts

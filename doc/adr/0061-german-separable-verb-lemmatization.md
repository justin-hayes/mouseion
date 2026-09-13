# ADR 0061: German separable-verb lemmatization from dependency data

Status: **Accepted** · Date: 2026-09-12 · Author: Justin + opencode

Amends **ADR 0005** (normalization and identity interpretation). Builds on
**ADR 0060**.

## Context

ADR 0005 defines vocabulary identity as `(language, canonical lemma, POS)`, with
the canonical lemma produced by a versioned, pure German normalization profile
that never mutates the raw lemma. The analyzer (Stanza) lemmatizes separated
German separable verbs as the bare base form (`stehe auf` → `stehen`), so the
canonical lemma — and therefore the learning identity — is the wrong lexeme:
the learner studies *stehen* (to stand) instead of *aufstehen* (to get up).
This is a known Stanza gap ([#1549](https://github.com/stanfordnlp/stanza/issues/1549)).

Dependency parsing is always-on ([ADR 0060](0060-persist-dependency-parses.md)).
The separated particle is identifiable by its dependency relation
(`compound:prt`) to the verb, so the full lexeme is recoverable deterministically
as particle lemma + verb lemma. Separately, roughly a third of separable
particles are tagged `ADV`, inside the content-word allowlist, so particles can
surface as their own vocabulary candidates.

## Decision

- The German normalization profile gains a **separable-verb reattachment step**:
  when a verb token has a `compound:prt` dependent whose surface is in a
  curated German separable-prefix set, its canonical lemma becomes the
  normalized concatenation of particle lemma + verb lemma. The profile version
  bumps. The raw lemma is untouched, and only new analyses are affected
  (roll-forward).
- **Separable particles are excluded from content-word candidates.**
- **German only**; the closed-class prefix set lives beside the German
  normalization table in the NLP producer.

## Consequences

- Separated forms persist the full lexeme; vocabulary identity splits `stehen`
  and `aufstehen` as dictionaries do. Known-vocabulary imports and known-lemma
  matching follow full lexemes.
- Occurrence counts shift: separated instances move off the base lemma onto the
  full lexeme, which can move lemmas across the recurring-vocabulary threshold
  (ADR 0048).
- Particle tokens no longer risk becoming vocabulary candidates.
- The stored German normalization profile version distinguishes the new rule
  set from the previous one.

## Alternatives considered

- **Go post-processing over the persisted corpus.** Rejected: a second writer
  for `canonical_lemma` would let the artifact and the token store diverge; the
  producer already owns normalization.
- **Deprel alone without the prefix guard.** Rejected: historical annotation
  noise ([UD #1163](https://github.com/UniversalDependencies/docs/issues/1163))
  and free-adverb homographs justify a closed-class guard.
- **Diagnostic-only.** Rejected: the `compound:prt` signal is strong and the
  identity defect is real.
- **Italian pronominal verbs and German reflexive verbs in the same pass.**
  Deferred: weaker signals and separate identity decisions.

## References

- [German separable-verb lemmatization feature](../features/separable-verb-lemmatization.md)
- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](0060-persist-dependency-parses.md)
- [Dependency Parse Foundation feature](../features/dependency-parse-foundation.md)
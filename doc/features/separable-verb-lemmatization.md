# German separable-verb lemmatization

Status: Implemented · Date: 2026-09-12 · Updated: 2026-09-15

## Motivation

German separable verbs (trennbare Verben) surface in two forms: attached
(`aufgestanden`, `aufzustehen`) and separated (`ich stehe auf`), where the
particle moves to clause-final position. The Stanza German lemmatizer emits
only the base lemma for separated forms (`stehe` → `stehen`, particle `auf` as
its own token) — a known gap ([stanza#1549](https://github.com/stanfordnlp/stanza/issues/1549)).
Under [ADR 0005](../adr/0005-vocabulary-identity-normalization-ranking.md)
vocabulary identity is the canonical lemma, so the learning identity is wrong:
the learner studies *stehen* (to stand) instead of *aufstehen* (to get up).

Dependency parsing is now always-on ([ADR 0060](../adr/0060-persist-dependency-parses.md)),
which makes the separated particle identifiable: it is a `compound:prt`
dependent of the verb, and the full lemma is recoverable deterministically as
**particle lemma + verb lemma**. This is also a known fix for the Stanza gap.

A second, related defect: roughly a third of separable particles are tagged
`ADV`, which falls inside the content-word allowlist, so a particle such as
*auf* can surface as its own vocabulary candidate from a book full of
*steht auf*.

## Goal

In the NLP producer, separable particles are reattached to German verb lemmas so
the canonical lemma of a separated form is the full lexeme (`aufstehen`), while
the raw lemma stays the analyzer's base form (`stehen`). Separable particles are
also excluded from content-word candidates. Unit and end-to-end fixtures cover
the reattachment and exclusion rules. A committed real-corpus report records
the observed reattachments and spot-check rows in
[`doc/evidence/german-separable-verb-precision.md`](../evidence/german-separable-verb-precision.md).

## Scope

- German (`de`) only, first pass.
- Rule: a verb token with a `compound:prt` dependent whose particle surface is
  in the curated separable-prefix set gets `canonical_lemma` =
  particle lemma + verb lemma, normalized by the existing German post-1996
  profile; the profile version bumps.
- `raw_lemma` is untouched.
- Selection excludes tokens whose dependency relation is `compound:prt`.
- Validation: unit fixtures (attached, separated, multiple particles,
  false-positive traps), an end-to-end analysis test, and a real-corpus
  precision measurement.

## Non-goals

- Italian pronominal verbs (`alzarsi`) — future; the clitic signal is weaker
  and UD lemma policy keeps the clitic out of the verb lemma.
- German inherently reflexive verbs (`sich erholen`) — future; whether `sich`
  is part of the learner's lemma is a separate decision.
- Light-verb constructions (`Rad fahren`) — not reliably detectable from
  dependency structure alone.
- Lexicon-based validation — no external lexicon (ADR 0018).
- Changing the raw analyzer lemma.
- Reworking learner-facing surfaces: this producer change only fixes
  `canonical_lemma` and candidate exclusion. Card presentation of the full
  lemma (bolding every component) is defined by
  [ADR 0067](../adr/0067-recognition-card-morphology-presentation.md).

## Requirements

### Rule

For each sentence, for each verb token `V`, consider its children `D` with
dependency relation `compound:prt` and surface in the curated prefix set. If
any, `canonical_lemma(V)` becomes the normalized concatenation of the particle
lemma(s) and the verb lemma. Multiple particles concatenate in surface order.
If the particle's surface is not in the prefix set, no reattachment — the
deprel alone is not sufficient.

The correction runs on whole sentences in the producer; sentences never span
analysis chunks, so the `compound:prt` context is always intact.

### Guard: curated prefix set

A closed-class list of German separable prefixes (for example `ab`, `an`,
`auf`, `aus`, `bei`, `ein`, `mit`, `nach`, `vor`, `zu`, `weg`, `her`, `hin`,
`durch`, `um`, `über`, `unter`, `los`, `fest`, `zurück`, `zusammen`,
`weiter`, ...). It lives beside the German normalization table in the NLP
producer and is the guard against free-adverb and prepositional homographs.

### Normalization

Reattachment happens as part of `canonical_lemma` derivation; the result passes
through the existing German post-1996 profile (lowercase, explicit historical
equivalences). The German normalization profile version bumps so the stored
profile field distinguishes the rule set. Only new analyses are affected
(environments are recreated from the current baseline rather than upgrading
prior analysis data).

### Selection

A token whose dependency relation is `compound:prt` is never a content-word
candidate: a separable particle is a component of the verb's full lemma, not
its own word.

### Validation

- **Fixtures**: sentences covering attached forms (`aufgestanden`), separated
  forms (`ich stehe auf`), multiple particles, and false-positive traps
  (`Die Tür ist auf` — predicative; `auf dem Berg` — prepositional; `nach wie
  vor` — fixed phrase). Assert the persisted `canonical_lemma`.
- **End-to-end analysis test**: a completed analysis persists the full lexeme
  as `canonical_lemma` with `raw_lemma` unchanged.

## Acceptance criteria

- [x] A separated form persists `canonical_lemma` = full lexeme (`aufstehen`)
      with `raw_lemma` = base (`stehen`)
- [x] Attached forms are unchanged (already the full lexeme)
- [x] False-positive traps do not reattach (surface not in the prefix set, or
      deprel not `compound:prt`)
- [x] Separable particles are excluded from content-word candidates
- [x] The German normalization profile version reflects the change; only new
      analyses are affected
- [x] The precision evidence report records the reattachment count and
      spot-check rows for false-positive review

## References

- [ADR 0061: German separable-verb lemmatization from dependency data](../adr/0061-german-separable-verb-lemmatization.md)
- [ADR 0060: Persist dependency parses in the normalized corpus](../adr/0060-persist-dependency-parses.md)
- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](../adr/0005-vocabulary-identity-normalization-ranking.md)
- [Dependency Parse Foundation](dependency-parse-foundation.md)
- Stanza issue [#1549](https://github.com/stanfordnlp/stanza/issues/1549);
  UD [compound:prt](https://universaldependencies.org/u/dep/compound-prt.html)

# ADR 0113: Recover German separable verbs whose particle is parsed as an adverb

Status: **Accepted** · Date: 2026-10-29 · Author: Justin + OpenCode

Decided in [#2329](https://github.com/justin-hayes/mouseion/issues/2329).
Amends [ADR 0061](0061-german-separable-verb-lemmatization.md); follows the
card presentation of [ADR 0067](0067-recognition-card-morphology-presentation.md)
and the analysis identity of [ADR 0107](0107-analysis-pipeline-identity-and-analysis-updates.md).

## Context

ADR 0061 reattaches a German separable particle to its verb only when the
dependency parse marks it `compound:prt` and its surface is in a curated prefix
set. Stanza sometimes attaches the same particle as an ordinary adverb instead.
*Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.* is analysed as `sieht`
→ *sehen* plus `fern` → *fern* (`ADV`, `advmod`, head `sieht`), so the learner
is offered a *sehen* card for a sentence that uses *fernsehen*. The dependency
relation alone cannot separate that particle from a true adverb such as *fern*
meaning "far" in *Das Dorf liegt fern der Stadt.* The card-quality review in
[#2325](https://github.com/justin-hayes/mouseion/issues/2325) treated the
symptom by omitting a mismatched card; this decision fixes the analysis.

## Decision

- **Explicit evidence, not the relation alone.** The producer gains a reviewed
  table of particle-and-verb evidence whose value is the combined dictionary
  headword. It initially holds *fern* + *sehen* → *fernsehen*. A pair enters the
  table only when the combined form is attested; every future entry needs the
  same review.
- **Recovery conditions.** The particle must be `ADV` with dependency `advmod`,
  its head must be the verb, it must follow the verb, and it must have no
  dependents of its own. The particle's clause-final position may still be
  followed by material in the Nachfeld. The table decides whether the pair is a
  separable verb.
- **The analyzer artifact keeps `advmod`.** The producer changes only the
  verb's `canonical_lemma`; the raw lemma is untouched. The Go analyzer
  boundary normalizes the recovered particle to `compound:prt`, exactly like a
  particle Stanza tagged itself, so selection, lemma review, concordance and
  card bolding treat it as a component of the full lemma (ADR 0067). Keeping
  the producer relation lets the precision measurement count the recovery.
- **Negatives keep the base lemma.** A true adverb of the base verb, or a
  particle-homograph adverb whose verb is not in the table, is not reattached.
- **Measurement.** The precision report counts `advmod` recoveries separately
  from accepted and skipped `compound:prt` observations, and lists each
  recovery as a spot-check row flagged `advmod-recovered`.
- **Roll forward.** The German normalization version moves from 6 to 7. Only
  new analyses change; there is no backfill and existing decks keep their
  frozen evidence.

## Consequences

- *sieht … fern* yields *fernsehen*, not *sehen*, and the particle is bolded
  with the finite verb on the card.
- The producer and the Go boundary disagree about the stored dependency for
  this one recovered relation; the boundary normalizes it once, and the
  producer keeps it for measurement. Every downstream consumer sees the same
  `compound:prt` view.
- A new evidence pair requires a code change and review. The table is
  deliberately not a general adverb rule.
- Books analysed under version 6 are offered an Analysis update under ADR 0107;
  nothing is re-analysed automatically.

## Alternatives considered

- **Reattach every `advmod` whose surface is a known prefix.** Rejected: a
  free adverb such as *fern* in *fern der Stadt* would change the verb.
- **Keep `advmod` in the Go result and teach every consumer a second relation.**
  Rejected: selection, lemma review, concordance and card export would each
  need the same interpretation, and a missed site would leak a particle.
- **Persist a new token field for the recovery.** Rejected: it would need a
  schema change and a second representation of a relation the existing
  `compound:prt` contract already carries.
- **Fix the card by omitting a mismatch only.** Rejected as the durable fix:
  the vocabulary identity remains wrong, so the learner cannot study the word
  the sentence uses.

## References

- [German separable-verb lemmatization](../features/separable-verb-lemmatization.md)
- [ADR 0061: German separable-verb lemmatization from dependency data](0061-german-separable-verb-lemmatization.md)
- [ADR 0067: Recognition-card morphology and multi-span target presentation](0067-recognition-card-morphology-presentation.md)
- [ADR 0107: Analysis pipeline identity and learner-confirmed Analysis updates](0107-analysis-pipeline-identity-and-analysis-updates.md)
- [German separable-verb precision evidence](../evidence/german-separable-verb-precision.md)

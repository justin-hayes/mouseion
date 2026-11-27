# ADR 0140: Ancient Greek and Latin function words leave vocabulary

Status: **Proposed; needs human review** · Date: 2026-11-27 ·
Author: Justin + OpenCode

Specified in [#2780](https://github.com/justin-hayes/mouseion/issues/2780) under
[#2759](https://github.com/justin-hayes/mouseion/issues/2759). It follows the
French precedent in [ADR 0102](0102-french-language-support.md) ("Eligibility
and stopwords") and amends [ADR 0005](0005-vocabulary-identity-normalization-ranking.md)'s
part-of-speech filter.

## Context

ADR 0005 keeps NOUN, VERB, ADJ and ADV and drops function words by part of
speech. Both classical treebanks tag some function words ADV, so they pass.

- **Greek (OGA).** In the live Corpus δέ is ADV 4,138 times, καί 3,098, μέν
  1,638 and οὐ 1,429. In the #2767 evidence run, 11 of the first 50 new words
  were such words, and the pass wrote grammar notes for them, not meanings.
- **Latin (OLA).** The native AGDT tags map to UD in `mapUPOS`: `c` becomes
  CCONJ or SCONJ, `g` PART and `d` ADV. The Latin check tallied canonical lemma
  by tag over 2.65 million tokens of seven Works (OLA v0.2.0, the pinned
  source; the live database holds no Latin yet): Caesar, Sallust, Nepos, Livy,
  Cicero, Pliny and Seneca. Most conjunctions are tagged `c` and never reach
  the filter: *que* (31,643), *sed* (13,878), *aut* (10,850), *autem* (5,221),
  *at*, *namque*, *quia*, *seu*. Others pass as `d` at least sometimes, and the
  same lemma can pass in one sentence and not in the next: *non* (28,854 `d`),
  *ne* (15,790 `d`, 4,820 `c`), *quidem* (5,570), *tamen* (5,563), *quoque*
  (2,515), *etiam* (2,519 `d`, 5,696 `c`), *ergo* (1,457), *haud* (1,033),
  *num* (678), *et* (340 `d`, 53,904 `c`), *nonne* (309), *vel* (258), *vero*
  (85), *igitur* (12), *ve* (12), *nec* (8), *enim* (2), *nam* (1), *atque* (1)
  and *itaque* (1).

## Decision

A closed per-language list of ADV function words leaves vocabulary eligibility
(Study new words, coverage, Prepared deck vocabulary and lemma review, which all
share `OccurrenceEligible`) and joins the language's context stopwords. The
mechanism is French's: `lexical.IsVocabularyStopword` matches a **canonical
lemma plus part of speech ADV**. A content-word homograph, such as Latin VERB
*uero*, stays eligible. Real adverbs and pronouns are not listed. Modern
languages, including Modern Greek, are unchanged.

**Ancient Greek (`grc`).** δέ, καί, τε, μέν, γάρ, οὖν, δή, ἄν, ἀλλά, γε, ἤ, ἄρα,
τοίνυν, μέντοι, and the negations οὐ (with οὐκ, οὐχ), μή (with the unaccented
lemma μη the treebank also emits), οὐδέ and μηδέ. ὡς, οὕτως, νῦν, ἐνταῦθα and
the pronouns αὐτός, οὗτος, τις stay eligible.

**Latin (`la`, canonical lemmas, so v is u).** Only function words the check
found tagged ADV: *et*, *ue*, *uel*, *nec*, *atque*, *enim*, *nam*, *uero*,
*igitur*, *ergo*, *itaque*, *tamen*, *quoque*, *quidem*, *etiam*, *non*, *ne*,
*nonne*, *num* and *haud*. The conjunctions that were always `c` (*que*, *sed*,
*aut*, *autem*, *at*, *ac*, *neque*, *namque*) are not listed, as they never pass
the filter. Subordinators that are only occasionally mis-tagged (*si*, *nisi*,
*ut*, *sive*, with 2 to 116 ADV tokens) and the deictic and manner adverbs
(*nunc*, *ita*, *sic*, *iam*, *tum*, *semper*) stay eligible.

## Consequences

- Greek and Latin new-word batches stop offering these words, and coverage no
  longer counts them. Existing Study material or Learned meanings for them are
  untouched.
- A function word tagged with another eligible part of speech by a parse error
  (for example Greek VERB οὐ, twice in the Corpus) stays eligible.
- Stored Browse counts carry the old projection, so the builder version moves
  from 4 to 5. Startup reconciliation then rebuilds each Book's counts, and
  Study and coverage read the new ones.
- The Latin dictionary-match gate counts eligible occurrences, so a classical
  Work's match rate is now measured without these words.
- The lists are code, so a change is a reviewed change.

## Alternatives considered

- **Match the lemma regardless of part of speech.** Rejected: it would hide
  content-word homographs such as *uero* "I speak the truth".
- **Exclude every ADV in these languages.** Rejected: *nunc*, *ita*, *νῦν* and
  *οὕτως* are vocabulary.

## Related

- [ADR 0005](0005-vocabulary-identity-normalization-ranking.md)
- [ADR 0102](0102-french-language-support.md)
- [Language Support](../features/language-support.md#ancient-greek-and-latin-function-words)

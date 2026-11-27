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
  CCONJ or SCONJ, `g` PART and `d` ADV. The Latin check ran over 828,000 tokens
  of Caesar, Sallust, Nepos and Livy (OLA v0.2.0, the pinned source; the live
  database holds no Latin yet). Conjunctions are tagged `c` and never reach
  the filter: *que* (14,452), *sed* (2,611), *autem*, *enim*, *nam*, *at*,
  *atque*, *aut*, *nisi*, *quia*. The rest pass as `d`: *non* (4,601), *ne*
  (4,788 as `d`, 1,778 as `c`), *tamen* (1,038), *quoque* (1,026), *haud*
  (908), *quidem* (783), *etiam* (314 `d`, 1,088 `c`), *ergo* (110), *et* (81
  `d`, 16,106 `c`), *vel* (32), *vero* (20), *igitur* (12) and *nonne*, *num*.
  A word is therefore tagged by its use, and the same lemma can pass in one
  sentence and not in the next.

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

**Latin (`la`, canonical lemmas, so v is u).** *et*, *que*, *ue*, *ac*, *atque*,
*aut*, *uel*, *nec*, *neque*, *sed*, *at*, *autem*, *enim*, *nam*, *namque*,
*uero*, *igitur*, *ergo*, *itaque*, *tamen*, *quoque*, *quidem*, *etiam*,
*non*, *ne*, *nonne*, *num* and *haud*. Words the check found as ADV are the
ones that matter; the conjunctions tagged `c` are listed because the tags vary
by sentence. Deictic and manner adverbs (*nunc*, *ita*, *sic*, *iam*, *tum*,
*semper*) stay eligible.

## Consequences

- Greek and Latin new-word batches stop offering these words, and coverage no
  longer counts them. Existing Study material or Learned meanings for them are
  untouched.
- A function word tagged with another eligible part of speech by a parse error
  (for example Greek VERB οὐ, twice in the Corpus) stays eligible.
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

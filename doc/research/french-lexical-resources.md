# French Lexical Resources Survey

**Research date:** 2026-10-17

**Ticket:** #2140 (parent map #2138; related decision #2141, not resolved here)

**Scope:** Assess which lexical resources could support learner-facing French
dictionary glosses, noun gender and plural, elision and aspirated `h`,
pronominal verbs, IPA, accents and apostrophes, variants, provenance, licensing,
refresh, and integration. Target is contemporary standard written French with
English contextual glosses. Frequency ranking, audio, and new workflows are out
of scope. NLP package selection (Stanza, spaCy, or other) belongs to the
parallel NLP research ticket and is not decided here.

**Evidence labels:** *Verified* means read from a source or repo file during
this research. *Unverified* means not confirmed; claims marked this way must not
be used as design inputs without checking.

## Executive Recommendation

1. **Use raw English Wiktionary via Wiktextract/Kaikki as the primary French
   dictionary source.** It is already the dictionary pipeline's input file, so
   adding `fr` to the existing derivation needs no new download. It supplies
   lemma, gloss, part of speech, gender, plural forms, and IPA. Its data is
   noisy and must be filtered, and its aspiration labels are not reliable
   enough to drive elision on their own (see below).
2. **Do not implement deterministic French article elision yet.** The
   `haricot` evidence shows the source contradicts itself on aspirated `h`. A
   vowel-initial heuristic is wrong for aspirated `h` words, and an unqualified
   source flag is not trustworthy. Elision needs a curated per-lemma `h` status
   with a documented owner, or an explicit "unknown" state that does not elide.
3. **Treat Lexique 4 as an optional cross-check, not a required source.** Its
   license is verified as CC BY-SA 4.0, but its column layout is unverified
   here, so its utility for inflection or phonology cannot be claimed yet.
4. **Use UD `fr_gsd` as a test fixture source, not as a dictionary.** Its
   license is verified as CC BY-SA 4.0, and it demonstrates elided tokens and
   multi-word tokens (MWTs) that any French tokenizer must handle.
5. **Defer Lefff and Morphalou.** Lefff's maintenance status is unclear from the
   sources read. Morphalou's license and columns are unverified, so it cannot be
   recommended until both are checked.

## Current Repository State

*Verified* by reading the worktree at `6e02d180`:

- `nlp/scripts/derive_dictionary_index.py` sets `LANGUAGES = {"de", "it", "el"}`.
  `article_for` returns `""` for any other language, so French articles are
  empty. Plural and gender logic is language-agnostic. `ipa_for` keeps only
  untagged or `standard` IPA. Normalization is NFC plus casefold for non-German
  languages. Adding `fr` needs no extra download, because the same raw
  Wiktextract file is streamed and filtered by `lang_code`.
- `internal/cardexport/cardexport.go` `nounArticleForms` has no `fr` case, so
  the card article is empty. `bookCoverArticles` also has no French entry.
- `internal/enrichment/lexical.go` `contextStopWords` contains `le` and `la`,
  but not `et`, `est`, `que`, `qui`, `les`, `des`, `du`, `une`, `en`, `dans`,
  `pas`, `se`, `ce`, `il`, or `elle`.
- `internal/canonicalization/canonicalization.go` has no French rule and falls
  back to the language-neutral path.
- `doc/adr/0073-modern-greek-language-support.md` is the precedent checklist
  for adding a language: shared processor set, dictionary index, card article,
  enrichment stopwords, lemma display, and sentence-quality rubric.

## Source Assessment

### 1. Raw English Wiktionary via Wiktextract / Kaikki (primary)

*Verified:*

- Kaikki's French page lists a postprocessed French JSONL of about 556.6 MB,
  marked deprecated. The raw English Wiktionary JSONL is about 23.9 GB (about
  2.8 GB gzipped), dump dated 2026-09-02 and extracted 2026-10-03. A raw
  French-edition extract of about 6.4 GB is listed as work in progress.
  Source: kaikki.org French dictionary index and raw-data pages.
- Wiktextract is MIT-licensed (GitHub README). The Kaikki data citation is
  Ylonen, LREC 2022.
- The `haricot` entry carries gender as the sense tag `masculine` and head
  template `{{fr-noun|m}}`. The plural form has tag `plural`.
- The `homme` entry carries gender `masculine`, plural `hommes`, and the
  category "French terms with mute h". It also has Louisiana-tagged senses.
- The `haricot` IPA `/a.ʁi.ko/` is untagged. A Louisiana IPA variant is tagged
  separately. Current code keeps only untagged or `standard` IPA, so the
  Louisiana variant is correctly dropped.

*Unverified:*

- The exact layout of French pronominal verb entries (for example `se laver`).
  A slug lookup for `se laver` returned 404 in the earlier attempt, and the
  pronominal-verb page was not read in this research pass. The answer on
  pronominal verbs is therefore open.
- The tags on French article entries (`le`, `l'`, `la`).
- The index-size estimate for a French-only derivation. Not measured.
- Whether the Wiktionary-derived data license (CC BY-SA) applies to the
  Kaikki release as shipped. Check before distribution.

**Assessment:** This is the only source here that already flows through the
pipeline and gives gender, plural, and IPA in one file. Its risks are noise in
multi-sense entries, sense-tag-based gender that must be validated, and the
aspiration inconsistency below.

### 2. Aspirated `h` evidence (`haricot` versus `homme`)

*Verified* in the Kaikki entries read during this research:

- `haricot` is listed in both "French terms with aspirated h" and "French terms
  with mute h". Two conflicting category labels appear on one entry.
- `homme` is listed only under "French terms with mute h".

Conclusion: the category label is not a single trustworthy flag. A source that
says both is unsafe as a sole elision input. `haricot` is a traditional
aspirated-`h` noun in standard usage, so the mute-`h` label is the one most
likely to be wrong, but that judgement is an expectation and was not checked
against an authoritative grammar in this pass.

**Consequence for elision:** A deterministic rule that elides before any vowel
letter would produce `l'haricot`, which is incorrect for an aspirated-`h` noun.
A rule that trusts the source's aspiration category would be wrong for
`haricot`. Elision therefore needs one of these:

- a curated per-lemma `h` status, with an owner and a review step, or
- an explicit "unknown" state that does not elide, showing the bare article.

Neither option has been chosen; the choice belongs to the implementation
ticket.

### 3. Lexique 4 (optional cross-check)

*Verified:* the Lexique site states CC BY-SA 4.0 for Lexique 4, released
2026-04-27, covering about 190,000 forms, subtitle frequencies, contextual
diversity, phonology, lemmas, and morphological structure. Citation: New et al.,
Behavior Research Methods 58(5), 140 (2026). The Lexique 3 manual is obsolete.

*Unverified:* Lexique 4 column names, and therefore whether lemma, gender,
plural, or phonology fields can be read without reverse engineering.

**Assessment:** A plausible cross-check for inflected-form lookups and phonology.
It is not required for the minimum resource set, and no frequency work is in
scope.

### 4. UD `fr_gsd` (test fixture only)

*Verified:* UD `fr_gsd` is CC BY-SA 4.0. It contains elided forms such as `l'`,
`d'`, `qu'`, `j'`, `n'`, `c'`, and `m'` as separate tokens with ASCII
apostrophe. It has MWTs `du`, `des`, `au`, and `aux` (nine types in the
treebank).

*Unverified:* the lemma quality of the treebank. Lemmas are program-assigned,
and no accuracy claim is made here.

**Assessment:** Good for tokenization and elision tests, not for dictionary
content.

### 5. Lefff (defer)

*Verified:* Lefff is distributed under LGPL-LR. It maps each form to a lemma and
morphological traits, and offers versions 3.4 and 2.1. The page text read was
last updated in April 2006.

*Unverified:* current maintenance status and whether a version newer than 3.4
exists.

**Assessment:** Potentially strong for inflection, but maintenance is unclear.
Defer unless a later check confirms it is maintained.

### 6. Morphalou 3.1 (defer)

*Verified from listings only (ELG/CLARIN):* about 159,000 lemmas and 976,570
inflected forms, distributed via ORTOLANG.

*Unverified:* license, column layout, and whether the ORTOLANG and ATILF pages
could be read. Both pages did not load in this research pass, and no other
route to them was used.

**Assessment:** Cannot be recommended until license and columns are checked.

## Accuracy Caveats

- No accuracy number from a historical benchmark is used here as a current
  claim. Stanza's older French results (1.5.1 on UD 2.12: lemmas 98.20, UPOS
  97.63, UAS 93.27) are version-specific and are not evidence for the pinned
  Stanza 1.15 pipeline. Current French package availability and accuracy are
  unverified and belong to the NLP research ticket.

## Minimum Resource Set

| Need | Resource | Status |
| --- | --- | --- |
| Lemma, gloss, POS, gender, plural, IPA | Wiktextract / Kaikki (`fr` filter) | Recommended; filter and validate |
| Elision and aspirated `h` | Curated per-lemma list, or explicit unknown | Required design decision, not chosen here |
| Pronominal verbs | Kaikki entry layout | Open (unverified) |
| Tokenization test fixture | UD `fr_gsd` | Recommended for tests only |
| Inflected-form cross-check | Lexique 4 | Optional; columns unverified |
| Inflection depth | Lefff, Morphalou | Deferred |

## Unresolved Gaps

1. Pronominal-verb entry layout in Kaikki.
2. Article-entry tags (`le`, `l'`, `la`) in Kaikki.
3. Source of authoritative aspirated-`h` status for the lemma list.
4. Morphalou license and columns.
5. Lexique 4 column names.
6. Lefff maintenance status.
7. Kaikki data license as shipped.
8. Index size for a French-only derivation.
9. Current Stanza French package and accuracy (NLP ticket).

## Follow-Up Questions for the Parent

- Who owns the aspirated-`h` list, and what source is authoritative for it?
- Should a lemma with an unknown `h` status show the bare article or no article
  in the card?
- Is Lexique 4 wanted for cross-checks at all, given the scope excludes
  frequency?

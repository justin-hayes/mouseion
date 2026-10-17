# ADR 0102: French language support

Status: **Accepted** · Date: 2026-10-17 · Author: Justin + opencode

Tracked by [#2157](https://github.com/justin-hayes/mouseion/issues/2157) under
[#2156](https://github.com/justin-hayes/mouseion/issues/2156). Builds on
**ADR 0023** (the NLP service owns language capabilities), **ADR 0063** (Stanza
models provisioned on a volume) and **ADR 0073** (per-language package and
processor selection). Amends ADR 0063's provisioning integrity, ADR 0005's
normalization profiles, and ADR 0064's dictionary-index language set and
derivation rules.

## Context

French (`fr`, display name **French**) becomes the fourth provisioned modern
study language. Like Italian and Modern Greek, it goes through the
capability-driven, language-generic path: no language enum, no allowlist, no
migration. The resource and policy decisions were settled before this ADR and
are recorded here, not reopened:

- the support map ([#2138](https://github.com/justin-hayes/mouseion/issues/2138));
- resource selection and support boundary
  ([#2141](https://github.com/justin-hayes/mouseion/issues/2141#issuecomment-6236651436));
- the readiness and acceptance contract
  ([#2142](https://github.com/justin-hayes/mouseion/issues/2142#issuecomment-6236750750));
- the bounded resource and pronominal-identity probe
  ([#2152](https://github.com/justin-hayes/mouseion/issues/2152#issuecomment-6236592308);
  [findings](https://github.com/justin-hayes/mouseion/blob/99ebc5410f64d20635644207a54e4fcc23365863/prototype/french-2152/FINDINGS.md)
  on the never-merged branch `prototype/french-2152-probe`);
- resource provenance
  ([#2153](https://github.com/justin-hayes/mouseion/issues/2153);
  [research note](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md)
  on the throwaway branch `research/french-resource-provenance`);
- the rights disposition
  ([#2155](https://github.com/justin-hayes/mouseion/issues/2155#issuecomment-6236283454),
  closed not planned).

The probe ran the pinned resources against 42 constructed diagnostic strings.
The Stanza pipeline handled elision, contractions and clitics well. The defects
it found are on Mouseion's side:

- **Sentence integrity.** A closing `»` is split from its sentence in every
  dialogue layout, and em-dash incises are split, so persisted representative
  sentences can be incomplete.
- **Target anchoring.** The card-export matcher (`textmatch.FoldedWordSpan`)
  treats `'`, `’` and hyphens as word characters. A target directly after an
  elided clitic (`l'homme`, `s’habille`) or before an inversion hyphen
  (`va-t-il`, `dit-elle`) therefore has no word boundary and is lost: 17 of 108
  eligible targets failed. All 108 persisted token spans sliced exactly to their
  surfaces.
- **Eligibility.** `ne`/`n'` and `pas` are ADV and pass the content-word filter.
- **Dictionary.** The derivation produces no French article, cannot tell mute h
  from unknown h, strips the optional schwa `(ə)` from IPA, drops IPA whose
  transcriptions are all region-tagged, and cannot tell an invariable plural from
  a missing one.
- **Identity.** Pronominal uses are tagged inconsistently both by the parser
  (`expl:pv`/`obj`/`iobj`) and by Wiktionary (`reflexive`/`pronominal`, separate
  headword pages only sometimes). A naive blanket pronominal rule produced a
  false `se parler`.

## Decision

### Stanza package: the pinned 1.15.0 combined package

French is provisioned and served with Stanza 1.15.0's French combined package.
The processor set is `tokenize,mwt,pos,lemma,depparse`; NER and coreference are
excluded:

```
tokenize = combined_nocharlm
mwt      = combined
lemma    = combined_nocharlm
pos      = combined_charlm
depparse = combined_charlm
```

`pos` and `depparse` depend on the `conll17` pretrained embeddings and the
`newswiki` forward and backward character models. The resource manifest is
pinned at `stanfordnlp/stanza-resources@569db9d`, and the eight runtime
artifacts at Hugging Face `stanfordnlp/stanza-fr@caeb7742`. They total about
319 MiB. On the probe host the pipeline loaded in 2.8 s, peaked at about
0.94 GB RSS, and took a median of 36 ms per diagnostic string.

As for Greek, `mwt` is internal preprocessing. Capabilities advertise
`tokenize,pos,lemma,depparse`, and the French capability's `model_version`
identifies the combined package. Lexique, Lefff, Morphalou and spaCy are
deferred.

### Checksum-verified offline provisioning (amends ADR 0063)

- **Generic artifact checksums.** A language configuration may declare an
  expected sha256 for each artifact it provisions. The provisioner verifies every
  declared artifact after download and before writing its success marker. A
  mismatch is fatal, so the NLP service does not start on a corrupted or
  substituted model.
- **French uses it at launch.** French declares the sha256 of all eight pinned
  artifacts and fetches them at the pinned manifest and Hugging Face revisions,
  never from a moving default. German, Italian and Greek declare no checksums
  and are unaffected; they can adopt the mechanism later without a new decision.
- **Marker.** The provision marker records the French package selection and its
  pinned checksums alongside the existing per-language model configuration. A
  change re-provisions in full instead of silently serving a stale model.
- **Offline serving.** Serving constructs the French pipeline offline, as for
  every other language. `fr` joins the Compose default
  `MOUSEION_NLP_WARM_LANGUAGES` (`de,it,el,fr`).

### Sentence integrity

The producer applies French-specific sentence handling. Each of these persists
as one complete sentence:

- a sentence closed by `»`, whatever follows it: an ordinary space, a narrow
  no-break space (U+202F), a paragraph break, or more text in the same paragraph;
- `« Tu viens ? » demanda-t-il.`;
- em-dash dialogue incises such as `— Tu viens ? demanda-t-il.`

Sentence text is still sliced from the source with exact offsets and is never
rewritten. Plain-prose sentence counts on the corpus do not change, and German
`„…“` and Italian segmentation do not regress.

### French canonicalization (amends ADR 0005)

A dedicated French normalization profile defines the canonical lemma as NFC plus
Unicode case folding with accents preserved, so `où`≠`ou` and `sûr`≠`sur`. In
the canonical lemma only, U+2019 RIGHT SINGLE QUOTATION MARK becomes U+0027
APOSTROPHE, so `aujourd'hui` and `aujourd’hui` are one identity. The raw lemma,
surfaces and source text are not changed.

As for Greek, the profile is applied in four places: the Python producer, the
dictionary-index derivation, Go's runtime dictionary lookup, and Go's
canonicalization registry.

### Vocabulary identity: base-lemma pronominal grouping

Pronominal and plain uses share the base verb identity with **no lexical
discriminator**: `je me lave` and `je lave` are both *laver*, and Known *aller*
covers `s'en aller`. The identity stays `(language, canonical lemma, upos)`.
Article and preposition contraction components (`au` → *à + le*, `du` →
*de + le*) are ADP/DET and never become content-word identities. No new state,
table or migration is needed.

**Accepted limitation.** Dictionary lookup on the base lemma cannot reach a
meaning that exists only under its own pronominal headword, such as `s'en aller`
"to go away, to leave". The contextual Gloss covers it on the card: `Il s'en
va…` must get a "go away/leave" Gloss.

### Eligibility and stopwords

French selection excludes ADV `ne`/`n'` (lemma *ne*) and ADV `pas` from
vocabulary and coverage. Both join the French contextual stopword set alongside
basic French function words. NOUN *pas* ("step") stays eligible, and so do
`près`, `là` and `où`. The existing UPOS filter applies unchanged.

### Target anchoring: anchor on persisted token offsets

Card export guarantees, for every language, that an eligible target is anchored
in its unchanged source sentence. This includes a target after an ASCII or curly
apostrophe elision and before an inversion hyphen. The mechanism is the
analyzer's **persisted token offsets**, not a changed word-boundary rule:

- The target span is the matching sentence token's persisted source span,
  relative to its sentence's persisted span. The matching token is the first, in
  sentence order, whose cleaned surface (`CleanLexicalSurface`) case-folds equal
  to the tested target.
- The span is accepted only if the persisted sentence text, sliced at those
  offsets, case-folds equal to the token surface. That check guards against stale
  or inconsistent offsets.
- The same anchor decides target presence everywhere card export uses it:
  rendering the bolded front, the sentence-quality "target present" check, and
  choosing a sentence's matching observed form. Otherwise elided sentences would
  be rejected before rendering.
- When a sentence has no persisted tokens, or no token yields a verified span,
  card export falls back to the unchanged word-bounded matcher.
  `FoldedWordSpan`, its word-rune set and its tests are not changed.
- Persisted source sentences are never rewritten. Normalizing apostrophes in
  source text is not a fix.

The acceptance check is 108/108 eligible probe-corpus targets anchored, in both
apostrophe forms, with byte-identical persisted sentences. An Italian `L'uomo`
elision regression is added, and German and Greek match results stay unchanged.

### Dictionary index (amends ADR 0064)

`fr` is added to the Kaikki/Wiktextract derivation, from the pinned raw
English-Wiktionary data (dump 2026-09-02, extraction 2026-10-03). It derives
English glosses, gender, plural or "invariable", h-status evidence and IPA:

- **h status.** The derivation records whether an entry carries Wiktionary's
  "French terms with aspirated h" category. That category means aspirated h.
  Absence means **unknown**, never mute, because Wiktionary never positively
  marks mute h and has isolated gaps (`héros` lacks the category). No curated
  aspirated-h list is used, and h status is never inferred from pronunciation.
- **Plurals.** A head marked invariable yields "invariable", which is shown
  rather than treated as a missing plural. A missing plural is omitted. Isolated
  merged-entry errors are accepted; for example `ciel` takes `cieux` for both of
  its plurals.
- **Gender** is recorded and shown on NOUN rows only, because page categories
  leak gender onto other parts of speech.
- **IPA.** The optional schwa `(ə)` is preserved verbatim. Untagged
  transcriptions and transcriptions tagged France, Europe or Paris are kept.
  When only other regional tags exist, IPA is omitted. The card never presents
  one transcription as covering every variety.
- **No verb principal parts or conjugation summaries.** The German
  principal-part extractor is not reused, and nothing is invented.
- **Provenance.** The index records the Kaikki dump date, extraction date and
  source-file hash, so a card's evidence can be traced to its snapshot.

### Card export: the French article rule

French nouns show an article only where the evidence supports it:

| Noun | Card shows |
|---|---|
| Consonant-initial, one supported gender | `le`/`la` + headword |
| Vowel-initial | `l'` + headword, plus a gender label |
| h-initial, aspirated-h category present | `le`/`la` + headword |
| Any other h-initial | bare headword + gender |
| Conflicting or missing gender | bare headword, no gender |

Vowel-initial means a, e, i, o or u (accented or not), or the ligatures æ and
œ. When the initial is uncertain (for example `y`), the card falls back to the
bare headword with gender rather than risk a wrong article. Gender appears only
on NOUN rows. Lemma capitalization display (`lemmadisplay`) stays German-only.

### Other surfaces

- **Sentence quality.** French uses the generic non-German rubric, with no GDEX
  knock-out, French-specific rubric or subject gate. A later subject gate must
  accept impersonal and expletive constructions such as `Il faut partir`.
- **Contextual Gloss and translation.** Provider requests label the language
  "French". A card succeeds only with a defensible contextual Gloss and sentence
  translation. Otherwise the ADR 0100 retryable provider omission (or a meaning
  omission) applies.
- **Deck output.** The APKG hierarchy is `Mouseion::fr::<book title>` with the
  tag `lang::fr`, rendered through the frozen card contract. Generated
  vocabulary stays provenance only, and Known vocabulary still changes only
  through reading completion (ADR 0092).

### Rights

French proceeds on the same personal self-hosted footing as Italian. Models and
dictionary data are downloaded at deployment time and never redistributed by
this repository. No third-party licence question is declared resolved: the
Stanza model notices conflict, the `newswiki` character-model corpus is
unidentified, the CoNLL17 source deposit carries CC BY-NC-SA 4.0, and
Wiktionary text is reused under CC BY-SA 4.0 or GFDL. Redistribution, and
public or commercial use, remain unsupported.

## Consequences

- French gains sentence segmentation, MWT expansion, combined-package
  morphology, lemmas and a dependency parse, served fully offline from
  byte-verified artifacts.
- Every language gains elision- and hyphen-safe card targets where persisted
  tokens exist. Italian `L'uomo`-style elided targets stop being lost. German and
  Greek match results must stay unchanged, and regression tests assert it.
- Spaced compounds and fixed expressions are split: `petit déjeuner` yields
  *déjeuner* and `d'accord` yields *accord*. This is a documented launch
  limitation; multiword-expression detection is a later refinement.
- Base-lemma grouping keeps Known, Reserved and snapshot shapes unchanged, at the
  cost of pronominal-only dictionary meanings.
- Some card fields are omitted rather than guessed: articles where the h status
  or gender is uncertain, regional-only IPA, and conjugation summaries.
- The provisioner gains a generic checksum mechanism and a richer marker.
  `compose.yaml` and NLP packaging changes cross the human-review boundary.
- No accuracy figure on real books exists yet. The dictionary-coverage and
  real-book card-review validations in the
  [French vertical](../features/language-support.md#french-vertical) supply it.

## Alternatives considered

- **Adjusting the word-boundary rule for target anchoring.** Rejected. Dropping
  apostrophes and hyphens from the word-rune set would change German and Greek
  results: `Bahn` would match inside `U-Bahn` and `Neill` inside `O'Neill`. An
  elision-aware rule would need a per-language clitic list in Go that duplicates
  the tokenizer's knowledge. It also could not tell elision (`l'homme`) from a
  word-internal apostrophe (`aujourd'hui`) without that list. Persisted offsets
  are already exact and language-neutral.
- **Normalizing apostrophes in source text.** Rejected: it rewrites the
  author's text, and curly-apostrophe elision still lacks a boundary.
- **Blanket pronominal identities** (`se laver` distinct from `laver`).
  Rejected: they need clitic-versus-reflexive detection, which failed on the
  probe (`se parler`). They fragment Known and Reserved vocabulary, and most
  pronominal forms have no headword of their own.
- **Selective identities for lexicalized pronominal verbs only.** Rejected:
  `expl:pv` was observed once and is inconsistent across relations, and
  Wiktionary's separate pages are not a reliable lexicalization signal. Both
  pronominal options remain out of scope, with no lexical discriminator.
- **Treating a missing aspirated-h category as mute h.** Rejected: it would
  teach *l'héros*. A curated aspirated-h list is not a launch prerequisite under
  the conservative fallback.
- **A French-specific sentence rubric or subject knock-out now.** Deferred,
  matching Italian and Greek.
- **Other resources (Lexique, Lefff, Morphalou, spaCy).** Deferred: the probe
  found no Stanza weakness that another resource would obviously fix.

## Related

- [ADR 0005: Vocabulary identity, normalization, and initial ranking defaults](0005-vocabulary-identity-normalization-ranking.md) — French normalization profile added here.
- [ADR 0023: NLP service owns language capabilities](0023-nlp-capabilities.md)
- [ADR 0063: Stanza model provisioning on a Docker volume instead of the image](0063-stanza-models-on-volume.md) — artifact checksum verification added here.
- [ADR 0064: Built-in dictionary enrichment provider](0064-dictionary-enrichment-provider.md) — French derivation, h status, IPA and provenance added here.
- [ADR 0073: Modern Greek language support](0073-modern-greek-language-support.md) — the per-language package and processor seam reused here.
- [ADR 0092: Retire known-vocabulary import](0092-retire-known-vocabulary-import.md)
- [ADR 0100: Retryable per-item omissions for prepared decks](0100-retryable-provider-omissions-preparation.md)
- [Language Support](../features/language-support.md#french-vertical)

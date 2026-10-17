# Language Support

Status: Implemented; French vertical specified, implementation pending · Date: 2026-08-25 · Issues: #222, #223, #224, #225, #2156

## Runtime contract

The NLP service is authoritative for language availability. It advertises only
configured pipelines that warmed successfully, including their display names
and supported features. Catalog sync uses those capabilities to walk every
offered non-English language that is ready; the resulting chosen-language Books
derive each learner's study-language set. The web application does not maintain
a learner-selected language allowlist; the learner's active study language
([ADR 0050](../adr/0050-active-study-language.md)) is a context selection into
the derived set, never an allowlist. German (`de`, display name **German**),
Italian (`it`, display name **Italian**), and Modern Greek (`el`, display name
**Greek**) are provisioned by the standard deployment.

If discovery is unavailable, stored display-name references keep derived
language labels legible, while sync and actions requiring a newly ready
capability are unavailable. A failed pipeline warmup leaves that language not
ready without hiding other successfully warmed languages or deleting existing
Books and vocabulary.

## Italian vertical

The deterministic Italian regression fixture validates the complete product
contract where a live model or database is unavailable:

1. capability discovery exposes ready Italian as **Italian**;
2. the ready Italian capability is used by catalog sync and Italian Books
   become the learner's derived study language;
3. Go consumes the Python/Stanza fixture with Italian contractions, accents,
   morphology, and clitics intact;
4. selection aggregates canonical content-word lemmas while filtering
   punctuation, determiners, adpositions, pronouns, conjunctions, and proper
   names under the same rules used for German;
5. coverage and 95/97/99 threshold insights use Italian lemma occurrences and
   the learner's owner-scoped known and reserved vocabulary;
6. the prepared-deck path emits an APKG containing cards tagged `lang::it` in
   the hierarchy `Mouseion::it::<book title>`, suitable for the Book's
    Goal's prepared artifact and historical provenance.

German remains covered by its existing analysis, selection, coverage, display,
and export regressions. Language columns and identities are general text values;
the persistence schema contains no German-only constraint, so Italian requires
no database migration.

## Greek vertical

The deterministic Greek regression fixture proves the same learner loop for
Modern Greek (`el`):

1. capability discovery exposes a warmed Greek pipeline as **Greek**;
2. catalogue sync admits offered Greek Books from capability identity, without
   an application language allowlist;
3. chosen Greek Books derive Greek as a study language and can be selected as
   the active study-language context;
4. the normalized-corpus boundary preserves accents, capitalization, final
   sigma, `στο`/`στην` multiword-token expansion, noun gender, inflected verbs,
   and dependency heads;
5. selection and coverage aggregate the shared Greek canonical lemma identity
   while keeping Known, Generated, and Reserved vocabulary owner- and
   language-scoped;
6. representative sentences use the generic quality rubric. German GDEX
   resources are intentionally not applied to Greek;
7. prepared decks remain separate from Known vocabulary and use
   `Mouseion::el::<book title>` with the `lang::el` tag. Greek dictionary
   articles, glosses, plurals, and IPA are rendered through the frozen card
   contract.

The Greek contract deliberately does not add a language enum, persistence table,
or Greek-specific workflow state. Principal-part extraction remains Germanic,
and Greek sentence quality remains on the generic path until language-specific
resources are justified. If Greek warmup fails, cached/stored labels remain
legible and ready German and Italian capabilities remain available.

## French vertical

French (`fr`, display name **French**) follows the same learner loop.
[ADR 0102](../adr/0102-french-language-support.md) records its decisions, and
[#2156](https://github.com/justin-hayes/mouseion/issues/2156) tracks the
implementation, which is pending; French is not yet provisioned by the standard
deployment. This section is the acceptance contract.

### Proof tiers

1. **Deterministic French regression fixture, in default CI.** It proves the
   full product loop without a live model or database:
   1. capability discovery exposes a warmed French pipeline as **French**;
   2. catalogue sync admits offered French Books from capability identity,
      without an application language allowlist;
   3. chosen French Books derive French as a study language, which can be the
      active study-language context;
   4. the normalized corpus preserves accents, elision, contractions, clitics,
      morphology and dependency heads;
   5. selection and coverage, including the 95/97/99 threshold insights, use the
      base-lemma identity, with owner- and language-scoped Known, Reserved and
      Generated vocabulary;
   6. the prepared deck becomes an APKG in `Mouseion::fr::<book title>` with the
      `lang::fr` tag, and stays separate from Known vocabulary.
2. **Pinned-model golden check, opt-in.** This is a recorded release gate, run
   outside default CI. The 42 human-approved constructed diagnostic strings from
   the probe (22 approved inputs plus 20 extended) are the committed French
   regression corpus. The check asserts tokens, lemmas, sentence boundaries and
   target anchors against the exact pinned models.

### Acceptance checks

| Area | Check |
|---|---|
| Target anchoring | 108/108 eligible corpus targets are anchored, in both ASCII and curly apostrophe forms and before inversion hyphens, with byte-identical persisted sentences. Targets the existing word-bounded matcher misses are anchored on persisted token offsets, for every language. An Italian `L'uomo` elision regression is added, and German and Greek match results are unchanged. |
| Sentence boundaries | Each of these persists as one complete sentence: a sentence closed by `»` followed by an ordinary space, U+202F, a paragraph break or more text in the same paragraph; `« Tu viens ? » demanda-t-il.`; em-dash dialogue `— Tu viens ? demanda-t-il.` Plain-prose counts are unchanged on the corpus. German `„…“` and Italian segmentation do not regress. |
| Eligibility | ADV `ne`/`n'` (lemma *ne*) and ADV `pas` are excluded from French vocabulary and coverage and added to the French context stopwords. NOUN *pas*, `près`, `là` and `où` stay eligible. |
| Canonical identity | A dedicated French profile: NFC + casefold, accents preserved (`où`≠`ou`, `sûr`≠`sur`), U+2019 → U+0027 in the canonical lemma only, so `aujourd'hui` and `aujourd’hui` are one identity. It is applied identically in the producer, the dictionary derivation, Go lookup and the canonicalization registry. Source text is never rewritten. |
| Pronominal identity | Pronominal and plain uses share the base verb: `je me lave` counts as *laver*, and Known *aller* covers `s'en aller`. Contraction components never become content-word identities. |
| Articles and gender | Conflicting or missing gender always gives a bare headword with no gender; the rules below need one supported gender. Consonant-initial nouns get `le`/`la`. Vowel-initial nouns get `l'` plus a gender label. An h-initial noun gets `le`/`la` only with the aspirated-h category; otherwise it shows the bare headword with gender, so *héros* is shown bare. Gender appears only on NOUN rows. |
| IPA and plurals | `(ə)` is preserved verbatim. Untagged transcriptions and those tagged France, Europe or Paris are accepted; IPA is omitted when only other regional transcriptions exist. "invariable" is shown when the head says so. A missing plural, conjugation summary or IPA is omitted, never invented. |
| Contextual Gloss and translation | Requests are labelled "French". A card succeeds only with a defensible contextual Gloss and sentence translation; otherwise the [ADR 0100](../adr/0100-retryable-provider-omissions-preparation.md) retryable omission applies. `Il s'en va…` gets a "go away/leave" contextual Gloss. |
| Provisioning and provenance | `fr` joins the Compose default `MOUSEION_NLP_WARM_LANGUAGES`. The provisioner fetches the eight pinned artifacts (manifest `569db9d`, Hugging Face `caeb7742`) and verifies their sha256, failing fatally on a mismatch. The mechanism is generic, but only French uses it at launch. The marker records the French package selection and checksums, and serving is offline. The dictionary index records the Kaikki dump date (2026-09-02), extraction date (2026-10-03) and source hash. |
| Failure isolation | A failed French warmup leaves `de`, `it` and `el` ready, and stored French labels stay legible. |

### Proposed validations

The human runs these after implementation. They have not been measured yet and
are not CI checks; so far only the probe's bounded results exist, and no
accuracy figure on real books.

- **Dictionary coverage.** Derive the full pinned `fr` index. Take the most
  frequent content lemmas from the analysis of 2–3 real contemporary French
  books in the human's own catalogue; these stay local and are never committed.
  At least 95% of the top 2,000 must have an English gloss row. Misses are
  triaged as isolated or systematic, not failed automatically.
- **Real-book card review.** Review 50 random prepared cards from one real
  French book. Each card is checked for a correct identity under base-lemma
  grouping, an anchored target, a whole and unmodified sentence, a defensible
  Gloss and translation, and no wrong article, gender or IPA (omission is
  fine). The pass bar is at least 90% clean, with zero wrong articles and zero
  rewritten sentences.

### Accepted limitations

- **Spaced compounds and fixed expressions are split.** `petit déjeuner` yields
  *déjeuner*, and `d'accord` yields *accord*. The golden corpus records the split
  as the expected output. Multiword-expression detection is a later refinement.
- **Pronominal-only meanings are unreachable by dictionary lookup.** Base-lemma
  lookup on *aller* cannot reach `s'en aller` "go away". The contextual Gloss
  covers the meaning on the card.
- **Isolated merged-entry errors are accepted.** For example, `ciel` takes
  `cieux` as its plural for every sense. Systematic gaps in common vocabulary are
  not accepted.
- Omitted IPA, plurals or conjugation summaries are acceptable; invented values
  are not.

The French contract adds no language enum, persistence table, migration or
French-specific workflow state. Verb principal parts, a French sentence-quality
rubric, a subject gate and cover-title article styling are deferred.

### Licence and provenance

French artifacts are downloaded at deployment time and never baked into the
image or redistributed. Stanza is Apache-2.0. The French model notices are not
reconciled: Stanza's older model notice is a qualified ODC-By/"unclear", the
treebank notices include CC BY-SA and LGPL-LR, the CoNLL17 embedding source
carries CC BY-NC-SA 4.0, and the `newswiki` character-model corpus is
unidentified. The dictionary index is derived from English Wiktionary through
Wiktextract/Kaikki. Wiktionary text is available under CC BY-SA 4.0 or GFDL with
attribution, and the extractor's MIT licence does not cover the content. French
support is assessed for personal self-hosted use only; no third-party licence
question is declared resolved, and redistribution and public or commercial
hosting are unsupported. Operators should review these terms for their
deployment. The
[provenance research note](https://github.com/justin-hayes/mouseion/blob/c90b50de9583fa367986a3cac13700b6d19835df/doc/research/french-resource-provenance.md)
records the evidence.

## Model cache and deployment

`MOUSEION_NLP_WARM_LANGUAGES` selects which Stanza pipelines to provision, load,
and advertise, and is the single setting that drives all three operations.
The Compose `nlp-init` service reuses the NLP image and provisions each
configured language into the named `stanza-data` volume mounted at
`STANZA_RESOURCES_DIR=/opt/stanza_resources` using the language's effective
Stanza package and processor set. The `nlp` service mounts the same volume and
starts only after the init service completes successfully.

Greek's accurate package also requires the
`nlpaueb/bert-base-greek-uncased-v1` GreekBERT snapshot. The snapshot is
approximately 454 MB and is stored in the named `huggingface-data` volume at
`/opt/huggingface`, alongside the Stanza resources in `stanza-data` at
`/opt/stanza_resources`. The init container checks both caches before writing
its success marker; the serving container mounts both volumes and does not
need model-source network access.

Provisioning writes a marker containing the Stanza version, configured language
set, and each language's effective model configuration. An unchanged restart
completes without a download. If the language set grows, the init step downloads
only the missing language; update
`MOUSEION_NLP_WARM_LANGUAGES` in `.env` and run `docker compose up -d`, without
rebuilding the image. When the Stanza dependency version changes, the marker
causes the volume to be wiped and the complete configured bundle to be
provisioned again. A changed package, processor set, or external model
dependency has the same full-reprovision behavior. A missing or legacy marker,
including on an existing volume containing only the old partially-baked model
set, also triggers full reprovisioning.
Provisioning failure is fatal, so the NLP service does not start with an
incomplete cache.

For a manual launch, set persistent `STANZA_RESOURCES_DIR` and `HF_HOME`
directories, set `MOUSEION_NLP_WARM_LANGUAGES`, and run
`python -m mouseion_nlp.provision` before `python -m mouseion_nlp.server`.
Rerun the provisioner after adding a language or upgrading Stanza. Missing or
incompatible resources make only that language unavailable after startup.
Artifacts are downloaded at deployment time rather than baked or redistributed
in the application image. Stanza is Apache-2.0, GreekBERT is MIT, and the UD
Greek-GDT data used by the Stanza package is CC BY-NC-SA 3.0; operators should
review those licenses for their deployment.

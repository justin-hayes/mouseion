# Language Support

Status: Implemented · Date: 2026-08-25 · Issues: #222, #223, #224, #225

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

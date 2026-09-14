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
the derived set, never an allowlist. German (`de`, display name **German**) and
Italian (`it`, display name **Italian**) are provisioned by the standard
deployment.

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
   vocabulary-study history.

German remains covered by its existing analysis, selection, coverage, display,
and export regressions. Language columns and identities are general text values;
the persistence schema contains no German-only constraint, so Italian requires
no database migration.

## Model cache and deployment

`MOUSEION_NLP_WARM_LANGUAGES` selects which Stanza pipelines to provision, load,
and advertise, and is the single setting that drives all three operations.
The Compose `nlp-init` service reuses the NLP image and provisions each
configured language into the named `stanza-data` volume mounted at
`STANZA_RESOURCES_DIR=/opt/stanza_resources`. It downloads the full runtime
processor set: `tokenize,pos,lemma,depparse`. The `nlp` service mounts the
same volume and starts only after the init service completes successfully.

Provisioning writes a marker containing the Stanza version and configured
language set. An unchanged restart completes without a download. If the
language set grows, the init step downloads only the missing language; update
`MOUSEION_NLP_WARM_LANGUAGES` in `.env` and run `docker compose up -d`, without
rebuilding the image. When the Stanza dependency version changes, the marker
causes the volume to be wiped and the complete configured bundle to be
provisioned again. A missing marker, including on an existing volume containing
only the old partially-baked model set, has the same full-reprovision behavior.
Provisioning failure is fatal, so the NLP service does not start with an
incomplete cache.

For a manual launch, set `STANZA_RESOURCES_DIR` to the local Stanza resource
directory, set `MOUSEION_NLP_WARM_LANGUAGES`, and run
`python -m mouseion_nlp.provision` before `python -m mouseion_nlp.server`.
Rerun the provisioner after adding a language or upgrading Stanza. Missing or
incompatible resources make only that language unavailable after startup.

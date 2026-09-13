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
   morphology, clitics, and named entities intact;
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

`MOUSEION_NLP_WARM_LANGUAGES` selects the Stanza pipelines that the one-shot
Compose init service provisions and that the NLP service loads and advertises.
The standard deployment mounts the named `stanza-data` volume at
`STANZA_RESOURCES_DIR=/opt/stanza_resources`; the image itself contains no model
cache. Provisioning downloads the full
`tokenize,pos,lemma,depparse,ner` bundle for each configured language. The
marker in the volume makes unchanged restarts a no-op, while a grown language
set downloads only the missing language. A missing marker also causes a partial
or legacy volume to be wiped and provisioned fresh.

To add a deployment language, include its code in
`MOUSEION_NLP_WARM_LANGUAGES` and recreate the Compose stack; no image rebuild
is required. For a manual launch, run the provisioner with the same environment
before starting the service. Missing or incompatible resources make only that
language unavailable.

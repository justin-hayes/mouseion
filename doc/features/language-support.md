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

`MOUSEION_NLP_WARM_LANGUAGES` selects which already-installed Stanza pipelines
to load and advertise. It does not define or install a model-cache volume.
The standard NLP image sets `STANZA_RESOURCES_DIR=/opt/stanza_resources` and
downloads the German and Italian `tokenize,pos,lemma` resources while building
the image. Consequently, Compose's default `de,it` setting works without a
runtime download or writable model cache.

To add a deployment language, provision its processors in `nlp/Dockerfile`,
rebuild the NLP image, and then include its code in
`MOUSEION_NLP_WARM_LANGUAGES`. For a manual launch, download the same processors
into the local Stanza resource directory before starting the service. Missing
or incompatible resources make only that language unavailable.

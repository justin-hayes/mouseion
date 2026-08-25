# Language Support

Status: Implemented · Date: 2026-08-25 · Issues: #222, #223, #224, #225

## Runtime contract

The NLP service is authoritative for language availability. It advertises only
configured pipelines that warmed successfully, including their display names
and supported features. The web application offers those ready languages for a
learner's owner-scoped study-language selection and does not maintain a second
language allowlist. German (`de`, display name **German**) and Italian (`it`,
display name **Italian**) are provisioned by the standard deployment.

If discovery is unavailable, saved selections remain visible and unchanged,
while actions that require a newly discovered language are unavailable. A
failed pipeline warmup leaves that language not ready without hiding other
successfully warmed languages.

## Italian vertical

The deterministic Italian regression fixture validates the complete product
contract where a live model or database is unavailable:

1. capability discovery exposes ready Italian as **Italian**;
2. learner study-language selection remains owner-scoped;
3. Go consumes the Python/Stanza fixture with Italian contractions, accents,
   morphology, clitics, and named entities intact;
4. selection aggregates canonical content-word lemmas while filtering
   punctuation, determiners, adpositions, pronouns, conjunctions, and proper
   names under the same rules used for German;
5. coverage and 95/97/99 threshold insights use Italian lemma occurrences and
   the learner's owner-scoped known and active-campaign vocabulary;
6. the prepared-deck path emits an APKG containing cards tagged `lang::it` in
   the hierarchy `Mouseion::it::<book title>`, suitable for the campaign queue.

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

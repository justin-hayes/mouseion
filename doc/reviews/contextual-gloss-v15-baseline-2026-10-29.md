# Contextual Gloss review — v15 baseline

Status: **Baseline pending; no provider configured in the implementation
environment**

- Date: 2026-10-29
- Model: not configured
- Reasoning setting: not configured
- Prompt contract: `translation-v15-short-handles`
- Case set: `gloss-review-v1` (20 cases) in
  [`cmd/glossreview/testdata/gloss_review_cases.json`](../../cmd/glossreview/testdata/gloss_review_cases.json)
- Command: `go run ./cmd/glossreview` (see [the review guide](README.md))

## Run status

**Not performed.** The implementation environment had no
`MOUSEION_LLM_ENABLED=true`, `MOUSEION_LLM_API_KEY` or `MOUSEION_LLM_MODEL`, so
no request was sent and this report contains no model output. The operator must
run the command against the deployed provider and commit its dated report as the
actual v15 baseline that v16 must beat.

The command is deliberately absent from CI. Its unit test uses a stubbed provider
transport, so CI proves the case loading, automatic checks and report shape
without a network call or provider cost.

## Frozen case set

The 20 cases cover every verdict class in `de`, `it` and `el`.

| Class | Cases | Languages |
| --- | ---: | --- |
| Plain equivalent | 6 | de, el |
| Drop sentence material | 5 | de, it |
| Protected contextual win | 2 | de, it |
| Lemma mismatch | 1 | de |
| Spot check | 6 | de, it, el |

| Case | Language | Lemma | UPOS | Target form | Expected class | Expected Gloss |
| --- | --- | --- | --- | --- | --- | --- |
| `de-kalt` | de | kalt | ADJ | kalt | plain equivalent | cold |
| `de-feuer` | de | Feuer | NOUN | Feuer | plain equivalent | fire |
| `de-hoeflich` | de | höflich | ADJ | höflich | plain equivalent | polite |
| `de-finne` | de | Finne | NOUN | Finne | plain equivalent | Finn |
| `de-faehig` | de | fähig | ADJ | fähig | plain equivalent | capable |
| `de-erschiessen` | de | erschießen | VERB | erschießt | drop sentence material | to shoot dead |
| `de-eroeffnen` | de | eröffnen | VERB | eröffnet | drop sentence material | to open |
| `de-biegen` | de | biegen | VERB | bog | drop sentence material | to bend, to turn |
| `de-schiessen` | de | schießen | VERB | schossen | protected win | to gush out |
| `de-sehen-fern` | de | sehen | VERB | sieht | lemma mismatch | — |
| `de-doch` | de | doch | PART | doch | spot check | do come along |
| `de-umhauen` | de | umhauen | VERB | haut | spot check | to bowl over |
| `it-poggiare` | it | poggiare | VERB | poggiavo | drop sentence material | to lean, to rest |
| `it-macchina` | it | macchina | NOUN | macchine | drop sentence material | car, machine |
| `it-palazzo` | it | palazzo | NOUN | palazzo | protected win | apartment building |
| `it-ne` | it | ne | PRON | ne | spot check | about it |
| `it-in-gamba` | it | gamba | NOUN | gamba | spot check | capable |
| `el-tha` | el | θα | PART | θα | spot check | will |
| `el-kano-kryo` | el | κάνω | VERB | Κάνει | spot check | to be (of weather) |
| `el-vivlio` | el | βιβλίο | NOUN | βιβλίο | plain equivalent | book |

The representative sentence and frozen meaning evidence for each case live in
the case set. The `de-sehen-fern` sentence is *Großmutter sieht im Wohnzimmer
fern ohne Bild und Ton.*, where the target is the separable verb *fernsehen*,
not *sehen*.

## Review rubric

The operator records a verdict for each judgement-based item after reviewing the
case:

- [ ] Citation form
- [ ] No definition paraphrase
- [ ] No sentence inflection or arguments
- [ ] Protected win kept

The automatic checks in the command output are limited to **Gloss equals
translation** and **outcome is `lemma_mismatch`**. Under the v15 contract the
mismatch check reports **not supported by this contract**; it must not be counted
as a failure of the model. The report also prints the full review-corpus "Gloss contains translation" rate
for information only; it is not a rollout gate. The library-wide rate in #2325
is a separate measurement over the operator's `enrichment_cache`.

## Rollout use

Run the command at the deployed model and reasoning setting. Keep the generated
report under `doc/reviews/`, fill every reviewer verdict, and use it as the v15
baseline. The v16 review must keep every protected win, produce no definition
paraphrase and no sentence-inflected Gloss, and return `lemma_mismatch` for
`de-sehen-fern` once that outcome exists.

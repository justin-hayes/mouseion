# Contextual Gloss review — `translation-v17-clause-lemma-mismatch`

Status: **Gate failed — one missed-particle case was rejected as an invalid response** (#2354)

Superseded by the [re-run](contextual-gloss-v17-2026-11-09-rerun.md), made
after the codec fix for blank translations on omitted answers. Kept as the
record of the failed run.

- Date: 2026-11-09
- Model: `gpt-5.6-luna`
- Reasoning setting: `low`
- Prompt contract: `translation-v17-clause-lemma-mismatch`
- Provider: `openai-compatible` (`gpt-5.6-luna/translation-v17-clause-lemma-mismatch-reasoning-low`)
- Case set: `gloss-review-v2` (25 cases)
- Rubric: [doc/reviews/README.md](README.md)

## Gate

| Condition | Result |
| --- | --- |
| Keep every protected contextual win | **Pass**: `de-schiessen` ("to gush out") and `it-palazzo` ("apartment building") |
| No definition paraphrase on the case set | **Pass** |
| No sentence-inflected Gloss on the case set | **Pass**: `el-kano-kryo` ("to be cold") is the known κάνει κρύο idiom limitation (#2351), not counted, as in the v16 review |
| Return `lemma_mismatch` for every missed-particle case | **Fail**: `de-sehen-fern`, `de-ruft-an` and `de-steht-auf` pass; `de-kauft-ein` was rejected as an invalid response ("translation is empty") |
| No `lemma_mismatch` for any lookalike control or ordinary case | **Pass**: `de-liegt-fern` and `de-fernsehen` are glossed |

Full review-corpus "Gloss contains translation" rate: 18/21 (85.7%), for
information only.

Verdicts drafted by Claude on 2026-11-09 at the operator's request; **awaiting
operator confirmation**.

The failing case is not a wrong Gloss. The provider's answer for
*Sie kauft samstags auf dem Markt ein.* left `translation` blank, and the codec
rejects a blank `translation` on every lemma answer. The report does not keep
the raw answer, so whether it carried `lemma_mismatch` is not observed. In
deck preparation the same answer is retried as an invalid response and never
becomes a card. #2385 relaxed this rule for unresolved idiom answers only.

v17 has been the live contextual-Gloss prompt since #2359 merged; nothing in
the code holds it back pending this review.

## Summary

- Cases run: 25
- Outcomes: glossed=21, abstained=3, unresolved=0, invalid response=1, provider error=0
- Automatic check Gloss equals translation: 18 passed, 3 failed, 4 not applicable
- Automatic check outcome is `lemma_mismatch`: **FAIL**
- Full review-corpus "Gloss contains translation" rate: 18/21 (85.7%), for information only and not a rollout gate

## `de-kalt` — de `kalt` (`ADJ`)

- Expected verdict class: plain equivalent
- Expected Gloss: `cold`
- Representative sentence: `Das Wasser war kalt und schmeckte nach Wasser.`
- Notes: v15 paraphrased the explanatory tail of the first sense instead of returning the headword equivalent.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `kalt` |
| upos | `ADJ` |
| target_word | `kalt` |
| example_sentence | `Das Wasser war kalt und schmeckte nach Wasser.` |
| candidate_senses | `e1 wikt:de:kalt:1: cold, chilly; the physical perception of something having a low temperature; e2 wikt:de:kalt:2: unfriendly, distant` |

### Provider result

- Outcome: `glossed`
- Translation: `cold`
- Raw Gloss: `cold`
- Evidence IDs: `wikt:de:kalt:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-feuer` — de `Feuer` (`NOUN`)

- Expected verdict class: plain equivalent
- Expected Gloss: `fire`
- Representative sentence: `Das Feuer wärmte die Hände der Wanderer.`
- Notes: A definition-style Gloss such as "an instance of something burning" must become the plain equivalent.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `Feuer` |
| upos | `NOUN` |
| target_word | `Feuer` |
| example_sentence | `Das Feuer wärmte die Hände der Wanderer.` |
| candidate_senses | `e1 wikt:de:feuer:1: fire; the flames and heat produced by burning` |

### Provider result

- Outcome: `glossed`
- Translation: `fire`
- Raw Gloss: `fire`
- Evidence IDs: `wikt:de:feuer:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-hoeflich` — de `höflich` (`ADJ`)

- Expected verdict class: plain equivalent
- Expected Gloss: `polite`
- Representative sentence: `Sie war höflich zu allen Gästen.`
- Notes: The adverb paraphrase "in a well-mannered way" is not the citation-form adjective.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `höflich` |
| upos | `ADJ` |
| target_word | `höflich` |
| example_sentence | `Sie war höflich zu allen Gästen.` |
| candidate_senses | `e1 wikt:de:hoeflich:1: polite, courteous; e2 wikt:de:hoeflich:2: in a well-mannered way` |

### Provider result

- Outcome: `glossed`
- Translation: `polite`
- Raw Gloss: `polite`
- Evidence IDs: `wikt:de:hoeflich:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-finne` — de `Finne` (`NOUN`)

- Expected verdict class: plain equivalent
- Expected Gloss: `Finn`
- Representative sentence: `Der Finne arbeitet seit Jahren in Berlin.`
- Notes: The plain equivalent is the demonym, not a paraphrase of nationality.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `Finne` |
| upos | `NOUN` |
| target_word | `Finne` |
| example_sentence | `Der Finne arbeitet seit Jahren in Berlin.` |
| candidate_senses | `e1 wikt:de:finne:1: Finn; a person from Finland` |

### Provider result

- Outcome: `glossed`
- Translation: `Finn`
- Raw Gloss: `Finn`
- Evidence IDs: `wikt:de:finne:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-faehig` — de `fähig` (`ADJ`)

- Expected verdict class: plain equivalent
- Expected Gloss: `capable`
- Representative sentence: `Er ist zu dieser Aufgabe fähig.`
- Notes: The sentence complement "to this task" must not enter the Gloss.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `fähig` |
| upos | `ADJ` |
| target_word | `fähig` |
| example_sentence | `Er ist zu dieser Aufgabe fähig.` |
| candidate_senses | `e1 wikt:de:faehig:1: capable, able` |

### Provider result

- Outcome: `glossed`
- Translation: `capable`
- Raw Gloss: `capable`
- Evidence IDs: `wikt:de:faehig:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-erschiessen` — de `erschießen` (`VERB`)

- Expected verdict class: drop sentence material
- Expected Gloss: `to shoot dead`
- Representative sentence: `Der Jäger erschießt den Wolf.`
- Notes: v15 returned the past-tense sentence form "shot dead" instead of the citation form.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `erschießen` |
| upos | `VERB` |
| target_word | `erschießt` |
| example_sentence | `Der Jäger erschießt den Wolf.` |
| candidate_senses | `e1 wikt:de:erschiessen:1: to shoot dead` |

### Provider result

- Outcome: `glossed`
- Translation: `to shoot dead`
- Raw Gloss: `to shoot dead`
- Evidence IDs: `wikt:de:erschiessen:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-eroeffnen` — de `eröffnen` (`VERB`)

- Expected verdict class: drop sentence material
- Expected Gloss: `to open`
- Representative sentence: `Sie eröffnet nächste Woche ein Café.`
- Notes: The Gloss must not carry the sentence's object or time expression.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `eröffnen` |
| upos | `VERB` |
| target_word | `eröffnet` |
| example_sentence | `Sie eröffnet nächste Woche ein Café.` |
| candidate_senses | `e1 wikt:de:eroeffnen:1: to open (a business or event); to inaugurate` |

### Provider result

- Outcome: `glossed`
- Translation: `to open`
- Raw Gloss: `to open`
- Evidence IDs: `wikt:de:eroeffnen:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-biegen` — de `biegen` (`VERB`)

- Expected verdict class: drop sentence material
- Expected Gloss: `to bend, to turn`
- Representative sentence: `Das Auto bog langsam um die Ecke.`
- Notes: The citation form is the infinitive; the sentence's path and direction stay in the English sentence.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `biegen` |
| upos | `VERB` |
| target_word | `bog` |
| example_sentence | `Das Auto bog langsam um die Ecke.` |
| candidate_senses | `e1 wikt:de:biegen:1: to bend; to turn (a corner)` |

### Provider result

- Outcome: `glossed`
- Translation: `to turn`
- Raw Gloss: `to turn`
- Evidence IDs: `wikt:de:biegen:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-schiessen` — de `schießen` (`VERB`)

- Expected verdict class: protected contextual win
- Expected Gloss: `to gush out`
- Representative sentence: `Die Tränen schossen ihr aus den Augen.`
- Notes: A genuine contextual improvement that the v16 contract must keep.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `schießen` |
| upos | `VERB` |
| target_word | `schossen` |
| example_sentence | `Die Tränen schossen ihr aus den Augen.` |
| candidate_senses | `e1 wikt:de:schiessen:1: to shoot; e2 wikt:de:schiessen:2: to gush out, to rush (of tears or water)` |

### Provider result

- Outcome: `glossed`
- Translation: `to gush out`
- Raw Gloss: `to gush out`
- Evidence IDs: `wikt:de:schiessen:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: pass — "to gush out"

## `de-sehen-fern` — de `sehen` (`VERB`)

- Expected verdict class: lemma mismatch
- Representative sentence: `Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.`
- Notes: The sentence uses separable fernsehen, not sehen. v15 glossed it as "to watch television".

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `sehen` |
| upos | `VERB` |
| target_word | `sieht` |
| example_sentence | `Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.` |
| candidate_senses | `e1 wikt:de:sehen:1: to see, to look; e2 wikt:de:sehen:2: to understand, to realize` |

### Provider result

- Outcome: `abstained`
- Translation: `to see`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `lemma_mismatch`
- Unresolved reason: `The sentence uses the separable verb fernsehen ('to watch TV'), with fern separated from sieht; the supplied lemma sehen is only its base verb.`

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **not applicable** — no resolved Gloss |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No definition paraphrase — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No sentence inflection or arguments — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] Protected win kept — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`

## `de-doch` — de `doch` (`PART`)

- Expected verdict class: spot check
- Expected Gloss: `do come along`
- Representative sentence: `Komm doch mit!`
- Notes: Function word: a longer phrase is acceptable when a one-word cue would mislead.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `doch` |
| upos | `PART` |
| target_word | `doch` |
| example_sentence | `Komm doch mit!` |
| candidate_senses | `e1 wikt:de:doch:1: however, nevertheless; e2 wikt:de:doch:2: modal particle adding emphasis or urging` |

### Provider result

- Outcome: `glossed`
- Translation: `come on`
- Raw Gloss: `come on`
- Evidence IDs: `wikt:de:doch:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-umhauen` — de `umhauen` (`VERB`)

- Expected verdict class: spot check
- Expected Gloss: `to bowl over`
- Representative sentence: `Die Nachricht haut mich um.`
- Notes: Separable verb idiom: the particle is part of the lemma, not sentence material.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `umhauen` |
| upos | `VERB` |
| target_word | `haut` |
| example_sentence | `Die Nachricht haut mich um.` |
| candidate_senses | `e1 wikt:de:umhauen:1: to knock down; e2 wikt:de:umhauen:2: to bowl over, to astonish` |

### Provider result

- Outcome: `glossed`
- Translation: `to bowl over, to astonish`
- Raw Gloss: `to bowl over, to astonish`
- Evidence IDs: `wikt:de:umhauen:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `it-poggiare` — it `poggiare` (`VERB`)

- Expected verdict class: drop sentence material
- Expected Gloss: `to lean, to rest`
- Representative sentence: `Poggiavo il gomito sul tavolo.`
- Notes: v15 returned "rested my wrist" and carried the sentence's person, tense and object.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `it` |
| target_language | `en` |
| canonical_lemma | `poggiare` |
| upos | `VERB` |
| target_word | `poggiavo` |
| example_sentence | `Poggiavo il gomito sul tavolo.` |
| candidate_senses | `e1 wikt:it:poggiare:1: to lean, to rest (something against something); e2 wikt:it:poggiare:2: to place` |

### Provider result

- Outcome: `glossed`
- Translation: `to lean, to rest`
- Raw Gloss: `to rest`
- Evidence IDs: `wikt:it:poggiare:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **FAIL** — Gloss "to rest" differs from translation "to lean, to rest" |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass — "to rest" drops *il gomito sul tavolo*; narrower than the translation "to lean, to rest" but correct for the sentence
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `it-macchina` — it `macchina` (`NOUN`)

- Expected verdict class: drop sentence material
- Expected Gloss: `car, machine`
- Representative sentence: `Le macchine erano parcheggiate fuori.`
- Notes: The sentence's plural must not turn the Gloss into "cars".

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `it` |
| target_language | `en` |
| canonical_lemma | `macchina` |
| upos | `NOUN` |
| target_word | `macchine` |
| example_sentence | `Le macchine erano parcheggiate fuori.` |
| candidate_senses | `e1 wikt:it:macchina:1: machine; e2 wikt:it:macchina:2: car, automobile` |

### Provider result

- Outcome: `glossed`
- Translation: `car`
- Raw Gloss: `car`
- Evidence IDs: `wikt:it:macchina:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `it-palazzo` — it `palazzo` (`NOUN`)

- Expected verdict class: protected contextual win
- Expected Gloss: `apartment building`
- Representative sentence: `Abitano in un palazzo del centro.`
- Notes: A genuine contextual improvement that the v16 contract must keep.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `it` |
| target_language | `en` |
| canonical_lemma | `palazzo` |
| upos | `NOUN` |
| target_word | `palazzo` |
| example_sentence | `Abitano in un palazzo del centro.` |
| candidate_senses | `e1 wikt:it:palazzo:1: palace; e2 wikt:it:palazzo:2: apartment building, block of flats` |

### Provider result

- Outcome: `glossed`
- Translation: `apartment building`
- Raw Gloss: `apartment building`
- Evidence IDs: `wikt:it:palazzo:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: pass — "apartment building"

## `it-ne` — it `ne` (`PRON`)

- Expected verdict class: spot check
- Expected Gloss: `about it`
- Representative sentence: `Ne parliamo domani.`
- Notes: Italian function word: the clitic's meaning needs the sentence's verb to read naturally.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `it` |
| target_language | `en` |
| canonical_lemma | `ne` |
| upos | `PRON` |
| target_word | `ne` |
| example_sentence | `Ne parliamo domani.` |
| candidate_senses | `e1 wikt:it:ne:1: of it, of them; about it` |

### Provider result

- Outcome: `glossed`
- Translation: `about it`
- Raw Gloss: `about it`
- Evidence IDs: `wikt:it:ne:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `it-in-gamba` — it `gamba` (`NOUN`)

- Expected verdict class: spot check
- Expected Gloss: `capable`
- Representative sentence: `È un ragazzo in gamba.`
- Notes: Idiom spot check: the whole phrase, not the literal leg, carries the meaning.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `it` |
| target_language | `en` |
| canonical_lemma | `gamba` |
| upos | `NOUN` |
| target_word | `gamba` |
| example_sentence | `È un ragazzo in gamba.` |
| candidate_senses | `e1 wikt:it:gamba:1: leg; e2 wikt:it:gamba:2: in gamba: capable, smart` |

### Provider result

- Outcome: `glossed`
- Translation: `capable, smart`
- Raw Gloss: `capable, smart`
- Evidence IDs: `wikt:it:gamba:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `el-tha` — el `θα` (`PART`)

- Expected verdict class: spot check
- Expected Gloss: `will`
- Representative sentence: `Θα πάμε αύριο.`
- Notes: Modern Greek function word.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `el` |
| target_language | `en` |
| canonical_lemma | `θα` |
| upos | `PART` |
| target_word | `θα` |
| example_sentence | `Θα πάμε αύριο.` |
| candidate_senses | `e1 wikt:el:tha:1: future particle marking future tense: will, shall` |

### Provider result

- Outcome: `glossed`
- Translation: `will`
- Raw Gloss: `will`
- Evidence IDs: `wikt:el:tha:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `el-kano-kryo` — el `κάνω` (`VERB`)

- Expected verdict class: spot check
- Expected Gloss: `to be (of weather)`
- Representative sentence: `Κάνει πολύ κρύο απόψε.`
- Notes: Modern Greek idiom: the weather expression uses κάνω, not a literal "make".

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `el` |
| target_language | `en` |
| canonical_lemma | `κάνω` |
| upos | `VERB` |
| target_word | `Κάνει` |
| example_sentence | `Κάνει πολύ κρύο απόψε.` |
| candidate_senses | `e1 wikt:el:kano:1: to do, to make; e2 wikt:el:kano:2: κάνει κρύο: it is cold (of the weather)` |

### Provider result

- Outcome: `glossed`
- Translation: `to do, to make`
- Raw Gloss: `to be cold`
- Evidence IDs: `wikt:el:kano:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **FAIL** — Gloss "to be cold" differs from translation "to do, to make" |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: known limitation, not counted against v17 — "cold" comes from κρύο; κάνει κρύο ("it is cold") is an idiom headed elsewhere (#2351), as in the v16 review
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `el-vivlio` — el `βιβλίο` (`NOUN`)

- Expected verdict class: plain equivalent
- Expected Gloss: `book`
- Representative sentence: `Διαβάζω ένα βιβλίο.`
- Notes: Modern Greek plain-equivalent spot check.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `el` |
| target_language | `en` |
| canonical_lemma | `βιβλίο` |
| upos | `NOUN` |
| target_word | `βιβλίο` |
| example_sentence | `Διαβάζω ένα βιβλίο.` |
| candidate_senses | `e1 wikt:el:vivlio:1: book` |

### Provider result

- Outcome: `glossed`
- Translation: `book`
- Raw Gloss: `book`
- Evidence IDs: `wikt:el:vivlio:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-ruft-an` — de `rufen` (`VERB`)

- Expected verdict class: lemma mismatch
- Representative sentence: `Er ruft seine Mutter jeden Abend an.`
- Notes: The sentence uses separable anrufen, but the analysis froze the bare base-verb lemma rufen.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `rufen` |
| upos | `VERB` |
| target_word | `ruft` |
| example_sentence | `Er ruft seine Mutter jeden Abend an.` |
| candidate_senses | `e1 wikt:de:rufen:1: to call, to shout; e2 wikt:de:rufen:2: to summon` |

### Provider result

- Outcome: `abstained`
- Translation: `to call, to shout`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `lemma_mismatch`
- Unresolved reason: `The sentence uses the separable verb anrufen, with an split from ruft; the supplied lemma is only rufen.`

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **not applicable** — no resolved Gloss |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No definition paraphrase — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No sentence inflection or arguments — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] Protected win kept — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`

## `de-steht-auf` — de `stehen` (`VERB`)

- Expected verdict class: lemma mismatch
- Representative sentence: `Sie steht jeden Morgen um sechs Uhr auf.`
- Notes: The sentence uses separable aufstehen (to get up), but the analysis froze the bare base-verb lemma stehen.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `stehen` |
| upos | `VERB` |
| target_word | `steht` |
| example_sentence | `Sie steht jeden Morgen um sechs Uhr auf.` |
| candidate_senses | `e1 wikt:de:stehen:1: to stand; e2 wikt:de:stehen:2: to be situated, to be located` |

### Provider result

- Outcome: `abstained`
- Translation: `to stand`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `lemma_mismatch`
- Unresolved reason: `The sentence uses the separable verb aufstehen, with steht ... auf, rather than the supplied lemma stehen.`

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **not applicable** — no resolved Gloss |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No definition paraphrase — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] No sentence inflection or arguments — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`
- [x] Protected win kept — verdict: n/a — no card; the provider correctly returned `lemma_mismatch`

## `de-kauft-ein` — de `kaufen` (`VERB`)

- Expected verdict class: lemma mismatch
- Representative sentence: `Sie kauft samstags auf dem Markt ein.`
- Notes: The sentence uses separable einkaufen (to shop), but the analysis froze the bare base-verb lemma kaufen.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `kaufen` |
| upos | `VERB` |
| target_word | `kauft` |
| example_sentence | `Sie kauft samstags auf dem Markt ein.` |
| candidate_senses | `e1 wikt:de:kaufen:1: to buy, to purchase` |

### Provider result

- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Translation: —
- Raw Gloss: —
- Evidence IDs: —
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **not applicable** — no resolved Gloss |
| outcome is `lemma_mismatch` | **FAIL** — expected lemma_mismatch=true, observed=false |

### Reviewer verdicts


- [ ] Citation form — verdict: n/a — no Gloss; the response was rejected as invalid ("translation is empty"), so `lemma_mismatch` was not observed (fails the gate)
- [ ] No definition paraphrase — verdict: n/a — no Gloss; the response was rejected as invalid ("translation is empty"), so `lemma_mismatch` was not observed (fails the gate)
- [ ] No sentence inflection or arguments — verdict: n/a — no Gloss; the response was rejected as invalid ("translation is empty"), so `lemma_mismatch` was not observed (fails the gate)
- [ ] Protected win kept — verdict: n/a — no Gloss; the response was rejected as invalid ("translation is empty"), so `lemma_mismatch` was not observed (fails the gate)

## `de-liegt-fern` — de `liegen` (`VERB`)

- Expected verdict class: plain equivalent
- Expected Gloss: `to lie`
- Representative sentence: `Das Dorf liegt fern der Stadt.`
- Notes: Lookalike control: fern is an adverb meaning 'far from', not the separable prefix of fernsehen. The sentence uses liegen and must be glossed, not flagged as a mismatch.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `liegen` |
| upos | `VERB` |
| target_word | `liegt` |
| example_sentence | `Das Dorf liegt fern der Stadt.` |
| candidate_senses | `e1 wikt:de:liegen:1: to lie; to be situated; e2 wikt:de:liegen:2: to be located` |

### Provider result

- Outcome: `glossed`
- Translation: `to lie; to be situated`
- Raw Gloss: `to be situated`
- Evidence IDs: `wikt:de:liegen:1, wikt:de:liegen:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **FAIL** — Gloss "to be situated" differs from translation "to lie; to be situated" |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass — "to be situated" fits *liegt fern der Stadt*; the lookalike control is correctly not a lemma mismatch
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case

## `de-fernsehen` — de `fernsehen` (`VERB`)

- Expected verdict class: plain equivalent
- Expected Gloss: `to watch television`
- Representative sentence: `Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.`
- Notes: Lookalike control: the same sentence with the correctly analysed separable verb fernsehen, which re-analysed books produce since #2329. It must be glossed, not flagged as a sehen mismatch.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `i1` |
| language | `de` |
| target_language | `en` |
| canonical_lemma | `fernsehen` |
| upos | `VERB` |
| target_word | `sieht` |
| example_sentence | `Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.` |
| candidate_senses | `e1 wikt:de:fernsehen:1: to watch television` |

### Provider result

- Outcome: `glossed`
- Translation: `to watch television`
- Raw Gloss: `to watch television`
- Evidence IDs: `wikt:de:fernsehen:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| Gloss equals translation | **PASS** |
| outcome is `lemma_mismatch` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass — the correctly lemmatised lookalike is glossed, not flagged
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Protected win kept — verdict: n/a — not a protected-win case


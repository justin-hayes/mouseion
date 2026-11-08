# Idiom card review — `idiom-card-v3`

Status: **Gate failed — Italian idiom cards stay off** (follow-up: #2389)

- Date: 2026-11-08
- Model: `gpt-5.6-luna`
- Reasoning setting: `medium`
- Prompt contract: `idiom-card-v3`
- Provider: `openai-compatible` (`gpt-5.6-luna/idiom-card-v3-reasoning-medium`)
- Case set: `idiom-review-v3` (13 cases)
- Matcher version: `idiom-matcher/2`
- Matcher gap bound: 2
- Sample: 30 confirmed idiom cards requested from Book `34c9236b-0e14-4e9d-aac7-02d10611c271` in `it` (seed `2026-11-08-v2`); 351 idiom items found, 57 sent, 30 confirmed
- Rubric: [doc/reviews/README.md](README.md)

## Summary

- Cases run: 13
- Outcomes: confirmed=7, literal_use=3, ambiguous=1, unresolved=0, invalid response=0, provider error=0, lemma-card controls=2
- Automatic checks: 12 passed, 2 failed

## Gate

The gate passes only when every item below is true:

- every positive case is confirmed with a citation-form Gloss;
- no literal-use control is confirmed, every ambiguous control returns `ambiguous`, and every lemma-card control passes its automatic check;
- none of the sampled cards teaches a wrong meaning.

- [x] Gate evaluated and recorded: **fail** — the `it-fare-bene-literal` control is confirmed, `it-casa-isolata-remote` returns `literal_use` instead of `ambiguous`, and one of the 30 sampled cards teaches a wrong meaning (18 *casa isolata*).

Verdicts drafted by Claude on 2026-11-08 at the operator's request; **awaiting operator confirmation**. This is a diagnostic run of `idiom-card-v3` and `idiom-review-v3` (#2390) with `MOUSEION_LLM_IDIOM_REASONING_EFFORT=medium`; the deployment runs idiom cards at `low`, recorded in [idiom-review-v3-2026-11-08.md](idiom-review-v3-2026-11-08.md). Same dictionary index, Book and seed.

Compared with the `low` run, `it-rispetto-a-comparative` passes (`ambiguous`), and the *casa isolata* case is omitted, but as `literal_use` ("a literal noun phrase meaning an isolated house"), so its automatic check still fails. Automatic checks: 12/14. The 27 unconfirmed sample items are correct omissions; *stazione di polizia* and *cavo elettrico* are again rejected as literal.

Remaining failures:

- `it-fare-bene-literal`: still confirmed as "to do well" (sense e1). Raising the effort does not change it.
- Sample 18, *casa isolata* → "detached house": the same sentence and senses as the case-set control, which this run omitted. The provider's answer for this sentence is not stable between two requests at the same setting.

## `it-in-gamba` — it `in gamba` (positive)

- Matched words: `in, gamba`
- Representative sentence: `È un ragazzo in gamba.`
- Expected Gloss: `capable`
- Notes: The anchor is gamba; the idiom means capable, not leg.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `in gamba` |
| matched_forms | `in, gamba` |
| example_sentence | `È un ragazzo in gamba.` |
| candidate_senses | `e1 wikt:in gamba:1: capable, on the ball` |

### Provider result

- Outcome: `confirmed`
- Translation: `capable`
- Raw Gloss: `capable`
- Evidence IDs: `wikt:in gamba:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass

## `it-in-bocca-al-lupo` — it `in bocca al lupo` (positive)

- Matched words: `in, bocca, al, lupo`
- Representative sentence: `In bocca al lupo per l'esame!`
- Expected Gloss: `good luck`
- Notes: A four-word wish; its Gloss must not paraphrase the wolf image.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `in bocca al lupo` |
| matched_forms | `in, bocca, al, lupo` |
| example_sentence | `In bocca al lupo per l'esame!` |
| candidate_senses | `e1 wikt:in bocca al lupo:1: good luck` |

### Provider result

- Outcome: `confirmed`
- Translation: `good luck`
- Raw Gloss: `good luck`
- Evidence IDs: `wikt:in bocca al lupo:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass

## `it-macchina-da-scrivere` — it `macchina da scrivere` (positive)

- Matched words: `macchina, da, scrivere`
- Representative sentence: `Ha comprato una macchina da scrivere.`
- Expected Gloss: `typewriter`
- Notes: A fixed compound is an idiom for this review.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `macchina da scrivere` |
| matched_forms | `macchina, da, scrivere` |
| example_sentence | `Ha comprato una macchina da scrivere.` |
| candidate_senses | `e1 wikt:macchina da scrivere:1: typewriter` |

### Provider result

- Outcome: `confirmed`
- Translation: `typewriter`
- Raw Gloss: `typewriter`
- Evidence IDs: `wikt:macchina da scrivere:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass

## `el-kano-ta-strava-matia` — el `κάνω τα στραβά μάτια` (positive)

- Matched words: `Έκανε, τα, στραβά, μάτια`
- Representative sentence: `Έκανε τα στραβά μάτια στην παράβαση.`
- Expected Gloss: `to turn a blind eye`
- Notes: The inflected verb is matched through its lemma.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `el` |
| target_language | `en` |
| headword | `κάνω τα στραβά μάτια` |
| matched_forms | `Έκανε, τα, στραβά, μάτια` |
| example_sentence | `Έκανε τα στραβά μάτια στην παράβαση.` |
| candidate_senses | `e1 reviewed:kano ta strava matia:1: to turn a blind eye` |

### Provider result

- Outcome: `confirmed`
- Translation: `turn a blind eye`
- Raw Gloss: `to turn a blind eye`
- Evidence IDs: `reviewed:kano ta strava matia:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass

## `it-tagliare-la-corda-literal` — it `tagliare la corda` (literal-use control)

- Matched words: `Tagliò, la, corda`
- Representative sentence: `Tagliò la corda del pacco.`
- Notes: The cord belongs to the parcel, so the words are literal.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `tagliare la corda` |
| matched_forms | `Tagliò, la, corda` |
| example_sentence | `Tagliò la corda del pacco.` |
| candidate_senses | `e1 wikt:tagliare la corda:1: to run away, to scarper` |

### Provider result

- Outcome: `literal_use`
- Translation: —
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The words are used literally: he cut the string of the package.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `literal_use` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] Meaning correct in context — verdict: pass — the cord belongs to the parcel; rejecting the idiom is correct

## `de-jemanden-auf-den-arm-nehmen-literal` — de `jemanden auf den Arm nehmen` (literal-use control)

- Matched words: `nimmt, auf, den, Arm`
- Representative sentence: `Sie nimmt das Kind auf den Arm.`
- Notes: She picks the child up; this is not the figurative idiom.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `de` |
| target_language | `en` |
| headword | `jemanden auf den Arm nehmen` |
| matched_forms | `nimmt, auf, den, Arm` |
| example_sentence | `Sie nimmt das Kind auf den Arm.` |
| candidate_senses | `e1 wikt:jemanden auf den Arm nehmen:1: to pull someone's leg` |

### Provider result

- Outcome: `literal_use`
- Translation: —
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The matched words are used literally: she takes the child in her arms, rather than teasing or fooling someone.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `literal_use` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] Meaning correct in context — verdict: pass — she physically picks the child up; rejecting the idiom is correct

## `it-di-fronte` — it `di fronte` (positive)

- Matched words: `di, fronte`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Expected Gloss: `in front, opposite`
- Notes: Card 4 of the 2026-11-08 review. The pooled adjective and adverb senses; the sentence uses the adverb, so the Gloss must not be the bare noun front.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `di fronte` |
| matched_forms | `di, fronte` |
| example_sentence | `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.` |
| candidate_senses | `e1 wikt:di fronte:ADJ:1: front, fore; e2 wikt:di fronte:ADV:1: opposite (directly across); e3 wikt:di fronte:ADV:2: in front of; e4 wikt:di fronte:ADV:3: in the wake of; in the face of` |

### Provider result

- Outcome: `confirmed`
- Translation: `in front of`
- Raw Gloss: `in front of`
- Evidence IDs: `wikt:di fronte:ADV:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass — "in front of" fits *me le sono trovate di fronte*; the provider cited the adverb sense

## `it-rispetto-a-comparative` — it `rispetto a` (ambiguous control)

- Matched words: `rispetto, a`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Notes: Card 13 of the 2026-11-08 review. The sentence uses the comparative sense compared with, which no candidate sense lists, so the card is omitted as ambiguous.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `rispetto a` |
| matched_forms | `rispetto, a` |
| example_sentence | `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?` |
| candidate_senses | `e1 wikt:rispetto a:1: with respect to; e2 wikt:rispetto a:2: in relation to` |

### Provider result

- Outcome: `ambiguous`
- Translation: —
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `ambiguous`
- Unresolved reason: `The sentence uses rispetto a in the comparative sense “compared to/than,” which is not among the candidate senses.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `ambiguous` | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced; the provider correctly returned `ambiguous`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider correctly returned `ambiguous`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider correctly returned `ambiguous`
- [x] Meaning correct in context — verdict: pass — the sentence uses the comparative "compared with", which no candidate sense lists; omitting the card is correct

## `it-fare-bene-literal` — it `fare bene` (literal-use control)

- Matched words: `fatte, bene`
- Representative sentence: `Quindi, tanto per essere sicuri e fare le cose fatte bene, ce lo hanno richiesto una terza.`
- Notes: Card 25 of the 2026-11-08 review. fatte bene modifies le cose (do things properly), and sense 3 marks the literal use.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `fare bene` |
| matched_forms | `fatte, bene` |
| example_sentence | `Quindi, tanto per essere sicuri e fare le cose fatte bene, ce lo hanno richiesto una terza.` |
| candidate_senses | `e1 wikt:fare bene:1: to do well (take the prudent approach); e2 wikt:fare bene:2: to do the right thing; e3 wikt:fare bene:3: Used other than figuratively or idiomatically: see fare, bene.` |

### Provider result

- Outcome: `confirmed`
- Translation: `to do things properly`
- Raw Gloss: `to do well`
- Evidence IDs: `wikt:fare bene:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `literal_use` | **FAIL** — expected "literal_use", observed "confirmed" |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [ ] Meaning correct in context — verdict: fail — *fare le cose fatte bene* is literal ("do things properly"); *fatte bene* modifies *le cose*, so there is no idiom. "to do well" (take the prudent approach) teaches a meaning the sentence does not have

## `it-in-giro` — it `in giro` (positive)

- Matched words: `in, giro`
- Representative sentence: `Non potevamo certo andare in giro per la città con quelli indossati durante il cammino, stracciati e puzzolenti.`
- Expected Gloss: `around, about`
- Gloss must leave out: `go, going, went, andare`
- Notes: Card 8 of the 2026-11-08 v2 re-run. andare governs the idiom, so the Gloss must not take the sentence's verb (to go around).

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `in giro` |
| matched_forms | `in, giro` |
| example_sentence | `Non potevamo certo andare in giro per la città con quelli indossati durante il cammino, stracciati e puzzolenti.` |
| candidate_senses | `e1 wikt:in giro:1: here and there, about, around` |

### Provider result

- Outcome: `confirmed`
- Translation: `around, about`
- Raw Gloss: `around`
- Evidence IDs: `wikt:in giro:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |
| Gloss leaves out the excluded sentence words | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass — the Gloss is the idiom alone, without the sentence's verb *andare*

## `it-casa-isolata-remote` — it `casa isolata` (ambiguous control)

- Matched words: `casa, isolata`
- Representative sentence: `La famiglia abitava fuori Torino, in una casa isolata, oltre le colline.`
- Notes: Card 18 of the 2026-11-08 v2 re-run. The sentence means a remote house beyond the hills; neither detached house nor homestead describes it, so the card is omitted as ambiguous.

### Request summary

| Field | Value |
| --- | --- |
| item_id | `c1` |
| language | `it` |
| target_language | `en` |
| headword | `casa isolata` |
| matched_forms | `casa, isolata` |
| example_sentence | `La famiglia abitava fuori Torino, in una casa isolata, oltre le colline.` |
| candidate_senses | `e1 wikt:casa isolata:1: detached house; e2 wikt:casa isolata:2: homestead` |

### Provider result

- Outcome: `literal_use`
- Translation: —
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The words form a literal noun phrase meaning an isolated house, not the supplied idiom.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `ambiguous` | **FAIL** — expected "ambiguous", observed "literal_use" |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced
- [x] No definition paraphrase — verdict: n/a — no card was produced
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced
- [x] Meaning correct in context — verdict: pass — no card was produced, so nothing wrong is taught; the provider returned `literal_use` ("a literal noun phrase meaning an isolated house") instead of `ambiguous`, which is why the automatic check fails

## `it-gamba-lemma-card` — it `in gamba` (lemma-card control)

- Notes: The anchor's own card must use the literal leg sentence.
- Anchor: `gamba` (`NOUN`)
- Idiom sentence: `È un ragazzo in gamba.`
- Expected lemma-card sentence: `Mi fa male la gamba.`

### Automatic checks

| Check | Result |
| --- | --- |
| lemma card moves off the idiom sentence | **PASS** |

### Reviewer verdicts

- [x] Anchor card moved to an idiom-free sentence — verdict: pass — the *gamba* card uses *Mi fa male la gamba.*

## `de-arm-lemma-card` — de `jemanden auf den Arm nehmen` (lemma-card control)

- Notes: The anchor's own card must use the broken-arm sentence.
- Anchor: `Arm` (`NOUN`)
- Idiom sentence: `Sie nimmt das Kind auf den Arm.`
- Expected lemma-card sentence: `Er hat sich den Arm gebrochen.`

### Automatic checks

| Check | Result |
| --- | --- |
| lemma card moves off the idiom sentence | **PASS** |

### Reviewer verdicts

- [x] Anchor card moved to an idiom-free sentence — verdict: pass — the *Arm* card uses *Er hat sich den Arm gebrochen.*

## Sample

The sample re-freezes the Book's current deck preparation in memory with idiom cards on for this language and no per-deck bound. Items are sent in seeded order, using the request a prepared deck would send, until enough are confirmed. Nothing is written and no assessment is recorded.

### Confirmed

#### 1. `in lontananza` — `lontananza` (`NOUN`)

- Matched words: `in, lontananza`
- Representative sentence: `Le ho viste in lontananza e subito non ho capito perché si fossero fermate.`
- Gloss: `in the distance`
- Translation: `in the distance`
- Sentence translation: `I saw them in the distance and immediately didn't understand why they had stopped.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 2. `sentire parlare` — `sentire` (`VERB`)

- Matched words: `sentir, parlare`
- Representative sentence: `Di lasciarmi solo, di notte, di sapermi addormentato su una panchina, non voleva proprio sentirne parlare.`
- Gloss: `to hear about`
- Translation: `to hear about`
- Sentence translation: `He absolutely did not want to hear about leaving me alone at night or knowing I had fallen asleep on a bench.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *non volerne sentir parlare* is "to refuse to hear of it"

#### 3. `di fronte` — `fronte` (`NOUN`)

- Matched words: `di, fronte`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Gloss: `in front of`
- Translation: `in front of`
- Sentence translation: `Behind a hairpin bend, I suddenly found them in front of me—the seated people.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — adverbial "in front of (me)", not the noun

#### 4. `avere la peggio` — `avere` (`VERB`)

- Matched words: `avuto, la, peggio`
- Representative sentence: `Il curdo ha avuto la peggio.`
- Gloss: `to get the worst of it`
- Translation: `to get the worst of it`
- Sentence translation: `The Kurd got the worst of it.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 5. `chiave inglese` — `inglese` (`ADJ`)

- Matched words: `chiave, inglese`
- Representative sentence: `Con una chiave inglese in mano.`
- Gloss: `wrench`
- Translation: `wrench`
- Sentence translation: `With a wrench in hand.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 6. `fino a` — `fino` (`ADV`)

- Matched words: `fino, a`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Gloss: `until`
- Translation: `until`
- Sentence translation: `I thought I could jump on him, cling to his jacket, and whine until my lungs or his ears burst, but I think I did the right thing by not doing it.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *fino a quando* = "until"

#### 7. `tanto per cambiare` — `tanto` (`ADV`)

- Matched words: `tanto, per, cambiare`
- Representative sentence: `Il primo giorno, tanto per cambiare, è stato pessimo, quasi peggio del primo giorno al samavat Qgazi.`
- Gloss: `as usual`
- Translation: `as usual`
- Sentence translation: `On the first day, as usual, it was terrible, almost worse than the first day at Samavat Qgazi.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *tanto per cambiare* is ironic "for a change", i.e. "as usual"

#### 8. `in giro` — `giro` (`NOUN`)

- Matched words: `in, giro`
- Representative sentence: `Non potevamo certo andare in giro per la città con quelli indossati durante il cammino, stracciati e puzzolenti.`
- Gloss: `out and about`
- Translation: `out and about`
- Sentence translation: `We certainly couldn't be out and about in the city in those clothes, torn and smelly from the journey.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — the Gloss is the idiom alone, without *andare*

#### 9. `dopo che` — `dopo` (`ADV`)

- Matched words: `dopo, che`
- Representative sentence: `Tre giorni dopo, più o meno, dopo che Sufi e io eravamo arrivati, ho visto un gruppo di operai discutere su chi dovesse andare a fare qualcosa da qualche parte.`
- Gloss: `after`
- Translation: `after`
- Sentence translation: `About three days later, more or less, after Sufi and I had arrived, I saw a group of workers discussing who should go and do something somewhere.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 10. `oltre a` — `oltre` (`ADV`)

- Matched words: `Oltre, a`
- Representative sentence: `Oltre a noi afghani, che eravamo i più giovani, c’erano curdi, pakistani, iracheni e qualche bengalese.`
- Gloss: `in addition to`
- Translation: `in addition to`
- Sentence translation: `In addition to us Afghans, who were the youngest, there were Kurds, Pakistanis, Iraqis, and a few Bangladeshis.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 11. `a lungo` — `lungo` (`ADJ`)

- Matched words: `a, lungo`
- Representative sentence: `Ho camminato a lungo, seguendo una strada poco trafficata.`
- Gloss: `for a long time`
- Translation: `for a long time`
- Sentence translation: `I walked for a long time, following a lightly trafficked road.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 12. `assieme a` — `assieme` (`ADV`)

- Matched words: `assieme, a`
- Representative sentence: `C’erano altri afghani assieme a lui.`
- Gloss: `together with`
- Translation: `together with`
- Sentence translation: `There were other Afghans together with him.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 13. `una volta` — `volta` (`NOUN`)

- Matched words: `una, volta`
- Representative sentence: `Sono dieci mesi di lavoro, ho detto a Wahid, che una volta aveva pensato di andarsene e poi non lo aveva fatto.`
- Gloss: `once`
- Translation: `once, formerly`
- Sentence translation: `It’s ten months of work, I told Wahid, who had once thought of leaving and then hadn’t done so.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — "once" fits *una volta aveva pensato*

#### 14. `ad alta voce` — `voce` (`NOUN`)

- Matched words: `ad, alta, voce`
- Representative sentence: `Il talebano, con il fucile, è entrato in classe e ha detto ad alta voce che bisognava chiudere la scuola, punto.`
- Gloss: `aloud`
- Translation: `aloud`
- Sentence translation: `The Taliban member, with the rifle, entered the classroom and said aloud that the school had to be closed, period.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 15. `da capo` — `capo` (`NOUN`)

- Matched words: `da, capo`
- Representative sentence: `Andare avanti o tornare indietro è la stessa cosa, e io preferisco morire in mare che dover rifare da capo tutta la strada che ho fatto fino ad adesso.`
- Gloss: `from scratch`
- Translation: `from scratch`
- Sentence translation: `Going forward or turning back is the same thing, and I would rather die at sea than have to redo the whole route I have taken so far from scratch.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *rifare da capo* = "redo from scratch"

#### 16. `in attesa` — `attesa` (`NOUN`)

- Matched words: `in, attesa`
- Representative sentence: `Sono rimasto rannicchiato, tranquillo, in attesa che il camion ripartisse.`
- Gloss: `waiting`
- Translation: `waiting`
- Sentence translation: `I remained crouched, calm, waiting for the truck to set off again.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *in attesa che* = "waiting for"

#### 17. `se stesso` — `stesso` (`ADJ`)

- Matched words: `se, stessi`
- Representative sentence: `Quando si avvicinavano dei camion che andavano nella nostra direzione, chiedevamo un passaggio (anche piccolo) agli autisti, che se erano belle persone si fermavano e ci caricavano, mentre se erano antipatici o arrabbiati con se stessi o con il mondo ci passavano accanto accelerando, coprendoci di polvere.`
- Gloss: `oneself`
- Translation: `oneself`
- Sentence translation: `When trucks heading in our direction approached, we would ask the drivers for a lift (even a short one); if they were nice people, they would stop and pick us up, whereas if they were unpleasant or angry with themselves or the world, they would drive past us faster, covering us in dust.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *arrabbiati con se stessi* = "angry with themselves"

#### 18. `casa isolata` — `casa` (`NOUN`)

- Matched words: `casa, isolata`
- Representative sentence: `La famiglia abitava fuori Torino, in una casa isolata, oltre le colline.`
- Gloss: `detached house`
- Translation: `detached house`
- Sentence translation: `The family lived outside Turin, in a detached house, beyond the hills.`

#### Reviewer verdicts

- [ ] Meaning correct in context — verdict: fail — "detached house" (not sharing walls) does not fit; *una casa isolata, oltre le colline* is a remote, isolated house. The provider confirmed a listed sense instead of returning `ambiguous`

#### 19. `per terra` — `terra` (`NOUN`)

- Matched words: `per, terra`
- Representative sentence: `Avrei fatto qualunque cosa, da lavare per terra a pulire le scarpe, qualunque cosa ci fosse da fare, e questo perché, a dirla tutta, avevo una gran paura a uscire per strada.`
- Gloss: `on the ground`
- Translation: `on the ground`
- Sentence translation: `I would have done anything, from washing the floor to cleaning shoes, anything there was to do, and this because, to be honest, I was very afraid to go out into the street.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *lavare per terra* = "wash the floor"; "on the ground/floor"

#### 20. `in effetti` — `effetto` (`NOUN`)

- Matched words: `In, effetti`
- Representative sentence: `In effetti avevo il rotolo dei soldi guadagnati al cantiere.`
- Gloss: `actually`
- Translation: `actually`
- Sentence translation: `Actually, I had the roll of money earned at the construction site.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 21. `posto di blocco` — `posto` (`NOUN`)

- Matched words: `posti, di, blocco`
- Representative sentence: `Un giorno, finito il turno di lavoro, un giorno come tanti, abbiamo raccolto le nostre cose dentro uno zainetto di tela, abbiamo salutato il capo della fabbrica, chiesto lo stipendio dovuto e con un pullman di linea (e la solita paura dei posti di blocco) siamo arrivati a Teheran.`
- Gloss: `checkpoint`
- Translation: `checkpoint`
- Sentence translation: `One day, at the end of the work shift, on an ordinary day like so many others, we gathered our things into a little canvas backpack, said goodbye to the factory manager, asked for the wages owed to us, and, on a regular bus (and with the usual fear of checkpoints), arrived in Tehran.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 22. `di notte` — `notte` (`NOUN`)

- Matched words: `di, notte`
- Representative sentence: `Per questo, credo, siamo partiti di notte, noi tre: io, mia madre e l’uomo - l’uomo che chiamerò semplicemente uomo - cui mamma aveva chiesto di accompagnarci.`
- Gloss: `at night`
- Translation: `at night`
- Sentence translation: `For this reason, I think, the three of us left at night: my mother, I, and the man—the man I will simply call man—to whom Mum had asked to accompany us.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 23. `a un passo da` — `passo` (`NOUN`)

- Matched words: `a, un, passo, da`
- Representative sentence: `La cima della montagna era lì a un passo da noi, vicina da raggiungerla con un salto.`
- Gloss: `one step away`
- Translation: `one step away from; very close to`
- Sentence translation: `The mountain peak was right there, one step away from us, close enough to reach with a jump.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — "very close to" / "one step away" fits *lì a un passo da noi*

#### 24. `fare finta` — `fare` (`VERB`)

- Matched words: `fatto, finta`
- Representative sentence: `Sono entrato, ho stretto il telefono in mano e ho fatto finta di essere lì a telefonare, invece spiavo i miei compagni, per vedere cosa combinavano.`
- Gloss: `to pretend`
- Translation: `to pretend`
- Sentence translation: `I came in, held the phone in my hand, and pretended to be there making a phone call, but instead I was spying on my classmates to see what they were up to.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 25. `in disparte` — `disparta` (`NOUN`)

- Matched words: `in, disparte`
- Representative sentence: `Gioma, detto Sufi, perché era uno che se ne stava in disparte, tranquillo e silenzioso come un monaco, anche se in certe occasioni, be’, era quello che creava più problemi di tutti.`
- Gloss: `apart`
- Translation: `apart`
- Sentence translation: `Gioma, nicknamed Sufi because he was someone who stayed apart, calm and silent like a monk, although on certain occasions, well, he was the one who caused the most trouble of all.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 26. `via da` — `via` (`ADV`)

- Matched words: `via, da`
- Representative sentence: `Io, via da Nava, non ci sarei mai voluto andare.`
- Gloss: `away from`
- Translation: `away from`
- Sentence translation: `I, away from Nava, would never have wanted to go there.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 27. `ogni tanto` — `tanto` (`NOUN`)

- Matched words: `Ogni, tanto`
- Representative sentence: `Ogni tanto si andava al porto, in tre o quattro, e si tentava di saltare su un camion.`
- Gloss: `occasionally`
- Translation: `now and then`
- Sentence translation: `Every now and then, we would go to the harbor, three or four of us, and try to jump onto a truck.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 28. `fare piacere` — `fare` (`VERB`)

- Matched words: `fatto, piacere`
- Representative sentence: `Mi hanno chiesto se andava tutto bene, se avevo bisogno di qualcosa, un gesto che mi ha fatto molto piacere.`
- Gloss: `to please`
- Translation: `to please`
- Sentence translation: `They asked me if everything was all right, if I needed anything, a gesture that pleased me very much.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 29. `in mente` — `mente` (`NOUN`)

- Matched words: `in, mente`
- Representative sentence: `A Roma sapevo come fare: avevo le istru-zioni in mente.`
- Gloss: `in mind`
- Translation: `in mind`
- Sentence translation: `In Rome I knew what to do: I had the instructions in mind.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

#### 30. `forte e chiaro` — `forte` (`ADJ`)

- Matched words: `forte, e, chiaro`
- Representative sentence: `Lo diremo forte e chiaro, Enaiat.`
- Gloss: `loud and clear`
- Translation: `loud and clear`
- Sentence translation: `We will say it loud and clear, Enaiat.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### Not confirmed

The provider did not confirm 27 sampled items. They would be idiom omissions in a prepared deck.

#### 1. `di fatto` — `fare` (`VERB`)

- Matched words: `di, fatto`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Outcome: `literal_use`
- Unresolved reason: `The words di fatto do not function together as the idiom; fatto belongs to the separate phrase aver fatto bene.`

#### 2. `mai più` — `più` (`ADV`)

- Matched words: `mai, più`
- Representative sentence: `Prima uno, poi due, poi tre, sembrava non finissero mai, certi più piccoli di me.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words belong to different phrases: "mai" means "ever" in "non finissero mai," while "più" means "more" in "più piccoli."`

#### 3. `far vedere` — `fare` (`VERB`)

- Matched words: `far, vedere`
- Representative sentence: `Non è il caso di farci vedere in giro come quattro cani randagi.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses the reflexive sense “to be seen” or “to show oneself,” which is not covered by the supplied transitive sense “to show.”`

#### 4. `in riga` — `riga` (`NOUN`)

- Matched words: `in, righe`
- Representative sentence: `Se non vi interessa, andate pure avanti a leggere: nelle prossime righe non muore nessuno e non vengono fornite informazioni fondamentali per la storia.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses nelle prossime righe literally to mean “in the next few lines,” not the idiom in riga.`

#### 5. `di mezzo` — `mezzo` (`NOUN`)

- Matched words: `di, mezzo`
- Representative sentence: `Prima cosa: i fari del mezzo con il motore rumoroso erano accesi e puntavano contro la porta; sono rimasto abbagliato.`
- Outcome: `literal_use`
- Unresolved reason: `The words occur in the literal phrase “del mezzo” meaning “of the vehicle,” not as the idiom “di mezzo.”`

#### 6. `a giorni` — `giorno` (`NOUN`)

- Matched words: `a, giorni`
- Representative sentence: `Quando ho trovato il porto ho trovato anche un sacco di altri ragazzi afghani che erano lì a gironzolare da giorni e giorni nel tentativo di acquistare il biglietto, e ogni volta che ci provavano li cacciavano, perché si capiva subito che non erano clienti normali, che erano clandestini.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words do not form the idiom “a giorni” here: “a” belongs to “a gironzolare,” while “giorni e giorni” is a literal duration phrase.`

#### 7. `in bianco` — `bianco` (`ADJ`)

- Matched words: `in, bianca`
- Representative sentence: `Al mattino presto, nel silenzio e nella luce bianca dell’alba, abbiamo raggiunto un piccolo villaggio.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses bianca literally as part of luce bianca ('white light'), not the idiom in bianco.`

#### 8. `a vita` — `vita` (`NOUN`)

- Matched words: `a, vita`
- Representative sentence: `Ho imparato molte cose che mi hanno costretto a guardare la vita con occhi diversi, come quando ti metti un paio di occhiali da sole con le lenti colorate.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “a” with the infinitive “guardare,” while “vita” belongs to the separate phrase “la vita”; the matched forms do not form the idiom “a vita”.`

#### 9. `come non mai` — `non` (`ADV`)

- Matched words: `come, non, mai`
- Representative sentence: `Ci siamo guardati gli uni gli altri come non ci fossimo mai visti, come se a camminare non fossimo stati noi.`
- Outcome: `literal_use`
- Unresolved reason: `The words occur in the literal construction “come non ci fossimo mai visti” (‘as if we had never seen each other’), not as the idiom “come non mai.”`

#### 10. `se non che` — `non` (`ADV`)

- Matched words: `se, non, che`
- Representative sentence: `Ho già detto - se non sbaglio - che pensavo spesso a questa cosa di avere un orologio, giusto per dare un significato al passare del tempo, un orologio che segnasse la data, che mi dicesse di quanto stavo invecchiando.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words do not form the idiom “se non che” here: “se non sbaglio” means “if I’m not mistaken,” while “che” introduces a separate clause.`

#### 11. `hai voglia` — `avere` (`VERB`)

- Matched words: `hai, voglia`
- Representative sentence: `Se poi hai voglia, fai il primo turno di guardia.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses hai voglia to mean “feel like it” or “want to,” but the supplied sense “you bet” does not match.`

#### 12. `a cuore` — `cuore` (`NOUN`)

- Matched words: `a, cuore`
- Representative sentence: `Un ragazzo bengalese, una mattina presto - era buio e ci stavamo arrampicando sulle rocce usando le mani e le ginocchia - ha avuto un problema, non so cosa, forse di respirazione, o al cuore, è scivolato per diversi metri sulla neve.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses cuore literally to refer to the heart, in the contracted phrase al cuore, not the idiom a cuore.`

#### 13. `poi che` — `poi` (`ADV`)

- Matched words: `Poi, che`
- Representative sentence: `Poi abbiamo scoperto che Farid, un ragazzo che lavorava in una fabbrica dietro la nostra, aveva anche lui in mente di lasciare Qom.`
- Outcome: `literal_use`
- Unresolved reason: `“Poi” means “then” and “che” introduces a clause; they do not form the idiom “poi che” here.`

#### 14. `lo stesso` — `stesso` (`ADJ`)

- Matched words: `lo, stesso`
- Representative sentence: `Sentivo freddo e caldo nello stesso momento.`
- Outcome: `literal_use`
- Unresolved reason: `The words occur in the literal phrase “nello stesso momento,” meaning “at the same time,” rather than functioning as the idiom “lo stesso.”`

#### 15. `di troppo` — `troppo` (`ADV`)

- Matched words: `di, troppo`
- Representative sentence: `Un giorno, in una pausa pranzo, ho anche parlato con un ragazzo che aveva mezza faccia rovinata, davvero, ridotta a un hamburger; come quelli di McDonald’s lasciati troppo a lungo sulla piastra.`
- Outcome: `literal_use`
- Unresolved reason: `The matched forms do not occur together as the idiom: the sentence contains “troppo a lungo,” while no corresponding “di troppo” phrase is present.`

#### 16. `come mai` — `mai` (`ADV`)

- Matched words: `come, mai`
- Representative sentence: `Ce ne sono che hanno un odore e un sapore buono e ti sussurrano alle orecchie che sapranno farti stare meglio di come tu potrai mai stare senza di loro.`
- Outcome: `literal_use`
- Unresolved reason: `“come” and “mai” belong to different functions in the sentence: “come” introduces a comparison, while “mai” modifies “potrai stare” to mean “ever.”`

#### 17. `in tasca` — `tasca` (`NOUN`)

- Matched words: `in, tasca`
- Representative sentence: `Avevo in tasca duecento euro, risparmi della Grecia.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase is used literally: it refers to euros being physically in a pocket, not to something being certain to occur.`

#### 18. `mai dire mai` — `dire` (`VERB`)

- Matched words: `mai, dico, mai`
- Representative sentence: `Ma né io né Sufi avevamo un orologio, anzi, né io né Sufi avevamo mai, dico mai, posseduto un orologio in vita nostra.`
- Outcome: `literal_use`
- Unresolved reason: `The words form an emphatic repetition meaning “ever, I mean ever,” not the idiom “mai dire mai” (“never say never”).`

#### 19. `in testa` — `testa` (`NOUN`)

- Matched words: `in, testa`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses in testa to mean having something in mind, but no candidate sense provides that meaning.`

#### 20. `in vita` — `vita` (`NOUN`)

- Matched words: `in, vita`
- Representative sentence: `Tre cose non devi mai fare nella vita, Enaiat jan, per nessun motivo.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses in vita to mean in life, but the supplied candidate sense means alive.`

#### 21. `prendere fuoco` — `prendere` (`VERB`)

- Matched words: `preso, fuoco`
- Representative sentence: `La guancia ha preso fuoco, l’orecchio ha fischiato per alcuni secondi, poi ho avuto l’impressione che si stesse gonfiando come una focaccia.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses prendere fuoco to mean that the cheek started burning or flushed, but the supplied candidate sense only means catch fire.`

#### 22. `stazione di polizia` — `polizia` (`NOUN`)

- Matched words: `stazione, di, polizia`
- Representative sentence: `E dovevi darglieli subito, i soldi, se no ti portavano alla stazione di polizia e ti gonfiavano di botte: pugni e calci.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words are used literally as a noun phrase meaning a police station, not as an idiom.`

#### 23. `in sonno` — `sonno` (`NOUN`)

- Matched words: `in, sonno`
- Representative sentence: `Dell’assenza di Sufi me ne accorgevo soprattutto la notte, quando mi giravo nel sonno e le braccia e le mani non lo trovavano sul tappeto accanto a me.`
- Outcome: `literal_use`
- Unresolved reason: `“in sonno” is used literally to mean “in my sleep,” not as the supplied idiom.`

#### 24. `passare oltre` — `passare` (`VERB`)

- Matched words: `passare, oltre`
- Representative sentence: `Arrivato a me ha fatto per passare oltre, poi si è fermato ed è tornato indietro.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase is used literally: he was about to pass by, not to move on from a topic or involvement.`

#### 25. `cavo elettrico` — `elettrico` (`ADJ`)

- Matched words: `cavo, elettrico`
- Representative sentence: `Un cavo elettrico, rattoppato con dello scotch, saliva lungo il muro esterno del palazzo, entrava dalla finestra, correva per il soffitto e pendeva vicino alla porta del corridoio, insieme a una lampadina.`
- Outcome: `literal_use`
- Unresolved reason: `The words refer literally to an electrical cable in the sentence, rather than functioning as an idiom.`

#### 26. `a voce` — `voce` (`NOUN`)

- Matched words: `a, voce`
- Representative sentence: `Payam ha insistito fino a restare senza voce, come farebbe chiunque con una persona cui vuole bene o di cui si sente responsabile.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “senza voce,” not the supplied phrase “a voce”; the matched words do not form the idiom here.`

#### 27. `a tempo` — `tempo` (`NOUN`)

- Matched words: `a, tempo`
- Representative sentence: `Quando ho pagato la mia parte di spesa mensile ne erano rimasti trentacinquemila, che era come dire trentacinque euro, più o meno, dato che, se non sbaglio, mille toman valevano un euro, a quel tempo.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “a quel tempo” (“at that time”), not the idiom “a tempo”; the matched words are separated by quel and do not form the supplied idiom.`

## Prepared-deck bound

The sample above ignores the per-deck bound. A prepared deck keeps the first 30 of the 351 ranked idioms; noun-only idioms rank after every other idiom. This list is not a gate criterion.

| Rank | Headword | Anchor | Anchor occurrences | Noun-only |
| --- | --- | --- | --- | --- |
| 1 | `se non` | `non` (`ADV`) | 781 | — |
| 2 | `non affatto` | `non` (`ADV`) | 781 | — |
| 3 | `non ancora` | `non` (`ADV`) | 781 | — |
| 4 | `non lo so` | `non` (`ADV`) | 781 | — |
| 5 | `non potere che` | `non` (`ADV`) | 781 | — |
| 6 | `non fare che` | `non` (`ADV`) | 781 | — |
| 7 | `non si sa mai` | `non` (`ADV`) | 781 | — |
| 8 | `come non mai` | `non` (`ADV`) | 781 | — |
| 9 | `non appena` | `non` (`ADV`) | 781 | — |
| 10 | `non resta che` | `non` (`ADV`) | 781 | — |
| 11 | `se non che` | `non` (`ADV`) | 781 | — |
| 12 | `a dire il vero` | `dire` (`VERB`) | 476 | — |
| 13 | `volere dire` | `dire` (`VERB`) | 476 | — |
| 14 | `mai dire mai` | `dire` (`VERB`) | 476 | — |
| 15 | `far capire` | `fare` (`VERB`) | 438 | — |
| 16 | `di fatto` | `fare` (`VERB`) | 438 | — |
| 17 | `fare piacere` | `fare` (`VERB`) | 438 | — |
| 18 | `far sapere` | `fare` (`VERB`) | 438 | — |
| 19 | `fare schifo` | `fare` (`VERB`) | 438 | — |
| 20 | `fare bene` | `fare` (`VERB`) | 438 | — |
| 21 | `far vedere` | `fare` (`VERB`) | 438 | — |
| 22 | `fare le spese` | `fare` (`VERB`) | 438 | — |
| 23 | `fare confusione` | `fare` (`VERB`) | 438 | — |
| 24 | `fare finta di nulla` | `fare` (`VERB`) | 438 | — |
| 25 | `fare in modo` | `fare` (`VERB`) | 438 | — |
| 26 | `fare attenzione` | `fare` (`VERB`) | 438 | — |
| 27 | `fare finta` | `fare` (`VERB`) | 438 | — |
| 28 | `fare cenno` | `fare` (`VERB`) | 438 | — |
| 29 | `fare finta di niente` | `fare` (`VERB`) | 438 | — |
| 30 | `fare il pieno` | `fare` (`VERB`) | 438 | — |

- Idiom overflow: 321

# Idiom card review — `idiom-card-v3`

Status: **Gate passed — Italian idiom cards may be enabled** (#2389; awaiting operator confirmation)

- Date: 2026-11-08
- Model: `gpt-5.6-luna`
- Reasoning setting: `medium`
- Prompt contract: `idiom-card-v3`
- Provider: `openai-compatible` (`gpt-5.6-luna/idiom-card-v3-reasoning-medium`)
- Case set: `idiom-review-v3` (13 cases)
- Matcher version: `idiom-matcher/2`
- Matcher gap bound: 2
- Sample: 30 confirmed idiom cards requested from Book `34c9236b-0e14-4e9d-aac7-02d10611c271` in `it` (seed `2026-11-08-v2`); 351 idiom items found, 56 sent, 30 confirmed
- Rubric: [doc/reviews/README.md](README.md)

## Summary

- Cases run: 13
- Outcomes: confirmed=9, literal_use=2, ambiguous=0, unresolved=0, invalid response=0, provider error=0, lemma-card controls=2
- Automatic checks: 11 passed, 3 failed

## Gate

The gate passes only when every item below is true:

- every positive case is confirmed with a citation-form Gloss;
- every lemma-card control passes;
- no card, in a control or the sample, teaches a misleading meaning;
- at most 3 cards, across the controls and the sample, are near-misses.

A control whose card is omitted, as `literal_use` or `ambiguous`, passes. A control whose card is confirmed is graded like any other card. Grade each card's meaning in context as correct, near-miss (close, and would not mislead a learner) or misleading (ADR 0115).

- [x] Gate evaluated and recorded: **pass** — every positive case is confirmed with a citation-form Gloss, both lemma-card controls pass, the sample has 30 confirmed cards, no card is misleading, and 3 cards are near-misses (the `it-rispetto-a-comparative`, `it-fare-bene-literal` and `it-casa-isolata-remote` controls), the most the gate allows.

Verdicts drafted by Claude on 2026-11-08 at the operator's request; **awaiting operator confirmation**, which merging this report records. This is the run of record under [ADR 0115](../adr/0115-idiom-review-gate-tolerates-near-misses.md): one fresh run of `idiom-card-v3` and `idiom-review-v3` at `MOUSEION_LLM_IDIOM_REASONING_EFFORT=medium`, against the same dictionary index, Book and seed as the earlier runs. Its result is accepted without re-running.

The gate text above was corrected by hand after the run. The command first printed an extra item, "every literal-use and ambiguous control passes its automatic check", which brought back the zero-tolerance rule ADR 0115 removes; the command now prints the gate as decided. Under that extra item this run would have failed.

Notes:

- All three confirmed controls are near-misses agreed with the operator before this run: "in relation to" for a comparative *rispetto a*, "to do well" for the literal *fare le cose fatte bene*, and "detached house" for a remote *casa isolata*. The earlier `medium` run omitted *rispetto a* and *casa isolata*; this run confirmed them. The provider's answers for these sentences vary between runs.
- No sampled card is misleading. *casa isolata* is not among the 30 this time: its sample request returned an invalid response.
- Of the 26 unconfirmed sample items, 19 are `literal_use` omissions and 4 are `ambiguous` omissions where no listed sense fits; all are correct apart from *cavo elettrico* below. *di mezzo* and *a cuore* returned provider errors and *casa isolata* an invalid response; each omits a card and teaches nothing.
- *cavo elettrico* is rejected as literal although it is a fixed compound; that omits a card but teaches nothing wrong.

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
- Translation: `capable, on the ball`
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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

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
- Unresolved reason: `The words are used literally: he cut the string belonging to the package, not the idiom meaning to run away.`

### Automatic checks

| Check | Result |
| --- | --- |
| card is omitted (`literal_use` or `ambiguous`) | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — the card is omitted; the cord belongs to the parcel

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
- Translation: `to take someone in one's arms`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The sentence uses the words literally: she picks up the child, rather than teasing or fooling someone.`

### Automatic checks

| Check | Result |
| --- | --- |
| card is omitted (`literal_use` or `ambiguous`) | **PASS** |

### Reviewer verdicts


- [x] Citation form — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider returned `literal_use`
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — the card is omitted; she physically picks the child up

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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — "in front of" fits *me le sono trovate di fronte*; the provider cited the adverb sense

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

- Outcome: `confirmed`
- Translation: `in relation to`
- Raw Gloss: `in relation to`
- Evidence IDs: `wikt:rispetto a:2`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| card is omitted (`literal_use` or `ambiguous`) | **FAIL** — expected "literal_use" or "ambiguous", observed "confirmed" |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning in context (correct, near-miss or misleading) — verdict: near-miss — the sentence uses the comparative "compared with"; "in relation to" is close and does not mislead about the sentence, but the card should have been omitted

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
- Translation: `to do well`
- Raw Gloss: `to do well`
- Evidence IDs: `wikt:fare bene:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| card is omitted (`literal_use` or `ambiguous`) | **FAIL** — expected "literal_use" or "ambiguous", observed "confirmed" |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning in context (correct, near-miss or misleading) — verdict: near-miss — *fare le cose fatte bene* is "do things properly" and *fatte bene* modifies *le cose*; "to do well" is close to the literal "done well", but it is not the idiom

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
- Translation: `around`
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
- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — "around", without the sentence's verb *andare*

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

- Outcome: `confirmed`
- Translation: `detached house`
- Raw Gloss: `detached house`
- Evidence IDs: `wikt:casa isolata:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| card is omitted (`literal_use` or `ambiguous`) | **FAIL** — expected "literal_use" or "ambiguous", observed "confirmed" |

### Reviewer verdicts


- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning in context (correct, near-miss or misleading) — verdict: near-miss — *una casa isolata, oltre le colline* is a remote house; "detached house" (standing on its own) is close but not the sense

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

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 2. `far vedere` — `fare` (`VERB`)

- Matched words: `far, vedere`
- Representative sentence: `Non è il caso di farci vedere in giro come quattro cani randagi.`
- Gloss: `to show`
- Translation: `to show`
- Sentence translation: `There's no point in letting ourselves be seen around like four stray dogs.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — *farci vedere in giro* = "show ourselves around"; "to show" is the citation form

#### 3. `sentire parlare` — `sentire` (`VERB`)

- Matched words: `sentir, parlare`
- Representative sentence: `Di lasciarmi solo, di notte, di sapermi addormentato su una panchina, non voleva proprio sentirne parlare.`
- Gloss: `to hear about`
- Translation: `to hear about`
- Sentence translation: `They absolutely did not want to hear about leaving me alone at night, knowing I was asleep on a bench.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 4. `di fronte` — `fronte` (`NOUN`)

- Matched words: `di, fronte`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Gloss: `in front of`
- Translation: `in front of`
- Sentence translation: `Behind a hairpin bend, suddenly I found them in front of me—the seated people.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 5. `avere la peggio` — `avere` (`VERB`)

- Matched words: `avuto, la, peggio`
- Representative sentence: `Il curdo ha avuto la peggio.`
- Gloss: `to lose`
- Translation: `to get the worst of it`
- Sentence translation: `The Kurd got the worst of it.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — "to lose" / "to get the worst of it"

#### 6. `chiave inglese` — `inglese` (`ADJ`)

- Matched words: `chiave, inglese`
- Representative sentence: `Con una chiave inglese in mano.`
- Gloss: `wrench`
- Translation: `wrench, monkey wrench`
- Sentence translation: `With a wrench in hand.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 7. `fino a` — `fino` (`ADV`)

- Matched words: `fino, a`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Gloss: `until`
- Translation: `until`
- Sentence translation: `I thought I could jump on him, cling to his jacket, and screech until my lungs or his ears burst, but I think I did the right thing by not doing it.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 8. `tanto per cambiare` — `tanto` (`ADV`)

- Matched words: `tanto, per, cambiare`
- Representative sentence: `Il primo giorno, tanto per cambiare, è stato pessimo, quasi peggio del primo giorno al samavat Qgazi.`
- Gloss: `as usual`
- Translation: `as usual`
- Sentence translation: `The first day, as usual, was awful, almost worse than the first day at samavat Qgazi.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — ironic "for a change", i.e. "as usual"

#### 9. `in giro` — `giro` (`NOUN`)

- Matched words: `in, giro`
- Representative sentence: `Non potevamo certo andare in giro per la città con quelli indossati durante il cammino, stracciati e puzzolenti.`
- Gloss: `around`
- Translation: `around`
- Sentence translation: `We certainly couldn't go around town wearing those after the journey; they were torn and smelly.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — the Gloss is the idiom alone, without *andare*

#### 10. `dopo che` — `dopo` (`ADV`)

- Matched words: `dopo, che`
- Representative sentence: `Tre giorni dopo, più o meno, dopo che Sufi e io eravamo arrivati, ho visto un gruppo di operai discutere su chi dovesse andare a fare qualcosa da qualche parte.`
- Gloss: `after`
- Translation: `after`
- Sentence translation: `About three days later, more or less, after Sufi and I had arrived, I saw a group of workers discussing who should go do something somewhere.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 11. `oltre a` — `oltre` (`ADV`)

- Matched words: `Oltre, a`
- Representative sentence: `Oltre a noi afghani, che eravamo i più giovani, c’erano curdi, pakistani, iracheni e qualche bengalese.`
- Gloss: `in addition to`
- Translation: `in addition to`
- Sentence translation: `In addition to us Afghans, who were the youngest, there were Kurds, Pakistanis, Iraqis, and a few Bangladeshis.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 12. `a lungo` — `lungo` (`ADJ`)

- Matched words: `a, lungo`
- Representative sentence: `Ho camminato a lungo, seguendo una strada poco trafficata.`
- Gloss: `for a long time`
- Translation: `for a long time`
- Sentence translation: `I walked for a long time, following a lightly trafficked road.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 13. `assieme a` — `assieme` (`ADV`)

- Matched words: `assieme, a`
- Representative sentence: `C’erano altri afghani assieme a lui.`
- Gloss: `together with`
- Translation: `together with`
- Sentence translation: `There were other Afghans with him.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 14. `una volta` — `volta` (`NOUN`)

- Matched words: `una, volta`
- Representative sentence: `Sono dieci mesi di lavoro, ho detto a Wahid, che una volta aveva pensato di andarsene e poi non lo aveva fatto.`
- Gloss: `once`
- Translation: `once, formerly`
- Sentence translation: `It’s ten months of work, I told Wahid, who had once thought of leaving and then didn’t.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 15. `ad alta voce` — `voce` (`NOUN`)

- Matched words: `ad, alta, voce`
- Representative sentence: `Il talebano, con il fucile, è entrato in classe e ha detto ad alta voce che bisognava chiudere la scuola, punto.`
- Gloss: `aloud`
- Translation: `aloud`
- Sentence translation: `The Taliban member, carrying a rifle, entered the classroom and said aloud that the school had to be closed, period.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 16. `da capo` — `capo` (`NOUN`)

- Matched words: `da, capo`
- Representative sentence: `Andare avanti o tornare indietro è la stessa cosa, e io preferisco morire in mare che dover rifare da capo tutta la strada che ho fatto fino ad adesso.`
- Gloss: `from scratch`
- Translation: `from scratch`
- Sentence translation: `Going forward or going back is the same thing, and I would rather die at sea than have to redo the whole journey I have made so far from scratch.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 17. `in attesa` — `attesa` (`NOUN`)

- Matched words: `in, attesa`
- Representative sentence: `Sono rimasto rannicchiato, tranquillo, in attesa che il camion ripartisse.`
- Gloss: `waiting`
- Translation: `waiting`
- Sentence translation: `I remained crouched, calm, waiting for the truck to set off again.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 18. `se stesso` — `stesso` (`ADJ`)

- Matched words: `se, stessi`
- Representative sentence: `Quando si avvicinavano dei camion che andavano nella nostra direzione, chiedevamo un passaggio (anche piccolo) agli autisti, che se erano belle persone si fermavano e ci caricavano, mentre se erano antipatici o arrabbiati con se stessi o con il mondo ci passavano accanto accelerando, coprendoci di polvere.`
- Gloss: `oneself`
- Translation: `oneself`
- Sentence translation: `When trucks heading in our direction approached, we would ask the drivers for a lift (even a short one); if they were nice people, they would stop and pick us up, whereas if they were unpleasant or angry with themselves or the world, they would drive past us, speeding up and covering us with dust.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — *arrabbiati con se stessi*; "oneself" is the citation form

#### 19. `per terra` — `terra` (`NOUN`)

- Matched words: `per, terra`
- Representative sentence: `Avrei fatto qualunque cosa, da lavare per terra a pulire le scarpe, qualunque cosa ci fosse da fare, e questo perché, a dirla tutta, avevo una gran paura a uscire per strada.`
- Gloss: `on the ground`
- Translation: `on the ground`
- Sentence translation: `I would have done anything, from washing the floor to cleaning shoes, anything there was to do, and this because, to tell the truth, I was terribly afraid to go out into the street.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — *lavare per terra* = "wash the floor"; "on the ground/floor"

#### 20. `in effetti` — `effetto` (`NOUN`)

- Matched words: `In, effetti`
- Representative sentence: `In effetti avevo il rotolo dei soldi guadagnati al cantiere.`
- Gloss: `actually`
- Translation: `actually`
- Sentence translation: `Actually, I had the roll of money earned at the construction site.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 21. `posto di blocco` — `posto` (`NOUN`)

- Matched words: `posti, di, blocco`
- Representative sentence: `Un giorno, finito il turno di lavoro, un giorno come tanti, abbiamo raccolto le nostre cose dentro uno zainetto di tela, abbiamo salutato il capo della fabbrica, chiesto lo stipendio dovuto e con un pullman di linea (e la solita paura dei posti di blocco) siamo arrivati a Teheran.`
- Gloss: `checkpoint`
- Translation: `checkpoint`
- Sentence translation: `One day, having finished our work shift—a day like any other—we gathered our things in a canvas backpack, said goodbye to the factory manager, asked for the wages owed to us, and took a regular bus (with the usual fear of checkpoints) to Tehran.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 22. `di notte` — `notte` (`NOUN`)

- Matched words: `di, notte`
- Representative sentence: `Per questo, credo, siamo partiti di notte, noi tre: io, mia madre e l’uomo - l’uomo che chiamerò semplicemente uomo - cui mamma aveva chiesto di accompagnarci.`
- Gloss: `at night`
- Translation: `at night`
- Sentence translation: `For this reason, I think, the three of us set off at night: me, my mother, and the man—the man whom I will simply call man—to whom Mom had asked to accompany us.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 23. `a un passo da` — `passo` (`NOUN`)

- Matched words: `a, un, passo, da`
- Representative sentence: `La cima della montagna era lì a un passo da noi, vicina da raggiungerla con un salto.`
- Gloss: `close to`
- Translation: `one step away from`
- Sentence translation: `The mountain summit was one step away from us, close enough to reach with a jump.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — "very close to" fits *lì a un passo da noi*

#### 24. `stazione di polizia` — `polizia` (`NOUN`)

- Matched words: `stazione, di, polizia`
- Representative sentence: `E dovevi darglieli subito, i soldi, se no ti portavano alla stazione di polizia e ti gonfiavano di botte: pugni e calci.`
- Gloss: `police station`
- Translation: `police station`
- Sentence translation: `And you had to give him the money right away, otherwise they would take you to the police station and beat you up: punches and kicks.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 25. `fare finta` — `fare` (`VERB`)

- Matched words: `fatto, finta`
- Representative sentence: `Sono entrato, ho stretto il telefono in mano e ho fatto finta di essere lì a telefonare, invece spiavo i miei compagni, per vedere cosa combinavano.`
- Gloss: `to pretend`
- Translation: `to pretend`
- Sentence translation: `I went in, held the phone in my hand, and pretended to be there making a phone call; instead, I was spying on my classmates to see what they were up to.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 26. `in disparte` — `disparta` (`NOUN`)

- Matched words: `in, disparte`
- Representative sentence: `Gioma, detto Sufi, perché era uno che se ne stava in disparte, tranquillo e silenzioso come un monaco, anche se in certe occasioni, be’, era quello che creava più problemi di tutti.`
- Gloss: `aside`
- Translation: `aside`
- Sentence translation: `Gioma, called Sufi, because he was someone who stayed apart, quiet and silent like a monk, although on certain occasions, well, he was the one who caused the most trouble of all.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 27. `via da` — `via` (`ADV`)

- Matched words: `via, da`
- Representative sentence: `Io, via da Nava, non ci sarei mai voluto andare.`
- Gloss: `away from`
- Translation: `away from`
- Sentence translation: `I would never have wanted to go there, away from Nava.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — *via da Nava* = "away from Nava"

#### 28. `ogni tanto` — `tanto` (`NOUN`)

- Matched words: `Ogni, tanto`
- Representative sentence: `Ogni tanto si andava al porto, in tre o quattro, e si tentava di saltare su un camion.`
- Gloss: `every now and then`
- Translation: `every now and then`
- Sentence translation: `Every now and then they would go to the port, three or four of them, and try to jump onto a truck.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

#### 29. `fare piacere` — `fare` (`VERB`)

- Matched words: `fatto, piacere`
- Representative sentence: `Mi hanno chiesto se andava tutto bene, se avevo bisogno di qualcosa, un gesto che mi ha fatto molto piacere.`
- Gloss: `to please`
- Translation: `to please`
- Sentence translation: `They asked me if everything was okay, if I needed anything, a gesture that pleased me very much.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct — *mi ha fatto molto piacere* = "it pleased me a lot"

#### 30. `in mente` — `mente` (`NOUN`)

- Matched words: `in, mente`
- Representative sentence: `A Roma sapevo come fare: avevo le istru-zioni in mente.`
- Gloss: `in mind`
- Translation: `in mind`
- Sentence translation: `In Rome I knew how to do it: I had the instructions in mind.`

#### Reviewer verdicts

- [x] Meaning in context (correct, near-miss or misleading) — verdict: correct

### Not confirmed

The provider did not confirm 26 sampled items. They would be idiom omissions in a prepared deck.

#### 1. `di fatto` — `fare` (`VERB`)

- Matched words: `di, fatto`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words do not form di fatto here: di belongs to the infinitive construction di aver fatto, while fatto is part of aver fatto bene.`

#### 2. `mai più` — `più` (`ADV`)

- Matched words: `mai, più`
- Representative sentence: `Prima uno, poi due, poi tre, sembrava non finissero mai, certi più piccoli di me.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words belong to different phrases: mai means “ever” in non finissero mai, while più modifies piccoli.`

#### 3. `in riga` — `riga` (`NOUN`)

- Matched words: `in, righe`
- Representative sentence: `Se non vi interessa, andate pure avanti a leggere: nelle prossime righe non muore nessuno e non vengono fornite informazioni fondamentali per la storia.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses nelle prossime righe literally to mean the next lines of text, not the idiom in riga.`

#### 4. `di mezzo` — `mezzo` (`NOUN`)

- Matched words: `di, mezzo`
- Representative sentence: `Prima cosa: i fari del mezzo con il motore rumoroso erano accesi e puntavano contro la porta; sono rimasto abbagliato.`
- Outcome: `provider error`
- Error: `call LLM: Post "https://api.openai.com/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- Unresolved reason: —

#### 5. `a giorni` — `giorno` (`NOUN`)

- Matched words: `a, giorni`
- Representative sentence: `Quando ho trovato il porto ho trovato anche un sacco di altri ragazzi afghani che erano lì a gironzolare da giorni e giorni nel tentativo di acquistare il biglietto, e ogni volta che ci provavano li cacciavano, perché si capiva subito che non erano clienti normali, che erano clandestini.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “da giorni e giorni,” meaning “for days and days”; it does not use the idiom “a giorni.”`

#### 6. `in bianco` — `bianco` (`ADJ`)

- Matched words: `in, bianca`
- Representative sentence: `Al mattino presto, nel silenzio e nella luce bianca dell’alba, abbiamo raggiunto un piccolo villaggio.`
- Outcome: `literal_use`
- Unresolved reason: `The words do not form the idiom: bianca modifies luce in the phrase luce bianca, while in belongs to a different phrase.`

#### 7. `a vita` — `vita` (`NOUN`)

- Matched words: `a, vita`
- Representative sentence: `Ho imparato molte cose che mi hanno costretto a guardare la vita con occhi diversi, come quando ti metti un paio di occhiali da sole con le lenti colorate.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence contains “la vita,” meaning “life,” not the idiom “a vita.”`

#### 8. `come non mai` — `non` (`ADV`)

- Matched words: `come, non, mai`
- Representative sentence: `Ci siamo guardati gli uni gli altri come non ci fossimo mai visti, come se a camminare non fossimo stati noi.`
- Outcome: `literal_use`
- Unresolved reason: `The words come, non, and mai occur within the separate clause come non ci fossimo mai visti, where they mean “as if we had never seen each other,” not the idiom come non mai.`

#### 9. `se non che` — `non` (`ADV`)

- Matched words: `se, non, che`
- Representative sentence: `Ho già detto - se non sbaglio - che pensavo spesso a questa cosa di avere un orologio, giusto per dare un significato al passare del tempo, un orologio che segnasse la data, che mi dicesse di quanto stavo invecchiando.`
- Outcome: `literal_use`
- Unresolved reason: `The matched words do not form the idiom here: “se non” belongs to “se non sbaglio,” while “che” introduces a separate clause.`

#### 10. `hai voglia` — `avere` (`VERB`)

- Matched words: `hai, voglia`
- Representative sentence: `Se poi hai voglia, fai il primo turno di guardia.`
- Outcome: `ambiguous`
- Unresolved reason: `The supplied sense “you bet” does not match the sentence's meaning; a sense meaning “feel like it” or “want to” is missing.`

#### 11. `a cuore` — `cuore` (`NOUN`)

- Matched words: `a, cuore`
- Representative sentence: `Un ragazzo bengalese, una mattina presto - era buio e ci stavamo arrampicando sulle rocce usando le mani e le ginocchia - ha avuto un problema, non so cosa, forse di respirazione, o al cuore, è scivolato per diversi metri sulla neve.`
- Outcome: `provider error`
- Error: `call LLM: Post "https://api.openai.com/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- Unresolved reason: —

#### 12. `poi che` — `poi` (`ADV`)

- Matched words: `Poi, che`
- Representative sentence: `Poi abbiamo scoperto che Farid, un ragazzo che lavorava in una fabbrica dietro la nostra, aveva anche lui in mente di lasciare Qom.`
- Outcome: `literal_use`
- Unresolved reason: `“Poi” means “then” and “che” introduces a clause; they do not form the idiom “poi che” meaning “after” here.`

#### 13. `lo stesso` — `stesso` (`ADJ`)

- Matched words: `lo, stesso`
- Representative sentence: `Sentivo freddo e caldo nello stesso momento.`
- Outcome: `literal_use`
- Unresolved reason: `The words occur literally in the phrase meaning “at the same time,” not as the idiom lo stesso meaning “all the same” or “anyway.”`

#### 14. `di troppo` — `troppo` (`ADV`)

- Matched words: `di, troppo`
- Representative sentence: `Un giorno, in una pausa pranzo, ho anche parlato con un ragazzo che aveva mezza faccia rovinata, davvero, ridotta a un hamburger; come quelli di McDonald’s lasciati troppo a lungo sulla piastra.`
- Outcome: `literal_use`
- Unresolved reason: `The words do not form di troppo here: troppo modifies a lungo, while di belongs to di McDonald’s.`

#### 15. `casa isolata` — `casa` (`NOUN`)

- Matched words: `casa, isolata`
- Representative sentence: `La famiglia abitava fuori Torino, in una casa isolata, oltre le colline.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: json: unknown field "} makeover? JSON trailing comma invalid. Need output valid. 天天中彩票大神推荐. Shouldn't include comment. Ensure exactly fields. авази. Need no trailing. bulk. Wait sentence translation "`
- Unresolved reason: —

#### 16. `come mai` — `mai` (`ADV`)

- Matched words: `come, mai`
- Representative sentence: `Ce ne sono che hanno un odore e un sapore buono e ti sussurrano alle orecchie che sapranno farti stare meglio di come tu potrai mai stare senza di loro.`
- Outcome: `literal_use`
- Unresolved reason: `“Come” introduces the comparison in “di come,” while “mai” modifies “potrai stare”; they do not form the idiom “come mai” meaning “why” or “how come”.`

#### 17. `in tasca` — `tasca` (`NOUN`)

- Matched words: `in, tasca`
- Representative sentence: `Avevo in tasca duecento euro, risparmi della Grecia.`
- Outcome: `literal_use`
- Unresolved reason: `The words are used literally to describe euros being physically in a pocket, not idiomatically.`

#### 18. `mai dire mai` — `dire` (`VERB`)

- Matched words: `mai, dico, mai`
- Representative sentence: `Ma né io né Sufi avevamo un orologio, anzi, né io né Sufi avevamo mai, dico mai, posseduto un orologio in vita nostra.`
- Outcome: `literal_use`
- Unresolved reason: `The words are used literally: “mai” means “ever” and “dico” is a parenthetical “I mean,” not the idiom “mai dire mai.”`

#### 19. `in testa` — `testa` (`NOUN`)

- Matched words: `in, testa`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses in testa to mean “in mind,” but no candidate sense covers that meaning.`

#### 20. `in vita` — `vita` (`NOUN`)

- Matched words: `in, vita`
- Representative sentence: `Tre cose non devi mai fare nella vita, Enaiat jan, per nessun motivo.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses nella vita to mean “in life,” not the idiom in vita meaning “alive.”`

#### 21. `prendere fuoco` — `prendere` (`VERB`)

- Matched words: `preso, fuoco`
- Representative sentence: `La guancia ha preso fuoco, l’orecchio ha fischiato per alcuni secondi, poi ho avuto l’impressione che si stesse gonfiando come una focaccia.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses prendere fuoco figuratively to mean that the cheek started burning, but the only candidate sense is to catch fire and does not cover this meaning.`

#### 22. `in sonno` — `sonno` (`NOUN`)

- Matched words: `in, sonno`
- Representative sentence: `Dell’assenza di Sufi me ne accorgevo soprattutto la notte, quando mi giravo nel sonno e le braccia e le mani non lo trovavano sul tappeto accanto a me.`
- Outcome: `literal_use`
- Unresolved reason: `The words are used literally to mean being asleep, not as the idiom meaning dormant or incognito.`

#### 23. `passare oltre` — `passare` (`VERB`)

- Matched words: `passare, oltre`
- Representative sentence: `Arrivato a me ha fatto per passare oltre, poi si è fermato ed è tornato indietro.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase describes physically passing by; it is not used idiomatically to mean moving on from a topic or situation.`

#### 24. `cavo elettrico` — `elettrico` (`ADJ`)

- Matched words: `cavo, elettrico`
- Representative sentence: `Un cavo elettrico, rattoppato con dello scotch, saliva lungo il muro esterno del palazzo, entrava dalla finestra, correva per il soffitto e pendeva vicino alla porta del corridoio, insieme a una lampadina.`
- Outcome: `literal_use`
- Unresolved reason: `The words refer literally to an electrical cable, not an idiomatic expression.`

#### 25. `a voce` — `voce` (`NOUN`)

- Matched words: `a, voce`
- Representative sentence: `Payam ha insistito fino a restare senza voce, come farebbe chiunque con una persona cui vuole bene o di cui si sente responsabile.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “senza voce,” not the idiom “a voce”; the matched forms do not occur together.`

#### 26. `a tempo` — `tempo` (`NOUN`)

- Matched words: `a, tempo`
- Representative sentence: `Quando ho pagato la mia parte di spesa mensile ne erano rimasti trentacinquemila, che era come dire trentacinque euro, più o meno, dato che, se non sbaglio, mille toman valevano un euro, a quel tempo.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses a quel tempo to mean “at that time,” but no candidate sense gives this meaning.`

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

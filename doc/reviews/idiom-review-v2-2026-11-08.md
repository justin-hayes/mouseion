# Idiom card review — `idiom-card-v2`

Status: **Gate failed — Italian idiom cards stay off** (follow-up: #2385)

- Date: 2026-11-08
- Model: `gpt-5.6-luna`
- Reasoning setting: `low`
- Prompt contract: `idiom-card-v2`
- Provider: `openai-compatible` (`gpt-5.6-luna/idiom-card-v2-reasoning-low`)
- Case set: `idiom-review-v2` (11 cases)
- Matcher version: `idiom-matcher/2`
- Matcher gap bound: 2
- Sample: 30 confirmed idiom cards requested from Book `34c9236b-0e14-4e9d-aac7-02d10611c271` in `it` (seed `2026-11-08-v2`); 351 idiom items found, 351 sent, 1 confirmed
- Rubric: [doc/reviews/README.md](README.md)

## Summary

- Cases run: 11
- Outcomes: confirmed=7, literal_use=0, ambiguous=0, unresolved=0, invalid response=2, provider error=0, lemma-card controls=2
- Automatic checks: 7 passed, 4 failed

## Gate

The gate passes only when every item below is true:

- every positive case is confirmed with a citation-form Gloss;
- no literal-use control is confirmed, every ambiguous control returns `ambiguous`, and every lemma-card control passes its automatic check;
- none of the sampled cards teaches a wrong meaning.

- [x] Gate evaluated and recorded: **fail** — four automatic checks fail and the sample has 1 confirmed card of the 30 the gate needs.

Verdicts drafted by Claude on 2026-11-08 at the operator's request and confirmed by the operator the same day. Run against the dictionary index rebuilt after #2380 (same 2026-10-03 dump; Italian idiom entries 9,661 → 8,666). The 2026-11-08 v1 report ([idiom-review-2026-11-08.md](idiom-review-2026-11-08.md)) is unchanged. Failing cases:

- `it-tagliare-la-corda-literal` and `de-jemanden-auf-den-arm-nehmen-literal` (both passed under v1): the provider answered `literal_use` with an empty `translation`, and the codec rejects every response with an empty `translation`, so the controls end as `invalid response`. The v2 prompt defines `translation` as "the idiom in that sense", which has no value when the words are literal.
- `it-rispetto-a-comparative`: confirmed as "in relation to" instead of `ambiguous`.
- `it-fare-bene-literal`: confirmed as "to do well" instead of `literal_use`, even though candidate sense e3 is the literal-use marker.
- Sample: 351 items sent, 1 confirmed, 93 `ambiguous`, 17 `literal_use`, 237 `invalid response` (164 `translation is empty`, 66 `context_only contradicts evidence IDs`, 5 empty or unknown evidence ID, 2 malformed JSON), 3 provider timeouts. Most `ambiguous` reasons say the candidate sense has no `evidence_id` to cite.

Observations beyond the gate:

- Idiom senses carry no evidence IDs outside the case set. `Index.IdiomEntries` returns the index's senses as stored, without the evidence IDs that lemma lookups derive, and the deck freeze passes them on unchanged. Under v1 the provider could confirm with `context_only=true`, so the v1 sample cards were glossed from context. v2 requires a confirmed gloss to cite a candidate sense, so almost nothing in a real Book can confirm.
- The case set does not catch this because its candidate senses carry frozen evidence IDs.

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
- Raw Gloss: `capable, on the ball`
- Evidence IDs: `wikt:in gamba:1`
- Context only: false
- Unresolved kind: —
- Unresolved reason: —

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

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

Leave these blank for the operator; the command cannot judge them.

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

Leave these blank for the operator; the command cannot judge them.

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
- Translation: `to turn a blind eye`
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

Leave these blank for the operator; the command cannot judge them.

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
| outcome is `literal_use` | **FAIL** — expected "literal_use", observed "invalid response" |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] No definition paraphrase — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] No sentence inflection or arguments — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] Meaning correct in context — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check

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
| outcome is `literal_use` | **FAIL** — expected "literal_use", observed "invalid response" |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] No definition paraphrase — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] No sentence inflection or arguments — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check
- [x] Meaning correct in context — verdict: n/a — no Gloss; the codec rejected the answer as `translation is empty`, so the control fails its automatic check

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

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: pass — the sentence uses the adverb; "in front of" is the dictionary's ADV sense 2 and reads correctly

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
| outcome is `ambiguous` | **FAIL** — expected "ambiguous", observed "confirmed" |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: **fail** — the sentence is comparative ("compared with"); the control expects `ambiguous`, and the provider glossed "in relation to"

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
| outcome is `literal_use` | **FAIL** — expected "literal_use", observed "confirmed" |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: pass
- [x] No definition paraphrase — verdict: pass
- [x] No sentence inflection or arguments — verdict: pass
- [x] Meaning correct in context — verdict: **fail** — *fatte bene* modifies *le cose* and is literal; the provider still confirmed "to do well"

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

- [x] Anchor card moved to an idiom-free sentence — verdict: pass

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

- [x] Anchor card moved to an idiom-free sentence — verdict: pass

## Sample

The sample re-freezes the Book's current deck preparation in memory with idiom cards on for this language and no per-deck bound. Items are sent in seeded order, using the request a prepared deck would send, until enough are confirmed. Nothing is written and no assessment is recorded.

Only 1 idiom items were confirmed; the gate requires 30.

### Confirmed

#### 1. `se non` — `non` (`ADV`)

- Matched words: `se, non`
- Representative sentence: `E se non ci mangiano loro, ci penseranno i coccodrilli, anche se voi dite che non ci sono.`
- Gloss: `if not`
- Translation: `if not`
- Sentence translation: `And if they don't eat us, the crocodiles will take care of it, even if you say they aren't there.`

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *se non* is "if not" here ("And if they don't eat us")

### Not confirmed

The provider did not confirm 350 sampled items. They would be idiom omissions in a prepared deck.

#### 1. `in lontananza` — `lontananza` (`NOUN`)

- Matched words: `in, lontananza`
- Representative sentence: `Le ho viste in lontananza e subito non ho capito perché si fossero fermate.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 2. `di fatto` — `fare` (`VERB`)

- Matched words: `di, fatto`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 3. `mai più` — `più` (`ADV`)

- Matched words: `mai, più`
- Representative sentence: `Prima uno, poi due, poi tre, sembrava non finissero mai, certi più piccoli di me.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses mai literally in non-idiomatic mai finissero mai ('would never end'), not the idiom mai più ('never again').`

#### 4. `far vedere` — `fare` (`VERB`)

- Matched words: `far, vedere`
- Representative sentence: `Non è il caso di farci vedere in giro come quattro cani randagi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 5. `sentire parlare` — `sentire` (`VERB`)

- Matched words: `sentir, parlare`
- Representative sentence: `Di lasciarmi solo, di notte, di sapermi addormentato su una panchina, non voleva proprio sentirne parlare.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 6. `in riga` — `riga` (`NOUN`)

- Matched words: `in, righe`
- Representative sentence: `Se non vi interessa, andate pure avanti a leggere: nelle prossime righe non muore nessuno e non vengono fornite informazioni fondamentali per la storia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 7. `di mezzo` — `mezzo` (`NOUN`)

- Matched words: `di, mezzo`
- Representative sentence: `Prima cosa: i fari del mezzo con il motore rumoroso erano accesi e puntavano contro la porta; sono rimasto abbagliato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 8. `di fronte` — `fronte` (`NOUN`)

- Matched words: `di, fronte`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is present, but no evidence_id is supplied for it.`

#### 9. `avere la peggio` — `avere` (`VERB`)

- Matched words: `avuto, la, peggio`
- Representative sentence: `Il curdo ha avuto la peggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 10. `a giorni` — `giorno` (`NOUN`)

- Matched words: `a, giorni`
- Representative sentence: `Quando ho trovato il porto ho trovato anche un sacco di altri ragazzi afghani che erano lì a gironzolare da giorni e giorni nel tentativo di acquistare il biglietto, e ogni volta che ci provavano li cacciavano, perché si capiva subito che non erano clienti normali, che erano clandestini.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 11. `chiave inglese` — `inglese` (`ADJ`)

- Matched words: `chiave, inglese`
- Representative sentence: `Con una chiave inglese in mano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 12. `fino a` — `fino` (`ADV`)

- Matched words: `fino, a`
- Representative sentence: `Ho pensato che potevo saltargli addosso, appendermi alla sua giacca e fri-gnare fino a quando sarebbero scoppiati i polmoni (a me) o le orecchie (a lui), ma credo di aver fatto bene a non farlo.`
- Outcome: `ambiguous`
- Unresolved reason: `The temporal sense is clear, but the supplied candidate sense has no evidence_id to cite.`

#### 13. `tanto per cambiare` — `tanto` (`ADV`)

- Matched words: `tanto, per, cambiare`
- Representative sentence: `Il primo giorno, tanto per cambiare, è stato pessimo, quasi peggio del primo giorno al samavat Qgazi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 14. `in bianco` — `bianco` (`ADJ`)

- Matched words: `in, bianca`
- Representative sentence: `Al mattino presto, nel silenzio e nella luce bianca dell’alba, abbiamo raggiunto un piccolo villaggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 15. `in giro` — `giro` (`NOUN`)

- Matched words: `in, giro`
- Representative sentence: `Non potevamo certo andare in giro per la città con quelli indossati durante il cammino, stracciati e puzzolenti.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no evidence_id was supplied for any candidate sense.`

#### 16. `a vita` — `vita` (`NOUN`)

- Matched words: `a, vita`
- Representative sentence: `Ho imparato molte cose che mi hanno costretto a guardare la vita con occhi diversi, come quando ti metti un paio di occhiali da sole con le lenti colorate.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 17. `dopo che` — `dopo` (`ADV`)

- Matched words: `dopo, che`
- Representative sentence: `Tre giorni dopo, più o meno, dopo che Sufi e io eravamo arrivati, ho visto un gruppo di operai discutere su chi dovesse andare a fare qualcosa da qualche parte.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 18. `come non mai` — `non` (`ADV`)

- Matched words: `come, non, mai`
- Representative sentence: `Ci siamo guardati gli uni gli altri come non ci fossimo mai visti, come se a camminare non fossimo stati noi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 19. `se non che` — `non` (`ADV`)

- Matched words: `se, non, che`
- Representative sentence: `Ho già detto - se non sbaglio - che pensavo spesso a questa cosa di avere un orologio, giusto per dare un significato al passare del tempo, un orologio che segnasse la data, che mi dicesse di quanto stavo invecchiando.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 20. `oltre a` — `oltre` (`ADV`)

- Matched words: `Oltre, a`
- Representative sentence: `Oltre a noi afghani, che eravamo i più giovani, c’erano curdi, pakistani, iracheni e qualche bengalese.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 21. `hai voglia` — `avere` (`VERB`)

- Matched words: `hai, voglia`
- Representative sentence: `Se poi hai voglia, fai il primo turno di guardia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 22. `a lungo` — `lungo` (`ADJ`)

- Matched words: `a, lungo`
- Representative sentence: `Ho camminato a lungo, seguendo una strada poco trafficata.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to support a confirmed gloss.`

#### 23. `assieme a` — `assieme` (`ADV`)

- Matched words: `assieme, a`
- Representative sentence: `C’erano altri afghani assieme a lui.`
- Outcome: `provider error`
- Error: `call LLM: Post "https://api.openai.com/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- Unresolved reason: —

#### 24. `una volta` — `volta` (`NOUN`)

- Matched words: `una, volta`
- Representative sentence: `Sono dieci mesi di lavoro, ho detto a Wahid, che una volta aveva pensato di andarsene e poi non lo aveva fatto.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses the formerly/previously sense, but the matching candidate sense has no evidence_id to cite.`

#### 25. `ad alta voce` — `voce` (`NOUN`)

- Matched words: `ad, alta, voce`
- Representative sentence: `Il talebano, con il fucile, è entrato in classe e ha detto ad alta voce che bisognava chiudere la scuola, punto.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 26. `a cuore` — `cuore` (`NOUN`)

- Matched words: `a, cuore`
- Representative sentence: `Un ragazzo bengalese, una mattina presto - era buio e ci stavamo arrampicando sulle rocce usando le mani e le ginocchia - ha avuto un problema, non so cosa, forse di respirazione, o al cuore, è scivolato per diversi metri sulla neve.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 27. `da capo` — `capo` (`NOUN`)

- Matched words: `da, capo`
- Representative sentence: `Andare avanti o tornare indietro è la stessa cosa, e io preferisco morire in mare che dover rifare da capo tutta la strada che ho fatto fino ad adesso.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 28. `in attesa` — `attesa` (`NOUN`)

- Matched words: `in, attesa`
- Representative sentence: `Sono rimasto rannicchiato, tranquillo, in attesa che il camion ripartisse.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 29. `poi che` — `poi` (`ADV`)

- Matched words: `Poi, che`
- Representative sentence: `Poi abbiamo scoperto che Farid, un ragazzo che lavorava in una fabbrica dietro la nostra, aveva anche lui in mente di lasciare Qom.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 30. `se stesso` — `stesso` (`ADJ`)

- Matched words: `se, stessi`
- Representative sentence: `Quando si avvicinavano dei camion che andavano nella nostra direzione, chiedevamo un passaggio (anche piccolo) agli autisti, che se erano belle persone si fermavano e ci caricavano, mentre se erano antipatici o arrabbiati con se stessi o con il mondo ci passavano accanto accelerando, coprendoci di polvere.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 31. `lo stesso` — `stesso` (`ADJ`)

- Matched words: `lo, stesso`
- Representative sentence: `Sentivo freddo e caldo nello stesso momento.`
- Outcome: `literal_use`
- Unresolved reason: `The words occur literally in “nello stesso momento,” meaning “at the same time,” not as the idiom “lo stesso.”`

#### 32. `di troppo` — `troppo` (`ADV`)

- Matched words: `di, troppo`
- Representative sentence: `Un giorno, in una pausa pranzo, ho anche parlato con un ragazzo che aveva mezza faccia rovinata, davvero, ridotta a un hamburger; come quelli di McDonald’s lasciati troppo a lungo sulla piastra.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 33. `casa isolata` — `casa` (`NOUN`)

- Matched words: `casa, isolata`
- Representative sentence: `La famiglia abitava fuori Torino, in una casa isolata, oltre le colline.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 34. `come mai` — `mai` (`ADV`)

- Matched words: `come, mai`
- Representative sentence: `Ce ne sono che hanno un odore e un sapore buono e ti sussurrano alle orecchie che sapranno farti stare meglio di come tu potrai mai stare senza di loro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 35. `per terra` — `terra` (`NOUN`)

- Matched words: `per, terra`
- Representative sentence: `Avrei fatto qualunque cosa, da lavare per terra a pulire le scarpe, qualunque cosa ci fosse da fare, e questo perché, a dirla tutta, avevo una gran paura a uscire per strada.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 36. `in tasca` — `tasca` (`NOUN`)

- Matched words: `in, tasca`
- Representative sentence: `Avevo in tasca duecento euro, risparmi della Grecia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: json: unknown field "}_fw(function(){return true;}); 天天中彩票中奖เงินบาทไทย.unregister? Need valid JSON no extra. I accidentally include malformed trailing in final draft? Need correct. --- Actually final response should JSON. Ensure only fields. Let's output.】【。】【”】【)期特码? Wait generated text can replace.agaduhan. We'll final clean. kontaktannonser.} 亿贝? Need no issue. Let's send.} ԥхын? This is analysis weird. final now.jury.лиқиниң.}ಿಕಿತ. ensure JSON. ]))? Use standard.} тру. the final channel.} 二四六. Let's do.} unerquicklich.} 手机上天天中彩票. Sorry. final.} [No malformed.]"`
- Unresolved reason: —

#### 37. `mai dire mai` — `dire` (`VERB`)

- Matched words: `mai, dico, mai`
- Representative sentence: `Ma né io né Sufi avevamo un orologio, anzi, né io né Sufi avevamo mai, dico mai, posseduto un orologio in vita nostra.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 38. `in effetti` — `effetto` (`NOUN`)

- Matched words: `In, effetti`
- Representative sentence: `In effetti avevo il rotolo dei soldi guadagnati al cantiere.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is present, but no evidence_id was supplied to support it.`

#### 39. `in testa` — `testa` (`NOUN`)

- Matched words: `in, testa`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 40. `posto di blocco` — `posto` (`NOUN`)

- Matched words: `posti, di, blocco`
- Representative sentence: `Un giorno, finito il turno di lavoro, un giorno come tanti, abbiamo raccolto le nostre cose dentro uno zainetto di tela, abbiamo salutato il capo della fabbrica, chiesto lo stipendio dovuto e con un pullman di linea (e la solita paura dei posti di blocco) siamo arrivati a Teheran.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 41. `di notte` — `notte` (`NOUN`)

- Matched words: `di, notte`
- Representative sentence: `Per questo, credo, siamo partiti di notte, noi tre: io, mia madre e l’uomo - l’uomo che chiamerò semplicemente uomo - cui mamma aveva chiesto di accompagnarci.`
- Outcome: `ambiguous`
- Unresolved reason: `The temporal sense matches, but no candidate sense provides an evidence_id required to confirm the gloss.`

#### 42. `a un passo da` — `passo` (`NOUN`)

- Matched words: `a, un, passo, da`
- Representative sentence: `La cima della montagna era lì a un passo da noi, vicina da raggiungerla con un salto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 43. `in vita` — `vita` (`NOUN`)

- Matched words: `in, vita`
- Representative sentence: `Tre cose non devi mai fare nella vita, Enaiat jan, per nessun motivo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 44. `prendere fuoco` — `prendere` (`VERB`)

- Matched words: `preso, fuoco`
- Representative sentence: `La guancia ha preso fuoco, l’orecchio ha fischiato per alcuni secondi, poi ho avuto l’impressione che si stesse gonfiando come una focaccia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 45. `stazione di polizia` — `polizia` (`NOUN`)

- Matched words: `stazione, di, polizia`
- Representative sentence: `E dovevi darglieli subito, i soldi, se no ti portavano alla stazione di polizia e ti gonfiavano di botte: pugni e calci.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 46. `in sonno` — `sonno` (`NOUN`)

- Matched words: `in, sonno`
- Representative sentence: `Dell’assenza di Sufi me ne accorgevo soprattutto la notte, quando mi giravo nel sonno e le braccia e le mani non lo trovavano sul tappeto accanto a me.`
- Outcome: `literal_use`
- Unresolved reason: `Here in sonno means while asleep, not dormant or an incognito agent.`

#### 47. `passare oltre` — `passare` (`VERB`)

- Matched words: `passare, oltre`
- Representative sentence: `Arrivato a me ha fatto per passare oltre, poi si è fermato ed è tornato indietro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 48. `fare finta` — `fare` (`VERB`)

- Matched words: `fatto, finta`
- Representative sentence: `Sono entrato, ho stretto il telefono in mano e ho fatto finta di essere lì a telefonare, invece spiavo i miei compagni, per vedere cosa combinavano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 49. `cavo elettrico` — `elettrico` (`ADJ`)

- Matched words: `cavo, elettrico`
- Representative sentence: `Un cavo elettrico, rattoppato con dello scotch, saliva lungo il muro esterno del palazzo, entrava dalla finestra, correva per il soffitto e pendeva vicino alla porta del corridoio, insieme a una lampadina.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 50. `in disparte` — `disparta` (`NOUN`)

- Matched words: `in, disparte`
- Representative sentence: `Gioma, detto Sufi, perché era uno che se ne stava in disparte, tranquillo e silenzioso come un monaco, anche se in certe occasioni, be’, era quello che creava più problemi di tutti.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 51. `via da` — `via` (`ADV`)

- Matched words: `via, da`
- Representative sentence: `Io, via da Nava, non ci sarei mai voluto andare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 52. `ogni tanto` — `tanto` (`NOUN`)

- Matched words: `Ogni, tanto`
- Representative sentence: `Ogni tanto si andava al porto, in tre o quattro, e si tentava di saltare su un camion.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id, so it cannot support a confirmed gloss.`

#### 53. `a voce` — `voce` (`NOUN`)

- Matched words: `a, voce`
- Representative sentence: `Payam ha insistito fino a restare senza voce, come farebbe chiunque con una persona cui vuole bene o di cui si sente responsabile.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 54. `fare piacere` — `fare` (`VERB`)

- Matched words: `fatto, piacere`
- Representative sentence: `Mi hanno chiesto se andava tutto bene, se avevo bisogno di qualcosa, un gesto che mi ha fatto molto piacere.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 55. `a tempo` — `tempo` (`NOUN`)

- Matched words: `a, tempo`
- Representative sentence: `Quando ho pagato la mia parte di spesa mensile ne erano rimasti trentacinquemila, che era come dire trentacinque euro, più o meno, dato che, se non sbaglio, mille toman valevano un euro, a quel tempo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 56. `in mente` — `mente` (`NOUN`)

- Matched words: `in, mente`
- Representative sentence: `A Roma sapevo come fare: avevo le istru-zioni in mente.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 57. `forte e chiaro` — `forte` (`ADJ`)

- Matched words: `forte, e, chiaro`
- Representative sentence: `Lo diremo forte e chiaro, Enaiat.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 58. `a mano` — `mano` (`NOUN`)

- Matched words: `a, mano`
- Representative sentence: `Una sera sono rientrato dal lavoro davvero triste e mi sono seduto davanti al piccolo televisore in bianco e nero, uno di quelli con le antenne da girare a mano, che passi più tempo a cercare di sintonizzarti che a vedere le trasmissioni.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 59. `non potere che` — `non` (`ADV`)

- Matched words: `non, potevo, che`
- Representative sentence: `Dovevo decidere in fretta cosa fare, perché se c’era da pagare un biglietto o qualcosa del genere non potevo sperare che quei soldi mi crescessero in tasca come una pianta, giusto?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 60. `di ritorno` — `ritorno` (`NOUN`)

- Matched words: `di, ritorno`
- Representative sentence: `Mamma parlava con la gente per tentare di organizzare il suo viaggio di ritorno, anche se io non lo sapevo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 61. `a capo` — `capo` (`NOUN`)

- Matched words: `a, capo`
- Representative sentence: `Quelli avevano tutta l’aria di essere dei prepotenti, gente di cui non fidarsi, e se mi fossi scagliato addosso al capo sono pronto a scommettere che mi sarei ritrovato con le ossa rotte e tutta la mia merce dentro le loro scatole.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 62. `stare per` — `stare` (`VERB`)

- Matched words: `stavo, per`
- Representative sentence: `Io sono scattato in piedi, velocissimo, e stavo per fuggire, ma lei mi ha fatto segno di entrare in casa.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 63. `fare schifo` — `fare` (`VERB`)

- Matched words: `fare, schifo`
- Representative sentence: `Arrivati a Venezia, a piazzale Roma, mi ha comprato un panino perché dovevo avere la faccia di uno che aveva fame, mi ha portato in una chiesa dove ha recuperato dei vestiti nuovi da darmi e dove ho potuto lavarmi, per non fare schifo alla gente.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 64. `padre nostro` — `padre` (`NOUN`)

- Matched words: `padri, nostri`
- Representative sentence: `C’è chi dice che i padri dei nostri padri erano i koshani, gli antichi abitanti di quelle terre, i leggendari costruttori dei Buddha di Bamiyan.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 65. `a piedi` — `piede` (`NOUN`)

- Matched words: `a, piedi`
- Representative sentence: `Ha sorriso, ha pagato il cappuccino salutando il barista, che a quanto pare conosceva, e ci siamo diretti a piedi all’Ufficio minori stranieri.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 66. `andare bene` — `andare` (`VERB`)

- Matched words: `andava, bene`
- Representative sentence: `Mi hanno chiesto se andava tutto bene, se avevo bisogno di qualcosa, un gesto che mi ha fatto molto piacere.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 67. `tra i piedi` — `piede` (`NOUN`)

- Matched words: `tra, i, piedi`
- Representative sentence: `La palla è rimbalzata contro la pietra e si è fermata tra i miei piedi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 68. `di già` — `già` (`ADV`)

- Matched words: `di, già`
- Representative sentence: `Era stato lui, kaka Hamid, a insegnarmi a leggere e scrivere meglio di quanto già non sapessi fare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 69. `mestolo di legno` — `legno` (`NOUN`)

- Matched words: `mestolo, di, legno`
- Representative sentence: `Correvo e pensavo alle donne di Nava che mescolavano con un mestolo di legno il qhorma palaw.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 70. `prendere in giro` — `prendere` (`VERB`)

- Matched words: `preso, in, giro`
- Representative sentence: `Ossia, credo di averla fatta, perché io a volte non me ne accorgo quando capita qualcosa di buffo o di strano, anche perché se me ne accorgessi eviterei di farlo succedere, e di sentirmi preso in giro eccetera.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate senses have no evidence_id handles, so no supported gloss can be cited.`

#### 71. `giorno e notte` — `giorno` (`NOUN`)

- Matched words: `giorni, e, notti`
- Representative sentence: `L’uno dopo l’altro, come dalle pietre di ghiaccio, sono gocciolati via dieci giorni e dieci notti.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase refers literally to a duration of ten days and ten nights, not to the idiomatic sense of unceasingly.`

#### 72. `su e giù` — `giù` (`ADV`)

- Matched words: `su, e, giù`
- Representative sentence: `Ma adesso eravamo troppo in alto per fare su e giù.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 73. `campo di concentramento` — `campo` (`NOUN`)

- Matched words: `campi, di, concentramento`
- Representative sentence: `Due campi di concentramento, per quello che ho potuto leggere, poi, sui campi di concentramento: non so se mi spiego.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot be cited as required.`

#### 74. `a mezzo` — `mezzo` (`ADJ`)

- Matched words: `a, mezza`
- Representative sentence: `Dopo un’ora, lungo una sterrata mal ridotta, a mezza costa, ci ha fermato un pastore accompagnato da un cane che ruotava su se stesso come un pazzo per mordersi la coda; il cane, non il pastore.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 75. `in tempo` — `tempo` (`NOUN`)

- Matched words: `in, tempo`
- Representative sentence: `Ora dichiariamolo a che punto ero nel tempo e nella storia.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses nel tempo literally as a temporal phrase, not the idiom in tempo.`

#### 76. `in compenso` — `compenso` (`NOUN`)

- Matched words: `In, compenso`
- Representative sentence: `In compenso ho scoperto che c’era un posto, vicino a un bazar, una zona diroccata in direzione del Bosforo, dove si poteva andare la mattina presto con la speranza di trovare del lavoro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 77. `senza fiato` — `fiato` (`NOUN`)

- Matched words: `senza, fiato`
- Representative sentence: `Nel groviglio sono riuscito a prendere dalla sua roba un paio di calze, poi mi ha rifilato un calcio nello stomaco e sono rimasto senza fiato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 78. `la luna` — `luna` (`NOUN`)

- Matched words: `la, luna`
- Representative sentence: `Poi è arrivato un uomo, un vecchio amico di papà, che non è voluto entrare in casa; ha detto di muoversi, che la luna non era ancora spuntata e il buio era sabbia negli occhi per i talebani o chissà chi altro che rischiavamo di incontrare.`
- Outcome: `provider error`
- Error: `call LLM: Post "https://api.openai.com/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- Unresolved reason: —

#### 79. `a bordo` — `bordo` (`NOUN`)

- Matched words: `a, bordo`
- Representative sentence: `Con l’aiuto di Soltan e Rahmat sono salito a bordo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 80. `in piedi` — `piede` (`NOUN`)

- Matched words: `in, piedi`
- Representative sentence: `Io mi sono seduto su un muretto a contare le macchine che passavano (quelle colorate) mentre mamma è rimasta in piedi, immobile, come se sotto il burqa non ci fosse nessuno.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence clearly uses the standing sense, but the matching candidate sense has no evidence_id to cite.`

#### 81. `avere bisogno di` — `avere` (`VERB`)

- Matched words: `avevo, bisogno, di`
- Representative sentence: `Mi hanno chiesto se andava tutto bene, se avevo bisogno di qualcosa, un gesto che mi ha fatto molto piacere.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 82. `impronta digitale` — `impronta` (`NOUN`)

- Matched words: `impronte, digitali`
- Representative sentence: `Della questione delle impronte digitali me ne avevano parlato alcuni ragazzi che lavoravano alla fabbrica di pietre, in Iran.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot support a confirmed gloss.`

#### 83. `rimanere male` — `rimanere` (`VERB`)

- Matched words: `rimasto, male`
- Representative sentence: `Abbiamo anche litigato, per questo motivo, ma non ricordo bene come è successo, ricordo solo che non ci siamo salutati e io ci sono rimasto male per un sacco di tempo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot be cited as required.`

#### 84. `andare a finire` — `andare` (`VERB`)

- Matched words: `andato, a, finire`
- Representative sentence: `Per quanto riguarda i soldi, tutto è andato a finire bene.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 85. `di peso` — `peso` (`NOUN`)

- Matched words: `di, peso`
- Representative sentence: `Era blu e giallo, il gommone; non tanto grande, a dirla tutta, e il peso massimo per cui era stato costruito era più basso del peso complessivo di noi cinque, ma abbiamo fatto finta di nulla.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 86. `a tutto volume` — `volume` (`NOUN`)

- Matched words: `a, tutto, volume`
- Representative sentence: `Dalla gelateria usciva della musica allegra, a tutto volume.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 87. `occhio di riguardo` — `occhio` (`NOUN`)

- Matched words: `occhio, di, riguardo`
- Representative sentence: `Così, in giro per la città, c’era fame di manovali in nero nei cui confronti, per fare bella figura con il mondo, anche la polizia aveva un occhio di riguardo, credo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 88. `avere fame` — `avere` (`VERB`)

- Matched words: `aveva, fame`
- Representative sentence: `Arrivati a Venezia, a piazzale Roma, mi ha comprato un panino perché dovevo avere la faccia di uno che aveva fame, mi ha portato in una chiesa dove ha recuperato dei vestiti nuovi da darmi e dove ho potuto lavarmi, per non fare schifo alla gente.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 89. `acqua salata` — `acqua` (`NOUN`)

- Matched words: `acqua, salata`
- Representative sentence: `E sulle mani avevamo ferite sottili, taglietti minuscoli: non sapevamo come ce li fossimo procurati, ma bruciavano ogni volta che l’acqua salata li bagnava.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 90. `in particolare` — `particolare` (`ADJ`)

- Matched words: `in, particolare`
- Representative sentence: `Prendeva un taxi e andava a comprare tutto quello che serviva in un negozio particolare, un magazzino dove c’era un po’ di tutto, e il proprietario era un amico, poi rientrava subito.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 91. `in modo che` — `modo` (`NOUN`)

- Matched words: `in, modo, che`
- Representative sentence: `Era gentile e mi dava dei buoni consigli per non farmi ammazzare e per lavorare in modo che kaka Rahim fosse contento.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate senses provide no evidence_id handles, so the sense cannot be cited as confirmed.`

#### 92. `buttare giù` — `giù` (`ADV`)

- Matched words: `buttato, giù`
- Representative sentence: `Allora mi sono detto che dovevo scendere, subito, e mi sono buttato giù.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 93. `scuola superiore` — `scuola` (`NOUN`)

- Matched words: `scuola, superiore`
- Representative sentence: `A settembre mi sono iscritto alla scuola superiore, una scuola per Operatore dei servizi sociali, e ho fatto subito una figuraccia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 94. `di dentro` — `dentro` (`ADV`)

- Matched words: `di, dentro`
- Representative sentence: `Ci ha portati a casa sua in taxi, uno di quei taxi multipli con un sacco di gente dentro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 95. `per caso` — `caso` (`NOUN`)

- Matched words: `per, caso`
- Representative sentence: `Ho domandato di mia mamma, se per caso l’aveva vista uscire, lui, che nessuno usciva o entrava senza che lui se ne accorgesse, giusto?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 96. `da nessuna parte` — `parte` (`NOUN`)

- Matched words: `da, nessuna, parte`
- Representative sentence: `Non vado da nessuna parte, tranquillo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 97. `per sempre` — `sempre` (`ADV`)

- Matched words: `per, sempre`
- Representative sentence: `Chi non ricomincia subito a camminare resta qui per sempre, hanno detto.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id to support a confirmed gloss.`

#### 98. `di fianco` — `fianco` (`NOUN`)

- Matched words: `di, fianco`
- Representative sentence: `Tutti gli altri sono sfilati di fianco, in silenzio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 99. `fare fretta` — `fare` (`VERB`)

- Matched words: `fare, fretta`
- Representative sentence: `I turchi ci hanno detto di raccogliere le nostre cose e di fare in fretta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 100. `punto di riferimento` — `punto` (`NOUN`)

- Matched words: `punto, di, riferimento`
- Representative sentence: `Era il nostro punto di riferimento.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: unknown evidence ID`
- Unresolved reason: —

#### 101. `essere lì per` — `lì` (`ADV`)

- Matched words: `Siamo, lì, per`
- Representative sentence: `Siamo rimasti chiusi lì per quattro giorni.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 102. `in cerchio` — `cerchio` (`NOUN`)

- Matched words: `in, cerchio`
- Representative sentence: `Ci hanno ordinato di metterci in cerchio, nel cortile, i bambini davanti, perché eravamo più bassi, e gli adulti dietro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 103. `buona fortuna` — `buono` (`ADJ`)

- Matched words: `buona, fortuna`
- Representative sentence: `Poi ho pensato che il buona fortuna e il ci vediamo presto non andavano granché d’accordo: perché buona fortuna se ci saremmo visti presto?`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 104. `fare lezione` — `fare` (`VERB`)

- Matched words: `faceva, lezione`
- Representative sentence: `Ha visto che noi eravamo in classe, con il maestro che faceva lezione.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching idiomatic sense has no evidence_id to cite.`

#### 105. `a gomito` — `gomito` (`NOUN`)

- Matched words: `a, gomito`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 106. `a forma di` — `forma` (`NOUN`)

- Matched words: `a, forme, di`
- Representative sentence: `E’ stato Payam ad accompagnarmi all’Ufficio minori stranieri, senza nemmeno darmi il tempo di abituarmi alle forme delle case o al fresco dell’aria (era metà settembre).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 107. `lasciare intendere` — `lasciare` (`VERB`)

- Matched words: `lasciato, intendi`
- Representative sentence: `Da quando avevo lasciato Nava, intendi?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 108. `di riserva` — `riserva` (`NOUN`)

- Matched words: `di, riserva`
- Representative sentence: `Nella cintura, in un taglio fatto da me, avevo due banconote di riserva.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no candidate sense provides an evidence_id to support the gloss.`

#### 109. `per il momento` — `momento` (`NOUN`)

- Matched words: `per, il, momento`
- Representative sentence: `Allora ho chiamato Sufi, a Qom, in quella fabbrica dove lavorava la pietra, e lui mi ha detto che lì, per il momento, le cose erano tranquille.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching idiomatic sense is supplied, but no evidence_id is provided to support a confirmed gloss.`

#### 110. `in nome di Dio` — `nome` (`NOUN`)

- Matched words: `in, nome, di, Dio`
- Representative sentence: `Ci sono anche afghani, tra di loro, ovvio, ma non solo: sono ignoranti, ignoranti di tutto il mondo che impediscono ai bambini di studiare perché temono che possano capire che non fanno ciò che fanno nel nome di Dio, ma per i loro affari.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 111. `fare onore` — `fare` (`VERB`)

- Matched words: `fatto, onore`
- Representative sentence: `Mi sono fatto onore, in quelle partite, per quanto ho potuto.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is identifiable, but no evidence_id is supplied for it.`

#### 112. `a quattro zampe` — `zampa` (`NOUN`)

- Matched words: `a, quattro, zampe`
- Representative sentence: `Sono corso in quella direzione, a quattro zampe, da animaletto; non riuscivo a stare in piedi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching idiomatic sense is clear, but no evidence_id is supplied for it.`

#### 113. `fare attenzione` — `fare` (`VERB`)

- Matched words: `Fai, attenzione`
- Representative sentence: `Fai attenzione alla polizia, ho detto.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate senses provide no evidence_id handles, so the required supporting evidence is unavailable.`

#### 114. `dare per` — `dare` (`VERB`)

- Matched words: `darti, per`
- Representative sentence: `Ma fino a quel momento devi darti da fare per me da quando ti alzi a quando vai a dormire, qualunque cosa ti chieda di fare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 115. `a seconda di` — `seconda` (`NOUN`)

- Matched words: `a, seconda, di`
- Representative sentence: `Ho pensato che o c’era da qualche parte una riserva di buone fortune diverse a seconda delle occasioni, oppure era la stessa buona fortuna che mi aveva augurato il vecchio amico di mio padre dopo averci accompagnato a Kandahar.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense fits, but no evidence_id is supplied for it.`

#### 116. `non ancora` — `non` (`ADV`)

- Matched words: `non, ancora`
- Representative sentence: `Ci sono un sacco di cose che non ho ancora visto nella mia vita e che vorrei vedere, e se a questo aggiungiamo che anche a Qom, anche qui, ogni volta che metto piede fuori dalla fabbrica, anche qui è pericoloso, be’, sapete cosa vi dico?`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot support a confirmed gloss.`

#### 117. `in nero` — `nero` (`ADJ`)

- Matched words: `in, nero`
- Representative sentence: `Kaka Rahim stava leggendo un giornale scritto in inglese, un po’ in rosso un po’ in nero, senza immagini, e fumava una sigaretta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 118. `tutto bene` — `bene` (`ADV`)

- Matched words: `tutto, bene`
- Representative sentence: `E non lo facciamo solo per la fratellanza eccetera, ha detto, ma anche perché quando si parte insieme a degli amici le possibilità che tutto vada bene sono maggiori di quando si parte da soli, senza nessuno cui chiedere aiuto in caso di pericolo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id, so it cannot support a confirmed gloss.`

#### 119. `tirare fuori` — `fuori` (`ADV`)

- Matched words: `tirava, fuori`
- Representative sentence: `Ma dopo un po’, quando si è fatto buio, abbiamo visto uno di loro che s’infilava sotto un albero e da sotto l’albero tirava fuori un cartone, un altro uguale, un altro ancora lo stesso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 120. `e così via` — `così` (`ADV`)

- Matched words: `e, così, via`
- Representative sentence: `Pensavo che se la polizia ci avesse fermato per un controllo e mi avesse visto dormire tranquillo con un quotidiano iraniano in grembo avrebbe pensato che fossi a posto e così via.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 121. `su le mani` — `mano` (`NOUN`)

- Matched words: `su, le, mani`
- Representative sentence: `E sulle mani avevamo ferite sottili, taglietti minuscoli: non sapevamo come ce li fossimo procurati, ma bruciavano ogni volta che l’acqua salata li bagnava.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 122. `avere paura` — `avere` (`VERB`)

- Matched words: `avevamo, paura`
- Representative sentence: `Noi non potevamo, non volevamo, avevamo paura perfino di starnutire, cosa che, essendo mezzi nudi, con addosso soltanto le mutande (avevamo impacchettato i vestiti e le altre cose dentro dei sacchetti di plastica e li avevamo legati con lo scotch per evitare che entrasse l’acqua), poteva anche succedere.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense lacks an evidence_id, so it cannot be cited as confirmed.`

#### 123. `fare il pieno` — `fare` (`VERB`)

- Matched words: `fatto, il, pieno`
- Representative sentence: `Quindi, o ero io a non essere capace a vendere, o a nessuno interessavano le mie cose, o tutti avevano già fatto il pieno di merendine, calze e fazzoletti, o per piazzare la merce c’era un trucco che non conoscevo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 124. `girare la testa` — `testa` (`NOUN`)

- Matched words: `girato, la, testa`
- Representative sentence: `Ho girato la testa e dischiuso lentamente una palpebra, con le mani tra le gambe.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 125. `a braccio` — `braccio` (`NOUN`)

- Matched words: `a, braccia`
- Representative sentence: `Ed era così che mi sentivo, come se un gigante mi avesse preso tra le mani per fare di me una polpetta: avevo male alla testa, alle braccia e in un posto che non saprei dire, tra i polmoni e lo stomaco.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 126. `rendere conto` — `conto` (`NOUN`)

- Matched words: `render, conto`
- Representative sentence: `Ho fatto appena in tempo a rendermene conto, di questa macchina della polizia eccetera, che c’ero già finito dentro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 127. `portare a casa` — `casa` (`NOUN`)

- Matched words: `portati, a, casa`
- Representative sentence: `Ci ha portati a casa sua in taxi, uno di quei taxi multipli con un sacco di gente dentro.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses portare a casa literally to mean taking someone to a home, not figuratively securing or delivering something.`

#### 128. `di ferro` — `ferro` (`NOUN`)

- Matched words: `di, ferro`
- Representative sentence: `La palpebra era pesante come la saracinesca di ferro del negozio di sandali di osta sahib.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase literally describes a shutter made of iron, not something strong or resilient.`

#### 129. `stare in piedi` — `stare` (`VERB`)

- Matched words: `stare, in, piedi`
- Representative sentence: `Sono corso in quella direzione, a quattro zampe, da animaletto; non riuscivo a stare in piedi.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase is used literally to mean being physically upright, not figuratively to mean being well-founded or coherent.`

#### 130. `far capire` — `fare` (`VERB`)

- Matched words: `far, capire`
- Representative sentence: `Non riuscivo a capirli e a farmi capire, così dopo un po’ quelli smettevano di darmi retta e si giravano per tornare alle loro faccende, che a quanto pare erano più urgenti della mia condizione di abbandonato.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses the first candidate sense, but no evidence_id is supplied for that sense.`

#### 131. `essere di famiglia` — `essere` (`VERB`)

- Matched words: `erano, di, famiglie`
- Representative sentence: `C’erano gruppi di turisti, famiglie in vacanza, signori anziani a passeggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 132. `di poco` — `poco` (`ADV`)

- Matched words: `di, po’`
- Representative sentence: `La terza sera, dopo una discussione, abbiamo deciso di partire un po’ prima.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 133. `numero di telefono` — `numero` (`NOUN`)

- Matched words: `numero, di, telefono`
- Representative sentence: `Mi hanno anche dato un numero di telefono, guarda.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 134. `in ginocchio` — `ginocchio` (`NOUN`)

- Matched words: `in, ginocchio`
- Representative sentence: `A forza di stare in ginocchio, le gambe s’erano bloccate.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 135. `di scatto` — `scatto` (`NOUN`)

- Matched words: `di, scatto`
- Representative sentence: `Mi sono voltato di scatto e sono corso giù per la strada pensando che, se correvo abbastanza veloce, forse quella buona fortuna se la sarebbe presa qualcun altro, perché per me era meglio evitarla.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 136. `non appena` — `non` (`ADV`)

- Matched words: `non, appena`
- Representative sentence: `Ma non appena ha stretto la manopola dell’acceleratore e ha premuto la leva della frizione, chissà perché, la moto si è accesa.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 137. `in continuazione` — `continuazione` (`NOUN`)

- Matched words: `in, continuazione`
- Representative sentence: `Avrei bevuto in continuazione per spegnere l’incendio che il male mi aveva appiccato in gola.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot support a confirmed gloss.`

#### 138. `a forza di` — `forza` (`NOUN`)

- Matched words: `A, forza, di`
- Representative sentence: `A forza di stare in ginocchio, le gambe s’erano bloccate.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 139. `luna park` — `luna` (`NOUN`)

- Matched words: `luna, park`
- Representative sentence: `Poi, invece, ci sollevavano e ci passavano sotto, e alla fine, quando eravamo sulla cresta, ci lasciavano cadere di botto, come certe giostre su cui mi è capitato di andare qui in Italia, al luna park.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id to cite.`

#### 140. `buttare fuori` — `fuori` (`ADV`)

- Matched words: `buttare, fuori`
- Representative sentence: `Io faccio il mio lavoro di buttare fuori l’acqua e voi vi picchiate?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 141. `era ora` — `ora` (`NOUN`)

- Matched words: `Era, ora`
- Representative sentence: `Era l’ora dell’intervallo, e forse le grida dei bambini mi avrebbero suggerito la risposta giusta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 142. `come se` — `come` (`ADV`)

- Matched words: `come, se`
- Representative sentence: `Ed era così che mi sentivo, come se un gigante mi avesse preso tra le mani per fare di me una polpetta: avevo male alla testa, alle braccia e in un posto che non saprei dire, tra i polmoni e lo stomaco.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but the candidate senses provide no evidence_id handles to cite.`

#### 143. `fare cenno` — `fare` (`VERB`)

- Matched words: `fatto, cenno`
- Representative sentence: `Ho fatto un cenno con la mano per ringraziarlo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: empty evidence ID`
- Unresolved reason: —

#### 144. `energia elettrica` — `elettrico` (`ADJ`)

- Matched words: `energia, elettrica`
- Representative sentence: `Non era tecnologico, non c’era energia elettrica.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 145. `fare i conti` — `fare` (`VERB`)

- Matched words: `fatto, i, conti`
- Representative sentence: `Lui, in silenzio, ha preso un foglio di carta e ha fatto i conti.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 146. `da parte` — `parte` (`NOUN`)

- Matched words: `da, parti`
- Representative sentence: `Khasta kofta significa stanco come una polpetta, perché quando le donne fanno le polpette, dalle nostre parti, le battono e le battono e le battono, per un tempo lungo, nell’incavo della mano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 147. `primo piano` — `primo` (`ADJ`)

- Matched words: `primo, piano`
- Representative sentence: `Al primo piano c’erano un bagno con la doccia e due saloni spaziosi con cuscini e tappeti e tante finestre, ma tutte oscurate.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 148. `pieno zeppo` — `pieno` (`ADJ`)

- Matched words: `pieno, zeppo`
- Representative sentence: `Uscito dal porto - erano trascorsi quindici minuti, direi, in ogni caso meno di mezz’ora - il camion ha rallentato ed è entrato in un cortile, un cortile pieno zeppo di altri camion, motrici, rimorchi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 149. `grazie a` — `grazie` (`NOUN`)

- Matched words: `grazie, a`
- Representative sentence: `Lui si è messo a urlare, ha detto che se lo faceva lui, che era un adulto e gestiva un samavat importante come il samavat Qgazi, potevo farlo anch’io che ero un bambino piccolo, e che se ero lì era solo grazie a lui.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence clearly uses the causal sense, but no candidate sense provides an evidence_id to support a confirmed gloss.`

#### 150. `volere bene` — `bene` (`ADV`)

- Matched words: `vuole, bene`
- Representative sentence: `Payam ha insistito fino a restare senza voce, come farebbe chiunque con una persona cui vuole bene o di cui si sente responsabile.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 151. `fare amicizia` — `fare` (`VERB`)

- Matched words: `fatto, amicizia`
- Representative sentence: `A sera aspettavamo che quei ragazzi tornassero a casa loro per chiedere ospitalità e da mangiare, dato che dopo la partita avevamo fatto amicizia.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is supplied, but no evidence_id is provided for it.`

#### 152. `prendere la porta` — `prendere` (`VERB`)

- Matched words: `prender, la, Porta`
- Representative sentence: `Durante il viaggio la signora mi ha chiesto se avevo il numero di qualcuno che potesse venire a prendermi alla stazione di Porta Nuova.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 153. `a dire il vero` — `dire` (`VERB`)

- Matched words: `A, dire, il, vero`
- Representative sentence: `A dire il vero non so neppure che differenza ci sia tra andare a destra o a sinistra, uscito dalla porta.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 154. `a differenza di` — `differenza` (`NOUN`)

- Matched words: `A, differenza, di`
- Representative sentence: `A differenza di Esfahàn, a Qom ero io l’unico a uscire dalla fabbrica - per andare a fare la spesa, appunto - cosa che per me era molto, molto rischiosa; ma non potevo evitarlo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but the candidate sense provides no evidence_id handle to cite.`

#### 155. `stare zitto` — `stare` (`VERB`)

- Matched words: `stare, zitti`
- Representative sentence: `Ci hanno fatto scendere, ci hanno detto di stare vicini e zitti, e senza torce o altro abbiamo cominciato a camminare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 156. `scuola elementare` — `scuola` (`NOUN`)

- Matched words: `scuola, elementare`
- Representative sentence: `Il balcone della stanza dove lavoro affaccia su una scuola elementare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 157. `per forza` — `forza` (`NOUN`)

- Matched words: `per, forza`
- Representative sentence: `Ho chiuso gli occhi per trovare la forza, ho chiuso gli occhi e quando li ho riaperti ho visto che la pipì era rossa.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 158. `non affatto` — `non` (`ADV`)

- Matched words: `non, affatto`
- Representative sentence: `Ma lì non era affatto divertente.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id handle, so it cannot be cited as required.`

#### 159. `per favore` — `favore` (`NOUN`)

- Matched words: `per, favore`
- Representative sentence: `Agha, per favore, quanto manca alla cima della montagna?`
- Outcome: `ambiguous`
- Unresolved reason: `The matching idiom is clearly used as “please,” but no evidence_id is provided for any candidate sense.`

#### 160. `da parte di` — `parte` (`NOUN`)

- Matched words: `da, parte, di`
- Representative sentence: `Non hai messo da parte dei soldi?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 161. `visto che` — `vedere` (`VERB`)

- Matched words: `visto, che`
- Representative sentence: `Ho visto che una parte del muro di cinta, quello che divideva il cortile dalla strada, era crollato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 162. `fare affari` — `affare` (`NOUN`)

- Matched words: `fare, affari`
- Representative sentence: `Che non è un bel modo di fare affari, dico io.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 163. `in regalo` — `regalo` (`NOUN`)

- Matched words: `in, regalo`
- Representative sentence: `Cola non si rifiuta mai, così ho aperto la lattina e l’ho bevuta e ho pensato che, davvero, era strano forte quel tizio, che prima s’arrabbiava e poi mi portava una lattina in regalo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 164. `a testa` — `testa` (`NOUN`)

- Matched words: `a, testa`
- Representative sentence: `Ed era così che mi sentivo, come se un gigante mi avesse preso tra le mani per fare di me una polpetta: avevo male alla testa, alle braccia e in un posto che non saprei dire, tra i polmoni e lo stomaco.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 165. `per prima cosa` — `cosa` (`NOUN`)

- Matched words: `Per, prima, cosa`
- Representative sentence: `Per prima cosa sono andati da mio zio, il fratello di mio padre.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 166. `passare sopra` — `passare` (`VERB`)

- Matched words: `passando, sopra`
- Representative sentence: `Così sono scivolato sulla pancia passando sopra i corpi delle persone che, mentre strisciavo, mi davano pugni e pizzicotti, com’è giusto, dato che li stavo schiacciando.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: json: unknown field "} krwar?ેણ_increment ”】【json  和盛毛片高清免费视频 ? Wait invalid extra comma. Need valid exact. final redo. 天天彩 {"`
- Unresolved reason: —

#### 167. `fare le spese` — `fare` (`VERB`)

- Matched words: `fare, le, spese`
- Representative sentence: `Allora siamo andati in giro per i bazar a fare le nostre spese eccetera e c’era un’euforia nell’aria che non saprei dire.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 168. `a memoria` — `memoria` (`NOUN`)

- Matched words: `a, memoria`
- Representative sentence: `Poi mi sono venute alla memoria le parole di un uomo cui avevo tentato di consegnare una lettera da mandare a mia madre, quando vivevo a Quetta, quasi tre anni prima.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 169. `fare la pipì` — `fare` (`VERB`)

- Matched words: `fare, la, pipì`
- Representative sentence: `Quando finalmente sono stato in grado di muovermi e di respirare ho cercato un posto per fare la pipì, tutta la pipì che durante il viaggio non ero riuscito a fare, tutta la pipì che avevo tenuto per tre giorni.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 170. `tutti i giorni` — `giorno` (`NOUN`)

- Matched words: `Tutti, i, giorni`
- Representative sentence: `Tutti i giorni, due volte al giorno, ci davano un uovo, un pomodoro e un pezzo di pane.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 171. `in alto` — `alto` (`ADJ`)

- Matched words: `in, alta`
- Representative sentence: `In questa città turca, la prima città turca in cui ci fermavamo, ci siamo infilati in un campo e abbiamo dormito una notte nell’erba alta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 172. `a causa di` — `causa` (`NOUN`)

- Matched words: `a, causa, di`
- Representative sentence: `Io ho provato ad avvicinarli, ma mi sono accorto che quegli hazara erano diversi dagli altri che avevo conosciuto, e che persino le parole più semplici della mia terra, in bocca a loro, si trasformavano in intricate parole straniere a causa dell’accento.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 173. `permesso di soggiorno` — `permesso` (`NOUN`)

- Matched words: `permesso, di, soggiorno`
- Representative sentence: `Mi aveva chiesto subito - sentivo ancora il calore del suo abbraccio contro il petto - quali erano le mie intenzioni, perché non potevo stare nell’indecisione a lungo; l’indecisione non è sana per chi è senza permesso di soggiorno.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 174. `di nome` — `nome` (`NOUN`)

- Matched words: `di, nomi`
- Representative sentence: `Danila era la madre, e anche di lei, e dei figli, Matteo e Francesco, mi sento di dire i nomi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 175. `stare a` — `stare` (`VERB`)

- Matched words: `stavo, a`
- Representative sentence: `Ma io non ero interessato a quei traffici e me ne stavo al sicuro da ogni problema.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 176. `altra volta` — `volta` (`NOUN`)

- Matched words: `altra, volta`
- Representative sentence: `La fogna in strada si è intasata un’altra volta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 177. `di sera` — `sera` (`NOUN`)

- Matched words: `di, sera`
- Representative sentence: `Grazie a lui hai un letto, un piatto di bring la sera, una tazza di yogurt a pranzo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 178. `per volta` — `volta` (`NOUN`)

- Matched words: `per, volta`
- Representative sentence: `Il sedicesimo giorno, per la prima volta, ho parlato con un ragazzo pakistano poco più grande di me (di solito tra afghani e pakistani non ci si parlava granché).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 179. `di parte` — `partire` (`VERB`)

- Matched words: `di, parte`
- Representative sentence: `E non lo facciamo solo per la fratellanza eccetera, ha detto, ma anche perché quando si parte insieme a degli amici le possibilità che tutto vada bene sono maggiori di quando si parte da soli, senza nessuno cui chiedere aiuto in caso di pericolo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 180. `acqua di Colonia` — `acqua` (`NOUN`)

- Matched words: `acqua, di, colonia`
- Representative sentence: `Ed è stato, ricordo, un viaggio meraviglioso, seduti comodi, insieme a decine e decine di passeggeri locali: odore di acqua di colonia, la carrozza per mangiare e i sedili puliti e morbidi per dormire.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 181. `in ogni caso` — `caso` (`NOUN`)

- Matched words: `in, ogni, caso`
- Representative sentence: `Uscito dal porto - erano trascorsi quindici minuti, direi, in ogni caso meno di mezz’ora - il camion ha rallentato ed è entrato in un cortile, un cortile pieno zeppo di altri camion, motrici, rimorchi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense lacks an evidence_id, so it cannot be cited as confirmed.`

#### 182. `dare torto` — `dare` (`VERB`)

- Matched words: `dar, torto`
- Representative sentence: `Poi un giorno ha detto che lui non sapeva come fare per risarcirli e che comunque non era affare suo, perché lui aveva la sua famiglia cui pensare, cosa che peraltro era vera e perciò non posso dargli torto.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to support the gloss.`

#### 183. `in basso` — `basso` (`NOUN`)

- Matched words: `in, basso`
- Representative sentence: `Il trafficante ha indicato in basso.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence uses the downward-direction sense, but no candidate sense provides an evidence_id to support a confirmed gloss.`

#### 184. `di prima` — `prima` (`ADV`)

- Matched words: `di, prima`
- Representative sentence: `Minuscoli semi neri galleggiavano sperduti sulla superficie unta insieme a briciole di pane, prima li ho spostati con la punta del cucchiaio creando vortici e correnti, poi li ho raccolti, li ho ingoiati e ho finito il pasto bevendo direttamente dalla tazza.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 185. `di tanto in tanto` — `tanto` (`ADV`)

- Matched words: `di, tanto, in, tanto`
- Representative sentence: `Soltan si lamentava - Hussein Alì credo non avesse fiato per farlo, altrimenti si sarebbe lamentato anche lui, come al solito - e di tanto in tanto si sporgeva mezzo nudo facendo segno con le braccia alle macchine di fermarsi, di darci un passaggio.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 186. `andare oltre` — `andare` (`VERB`)

- Matched words: `andare, oltre`
- Representative sentence: `Non volevo andare oltre, altrimenti mi sarei perso, quindi ero preoccupato, com’è giusto.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence describes physically going farther and getting lost, not idiomatic progress or entering into an activity.`

#### 187. `secondo piano` — `secondo` (`ADJ`)

- Matched words: `secondo, piano`
- Representative sentence: `Al secondo piano c’era un magazzino per il cibo degli animali.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase refers literally to a building level, not to either figurative idiom sense.`

#### 188. `lavorare duro` — `lavorare` (`VERB`)

- Matched words: `lavoravi, duro`
- Representative sentence: `Lavoravi tutto il giorno, lavoravi duro, e la sera venivi pagato quanto stabilito eccetera.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 189. `di nascosto` — `nascosto` (`ADJ`)

- Matched words: `di, nascosto`
- Representative sentence: `Prima di spaventarmi troppo, con le mani ho sparpagliato il paragone nell’aria, come faceva un mio amico grande, a Nava, quando fumava di nascosto le radici delle piante, per evitare che l’odore gli impregnasse i vestiti.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id.`

#### 190. `sistema solare` — `sistema` (`NOUN`)

- Matched words: `sistema, solare`
- Representative sentence: `Il cantiere era il sistema solare.`
- Outcome: `literal_use`
- Unresolved reason: `The sentence uses “sistema solare” literally as the astronomical term, not as an idiom.`

#### 191. `da solo` — `solo` (`ADJ`)

- Matched words: `da, soli`
- Representative sentence: `E non lo facciamo solo per la fratellanza eccetera, ha detto, ma anche perché quando si parte insieme a degli amici le possibilità che tutto vada bene sono maggiori di quando si parte da soli, senza nessuno cui chiedere aiuto in caso di pericolo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 192. `andare via` — `andare` (`VERB`)

- Matched words: `andasse, via`
- Representative sentence: `Il fatto, ecco, il fatto è che non me l’aspettavo che lei andasse via davvero.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 193. `non resta che` — `non` (`ADV`)

- Matched words: `non, resta, che`
- Representative sentence: `E quando una cosa è così poco facile da diventare impossibile non resta che smettere di provare e pensare a una alternativa.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 194. `in grande` — `grande` (`ADJ`)

- Matched words: `In, grande`
- Representative sentence: `Nel grande mare, il Mediterraneo, sarei affogato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 195. `fare confusione` — `fare` (`VERB`)

- Matched words: `fare, confusione`
- Representative sentence: `Mi è rimasta impressa e mi sono trovato in bocca la capacità di dire scindere Torino, scindere Torino, in modo da non fare più confusione com’era successo con Rome.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching figurative sense is identifiable, but no evidence_id is provided for it.`

#### 196. `insieme a` — `insieme` (`ADV`)

- Matched words: `insieme, a`
- Representative sentence: `Minuscoli semi neri galleggiavano sperduti sulla superficie unta insieme a briciole di pane, prima li ho spostati con la punta del cucchiaio creando vortici e correnti, poi li ho raccolti, li ho ingoiati e ho finito il pasto bevendo direttamente dalla tazza.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but the candidate sense provides no evidence_id to cite.`

#### 197. `mettere piede` — `mettere` (`VERB`)

- Matched words: `metto, piede`
- Representative sentence: `Ci sono un sacco di cose che non ho ancora visto nella mia vita e che vorrei vedere, e se a questo aggiungiamo che anche a Qom, anche qui, ogni volta che metto piede fuori dalla fabbrica, anche qui è pericoloso, be’, sapete cosa vi dico?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 198. `anche se` — `anche` (`ADV`)

- Matched words: `anche, se`
- Representative sentence: `I soldi che ti servono li guadagnerai lavorando, anche se il lavoro sarà faticoso.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 199. `attorno a` — `attorno` (`ADV`)

- Matched words: `attorno, a`
- Representative sentence: `Tre uomini stavano lavorando attorno alla gru.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 200. `cartello stradale` — `cartello` (`NOUN`)

- Matched words: `cartello, stradale`
- Representative sentence: `Il primo cartello stradale che ho incontrato era un cartello blu.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 201. `prendere coraggio` — `prendere` (`VERB`)

- Matched words: `prendere, coraggio`
- Representative sentence: `Mi bastava sentire: lui ce l’ha fatta, c’è riuscito, è arrivato in Turchia, in Grecia, a Londra, ed ecco che subito mi facevo prendere dal coraggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 202. `di brutto` — `brutto` (`ADJ`)

- Matched words: `di, brutto`
- Representative sentence: `Mi sono seduto sul materasso prima di sdraiarmi e mi sono accorto di quanto fosse brutto il samavat: i muri scrostati, la puzza, la polvere ovunque e, tra la polvere, i pidocchi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 203. `in ogni dove` — `dove` (`ADV`)

- Matched words: `In, ogni, dove`
- Representative sentence: `In ogni stazione dove ci fermavamo, o anche solo se il treno rallentava, mi sporgevo dietro il suo computer e chiedevo please Rome, please Rome.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 204. `carro armato` — `carro` (`NOUN`)

- Matched words: `carri, armati`
- Representative sentence: `Avevo contato anche il numero di ponti fatti saltare dai talebani con la dinamite, e le macchine bruciate e i carri armati neri abbandonati dai militari.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 205. `guardare storto` — `guardare` (`VERB`)

- Matched words: `guardato, storto`
- Representative sentence: `Hussein Alì mi ha guardato storto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 206. `te stesso` — `stesso` (`ADJ`)

- Matched words: `te, stesso`
- Representative sentence: `Questo non per cattiveria o cosa, ma perché prima di occuparti degli altri devi trovare il modo di stare bene con te stesso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 207. `non fare che` — `non` (`ADV`)

- Matched words: `non, far, che`
- Representative sentence: `Mamma camminava con il chador, ma si era portata nel sacchetto un burqa da indossare quando incontravamo delle persone, un modo buono per non far vedere che era una hazara e per nascondere me.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 208. `non lo so` — `non` (`ADV`)

- Matched words: `Non, lo, so`
- Representative sentence: `Non lo so, quanto sono alti?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 209. `in programma` — `programma` (`NOUN`)

- Matched words: `in, programmi`
- Representative sentence: `Non fare nulla non era nei miei programmi, e non potevo ricevere visite, neppure dalla famiglia che mi aveva ospitato.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is supplied without an evidence_id handle.`

#### 210. `a sangue` — `sangue` (`NOUN`)

- Matched words: `a, sangue`
- Representative sentence: `Dopo il bagno abbiamo cominciato a piangere e gridare e a lamentarci finché è arrivata la notte, e di notte i poliziotti di turno hanno meno pazienza, e se ti va male ti picchiano a sangue, ma se ti va bene ti lasciano andare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: empty evidence ID`
- Unresolved reason: —

#### 211. `a distanza` — `distanza` (`NOUN`)

- Matched words: `a, distanza`
- Representative sentence: `L’Iran che, visto dai finestrini, così, a distanza, mi sembrava molto più verde sia del Pakistan sia dell’Afghanistan.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 212. `o meno` — `meno` (`ADV`)

- Matched words: `O, meno`
- Representative sentence: `O per lo meno, quello è stato il primo passo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 213. `di servizio` — `servizio` (`NOUN`)

- Matched words: `di, servizi`
- Representative sentence: `A settembre mi sono iscritto alla scuola superiore, una scuola per Operatore dei servizi sociali, e ho fatto subito una figuraccia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 214. `in grado` — `grado` (`NOUN`)

- Matched words: `in, grado`
- Representative sentence: `Quando finalmente sono stato in grado di muovermi e di respirare ho cercato un posto per fare la pipì, tutta la pipì che durante il viaggio non ero riuscito a fare, tutta la pipì che avevo tenuto per tre giorni.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 215. `non so che` — `non` (`ADV`)

- Matched words: `non, so, che`
- Representative sentence: `A dire il vero non so neppure che differenza ci sia tra andare a destra o a sinistra, uscito dalla porta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 216. `in essere` — `essere` (`VERB`)

- Matched words: `in, ero`
- Representative sentence: `Una gru enorme aveva agganciato il rimorchio in cui ero.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 217. `altro che` — `altro` (`ADJ`)

- Matched words: `altri, che`
- Representative sentence: `Quando ho trovato il porto ho trovato anche un sacco di altri ragazzi afghani che erano lì a gironzolare da giorni e giorni nel tentativo di acquistare il biglietto, e ogni volta che ci provavano li cacciavano, perché si capiva subito che non erano clienti normali, che erano clandestini.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 218. `modo di fare` — `fare` (`VERB`)

- Matched words: `modo, di, fare`
- Representative sentence: `Che non è un bel modo di fare affari, dico io.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence matches the behavior/attitude sense, but no candidate sense includes an evidence_id required to confirm it.`

#### 219. `a colpi di` — `colpo` (`NOUN`)

- Matched words: `a, colpi, di`
- Representative sentence: `Lo abbiamo sgonfiato, prima spremendo l’aria attraverso i tappi, poi, impazienti, squarciando la plastica a colpi di pietra.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 220. `e allora` — `allora` (`ADV`)

- Matched words: `e, allora`
- Representative sentence: `Stava rigida, con il polso appoggiato sul tavolo, e allora io facevo lo stesso, irrigidivo la schiena e poggiavo il polso sul tavolo, e visto che lei si puliva la bocca dopo ogni boccone, pure io mi pulivo la bocca dopo ogni boccone.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 221. `per ora` — `ora` (`NOUN`)

- Matched words: `per, ora`
- Representative sentence: `A Nava, per sapere l’ora, misuravo l’ombra con i passi e quando non c’era il sole tiravo a indovinare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 222. `scendere in campo` — `scendere` (`VERB`)

- Matched words: `scesa, in, campo`
- Representative sentence: `Poi ha fatto una pausa, mentre la squadra che era scesa in campo dopo di noi ha segnato e tutti hanno urlato per la gioia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 223. `tirare su` — `tirare` (`VERB`)

- Matched words: `tirare, su`
- Representative sentence: `Siamo riusciti a tornare a terra - una fatica immensa - e a tirare il gommone sulle pietre.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 224. `da brivido` — `brivido` (`NOUN`)

- Matched words: `da, brividi`
- Representative sentence: `Respirando producevo un sibilo sottile, ed ero percorso da certi brividi che neppure un terremoto avrebbe scosso tanto le mie fondamenta.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 225. `in mezzo a` — `mezzo` (`NOUN`)

- Matched words: `in, mezzo, a`
- Representative sentence: `Ha detto a me e a mio fratello di stare sempre fuori casa, in mezzo agli altri bambini, perché la sera che i pashtun erano stati a casa nostra noi non c’eravamo e non ci avevano visto in faccia.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but the candidate sense provides no evidence_id to cite.`

#### 226. `pollo fritto` — `friggere` (`ADJ`)

- Matched words: `pollo, fritto`
- Representative sentence: `Hanno portato da mangiare: riso con pollo fritto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 227. `rete metallica` — `rete` (`NOUN`)

- Matched words: `rete, metallica`
- Representative sentence: `Il camion era parcheggiato in un cortile polveroso circondato da una rete metallica.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no evidence_id was supplied for citation.`

#### 228. `fare finta di nulla` — `fare` (`VERB`)

- Matched words: `far, finta, di, nulla`
- Representative sentence: `Ho fatto segno di far finta di nulla, come se non ci conoscessimo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 229. `stare dietro` — `stare` (`VERB`)

- Matched words: `star, dietro`
- Representative sentence: `Ma noi siamo scappati veloci, e lui era troppo malmesso per starci dietro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 230. `in parte` — `parte` (`NOUN`)

- Matched words: `in, parte`
- Representative sentence: `La stazione, ricordo, era un edificio lungo e basso, con una scalinata di pietra, un porticato a onde e un cartello sopra il tetto, in parte blu e in parte trasparente, su cui c’era scritto Kerman Railway Station in giallo e, in rosso, la stessa cosa in farsi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 231. `si vede` — `vedere` (`VERB`)

- Matched words: `si, vede`
- Representative sentence: `Io vedevo la frutta che nasceva: i fiori sbocciavano davanti ai miei occhi e diventavano frutta; anche qui i fiori diventano frutta, ma non lo si vede.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase is used literally to mean that the transformation cannot be seen, not figuratively.`

#### 232. `mi sa` — `sapere` (`VERB`)

- Matched words: `mi, sa`
- Representative sentence: `Se tutti gli italiani sono così, ho pensato, mi sa che questo è un posto in cui potrei anche fermarmi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 233. `di sotto` — `sotto` (`ADV`)

- Matched words: `di, sotto`
- Representative sentence: `Ora scendete di sotto, in cortile, presentatevi agli altri operai e fate quello che vi dicono di fare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 234. `mettere a dormire` — `dormire` (`VERB`)

- Matched words: `messi, a, dormire`
- Representative sentence: `Era una specie di stalla, enorme e con il soffitto altissimo, una stalla che al posto delle mucche ospitava i clandestini, e noi afghani ci hanno messi a dormire accanto ai pakistani, che non è una buona idea.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 235. `per il verso giusto` — `giusto` (`ADJ`)

- Matched words: `per, il, verso, giusto`
- Representative sentence: `Passata quella dogana abbiamo camminato per alcune ore verso la città più vicina, ma ormai era chiaro che qualcosa non stava andando per il verso giusto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 236. `dito indice` — `dito` (`NOUN`)

- Matched words: `dito, indice`
- Representative sentence: `Con un filo di voce e il dito indice puntato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 237. `in tondo` — `tondo` (`NOUN`)

- Matched words: `in, tondo`
- Representative sentence: `Girando in tondo attorno al punto in cui pensavamo potesse essere, anche se, con tutta probabilità, ne eravamo già lontani.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: empty evidence ID`
- Unresolved reason: —

#### 238. `perché mai` — `mai` (`ADV`)

- Matched words: `perché, mai`
- Representative sentence: `Devo dire la verità, perché se mai qualcuno leggerà queste parole, qualcuno degli uomini che ho incontrato a Esfahàn, voglio che lo sappiano, dato che credo di non averglielo mai detto: mi sono trovato bene, lì nel cantiere.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 239. `far notare` — `fare` (`VERB`)

- Matched words: `far, notare`
- Representative sentence: `Vorrei far notare che io ero molto più piccolo di loro e molto meno numeroso.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 240. `più o meno` — `più` (`ADV`)

- Matched words: `più, o, meno`
- Representative sentence: `Sì, direi diciotto mesi, più o meno.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 241. `davanti a` — `davanti` (`ADV`)

- Matched words: `davanti, a`
- Representative sentence: `Nel posto davanti al mio c’era un signore grosso che ha subito aperto il computer portatile per lavorare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 242. `di colpo` — `colpo` (`NOUN`)

- Matched words: `di, colpi`
- Representative sentence: `Ho seguito la scena di loro che correvano, che venivano raggiunti e catturati (sono volati dei colpi di manganello).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 243. `dare la precedenza` — `dare` (`VERB`)

- Matched words: `dato, la, precedenza`
- Representative sentence: `Per fortuna è intervenuta un’altra insegnante che ha detto di avere pazienza, che avremmo fatto un passo alla volta, che igiene e psicologia potevano anche aspettare, e che avremmo dato la precedenza alle altre materie.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 244. `acqua di mare` — `acqua` (`NOUN`)

- Matched words: `acqua, di, mare`
- Representative sentence: `Soltan ha provato a bere l’acqua del mare, e dopo la prima boccata ha cominciato a urlare che l’acqua era avvelenata, che i turchi e i greci l’avevano avvelenata per farci morire.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense lacks an evidence_id, so it cannot be cited for a confirmed gloss.`

#### 245. `andare a letto` — `andare` (`VERB`)

- Matched words: `andato, a, letto`
- Representative sentence: `Sono rimasto un po’ lì a sentire, poi ho mangiato della minestra e me ne sono andato a letto.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence clearly uses the sense “to go to bed,” but the matching candidate sense provides no evidence_id to cite.`

#### 246. `quanto a` — `quanto` (`ADV`)

- Matched words: `quanto, a`
- Representative sentence: `Al largo, non saprei dire quanto al largo, stava passando una barca che emetteva dei bagliori rossi e verdi ai lati, e sarà stato per quelle luci rosse e verdi o non so cosa, ma ci siamo convinti che fosse la guardia costiera.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 247. `da grande` — `grande` (`ADJ`)

- Matched words: `da, grandi`
- Representative sentence: `E’ andato via anche il signore gentile dalle mani grandi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 248. `di persona` — `persona` (`NOUN`)

- Matched words: `di, persone`
- Representative sentence: `Per volontà di Enaiatollah Akbari i nomi di alcune persone citate nel libro sono stati cambiati.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 249. `per primo` — `primo` (`ADJ`)

- Matched words: `per, prima`
- Representative sentence: `Il sedicesimo giorno, per la prima volta, ho parlato con un ragazzo pakistano poco più grande di me (di solito tra afghani e pakistani non ci si parlava granché).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 250. `a termine` — `termine` (`NOUN`)

- Matched words: `A, termine`
- Representative sentence: `Al termine di una prima salita, da dietro una grande roccia è sbucato un gruppo di persone.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 251. `andare avanti` — `andare` (`VERB`)

- Matched words: `Andare, avanti`
- Representative sentence: `Andare avanti o tornare indietro è la stessa cosa, e io preferisco morire in mare che dover rifare da capo tutta la strada che ho fatto fino ad adesso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 252. `in comune` — `comune` (`NOUN`)

- Matched words: `in, comune`
- Representative sentence: `In Iran succede che le case siano l’una vicina all’altra e che abbiano una piazzetta in comune, al centro, e che la piazzetta abbia solo due accessi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 253. `qualche volta` — `volta` (`NOUN`)

- Matched words: `qualche, volta`
- Representative sentence: `Loro si fidavano di me, mi accarezzavano la testa e qualche volta mi regalavano un dolce o che so io.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but the candidate senses provide no evidence_id handle to cite.`

#### 254. `cabina telefonica` — `cabina` (`NOUN`)

- Matched words: `cabina, telefonica`
- Representative sentence: `Lì accanto c’era un negozietto con una pompa di benzina e, sulla destra, scrostata e unta, una vecchia cabina telefonica mezzo nascosta dalle fronde di un albero.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id, so it cannot be cited as required.`

#### 255. `numero uno` — `numero` (`NOUN`)

- Matched words: `numero, un`
- Representative sentence: `C’era scritto un numero con un pennarello verde.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 256. `in viaggio` — `viaggio` (`NOUN`)

- Matched words: `in, viaggio`
- Representative sentence: `Stanco di essere sempre in viaggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 257. `di solito` — `solito` (`NOUN`)

- Matched words: `di, solito`
- Representative sentence: `Il sedicesimo giorno, per la prima volta, ho parlato con un ragazzo pakistano poco più grande di me (di solito tra afghani e pakistani non ci si parlava granché).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 258. `fine settimana` — `settimana` (`NOUN`)

- Matched words: `fine, settimana`
- Representative sentence: `Così, alla fine della settimana che mi ero preso dal lavoro, sono tornato a Baharestan.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 259. `a fine di` — `fine` (`NOUN`)

- Matched words: `a, fine, di`
- Representative sentence: `Quando sono andato a dormire, alla fine della giornata, ero molto più che khasta kofta: ero mangime per galline.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 260. `a nome di` — `nome` (`NOUN`)

- Matched words: `a, nome, di`
- Representative sentence: `Quando le onde della nave - no, non si è fermata, la nave; forse ci ha visto, forse no, non lo abbiamo capito - dicevo, quando le onde si sono calmate, be’, abbiamo continuato a remare e a gridare il nome di Liaqat.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 261. `notte fonda` — `notte` (`NOUN`)

- Matched words: `notte, fonda`
- Representative sentence: `Quando ci siamo contati, quando finalmente abbiamo potuto, durante una breve sosta a notte fonda, eravamo settantasette.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 262. `a terra` — `terra` (`NOUN`)

- Matched words: `a, terra`
- Representative sentence: `Ripassavo le azioni tra me e me, per essere svelto e preciso: saltare a terra, atterrare sulla punta dei piedi, rotolare, se era necessario, per attutire il colpo, cercare una via di fuga, correre, non voltarmi, correre.`
- Outcome: `literal_use`
- Unresolved reason: `Here, “a terra” literally means moving to the ground as part of the actions being rehearsed.`

#### 263. `senza senso` — `senso` (`NOUN`)

- Matched words: `senza, senso`
- Representative sentence: `Rischiano in continuazione di morire per un nonnulla, di essere uccisi per una parola di troppo o per qualche regola senza senso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 264. `prendere posto` — `prendere` (`VERB`)

- Matched words: `preso, posto`
- Representative sentence: `Il nostro trafficante e i suoi soci hanno preso posto a tre o quattro file da me e da Sufi e da tutti gli altri afghani, in modo da poterci controllare confusi tra i passeggeri.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 265. `in mano` — `mano` (`NOUN`)

- Matched words: `in, mano`
- Representative sentence: `Passavo di corsa, con un secchio in mano.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase describes physically holding a bucket, not having a situation under control.`

#### 266. `a posto` — `posto` (`NOUN`)

- Matched words: `a, posto`
- Representative sentence: `Ma lei non ha risposto e nessuno ha risposto al posto suo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 267. `ci vediamo` — `vedere` (`VERB`)

- Matched words: `ci, vediamo`
- Representative sentence: `Io ho sorriso come dire ma certo, ci vediamo presto, stammi bene.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 268. `in scatola` — `scatola` (`NOUN`)

- Matched words: `in, scatola`
- Representative sentence: `Abbiamo messo tutto in una scatola di cartone, tenuta insieme da un cordino in modo che si potesse portare a tracolla, e siamo tornati indietro.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 269. `saltare fuori` — `fuori` (`ADV`)

- Matched words: `saltati, fuori`
- Representative sentence: `A un colpo di clacson noi siamo saltati fuori e siamo saliti sulla corriera, tra le facce stupite delle donne e dei bambini e anche di alcuni uomini che hanno provato a protestare, ma sono stati subito zittiti.`
- Outcome: `literal_use`
- Unresolved reason: `Here saltati fuori describes physically jumping out, not the idiomatic meaning of appearing or emerging unexpectedly.`

#### 270. `da piccolo` — `piccolo` (`ADJ`)

- Matched words: `da, piccole`
- Representative sentence: `Dell’assenza di una persona te ne accorgi dalle piccole cose.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 271. `in fretta` — `fretta` (`NOUN`)

- Matched words: `in, fretta`
- Representative sentence: `Lo abbiamo piegato in fretta e lo abbiamo nascosto sotto una roccia, e abbiamo coperto la roccia di sabbia.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id to cite.`

#### 272. `di botto` — `botto` (`NOUN`)

- Matched words: `di, botto`
- Representative sentence: `Poi, invece, ci sollevavano e ci passavano sotto, e alla fine, quando eravamo sulla cresta, ci lasciavano cadere di botto, come certe giostre su cui mi è capitato di andare qui in Italia, al luna park.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id to cite.`

#### 273. `da tempo` — `tempo` (`NOUN`)

- Matched words: `da, tempo`
- Representative sentence: `In ogni caso, ecco, il fatto era che solo uno a settimana, ma uno di quelli che erano lì da molto tempo, uno che sapeva come muoversi in città, faceva la spesa per tutti.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no evidence_id was supplied for it.`

#### 274. `in caso` — `caso` (`NOUN`)

- Matched words: `in, caso`
- Representative sentence: `E non lo facciamo solo per la fratellanza eccetera, ha detto, ma anche perché quando si parte insieme a degli amici le possibilità che tutto vada bene sono maggiori di quando si parte da soli, senza nessuno cui chiedere aiuto in caso di pericolo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no evidence_id is supplied for any candidate sense.`

#### 275. `andare e venire` — `andare` (`VERB`)

- Matched words: `andare, e, venire`
- Representative sentence: `Quetta c’erano moltissimi hazara, li avevo visti andare e venire dal samavat i giorni precedenti, quando c’era ancora mamma, che si era intrattenuta a parlare con loro per molto tempo, come avesse dei gran segreti da confidare.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 276. `buona giornata` — `buono` (`ADJ`)

- Matched words: `buona, giornata`
- Representative sentence: `E’ una buona giornata per voi questa, ha detto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 277. `ad esempio` — `esempio` (`NOUN`)

- Matched words: `ad, esempio`
- Representative sentence: `Ho sentito della musica e credo di avere chiesto qualcosa: chi stava suonando, ad esempio.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id, so it cannot be cited as required.`

#### 278. `a casa mia` — `casa` (`NOUN`)

- Matched words: `a, casa, mia`
- Representative sentence: `Ho paragonato quel posto a casa mia, ma è durato solo un istante.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 279. `volere dire` — `dire` (`VERB`)

- Matched words: `vuol, dire`
- Representative sentence: `Kaka Rahim, cosa vuol dire non torna?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 280. `prendere di mira` — `prendere` (`VERB`)

- Matched words: `preso, di, mira`
- Representative sentence: `Non c’era nessuno, e questo era un bene, ma io mi annoiavo, così ho preso di mira una campana appesa a un traliccio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 281. `di sempre` — `sempre` (`ADV`)

- Matched words: `di, sempre`
- Representative sentence: `Stanco di essere sempre in viaggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 282. `di sicuro` — `sicuro` (`ADJ`)

- Matched words: `di, sicuro`
- Representative sentence: `Ho dormito nei parchi per tanto tempo, ho detto, che qualche altra notte di sicuro non mi farà male.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 283. `tutto quanto` — `quanto` (`ADV`)

- Matched words: `tutti, quanto`
- Representative sentence: `Arrivare, soprattutto, che è la cosa più importante (a partire sono capaci tutti): quanto avrei dovuto spendere?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 284. `dare retta` — `dare` (`VERB`)

- Matched words: `dar, retta`
- Representative sentence: `Non riuscivo a capirli e a farmi capire, così dopo un po’ quelli smettevano di darmi retta e si giravano per tornare alle loro faccende, che a quanto pare erano più urgenti della mia condizione di abbandonato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 285. `dare in pasto` — `dare` (`VERB`)

- Matched words: `dati, in, pasto`
- Representative sentence: `Si raccontava che, da quelle parti, gli hazara di passaggio come noi venissero presi dai talebani e gettati vivi in un pozzo profondissimo o dati in pasto ai cani randagi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is present, but no evidence_id handle is supplied for citation.`

#### 286. `niente male` — `male` (`ADV`)

- Matched words: `niente, male`
- Representative sentence: `E poi c’era il fatto di avere dei soldi miei, che non era per niente male.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 287. `è una parola` — `parola` (`NOUN`)

- Matched words: `è, una, parola`
- Representative sentence: `Vedete, scin è una parola iraniana che significa sasso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 288. `mettere sotto` — `mettere` (`VERB`)

- Matched words: `messe, sotto`
- Representative sentence: `Mi sono tolto le calze e le ho messe sotto il letto.`
- Outcome: `literal_use`
- Unresolved reason: `The socks are literally placed under the bed, not used in either figurative sense.`

#### 289. `a occhio` — `occhio` (`NOUN`)

- Matched words: `a, occhi`
- Representative sentence: `C’erano così tante moto e macchine che l’aria era irrespirabile, e la fogna non scorreva sotto il cemento, nascosta agli occhi e al naso, ma tra la carreggiata e il marciapiede, a pochi metri di distanza dalla porta del samavat.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 290. `a vicenda` — `vicenda` (`NOUN`)

- Matched words: `a, vicenda`
- Representative sentence: `Jamal, allora, mi ha dato un colpetto con la gamba per svegliarmi (si dormiva vicini, noi due, per proteggerci a vicenda).`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 291. `di corsa` — `corsa` (`NOUN`)

- Matched words: `di, corsa`
- Representative sentence: `Appena sentivamo il rumore di un motore alle nostre spalle, mamma e io ci nascondevamo di corsa in un fosso o tra gli arbusti o dietro le pietre, se c’erano pietre abbastanza grandi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id handle to cite.`

#### 292. `in ultimo` — `ultimo` (`ADJ`)

- Matched words: `in, ultimi`
- Representative sentence: `Io sono andato tre volte, negli ultimi mesi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 293. `ridere dietro` — `ridere` (`VERB`)

- Matched words: `ridere, dietro`
- Representative sentence: `Dev’essere per questo che quando si è reso conto che erano solo pietruzze e ci ha visto ridere dietro il muro di una moschea ha cominciato a rincorrerci per farci friggere nell’olio delle patate che noi chiamiamo chips, come gli inglesi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 294. `prendere il sole` — `prendere` (`VERB`)

- Matched words: `prendeva, il, sole`
- Representative sentence: `Con un balzo all’indietro è saltato su un muretto facendo scappare una lucertola che prendeva il sole.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 295. `fare fuoco` — `fare` (`VERB`)

- Matched words: `fare, fuoco`
- Representative sentence: `La mia casa era fatta così: una stanza per tutti, dove si dormiva, una stanza per gli ospiti, e un angolo per fare il fuoco e cucinare, che era più basso del pavimento, in modo tale che d’inverno il calore del fuoco lo scaldasse grazie a un sistema di condotte.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 296. `me stesso` — `stesso` (`ADJ`)

- Matched words: `me, stesso`
- Representative sentence: `Poi, una sera, prima di dormire lei mi ha preso la testa e me l’ha stretta forte, mi ha detto tre cose che non dovevo fare, ha detto che avrei dovuto desiderare qualcosa, con tutto me stesso.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 297. `a volta` — `volta` (`NOUN`)

- Matched words: `a, volte`
- Representative sentence: `Per conoscere le cose a volte serve domandare: ho chiesto ad alcuni amici fidati.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 298. `avere presente` — `presente` (`ADJ`)

- Matched words: `avete, presente`
- Representative sentence: `Un poliziotto ci aveva fatti sistemare in fila contro un muro e stava passando a controllare che tutti avessimo svuotato le tasche e quando vedeva qualcuno comportarsi in modo strano, oppure muoversi senza permesso o fare quella faccia estranea - avete presente?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 299. `che cosa` — `cosa` (`NOUN`)

- Matched words: `che, cosa`
- Representative sentence: `Arrivare, soprattutto, che è la cosa più importante (a partire sono capaci tutti): quanto avrei dovuto spendere?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 300. `far sapere` — `fare` (`VERB`)

- Matched words: `far, sapere`
- Representative sentence: `Conoscevo solo questo dei loro Paesi, e volevo fargli sapere che li apprezzavo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 301. `spina dorsale` — `spina` (`NOUN`)

- Matched words: `spina, dorsale`
- Representative sentence: `Quando i raggi del sole illuminavano le creste irregolari, che parevano la spina dorsale di un morto, la carovana si fermava.`
- Outcome: `literal_use`
- Unresolved reason: `The phrase is used literally in an anatomical comparison, not as a distinct idiomatic sense.`

#### 302. `in vista` — `vista` (`NOUN`)

- Matched words: `in, vista`
- Representative sentence: `Ma lì, be’, i preti avevano disposto una serie di Bibbie in tutte le lingue - anche la mia - in bella vista, vicino al portone d’ingresso, e prima di mangiare dovevi leggerne una pagina, se no non te ne davano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 303. `rispetto a` — `rispetto` (`ADV`)

- Matched words: `rispetto, a`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 304. `in fondo` — `fondo` (`NOUN`)

- Matched words: `in, fondo`
- Representative sentence: `D’un tratto, in fondo, ho visto spuntare due figure che si muovevano veloci.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 305. `di certo` — `certo` (`ADV`)

- Matched words: `Di, certo`
- Representative sentence: `Di certo lo avrei detto in un altro modo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense has no evidence_id handle, so it cannot support a confirmed gloss.`

#### 306. `a fondo` — `fondo` (`NOUN`)

- Matched words: `a, fondo`
- Representative sentence: `Gli amici, in Grecia, mi avevano suggerito di non scendere subito, di aspettare che il camion penetrasse a fondo nel Paese (qualunque Paese fosse), che si allontanasse dalle frontiere e poi di approfittare di una sosta dell’autista, magari a un autogrill, per sgattaiolare via.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 307. `parco giochi` — `parco` (`NOUN`)

- Matched words: `parco, giochi`
- Representative sentence: `E una piazzetta con delle panchine e un parco giochi.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 308. `di nuovo` — `nuovo` (`ADJ`)

- Matched words: `di, nuovo`
- Representative sentence: `Con questa, ho detto di nuovo.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense lacks an evidence_id, so it cannot be cited.`

#### 309. `in base a` — `base` (`NOUN`)

- Matched words: `in, base, a`
- Representative sentence: `Ogni mansione era affidata in base all’età.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 310. `fare finta di niente` — `fare` (`VERB`)

- Matched words: `facendo, finta, di, niente`
- Representative sentence: `Da alzarsi facendo finta di niente, dimenticarlo su una pietra e non trovarlo più, quel giorno.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to support confirmation.`

#### 311. `fare in modo` — `fare` (`VERB`)

- Matched words: `fatto, in, modo`
- Representative sentence: `Avrei fatto tutto in un altro modo e con altra delicatezza.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 312. `di lì a` — `lì` (`ADV`)

- Matched words: `di, lì, a`
- Representative sentence: `Sono entrato, ho stretto il telefono in mano e ho fatto finta di essere lì a telefonare, invece spiavo i miei compagni, per vedere cosa combinavano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 313. `doppio fondo` — `fondo` (`NOUN`)

- Matched words: `doppio, fondo`
- Representative sentence: `Insomma, il camion aveva un doppio fondo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 314. `a presto` — `presto` (`ADV`)

- Matched words: `A, presto`
- Representative sentence: `Al mattino presto, nel silenzio e nella luce bianca dell’alba, abbiamo raggiunto un piccolo villaggio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 315. `per via di` — `via` (`NOUN`)

- Matched words: `per, via, di`
- Representative sentence: `Da voi una cosa del genere non potrebbe esistere, per via dell’igiene.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 316. `pieno di vita` — `vita` (`NOUN`)

- Matched words: `piene, di, vita`
- Representative sentence: `Andavo per i quattordici e forse, be’, li stavo anche superando i miei primi quattordici anni, quando ho deciso che dall’Iran me ne sarei andato: ne avevo le tasche piene di quella vita.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 317. `per poco` — `poco` (`ADV`)

- Matched words: `per, poco`
- Representative sentence: `Gli ho portato l’acqua rimasta, e questo mi ha fatto sentire bene, anche se per poco, mi ha fatto sentire umano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 318. `in faccia` — `faccia` (`NOUN`)

- Matched words: `in, faccia`
- Representative sentence: `Ha detto a me e a mio fratello di stare sempre fuori casa, in mezzo agli altri bambini, perché la sera che i pashtun erano stati a casa nostra noi non c’eravamo e non ci avevano visto in faccia.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is supplied without an evidence_id, so it cannot be cited for a confirmed gloss.`

#### 319. `di passaggio` — `passaggio` (`NOUN`)

- Matched words: `di, passaggio`
- Representative sentence: `Si raccontava che, da quelle parti, gli hazara di passaggio come noi venissero presi dai talebani e gettati vivi in un pozzo profondissimo o dati in pasto ai cani randagi.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching sense is clear, but no evidence_id handle is supplied for it.`

#### 320. `avere le tasche piene` — `avere` (`VERB`)

- Matched words: `avevo, le, tasche, piene`
- Representative sentence: `Andavo per i quattordici e forse, be’, li stavo anche superando i miei primi quattordici anni, quando ho deciso che dall’Iran me ne sarei andato: ne avevo le tasche piene di quella vita.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 321. `più che altro` — `più` (`ADV`)

- Matched words: `più, che, altro`
- Representative sentence: `Era il mio orologio, e ci tenevo più che a qualunque altro oggetto o che so io.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 322. `e passa` — `passare` (`VERB`)

- Matched words: `e, passa`
- Representative sentence: `Hanno riempito il doppio fondo con noi, con tutti noi, con tutti e cinquanta e passa o quanti eravamo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 323. `non si sa mai` — `non` (`ADV`)

- Matched words: `Non, si, sa, mai`
- Representative sentence: `Non si sa mai, ha detto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 324. `fare luce` — `fare` (`VERB`)

- Matched words: `fare, luce`
- Representative sentence: `Per fare luce usavamo le lampade a petrolio.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 325. `fin qui` — `fino` (`ADV`)

- Matched words: `fin, qui`
- Representative sentence: `E’ l’odore della fogna, sta arrivando fin qui.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 326. `vecchio mio` — `vecchio` (`ADJ`)

- Matched words: `vecchio, mio`
- Representative sentence: `Ho pensato che o c’era da qualche parte una riserva di buone fortune diverse a seconda delle occasioni, oppure era la stessa buona fortuna che mi aveva augurato il vecchio amico di mio padre dopo averci accompagnato a Kandahar.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 327. `occhiali da sole` — `sole` (`NOUN`)

- Matched words: `occhiali, da, sole`
- Representative sentence: `Ho imparato molte cose che mi hanno costretto a guardare la vita con occhi diversi, come quando ti metti un paio di occhiali da sole con le lenti colorate.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 328. `giù di lì` — `lì` (`ADV`)

- Matched words: `giù, di, lì`
- Representative sentence: `Il mare ha cominciato ad agitarsi verso mezzanotte, credo, o giù di lì.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 329. `fuori luogo` — `luogo` (`NOUN`)

- Matched words: `fuori, luogo`
- Representative sentence: `Ho sgranato gli occhi, poi li ho abbassati, avrei voluto metterli in tasca, gli occhi, o spegnere il fuoco sulle guance, ma, ecco, la sua apparizione mi aveva colto talmente alla sprovvista che qualsiasi posizione, ogni mossa, ogni respiro mi sembrava goffo e fuori luogo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 330. `buona fede` — `buono` (`ADJ`)

- Matched words: `buona, fede`
- Representative sentence: `Lo diceva in buona fede - ne sono certo, a causa del cugino - e lo diceva in buona fede perché credeva di sapere come sarebbe stata la nostra camminata fra le montagne, ma non lo sapeva affatto, perché lui non c’era mai stato.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 331. `per tempo` — `tempo` (`NOUN`)

- Matched words: `per, tempo`
- Representative sentence: `Khasta kofta significa stanco come una polpetta, perché quando le donne fanno le polpette, dalle nostre parti, le battono e le battono e le battono, per un tempo lungo, nell’incavo della mano.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 332. `una volta che` — `volta` (`NOUN`)

- Matched words: `una, volta, che`
- Representative sentence: `Il talebano è passato tra di noi, respirando forte com’era capitato a me una volta che mi ero infilato una pietruzza nel naso e non riuscivo più a toglierla.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching temporal sense is clear, but no candidate sense provides an evidence_id to cite.`

#### 333. `un tempo` — `tempo` (`NOUN`)

- Matched words: `un, tempo`
- Representative sentence: `Mi sono fatto accompagnare da un afghano che era qui da un po’ di tempo e parlava abbastanza bene la lingua, per acquistare il biglietto e non sbagliare treno.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 334. `fare bene` — `fare` (`VERB`)

- Matched words: `fatte, bene`
- Representative sentence: `Quindi, tanto per essere sicuri e fare le cose fatte bene, ce lo hanno richiesto una terza.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching candidate sense has no evidence_id to cite.`

#### 335. `dare fondo` — `dare` (`VERB`)

- Matched words: `dato, fondo`
- Representative sentence: `Una buona giornata, sì, è stata un buona giornata, quella successiva, anche se abbiamo dato fondo alle riserve di cibo e di acqua.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 336. `fuori casa` — `casa` (`NOUN`)

- Matched words: `fuori, casa`
- Representative sentence: `Ha detto a me e a mio fratello di stare sempre fuori casa, in mezzo agli altri bambini, perché la sera che i pashtun erano stati a casa nostra noi non c’eravamo e non ci avevano visto in faccia.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 337. `un sacco` — `sacco` (`NOUN`)

- Matched words: `un, sacco`
- Representative sentence: `Sono atterrato come un sacco di patate (nonostante le prove mentali di prima), perché le gambe erano di legno e non potevano ammortizzare il salto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 338. `qualunque cosa` — `cosa` (`NOUN`)

- Matched words: `qualunque, cosa`
- Representative sentence: `Avrei fatto qualunque cosa, da lavare per terra a pulire le scarpe, qualunque cosa ci fosse da fare, e questo perché, a dirla tutta, avevo una gran paura a uscire per strada.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: context_only contradicts evidence IDs`
- Unresolved reason: —

#### 339. `sempre che` — `sempre` (`ADV`)

- Matched words: `sempre, che`
- Representative sentence: `Ho fatto un giro lungo, non ricordo bene dove e perché, ma credo anche di essermi perso, sempre che sia possibile perdersi quando non si sa dove andare.`
- Outcome: `ambiguous`
- Unresolved reason: `The matching conditional sense has no evidence_id handle to cite.`

#### 340. `a mano a mano` — `mano` (`NOUN`)

- Matched words: `A, mano, a, mano`
- Representative sentence: `A mano a mano che ci av-vicinavamo all’isola, la luce schiariva, ed è stato a quel punto che, su una collina, Soltan ha visto una bandiera.`
- Outcome: `provider error`
- Error: `call LLM: Post "https://api.openai.com/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
- Unresolved reason: —

#### 341. `in salita` — `salita` (`NOUN`)

- Matched words: `in, salita`
- Representative sentence: `Non riusciamo ad andare verso la Grecia perché di là il mare è in salita.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 342. `senza nome` — `nome` (`NOUN`)

- Matched words: `senza, nome`
- Representative sentence: `Il giornalista raccontava di un ragazzino senza nome che era stato ripreso dalle telecamere mentre tagliava la gola a un prigioniero urlando Allah Akbar.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 343. `per il resto` — `resto` (`NOUN`)

- Matched words: `per, il, resto`
- Representative sentence: `Capirete presto come funziona la vita qui dentro, per lavarsi, per mangiare, per pregare e per il resto.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 344. `fuori di` — `fuori` (`ADV`)

- Matched words: `fuori, di`
- Representative sentence: `Dopo tre volte di su e giù siamo emersi e fuori, sopra di noi, c’era un grande parco, tantissima gente e un concerto, un concerto nel parco, in quel parco che si chiamava Dikastirion, se ben ricordo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 345. `per fortuna` — `fortuna` (`NOUN`)

- Matched words: `Per, fortuna`
- Representative sentence: `Per fortuna è intervenuta un’altra insegnante che ha detto di avere pazienza, che avremmo fatto un passo alla volta, che igiene e psicologia potevano anche aspettare, e che avremmo dato la precedenza alle altre materie.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: empty evidence ID`
- Unresolved reason: —

#### 346. `qualsiasi cosa` — `cosa` (`NOUN`)

- Matched words: `Qualsiasi, cosa`
- Representative sentence: `Qualsiasi cosa succeda noi non ci conosciamo.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 347. `da vicino` — `vicino` (`ADV`)

- Matched words: `da, vicino`
- Representative sentence: `Guardandoli più da vicino mi sono accorto che erano pali della luce.`
- Outcome: `ambiguous`
- Unresolved reason: `The sentence matches the closely-inspecting sense, but no evidence_id is provided for any candidate sense.`

#### 348. `a piacere` — `piacere` (`VERB`)

- Matched words: `a, piaciuto`
- Representative sentence: `Eravamo in cinque, e tra i cinque c’era un signore anziano che a me è piaciuto subito: si chiamava Zaman.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 349. `per quanto` — `quanto` (`ADV`)

- Matched words: `per, quanto`
- Representative sentence: `Agha, per favore, quanto manca alla cima della montagna?`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

#### 350. `a pelo` — `pelo` (`NOUN`)

- Matched words: `a, peli`
- Representative sentence: `Sono rimasto zitto a osservare i peli sulle guance del padrone del samavat, ma senza vederli davvero.`
- Outcome: `invalid response`
- Error: `enrichment: invalid translation response: decode LLM translation: translation is empty`
- Unresolved reason: —

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

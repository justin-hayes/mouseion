# Idiom card review — `idiom-card-v1`

Status: **Operator review required**

- Date: 2026-11-08
- Model: `gpt-5.6-luna`
- Reasoning setting: `low`
- Prompt contract: `idiom-card-v1`
- Provider: `openai-compatible` (`gpt-5.6-luna/idiom-card-v1-reasoning-low`)
- Case set: `idiom-review-v1` (8 cases)
- Matcher version: `idiom-matcher/1`
- Matcher gap bound: 2
- Sample: 30 confirmed idiom cards requested from Book `34c9236b-0e14-4e9d-aac7-02d10611c271` in `it` (seed `2026-11-08`); selected 30
- Sample source: 363 distinct idiom items in the Book's current deck preparation, frozen in memory with idiom cards on and no per-deck bound (a prepared deck keeps 30); 36 provider calls, 6 not confirmed
- Rubric: [doc/reviews/README.md](README.md)

## Summary

- Cases run: 8
- Outcomes: confirmed=4, literal_use=2, ambiguous=0, unresolved=0, invalid response=0, provider error=0, lemma-card controls=2
- Automatic checks: 8 passed, 0 failed

## Gate

The gate passes only when every item below is true:

- every positive case is confirmed with a citation-form Gloss;
- no literal-use control is confirmed and every lemma-card control passes its automatic check;
- none of the sampled cards teaches a wrong meaning.

- [ ] Gate evaluated and recorded: **fail (draft)** — sample cards 4 (*di fronte*), 13 (*rispetto a*) and 25 (*fare bene*) teach a wrong meaning in context. Every case and the other 27 sample cards pass.

Verdicts drafted by Claude on 2026-11-08 at the operator's request; they await the operator's confirmation. Observations beyond the gate:

- Card 25 is a matcher/provider miss: the words are used literally, but the provider confirmed the idiom anyway, even though one candidate sense says "Used other than figuratively or idiomatically".
- Cards 4 and 13 show the Gloss following the dictionary's first sense rather than the sense the sentence uses (the translation for card 4 is right; the Gloss is not).
- Several sampled items (*sistema solare*, *acqua di mare*, *scuola superiore*, *stare in piedi*) are transparent compounds from Wiktionary's multi-word entries rather than idioms. They are correct but low value.

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

- Outcome: `literal_use`
- Translation: `to run away`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The sentence refers literally to cutting the string of a package, not to running away.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `literal_use` | **PASS** |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

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
- Translation: `to pull someone's leg`
- Raw Gloss: —
- Evidence IDs: —
- Context only: true
- Unresolved kind: `literal_use`
- Unresolved reason: `The sentence describes physically picking up a child, not teasing or fooling someone.`

### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `literal_use` | **PASS** |

### Reviewer verdicts

Leave these blank for the operator; the command cannot judge them.

- [x] Citation form — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No definition paraphrase — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] No sentence inflection or arguments — verdict: n/a — no card was produced; the provider correctly returned `literal_use`
- [x] Meaning correct in context — verdict: pass — she physically picks the child up; rejecting the idiom is correct

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

- [x] Anchor card moved to an idiom-free sentence — verdict: pass — the anchor card uses *Mi fa male la gamba.*

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

- [x] Anchor card moved to an idiom-free sentence — verdict: pass — the anchor card uses *Er hat sich den Arm gebrochen.*

## Sample

### 1. `a lungo` — `lungo` (`ADJ`)

- Matched words: `a, lungo`
- Representative sentence: `Ho camminato a lungo, seguendo una strada poco trafficata.`
- Candidate senses: `e1 : for a long time, at length`
- Gloss: `at length`
- Translation: `for a long time`
- Sentence translation: `I walked for a long time, following a road with little traffic.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — "walked at length / for a long time"

### 2. `prendere in giro` — `prendere` (`VERB`)

- Matched words: `preso, in, giro`
- Representative sentence: `Ossia, credo di averla fatta, perché io a volte non me ne accorgo quando capita qualcosa di buffo o di strano, anche perché se me ne accorgessi eviterei di farlo succedere, e di sentirmi preso in giro eccetera.`
- Candidate senses: `e1 : to make fun of, to tease, to poke fun at; e2 : to string along, take the piss`
- Gloss: `to make fun of`
- Translation: `to make fun of`
- Sentence translation: `That is, I think I did it, because sometimes I don't notice when something funny or strange happens, and also because if I did notice, I would avoid making it happen—and feeling mocked, etc.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 3. `altra volta` — `volta` (`NOUN`)

- Matched words: `altra, volta`
- Representative sentence: `La fogna in strada si è intasata un’altra volta.`
- Candidate senses: `e1 : encore`
- Gloss: `again`
- Translation: `again`
- Sentence translation: `The sewer in the street got clogged again.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 4. `di fronte` — `fronte` (`NOUN`)

- Matched words: `di, fronte`
- Representative sentence: `Dietro una curva a gomito, d’un tratto, me le sono trovate di fronte, le persone sedute.`
- Candidate senses: `e1 : front, fore`
- Gloss: `front`
- Translation: `in front of`
- Sentence translation: `Behind a hairpin bend, all of a sudden, I found the people sitting in front of me.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [ ] Meaning correct in context — verdict: fail — the Gloss "front" is a bare noun; in *me le sono trovate di fronte* the idiom means "in front (of me)", which only the translation says. A learner would learn *di fronte* = "front"

### 5. `in disparte` — `disparta` (`NOUN`)

- Matched words: `in, disparte`
- Representative sentence: `Gioma, detto Sufi, perché era uno che se ne stava in disparte, tranquillo e silenzioso come un monaco, anche se in certe occasioni, be’, era quello che creava più problemi di tutti.`
- Candidate senses: `e1 : isolated or left out on one side; aside`
- Gloss: `apart`
- Translation: `aside`
- Sentence translation: `Gioma, known as Sufi because he was someone who kept to himself, calm and quiet like a monk, although on certain occasions, well, he was the one who caused the most trouble of all.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — "kept apart / to himself" (the anchor lemma *disparta* is an analyzer error, not a meaning error)

### 6. `anche se` — `anche` (`ADV`)

- Matched words: `anche, se`
- Representative sentence: `I soldi che ti servono li guadagnerai lavorando, anche se il lavoro sarà faticoso.`
- Candidate senses: `e1 : even if, even though`
- Gloss: `even if`
- Translation: `even if, even though`
- Sentence translation: `You'll earn the money you need by working, even if the work is tiring.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 7. `per fortuna` — `fortuna` (`NOUN`)

- Matched words: `Per, fortuna`
- Representative sentence: `Per fortuna è intervenuta un’altra insegnante che ha detto di avere pazienza, che avremmo fatto un passo alla volta, che igiene e psicologia potevano anche aspettare, e che avremmo dato la precedenza alle altre materie.`
- Candidate senses: `e1 : luckily, fortunately`
- Gloss: `fortunately`
- Translation: `fortunately`
- Sentence translation: `Fortunately, another teacher intervened and said that we should be patient, that we would take it one step at a time, that hygiene and psychology could wait, and that we would give priority to the other subjects.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 8. `stare in piedi` — `stare` (`VERB`)

- Matched words: `stare, in, piedi`
- Representative sentence: `Sono corso in quella direzione, a quattro zampe, da animaletto; non riuscivo a stare in piedi.`
- Candidate senses: `e1 : to stand; to be standing; e2 : to be well-founded, coherent, or consistent; to hold water (of an argument, reasoning, etc.)`
- Gloss: `to stand`
- Translation: `to stand`
- Sentence translation: `I ran in that direction on all fours, like a little animal; I couldn't stand.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 9. `fare finta di nulla` — `fare` (`VERB`)

- Matched words: `far, finta, di, nulla`
- Representative sentence: `Ho fatto segno di far finta di nulla, come se non ci conoscessimo.`
- Candidate senses: `e1 : synonym of fare finta di niente`
- Gloss: `to pretend nothing is happening`
- Translation: `pretend nothing is happening`
- Sentence translation: `I gestured that we should pretend nothing was happening, as if we didn't know each other.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 10. `e così via` — `così` (`ADV`)

- Matched words: `e, così, via`
- Representative sentence: `Pensavo che se la polizia ci avesse fermato per un controllo e mi avesse visto dormire tranquillo con un quotidiano iraniano in grembo avrebbe pensato che fossi a posto e così via.`
- Candidate senses: `e1 : and so on, and so forth`
- Gloss: `and so on`
- Translation: `and so on`
- Sentence translation: `I thought that if the police had stopped us for a check and seen me sleeping peacefully with an Iranian newspaper on my lap, they would have thought I was okay, and so on.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 11. `di tanto in tanto` — `tanto` (`ADV`)

- Matched words: `di, tanto, in, tanto`
- Representative sentence: `Soltan si lamentava - Hussein Alì credo non avesse fiato per farlo, altrimenti si sarebbe lamentato anche lui, come al solito - e di tanto in tanto si sporgeva mezzo nudo facendo segno con le braccia alle macchine di fermarsi, di darci un passaggio.`
- Candidate senses: `e1 : occasionally, sometimes, now and then, every now and then, from time to time`
- Gloss: `occasionally`
- Translation: `from time to time`
- Sentence translation: `Soltan complained — I believe Hussein Ali did not have the breath to do so, otherwise he would have complained too, as usual — and from time to time he leaned out half-naked, waving his arms at the cars to stop and give us a lift.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 12. `scuola superiore` — `scuola` (`NOUN`)

- Matched words: `scuola, superiore`
- Representative sentence: `A settembre mi sono iscritto alla scuola superiore, una scuola per Operatore dei servizi sociali, e ho fatto subito una figuraccia.`
- Candidate senses: `e1 : high school`
- Gloss: `high school`
- Translation: `high school`
- Sentence translation: `In September I enrolled in high school, a school for social services operators, and immediately made a fool of myself.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 13. `rispetto a` — `rispetto` (`ADV`)

- Matched words: `rispetto, a`
- Representative sentence: `Se parli direttamente con le persone trasmetti un’emozione più intensa, anche se le parole sono incerte e la cadenza è diversa; in ogni caso, il messaggio che arriva assomiglia di più a quello che hai in testa, rispetto a quello che potrebbe ripetere un interprete - o no?`
- Candidate senses: `e1 : with respect to; e2 : in relation to`
- Gloss: `with respect to`
- Translation: `with respect to`
- Sentence translation: `If you speak directly with people, you convey a more intense emotion, even if the words are uncertain and the cadence is different; in any case, the message that comes across resembles more closely what you have in your head than what an interpreter might repeat—or not?`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [ ] Meaning correct in context — verdict: fail — here *rispetto a* means "compared with" (the sentence translation renders it "than"); "with respect to" reads as "regarding". The candidate senses offered no comparative sense

### 14. `su e giù` — `giù` (`ADV`)

- Matched words: `su, e, giù`
- Representative sentence: `Ma adesso eravamo troppo in alto per fare su e giù.`
- Candidate senses: `e1 : up and down; e2 : back and forth`
- Gloss: `up and down`
- Translation: `up and down`
- Sentence translation: `But now we were too high up to go up and down.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 15. `fare la pipì` — `fare` (`VERB`)

- Matched words: `fare, la, pipì`
- Representative sentence: `Quando finalmente sono stato in grado di muovermi e di respirare ho cercato un posto per fare la pipì, tutta la pipì che durante il viaggio non ero riuscito a fare, tutta la pipì che avevo tenuto per tre giorni.`
- Candidate senses: `e1 : to pee; to urinate`
- Gloss: `to pee`
- Translation: `to pee; to urinate`
- Sentence translation: `When I was finally able to move and breathe, I looked for a place to pee—all the pee I had been unable to do during the journey, all the pee I had held in for three days.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 16. `più o meno` — `più` (`ADV`)

- Matched words: `più, o, meno`
- Representative sentence: `Sì, direi diciotto mesi, più o meno.`
- Candidate senses: `e1 : more or less, approximately, around`
- Gloss: `approximately`
- Translation: `more or less`
- Sentence translation: `Yes, I'd say eighteen months, more or less.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 17. `a vicenda` — `vicenda` (`NOUN`)

- Matched words: `a, vicenda`
- Representative sentence: `Jamal, allora, mi ha dato un colpetto con la gamba per svegliarmi (si dormiva vicini, noi due, per proteggerci a vicenda).`
- Candidate senses: `e1 : mutually, reciprocally, each other, one another; e2 : in turn`
- Gloss: `each other`
- Translation: `mutually, reciprocally`
- Sentence translation: `Jamal, then, gave me a little nudge with his leg to wake me up (the two of us were sleeping close together to protect each other).`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 18. `sistema solare` — `sistema` (`NOUN`)

- Matched words: `sistema, solare`
- Representative sentence: `Il cantiere era il sistema solare.`
- Candidate senses: `e1 : solar system`
- Gloss: `solar system`
- Translation: `solar system`
- Sentence translation: `The construction site was the solar system.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — the meaning is right, but this is a transparent compound rather than an idiom

### 19. `acqua di mare` — `acqua` (`NOUN`)

- Matched words: `acqua, di, mare`
- Representative sentence: `Soltan ha provato a bere l’acqua del mare, e dopo la prima boccata ha cominciato a urlare che l’acqua era avvelenata, che i turchi e i greci l’avevano avvelenata per farci morire.`
- Candidate senses: `e1 : seawater`
- Gloss: `seawater`
- Translation: `seawater`
- Sentence translation: `Soltan tried to drink seawater, and after the first mouthful he began shouting that the water was poisoned, that the Turks and Greeks had poisoned it to make us die.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — the meaning is right, but this is a transparent compound; matched through *del* → *di*

### 20. `lavorare duro` — `lavorare` (`VERB`)

- Matched words: `lavoravi, duro`
- Representative sentence: `Lavoravi tutto il giorno, lavoravi duro, e la sera venivi pagato quanto stabilito eccetera.`
- Candidate senses: `e1 : to work hard`
- Gloss: `to work hard`
- Translation: `to work hard`
- Sentence translation: `You worked all day, worked hard, and in the evening were paid what had been agreed, etc.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 21. `da capo` — `capo` (`NOUN`)

- Matched words: `da, capo`
- Representative sentence: `Andare avanti o tornare indietro è la stessa cosa, e io preferisco morire in mare che dover rifare da capo tutta la strada che ho fatto fino ad adesso.`
- Candidate senses: `e1 : from scratch; from the beginning; from the top`
- Gloss: `from scratch`
- Translation: `from scratch; from the beginning; from the top`
- Sentence translation: `Going forward or turning back is the same thing, and I would rather die at sea than have to redo the whole route I have traveled so far from scratch.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass — *rifare da capo* = "redo from scratch / from the start" fits

### 22. `in cerchio` — `cerchio` (`NOUN`)

- Matched words: `in, cerchio`
- Representative sentence: `Ci hanno ordinato di metterci in cerchio, nel cortile, i bambini davanti, perché eravamo più bassi, e gli adulti dietro.`
- Candidate senses: `e1 : in circles`
- Gloss: `in a circle`
- Translation: `in a circle`
- Sentence translation: `They ordered us to stand in a circle in the courtyard, with the children in front because we were shorter, and the adults behind.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 23. `fare finta di niente` — `fare` (`VERB`)

- Matched words: `facendo, finta, di, niente`
- Representative sentence: `Da alzarsi facendo finta di niente, dimenticarlo su una pietra e non trovarlo più, quel giorno.`
- Candidate senses: `e1 : synonym of fare come lo struzzo`
- Gloss: `to pretend nothing happened`
- Translation: `pretend nothing happened`
- Sentence translation: `To get up pretending nothing happened, leave it on a stone, and never find it again—that day.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 24. `non appena` — `non` (`ADV`)

- Matched words: `non, appena`
- Representative sentence: `Ma non appena ha stretto la manopola dell’acceleratore e ha premuto la leva della frizione, chissà perché, la moto si è accesa.`
- Candidate senses: `e1 : as soon as`
- Gloss: `as soon as`
- Translation: `as soon as`
- Sentence translation: `But as soon as he twisted the throttle grip and pressed the clutch lever, for some reason, the motorcycle started.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 25. `fare bene` — `fare` (`VERB`)

- Matched words: `fatte, bene`
- Representative sentence: `Quindi, tanto per essere sicuri e fare le cose fatte bene, ce lo hanno richiesto una terza.`
- Candidate senses: `e1 : to do well (take the prudent approach); e2 : to do the right thing; e3 : Used other than figuratively or idiomatically: see fare, bene.`
- Gloss: `to do well`
- Translation: `to do well`
- Sentence translation: `So, just to be sure and do things properly, they asked us for it a third time.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [ ] Meaning correct in context — verdict: fail — *fare le cose fatte bene* is literal ("do things properly"); *fatte bene* modifies *le cose*, so there is no idiom. "to do well" (to succeed) is wrong here, and candidate sense e3 even marks the literal use

### 26. `ad esempio` — `esempio` (`NOUN`)

- Matched words: `ad, esempio`
- Representative sentence: `Ho sentito della musica e credo di avere chiesto qualcosa: chi stava suonando, ad esempio.`
- Candidate senses: `e1 : for example, for instance`
- Gloss: `for example`
- Translation: `for example`
- Sentence translation: `I heard some music and I think I asked something like: who was playing, for example.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 27. `non ancora` — `non` (`ADV`)

- Matched words: `non, ancora`
- Representative sentence: `Ci sono un sacco di cose che non ho ancora visto nella mia vita e che vorrei vedere, e se a questo aggiungiamo che anche a Qom, anche qui, ogni volta che metto piede fuori dalla fabbrica, anche qui è pericoloso, be’, sapete cosa vi dico?`
- Candidate senses: `e1 : not yet`
- Gloss: `not yet`
- Translation: `not yet`
- Sentence translation: `There are a lot of things I haven't seen yet in my life and would like to see, and if we add to this that in Qom too, here as well, every time I set foot outside the factory, it's dangerous here too, well, you know what I tell you?`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 28. `per via di` — `via` (`NOUN`)

- Matched words: `per, via, di`
- Representative sentence: `Da voi una cosa del genere non potrebbe esistere, per via dell’igiene.`
- Candidate senses: `e1 : because of, due to, by way of`
- Gloss: `because of`
- Translation: `because of`
- Sentence translation: `Where you are, something like that could not exist because of hygiene.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 29. `fare i conti` — `fare` (`VERB`)

- Matched words: `fatto, i, conti`
- Representative sentence: `Lui, in silenzio, ha preso un foglio di carta e ha fatto i conti.`
- Candidate senses: `e1 : to do the math; e2 : to calculate one's earnings and expenses; to budget; e3 : to come to terms [with con ‘with’]; to contend`
- Gloss: `to do the math`
- Translation: `to do the math`
- Sentence translation: `He quietly took a sheet of paper and did the math.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

### 30. `forte e chiaro` — `forte` (`ADJ`)

- Matched words: `forte, e, chiaro`
- Representative sentence: `Lo diremo forte e chiaro, Enaiat.`
- Candidate senses: `e1 : loud and clear`
- Gloss: `loud and clear`
- Translation: `loud and clear`
- Sentence translation: `We’ll say it loud and clear, Enaiat.`

#### Automatic checks

| Check | Result |
| --- | --- |
| outcome is `confirmed` | **PASS** |

#### Reviewer verdicts

- [x] Meaning correct in context — verdict: pass

## Sample items not confirmed

| Headword | Sentence | Outcome | Reason |
| --- | --- | --- | --- |
| `è una parola` | `Vedete, scin è una parola iraniana che significa sasso.` | `literal_use` | The phrase means literally “is a word” here and is not used idiomatically. |
| `come mai` | `Ce ne sono che hanno un odore e un sapore buono e ti sussurrano alle orecchie che sapranno farti stare meglio di come tu potrai mai stare senza di loro.` | `literal_use` | “Come” and “mai” occur separately in the construction meaning “ever,” not as the idiom “come mai” meaning “why” or “how come”. |
| `di sempre` | `Stanco di essere sempre in viaggio.` | `literal_use` | The sentence uses di and sempre separately; it does not contain the idiom di sempre. |
| `di dentro` | `Ci ha portati a casa sua in taxi, uno di quei taxi multipli con un sacco di gente dentro.` | `literal_use` | The sentence uses “dentro” literally to mean physically inside; it does not use the idiom “di dentro.” |
| `buona giornata` | `E’ una buona giornata per voi questa, ha detto.` | `literal_use` | The phrase describes a good day rather than functioning as the farewell idiom. |
| `portare a casa` | `Ci ha portati a casa sua in taxi, uno di quei taxi multipli con un sacco di gente dentro.` | `literal_use` | The sentence uses the expression literally to mean taking people to someone's home. |


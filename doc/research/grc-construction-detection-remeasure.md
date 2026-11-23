# Re-measuring the amended construction definitions: results

**Ticket:** [Task: re-measure the amended first-slice construction definitions](https://github.com/justin-hayes/mouseion/issues/2615).

**Status:** final (2026-11-23).

**Inputs:**

- the amendments from [Task: measure construction detection precision and recall on a human-checked sample](https://github.com/justin-hayes/mouseion/issues/2607) (see its [evidence](grc-construction-detection-measurement.md), *Corrections after review*);
- the [protocol](grc-construction-detection-remeasure/protocol.md), committed before any amended detector ran (`ac1455f6`).

**Whose judgments:**

- Agent sessions judged all 1,126 sampled items against Smyth 1920.
- Justin judged the 4 items the agents left unsure that no reference settled, and a 36-item blind spot-check. Of those 36, 33 were his judgments and 3 were settled by published references (see *Review and anchoring*).
- Where Justin reviewed an item in either study, his judgment is final.
- The agent's Smyth § numbers are cited from memory and unchecked. Check them before the spec cites them.

## Headline

Every amended construction now has a precision **point estimate at or above 0.90** on 120 random hits. Their 95% intervals, though, separate two groups:

- **Clear the bar on the lower bound:** **purpose (ἵνα only)** and **result**.
- **Straddle it:** conditional, articular infinitive, genitive absolute and indirect statement. Their point estimates sit between 0.91 and 0.94, and their lower bounds between 0.84 and 0.88.

Within the four that straddle:

- **Genitive absolute:** every false hit is a parser error (its ceiling is 1.00).
- **Conditional:** the new "whether" exclusion does more harm than good.
- **Indirect statement:** recall is still poor (0.60). The misses spread over a long tail of governing verbs.

The blind spot-check found **no anchoring**. Justin and the agent agreed on 32 of 33 blind items and on every subtype, and the one disagreement is a passage the reference works themselves disagree on.

## Files

- [`grc-construction-detection-remeasure/protocol.md`](grc-construction-detection-remeasure/protocol.md): rates, sample sizes and the review rule, fixed before the run.
- [`occurrences.tsv`](grc-construction-detection-remeasure/occurrences.tsv): one row per sampled item.
  - Columns: work, citation, sent_id, OGA anchor token id, construction, sample stratum, detector verdict and subtype, the matching item from the first measurement, the agent's judgment, subtype, Smyth §, error and note, and Justin's judgment, subtype, Smyth §, error and note.
  - The review column reads `2615-blind-justin`, `2615-unsure-justin`, `2615-blind-reference` or `2615-unsure-reference`, or `2607` where the item carries Justin's judgment from the first review.
  - No Greek text, OGA relations or heads, except a governing-verb lemma outside the curated lists.
- [`results-tables.md`](grc-construction-detection-remeasure/results-tables.md): the generated tables.
- [`detect2.py`](grc-construction-detection-remeasure/detect2.py), [`sample.py`](grc-construction-detection-remeasure/sample.py), [`analyze.py`](grc-construction-detection-remeasure/analyze.py) and [`judging.md`](grc-construction-detection-remeasure/judging.md): the amended detectors, sampler, analysis and judging instructions. They build on the first measurement's scripts in [`grc-construction-detection-measurement/`](grc-construction-detection-measurement/).
- Stays in `~/.cache/scratch/grc-syntax/v2/`: the pinned OGA trees, judging sheets, the review hand-off and its builder, and the reference excerpts.

## Precision against the 0.90 bar

Each construction's precision comes from 120 hits drawn at random from the whole corpus, so it estimates what a learner would see. The verdicts follow the protocol:

- **clears:** the Wilson lower bound is ≥ 0.90;
- **straddles:** the point estimate is ≥ 0.90 but the lower bound is below it;
- **fails:** the point estimate is below 0.90.

| Construction | Corpus hits | Precision (95% CI) | Ceiling (perfect parse) | First measurement | Verdict |
|---|---:|---|---|---|---|
| Conditional | 1,766 | 0.93 (111/120; 0.86–0.96) | 0.95 | 0.81 weighted | straddles |
| Articular infinitive | 625 | 0.92 (110/120; 0.85–0.95) | 0.97 | 0.99 weighted, on 40 | straddles |
| Result | 195 | 0.97 (116/120; **0.92**–0.99) | 0.97 | 1.00 on 40 | **clears** |
| Purpose (ἵνα only) | 160 | 1.00 (120/120; **0.97**–1.00) | 1.00 | 0.60, or 16/16 for ἵνα | **clears** |
| Genitive absolute | 434 | 0.91 (109/120; 0.84–0.95) | 1.00 | 0.87 weighted | straddles |
| Indirect statement | 1,944 | 0.94 (113/120; 0.88–0.97) | 0.98 | 0.78 weighted | straddles |

Two notes on the table:

- Purpose and result samples are large shares of small pools (120 of 160, and 120 of 195). The finite-population correction isn't applied, so their intervals are conservative.
- The articular infinitive scores lower than in the first measurement, which was a point estimate on 40 hits. Across all 240 sampled hits (random plus top-up), the 16 false hits break down as:
  - 8 articles the parser attached to the wrong word (`parser:head`);
  - 3 morphology errors and 1 label error;
  - 4 `detector:rule` cases, where the article governs something other than the infinitive.

Subtypes are gated separately, each on 60 hits or more (random hits plus top-up):

| Construction | Subtype | Corpus hits | Subtype correct | Verdict |
|---|---|---:|---|---|
| Articular infinitive | dative | 68 | 0.98 (59/60; 0.91–1.00) | **clears** |
| | genitive | 145 | 0.95 (57/60; 0.86–0.98) | straddles |
| | nominative | 135 | 0.85 (51/60; 0.74–0.92) | fails: the construction is right 58/60, but τό's case is read wrongly |
| | accusative | 277 | 0.82 (49/60; 0.70–0.89) | fails: mostly false hits from misattached articles |
| Result | natural | 130 | 1.00 (76/76; 0.95–1.00) | **clears** |
| | actual | 61 | 0.90 (54/60; 0.80–0.95) | straddles: 4 are independent "and so" ὥστε |
| Indirect statement | infinitive | 1,379 | 0.95 (81/85; 0.89–0.98) | straddles |
| | ὅτι-ὡς | 565 | 0.92 (55/60; 0.82–0.96) | straddles |

The nominative/accusative failures on the articular infinitive come from τό. It is nominative or accusative by function, and the parser's case tag on τό is unreliable. The dative and genitive (τῷ, τοῦ) are unambiguous in form and score well.

## Recall

Recall uses the protocol's pooled estimator for the amended constructions. The unchanged constructions carry their recall over from the first measurement: articular infinitive 0.98, result 0.91, genitive absolute 0.74.

| Construction | Recall | Ceiling | Where the misses come from |
|---|---|---|---|
| Conditional | **0.92** (first: 0.85) | 0.96 | Loose candidates: 17 of 60 are true, of a pool of 346. "Whether"-excluded εἰ: 24 of 40 are true, of 73. |
| Purpose (ἵνα) | **0.95** | 0.95 | All 9 ἵνα loose candidates are true. Mostly parser attachment. |
| Indirect statement | **0.60** (first: 0.49) | 0.64 | Infinitive loose candidates: 8 of 60 are true, of 4,680. ὅτι-ὡς loose candidates: 35 of 60 are true, of 1,030. |

The first measurement's 80 fully read random sentences, re-classified against the amended candidates, found **no** misses outside the loose queries. As before, zero in 33 occurrences still allows a few per cent.

## Findings for the abstention grilling

These are evidence for [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608). The grilling decides; this measurement doesn't.

1. **The conditional "whether" exclusion should be dropped or narrowed.**
   - *What it does:* the protocol's head-verb list excludes 73 corpus candidates. Only 16 of 40 sampled are true "whether" clauses (0.40, 0.26–0.55). In the other 24, the knowing or asking verb heads the *apodosis* rather than governing the clause. Justin's blind judgment of B6 (R1087) was one of these.
   - *Precision:* the exclusion lifts precision only from about 0.91 to 0.93.
   - *Recall:* it costs about 44 true conditionals, cutting recall from about 0.95 to 0.92.
   - *What would work better:* the remaining "whether" false hits (6 of 9 conditional false hits are `detector:rule`) need a test on clause position or relation, not on the head's lemma. That test is untested.
   - *Missing verbs:* σύνοιδα (B14, LSJ) and a verb of defining are absent from the list.
2. **Genitive absolute: precision is the parser's problem, not the rule's.**
   - *Why:* all 11 false hits are parser errors, mostly a genitive participle agreeing with a genitive governed by a noun, preposition or verb.
   - *Variant B* (the perception exclusion applies only when the head has no other genitive object) readmits 2 candidates. One is true: Symp. 215, which Justin judged yes at low confidence. The other is not. The effect on precision is nil (0.906 against 0.908).
   - *B3, Anab. 1.1.8,* οὐδὲν ἤχθετο αὐτῶν πολεμούντων:
     - Justin judged it a genitive absolute (medium confidence); the agent judged it not.
     - The references split: LSJ, Goodwin & White and Macmichael make the participle an object or cause of ἤχθετο. Kühner–Gerth §482.3 calls it "probably a genitive absolute", Kelsey allows both, and Mather & Hewitt read a causal absolute.
     - So this finding gives no support for adding verbs of emotion to the exclusion list.
3. **Indirect statement: precision is near the bar, and recall is the problem.**
   - *ὅτι-ὡς misses:* 18 of the 35 are `detector:list`, spread over about 20 governing verbs, with no verb accounting for more than 2. Examples: ἐνθυμέομαι, ἐπίσταμαι, ἐνδείκνυμι, μνημονεύω, κατανοέω, αἰσθάνομαι, σημαίνω, ἰσχυρίζομαι.
   - *Lists won't fix it:* a curated list won't close this. The alternative to weigh is ὅτι/ὡς as AuxC with an OBJ-labelled clause under *any* verb, which needs its own precision measurement.
   - *Cross-sentence speech:* reported speech continuing across sentences accounts for 7 misses (`detector:scope`), among them U1, U4, U5 and B21.
4. **Result "actual"** straddles because of independent "and so" ὥστε (4 false hits). Natural result clears outright.
5. **Articular infinitive:** ship by subtype. Dative clears; genitive straddles. Nominative and accusative fail, because τό's case is ambiguous and articles are misattached.
6. **Purpose (ἵνα only) and result clear the bar.** With natural result and the dative articular infinitive, these are the only units whose lower bound reaches 0.90.

## Review and anchoring

### The blind spot-check (36 items)

- *How it was shown:* the protocol drew 36 items, 6 per construction, from items Justin had not reviewed before. They were shown without the agent's judgment or note, and also without the detector's verdict.
- *References:* the agent never saw the references. They came from four separate lookup sessions with no access to the agent's judgments:
  - Smyth 1920, by citation and by Greek overlap;
  - LSJ, by passage URN, and Logeion (LSJ, DGE, Pape);
  - Gorman's gold AGDT trees, where they exist;
  - commentaries: Adam and Jowett & Campbell on the Republic; Burnet, Bury, Hug, Thompson and Archer-Hind on the other Plato works; Goodwin & White, Kelsey, Pretor, Macmichael and Mather & Hewitt on the Anabasis; Burnet, Adam, and Dyer & Seymour on the Apology and Crito;
  - grammars: Goodwin's *Moods and Tenses* and *Greek Grammar*, Gildersleeve's *Syntax*, Kühner–Gerth.
- *Settled items:* a reference settled three blind items explicitly, so they were scored as reference judgments and Justin didn't judge them:
  - B11 (Smyth 2631);
  - B14 (LSJ s.v. σύνοιδα, a "whether" clause);
  - B22 (Smyth 2265).

  The same lookup settled one unsure item, U1 (Adam, and Jowett & Campbell).

| | Agreement with the agent |
|---|---|
| Justin, blind | **32/33** (0.97; 0.85–0.99). The one disagreement is B3, which the references dispute. |
| References, blind | 3/3 |
| Blind overall | **35/36 (0.97; 0.86–1.00)**: passes the ≥ 0.90 bar |
| Subtypes on blind yes items | all agree |
| Justin's first-review judgments, on items drawn again | 27/29 |
| The agent now against the agent in the first measurement | 107/110 |

The first review's 144/148 agreement wasn't blind. The blind result shows it wasn't inflated by anchoring, so the agent-only judgments behind these figures are reliable.

### Unsure items

The agent marked 5 items unsure:

- **U1:** settled by a reference, yes ὅτι-ὡς, an anacoluthic clause resumed under μανθάνω.
- **U2:** genitive absolute, yes (low confidence).
- **U3:** articular infinitive, no. τό belongs to συμφέρον.
- **U4:** indirect statement, infinitive, yes. It depends on a supplied δοξάτω with an opinion-holder.
- **U5:** indirect statement, infinitive, yes. It continues δοκεῖ ἄν … σοι.

Every unsure item now has a judgment, so none drops out of the rates.

## Deviations from the protocol

1. **Conditional exclusion list.** The first measurement's "whether" list was not recorded beyond examples. The protocol fixed its own list of 18 lemmas, which excludes 73 corpus candidates against the 96 the first measurement reported.
2. **The blind items also hid the detector's verdict,** which is stricter than the protocol. **References were shown with each blind item.** They are independent of the agent, so the check still tests anchoring on the agent.
3. **Reference-settled items.** Four items were settled by an explicit published reference instead of Justin's judgment (U1, B11, B14, B22). They are recorded as `reference` and scored against the agent.
4. **B3** was first counted as settled by LSJ. Kühner–Gerth and two Anabasis commentaries disagree, so it went back to Justin.
5. **Error classes on the 5 unsure items** were assigned when recording, because no agent verdict existed to borrow from:
   - U1, U4 and U5: `detector:scope`;
   - U2: `detector:rule`;
   - U3: `parser:head`.
6. **Missing notes.** Justin's answers referred to longer notes ("see above") that were not received with the table. The judgments and confidence levels are recorded, but those notes are not.

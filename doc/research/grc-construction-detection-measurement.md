# Construction detection precision and recall: results (INTERIM)

**Ticket:** [Task: measure construction detection precision and recall on a human-checked sample](https://github.com/justin-hayes/mouseion/issues/2607).
**Status: INTERIM.** Smyth § numbers in the table and notes are the agent's, cited from memory, and must be checked before the spec cites them (the † convention of the inventory grilling). Every figure here is the agent's own judgment against Smyth 1920. Justin's live review of 148 items, in two sittings, is pending. The final figures will use his judgments wherever he reviewed an item (see [`protocol.md`](grc-construction-detection-measurement/protocol.md), committed before the run).

## Files

- [`protocol.md`](grc-construction-detection-measurement/protocol.md): rates, strata, sample sizes and the review rule, all fixed before the run.
- [`occurrences.tsv`](grc-construction-detection-measurement/occurrences.tsv): the per-occurrence table, one row per sampled item. Each row gives work, citation, sent_id, the OGA token id of the anchor, construction, sample stratum, detector verdict and subtype, the agent's judgment with its subtype, Smyth §, error class and note, the review sitting, and Justin's columns (empty until he reviews). It holds no Greek text, OGA relations or heads. The one exception is the lemma of a governing verb outside the curated list, kept for the list's review.
- [`random_occurrences.tsv`](grc-construction-detection-measurement/random_occurrences.tsv): every occurrence found in the 80 fully read random sentences, and whether a hit or a loose candidate covers it.
- [`results-tables.md`](grc-construction-detection-measurement/results-tables.md): the full generated tables, per author and pooled, with Wilson intervals, the detector ceiling and error counts.
- [`detect.py`](grc-construction-detection-measurement/detect.py), [`sample.py`](grc-construction-detection-measurement/sample.py), [`topup.py`](grc-construction-detection-measurement/topup.py), [`analyze.py`](grc-construction-detection-measurement/analyze.py): the prototype detectors and the pipeline. The pinned OGA files and the judging sheets stay in `~/.cache/scratch/grc-syntax/`.

## Headline (interim, pooled across Plato and Xenophon)

P is sampled precision on 40 random hits (20 per author). Recall is the protocol's corpus-weighted estimate. Bar = the provisional precision ≥ 0.90.

| Construction | Corpus hits | P (95% CI) | Plato P / Xen. P | Recall (Plato / Xen.) | Ceiling P / R | Clears 0.90? |
|---|---:|---|---|---|---|---|
| Conditional clause | 1,741 | 0.88 (0.74–0.95) | 0.80 / 0.95 | 0.85 (0.81 / 0.96) | 0.92 / 0.88 | no (Plato) |
| Articular infinitive | 625 | 0.97 (0.87–1.00) | 1.00 / 0.95 | 0.98 (1.00 / 0.84) | 1.00 / 0.98 | yes |
| Result clause | 195 | 1.00 (0.91–1.00) | 1.00 / 1.00 | 0.91 (0.94 / 0.84) | 1.00 / 0.99 | yes |
| Purpose clause | 322 | 0.60 (0.45–0.74) | 0.60 / 0.60 | 1.00 | 0.73 / 1.00 | **no** |
| Genitive absolute | 434 | 0.90 (0.76–0.96) | 0.89 / 0.90 | 0.74 (0.75 / 0.72) | 1.00 / 0.90 | borderline |
| Indirect statement | 2,445 | 0.82 (0.68–0.91) | 0.80 / 0.85 | 0.50 (0.42 / 0.84) | 0.89 / 0.71 | no (infinitive) |

Subtype precision (random hits plus top-up, pooled):

| Construction | Subtype | P |
|---|---|---|
| Articular infinitive | acc. / gen. / dat. / nom. | 1.00 (17/17) / 0.93 (13/14) / 0.90 (9/10) / 0.90 (9/10) |
| Result clause | natural / actual / none (elided verb) | 1.00 (28/28) / 1.00 (10/10) / 1.00 (10/10) |
| Indirect statement | infinitive / ὅτι-ὡς | 0.81 (30/37) / 1.00 (10/10) |

Abstention: result 0.01 (2 ὥστε + participle), genitive absolute 0.04 (3 elided participles, 14 perception heads), the others 0. Subtype abstention: result 12 of 195 hits have no subtype (elided verb). The articular infinitive always has one.

The random sentences (40 per author) found 37 occurrences, all covered by a hit or a loose candidate. No misses fell outside the loose queries, so recall rests on the loose-query samples.

## Findings per construction

**Conditional clause.** False hits are mostly "whether" clauses: indirect questions and εἴτε-style alternatives that the parser labels adverbial (Smyth 2671–2675; 2 parser, 3 detector). Misses are mostly εἴπερ, which the definition's conjunction set leaves out, plus elliptical εἰ clauses with no elided node. *Rule changes this suggests:* add εἴπερ, and exclude εἰ when it sits under a verb of knowing, asking or considering. The Plato precision of 0.80 is the gap to close.

**Articular infinitive.** It clears the bar. The two false hits are article attachment errors. The only true miss is an article placed on the coordinator of two infinitives. *Deviation 1* fixed the loose query; before the fix, recall read 0.37.

**Result clause.** It clears the bar, and every subtype is right. Misses come from parse errors: sentence-initial ὥστε + infinitive left detached from its infinitive (Smyth 2260), and clauses with an elided copula. One of the two "ὥστε + participle" abstentions was a coordinated infinitive result that the rule should have caught.

**Purpose clause.** It fails clearly, and mostly through the rule (9 of 16 false hits; 5 parser morphology, 2 attachment). The rule's ADV requirement does not exclude:

- object clauses of effort with a subjunctive or optative (ὅπως after verbs of striving or caring, Smyth 2210–2214);
- ὡς + superlative with δύναμαι (ὡς … ἂν δύνωνται, Smyth 1086);
- ὡς / ὅπως clauses of manner (Smyth 2462).

The parser also tags ὅπως + future indicative as an aorist subjunctive (Smyth 2211), which defeats the mood test. *Likely fix:* restrict ὡς sharply, or drop it, and exclude ὅπως under verbs of effort. Precision on ἵνα alone looks high but was not sampled separately.

**Genitive absolute.** It sits on the bar (0.90). All four false hits are parser errors: a genitive participle agreeing with a genitive governed by something else, labelled ADV. Misses are mostly subjectless genitive absolutes (Smyth 2072), which the grilling accepted as a recall loss, plus coordinated subjects the rule didn't find.

**Perception-verb exclusion (the hypothesis).** Of the 14 candidates excluded across the whole corpus, 12 are supplementary participles after verbs of hearing or perceiving (Smyth 2112), so excluding them was right, and 2 are genuine genitive absolutes. **The exclusion is 0.86 precise and removes the *Apology* false-positive class.** With it in place, Plato's hit precision is 0.89, where the research had measured 0.39 on the *Apology*.

**Indirect statement.**

- *ὅτι-ὡς* is precise (10/10).
- *Infinitive* is not (0.81). Most false hits (4 of 7) are personal δοκέω "seem to (do)" with no thinker expressed, the hard case the grilling named. The rest are infinitives the parser attached to the opinion verb.
- Plato recall is low (0.42). The curated list is the main cause, at 15 of the 23 classified loose-query misses: ὅτι clauses after verbs of knowing, perceiving, remembering and persuading.
- The other cause is reported speech that continues across a sentence boundary (oblique infinitives with no governing verb in the sentence). This is a scope question rather than a list one.

## Draft verbs-of-saying-and-thinking list (as measured)

φημί, φάσκω, λέγω, εἶπον, ἐρῶ, ἀγγέλλω, ἀπαγγέλλω, ἐξαγγέλλω, ἀποκρίνω (OGA's lemma for ἀποκρίνομαι), ὁμολογέω, ἀρνέομαι, ἀπαρνέομαι, ὑπισχνέομαι, ὄμνυμι, νομίζω, οἴομαι, ἡγέομαι, δοκέω, ὑπολαμβάνω, ἐλπίζω, ὑποπτεύω, πιστεύω.

**Governing verbs of true indirect statements the list missed:** οἶδα (×4), ἀκούω (×2), ὁράω, πείθω, σκέπτομαι, ἀναμιμνῄσκω, μιμνῄσκω, ἐννοέω, διηγέομαι, κρίνω, plus the adjectives δῆλος and καταφανής ("it is clear that"). **False hits caused by list membership:** δοκέω "seem" (4 of the 7 infinitive false hits). The perception-verb list (ἀκούω, ἀκροάομαι, αἰσθάνομαι, πυνθάνομαι) is the one tested above.

## Deviations from the protocol

1. **Articular-infinitive loose query double-counted hits.** An article whose head was an already-detected infinitive was also emitted as a loose candidate, so 13 of the first 14 "misses" were the hit itself. The query now drops articles of hit infinitives: 627 loose candidates removed, and the 14 invalidated sample items were dropped. The stratum was refilled to 15 per author from the corrected pool (seed 26071, items I496–I509).
2. **Judging was delegated** to eight agent sessions (one per construction, indirect statement in two halves, one for the random sentences), all working from the same instructions and scope. I spot-checked the purpose false hits and found them right. The judgments are still the agent's, not Justin's.
3. **Error classes are the judge's reading of the parse**, which it saw only around the anchor. A few `detector:scope` labels on elliptical conditionals might arguably be `parser:ellipsis`.

## Live review (sitting 1 done, sitting 2 pending)

148 items, drawn by `analyze.py --review` (seed 26072). For each construction the draw takes every *unsure* item, up to 10 agent–detector disagreements, and a random draw from the rest. **Sitting 1** (72): conditional, articular infinitive, result. **Sitting 2** (76): purpose, genitive absolute, indirect statement. The final figures replace the agent's judgment with Justin's on reviewed items, and report agent–Justin agreement on the randomly drawn part (92 items).

**Sitting 1 (2026-11-23), done:** 72 items (conditional, articular infinitive, result). Justin agreed with the agent on every judgment and subtype: 72 of 72, including all 49 from the random draw. So the sitting-1 figures above stand as reviewed, and agent–Justin agreement for these three constructions is 1.00. Decisions taken in the sitting:

- **εἴπερ is a conditional protasis.** The εἴπερ misses count as true misses (`detector:scope`). The shipping/abstention grilling should amend the conditional's conjunction set to add εἴπερ (and ἐάνπερ). Of the 103 εἴπερ tokens, 98 already have the hit shape. Elliptical εἴπερ with no clause node stays a silent miss.
- **The result subtype "actual" means ὥστε + any finite mood,** including the potential optative, not the indicative only. A hit without a subtype (elided verb) is the permitted subtype abstention, not an error.

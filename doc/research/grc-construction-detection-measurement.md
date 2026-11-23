# Construction detection precision and recall: results

**Ticket:** [Task: measure construction detection precision and recall on a human-checked sample](https://github.com/justin-hayes/mouseion/issues/2607).

**Status: final** (2026-11-23).

- **Whose judgments.** Justin reviewed 148 items live in two sittings, and the figures use his judgment on every one of them. The other 347 judged items carry the agent's judgment against Smyth 1920.
- **Agent reliability.** The agent agreed with Justin on 145 of the 148 reviewed items. On the randomly drawn part, agreement was ≥ 0.93 for every construction, which clears the protocol's 0.9 reliability bar.
- **Smyth citations.** Many § numbers were cited from memory, and those carry a †. They must be checked before the spec cites them.
- **The texts.** The pinned OGA files include parser training texts (the *Apology*, the *Crito* and the *Anabasis*), so accuracy there may be overstated. The other four Plato works have no gold trees.

## Files

- [`protocol.md`](grc-construction-detection-measurement/protocol.md): rates, strata, sample sizes and the review rule, committed before the run.
- [`occurrences.tsv`](grc-construction-detection-measurement/occurrences.tsv): the per-occurrence table, one row per sampled item. Columns: work, citation, sent_id, the OGA token id of the anchor, construction, sample stratum, detector verdict and subtype, the agent's judgment, subtype, Smyth §, error class and note, the review sitting, and Justin's judgment, subtype, Smyth § and error. It holds no Greek text, OGA relations or heads. The one exception is the lemma of a governing verb outside the curated list.
- [`random_occurrences.tsv`](grc-construction-detection-measurement/random_occurrences.tsv): every occurrence found in the 80 fully read random sentences, and what covers it.
- [`results-tables.md`](grc-construction-detection-measurement/results-tables.md): the generated tables, per author and pooled, with Wilson intervals, the detector ceiling, error counts and agent–Justin agreement.
- [`detect.py`](grc-construction-detection-measurement/detect.py), [`sample.py`](grc-construction-detection-measurement/sample.py), [`topup.py`](grc-construction-detection-measurement/topup.py), [`analyze.py`](grc-construction-detection-measurement/analyze.py), [`record.py`](grc-construction-detection-measurement/record.py): the prototype detectors and the pipeline. The pinned OGA files, trees, judging sheets and review sheets stay in `~/.cache/scratch/grc-syntax/`.

## Figures

P is precision on 40 random hits (20 per author). Recall is the protocol's corpus-weighted estimate. The ceiling assumes a perfect parse, i.e. with parser errors forgiven. Bar is the provisional precision ≥ 0.90.

| Construction | Corpus hits | P (95% CI) | Plato P / Xen. P | Recall (Plato / Xen.) | Ceiling P / R | Clears 0.90? |
|---|---:|---|---|---|---|---|
| Conditional clause | 1,741 | 0.88 (0.74–0.95) | 0.80 / 0.95 | 0.85 (0.81 / 0.96); **0.91 with εἴπερ** | 0.92 / 0.88 | no (Plato) |
| Articular infinitive | 625 | 0.97 (0.87–1.00) | 1.00 / 0.95 | 0.98 (1.00 / 0.84) | 1.00 / 0.98 | **yes** |
| Result clause | 195 | 1.00 (0.91–1.00) | 1.00 / 1.00 | 0.91 (0.94 / 0.84) | 1.00 / 0.99 | **yes** |
| Purpose clause | 322 | 0.60 (0.45–0.74) | 0.60 / 0.60 | 1.00 | 0.75 / 1.00 | **no** |
| Genitive absolute | 434 | 0.88 (0.74–0.95) | 0.85 / 0.90 | 0.74 (0.74 / 0.74) | 1.00 / 0.91 | no (just under) |
| Indirect statement | 2,445 | 0.80 (0.65–0.90) | 0.75 / 0.85 | 0.49 (0.41 / 0.84) | 0.86 / 0.65 | no (infinitive) |

Subtype precision (random hits plus top-up, pooled):

| Construction | Subtype | P |
|---|---|---|
| Articular infinitive | acc. / gen. / dat. / nom. | 1.00 (17/17) / 0.93 (13/14) / 0.90 (9/10) / 0.90 (9/10) |
| Result clause | natural / actual / none | 1.00 (28/28) / 1.00 (10/10) / 1.00 (10/10) |
| Indirect statement | ὅτι-ὡς / infinitive | **1.00 (10/10)** / 0.78 (29/37) |

**Abstention:**

- Result: 0.01 (2 ὥστε + participle; one of them was a coordinated infinitive result the rule should have caught).
- Genitive absolute: 0.04 (3 elided participles and 14 perception heads).
- The other four: 0.
- Subtype abstention: 12 of the 195 result hits show no subtype (elided verb). Justin confirmed this is permitted, not an error.

**Random sentences:** 40 per author found 37 occurrences, all covered by a hit or a loose candidate. No miss fell outside the loose queries, so recall rests on the loose-query samples.

## Findings and the definition changes they point to

The changes below are **evidence for [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608)**. That grilling decides them; this measurement doesn't.

**Conditional clause.**

- *False hits:* "whether" clauses, indirect questions and εἴτε … εἴτε alternatives, labelled adverbial (Smyth 2671–2675†).
- *Misses:* mostly εἴπερ. **Justin ruled εἴπερ a conditional protasis.** Of the 103 εἴπερ tokens, 98 already have the hit shape, and adding them lifts recall to 0.91 (Plato 0.89).
- *Changes:* add εἴπερ (and ἐάνπερ) to the conjunction set. Exclude εἰ under a verb of knowing, asking or considering, to close Plato's precision gap. Elliptical εἴπερ with no clause node stays a silent miss.

**Articular infinitive.** It ships as defined. Both false hits are article attachment errors, and every subtype is ≥ 0.90.

**Result clause.**

- *Ships as defined.* The misses are parse errors: sentence-initial ὥστε detached from its infinitive, and clauses with an elided copula.
- *Changes:* **Justin ruled that "actual" means ὥστε + any finite mood** (including the potential optative), not the indicative only. Under coordinated infinitives, prefer the infinitive over a participle.

**Purpose clause.** It fails clearly. Of the 16 false hits, 8 come from the rule:

- effort and concern clauses with ὅπως/ὡς + subjunctive or optative after verbs of striving, trying or taking care, which **Justin confirmed are not purpose** (Smyth 2209–2214†);
- ὡς / ὅπως + superlative + δύναμαι (Smyth 1086†);
- clauses of manner (Smyth 2462†).

Five more are the parser tagging ὅπως + future indicative as aorist subjunctive, which defeats the mood test. Even on a perfect parse the ceiling is only 0.75.

*Changes:* restrict ὡς, which has genuine final uses (for example *Anab.* 1.9.27), so dropping it costs recall. Exclude ὅπως under verbs of effort and care, any clause containing a superlative + δύναμαι, and correlative οὕτως … ὅπως. Then re-measure, or defer the construction.

**Genitive absolute.** At 0.88 it sits just under the bar. Every false hit is a parser error: a genitive participle agreeing with a genitive governed by something else, labelled ADV. One of them (*Rep.* 535) **Justin ruled a genitive of cause governed by ὑπεραγανακτέω (LSJ).** Misses are mostly subjectless absolutes (Smyth 2072), the recall loss the inventory already accepted.

**Perception-verb exclusion (the hypothesis).**

- *It works.* Across the whole corpus it excluded 14 candidates. 12 are supplementary participles after ἀκούω, ἀκροάομαι, αἰσθάνομαι or πυνθάνομαι (Smyth 2112†), so excluding them was right; 2 are genuine absolutes. That makes the exclusion 0.86 precise.
- *It removes the Apology false-positive class* that the parser-accuracy research measured at 0.39.
- *Its one failure pattern:* a hearing verb that already has its object, as in *Symp.* 215, ἀκούῃ … τῶν σῶν λόγων ἄλλου λέγοντος. A refinement would exclude only when the head has no other genitive object.
- *A candidate for the same list:* ἀνέχομαι + genitive + participle (*Anab.* 2.2.1), which behaves the same way.

**Indirect statement.**

- *ὅτι-ὡς* is precise (10/10) and ships.
- *Infinitive* is not (0.78). False hits:
  - personal δοκέω "seem to (do)" or "be reputed to". **Justin's rule: personal δοκέω is never labelled**; one clear rule beats separating "seem" from "be reputed";
  - ὁμολογέω "agree **to** (do)" with a present infinitive (*Rep.* 547), which is an object infinitive, not indirect statement;
  - infinitives the parser attached to the opinion verb.
- *Recall* is poor in Plato (0.41). The curated list is the main cause, at 15 of the 23 loose-query misses.
- *Cross-sentence speech:* reported speech that continues across a sentence boundary is true indirect statement. Justin classed it `detector:scope`, because a per-sentence detector can't see the governing verb. Treat it as accepted recall loss.
- *Personal passive:* a personal passive with a nominative subject (ἐλέγετο φυλακὴ φυλάττειν) is true indirect statement. The definition's "accusative SBJ or none" excludes it whenever the parser attaches the nominative, so the definition should allow a nominative subject under a passive verb of saying.

## Verbs-of-saying-and-thinking list

**As measured (draft):** φημί, φάσκω, λέγω, εἶπον, ἐρῶ, ἀγγέλλω, ἀπαγγέλλω, ἐξαγγέλλω, ἀποκρίνω (OGA's lemma for ἀποκρίνομαι), ὁμολογέω, ἀρνέομαι, ἀπαρνέομαι, ὑπισχνέομαι, ὄμνυμι, νομίζω, οἴομαι, ἡγέομαι, δοκέω, ὑπολαμβάνω, ἐλπίζω, ὑποπτεύω, πιστεύω.

**Missed governing verbs** (from true indirect statements the list didn't catch): οἶδα (×4 in the sample, ×3 in the random sentences), ἀκούω (×2), ὁράω, πείθω, σκέπτομαι, ἀναμιμνῄσκω, μιμνῄσκω, ἐννοέω, διηγέομαι, κρίνω, ἐπιλανθάνομαι, plus δῆλος / καταφανής ("it is clear that", with a full clause) and the noun λόγος (λόγον ἐκφέρει ὅτι).

**What this suggests for the list's review on the implementation issue:**

1. Add διηγέομαι and ἐννοέω outright. They are plain verbs of saying and thinking.
2. Add οἶδα, ἐπίσταμαι, μανθάνω, ὁράω, ἀκούω, (ἀνα)μιμνῄσκω and πείθω **for the ὅτι-ὡς subtype only.** With an infinitive or a participle they mean something else ("know how to", "persuade to do", the participle constructions deferred in the inventory).
3. Gate ὁμολογέω, ὑπισχνέομαι and ὄμνυμι on a **future** infinitive, or drop them from the infinitive subtype.
4. Keep δοκέω only when the thinker is expressed in the dative or the verb is impersonal, and never label the personal construction.

Whether "verbs of saying and thinking" should widen to knowing and perceiving is a scope decision for [Grilling: abstention rule and which constructions ship](https://github.com/justin-hayes/mouseion/issues/2608). By Smyth, all of these are indirect statement.

## Deviations from the protocol

1. **The articular-infinitive loose query double-counted hits.** It emitted the article of an already-detected infinitive as a separate candidate. The query now drops articles of hit infinitives: 627 candidates removed, and 14 invalidated sample items dropped. The stratum was refilled to 15 per author from the corrected pool (seed 26071, items I496–I509).
2. **Judging was delegated** to eight agent sessions working from the same instructions. The agent's false hits for purpose were spot-checked and held up, and the live review then measured the agent against Justin.
3. **Error classes were revised in review where Justin agreed.** The error class changed on I244 (`parser:label`), on I398 and I415 (`detector:scope`, cross-sentence speech), and on I229 and I231 (no error, since the missing subtype is permitted abstention). Unsure items I302 and I368 became **no**. I423 stays unsure and drops out of the rates.

## Live review

148 items in two sittings on 2026-11-23, drawn by `analyze.py --review` (seed 26072). For each construction the draw took every unsure item, up to 10 agent–detector disagreements, and a random draw from the rest. In sitting 2 the agent also gave its own recommendation on each item.

| Construction | Reviewed | Agreement (all reviewed) | Agreement (random part) |
|---|---:|---|---|
| Conditional | 24 | 24/24 | 14/14 |
| Articular infinitive | 24 | 24/24 | 20/20 |
| Result | 24 | 24/24 | 15/15 |
| Purpose | 24 | 24/24 | 14/14 |
| Genitive absolute | 26 | 24/26 (2 unsure → no) | 14/14 |
| Indirect statement | 26 | 25/26 (ὁμολογέω, yes → no) | 14/15 |

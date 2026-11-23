# Re-measuring the amended construction definitions: protocol

**Ticket:** [Task: re-measure the amended first-slice construction definitions](https://github.com/justin-hayes/mouseion/issues/2615).
**Status:** fixed before any amended detector was run (2026-11-23). Changes after the run are listed under *Deviations* in the results file and are never edited in here.

This protocol extends the [first measurement's protocol](../grc-construction-detection-measurement/protocol.md). Its definitions of truth (the Greek read against Smyth 1920, never the parse), its rates, its error classes and its ADR 0111 rule on what is committed all carry over unchanged. Only the definitions, the sample and the review change.

## The amended definitions

These come from the corrections section of the [first measurement's evidence](../grc-construction-detection-measurement.md).

| Construction | Change |
|---|---|
| Conditional | Conjunctions: εἰ, ἐάν, **εἴπερ**. **Exclude** εἰ whose head, looked up through COORD, has one of these lemmas: οἶδα, ἐπίσταμαι, γιγνώσκω, σκοπέω, ἐπισκοπέω, διασκοπέω, σκέπτομαι, ἐρωτάω, ἀνερωτάω, ἔρομαι, πυνθάνομαι, βουλεύω, ζητέω, ἐξετάζω, ὁράω, μανθάνω, ἀπορέω, ἐννοέω. Excluded candidates form their own stratum. |
| Purpose | ἵνα only (ἵνα μή included). ὅπως and ὡς final clauses are out of the first-slice scope, so they are neither hits nor misses. |
| Indirect statement, infinitive | δοκέω removed from the list. ὑπισχνέομαι and ὄμνυμι count only with a **future** infinitive. διηγέομαι and ἐννοέω added. |
| Indirect statement, ὅτι-ὡς | διηγέομαι and ἐννοέω added. οἶδα, ἐπίσταμαι, μανθάνω, ὁράω, ἀκούω, μιμνῄσκω, ἀναμιμνῄσκω and πείθω added for this subtype only. δοκέω stays in this subtype's list. OGA spells μιμνῄσκω both with and without the iota subscript, so both spellings are listed. |
| Genitive absolute | Unchanged (variant A). A refined variant B also runs: the perception exclusion applies only when the head has no other genitive object. |
| Articular infinitive | Unchanged. |
| Result | Unchanged, except that the "actual" subtype is ὥστε + any finite mood (indicative, subjunctive, optative or imperative). |

The nominative-subject personal passive ("acc. SBJ or none") is **not** amended here. It is a recall question, outside the amendments the ticket lists.

## Rates

The rates are the first protocol's, with two changes.

1. **Hits are sampled pooled, in proportion to the corpus.** Hits are drawn as one simple random sample across all 7 Works, not 20 per author. The sampled precision is then an unbiased estimate of the corpus precision a learner sees, and its Wilson interval applies directly. Per-author precision is still reported, on the items each author contributes.
2. **The verdict against the bar uses the interval:**
   - **clears**: the 95% Wilson lower bound is ≥ 0.90;
   - **straddles**: the point estimate is ≥ 0.90 but the lower bound is below it;
   - **fails**: the point estimate is below 0.90.

   The verdict is reported for each construction and each gated subtype.

Recall uses the first protocol's estimator. For the amended constructions it uses a fresh loose-query sample from the amended candidate pools. For the unchanged constructions, recall is carried over from the first measurement, since their loose queries and hit sets have not changed. Misses outside the loose queries (k̂) come from the first measurement's 80 fully read random sentences, re-classified against the amended candidates. ὅπως/ὡς purpose occurrences found there are dropped, because they are now out of scope.

The finite population correction is not applied. Where a sample is a large share of its pool (purpose, result), the Wilson interval is therefore conservative.

## Sample (seed 2615)

| Stratum | Size | Purpose |
|---|---:|---|
| Detector hits, pooled random | **120 per construction** (or every hit, where there are fewer) | construction precision |
| Subtype top-up | until each subtype has **≥ 60** judged hits, or every hit where there are fewer. This covers articular-infinitive case, result natural/actual, and indirect statement infinitive/ὅτι-ὡς. Result "none" is subtype abstention and is not topped up. | subtype precision |
| Loose-query non-hit candidates, pooled random | **60** for conditional, purpose, indirect statement infinitive and indirect statement ὅτι-ὡς | recall of the amended definitions |
| Conditional "whether"-excluded candidates | **40** random | what the exclusion costs (recall), and its precision |
| Genitive-absolute perception exclusions | every one (A and B) | variant B precision |

With 120 hits, a construction whose precision is 0.95 has a lower bound of about 0.89. A construction that makes at most 4 errors in 120 hits (0.97) has a lower bound of about 0.92. With 60 hits, a subtype clears only when it makes at most 1 error (60/60 gives 0.94; 59/60, 0.91; 58/60, 0.89).

**Reuse.** When a sampled item (the same work, sent_id, anchor token and construction) was also judged in the first measurement, the agent still judges it afresh, but Justin's earlier judgment overrides the agent's if he reviewed it. Earlier *agent* judgments are not reused; they feed only an agent–agent consistency figure.

## Judging and review

1. Agent sessions judge every sampled item against the amended scope, using the first measurement's judging instructions amended for this run (`judging.md`), so that each item records true / false / unsure, a subtype, a Smyth § and an error class.
2. **Justin reviews only:**
   1. every item an agent marks **unsure**, shown with the agent's note but no verdict;
   2. a **blind spot-check of 36 items**: 6 per construction, drawn at random (seed 26151) from the judged hit and loose items that Justin has not reviewed before. They are shown **without** the agent's judgment or note, and are interleaved in random order.
3. **Anchoring test.** Agent–Justin agreement on the blind items is reported overall and per construction. The pass condition is overall blind agreement ≥ 0.90 (at most 3 disagreements in 36). If the spot-check fails, the agent-only figures are flagged unreliable, and the constructions with disagreements get a second blind draw before any figure is called final. The first measurement's non-blind agreement (144/148) is reported beside it for comparison.
4. **Final figures** use Justin's judgment wherever he gave one (in this review or the first), and the agent's elsewhere. Items still unsure after review drop out of the rates.

## What gets committed

The same rule as the first measurement (ADR 0111): the public `research/` branch holds IDs, verdicts, judgments, Smyth §, error classes and plain-English notes only. It holds no Greek text, OGA relations or heads, except a governing-verb lemma outside the curated lists. Trees, sheets and review sheets stay in `~/.cache/scratch/grc-syntax/v2/`.

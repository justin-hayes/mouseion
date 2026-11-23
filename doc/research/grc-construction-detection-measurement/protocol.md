# Construction detection precision and recall: protocol

**Ticket:** [Task: measure construction detection precision and recall on a human-checked sample](https://github.com/justin-hayes/mouseion/issues/2607).
**Status:** fixed before any detector was run (2026-11-23). Changes after the run are listed under *Deviations* in the results file, never edited in here.

## What is measured

The six first-slice definitions from [Grilling: first-slice construction inventory and definitions](https://github.com/justin-hayes/mouseion/issues/2604#issuecomment-7098688802), run as prototype detectors over the native AGDT trees of the pinned OGA `oga_src` v0.2.0 files (all 7 Works). The SHA-256 of each file is checked against `internal/classical/layers.json`. Each subtype is reported separately:

| Construction | Subtypes reported |
|---|---|
| Conditional clause (protasis) | none |
| Articular infinitive | article case: nom. / gen. / dat. / acc. |
| Result clause | natural (infinitive) / actual (indicative) / none (elided verb) |
| Purpose clause | none |
| Genitive absolute | none; also reports the perception-verb exclusion as its own stratum |
| Indirect statement | infinitive / ὅτι-ὡς clause |

**Truth** is the construction as the grilling scoped it, judged by reading the Greek against Smyth (1920 numbering). It is never judged against the AGDT shape or against gold trees. For example, εἴπερ "if really" is a conditional protasis by Smyth, so a missed εἴπερ is a recall loss of the *definition*. Bare μή + subjunctive is outside the scoped purpose clause, so it is not a miss. Every judgment names a Smyth §.

## Rates

For each construction × subtype × author (Plato, Xenophon), and pooled:

- **Precision** = correct hits ÷ hits, over a random sample of detector hits. A hit is correct when the anchor is a true occurrence of the construction. For a subtype, the subtype must also be right. A hit whose construction is right but whose subtype is wrong counts as correct for the construction and wrong for the subtype.
- **Recall** = correct hits ÷ (correct hits + missed occurrences). Missed occurrences are found in two places:
  1. **Loose-query candidates:** a deliberately looser query per construction (below) whose non-hit results are sampled and read.
  2. **Random sentences:** fully read sentences. Any true occurrence there that is neither a hit nor a loose candidate is a miss *outside* the loose query.

  Corpus-level estimate per author: R̂ = H·p̂ ÷ (H·p̂ + C·m̂ + N·k̂), where H = hits, p̂ = sampled precision, C = non-hit loose candidates, m̂ = sampled fraction of them that are true occurrences, N = sentences in the stratum, and k̂ = true occurrences per random sentence that fall outside both the hits and the loose query. Pooled R̂ sums the per-author terms. Raw counts are reported alongside.
- **Abstention rate** = abstained candidates ÷ (hits + abstained candidates), where an abstained candidate matches a definition's shape but is withheld by a stated abstention rule: ὥστε + participle; a genitive absolute whose participle is elided; a perception-verb head. **Subtype abstention** = hits shown without a subtype ÷ hits. Abstained candidates that are true occurrences count as misses in recall.
- Intervals are 95% Wilson intervals on the sampled proportions.

## Error attribution

Every false hit and every miss gets exactly one class:

- `parser:label`: wrong relation label (ADV/OBJ, SBJ/OBJ …);
- `parser:head`: wrong attachment;
- `parser:morph`: wrong mood, case, gender or part of speech;
- `parser:lemma`: wrong lemma;
- `parser:ellipsis`: a missing or misplaced elided node;
- `detector:rule`: the tree is right by AGDT and the rule mishandles it;
- `detector:list`: indirect statement only; the governing verb is missing from the curated list, or wrongly in it;
- `detector:scope`: the definition's scope excludes a true occurrence (εἴπερ, for example).

Precision and recall are also reported with parser errors only, assuming a perfect tree (the "detector ceiling").

## Sample (seed 2607)

Strata per construction and author. Plato pools its six Works, so the sample follows each Work's share of hits.

| Stratum | Per construction × author | Purpose |
|---|---:|---|
| Detector hits | 20 random, plus a top-up so that every subtype has ≥ 10 pooled where the corpus has that many | precision |
| Loose-query non-hit candidates | 15 random (indirect statement: 15 per subtype) | recall |
| Abstained candidates | up to 10 random | abstention, recall |
| Perception-excluded genitive absolutes | every one, up to 40 pooled | tests the exclusion hypothesis |
| Random sentences (≥ 4 real tokens) | 40 per author, read for all six | recall outside the loose query |

Loose queries (non-hits only):

1. **Conditional:** any εἰ, ἐάν, εἴπερ, ἐάνπερ, any AuxC ἄν, or a form ἐάν / ἤν / κἄν / κεἰ, whatever its relation.
2. **Articular infinitive:** an infinitive with a neuter singular article among the 8 preceding tokens of its sentence; or a neuter singular article headed by a verb or an elided node.
3. **Result:** any ὥστε.
4. **Purpose:** any ἵνα, ὅπως or ὡς with a subjunctive or optative in its subtree or among the 10 following tokens.
5. **Genitive absolute:** any genitive participle; any elided ADV node with a genitive SBJ.
6. **Indirect statement:** (infinitive) any infinitive whose head, through COORD, is a list verb, or that has an accusative SBJ, or that is OBJ of any verb; (ὅτι-ὡς) any AuxC ὅτι, or AuxC ὡς with an OBJ/SBJ child.

## Judging and the live review

1. The agent judges every sampled item: true / false / unsure, the subtype, a Smyth 1920 §, an error class, and a note. These are the **interim** figures.
2. **Live review (about 150 items, two sittings):** for each construction, every item judged *unsure*, then the items where the agent disagrees with the detector (false hits, misses), up to 10, then a random draw from the rest to make about 25. Sitting 1 covers conditional, articular infinitive and result. Sitting 2 covers purpose, genitive absolute and indirect statement.
3. **Final figures** use Justin's judgment wherever he reviewed an item and the agent's elsewhere. Agent–Justin agreement on the randomly drawn part is reported per construction. Where it is below 0.9, that construction's agent-only judgments are flagged as unreliable.

## What gets committed

Per [Research: what ADR 0111 permits for OGA-derived syntax and constructions shown to learners](https://github.com/justin-hayes/mouseion/issues/2602#issuecomment-7098154703), the public `research/` branch holds only occurrence IDs (work, citation, sent_id, OGA token id), construction, subtype, stratum, detector verdict, judgments, Smyth §, error class, and plain-English notes with no Greek text, OGA relations or heads. Everything copied from OGA (trees, text, the judging sheets) stays in `~/.cache/scratch/grc-syntax/`. The curated verb lists are Mouseion-authored and are committed.

# German highlight width check

Issue: #2782 (parent #2759). Research only: no code or contract change and no provider call.

## Question

The Greek evidence run (`doc/evidence/greek-study-evidence-run-2026-11-27.md`) showed English highlights wider than the target, for example "with three hundred Greek hoplites" for ἔχων. The modern contextual-Gloss contract words its alignment instruction the same way, so does German show the same fault?

## What data exists

The repository holds **no recorded German model output that pairs a sentence translation with highlights**:

- `doc/reviews/contextual-gloss-v15`, `v16`, `v17` and `v17-rerun` record Glosses, translations, evidence IDs and unresolved kinds. They contain no sentence translations or highlights.
- `doc/reviews/english-target-alignment.md` states that the configured-model alignment review was **not performed**. It certifies fixtures only.
- `doc/evidence/` holds no German highlight data. `german-separable-verb-precision.md` is about lemma mismatches.

The only German sentence-translation and highlight pairs are hand-written test fixtures:
`internal/enrichment/testdata/multilingual_alignment_quality.json` and `contextual_gloss_unresolved_contract.json`. Authors wrote these fixtures. A model did not produce them, so they say little about model behaviour.

## Sample

Eight German fixtures. Five carry a non-empty highlight proposal and three carry an empty one.

| Fixture | Target | English | Highlight | Width |
| --- | --- | --- | --- | --- |
| German contiguous target | Haus | The house stands next to the old station. | `house` | tight |
| German discontinuous target | haut … um | And then the motorcycle knocks Piero over. | `knocks`, `over` | tight (excludes subject and object "Piero") |
| ambiguous omission | Bank | She sees the old bank at the end of the street. | `bank` | tight (excludes "old") |
| stray kind discarded | sieht | She sees the old garden behind the house. | `sees` | tight (excludes the object) |
| structurally invalid proposal | Bahnhof | The station is old. | `train station` | miss: not in the sentence, so discarded |
| lemma_mismatch (sehen/fern) | – | – | none | empty |
| lemma_mismatch (kaufen/ein) | – | – | none | empty |
| batch output (Baum) | – | – | none | empty |

## Counts

- Highlights with arguments or modifiers: **0 of 5**.
- Highlights that miss the target: **1 of 5**. The validator discards that one, so it never reaches Study.
- Tight highlights: **4 of 5**.

All five highlights come from fixtures an author wrote. They do not predict real model behaviour, and 5 highlights cannot show that the fault is absent.

## Deterministic post-processing already applied

`ValidateSentenceTranslationTargets` (`internal/enrichment/target_alignment.go`) is called from `contextual.go:143`, `llm_codec.go:1112`, `fluency_agreement.go:146,200` and `cardexport.go:397,1479,1539`. It discards the whole alignment when:

- an excerpt is empty, padded, unsafe or contains `<` or `>`;
- an excerpt is not an exact, unique, word-bounded match in the sentence;
- excerpts are out of order or overlap;
- the excerpts together cover every word of a sentence with more than one word.

It checks **safety, not width**. It does not stop "with three hundred Greek hoplites" from passing, because that is far from the whole sentence. Only a prompt rule or a length cap can narrow highlights.

## Prompt wording

The modern contract and the older translation contract say the excerpts "identify only the smallest meaning-bearing words or phrases corresponding to the supplied target" and "Do not include the whole sentence or unrelated words". Neither forbids the target's subject, object, complements or modifiers. They rule out unrelated words only, and arguments are related. This is the looseness the Greek run exposed, and #2779 closes it for the Greek and Latin passes.

## Recommendation

**Apply #2779's tightened alignment rule to the modern contract, as a separate decision.** The evidence does not show a German fault, but it cannot show that German is fine either: the repo has no recorded model output with German highlights. The wording gap is real, and the validator does not cover it. Before changing the contract:

1. Record a small configured-model German run, which is the review `english-target-alignment.md` left pending, and count over-wide highlights with the method above.
2. If any are over-wide, add the #2779 rule and bump the modern prompt version. That move needs its own assessment, because a version bump invalidates cached German results.
3. If none are, record that German is fine.

Until then, #2779 correctly leaves the modern contract untouched.

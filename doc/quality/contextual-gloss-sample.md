# Contextual Gloss quality sample

Status: **Protocol** · Results are maintained as reviewed evidence

This protocol defines a repeatable comparison of historical sense-list/fallback
card meanings with current contextual Glosses. It does not itself contain or
claim reviewed model outputs. Keep unreviewed collection notes out of the repo;
promote a completed sample here only after its source records and human judgments
are stable. Do not mark the feature acceptance criterion complete until that
reviewed sample is recorded.

## Freeze the comparison

For each case, save one immutable record containing:

- Case ID, language, sentence, target surface form, canonical lemma, and POS.
- Exact historical card Gloss and whole-sentence translation, taken from the
  historical artifact (not reconstructed from today's dictionary index).
- Exact frozen current candidate payload, including evidence IDs, glosses,
  origins, match strengths, and index version. Record an explicit empty payload
  when evidence is absent.
- Exact current response and rendered Gloss, or the item-level unresolved,
  invalid, or ambiguous outcome. Keep the sentence translation alongside it.
- Provider/model identifier, contextual prompt version, request date, and any
  retry or manual exclusion. Do not include credentials, learner identifiers,
  or book metadata.
- Reviewer, review date, per-dimension judgments, notes, and disposition.

Use the same case sentence and target in both generations. The historical side
is a control, not ground truth; a re-preparation may refresh evidence, so preserve
both generations' candidate payloads. If no historical artifact exists for a
case, label it `new-control` and exclude it from before/after aggregate claims.
Do not regenerate a completed record in place: add a new sample revision and
record the reason.

## Case selection

Select actual historical card cases from each language rather than treating
constructed examples as gold labels. Confirm sentence, target analysis, and
intended reading with the reviewer before including a case. Preserve enough
cases across `de`, `it`, and `el` to cover every dimension below.

| Coverage dimension | Case-selection requirement |
| --- | --- |
| Polysemy | Context chooses among two or more plausible senses. |
| Function words | Contextual English cue is natural, not a list of dictionary senses. |
| Idioms | Idiomatic reading is distinguished from a plausible literal reading. |
| Absent evidence | No supplied candidate supports the target; assess context-only inference or omission. |
| Conflicting evidence | Supplied candidate(s) conflict with context; assess whether the output resists them. |
| Context-only inference | No cited evidence; reviewer assesses whether the sentence supports the inference. |
| Missing POS | Only lemma-only weaker evidence is available; assess its use. |
| Invalid / ambiguous outcome | Include invalid provider data and a valid unresolved outcome; assess safe rejection or omission. |

The completed sample must cover every row across `de`, `it`, and `el`. Invalid
provider data may be represented by offline decoder fixtures; it is not a live
model wording assertion.

## Rubric

Score each applicable dimension `0` (fail), `1` (mixed), or `2` (pass), and cite
the sentence/evidence that supports the judgment. Use `N/A` only for dimensions
that genuinely do not apply.

| Dimension | 0 — fail | 1 — mixed | 2 — pass |
| --- | --- | --- | --- |
| Contextual correctness | Wrong sense or contradicts the sentence. | Plausible but underdetermined or imprecise. | Captures the target's contextual meaning. |
| Brevity | Unfocused, list-like, or needlessly long. | Understandable but can be materially shortened. | Concise; longer wording only prevents a misleading cue. |
| English wording | Unnatural, ungrammatical, or not a usable cue. | Understandable with a wording defect. | Idiomatic, clear English for a learner. |
| Evidence discipline | Unsupported claim or misuses conflicting evidence. | Evidence relation is unclear but not plainly false. | Cites fitting evidence, or appropriately marks context-only inference. |
| Outcome handling | Invalid/ambiguous result is published as a confident Gloss. | Safely omitted but reason/diagnostic is inadequate. | Invalid is rejected; unresolved is omitted with a useful reason. |

For each generation, record scores and a case disposition (`better`, `same`,
`worse`, `not comparable`, `invalid`, or `unresolved`). A score is not a
statistical quality claim: report counts and concrete regressions, not an
unqualified average. Treat any unsupported meaning, wrong contextual sense, or
published invalid/ambiguous result as an actionable regression regardless of
brevity or aggregate score.

## Required reviewed record

For each case, retain the frozen inputs and outputs listed under “Freeze the
comparison,” then record rubric scores for both generations, a disposition
(`better`, `same`, `worse`, `not comparable`, `invalid`, or `unresolved`), and
reviewer notes citing the sentence/evidence. Report counts and concrete
regressions, not an unqualified score average.

The reviewed record must identify the reviewer and review date; historical
artifact(s); index version(s); provider/model; contextual prompt version; sample
revision; limitations; overall decision; and actionable follow-up regressions.
State that a small purposive sample is not representative of all lemmas, genres,
senses, or provider behavior, and that valid response shape does not prove
semantic correctness. Conclusions apply only to the recorded
source/prompt/provider identities.

## Offline checks

Deterministic tests may verify frozen-input identity, valid evidence references,
context-only/unresolved consistency, rejection of malformed or unsupported
outcomes, and omission behavior. They must not call a live model or assert a
particular model wording. Semantic judgments and the completed before/after
review remain human work.
